package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

func buildLocalSession(name string) (*spawn.Session, *state.Store, error) {
	user := os.Getenv("USER")
	if user == "" {
		return nil, nil, fmt.Errorf("USER environment variable not set")
	}

	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to docker: %w", err)
	}

	memory, _ := config.ParseMemory(cfg.Defaults.Memory)
	gracePeriod, _ := time.ParseDuration(cfg.Session.GracePeriod)
	maxLifetime, _ := time.ParseDuration(cfg.Session.MaxLifetime)

	store, err := state.Open(cfg.State.DBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening state db: %w", err)
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
		Security:    cfg.Security,
		MaxPerUser:  cfg.Resources.MaxPerUser,
		Store:       store,
	}

	auditLogger, auditErr := audit.Open(cfg.Log.AuditLog)
	if auditErr == nil && auditLogger != nil {
		sess.Audit = auditLogger
	}

	return sess, store, nil
}
