package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <user[@project]>",
	Short: "Stop a session and destroy its container",
	Long:  `Destroys a session's container, companion services, and network. Argument format: "alice" (default project) or "alice@backend" (specific project).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, project := parseSessionArg(args[0])

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
			return fmt.Errorf("no active session for %s", args[0])
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return fmt.Errorf("connecting to docker: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		slog.Info("stopping session", "user", user, "project", project, "container", sess.ContainerName)
		if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
			return fmt.Errorf("destroying session: %w", err)
		}

		fmt.Printf("Destroyed session: %s (container %s)\n", args[0], sess.ContainerName)
		return nil
	},
}

func parseSessionArg(arg string) (user, project string) {
	if idx := strings.Index(arg, "@"); idx >= 0 {
		return arg[:idx], arg[idx+1:]
	}
	return arg, ""
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
