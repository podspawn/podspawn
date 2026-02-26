package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure client ~/.ssh/config for .pod namespace",
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "setup: nothing here yet\n")
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
