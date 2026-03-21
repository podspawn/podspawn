package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

type localSession struct {
	Session *spawn.Session
	Store   *state.Store
	closers []func()
}

func (ls *localSession) Close() {
	for _, fn := range ls.closers {
		fn()
	}
}

func buildLocalSession(name string) (*localSession, error) {
	if name != "" {
		if err := validateMachineName(name); err != nil {
			return nil, err
		}
	}

	user := os.Getenv("USER")
	if user == "" {
		return nil, fmt.Errorf("USER environment variable not set")
	}

	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		return nil, fmt.Errorf("connecting to docker: %w", err)
	}

	memory, _ := config.ParseMemory(cfg.Defaults.Memory)
	gracePeriod, _ := time.ParseDuration(cfg.Session.GracePeriod)
	maxLifetime, _ := time.ParseDuration(cfg.Session.MaxLifetime)

	store, err := state.Open(cfg.State.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening state db: %w", err)
	}

	sess := &spawn.Session{
		Username:    user,
		ProjectName: name,
		Runtime:     rt,
		Image:       cfg.Defaults.Image,
		Shell:       cfg.Defaults.Shell,
		CPUs:        cfg.Defaults.CPUs,
		Memory:      memory,
		LockDir:     cfg.State.LockDir,
		GracePeriod: gracePeriod,
		MaxLifetime: maxLifetime,
		Mode:        cfg.Session.Mode,
		HomesDir:    cfg.State.HomesDir,
		Security:    cfg.Security,
		MaxPerUser:  cfg.Resources.MaxPerUser,
		Store:       store,
	}

	ls := &localSession{
		Session: sess,
		Store:   store,
		closers: []func(){func() { _ = store.Close() }},
	}

	if name != "" {
		projects, loadErr := config.LoadProjects(cfg.ProjectsFile)
		if loadErr != nil {
			slog.Warn("failed to load projects", "error", loadErr)
		} else if p, ok := projects[name]; ok {
			sess.Project = &p
		}
	}

	if sess.Project == nil {
		if home, _ := os.UserHomeDir(); home != "" {
			podspawnDir := filepath.Join(home, ".podspawn")
			if _, statErr := os.Stat(filepath.Join(podspawnDir, "podfile.yaml")); statErr == nil {
				sess.Project = &config.ProjectConfig{LocalPath: podspawnDir}
			}
		}
	}

	if uo, loadErr := config.LoadUserOverrides("/etc/podspawn", user); loadErr != nil {
		slog.Warn("failed to load user overrides", "user", user, "error", loadErr)
	} else if uo != nil {
		sess.UserOverrides = uo
	}

	if auditLogger, auditErr := audit.Open(cfg.Log.AuditLog); auditErr != nil {
		slog.Warn("failed to open audit log", "error", auditErr)
	} else if auditLogger != nil {
		sess.Audit = auditLogger
		ls.closers = append(ls.closers, func() { _ = auditLogger.Close() })
	}

	return ls, nil
}
