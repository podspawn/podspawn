package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open <user@host> [path]",
	Short: "Open a podspawn environment in VS Code or Cursor",
	Long: `Opens VS Code (or Cursor) with Remote SSH connected to your container.

  podspawn open alice@backend             -> opens VS Code at /workspace
  podspawn open alice@backend /app        -> opens VS Code at /app
  podspawn open alice@backend --cursor    -> opens Cursor instead`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remotePath := "/workspace"
		if len(args) > 1 {
			remotePath = args[1]
		}
		useCursor, _ := cmd.Flags().GetBool("cursor")

		user, host, err := parseUserHost(args[0])
		if err != nil {
			return err
		}

		sshTarget := user + "@" + appendPodSuffix(host)

		// Detect IDE
		ideBin := "code"
		if useCursor {
			ideBin = "cursor"
		}

		binPath, err := exec.LookPath(ideBin)
		if err != nil {
			if useCursor {
				return fmt.Errorf("cursor not found in PATH; install it from https://cursor.sh")
			}
			return fmt.Errorf("code not found in PATH; install VS Code and enable the 'code' command")
		}

		// code --remote ssh-remote+user@host.pod /workspace
		remoteArg := fmt.Sprintf("ssh-remote+%s", sshTarget)

		fmt.Printf("opening %s in %s at %s\n", sshTarget, ideBin, remotePath)

		proc := exec.Command(binPath, "--remote", remoteArg, remotePath)
		proc.Stdin = os.Stdin
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr
		return proc.Run()
	},
}

func init() {
	openCmd.Flags().Bool("cursor", false, "Open in Cursor instead of VS Code")
	rootCmd.AddCommand(openCmd)
}
