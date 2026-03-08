package spawn

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/podspawn/podspawn/internal/lock"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
	"golang.org/x/term"
)

const sftpServerPath = "/usr/lib/openssh/sftp-server"

type Session struct {
	Username    string
	ProjectName string // empty = default image session
	Runtime     runtime.Runtime
	Image       string
	Shell       string
	CPUs        float64
	Memory      int64
	Store       state.SessionStore // nil = Phase 0 mode (destroy on exit)
	LockDir     string
	GracePeriod time.Duration
	MaxLifetime time.Duration
	Mode        string // "grace-period" | "destroy-on-disconnect"
}

func (s *Session) containerName() string {
	if s.ProjectName != "" {
		return "podspawn-" + s.Username + "-" + s.ProjectName
	}
	return "podspawn-" + s.Username
}

func (s *Session) Run(ctx context.Context) (int, error) {
	if s.Store != nil {
		containerName, err := s.ensureContainerWithState(ctx)
		if err != nil {
			return 1, err
		}
		return s.routeSession(ctx, containerName)
	}

	// Phase 0 fallback: no state store
	return s.ensureContainerLegacy(ctx, s.containerName())
}

// ensureContainerWithState uses locking + state store to manage the container.
func (s *Session) ensureContainerWithState(ctx context.Context) (string, error) {
	containerName := s.containerName()

	unlock, err := lock.Acquire(s.LockDir, s.Username)
	if err != nil {
		return "", fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	s.reconcileUser(ctx)

	sess, err := s.Store.GetSession(s.Username, s.ProjectName)
	if err != nil {
		return "", fmt.Errorf("checking session state: %w", err)
	}

	if sess != nil {
		alive, _ := s.Runtime.ContainerExists(ctx, sess.ContainerName)
		if !alive {
			slog.Warn("stale session, container gone", "user", s.Username, "container", sess.ContainerName)
			if err := s.Store.DeleteSession(s.Username, s.ProjectName); err != nil {
				return "", fmt.Errorf("cleaning stale session: %w", err)
			}
			sess = nil
		}
	}

	if sess != nil {
		if sess.Status == "grace_period" {
			slog.Info("cancelling grace period", "user", s.Username)
			if err := s.Store.CancelGracePeriod(s.Username, s.ProjectName); err != nil {
				return "", err
			}
		}
		if _, err := s.Store.UpdateConnections(s.Username, s.ProjectName, 1); err != nil {
			return "", err
		}
		slog.Info("reattaching to container", "name", sess.ContainerName, "connections", sess.Connections+1)
		return sess.ContainerName, nil
	}

	// Create new container
	slog.Info("creating container", "name", containerName, "image", s.Image)
	id, err := s.Runtime.CreateContainer(ctx, runtime.ContainerOpts{
		Name:   containerName,
		Image:  s.Image,
		Cmd:    []string{"sleep", "infinity"},
		CPUs:   s.CPUs,
		Memory: s.Memory,
		Labels: map[string]string{
			"managed-by":    "podspawn",
			"podspawn-user": s.Username,
		},
	})
	if err != nil {
		return "", err
	}
	if err := s.Runtime.StartContainer(ctx, containerName); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if err := s.Store.CreateSession(&state.Session{
		User:          s.Username,
		Project:       s.ProjectName,
		ContainerID:   id,
		ContainerName: containerName,
		Image:         s.Image,
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(s.MaxLifetime),
	}); err != nil {
		return "", fmt.Errorf("recording session: %w", err)
	}

	return containerName, nil
}

// ensureContainerLegacy is the Phase 0 path: no state, no locking.
func (s *Session) ensureContainerLegacy(ctx context.Context, containerName string) (int, error) {
	exists, err := s.Runtime.ContainerExists(ctx, containerName)
	if err != nil {
		return 1, fmt.Errorf("checking container %s: %w", containerName, err)
	}

	if !exists {
		slog.Info("creating container", "name", containerName, "image", s.Image)
		_, err := s.Runtime.CreateContainer(ctx, runtime.ContainerOpts{
			Name:   containerName,
			Image:  s.Image,
			Cmd:    []string{"sleep", "infinity"},
			CPUs:   s.CPUs,
			Memory: s.Memory,
			Labels: map[string]string{
				"managed-by":    "podspawn",
				"podspawn-user": s.Username,
			},
		})
		if err != nil {
			return 1, err
		}
		if err := s.Runtime.StartContainer(ctx, containerName); err != nil {
			return 1, err
		}
	} else {
		slog.Info("reattaching to container", "name", containerName)
	}

	return s.routeSession(ctx, containerName)
}

func (s *Session) routeSession(ctx context.Context, containerName string) (int, error) {
	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	switch {
	case origCmd == "":
		return s.interactiveShell(ctx, containerName)
	case isSFTP(origCmd):
		return s.execCommand(ctx, containerName, sftpServerPath)
	default:
		return s.execCommand(ctx, containerName, origCmd)
	}
}

func isSFTP(cmd string) bool {
	if cmd == "internal-sftp" {
		return true
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(fields[0]) == "sftp-server"
}

func (s *Session) interactiveShell(ctx context.Context, containerName string) (int, error) {
	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			return 1, fmt.Errorf("setting raw terminal: %w", err)
		}
		defer term.Restore(stdinFd, oldState) //nolint:errcheck // best-effort restore
	}

	exitCode, err := s.Runtime.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd:    []string{s.Shell},
		TTY:    true,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		ExecIDCallback: func(execID string) {
			go handleResize(ctx, s.Runtime, execID)
		},
	})
	if err != nil {
		return 1, err
	}

	return exitCode, nil
}

func (s *Session) execCommand(ctx context.Context, containerName, origCmd string) (int, error) {
	exitCode, err := s.Runtime.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd:    []string{"sh", "-c", origCmd},
		TTY:    false,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		return 1, err
	}
	return exitCode, nil
}

func handleResize(ctx context.Context, rt runtime.Runtime, execID string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				continue
			}
			_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
		}
	}
}

// reconcileUser cleans up stale state for the current user only.
func (s *Session) reconcileUser(ctx context.Context) {
	// Crash recovery: connections=0 with no grace expiry
	stale, err := s.Store.StaleZeroConnections(s.Username, s.ProjectName)
	if err != nil {
		slog.Warn("reconcile: failed to check stale sessions", "error", err)
		return
	}
	if stale != nil {
		slog.Info("reconcile: cleaning up stale session", "user", stale.User, "container", stale.ContainerName)
		_ = s.Runtime.RemoveContainer(ctx, stale.ContainerName)
		_ = s.Store.DeleteSession(stale.User, stale.Project)
	}

	// Expired grace period for this user/project
	sess, err := s.Store.GetSession(s.Username, s.ProjectName)
	if err != nil || sess == nil {
		return
	}
	if sess.Status == "grace_period" && sess.GraceExpiry.Valid && sess.GraceExpiry.Time.Before(time.Now()) {
		slog.Info("reconcile: grace period expired", "user", sess.User, "container", sess.ContainerName)
		_ = s.Runtime.RemoveContainer(ctx, sess.ContainerName)
		_ = s.Store.DeleteSession(sess.User, sess.Project)
	}
}

// Disconnect handles session teardown: decrements connection count,
// starts grace period or destroys container.
// Must be called with context that has enough timeout for cleanup.
func (s *Session) Disconnect(ctx context.Context) {
	if s.Store == nil {
		// Phase 0: always destroy
		containerName := s.containerName()
		if err := s.Runtime.RemoveContainer(ctx, containerName); err != nil {
			slog.Warn("cleanup failed", "container", containerName, "error", err)
		}
		return
	}

	unlock, err := lock.Acquire(s.LockDir, s.Username)
	if err != nil {
		slog.Error("disconnect: failed to acquire lock", "user", s.Username, "error", err)
		return
	}
	defer unlock()

	count, err := s.Store.UpdateConnections(s.Username, s.ProjectName, -1)
	if err != nil {
		slog.Error("disconnect: failed to decrement connections", "user", s.Username, "error", err)
		return
	}

	if count > 0 {
		slog.Info("session still active", "user", s.Username, "connections", count)
		return
	}

	containerName := s.containerName()
	if s.Mode == "destroy-on-disconnect" || s.GracePeriod == 0 {
		slog.Info("destroying container", "user", s.Username, "container", containerName)
		if err := s.Runtime.RemoveContainer(ctx, containerName); err != nil {
			slog.Warn("destroy failed", "container", containerName, "error", err)
		}
		_ = s.Store.DeleteSession(s.Username, s.ProjectName)
		return
	}

	expiry := time.Now().Add(s.GracePeriod)
	slog.Info("starting grace period", "user", s.Username, "expires", expiry)
	if err := s.Store.SetGracePeriod(s.Username, s.ProjectName, expiry); err != nil {
		slog.Error("failed to set grace period", "user", s.Username, "error", err)
	}
}

// RunAndCleanup runs the session and handles disconnect after.
// Returns the exit code to pass to os.Exit.
func (s *Session) RunAndCleanup(ctx context.Context) int {
	exitCode, err := s.Run(ctx)
	if err != nil {
		slog.Error("session failed", "user", s.Username, "error", err)
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Disconnect(cleanupCtx)

	return exitCode
}
