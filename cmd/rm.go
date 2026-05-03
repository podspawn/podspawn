package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

type machineRemovalStore interface {
	state.SessionStore
	state.MachineStore
}

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a local machine workspace and registry entry",
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

		if err := removeLocalMachine(context.Background(), ls.Session.Runtime, ls.Store, ls.Session.Username, args[0], force); err != nil {
			return err
		}

		ui.Success("Removed machine %s", args[0])
		return nil
	},
}

func removeLocalMachine(ctx context.Context, rt runtime.Runtime, store machineRemovalStore, user, name string, force bool) error {
	if strings.HasPrefix(name, ".tmp-") {
		return fmt.Errorf("refusing to remove ephemeral workspace name %q; clean up the .tmp-* directory directly", name)
	}

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
	if sess != nil && !force {
		return fmt.Errorf("machine %q is still running; use --force to remove it", name)
	}
	if sess != nil {
		if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
			return fmt.Errorf("destroying running machine: %w", err)
		}
	}

	if err := os.RemoveAll(machine.WorkspacePath); err != nil {
		return fmt.Errorf("removing workspace %s: %w", machine.WorkspacePath, err)
	}
	if err := store.DeleteMachine(user, name); err != nil {
		return fmt.Errorf("deleting machine row: %w", err)
	}
	return nil
}

func init() {
	rmCmd.Flags().Bool("force", false, "remove even if the machine is still running")
	rootCmd.AddCommand(rmCmd)
}
