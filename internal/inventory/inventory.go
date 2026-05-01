// Package inventory builds a category × month coverage matrix from a
// bookkeeping document tree (e.g. ~/Downloads/経理-gdrive).
//
// It walks the top-level subdirectories of root (or deeper, controlled
// by Options.Depth) and counts files per YYYY-MM extracted from
// filenames. A category is the relative path from root, e.g.
// "契約サービス関連書類 - 月額/GoogleWorkspace" at depth=2.
package inventory

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/O6lvl4/keiri/internal/config"
)

// now is overridable in tests.
var now = time.Now

// expectedLatestMonth returns the most recent calendar month that
// "should" be complete by today — i.e. last month. If a required
// monthly category is missing this YYYYMM, it's overdue.
func expectedLatestMonth() string {
	return now().AddDate(0, -1, 0).Format("200601")
}

// Year-month extraction supports several common conventions:
//   - YYYYMMDD or YYYYMM (e.g. "20240919_...", "20240919-001")
//   - YYYY-MM[-DD] or YYYY_MM[_DD] (e.g. "amex-2024-09-19.pdf")
//   - YYYY年MM月 / YYYY年M月 (e.g. "2024年07月Aid-On-Inc請求書.pdf")
// Year is constrained to 20XX so 4-digit identifiers like
// "Receipt-2400-2187" don't get misread as years.
var rYMPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[_\-./])(20[0-9]{2})(0[1-9]|1[0-2])(?:[0-9]{2})?`),
	regexp.MustCompile(`(?:^|[_\-./])(20[0-9]{2})[\-_](0[1-9]|1[0-2])(?:[\-_][0-9]{2})?`),
	regexp.MustCompile(`(20[0-9]{2})年(0?[1-9]|1[0-2])月`),
}

func extractYM(name string) string {
	for _, r := range rYMPatterns {
		m := r.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		y, mo := m[1], m[2]
		if len(mo) == 1 {
			mo = "0" + mo
		}
		return y + mo
	}
	return ""
}

// Matrix is the result of scanning a bookkeeping root.
type Matrix struct {
	Categories []string                  // relative paths from root
	Months     []string                  // YYYYMM, ascending
	Counts     map[string]map[string]int // [category][month] -> file count
	Totals     map[string]int
}

// Collect scans root and returns a populated Matrix. Exported for use
// by alternative front-ends (HTML viewer, JSON dump, ...).
func Collect(root string, depth, recentMonths int) (*Matrix, error) {
	r, err := collect(root, depth, recentMonths)
	if err != nil {
		return nil, err
	}
	return &Matrix{
		Categories: r.categories,
		Months:     r.months,
		Counts:     r.matrix,
		Totals:     r.totals,
	}, nil
}

// FindGaps reports months missing in cat assuming the category is
// recurring above threshold from its first non-empty month onward.
// The second return is true when the category is recurring enough.
func (m *Matrix) FindGaps(cat string, threshold float64) ([]string, bool) {
	return findGaps(m.Counts[cat], m.Months, threshold)
}

// AllMissing returns every month with zero files in cat from the
// first non-empty month onward (used for required-but-rare cats).
func (m *Matrix) AllMissing(cat string) []string {
	return collectAllMissing(m.Counts[cat], m.Months)
}

type result struct {
	categories []string // relative paths from root
	months     []string
	matrix     map[string]map[string]int
	totals     map[string]int
}

// listCategories enumerates categories at the requested depth below
// root. A directory whose only meaningful child is named "done" is
// flattened — its parent stays as the category and the file walk dives
// into "done" anyway. Names starting with "." or "_" are skipped.
func listCategories(root string, depth int) ([]string, error) {
	if depth < 1 {
		depth = 1
	}
	var cats []string
	var walk func(rel string, level int) error
	walk = func(rel string, level int) error {
		full := filepath.Join(root, rel)
		entries, err := os.ReadDir(full)
		if err != nil {
			return err
		}
		var children []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				continue
			}
			children = append(children, e)
		}
		// Flatten "done" wrapper directories.
		if len(children) == 1 && children[0].Name() == "done" && rel != "" {
			cats = append(cats, rel)
			return nil
		}
		if level == depth {
			for _, e := range children {
				child := e.Name()
				if rel != "" {
					child = rel + "/" + child
				}
				cats = append(cats, child)
			}
			return nil
		}
		for _, e := range children {
			child := e.Name()
			if rel != "" {
				child = rel + "/" + child
			}
			if err := walk(child, level+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("", 1); err != nil {
		return nil, err
	}
	sort.Strings(cats)
	return cats, nil
}

func collect(root string, depth, recentMonths int) (*result, error) {
	cats, err := listCategories(root, depth)
	if err != nil {
		return nil, err
	}
	matrix := map[string]map[string]int{}
	totals := map[string]int{}
	allMonths := map[string]struct{}{}

	for _, cat := range cats {
		matrix[cat] = map[string]int{}
		full := filepath.Join(root, cat)
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return nil
			}
			ym := extractYM(name)
			if ym == "" {
				return nil
			}
			matrix[cat][ym]++
			totals[cat]++
			allMonths[ym] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Make sure the most recent expected month is part of the report
	// even when no category has a file for it yet — that's exactly the
	// case we want to surface as "this month's docs haven't arrived".
	allMonths[expectedLatestMonth()] = struct{}{}

	months := make([]string, 0, len(allMonths))
	for m := range allMonths {
		months = append(months, m)
	}
	sort.Strings(months)
	if recentMonths > 0 && len(months) > recentMonths {
		months = months[len(months)-recentMonths:]
	}

	return &result{categories: cats, months: months, matrix: matrix, totals: totals}, nil
}

// Options controls Run output.
type Options struct {
	RecentMonths int
	GapThreshold float64
	Depth        int
	ShowMatrix   bool
	ShowGaps     bool
}

// DefaultOptions for `keiri inventory` invocations.
func DefaultOptions() Options {
	return Options{RecentMonths: 24, GapThreshold: 0.8, Depth: 1, ShowMatrix: true, ShowGaps: true}
}

// Run prints a coverage matrix and/or gap report to w.
func Run(root string, opts Options, w io.Writer) error {
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

	r, err := collect(root, opts.Depth, opts.RecentMonths)
	if err != nil {
		return err
	}
	if len(r.categories) == 0 {
		fmt.Fprintf(w, "no category subdirectories under %s\n", root)
		return nil
	}

	fmt.Fprintf(w, "▶ %s (depth=%d)\n\n", root, opts.Depth)

	if opts.ShowMatrix {
		printMatrix(r, cfg, w)
	}
	if opts.ShowGaps {
		if opts.ShowMatrix {
			fmt.Fprintln(w)
		}
		printGaps(r, cfg, opts.GapThreshold, w)
	}
	return nil
}

func printMatrix(r *result, cfg *config.Config, w io.Writer) {
	catWidth := 0
	for _, c := range r.categories {
		if width := utf8.RuneCountInString(c); width > catWidth {
			catWidth = width
		}
	}
	if catWidth > 40 {
		catWidth = 40
	}

	fmt.Fprintf(w, "%s   ", padRunes("", catWidth))
	for _, m := range r.months {
		fmt.Fprintf(w, " %s", m)
	}
	fmt.Fprintf(w, " %6s\n", "total")

	for _, c := range r.categories {
		label := c
		if utf8.RuneCountInString(label) > catWidth {
			label = trimRunes(label, catWidth-1) + "…"
		}
		marker := "  "
		switch cfg.Inventory.Classify(c) {
		case "required":
			marker = "★ "
		case "optional":
			marker = "· "
		}
		fmt.Fprintf(w, "%s%s ", marker, padRunes(label, catWidth))
		for _, m := range r.months {
			n := r.matrix[c][m]
			if n == 0 {
				fmt.Fprintf(w, " %6s", "·")
			} else {
				fmt.Fprintf(w, " %6d", n)
			}
		}
		fmt.Fprintf(w, " %6d\n", r.totals[c])
	}
}

// findGaps returns months that are *missing* in a category whose
// coverage from the first non-empty month onward meets the threshold.
// Returns (nil, false) if the category isn't recurring enough.
func findGaps(matrix map[string]int, months []string, threshold float64) ([]string, bool) {
	firstIdx := -1
	for i, m := range months {
		if matrix[m] > 0 {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 {
		return nil, false
	}
	rangeMonths := months[firstIdx:]
	nonZero := 0
	var gaps []string
	for _, m := range rangeMonths {
		if matrix[m] > 0 {
			nonZero++
		} else {
			gaps = append(gaps, m)
		}
	}
	coverage := float64(nonZero) / float64(len(rangeMonths))
	if coverage < threshold {
		return nil, false
	}
	return gaps, true
}

func printGaps(r *result, cfg *config.Config, threshold float64, w io.Writer) {
	bold := func(s string) { fmt.Fprintf(w, "\033[1m%s\033[0m", s) }
	dim := func(s string) { fmt.Fprintf(w, "\033[2m%s\033[0m", s) }
	yellow := func(s string) { fmt.Fprintf(w, "\033[33m%s\033[0m", s) }

	hasConfig := len(cfg.Inventory.Required) > 0 || len(cfg.Inventory.Optional) > 0
	if hasConfig {
		bold("⚠ Gaps (required categories only)\n")
	} else {
		bold(fmt.Sprintf("⚠ Gaps (categories with ≥%.0f%% monthly coverage)\n", threshold*100))
	}

	any := false
	for _, c := range r.categories {
		class := cfg.Inventory.Classify(c)
		if hasConfig {
			if class != "required" {
				continue
			}
		} else {
			if class == "optional" {
				continue
			}
		}

		gaps, recurring := findGaps(r.matrix[c], r.months, threshold)
		// For required categories we always report missing months even
		// if the coverage threshold isn't met (config opts in).
		if hasConfig && class == "required" && !recurring {
			gaps = collectAllMissing(r.matrix[c], r.months)
			recurring = true
		}
		if !recurring {
			continue
		}
		gaps = filterSkipped(gaps, c, cfg)
		fmt.Fprintf(w, "  %s ", c)
		if len(gaps) == 0 {
			dim("(complete)\n")
			continue
		}
		any = true
		yellow(fmt.Sprintf("missing %d month(s):", len(gaps)))
		for _, g := range gaps {
			fmt.Fprintf(w, " %s", g)
		}
		fmt.Fprintln(w)
	}
	if !any && !hasConfig {
		dim("  (no recurring category has missing months)\n")
	}
}

// filterSkipped removes months listed under inventory.skip[category] from gaps.
func filterSkipped(gaps []string, category string, cfg *config.Config) []string {
	if cfg == nil || len(cfg.Inventory.Skip) == 0 {
		return gaps
	}
	out := gaps[:0:0]
	for _, g := range gaps {
		if cfg.Inventory.IsSkipped(category, g) {
			continue
		}
		out = append(out, g)
	}
	return out
}

func collectAllMissing(matrix map[string]int, months []string) []string {
	firstIdx := -1
	for i, m := range months {
		if matrix[m] > 0 {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 {
		// If the category is required but has zero files in range,
		// every month counts as missing.
		out := make([]string, len(months))
		copy(out, months)
		return out
	}
	var gaps []string
	for _, m := range months[firstIdx:] {
		if matrix[m] == 0 {
			gaps = append(gaps, m)
		}
	}
	return gaps
}

func padRunes(s string, n int) string {
	pad := n - utf8.RuneCountInString(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func trimRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	out := make([]rune, 0, n)
	for _, r := range s {
		if len(out) >= n {
			break
		}
		out = append(out, r)
	}
	return string(out)
}
