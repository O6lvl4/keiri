package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/O6lvl4/chropen/profile"
	"github.com/O6lvl4/keiri/internal/config"
	"github.com/O6lvl4/keiri/internal/inventory"
	"github.com/spf13/cobra"
)

var (
	flagCatchupRoot         string
	flagCatchupMonths       int
	flagCatchupDepth        int
	flagCatchupGapThreshold float64
	flagCatchupOpen         bool
)

var catchupCmd = &cobra.Command{
	Use:   "catchup",
	Short: "Show what's missing for required categories and where to fetch it",
	Long: `Print a per-category list of missing months along with each
category's billing portal URL declared in .keiri.yaml. With --open the
unique portal URLs are launched in the default browser so you can
go fetch the missing documents in one motion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := flagCatchupRoot
		if root == "" {
			root = os.Getenv(envInventoryRoot)
		}
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			root = filepath.Join(home, "Downloads", "経理-gdrive")
		}

		cfg, err := config.Load(root)
		if err != nil {
			return err
		}

		opts := inventory.DefaultOptions()
		opts.RecentMonths = flagCatchupMonths
		opts.Depth = flagCatchupDepth
		opts.GapThreshold = flagCatchupGapThreshold
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

		type entry struct {
			Category string
			Missing  []string
			Portal   config.Portal
		}

		type portalKey struct{ URL, Profile string }
		var items []entry
		seen := map[portalKey]config.Portal{}
		for _, c := range m.Categories {
			if cfg.Inventory.Classify(c) != "required" {
				continue
			}
			gaps, recurring := m.FindGaps(c, opts.GapThreshold)
			if !recurring {
				gaps = m.AllMissing(c)
			}
			if len(gaps) == 0 {
				continue
			}
			portal := cfg.PortalFor(c)
			items = append(items, entry{Category: c, Missing: gaps, Portal: portal})
			if portal.URL != "" {
				seen[portalKey{portal.URL, portal.ChromeProfile}] = portal
			}
		}

		bold := func(s string) { fmt.Printf("\033[1m%s\033[0m", s) }
		yellow := func(s string) { fmt.Printf("\033[33m%s\033[0m", s) }
		cyan := func(s string) { fmt.Printf("\033[36m%s\033[0m", s) }
		dim := func(s string) { fmt.Printf("\033[2m%s\033[0m", s) }

		if len(items) == 0 {
			fmt.Println("nothing to catch up on — all required categories complete ✨")
			return nil
		}

		bold(fmt.Sprintf("⚠ Catch-up needed (%d required categories)\n\n", len(items)))
		for _, it := range items {
			fmt.Printf("  %s\n", it.Category)
			fmt.Print("    missing: ")
			yellow(strings.Join(it.Missing, " ") + "\n")
			fmt.Print("    portal:  ")
			if it.Portal.URL != "" {
				cyan(it.Portal.URL)
				if it.Portal.ChromeProfile != "" {
					dim(fmt.Sprintf("  [chrome: %s]", it.Portal.ChromeProfile))
				}
				fmt.Println()
			} else {
				dim("(no portal configured)\n")
			}
		}

		if flagCatchupOpen {
			if len(seen) == 0 {
				fmt.Println("\nno portal URLs to open")
				return nil
			}
			keys := make([]string, 0, len(seen))
			for k := range seen {
				keys = append(keys, k.URL+"\x00"+k.Profile)
			}
			sort.Strings(keys)
			fmt.Printf("\nopening %d portal(s)...\n", len(seen))
			for _, k := range keys {
				url, profile, _ := strings.Cut(k, "\x00")
				if err := openPortal(config.Portal{URL: url, ChromeProfile: profile}); err != nil {
					fmt.Fprintf(os.Stderr, "  open %s: %v\n", url, err)
				}
			}
		}
		return nil
	},
}

func openPortal(p config.Portal) error {
	if p.URL == "" {
		return nil
	}
	if p.ChromeProfile != "" {
		return profile.OpenAs(p.ChromeProfile, p.URL)
	}
	return exec.Command("open", p.URL).Run()
}

func init() {
	catchupCmd.Flags().StringVar(&flagCatchupRoot, "dir", "", "bookkeeping root directory (default: ~/Downloads/経理-gdrive or $KEIRI_ROOT)")
	catchupCmd.Flags().IntVarP(&flagCatchupMonths, "months", "m", 24, "number of trailing months to consider (0 = all)")
	catchupCmd.Flags().IntVar(&flagCatchupDepth, "depth", 0, "category depth from root (0 = use .keiri.yaml or 1)")
	catchupCmd.Flags().Float64Var(&flagCatchupGapThreshold, "gap-threshold", 0.8, "coverage ratio above which a category is treated as recurring")
	catchupCmd.Flags().BoolVarP(&flagCatchupOpen, "open", "o", false, "open each portal URL in the default browser")
	rootCmd.AddCommand(catchupCmd)
}
