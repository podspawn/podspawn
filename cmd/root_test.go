package cmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFileExists_NotFound(t *testing.T) {
	exists, err := fileExists(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if exists {
		t.Fatal("expected false for missing file")
	}
}

func TestFileExists_Exists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("key: val"), 0644); err != nil {
		t.Fatal(err)
	}
	exists, err := fileExists(path)
	if err != nil {
		t.Fatalf("expected nil error for existing file, got %v", err)
	}
	if !exists {
		t.Fatal("expected true for existing file")
	}
}

func TestFileExists_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("key: val"), 0644); err != nil {
		t.Fatal(err)
	}
	// Remove read+execute from the directory so Stat on the file fails with EACCES
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	_, err := fileExists(path)
	if err == nil {
		t.Fatal("expected error for permission-denied path, got nil")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("expected permission error, got %v", err)
	}
}
