package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Reconcile orphaned containers and enforce TTLs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cleanup: noop")
	},
}

func init() {
	cleanupCmd.Flags().Bool("daemon", false, "Run as background cleanup daemon")
	rootCmd.AddCommand(cleanupCmd)
}
