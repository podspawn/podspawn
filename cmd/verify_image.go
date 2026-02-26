package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var verifyImageCmd = &cobra.Command{
	Use:   "verify-image <image>",
	Short: "Check if a container image is compatible with podspawn",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "verify-image: skipping %s (no checks registered)\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyImageCmd)
}
