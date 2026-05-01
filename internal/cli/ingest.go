package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/O6lvl4/keiri/internal/config"
	"github.com/O6lvl4/keiri/internal/ingest"
	"github.com/spf13/cobra"
)

var (
	flagIngestRoot   string
	flagIngestDryRun bool
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <file>...",
	Short: "Auto-classify PDFs against .keiri.yaml ingest rules",
	Long: `For each file, extract text via pdftotext, find the first matching
rule under .keiri.yaml's "ingest.rules", and rename + move it under
the bookkeeping root.

Filename templates support {yyyy-mm-dd}, {yyyymmdd}, {yyyy-mm},
{yyyymm}, {yyyy}, {original}, {ext}.

Example .keiri.yaml entry:

  ingest:
    rules:
      - match: "American Express"
        dest: "クレジットカード/done"
        name: "amex-{yyyy-mm-dd}.pdf"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := flagIngestRoot
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
		if len(cfg.Ingest.Rules) == 0 {
			return fmt.Errorf("no ingest rules in %s/.keiri.yaml", root)
		}

		bold := func(s string) { fmt.Printf("\033[1m%s\033[0m", s) }
		green := func(s string) { fmt.Printf("\033[32m%s\033[0m", s) }
		dim := func(s string) { fmt.Printf("\033[2m%s\033[0m", s) }
		yellow := func(s string) { fmt.Printf("\033[33m%s\033[0m", s) }
		red := func(s string) { fmt.Printf("\033[31m%s\033[0m", s) }

		applied, skipped, errs := 0, 0, 0
		for _, src := range args {
			abs, _ := filepath.Abs(src)
			res, err := ingest.File(root, abs, cfg.Ingest.Rules, flagIngestDryRun)
			fmt.Printf("→ %s\n", src)
			if err != nil {
				red(fmt.Sprintf("    error: %v\n", err))
				errs++
				continue
			}
			if res.Skipped {
				yellow(fmt.Sprintf("    skip: %s\n", res.SkipReason))
				skipped++
				continue
			}
			rel, err := filepath.Rel(root, res.Dest)
			if err != nil {
				rel = res.Dest
			}
			bold(fmt.Sprintf("    %s\n", res.Vendor))
			if flagIngestDryRun {
				dim(fmt.Sprintf("    would write: %s\n", rel))
			} else {
				green(fmt.Sprintf("    → %s\n", rel))
				applied++
			}
		}
		if !flagIngestDryRun {
			fmt.Printf("\napplied: %d, skipped: %d, errors: %d\n", applied, skipped, errs)
			if errs > 0 {
				return fmt.Errorf("%d error(s)", errs)
			}
		}
		return nil
	},
}

func init() {
	ingestCmd.Flags().StringVar(&flagIngestRoot, "dir", "", "bookkeeping root directory (default: ~/Downloads/経理-gdrive or $KEIRI_ROOT)")
	ingestCmd.Flags().BoolVarP(&flagIngestDryRun, "dry-run", "n", false, "preview without writing")
	rootCmd.AddCommand(ingestCmd)
}
