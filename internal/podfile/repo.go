package podfile

import (
	"fmt"
	"os/exec"
)

// CloneRepo clones a git repository to the given destination directory.
func CloneRepo(url, dest, branch string) error {
	args := []string{"clone", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, url, dest)

	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %s: %s: %w", url, string(out), err)
	}
	return nil
}

// PullRepo runs git pull in the given directory.
func PullRepo(dir string) error {
	cmd := exec.Command("git", "-C", dir, "pull")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull in %s: %s: %w", dir, string(out), err)
	}
	return nil
}
