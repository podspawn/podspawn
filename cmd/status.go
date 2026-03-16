package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/metrics"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status and metrics",
	Long:  `Outputs session and container metrics. Use --prometheus for Prometheus exposition format.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			return fmt.Errorf("opening state db: %w", err)
		}
		defer func() { _ = store.Close() }()

		rt, rtErr := runtime.NewDockerRuntime()
		if rtErr != nil {
			rt = nil // metrics will show 0 containers
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		snap, err := metrics.Collect(ctx, store, rt)
		if err != nil {
			return err
		}

		prom, _ := cmd.Flags().GetBool("prometheus")
		if prom {
			metrics.WritePrometheus(os.Stdout, snap)
		} else {
			metrics.WriteHuman(os.Stdout, snap)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().Bool("prometheus", false, "Output in Prometheus exposition format")
	rootCmd.AddCommand(statusCmd)
}
