package podfile

import (
	"context"
	"fmt"
	"os/exec"
)

// CloneRepo clones a git repository to dest.
func CloneRepo(ctx context.Context, url, dest, branch string) error {
	args := []string{"clone", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, url, dest)

	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %s: %s: %w", url, string(out), err)
	}
	return nil
}

// PullRepo runs git pull in dir.
func PullRepo(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull in %s: %s: %w", dir, string(out), err)
	}
	return nil
}
