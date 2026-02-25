package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var authKeysCmd = &cobra.Command{
	Use:   "auth-keys <username> [key-type] [key-data]",
	Short: "AuthorizedKeysCommand handler for sshd",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("auth-keys not wired up yet")
	},
}

func init() {
	rootCmd.AddCommand(authKeysCmd)
}
