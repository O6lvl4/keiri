package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/O6lvl4/keiri/internal/receipts"
	"github.com/spf13/cobra"
)

const envReceiptsDir = "KEIRI_RECEIPTS_DIR"

var flagReceiptsDir string

var receiptsCmd = &cobra.Command{
	Use:   "receipts",
	Short: "Lint, plan, and apply naming-rule fixes for receipt files",
	Long: `Lint, plan, and apply naming-rule fixes for receipt files.

Default directory is ~/Downloads/経理/領収書 — override with --dir
or $KEIRI_RECEIPTS_DIR.`,
}

var receiptsLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Detect naming-rule violations (read-only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return receipts.Lint(receiptsDir(), os.Stdout)
	},
}

var receiptsPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show proposed renames (no writes)",
	RunE: func(cmd *cobra.Command, args []string) error {
		plans, err := receipts.GeneratePlan(receiptsDir())
		if err != nil {
			return err
		}
		fmt.Printf("\033[1m▶ plan:\033[0m %d changes\n\n", len(plans))
		for _, p := range plans {
			fmt.Printf("  %s\n    → %s\n", p.Old, p.New)
		}
		return nil
	},
}

var receiptsApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the rename plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		plans, err := receipts.GeneratePlan(receiptsDir())
		if err != nil {
			return err
		}
		if len(plans) == 0 {
			fmt.Println("nothing to do")
			return nil
		}
		applied, skipped, errs := receipts.Apply(receiptsDir(), plans, os.Stdout)
		fmt.Printf("\napplied: %d, skipped: %d, errors: %d\n", applied, skipped, errs)
		if errs > 0 {
			return fmt.Errorf("%d error(s) during apply", errs)
		}
		return nil
	},
}

func receiptsDir() string {
	if flagReceiptsDir != "" {
		return flagReceiptsDir
	}
	if v := os.Getenv(envReceiptsDir); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads", "経理", "領収書")
}

func init() {
	receiptsCmd.PersistentFlags().StringVar(&flagReceiptsDir, "dir", "", "receipts directory (default: ~/Downloads/経理/領収書 or $KEIRI_RECEIPTS_DIR)")
	receiptsCmd.AddCommand(receiptsLintCmd, receiptsPlanCmd, receiptsApplyCmd)
	rootCmd.AddCommand(receiptsCmd)
}
