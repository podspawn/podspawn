package podfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0o644)            //nolint:errcheck

	name, marker := DetectProjectType(dir)
	if name != "go" {
		t.Errorf("got %q, want go", name)
	}
	if marker != "go.mod" {
		t.Errorf("marker = %q, want go.mod", marker)
	}
}

func TestDetectNode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644) //nolint:errcheck

	name, _ := DetectProjectType(dir)
	if name != "node" {
		t.Errorf("got %q, want node", name)
	}
}

func TestDetectPython(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0o644) //nolint:errcheck

	name, _ := DetectProjectType(dir)
	if name != "python" {
		t.Errorf("got %q, want python", name)
	}
}

func TestDetectFullstackWinsOverSingle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0o644)            //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)    //nolint:errcheck

	name, _ := DetectProjectType(dir)
	if name != "fullstack" {
		t.Errorf("got %q, want fullstack (higher priority)", name)
	}
}

func TestDetectEmpty(t *testing.T) {
	dir := t.TempDir()
	name, marker := DetectProjectType(dir)
	if name != "minimal" {
		t.Errorf("got %q, want minimal", name)
	}
	if marker != "" {
		t.Errorf("marker = %q, want empty", marker)
	}
}
