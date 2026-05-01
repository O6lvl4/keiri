package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/O6lvl4/keiri/internal/inventory"
	"github.com/O6lvl4/keiri/internal/view"
	"github.com/spf13/cobra"
)

var (
	flagViewRoot         string
	flagViewMonths       int
	flagViewDepth        int
	flagViewGapThreshold float64
	flagViewOut          string
	flagViewNoOpen       bool
)

var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "Render an HTML inventory report and open it",
	Long: `Build a single-page HTML report of the bookkeeping root and
open it in your default browser. Same data as "keiri inventory" but
visual: required/optional rows, color-coded cells, and a gap summary
at the bottom.

By default the report is written to a temp file and opened. Pass
--out to keep it, or --no-open to skip launching a browser.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := flagViewRoot
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

		out := flagViewOut
		toStdout := out == "-"
		if out == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			cacheDir := filepath.Join(home, ".cache", "keiri")
			if err := os.MkdirAll(cacheDir, 0o755); err != nil {
				return err
			}
			out = filepath.Join(cacheDir, "view.html")
		}

		opts := inventory.DefaultOptions()
		opts.RecentMonths = flagViewMonths
		opts.Depth = flagViewDepth
		opts.GapThreshold = flagViewGapThreshold

		var w *os.File
		if toStdout {
			w = os.Stdout
		} else {
			f, err := os.Create(out)
			if err != nil {
				return err
			}
			defer f.Close()
			w = f
		}

		if err := view.Render(root, opts, w); err != nil {
			return err
		}
		if toStdout {
			return nil
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", out)
		if !flagViewNoOpen {
			if err := openInBrowser(out); err != nil {
				fmt.Fprintf(os.Stderr, "open failed: %v\n", err)
			}
		}
		return nil
	},
}

func openInBrowser(path string) error {
	return exec.Command("open", path).Run()
}

func init() {
	viewCmd.Flags().StringVar(&flagViewRoot, "dir", "", "bookkeeping root directory (default: ~/Downloads/経理-gdrive or $KEIRI_ROOT)")
	viewCmd.Flags().IntVarP(&flagViewMonths, "months", "m", 24, "number of trailing months to display (0 = all)")
	viewCmd.Flags().IntVar(&flagViewDepth, "depth", 0, "category depth from root (0 = use .keiri.yaml or 1)")
	viewCmd.Flags().Float64Var(&flagViewGapThreshold, "gap-threshold", 0.8, "coverage ratio above which a category is treated as recurring (0..1)")
	viewCmd.Flags().StringVarP(&flagViewOut, "out", "o", "", "output HTML path (default: temp file; \"-\" for stdout)")
	viewCmd.Flags().BoolVar(&flagViewNoOpen, "no-open", false, "do not auto-open the rendered file")
	rootCmd.AddCommand(viewCmd)
}
