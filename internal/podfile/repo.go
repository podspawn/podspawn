package podfile

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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

func DefaultBranch(ctx context.Context, url string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--symref", url, "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s: %s: %w", url, string(out), err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "ref:" && fields[2] == "HEAD" {
			parts := strings.Split(fields[1], "/")
			if len(parts) == 0 {
				break
			}
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("could not determine default branch for %s", url)
}
