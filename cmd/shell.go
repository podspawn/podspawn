package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell [user@]<name>",
	Short: "Attach to a container directly (no SSH)",
	Long: `Open a shell in a podspawn container using Docker exec.

Use this when SSH isn't available (e.g., OrbStack is using port 22).
Unlike ssh, this does not support SFTP, scp, port forwarding, or VS Code Remote.

  podspawn shell myproject          -> container podspawn-<you>-myproject
  podspawn shell alice@backend      -> container podspawn-alice-backend
  podspawn shell alice              -> container podspawn-alice (default image)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, project := parseShellTarget(args[0])

		if user == "" {
			user = os.Getenv("USER")
			if user == "" {
				return fmt.Errorf("could not determine username, use user@name syntax")
			}
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return fmt.Errorf("connecting to docker: %w", err)
		}

		memory, _ := config.ParseMemory(cfg.Defaults.Memory)
		gracePeriod, _ := time.ParseDuration(cfg.Session.GracePeriod)
		maxLifetime, _ := time.ParseDuration(cfg.Session.MaxLifetime)

		store, err := state.Open(cfg.State.DBPath)
		if err != nil {
			slog.Warn("running without state tracking", "error", err)
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
			MaxPerUser:  cfg.Resources.MaxPerUser,
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

		// Clear SSH_ORIGINAL_COMMAND so spawn routes to interactive shell
		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")

		exitCode := sess.RunAndCleanup(context.Background())
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

// parseShellTarget splits "user@project" or just "project".
func parseShellTarget(target string) (user, project string) {
	for i, c := range target {
		if c == '@' {
			return target[:i], target[i+1:]
		}
	}
	return "", target
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
