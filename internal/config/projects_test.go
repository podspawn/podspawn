package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectsMissing(t *testing.T) {
	projects, err := LoadProjects("/tmp/nonexistent-podspawn-projects.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected empty map, got %v", projects)
	}
}

func TestSaveAndLoadProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.yaml")

	projects := map[string]ProjectConfig{
		"backend": {
			Repo:        "github.com/company/backend",
			LocalPath:   "/var/lib/podspawn/projects/backend",
			PodfileHash: "abc123",
			ImageTag:    "podspawn/backend:podfile-abc123",
		},
		"frontend": {
			Repo:      "github.com/company/frontend",
			LocalPath: "/var/lib/podspawn/projects/frontend",
		},
	}

	if err := SaveProjects(path, projects); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProjects(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(loaded))
	}
	if loaded["backend"].Repo != "github.com/company/backend" {
		t.Errorf("backend repo = %q", loaded["backend"].Repo)
	}
	if loaded["backend"].ImageTag != "podspawn/backend:podfile-abc123" {
		t.Errorf("backend image_tag = %q", loaded["backend"].ImageTag)
	}
	if loaded["frontend"].LocalPath != "/var/lib/podspawn/projects/frontend" {
		t.Errorf("frontend local_path = %q", loaded["frontend"].LocalPath)
	}
}

func TestSaveProjectsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.yaml")

	// Write initial
	if err := SaveProjects(path, map[string]ProjectConfig{
		"v1": {Repo: "old"},
	}); err != nil {
		t.Fatal(err)
	}

	// Overwrite
	if err := SaveProjects(path, map[string]ProjectConfig{
		"v2": {Repo: "new"},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProjects(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded["v1"]; ok {
		t.Error("v1 should be overwritten")
	}
	if loaded["v2"].Repo != "new" {
		t.Errorf("v2 repo = %q, want 'new'", loaded["v2"].Repo)
	}

	// No temp files left behind
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected 1 file, got %v", names)
	}
}

func TestSaveProjectsCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	path := filepath.Join(dir, "projects.yaml")

	if err := SaveProjects(path, map[string]ProjectConfig{}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}
