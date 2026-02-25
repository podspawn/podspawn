package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var serverSetupCmd = &cobra.Command{
	Use:   "server-setup",
	Short: "Configure sshd for podspawn integration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("server-setup: pending implementation")
	},
}

func init() {
	rootCmd.AddCommand(serverSetupCmd)
}
