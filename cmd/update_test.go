package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirWritableOnWritableDir(t *testing.T) {
	dir := t.TempDir()
	if !dirWritable(dir) {
		t.Error("expected writable temp dir to return true")
	}
}

func TestDirWritableOnReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Skip("can't set read-only permissions (running as root?)")
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	if dirWritable(dir) {
		t.Error("expected read-only dir to return false")
	}
}

func TestInstallBinaryWritableDir(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "new-binary")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi"), 0755); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "podspawn")
	if err := os.WriteFile(dst, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := installBinary(src, dst); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\necho hi" {
		t.Errorf("binary not replaced, got %q", string(data))
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("binary should be executable")
	}

	// Staging file should be cleaned up
	if _, err := os.Stat(dst + ".new"); err == nil {
		t.Error("staging file should be removed after install")
	}
}

func TestInstallBinaryReadOnlyDirNeedsSudo(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "new-binary")
	if err := os.WriteFile(src, []byte("new content"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a read-only subdirectory to simulate /usr/local/bin
	protectedDir := filepath.Join(dir, "protected")
	if err := os.Mkdir(protectedDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protectedDir, 0755) })

	dst := filepath.Join(protectedDir, "podspawn")

	if dirWritable(protectedDir) {
		t.Skip("can't create read-only dir (running as root?)")
	}

	// installBinary will try sudo, which will fail in test env.
	// The important thing is it doesn't fail with "staging new binary: permission denied"
	// but instead fails with a sudo-related error.
	err := installBinary(src, dst)
	if err == nil {
		t.Fatal("expected error when sudo isn't available in test")
	}
	if got := err.Error(); !strings.Contains(got, "sudo cp failed") {
		t.Errorf("expected sudo-related error, got: %s", got)
	}
}
