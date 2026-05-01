// Package receipts implements naming-rule lint/plan/apply for receipt files.
package receipts

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed vendors.tsv
var defaultVendorsTSV string

type vendorRule struct {
	pattern string
	canon   string
}

func parseVendors(tsv string) []vendorRule {
	var rules []vendorRule
	for _, line := range strings.Split(tsv, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		rules = append(rules, vendorRule{pattern: parts[0], canon: parts[1]})
	}
	return rules
}

var (
	rDatePrefix    = regexp.MustCompile(`^[0-9]{8}`)
	rDateLeading   = regexp.MustCompile(`^[0-9]{8}_?`)
	rBracketN      = regexp.MustCompile(`\[[0-9]+\]`)
	rParenN        = regexp.MustCompile(` \(([0-9]+)\)`)
	rUnderscoreSeq = regexp.MustCompile(`_+`)
	rUnderscoreDot = regexp.MustCompile(`_\.`)
	rLeadingSep    = regexp.MustCompile(`^[_-]+`)
	rTrailingSep   = regexp.MustCompile(`[_-]+$`)
	rSepDashSep    = regexp.MustCompile(`_-_`)
	rUnderscoreDsh = regexp.MustCompile(`_-`)
	rDshUnderscore = regexp.MustCompile(`-_`)
	rNoiseHard     = regexp.MustCompile(`(^|_)(receipt|Receipt)(_|$)`)
	rNoiseSoft     = regexp.MustCompile(`(^|[_-])(領収書|Your|your|Order|order)([_-]|$)`)
)

func cleanupSeps(s string) string {
	s = rSepDashSep.ReplaceAllString(s, "_")
	s = rUnderscoreDsh.ReplaceAllString(s, "_")
	s = rDshUnderscore.ReplaceAllString(s, "-")
	s = rUnderscoreSeq.ReplaceAllString(s, "_")
	s = rLeadingSep.ReplaceAllString(s, "")
	s = rTrailingSep.ReplaceAllString(s, "")
	s = rUnderscoreDot.ReplaceAllString(s, ".")
	return s
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "のコピー", "")
	s = strings.ReplaceAll(s, " _ ", "_")
	s = strings.ReplaceAll(s, "｜", "_")
	s = strings.ReplaceAll(s, "－", "-")
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "（", "_")
	s = strings.ReplaceAll(s, "）", "_")
	s = rBracketN.ReplaceAllString(s, "")
	s = rParenN.ReplaceAllString(s, "_v$1")
	s = strings.ReplaceAll(s, " ", "_")
	return cleanupSeps(s)
}

func stripNoiseWords(s string) string {
	for i := 0; i < 3; i++ {
		s = rNoiseHard.ReplaceAllString(s, "$1$3")
		s = rNoiseSoft.ReplaceAllString(s, "$1$3")
	}
	s = rUnderscoreSeq.ReplaceAllString(s, "_")
	s = rLeadingSep.ReplaceAllString(s, "")
	s = rTrailingSep.ReplaceAllString(s, "")
	return s
}

// Plan describes a single rename.
type Plan struct {
	Old string
	New string
}

func listFiles(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

// GeneratePlan computes proposed renames for every file in dir.
func GeneratePlan(dir string) ([]Plan, error) {
	entries, err := listFiles(dir)
	if err != nil {
		return nil, err
	}
	rules := parseVendors(defaultVendorsTSV)
	var plans []Plan
	for _, e := range entries {
		f := e.Name()
		ext := ""
		base := f
		if idx := strings.LastIndex(f, "."); idx > 0 {
			ext = f[idx+1:]
			base = f[:idx]
		}
		norm := normalize(base)

		dateStr := rDatePrefix.FindString(norm)
		if dateStr == "" {
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			dateStr = info.ModTime().Format("20060102")
		}

		rest := rDateLeading.ReplaceAllString(norm, "")
		rest = cleanupSeps(rest)

		var matched *vendorRule
		for i, rule := range rules {
			if strings.Contains(rest, rule.pattern) {
				matched = &rules[i]
				break
			}
		}

		var newName string
		if matched != nil {
			rest = strings.ReplaceAll(rest, matched.pattern, "")
			rest = stripNoiseWords(rest)
			rest = cleanupSeps(rest)
			if rest != "" {
				newName = fmt.Sprintf("%s_%s_%s.%s", dateStr, matched.canon, rest, ext)
			} else {
				newName = fmt.Sprintf("%s_%s.%s", dateStr, matched.canon, ext)
			}
		} else {
			rest = stripNoiseWords(rest)
			rest = cleanupSeps(rest)
			if rest != "" {
				newName = fmt.Sprintf("%s_%s.%s", dateStr, rest, ext)
			} else {
				newName = fmt.Sprintf("%s.%s", dateStr, ext)
			}
		}
		newName = cleanupSeps(newName)

		if newName != f {
			plans = append(plans, Plan{Old: f, New: newName})
		}
	}
	return plans, nil
}

// Apply executes the rename plan in dir.
func Apply(dir string, plans []Plan, w io.Writer) (applied, skipped, errors int) {
	for _, p := range plans {
		oldPath := filepath.Join(dir, p.Old)
		newPath := filepath.Join(dir, p.New)
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			fmt.Fprintf(w, "  src missing: %s\n", p.Old)
			skipped++
			continue
		}
		if _, err := os.Stat(newPath); err == nil {
			fmt.Fprintf(w, "  dst exists: %s\n", p.New)
			skipped++
			continue
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			fmt.Fprintf(w, "  rename failed: %s → %s: %v\n", p.Old, p.New, err)
			errors++
			continue
		}
		fmt.Fprintf(w, "  → %s\n", p.New)
		applied++
	}
	return
}

// Lint prints naming-rule violations to w.
func Lint(dir string, w io.Writer) error {
	entries, err := listFiles(dir)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.Name())
	}

	bold := func(s string) { fmt.Fprintf(w, "\033[1m%s\033[0m\n", s) }

	bold(fmt.Sprintf("▶ %s", dir))
	fmt.Fprintf(w, "  total: %d\n\n", len(files))

	rDate := regexp.MustCompile(`^[0-9]{8}_`)
	bold("[1] 日付プレフィックス無し")
	hit := 0
	for _, f := range files {
		if !rDate.MatchString(f) {
			fmt.Fprintf(w, "  %s\n", f)
			hit++
		}
	}
	if hit == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w)

	bold("[2] 冗長な二重日付（期間/IDを除く）")
	rPeriodHy := regexp.MustCompile(`[0-9]{8}-[0-9]{8}`)
	rPeriodTo := regexp.MustCompile(`[0-9]{8}_to_[0-9]{8}`)
	rDateDash := regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)
	rOrderID := regexp.MustCompile(`#[0-9]+-[0-9]+`)
	r8 := regexp.MustCompile(`[0-9]{8}`)
	hit = 0
	for _, f := range files {
		s := f
		s = rPeriodHy.ReplaceAllString(s, "")
		s = rPeriodTo.ReplaceAllString(s, "")
		s = rDateDash.ReplaceAllString(s, "")
		s = rOrderID.ReplaceAllString(s, "")
		if len(r8.FindAllString(s, -1)) >= 2 {
			fmt.Fprintf(w, "  %s\n", f)
			hit++
		}
	}
	if hit == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w)

	bold("[3] コピー痕跡（のコピー / [N] / (N)）")
	rCopy := regexp.MustCompile(`のコピー|\[[0-9]+\]| \([0-9]+\)`)
	hit = 0
	for _, f := range files {
		if rCopy.MatchString(f) {
			fmt.Fprintf(w, "  %s\n", f)
			hit++
		}
	}
	if hit == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w)

	bold("[4] 装飾・揺れ文字（｜ － – /  半角空白囲み）")
	rDecor := regexp.MustCompile(`｜|－|–| _ `)
	hit = 0
	for _, f := range files {
		if rDecor.MatchString(f) {
			fmt.Fprintf(w, "  %s\n", f)
			hit++
		}
	}
	if hit == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w)

	bold("[5] 拡張子分布")
	exts := map[string]int{}
	for _, f := range files {
		if idx := strings.LastIndex(f, "."); idx > 0 {
			exts[f[idx+1:]]++
		}
	}
	keys := make([]string, 0, len(exts))
	for k := range exts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %5d %s\n", exts[k], k)
	}
	fmt.Fprintln(w)

	bold("[6] 月別件数（YYYYMM）")
	rMonth := regexp.MustCompile(`^[0-9]{6}`)
	months := map[string]int{}
	for _, f := range files {
		if m := rMonth.FindString(f); m != "" {
			months[m]++
		}
	}
	keys = keys[:0]
	for k := range months {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %5d %s\n", months[k], k)
	}

	return nil
}
