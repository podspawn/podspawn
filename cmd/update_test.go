package cmd

import (
	"errors"
	"os"
	"path/filepath"
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

func TestInstallBinaryReadOnlyDirDelegatesToElevated(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "new-binary")
	if err := os.WriteFile(src, []byte("new content"), 0755); err != nil {
		t.Fatal(err)
	}

	// Read-only subdirectory standing in for /usr/local/bin
	protectedDir := filepath.Join(dir, "protected")
	if err := os.Mkdir(protectedDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protectedDir, 0755) })

	dst := filepath.Join(protectedDir, "podspawn")

	if dirWritable(protectedDir) {
		t.Skip("can't create read-only dir (running as root?)")
	}

	var gotSrc, gotDst string
	calls := 0
	orig := elevatedInstall
	elevatedInstall = func(s, d string) error {
		calls++
		gotSrc, gotDst = s, d
		return nil
	}
	t.Cleanup(func() { elevatedInstall = orig })

	if err := installBinary(src, dst); err != nil {
		t.Fatalf("installBinary returned %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("elevatedInstall called %d times, want 1", calls)
	}
	if gotSrc != src || gotDst != dst {
		t.Errorf("elevatedInstall got (%q, %q), want (%q, %q)", gotSrc, gotDst, src, dst)
	}
}

func TestInstallBinaryPropagatesElevatedError(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "new-binary")
	if err := os.WriteFile(src, []byte("new content"), 0755); err != nil {
		t.Fatal(err)
	}

	protectedDir := filepath.Join(dir, "protected")
	if err := os.Mkdir(protectedDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protectedDir, 0755) })

	if dirWritable(protectedDir) {
		t.Skip("can't create read-only dir (running as root?)")
	}

	sentinel := errors.New("no askpass, no tty")
	orig := elevatedInstall
	elevatedInstall = func(string, string) error { return sentinel }
	t.Cleanup(func() { elevatedInstall = orig })

	err := installBinary(src, filepath.Join(protectedDir, "podspawn"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("installBinary returned %v, want it to wrap %v", err, sentinel)
	}
}
