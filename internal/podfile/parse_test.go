package podfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFullPodfile(t *testing.T) {
	input := `
base: ubuntu:24.04
packages:
  - nodejs@22
  - ripgrep
  - fzf
shell: /bin/zsh
dotfiles:
  repo: github.com/user/dots
  install: install.sh
repos:
  - url: github.com/company/backend
    path: /workspace/backend
  - url: github.com/company/frontend
    path: /workspace/frontend
    branch: develop
env:
  EDITOR: nvim
  DATABASE_URL: "postgres://postgres:devpass@postgres:5432/dev"
services:
  - name: postgres
    image: postgres:16
    ports: [5432]
    env:
      POSTGRES_PASSWORD: devpass
  - name: redis
    image: redis:7
    ports: [6379]
ports:
  expose: [3000, 8080]
resources:
  cpus: 4
  memory: 8g
on_create: "make setup"
on_start: "echo connected"
extra_commands:
  - "apt-get install -y custom-tool"
`
	pf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if pf.Base != "ubuntu:24.04" {
		t.Errorf("base = %q, want ubuntu:24.04", pf.Base)
	}
	if len(pf.Packages) != 3 {
		t.Errorf("packages count = %d, want 3", len(pf.Packages))
	}
	if pf.Shell != "/bin/zsh" {
		t.Errorf("shell = %q, want /bin/zsh", pf.Shell)
	}
	if pf.Dotfiles == nil || pf.Dotfiles.Repo != "github.com/user/dots" {
		t.Error("dotfiles not parsed correctly")
	}
	if len(pf.Repos) != 2 {
		t.Fatalf("repos count = %d, want 2", len(pf.Repos))
	}
	if pf.Repos[0].Branch != "main" {
		t.Errorf("repo[0] branch = %q, want main (default)", pf.Repos[0].Branch)
	}
	if pf.Repos[1].Branch != "develop" {
		t.Errorf("repo[1] branch = %q, want develop", pf.Repos[1].Branch)
	}
	if len(pf.Services) != 2 {
		t.Fatalf("services count = %d, want 2", len(pf.Services))
	}
	if pf.Services[0].Name != "postgres" {
		t.Errorf("service[0] name = %q, want postgres", pf.Services[0].Name)
	}
	if len(pf.Ports.Expose) != 2 {
		t.Errorf("ports count = %d, want 2", len(pf.Ports.Expose))
	}
	if pf.Resources.CPUs != 4 {
		t.Errorf("cpus = %f, want 4", pf.Resources.CPUs)
	}
	if pf.OnCreate != "make setup" {
		t.Errorf("on_create = %q, want 'make setup'", pf.OnCreate)
	}
	if len(pf.ExtraCommands) != 1 {
		t.Errorf("extra_commands count = %d, want 1", len(pf.ExtraCommands))
	}
}

func TestParseMinimal(t *testing.T) {
	pf, err := Parse(strings.NewReader("base: ubuntu:24.04\n"))
	if err != nil {
		t.Fatal(err)
	}
	if pf.Shell != "/bin/bash" {
		t.Errorf("default shell = %q, want /bin/bash", pf.Shell)
	}
	if pf.Packages != nil {
		t.Errorf("packages should be nil, got %v", pf.Packages)
	}
}

func TestParseMissingBase(t *testing.T) {
	_, err := Parse(strings.NewReader("packages:\n  - git\n"))
	if err == nil {
		t.Fatal("expected error for missing base")
	}
	if !strings.Contains(err.Error(), "base image is required") {
		t.Errorf("error = %q, want 'base image is required'", err)
	}
}

func TestParseDuplicateServiceNames(t *testing.T) {
	input := `
base: ubuntu:24.04
services:
  - name: postgres
    image: postgres:16
  - name: postgres
    image: postgres:15
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for duplicate service names")
	}
	if !strings.Contains(err.Error(), "duplicate service name") {
		t.Errorf("error = %q, want duplicate service name", err)
	}
}

func TestParseInvalidMemory(t *testing.T) {
	input := `
base: ubuntu:24.04
resources:
  memory: "not-a-size"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid memory")
	}
	if !strings.Contains(err.Error(), "invalid resources.memory") {
		t.Errorf("error = %q, want invalid resources.memory", err)
	}
}

func TestParseRelativeShell(t *testing.T) {
	input := `
base: ubuntu:24.04
shell: zsh
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for relative shell path")
	}
	if !strings.Contains(err.Error(), "shell must be absolute path") {
		t.Errorf("error = %q, want shell must be absolute path", err)
	}
}

func TestParseRelativeRepoPath(t *testing.T) {
	input := `
base: ubuntu:24.04
repos:
  - url: github.com/co/repo
    path: relative/path
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for relative repo path")
	}
	if !strings.Contains(err.Error(), "repo path must be absolute") {
		t.Errorf("error = %q, want repo path must be absolute", err)
	}
}

func TestParseServiceMissingImage(t *testing.T) {
	input := `
base: ubuntu:24.04
services:
  - name: pg
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for service without image")
	}
	if !strings.Contains(err.Error(), "image is required") {
		t.Errorf("error = %q, want image is required", err)
	}
}

func TestParseUnknownFieldsIgnored(t *testing.T) {
	input := `
base: ubuntu:24.04
future_field: some_value
`
	pf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unknown fields should be ignored: %v", err)
	}
	if pf.Base != "ubuntu:24.04" {
		t.Errorf("base = %q", pf.Base)
	}
}

func TestParseEmptyPackages(t *testing.T) {
	input := `
base: ubuntu:24.04
packages: []
`
	pf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Packages) != 0 {
		t.Errorf("packages = %v, want empty", pf.Packages)
	}
}

func TestParseRepoPathEmptyAllowed(t *testing.T) {
	input := `
base: ubuntu:24.04
repos:
  - url: github.com/co/repo
`
	pf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if pf.Repos[0].Path != "" {
		t.Errorf("empty path should be allowed, got %q", pf.Repos[0].Path)
	}
}

func TestParseRepoURLRequired(t *testing.T) {
	input := `
base: ubuntu:24.04
repos:
  - path: /workspace/app
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for repo with empty URL")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error should mention url requirement, got: %s", err)
	}
}

func TestParseServiceEmptyName(t *testing.T) {
	input := `
base: ubuntu:24.04
services:
  - image: postgres:16
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for service without name")
	}
	if !strings.Contains(err.Error(), "service name is required") {
		t.Errorf("error = %q, want service name is required", err)
	}
}

func TestFindAndRead(t *testing.T) {
	dir := t.TempDir()

	// No podfile at all
	_, err := FindAndRead(dir)
	if err == nil {
		t.Fatal("expected error for missing podfile")
	}

	// podfile.yaml in root
	rootPodfile := filepath.Join(dir, "podfile.yaml")
	if err := os.WriteFile(rootPodfile, []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := FindAndRead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ubuntu:24.04") {
		t.Error("expected podfile content")
	}

	// .podspawn/podfile.yaml takes priority
	podspawnDir := filepath.Join(dir, ".podspawn")
	if err := os.MkdirAll(podspawnDir, 0755); err != nil {
		t.Fatal(err)
	}
	nestedPodfile := filepath.Join(podspawnDir, "podfile.yaml")
	if err := os.WriteFile(nestedPodfile, []byte("base: node:22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err = FindAndRead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "node:22") {
		t.Error(".podspawn/podfile.yaml should take priority")
	}
}
