package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/podspawn/podspawn/internal/sshconfig"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure client ~/.ssh/config for .pod namespace",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}

		configPath := filepath.Join(home, ".ssh", "config")
		added, err := sshconfig.EnsurePodBlock(configPath)
		if err != nil {
			return err
		}

		if added {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "added *.pod block to ~/.ssh/config")
		} else {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "~/.ssh/config already has Host *.pod block, skipping")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
