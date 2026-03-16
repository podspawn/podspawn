package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <user@host> [-- command...]",
	Short: "SSH into a podspawn environment",
	Long: `Convenience wrapper around ssh. Appends .pod to the hostname if not already present.

  podspawn ssh alice@backend       -> ssh alice@backend.pod
  podspawn ssh alice@backend.pod   -> ssh alice@backend.pod (no change)
  podspawn ssh alice@myserver.com  -> ssh alice@myserver.com (non-.pod passthrough)
  podspawn ssh alice@backend -- ls -> ssh alice@backend.pod ls`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, host, err := parseUserHost(args[0])
		if err != nil {
			return err
		}

		sshTarget := user + "@" + appendPodSuffix(host)

		sshArgs := []string{sshTarget}

		// Everything after -- is the remote command
		for i, arg := range args[1:] {
			if arg == "--" {
				sshArgs = append(sshArgs, args[i+2:]...)
				break
			}
		}

		sshBin, err := exec.LookPath("ssh")
		if err != nil {
			return fmt.Errorf("ssh not found in PATH")
		}

		proc := exec.Command(sshBin, sshArgs...)
		proc.Stdin = os.Stdin
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr
		return proc.Run()
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
