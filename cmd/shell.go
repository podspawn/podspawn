package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/podspawn/podspawn/internal/session"
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

		if isLocalMode {
			res, err := ls.Service.Inspect(context.Background(), session.Ref{
				User: ls.Session.Username,
				Name: name,
			})
			if err != nil {
				if errors.Is(err, session.ErrWorkspaceNotFound) {
					return fmt.Errorf("no workspace or session named %q for user %q", name, ls.Session.Username)
				}
				return err
			}
			if res.Workspace != nil {
				// Guardrail: cmd/shell.go may touch the spawn template;
				// this path uses ApplyWorkspaceToTemplate directly rather
				// than CreateResult.Attach() because Inspect is read-only.
				session.ApplyWorkspaceToTemplate(ls.Session, res.Workspace, false)
			}
		}

		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")

		exitCode := ls.Session.RunAndCleanup(context.Background())
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func parseShellTarget(target string) (user, project string) {
	if u, p, ok := strings.Cut(target, "@"); ok {
		return u, p
	}
	return "", target
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
