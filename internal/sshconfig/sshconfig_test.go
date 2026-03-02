package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsurePodBlockCreatesFromScratch(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".ssh", "config")

	added, err := EnsurePodBlock(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("expected block to be added")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Host *.pod") {
		t.Error("file should contain Host *.pod")
	}
	if !strings.Contains(string(data), "podspawn connect") {
		t.Error("file should contain ProxyCommand")
	}
}

func TestEnsurePodBlockAppendsToExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config")
	existing := "Host github.com\n    IdentityFile ~/.ssh/github\n"
	if err := os.WriteFile(configPath, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	added, err := EnsurePodBlock(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("expected block to be added")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Host github.com") {
		t.Error("original content should be preserved")
	}
	if !strings.Contains(content, "Host *.pod") {
		t.Error("pod block should be appended")
	}
}

func TestEnsurePodBlockIdempotent(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".ssh", "config")

	if _, err := EnsurePodBlock(configPath); err != nil {
		t.Fatal(err)
	}

	before, _ := os.ReadFile(configPath)

	added, err := EnsurePodBlock(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("second call should not add the block again")
	}

	after, _ := os.ReadFile(configPath)
	if string(before) != string(after) {
		t.Error("file should be unchanged on second call")
	}
}

func TestEnsurePodBlockExistingNoTrailingNewline(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ssh")
	os.MkdirAll(dir, 0700) //nolint:errcheck
	configPath := filepath.Join(dir, "config")
	os.WriteFile(configPath, []byte("Host foo\n    Bar baz"), 0600) //nolint:errcheck

	added, err := EnsurePodBlock(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("expected block to be added")
	}

	data, _ := os.ReadFile(configPath)
	lines := strings.Split(string(data), "\n")
	foundSeparation := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "Host *.pod" && i > 0 && strings.TrimSpace(lines[i-1]) == "" {
			foundSeparation = true
		}
	}
	if !foundSeparation {
		t.Error("pod block should be separated from existing content by a blank line")
	}
}

func TestEnsurePodBlockDetectsWithWhitespace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ssh")
	os.MkdirAll(dir, 0700) //nolint:errcheck
	configPath := filepath.Join(dir, "config")
	os.WriteFile(configPath, []byte("  Host *.pod  \n    ProxyCommand something-else\n"), 0600) //nolint:errcheck

	added, err := EnsurePodBlock(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("should detect existing Host *.pod even with whitespace")
	}
}

func TestEnsurePodBlockDirPermissions(t *testing.T) {
	sshDir := filepath.Join(t.TempDir(), ".ssh")
	configPath := filepath.Join(sshDir, "config")

	if _, err := EnsurePodBlock(configPath); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("dir perms = %o, want 0700", perm)
	}
}

func TestEnsurePodBlockFilePermissions(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".ssh", "config")

	if _, err := EnsurePodBlock(configPath); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perms = %o, want 0600", perm)
	}
}
