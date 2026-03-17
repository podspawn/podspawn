package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell [user@]<name>",
	Short: "Attach to a container directly (no SSH)",
	Long: `Open a shell in a podspawn container using Docker exec.

  podspawn shell mydev             -> attach to machine "mydev"
  podspawn shell alice@backend     -> attach as alice to "backend"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, name := parseShellTarget(args[0])

		ls, err := buildLocalSession(name)
		if err != nil {
			return err
		}
		defer ls.Close()

		if user != "" {
			ls.Session.Username = user
		}

		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")

		exitCode := ls.Session.RunAndCleanup(context.Background())
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

// parseShellTarget splits "user@project" or just "project".
func parseShellTarget(target string) (user, project string) {
	for i, c := range target {
		if c == '@' {
			return target[:i], target[i+1:]
		}
	}
	return "", target
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
