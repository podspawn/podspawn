package podfile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit sets up a fresh repo with one initial commit so HeadCommit/IsClean
// have something real to inspect. Skips the test if git isn't on PATH.
func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "ops@podspawn.test"},
		{"config", "user.name", "Stage4 Test"},
		{"commit", "--allow-empty", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, string(out), err)
		}
	}
	return dir
}

func TestHeadCommitReturnsCurrentSHA(t *testing.T) {
	dir := gitInit(t)

	got, err := HeadCommit(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 || strings.TrimSpace(got) != got {
		t.Fatalf("expected 40-char trimmed SHA, got %q", got)
	}

	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	want, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("HeadCommit = %q, want %q", got, strings.TrimSpace(string(want)))
	}
}

func TestIsCleanDetectsUncommittedFile(t *testing.T) {
	dir := gitInit(t)

	clean, err := IsClean(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Fatal("fresh repo with one commit should be clean")
	}

	scratch := filepath.Join(dir, "scratch.txt")
	if err := os.WriteFile(scratch, []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	clean, err = IsClean(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if clean {
		t.Fatal("untracked file should make IsClean return false")
	}
}

func TestHeadCommitRejectsNonRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := HeadCommit(context.Background(), t.TempDir()); err == nil {
		t.Fatal("HeadCommit should error in a non-git directory")
	}
}
