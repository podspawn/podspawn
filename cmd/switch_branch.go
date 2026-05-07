package cmd

import (
	"context"
	"fmt"

	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
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

		if err := switchLocalWorkspaceBranch(context.Background(), ls.Session.Runtime, ls.Store, ls.Session.Username, args[0], args[1]); err != nil {
			return err
		}

		ui.Success("Switched %s to branch %s", args[0], args[1])
		return nil
	},
}

func switchLocalWorkspaceBranch(ctx context.Context, rt runtime.Runtime, store workspaceBranchStore, user, name, branch string) error {
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

	if err := podfile.SwitchBranch(ctx, workspace.WorkspacePath, branch); err != nil {
		return fmt.Errorf("switching workspace branch: %w", err)
	}
	if err := store.UpdateWorkspaceBranch(user, name, branch); err != nil {
		return fmt.Errorf("recording workspace branch: %w", err)
	}
	if err := store.UpdateWorkspaceInitialized(user, name, false); err != nil {
		return fmt.Errorf("marking workspace for reinitialization: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(switchBranchCmd)
}
