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

type machineBranchStore interface {
	state.SessionStore
	state.MachineStore
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

		if err := switchLocalMachineBranch(context.Background(), ls.Session.Runtime, ls.Store, ls.Session.Username, args[0], args[1]); err != nil {
			return err
		}

		ui.Success("Switched %s to branch %s", args[0], args[1])
		return nil
	},
}

func switchLocalMachineBranch(ctx context.Context, rt runtime.Runtime, store machineBranchStore, user, name, branch string) error {
	machine, err := store.GetMachine(user, name)
	if err != nil {
		return fmt.Errorf("checking machine registry: %w", err)
	}
	if machine == nil {
		return fmt.Errorf("no machine %q for user %q", name, user)
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
		return fmt.Errorf("machine %q is still running; stop it before switching branches", name)
	}

	if err := podfile.SwitchBranch(ctx, machine.WorkspacePath, branch); err != nil {
		return fmt.Errorf("switching workspace branch: %w", err)
	}
	if err := store.UpdateMachineBranch(user, name, branch); err != nil {
		return fmt.Errorf("recording machine branch: %w", err)
	}
	if err := store.UpdateMachineInitialized(user, name, false); err != nil {
		return fmt.Errorf("marking machine for reinitialization: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(switchBranchCmd)
}
