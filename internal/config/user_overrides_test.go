package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUserOverridesMissing(t *testing.T) {
	uo, err := LoadUserOverrides(t.TempDir(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if uo != nil {
		t.Error("expected nil for missing user")
	}
}

func TestLoadUserOverrides(t *testing.T) {
	baseDir := t.TempDir()
	usersDir := filepath.Join(baseDir, "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
image: custom:latest
cpus: 8
memory: 16g
shell: /bin/zsh
env:
  EDITOR: nvim
  TERM: xterm-256color
dotfiles:
  repo: github.com/user/dots
  install: ./install.sh
`
	if err := os.WriteFile(filepath.Join(usersDir, "deploy.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	uo, err := LoadUserOverrides(baseDir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if uo == nil {
		t.Fatal("expected overrides")
	}
	if uo.Image != "custom:latest" {
		t.Errorf("image = %q", uo.Image)
	}
	if uo.CPUs != 8 {
		t.Errorf("cpus = %f", uo.CPUs)
	}
	if uo.Memory != "16g" {
		t.Errorf("memory = %q", uo.Memory)
	}
	if uo.Shell != "/bin/zsh" {
		t.Errorf("shell = %q", uo.Shell)
	}
	if uo.Env["EDITOR"] != "nvim" {
		t.Errorf("env EDITOR = %q", uo.Env["EDITOR"])
	}
	if uo.Dotfiles == nil || uo.Dotfiles.Repo != "github.com/user/dots" {
		t.Error("dotfiles not parsed")
	}
}

func TestLoadUserOverridesPartial(t *testing.T) {
	baseDir := t.TempDir()
	usersDir := filepath.Join(baseDir, "users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(usersDir, "deploy.yaml"), []byte("cpus: 4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	uo, err := LoadUserOverrides(baseDir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if uo.CPUs != 4 {
		t.Errorf("cpus = %f, want 4", uo.CPUs)
	}
	if uo.Image != "" {
		t.Errorf("image should be empty, got %q", uo.Image)
	}
}
