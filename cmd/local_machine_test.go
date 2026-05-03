package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

func TestSetupNamedMachineCreatesMachineRecord(t *testing.T) {
	t.Setenv("USER", "tenant")

	root := t.TempDir()
	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(root, "state.db")
	cfg.ProjectsFile = filepath.Join(root, "projects.yaml")
	t.Cleanup(func() { cfg = oldCfg })

	remotePath, registeredPath := createProjectRepo(t)
	projects := map[string]config.ProjectConfig{
		"backend": {
			Repo:      remotePath,
			LocalPath: registeredPath,
		},
	}
	if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
		t.Fatal(err)
	}

	store, err := state.Open(cfg.State.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ls := &localSession{
		Session: &spawn.Session{
			Username:     "tenant",
			Store:        store,
			MachineStore: store,
		},
		Store: store,
	}

	created, err := setupNamedMachine(context.Background(), ls, "auth-fix", "backend", "")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a new machine to be created")
	}

	machine, err := store.GetMachine("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if machine == nil {
		t.Fatal("expected machine record")
	}
	if machine.Project != "backend" {
		t.Errorf("project = %q, want backend", machine.Project)
	}
	if machine.Branch != "feat/auth-retry" {
		t.Errorf("branch = %q, want feat/auth-retry", machine.Branch)
	}
	if machine.WorkspaceTarget != "/workspace/backend" {
		t.Errorf("workspace_target = %q, want /workspace/backend", machine.WorkspaceTarget)
	}

	currentBranch := gitOutput(t, machine.WorkspacePath, "branch", "--show-current")
	if currentBranch != "feat/auth-retry" {
		t.Errorf("workspace branch = %q, want feat/auth-retry", currentBranch)
	}
}

func TestSetupNamedMachineRejectsBranchMismatch(t *testing.T) {
	t.Setenv("USER", "tenant")

	root := t.TempDir()
	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(root, "state.db")
	cfg.ProjectsFile = filepath.Join(root, "projects.yaml")
	t.Cleanup(func() { cfg = oldCfg })

	remotePath, registeredPath := createProjectRepo(t)
	projects := map[string]config.ProjectConfig{
		"backend": {
			Repo:      remotePath,
			LocalPath: registeredPath,
		},
	}
	if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
		t.Fatal(err)
	}

	store, err := state.Open(cfg.State.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	ls := &localSession{
		Session: &spawn.Session{
			Username:     "tenant",
			Store:        store,
			MachineStore: store,
		},
		Store: store,
	}

	if _, err := setupNamedMachine(context.Background(), ls, "auth-fix", "backend", "feat/auth-retry"); err != nil {
		t.Fatal(err)
	}

	_, err = setupNamedMachine(context.Background(), ls, "auth-fix", "backend", "main")
	if err == nil {
		t.Fatal("expected branch mismatch error")
	}
	if !strings.Contains(err.Error(), "already uses branch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func createProjectRepo(t *testing.T) (remotePath, registeredPath string) {
	t.Helper()

	root := t.TempDir()
	remotePath = filepath.Join(root, "remote.git")
	runGit(t, root, "init", "--bare", "--initial-branch=main", remotePath)

	worktreePath := filepath.Join(root, "worktree")
	runGit(t, root, "clone", remotePath, worktreePath)
	runGit(t, worktreePath, "config", "user.email", "tenant@example.com")
	runGit(t, worktreePath, "config", "user.name", "Tenant")

	podfileContent := "base: ubuntu:24.04\nbranch: feat/auth-retry\nworkspace: /workspace/backend\n"
	if err := os.WriteFile(filepath.Join(worktreePath, "podfile.yaml"), []byte(podfileContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "podfile.yaml", "README.md")
	runGit(t, worktreePath, "commit", "-m", "init")
	runGit(t, worktreePath, "push", "-u", "origin", "main")

	runGit(t, worktreePath, "checkout", "-b", "feat/auth-retry")
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "README.md")
	runGit(t, worktreePath, "commit", "-m", "feature")
	runGit(t, worktreePath, "push", "-u", "origin", "feat/auth-retry")

	registeredPath = filepath.Join(root, "registered")
	runGit(t, root, "clone", "--branch", "main", remotePath, registeredPath)

	return remotePath, registeredPath
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
