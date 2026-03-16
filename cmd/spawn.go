package cmd

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
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
		project, _ := cmd.Flags().GetString("project")

		// Project routing: SSH client sends PODSPAWN_PROJECT=<hostname>.pod
		// via SetEnv/SendEnv. Strip the .pod suffix to get the project name.
		if project == "" {
			if envProject := os.Getenv("PODSPAWN_PROJECT"); envProject != "" {
				project = strings.TrimSuffix(envProject, ".pod")
			}
		}

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
			ProjectName: project,
			Runtime:     rt,
			Image:       cfg.Defaults.Image,
			Shell:       cfg.Defaults.Shell,
			CPUs:        cfg.Defaults.CPUs,
			Memory:      memory,
			LockDir:     cfg.State.LockDir,
			GracePeriod: gracePeriod,
			MaxLifetime: maxLifetime,
			Mode:        cfg.Session.Mode,
			Security:    cfg.Security,
		}
		if store != nil {
			sess.Store = store
		}

		if project != "" {
			projects, loadErr := config.LoadProjects(cfg.ProjectsFile)
			if loadErr != nil {
				slog.Warn("failed to load projects", "error", loadErr)
			} else if p, ok := projects[project]; ok {
				sess.Project = &p
			}
		}

		uo, err := config.LoadUserOverrides("/etc/podspawn", user)
		if err != nil {
			slog.Warn("failed to load user overrides", "user", user, "error", err)
		}
		if uo != nil {
			sess.UserOverrides = uo
		}

		auditLogger, auditErr := audit.Open(cfg.Log.AuditLog)
		if auditErr != nil {
			slog.Warn("failed to open audit log", "error", auditErr)
		}
		if auditLogger != nil {
			defer func() { _ = auditLogger.Close() }()
		}
		sess.Audit = auditLogger

		exitCode := sess.RunAndCleanup(context.Background())
		os.Exit(exitCode)
	},
}

func init() {
	spawnCmd.Flags().String("user", "", "username for the session")
	spawnCmd.Flags().String("project", "", "project name for podfile-aware sessions")
	_ = spawnCmd.MarkFlagRequired("user")
	rootCmd.AddCommand(spawnCmd)
}
