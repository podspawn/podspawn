package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <session>",
	Short: "Stop a session and destroy its container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("stop %q: not implemented", args[0])
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
