package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "keiri",
	Short: "Bookkeeping document hygiene CLI",
	Long: `keiri keeps receipts, invoices, and other bookkeeping documents tidy.

Today: receipts.
  keiri receipts lint    – flag naming-rule violations
  keiri receipts plan    – propose canonical filenames
  keiri receipts apply   – execute the plan

Tomorrow: invoices, contracts, payroll slips, ...`,
	SilenceUsage: true,
}

// Execute is the program entry point.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
