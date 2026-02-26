package spawn

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"golang.org/x/term"
)

const defaultImage = "ubuntu:24.04"

type Session struct {
	Username string
	Runtime  runtime.Runtime
}

func (s *Session) Run(ctx context.Context) (int, error) {
	containerName := "podspawn-" + s.Username

	exists, err := s.Runtime.ContainerExists(ctx, containerName)
	if err != nil {
		return 1, fmt.Errorf("checking container %s: %w", containerName, err)
	}

	if !exists {
		slog.Info("creating container", "name", containerName, "image", defaultImage)
		_, err := s.Runtime.CreateContainer(ctx, runtime.ContainerOpts{
			Name:  containerName,
			Image: defaultImage,
			Cmd:   []string{"sleep", "infinity"},
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

	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if origCmd == "" {
		return s.interactiveShell(ctx, containerName)
	}
	return s.execCommand(ctx, containerName, origCmd)
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
		Cmd:    []string{"/bin/bash"},
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

// Cleanup removes the user's container. Phase 0 calls this on
// session end; later phases will add grace periods.
func (s *Session) Cleanup(ctx context.Context) {
	containerName := "podspawn-" + s.Username
	if err := s.Runtime.RemoveContainer(ctx, containerName); err != nil {
		slog.Warn("cleanup failed", "container", containerName, "error", err)
	}
}

// RunAndCleanup runs the session and removes the container after.
// Returns the exit code to pass to os.Exit.
func (s *Session) RunAndCleanup(ctx context.Context) int {
	exitCode, err := s.Run(ctx)
	if err != nil {
		slog.Error("session failed", "user", s.Username, "error", err)
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Cleanup(cleanupCtx)

	return exitCode
}
