// Package view renders an HTML report of a keiri inventory.
package view

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/O6lvl4/keiri/internal/config"
	"github.com/O6lvl4/keiri/internal/inventory"
)

//go:embed template.html
var tplSource string

var tpl = template.Must(template.New("report").Parse(tplSource))

type cell struct {
	Display string
	Class   string
	Portal  string // URL set on missing cells when a portal is configured
	Year    string // YYYY of this column's month (for year filtering in the viewer)
}

type row struct {
	Category string
	Required bool
	Optional bool
	Marker   string
	Portal   string
	Cells    []cell
	Total    int
}

type gap struct {
	Category string
	Status   string // "complete" | "missing"
	Missing  []string
	Portal   string // URL
}

type pill struct {
	Label string
	Class string
}

type page struct {
	Root      string
	Generated string
	Depth     int
	Months    []string
	Years     []string // distinct YYYY across Months, ascending (for the year navigator)
	Rows      []row
	Gaps      []gap
	Summary   []pill
}

// yearOf returns the YYYY part of a YYYYMM month string ("" if too short).
func yearOf(month string) string {
	if len(month) >= 4 {
		return month[:4]
	}
	return month
}

// Render writes an HTML report to w.
func Render(root string, opts inventory.Options, w io.Writer) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if opts.Depth == 0 {
		if cfg.Inventory.Depth > 0 {
			opts.Depth = cfg.Inventory.Depth
		} else {
			opts.Depth = 1
		}
	}

	m, err := inventory.Collect(root, opts.Depth, opts.RecentMonths)
	if err != nil {
		return err
	}

	hasConfig := len(cfg.Inventory.Required) > 0 || len(cfg.Inventory.Optional) > 0

	p := page{
		Root:      root,
		Generated: time.Now().Format("2006-01-02 15:04"),
		Depth:     opts.Depth,
		Months:    m.Months,
	}

	seenYear := map[string]bool{}
	for _, mo := range m.Months {
		if y := yearOf(mo); !seenYear[y] {
			seenYear[y] = true
			p.Years = append(p.Years, y)
		}
	}

	requiredCount := 0
	missingTotal := 0
	completeCount := 0

	for _, c := range m.Categories {
		class := cfg.Inventory.Classify(c)
		isReq := class == "required"
		isOpt := class == "optional"
		portalCfg := cfg.PortalFor(c)
		portal := portalCfg.URL
		marker := " "
		if isReq {
			marker = "★"
		} else if isOpt {
			marker = "·"
		}

		cells := make([]cell, 0, len(m.Months))
		for _, mo := range m.Months {
			n := m.Counts[c][mo]
			skipped := cfg.Inventory.IsSkipped(c, mo)
			cl := "empty"
			disp := "·"
			cellPortal := ""
			switch {
			case skipped && n == 0:
				cl = "cell-skip"
				disp = "—"
			case n == 0 && isReq:
				cl = "cell-miss"
				cellPortal = portal
			case n == 1:
				cl = "cell-good"
				disp = "1"
			case n > 1:
				cl = "cell-extra"
				disp = fmt.Sprintf("%d", n)
			}
			cells = append(cells, cell{Display: disp, Class: cl, Portal: cellPortal, Year: yearOf(mo)})
		}

		p.Rows = append(p.Rows, row{
			Category: c,
			Required: isReq,
			Optional: isOpt,
			Marker:   marker,
			Portal:   portal,
			Cells:    cells,
			Total:    m.Totals[c],
		})

		if isReq || (!hasConfig && !isOpt) {
			gaps, recurring := m.FindGaps(c, opts.GapThreshold)
			if isReq && !recurring {
				gaps = m.AllMissing(c)
				recurring = true
			}
			if recurring {
				if isReq {
					requiredCount++
				}
				// Drop months explicitly skipped via .keiri.yaml.
				filtered := gaps[:0:0]
				for _, g := range gaps {
					if cfg.Inventory.IsSkipped(c, g) {
						continue
					}
					filtered = append(filtered, g)
				}
				if len(filtered) == 0 {
					p.Gaps = append(p.Gaps, gap{Category: c, Status: "complete", Portal: portal})
					if isReq {
						completeCount++
					}
				} else {
					p.Gaps = append(p.Gaps, gap{Category: c, Status: "missing", Missing: filtered, Portal: portal})
					if isReq {
						missingTotal += len(filtered)
					}
				}
			}
		}
	}

	if hasConfig {
		p.Summary = []pill{
			{Label: fmt.Sprintf("%d required", requiredCount), Class: "ok"},
			{Label: fmt.Sprintf("%d complete", completeCount), Class: "ok"},
			{Label: fmt.Sprintf("%d months missing", missingTotal), Class: "warn"},
		}
	}

	return tpl.Execute(w, p)
}
