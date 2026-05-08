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

func CheckoutBranch(ctx context.Context, dir, branch string) error {
	if branch == "" {
		return nil
	}

	refspec := branch + ":refs/remotes/origin/" + branch
	fetch := exec.CommandContext(ctx, "git", "-C", dir, "fetch", "origin", refspec)
	fetchOut, err := fetch.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch %s in %s: %s: %w", branch, dir, string(fetchOut), err)
	}

	checkout := exec.CommandContext(ctx, "git", "-C", dir, "checkout", "-B", branch, "origin/"+branch)
	checkoutOut, err := checkout.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout %s in %s: %s: %w", branch, dir, string(checkoutOut), err)
	}

	return nil
}

func SwitchBranch(ctx context.Context, dir, branch string) error {
	if branch == "" {
		return nil
	}

	clean, err := IsClean(ctx, dir)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("workspace %s has uncommitted changes", dir)
	}

	return CheckoutBranch(ctx, dir, branch)
}

// IsClean reports whether the working tree at dir has no uncommitted changes.
// "Clean" is defined as `git status --porcelain` returning empty stdout.
func IsClean(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %s: %w", dir, string(out), err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

// HeadCommit returns the SHA at HEAD for the repo at dir. If a hook performed
// a commit during on_create (e.g. a generated lockfile), that commit is what
// gets returned: the capture happens after hooks run, by design.
func HeadCommit(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse in %s: %s: %w", dir, string(out), err)
	}
	return strings.TrimSpace(string(out)), nil
}
