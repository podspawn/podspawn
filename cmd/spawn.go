package cmd

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "ForceCommand handler, routes session I/O to containers",
	Run: func(cmd *cobra.Command, args []string) {
		user, _ := cmd.Flags().GetString("user")

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			slog.Error("failed to connect to docker", "error", err)
			os.Exit(1)
		}

		// These are validated by config.Load, so errors here are unreachable
		memory, _ := config.ParseMemory(cfg.Defaults.Memory)
		gracePeriod, _ := time.ParseDuration(cfg.Session.GracePeriod)
		maxLifetime, _ := time.ParseDuration(cfg.Session.MaxLifetime)

		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			slog.Warn("failed to open state db, running without state tracking", "error", err)
		}
		if store != nil {
			defer func() { _ = store.Close() }()
		}

		sess := &spawn.Session{
			Username:    user,
			Runtime:     rt,
			Image:       cfg.Defaults.Image,
			Shell:       cfg.Defaults.Shell,
			CPUs:        cfg.Defaults.CPUs,
			Memory:      memory,
			Store:       store,
			LockDir:     cfg.State.LockDir,
			GracePeriod: gracePeriod,
			MaxLifetime: maxLifetime,
			Mode:        cfg.Session.Mode,
		}

		exitCode := sess.RunAndCleanup(context.Background())
		os.Exit(exitCode)
	},
}

func init() {
	spawnCmd.Flags().String("user", "", "username for the session")
	_ = spawnCmd.MarkFlagRequired("user")
	rootCmd.AddCommand(spawnCmd)
}
