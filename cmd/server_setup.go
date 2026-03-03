package cmd

import (
	"fmt"
	"os"

	"github.com/podspawn/podspawn/internal/serversetup"
	"github.com/spf13/cobra"
)

var serverSetupCmd = &cobra.Command{
	Use:   "server-setup",
	Short: "Configure sshd for podspawn integration",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getuid() != 0 {
			return fmt.Errorf("server-setup requires root privileges")
		}

		binaryPath, err := serversetup.ResolveBinaryPath()
		if err != nil {
			return err
		}

		sshdConfig, _ := cmd.Flags().GetString("sshd-config")
		serviceName, _ := cmd.Flags().GetString("service-name")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		paths := serversetup.DefaultPaths(binaryPath)
		if sshdConfig != "" {
			paths.SSHDConfig = sshdConfig
		}

		return serversetup.Run(paths, serversetup.ExecCommander{}, serversetup.Options{
			DryRun:      dryRun,
			ServiceName: serviceName,
		}, cmd.ErrOrStderr())
	},
}

func init() {
	serverSetupCmd.Flags().String("sshd-config", "", "path to sshd_config (default: /etc/ssh/sshd_config)")
	serverSetupCmd.Flags().String("service-name", "", "SSH service name override (auto-detected if not set)")
	serverSetupCmd.Flags().Bool("dry-run", false, "show what would be done without making changes")
	rootCmd.AddCommand(serverSetupCmd)
}
