package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

type workspaceRemovalStore interface {
	state.SessionStore
	state.WorkspaceStore
}

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a local workspace and registry entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isLocalMode {
			return fmt.Errorf("podspawn rm is only available in local mode")
		}

		force, _ := cmd.Flags().GetBool("force")
		ls, err := buildLocalSession(args[0])
		if err != nil {
			return err
		}
		defer ls.Close()

		if err := removeLocalWorkspace(context.Background(), ls.Service, ls.Session.Runtime, ls.Store, ls.Session.Audit, os.Getenv("USER"), ls.Session.Username, args[0], force); err != nil {
			return err
		}

		ui.Success("Removed workspace %s", args[0])
		return nil
	},
}

// rmActor mirrors stopActor / shellActor: when the OS user invoking the
// command differs from the workspace owner, the requester is an operator
// acting on someone else's workspace.
func rmActor(invoker, owner string) identity.Actor {
	if invoker == "" || invoker == owner {
		return identity.Human(owner)
	}
	return identity.Operator(invoker)
}

func removeLocalWorkspace(ctx context.Context, svc *session.Service, rt runtime.Runtime, store workspaceRemovalStore, logger *audit.Logger, invokerOSUser, user, name string, force bool) error {
	if strings.HasPrefix(name, ".tmp-") {
		return fmt.Errorf("refusing to remove ephemeral workspace name %q; clean up the .tmp-* directory directly", name)
	}

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
	if sess != nil && !force {
		return fmt.Errorf("workspace %q is still running; use --force to remove it", name)
	}

	// Stage 8 gate: the existing lookups have already produced
	// workspace + sess; the gate consumes them. Returned err is bare
	// so the typed PolicyError (and its embedded reason) reaches the
	// CLI intact.
	actor := rmActor(invokerOSUser, user)
	if svc != nil {
		if err := svc.Authorize(ctx, policy.Request{
			Op:          policy.OpWorkspaceRemove,
			Actor:       actor,
			OwnerUser:   user,
			Workspace:   workspace,
			Session:     sess,
			ContainerID: policy.ContainerIDOf(sess),
		}); err != nil {
			return err
		}
	}

	if sess != nil {
		if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
			return fmt.Errorf("destroying running workspace: %w", err)
		}
	}

	if err := os.RemoveAll(workspace.WorkspacePath); err != nil {
		return fmt.Errorf("removing workspace %s: %w", workspace.WorkspacePath, err)
	}
	if err := store.DeleteWorkspace(user, name); err != nil {
		return fmt.Errorf("deleting workspace row: %w", err)
	}
	logger.WorkspaceDelete(audit.Subject{
		User:        user,
		Actor:       actor,
		WorkspaceID: workspace.ID,
	}, workspace.Name, workspace.Project, workspace.Branch, workspace.WorkspacePath, "rm")
	return nil
}

func init() {
	rmCmd.Flags().Bool("force", false, "remove even if the machine is still running")
	rootCmd.AddCommand(rmCmd)
}
