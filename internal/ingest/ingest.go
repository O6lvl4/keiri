// Package ingest classifies a single bookkeeping document (PDF) by
// extracting its text, matching against rules from .keiri.yaml, and
// moving the file to its canonical destination with a templated name.
package ingest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/O6lvl4/keiri/internal/config"
)

// Filename template placeholders.
//
//   {yyyy-mm-dd}  → 2026-04-19
//   {yyyymmdd}    → 20260419
//   {yyyy-mm}     → 2026-04
//   {yyyymm}      → 202604
//   {yyyy}        → 2026
//   {original}    → input filename without extension
//   {ext}         → input filename extension without leading dot

var (
	rDateHyphen  = regexp.MustCompile(`(20[0-9]{2})-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])`)
	rDateCompact = regexp.MustCompile(`(20[0-9]{2})(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])`)
	rDateJP      = regexp.MustCompile(`(20[0-9]{2})年(0?[1-9]|1[0-2])月(0?[1-9]|[12][0-9]|3[01])日`)
	rDateEnglish = regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December) (\d{1,2}), (20\d{2})`)
)

var monthByName = map[string]string{
	"January": "01", "February": "02", "March": "03", "April": "04",
	"May": "05", "June": "06", "July": "07", "August": "08",
	"September": "09", "October": "10", "November": "11", "December": "12",
}

// Result describes one ingested file.
type Result struct {
	Source     string
	Dest       string // full destination path (or proposed if dry-run)
	Vendor     string // matched rule's keyword
	Date       time.Time
	DryRun     bool
	Skipped    bool
	SkipReason string
}

// extractText shells out to pdftotext.
func extractText(path string) (string, error) {
	cmd := exec.Command("pdftotext", path, "-")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("pdftotext: %w (is poppler installed?)", err)
	}
	return string(out), nil
}

func findRule(rules []config.IngestRule, text string) *config.IngestRule {
	for i, r := range rules {
		if !ruleMatches(r, text) {
			continue
		}
		return &rules[i]
	}
	return nil
}

func ruleMatches(r config.IngestRule, text string) bool {
	if r.Match == "" && len(r.MatchAll) == 0 {
		return false
	}
	if r.Match != "" && !strings.Contains(text, r.Match) {
		return false
	}
	for _, m := range r.MatchAll {
		if !strings.Contains(text, m) {
			return false
		}
	}
	return true
}

func tryDate(s string) (time.Time, bool) {
	if m := rDateHyphen.FindString(s); m != "" {
		if t, err := time.Parse("2006-01-02", m); err == nil {
			return t, true
		}
	}
	if m := rDateJP.FindStringSubmatch(s); m != nil {
		joined := fmt.Sprintf("%s-%s-%s", m[1], pad2(m[2]), pad2(m[3]))
		if t, err := time.Parse("2006-01-02", joined); err == nil {
			return t, true
		}
	}
	if m := rDateEnglish.FindStringSubmatch(s); m != nil {
		mo, ok := monthByName[m[1]]
		if ok {
			joined := fmt.Sprintf("%s-%s-%s", m[3], mo, pad2(m[2]))
			if t, err := time.Parse("2006-01-02", joined); err == nil {
				return t, true
			}
		}
	}
	if m := rDateCompact.FindString(s); m != "" {
		if t, err := time.Parse("20060102", m); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func extractDate(filename, text string, fallback time.Time) time.Time {
	if t, ok := tryDate(filename); ok {
		return t
	}
	if t, ok := tryDate(text); ok {
		return t
	}
	return fallback
}

func renderName(tmpl string, date time.Time, original, ext string) string {
	prev := date.AddDate(0, -1, 0)
	r := strings.NewReplacer(
		"{yyyy-mm-dd}", date.Format("2006-01-02"),
		"{yyyymmdd}", date.Format("20060102"),
		"{yyyy-mm}", date.Format("2006-01"),
		"{yyyymm}", date.Format("200601"),
		"{yyyy}", date.Format("2006"),
		"{mm}", date.Format("01"),
		"{prev-yyyy-mm}", prev.Format("2006-01"),
		"{prev-yyyymm}", prev.Format("200601"),
		"{prev-yyyy}", prev.Format("2006"),
		"{prev-mm}", prev.Format("01"),
		"{original}", original,
		"{ext}", ext,
	)
	return r.Replace(tmpl)
}

// File classifies and moves a single file.
func File(root, srcPath string, rules []config.IngestRule, dry bool) (*Result, error) {
	text, err := extractText(srcPath)
	if err != nil {
		return nil, err
	}
	rule := findRule(rules, text)
	if rule == nil {
		return &Result{Source: srcPath, Skipped: true, SkipReason: "no matching rule", DryRun: dry}, nil
	}

	base := filepath.Base(srcPath)
	original := strings.TrimSuffix(base, filepath.Ext(base))
	ext := strings.TrimPrefix(filepath.Ext(srcPath), ".")

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	date := extractDate(base, text, info.ModTime())

	destName := renderName(rule.Name, date, original, ext)
	destDir := filepath.Join(root, rule.Dest)
	destPath := filepath.Join(destDir, destName)

	vendor := rule.Match
	if vendor == "" && len(rule.MatchAll) > 0 {
		vendor = strings.Join(rule.MatchAll, " + ")
	}
	res := &Result{
		Source: srcPath,
		Dest:   destPath,
		Vendor: vendor,
		Date:   date,
		DryRun: dry,
	}

	if dry {
		return res, nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(destPath); err == nil {
		res.Skipped = true
		res.SkipReason = "destination exists"
		return res, nil
	}
	if err := os.Rename(srcPath, destPath); err != nil {
		// Different filesystems → fall back to copy+remove.
		if !errors.Is(err, os.ErrNotExist) {
			if cerr := copyAndRemove(srcPath, destPath); cerr != nil {
				return nil, fmt.Errorf("rename: %w; fallback copy: %v", err, cerr)
			}
			return res, nil
		}
		return nil, err
	}
	return res, nil
}

func copyAndRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
