package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

type shellTargetStore interface {
	GetMachine(user, name string) (*state.Machine, error)
	GetSession(user, project string) (*state.Session, error)
}

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
			if err := requireExistingShellTarget(ls.Store, ls.Session.Username, name); err != nil {
				return err
			}

			machine, machineErr := ls.Store.GetMachine(ls.Session.Username, name)
			if machineErr != nil {
				return machineErr
			}
			if machine != nil {
				configureSessionFromMachine(ls, machine, false)
			}
		}

		// Prevent routeSession from treating this as a non-interactive command
		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")

		exitCode := ls.Session.RunAndCleanup(context.Background())
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func requireExistingShellTarget(store shellTargetStore, user, name string) error {
	machine, err := store.GetMachine(user, name)
	if err != nil {
		return err
	}
	if machine != nil {
		return nil
	}

	session, err := store.GetSession(user, name)
	if err != nil {
		return err
	}
	if session != nil {
		return nil
	}

	return fmt.Errorf("no machine or session named %q for user %q", name, user)
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
