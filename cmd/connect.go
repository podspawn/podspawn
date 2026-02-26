package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect <user> <host> <port>",
	Short: "ProxyCommand handler for .pod namespace routing",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "connect: stub")
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}
