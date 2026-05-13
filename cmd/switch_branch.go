package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

type workspaceBranchStore interface {
	state.SessionStore
	state.WorkspaceStore
}

var switchBranchCmd = &cobra.Command{
	Use:   "switch-branch <name> <branch>",
	Short: "Switch a stopped local machine workspace to a different git branch",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isLocalMode {
			return fmt.Errorf("podspawn switch-branch is only available in local mode")
		}

		ls, err := buildLocalSession(args[0])
		if err != nil {
			return err
		}
		defer ls.Close()

		if err := switchLocalWorkspaceBranch(context.Background(), ls.Service, ls.Session.Runtime, ls.Store, os.Getenv("USER"), ls.Session.Username, args[0], args[1]); err != nil {
			return err
		}

		ui.Success("Switched %s to branch %s", args[0], args[1])
		return nil
	},
}

// switchActor mirrors rmActor / stopActor / shellActor: cross-user
// invocations show up as operator-on-someone-else, not as the owner.
func switchActor(invoker, owner string) identity.Actor {
	if invoker == "" || invoker == owner {
		return identity.Human(owner)
	}
	return identity.Operator(invoker)
}

func switchLocalWorkspaceBranch(ctx context.Context, svc *session.Service, rt runtime.Runtime, store workspaceBranchStore, invokerOSUser, user, name, branch string) error {
	workspace, err := store.GetWorkspace(user, name)
	if err != nil {
		return fmt.Errorf("checking workspace registry: %w", err)
	}
	if workspace == nil {
		return fmt.Errorf("no workspace %q for user %q", name, user)
	}

	sess, err := store.GetSession(user, name)
	if err != nil {
		return fmt.Errorf("checking active sessions: %w", err)
	}
	if sess != nil && rt != nil {
		alive, err := rt.ContainerExists(ctx, sess.ContainerName)
		if err != nil {
			return fmt.Errorf("checking running container: %w", err)
		}
		if !alive {
			if err := store.DeleteSession(user, name); err != nil {
				return fmt.Errorf("cleaning stale session: %w", err)
			}
			sess = nil
		}
	}
	if sess != nil {
		return fmt.Errorf("workspace %q is still running; stop it before switching branches", name)
	}

	// Stage 8 gate: the existing lookups have already produced
	// workspace + sess; the gate consumes them. RequestedBranch is
	// args[1]. Returned err is bare so the PolicyError (including
	// reason) reaches the CLI.
	if svc != nil {
		if err := svc.Authorize(ctx, policy.Request{
			Op:              policy.OpWorkspaceBranchSwitch,
			Actor:           switchActor(invokerOSUser, user),
			OwnerUser:       user,
			Workspace:       workspace,
			Session:         sess,
			ContainerID:     policy.ContainerIDOf(sess),
			RequestedBranch: branch,
		}); err != nil {
			return err
		}
	}

	if err := podfile.SwitchBranch(ctx, workspace.WorkspacePath, branch); err != nil {
		return fmt.Errorf("switching workspace branch: %w", err)
	}
	if err := store.UpdateWorkspaceBranch(user, name, branch); err != nil {
		return fmt.Errorf("recording workspace branch: %w", err)
	}
	if err := store.UpdateWorkspaceInitialized(user, name, false); err != nil {
		return fmt.Errorf("marking workspace for reinitialization: %w", err)
	}
	if err := store.UpdateWorkspaceHeadCommit(user, name, ""); err != nil {
		return fmt.Errorf("clearing workspace head_commit: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(switchBranchCmd)
}
