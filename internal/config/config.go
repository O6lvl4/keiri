// Package config loads .keiri.yaml from a bookkeeping root.
//
// The file is optional. When absent the Config zero-value is returned,
// which means "everything is required, depth=1".
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

const FileName = ".keiri.yaml"

// nfc normalizes a string to Unicode NFC. macOS / Google Drive store
// filenames decomposed (NFD), while .keiri.yaml keys are typically NFC,
// so category↔config comparisons must normalize both sides first — else
// names with dakuten (e.g. "クレジットカード") silently fail to match.
func nfc(s string) string { return norm.NFC.String(s) }

type Inventory struct {
	Depth    int                 `yaml:"depth"`
	Required []string            `yaml:"required"`
	Optional []string            `yaml:"optional"`
	Skip     map[string][]string `yaml:"skip,omitempty"` // category -> [YYYYMM] months that intentionally have no document
}

type Config struct {
	Inventory Inventory         `yaml:"inventory"`
	Portals   map[string]Portal `yaml:"portals,omitempty"`
	Ingest    Ingest            `yaml:"ingest,omitempty"`
}

// Portal targets a billing portal. YAML accepts either a bare URL
// string or an object with `url:` and optional `chrome-profile:`.
type Portal struct {
	URL           string `yaml:"url"`
	ChromeProfile string `yaml:"chrome-profile,omitempty"`
}

// UnmarshalYAML lets us accept "url-as-string" alongside the struct form.
func (p *Portal) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		p.URL = node.Value
		return nil
	}
	type raw Portal
	return node.Decode((*raw)(p))
}

type Ingest struct {
	Rules []IngestRule `yaml:"rules"`
}

type IngestRule struct {
	Match    string   `yaml:"match,omitempty"`     // single substring (legacy)
	MatchAll []string `yaml:"match-all,omitempty"` // every entry must be present
	Dest     string   `yaml:"dest"`                // destination directory, relative to root
	Name     string   `yaml:"name"`                // filename template
}

// Load reads root/.keiri.yaml. Missing file → zero-value Config, no error.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, FileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// IsOptional returns true if the path matches any optional pattern.
// Patterns are matched as path prefixes (e.g. "契約サービス関連書類 - 月額/Apple"
// matches that exact subdirectory).
func (i Inventory) IsOptional(path string) bool {
	return matches(i.Optional, path)
}

// IsRequired returns true if the path matches any required pattern.
func (i Inventory) IsRequired(path string) bool {
	return matches(i.Required, path)
}

// Classify returns "required", "optional", or "" (unspecified).
// Required wins over optional when both match.
func (i Inventory) Classify(path string) string {
	if i.IsRequired(path) {
		return "required"
	}
	if i.IsOptional(path) {
		return "optional"
	}
	return ""
}

func matches(patterns []string, path string) bool {
	path = nfc(path)
	for _, p := range patterns {
		p = nfc(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// IsSkipped reports whether month was explicitly marked "no document
// expected" for category in the config.
func (i Inventory) IsSkipped(category, month string) bool {
	if len(i.Skip) == 0 {
		return false
	}
	category = nfc(category)
	for k, months := range i.Skip {
		nk := nfc(k)
		if nk == "" {
			continue
		}
		if category == nk || strings.HasPrefix(category, nk+"/") {
			for _, m := range months {
				if m == month {
					return true
				}
			}
		}
	}
	return false
}

// PortalFor returns the Portal declared in `portals:` for category,
// falling back to a path-prefix match. Zero-value when nothing matches.
func (c *Config) PortalFor(category string) Portal {
	if c == nil || c.Portals == nil {
		return Portal{}
	}
	category = nfc(category)
	for k, v := range c.Portals {
		nk := nfc(k)
		if nk != "" && (category == nk || strings.HasPrefix(category, nk+"/")) {
			return v
		}
	}
	return Portal{}
}
