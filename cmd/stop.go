package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a machine and destroy its container",
	Long: `Destroys a machine's container, companion services, and network.

  Local mode:  podspawn stop mydev
  Server mode: podspawn stop alice  or  podspawn stop alice@backend`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, project := resolveStopArg(args[0])

		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		sess, err := store.GetSession(user, project)
		if err != nil {
			return fmt.Errorf("looking up session: %w", err)
		}
		if sess == nil {
			return fmt.Errorf("no active machine %q", args[0])
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return fmt.Errorf("connecting to docker: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		slog.Info("stopping machine", "user", user, "name", project, "container", sess.ContainerName)
		if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
			return fmt.Errorf("destroying machine: %w", err)
		}

		fmt.Printf("destroyed machine %q (%s)\n", args[0], sess.ContainerName)
		return nil
	},
}

// resolveStopArg interprets the stop argument based on mode.
// Local mode: bare name is a machine name, user is $USER.
// Server mode: user[@project] format.
func resolveStopArg(arg string) (user, project string) {
	if idx := strings.Index(arg, "@"); idx >= 0 {
		return arg[:idx], arg[idx+1:]
	}
	if isLocalMode {
		return os.Getenv("USER"), arg
	}
	return arg, ""
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
