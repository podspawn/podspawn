package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/policy"
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

		// Capture the invoker before we (maybe) overwrite Username with the
		// cross-user target. buildLocalSession seeded Username + Actor from
		// the invoking OS user; for `shell alice@backend` the owner becomes
		// alice while the actor must stay the invoker — as an operator,
		// since they are acting on someone else's session.
		invoker := ls.Session.Username
		if user != "" {
			ls.Session.Username = user
		}
		ls.Session.Actor = shellActor(invoker, user)

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
			// Stage 8 attach gate. Inspect already returned the context;
			// we consume it without any extra lookup. Returning err bare
			// preserves both errors.Is(err, policy.ErrPolicyDenied) and the
			// human-readable reason on the way to the CLI.
			if err := ls.Service.Authorize(context.Background(), policy.Request{
				Op:          policy.OpSessionAttach,
				Actor:       ls.Session.Actor,
				OwnerUser:   ls.Session.Username,
				Workspace:   res.Workspace,
				Session:     res.Session,
				ContainerID: policy.ContainerIDOf(res.Session),
			}); err != nil {
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

// shellActor resolves who is running `podspawn shell`. `invoker` is the
// process's OS user (the session's pre-parse Username); `target` is the
// user@ prefix if one was supplied, "" otherwise. When the target differs
// from the invoker, the requester is an operator acting on someone else's
// session; otherwise it's a human acting on their own. The target user is
// never substituted for the actor.
func shellActor(invoker, target string) identity.Actor {
	if target == "" || target == invoker {
		return identity.Human(invoker)
	}
	return identity.Operator(invoker)
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
