package cli

import (
	"os"
	"path/filepath"

	"github.com/O6lvl4/keiri/internal/inventory"
	"github.com/spf13/cobra"
)

const envInventoryRoot = "KEIRI_ROOT"

var (
	flagInventoryRoot         string
	flagInventoryMonths       int
	flagInventoryGapThreshold float64
	flagInventoryDepth        int
	flagInventoryNoMatrix     bool
	flagInventoryNoGaps       bool
)

var inventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Show monthly coverage matrix and detect gaps in recurring categories",
	Long: `Walk the top-level subdirectories of a bookkeeping root and
print a category × month matrix of file counts.

A "·" in a cell means "no document in that month/category". Below the
matrix, categories that look recurring (≥ --gap-threshold coverage
since their first month) get a warning listing the months that are
missing — so e.g. "this is a monthly subscription, May is missing"
becomes obvious.

Default root is ~/Downloads/経理-gdrive — override with --dir or
$KEIRI_ROOT.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := flagInventoryRoot
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
		opts := inventory.DefaultOptions()
		opts.RecentMonths = flagInventoryMonths
		opts.GapThreshold = flagInventoryGapThreshold
		opts.Depth = flagInventoryDepth
		opts.ShowMatrix = !flagInventoryNoMatrix
		opts.ShowGaps = !flagInventoryNoGaps
		return inventory.Run(root, opts, os.Stdout)
	},
}

func init() {
	inventoryCmd.Flags().StringVar(&flagInventoryRoot, "dir", "", "bookkeeping root directory (default: ~/Downloads/経理-gdrive or $KEIRI_ROOT)")
	inventoryCmd.Flags().IntVarP(&flagInventoryMonths, "months", "m", 24, "number of trailing months to display (0 = all)")
	inventoryCmd.Flags().Float64Var(&flagInventoryGapThreshold, "gap-threshold", 0.8, "coverage ratio above which a category is treated as recurring (0..1)")
	inventoryCmd.Flags().IntVar(&flagInventoryDepth, "depth", 0, "category depth from root (0 = use .keiri.yaml or 1)")
	inventoryCmd.Flags().BoolVar(&flagInventoryNoMatrix, "no-matrix", false, "skip the matrix output")
	inventoryCmd.Flags().BoolVar(&flagInventoryNoGaps, "no-gaps", false, "skip the gaps section")
	rootCmd.AddCommand(inventoryCmd)
}
