package podfile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/podspawn/podspawn/internal/runtime"
)

// CloneDotfiles clones a dotfiles repo into ~/dotfiles inside the container
// and optionally runs an install script.
func CloneDotfiles(ctx context.Context, rt runtime.Runtime, containerName string, cfg *DotfilesConfig) error {
	slog.Info("cloning dotfiles", "repo", cfg.Repo, "container", containerName)
	exitCode, err := rt.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd: []string{"git", "clone", cfg.Repo, "/root/dotfiles"},
	})
	if err != nil {
		return fmt.Errorf("cloning dotfiles: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("dotfiles clone exited %d", exitCode)
	}

	if cfg.Install != "" {
		slog.Info("running dotfiles install", "script", cfg.Install)
		exitCode, err = rt.Exec(ctx, containerName, runtime.ExecOpts{
			Cmd: []string{"sh", "-c", "cd /root/dotfiles && " + cfg.Install},
		})
		if err != nil {
			return fmt.Errorf("running dotfiles install: %w", err)
		}
		if exitCode != 0 {
			slog.Warn("dotfiles install exited non-zero", "code", exitCode)
		}
	}

	return nil
}

// CloneRepoInContainer clones a project repo into the container at the
// specified path.
func CloneRepoInContainer(ctx context.Context, rt runtime.Runtime, containerName string, repo RepoConfig) error {
	slog.Info("cloning repo", "url", repo.URL, "path", repo.Path, "container", containerName)

	args := []string{"git", "clone", "--single-branch"}
	if repo.Branch != "" {
		args = append(args, "--branch", repo.Branch)
	}
	args = append(args, repo.URL)
	if repo.Path != "" {
		args = append(args, repo.Path)
	}

	exitCode, err := rt.Exec(ctx, containerName, runtime.ExecOpts{Cmd: args})
	if err != nil {
		return fmt.Errorf("cloning %s: %w", repo.URL, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("repo clone %s exited %d", repo.URL, exitCode)
	}
	return nil
}

// RunHook executes a shell command inside the container. Used for
// on_create and on_start lifecycle hooks.
func RunHook(ctx context.Context, rt runtime.Runtime, containerName, hookName, script string) {
	if script == "" {
		return
	}
	slog.Info("running hook", "hook", hookName, "container", containerName)
	exitCode, err := rt.Exec(ctx, containerName, runtime.ExecOpts{
		Cmd: []string{"sh", "-c", script},
	})
	if err != nil {
		slog.Warn("hook failed", "hook", hookName, "error", err)
		return
	}
	if exitCode != 0 {
		slog.Warn("hook exited non-zero", "hook", hookName, "code", exitCode)
	}
}
