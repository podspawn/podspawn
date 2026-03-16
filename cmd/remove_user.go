package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var removeUserCmd = &cobra.Command{
	Use:   "remove-user <username>",
	Short: "Remove a container user and destroy their active sessions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]
		force, _ := cmd.Flags().GetBool("force")

		keyFile := filepath.Join(cfg.Auth.KeyDir, username)
		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			return fmt.Errorf("user %q is not registered (no key file at %s)", username, keyFile)
		}

		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		sessions, err := store.ListSessionsByUser(username)
		if err != nil {
			return fmt.Errorf("listing sessions for %s: %w", username, err)
		}

		if len(sessions) > 0 && !force {
			return fmt.Errorf("user %s has %d active session(s); use --force to destroy them", username, len(sessions))
		}

		if len(sessions) > 0 {
			rt, rtErr := runtime.NewDockerRuntime()
			if rtErr != nil {
				return fmt.Errorf("connecting to docker: %w", rtErr)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			for _, sess := range sessions {
				slog.Info("destroying session", "user", username, "container", sess.ContainerName)
				if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
					slog.Warn("failed to destroy session", "container", sess.ContainerName, "error", err)
				}
			}
		}

		if err := os.Remove(keyFile); err != nil {
			return fmt.Errorf("removing key file: %w", err)
		}

		fmt.Fprintf(os.Stderr, "removed user %s (%d session(s) destroyed)\n", username, len(sessions))
		return nil
	},
}

func init() {
	removeUserCmd.Flags().Bool("force", false, "Destroy active sessions without confirmation")
	rootCmd.AddCommand(removeUserCmd)
}
