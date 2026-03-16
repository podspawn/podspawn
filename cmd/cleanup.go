package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Reconcile orphaned containers and enforce TTLs",
	Long:  `Runs a single cleanup pass by default. With --daemon, loops every --interval until interrupted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return fmt.Errorf("connecting to docker: %w", err)
		}

		daemon, _ := cmd.Flags().GetBool("daemon")
		intervalStr, _ := cmd.Flags().GetString("interval")
		interval, err := time.ParseDuration(intervalStr)
		if err != nil {
			return fmt.Errorf("invalid interval %q: %w", intervalStr, err)
		}

		if !daemon {
			cleanup.RunOnce(context.Background(), rt, store)
			fmt.Println("Cleanup pass complete.")
			return nil
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		return cleanup.RunDaemon(ctx, rt, store, interval)
	},
}

func init() {
	cleanupCmd.Flags().Bool("daemon", false, "Run as background cleanup daemon")
	cleanupCmd.Flags().String("interval", "60s", "Cleanup interval in daemon mode")
	rootCmd.AddCommand(cleanupCmd)
}
