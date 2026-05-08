//go:build integration

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

func mustDockerRuntimeForCmd(t *testing.T) runtime.Runtime {
	t.Helper()
	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if _, err := rt.ListContainers(context.Background(), map[string]string{"managed-by": "podspawn"}); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	return rt
}

func TestBranchWorkspaceLifecycleIntegration(t *testing.T) {
	t.Setenv("USER", "tenant")

	root := t.TempDir()
	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(root, "state.db")
	cfg.ProjectsFile = filepath.Join(root, "projects.yaml")
	cfg.Defaults.Image = "ubuntu:24.04"
	cfg.Defaults.Shell = "/bin/bash"
	t.Cleanup(func() { cfg = oldCfg })

	remotePath, registeredPath := createProjectRepoWithCreateHook(t)
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

	rt := mustDockerRuntimeForCmd(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const machineName = "auth-fix-integ"
	containerName := "podspawn-tenant-" + machineName
	t.Cleanup(func() {
		_ = rt.RemoveContainer(context.Background(), containerName)
	})

	createSession := &spawn.Session{
		Username:       "tenant",
		ProjectName:    machineName,
		Runtime:        rt,
		Image:          "ubuntu:24.04",
		Shell:          "/bin/bash",
		Store:          store,
		WorkspaceStore: store,
		LockDir:        filepath.Join(root, "locks"),
		GracePeriod:    60 * time.Second,
		MaxLifetime:    8 * time.Hour,
		Mode:           "grace-period",
		HookFatal:      true,
	}
	ls := &localSession{Session: createSession, Store: store}

	created, err := setupNamedMachine(ctx, ls, machineName, "backend", "")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected create to register a new machine")
	}

	if _, err := createSession.Ensure(ctx); err != nil {
		t.Fatal(err)
	}

	machine, err := store.GetWorkspace("tenant", machineName)
	if err != nil {
		t.Fatal(err)
	}
	if machine == nil {
		t.Fatal("expected machine after create")
	}

	rows, err := collectLocalMachineRows(store, "tenant", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "running" {
		t.Fatalf("rows after create = %+v, want one running machine", rows)
	}
	if _, err := os.Stat(machine.WorkspacePath); err != nil {
		t.Fatalf("workspace should exist after create: %v", err)
	}

	hookCountPath := filepath.Join(machine.WorkspacePath, ".hook-count")
	if count := countNonEmptyLines(t, hookCountPath); count != 1 {
		t.Fatalf("hook count after create = %d, want 1", count)
	}

	sess, err := store.GetSession("tenant", machineName)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("expected session row after create")
	}

	if err := cleanup.DestroySession(ctx, rt, store, sess); err != nil {
		t.Fatal(err)
	}

	rows, err = collectLocalMachineRows(store, "tenant", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "stopped" {
		t.Fatalf("rows after stop = %+v, want one stopped machine", rows)
	}
	if _, err := os.Stat(machine.WorkspacePath); err != nil {
		t.Fatalf("workspace should survive stop: %v", err)
	}

	shellSession := &spawn.Session{
		Username:       "tenant",
		Runtime:        rt,
		Image:          "ubuntu:24.04",
		Shell:          "/bin/bash",
		Store:          store,
		WorkspaceStore: store,
		LockDir:        filepath.Join(root, "locks"),
		GracePeriod:    60 * time.Second,
		MaxLifetime:    8 * time.Hour,
		Mode:           "grace-period",
		HookFatal:      true,
	}
	shellLS := &localSession{Session: shellSession, Store: store}
	configureSessionFromWorkspace(shellLS, machine, false)

	if _, err := shellSession.Ensure(ctx); err != nil {
		t.Fatal(err)
	}

	rows, err = collectLocalMachineRows(store, "tenant", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "running" {
		t.Fatalf("rows after shell recreate = %+v, want one running machine", rows)
	}
	if count := countNonEmptyLines(t, hookCountPath); count != 1 {
		t.Fatalf("hook count after shell recreate = %d, want 1", count)
	}
}

// TestTwoWorkspacesFromOneProjectKeepIndependentGitState proves the Stage-4
// substrate guarantee: each named workspace gets its own clone, so a commit
// in one cannot bleed into a sibling workspace from the same registered
// project. No Docker; just exercise setupNamedMachine through the clone path.
func TestTwoWorkspacesFromOneProjectKeepIndependentGitState(t *testing.T) {
	t.Setenv("USER", "tenant")

	root := t.TempDir()
	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(root, "state.db")
	cfg.ProjectsFile = filepath.Join(root, "projects.yaml")
	cfg.Defaults.Image = "ubuntu:24.04"
	cfg.Defaults.Shell = "/bin/bash"
	t.Cleanup(func() { cfg = oldCfg })

	// Force the workspace clones into the test root rather than ~/.podspawn,
	// since localWorkspacesRoot reads HOME.
	t.Setenv("HOME", root)

	remotePath, _ := createProjectRepo(t)
	projects := map[string]config.ProjectConfig{
		"backend": {Repo: remotePath},
	}
	if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
		t.Fatal(err)
	}

	store, err := state.Open(cfg.State.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	newSession := func() *spawn.Session {
		return &spawn.Session{
			Username:       "tenant",
			Store:          store,
			WorkspaceStore: store,
			Mode:           "grace-period",
		}
	}

	mainSess := newSession()
	mainLS := &localSession{Session: mainSess, Store: store}
	if _, err := setupNamedMachine(context.Background(), mainLS, "auth-a", "backend", "main"); err != nil {
		t.Fatalf("setup auth-a: %v", err)
	}

	featSess := newSession()
	featLS := &localSession{Session: featSess, Store: store}
	if _, err := setupNamedMachine(context.Background(), featLS, "auth-b", "backend", "feat/auth-retry"); err != nil {
		t.Fatalf("setup auth-b: %v", err)
	}

	mainPath := mainSess.WorkspacePath
	featPath := featSess.WorkspacePath
	if mainPath == "" || featPath == "" {
		t.Fatalf("workspace paths missing: main=%q feat=%q", mainPath, featPath)
	}
	if mainPath == featPath {
		t.Fatalf("workspaces must not share a path; both got %q", mainPath)
	}

	mainBranch := gitOutput(t, mainPath, "branch", "--show-current")
	featBranch := gitOutput(t, featPath, "branch", "--show-current")
	if mainBranch != "main" {
		t.Fatalf("auth-a checked out %q, want main", mainBranch)
	}
	if featBranch != "feat/auth-retry" {
		t.Fatalf("auth-b checked out %q, want feat/auth-retry", featBranch)
	}

	// Commit a unique file inside auth-a; auth-b must not see it.
	runGit(t, mainPath, "config", "user.email", "tenant@example.com")
	runGit(t, mainPath, "config", "user.name", "Tenant")
	probe := "auth-a-only.txt"
	if err := os.WriteFile(filepath.Join(mainPath, probe), []byte("local-only"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, mainPath, "add", probe)
	runGit(t, mainPath, "commit", "-m", "auth-a probe")

	if _, err := os.Stat(filepath.Join(featPath, probe)); !os.IsNotExist(err) {
		t.Fatalf("auth-b should not see auth-a's probe file (err=%v)", err)
	}

	mainHead := gitOutput(t, mainPath, "rev-parse", "HEAD")
	featHead := gitOutput(t, featPath, "rev-parse", "HEAD")
	if mainHead == featHead {
		t.Fatalf("workspaces share HEAD %q; expected divergence after auth-a commit", mainHead)
	}
}

func createProjectRepoWithCreateHook(t *testing.T) (remotePath, registeredPath string) {
	t.Helper()

	root := t.TempDir()
	remotePath = filepath.Join(root, "remote.git")
	runGit(t, root, "init", "--bare", "--initial-branch=main", remotePath)

	worktreePath := filepath.Join(root, "worktree")
	runGit(t, root, "clone", remotePath, worktreePath)
	runGit(t, worktreePath, "config", "user.email", "tenant@example.com")
	runGit(t, worktreePath, "config", "user.name", "Tenant")

	podfileContent := strings.Join([]string{
		"base: ubuntu:24.04",
		"branch: main",
		"workspace: /workspace/backend",
		"on_create: |",
		"  echo created >> /workspace/backend/.hook-count",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(worktreePath, "podfile.yaml"), []byte(podfileContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "podfile.yaml", "README.md")
	runGit(t, worktreePath, "commit", "-m", "init")
	runGit(t, worktreePath, "push", "-u", "origin", "main")

	registeredPath = filepath.Join(root, "registered")
	runGit(t, root, "clone", "--branch", "main", remotePath, registeredPath)

	return remotePath, registeredPath
}

func countNonEmptyLines(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
