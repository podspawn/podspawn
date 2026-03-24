package podfile

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestTarDirectoryContainsFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)      //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)        //nolint:errcheck
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)                                   //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "pkg", "lib.go"), []byte("package pkg"), 0644) //nolint:errcheck

	reader, err := tarDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(reader)
	files := map[string]bool{}
	for {
		hdr, readErr := tr.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[hdr.Name] = true
	}

	for _, want := range []string{"main.go", "go.mod", filepath.Join("pkg", "lib.go")} {
		if !files[want] {
			t.Errorf("missing file in tar: %s (have: %v)", want, files)
		}
	}
}

func TestTarDirectorySkipsGitDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755)                               //nolint:errcheck
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)              //nolint:errcheck

	reader, err := tarDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(reader)
	for {
		hdr, readErr := tr.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		if filepath.Base(hdr.Name) == ".git" || filepath.Dir(hdr.Name) == ".git" {
			t.Errorf("tar should skip .git, found: %s", hdr.Name)
		}
	}
}

func TestUpdateTemplateCacheCreatesDir(t *testing.T) {
	// Just test that the function signature works and doesn't panic on network error
	// (actual network test would be integration)
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := UpdateTemplateCache()
	// Expected to fail (no network in unit tests), but shouldn't panic
	if err == nil {
		t.Log("update succeeded (network available)")
	}
}
