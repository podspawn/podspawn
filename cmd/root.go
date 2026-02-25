package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "podspawn",
	Short: "Ephemeral SSH dev containers via native sshd",
	Long:  "Podspawn spawns ephemeral Docker containers on SSH connection, using the host's native sshd. No custom SSH server, no daemon, just containers.",
}

func Execute() error {
	return rootCmd.Execute()
}
