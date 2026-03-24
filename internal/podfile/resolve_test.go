package podfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveExtendsNoExtends(t *testing.T) {
	raw := &RawPodfile{Podfile: Podfile{Base: "ubuntu:24.04"}}
	got, err := ResolveExtends(raw, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q", got.Base)
	}
}

func TestResolveExtendsLocalPath(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	writeTestFile(t, basePath, "base: ubuntu:22.04\npackages: [git]\n")

	raw := &RawPodfile{
		Podfile: Podfile{Extends: "./base.yaml", Packages: []string{"npm"}},
	}
	got, err := ResolveExtends(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "ubuntu:22.04" {
		t.Errorf("base = %q, want ubuntu:22.04", got.Base)
	}
	if len(got.Packages) != 2 {
		t.Errorf("packages = %v, want [git npm]", got.Packages)
	}
}

func TestResolveExtendsLocalPathChildOverridesBase(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "base.yaml"), "base: ubuntu:22.04\nshell: /bin/sh\n")

	raw := &RawPodfile{
		Podfile: Podfile{Extends: "./base.yaml", Base: "ubuntu:24.04"},
	}
	got, err := ResolveExtends(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q, want ubuntu:24.04 (child override)", got.Base)
	}
	if got.Shell != "/bin/sh" {
		t.Errorf("shell = %q, want /bin/sh (inherited)", got.Shell)
	}
}

func TestResolveExtendsMultiLevel(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "grandparent.yaml"), "base: ubuntu:24.04\npackages: [git]\n")
	writeTestFile(t, filepath.Join(dir, "parent.yaml"), "extends: ./grandparent.yaml\npackages: [go]\n")

	raw := &RawPodfile{
		Podfile: Podfile{Extends: "./parent.yaml", Packages: []string{"node"}},
	}
	got, err := ResolveExtends(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q", got.Base)
	}
	want := []string{"git", "go", "node"}
	if len(got.Packages) != 3 {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
}

func TestResolveExtendsCircularDetection(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.yaml"), "extends: ./b.yaml\nbase: ubuntu:24.04\n")
	writeTestFile(t, filepath.Join(dir, "b.yaml"), "extends: ./a.yaml\nbase: ubuntu:24.04\n")

	raw, _ := ParseRaw(strings.NewReader("extends: ./a.yaml\nbase: ubuntu:24.04\n"))
	_, err := ResolveExtends(raw, dir)
	if err == nil {
		t.Fatal("expected circular extends error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveExtendsRegistryShorthand(t *testing.T) {
	raw := &RawPodfile{
		Podfile: Podfile{Extends: "ubuntu-dev", Packages: []string{"go"}},
	}
	got, err := ResolveExtends(raw, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q, want ubuntu:24.04", got.Base)
	}
	// Should have ubuntu-dev packages + go
	if len(got.Packages) < 2 {
		t.Errorf("packages too few: %v", got.Packages)
	}
}

func TestResolveExtendsMissingRef(t *testing.T) {
	raw := &RawPodfile{
		Podfile: Podfile{Extends: "./nonexistent.yaml"},
	}
	_, err := ResolveExtends(raw, "/tmp")
	if err == nil {
		t.Fatal("expected error for missing extends reference")
	}
}

func TestResolveExtendsDepthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a chain of 12 files (exceeds max depth of 10)
	for i := 0; i < 12; i++ {
		var content string
		if i == 11 {
			content = "base: ubuntu:24.04\n"
		} else {
			content = "extends: ./level" + string(rune('a'+i+1)) + ".yaml\nbase: ubuntu:24.04\n"
		}
		writeTestFile(t, filepath.Join(dir, "level"+string(rune('a'+i))+".yaml"), content)
	}

	raw, _ := ParseRaw(strings.NewReader("extends: ./levela.yaml\nbase: ubuntu:24.04\n"))
	_, err := ResolveExtends(raw, dir)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !strings.Contains(err.Error(), "too deep") {
		t.Errorf("unexpected error: %v", err)
	}
}
