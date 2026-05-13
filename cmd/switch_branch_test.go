package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/state"
)

func TestSwitchLocalMachineBranchUpdatesWorkspaceAndState(t *testing.T) {
	t.Setenv("USER", "tenant")

	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	remotePath, workspacePath := createProjectRepo(t)

	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         remotePath,
		Branch:          "main",
		Mode:            "grace-period",
		WorkspacePath:   workspacePath,
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
		Initialized:     true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := switchLocalWorkspaceBranch(context.Background(), nil, rt, store, "tenant", "tenant", "auth-fix", "feat/auth-retry"); err != nil {
		t.Fatalf("switchLocalWorkspaceBranch() error = %v", err)
	}

	machine, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if machine.Branch != "feat/auth-retry" {
		t.Fatalf("branch = %q, want feat/auth-retry", machine.Branch)
	}
	if machine.Initialized {
		t.Fatal("machine should be marked uninitialized after branch switch")
	}

	currentBranch := gitOutput(t, workspacePath, "branch", "--show-current")
	if currentBranch != "feat/auth-retry" {
		t.Fatalf("workspace branch = %q, want feat/auth-retry", currentBranch)
	}
}

func TestSwitchLocalMachineBranchClearsHeadCommit(t *testing.T) {
	t.Setenv("USER", "tenant")

	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	remotePath, workspacePath := createProjectRepo(t)

	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         remotePath,
		Branch:          "main",
		Mode:            "grace-period",
		WorkspacePath:   workspacePath,
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
		Initialized:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateWorkspaceHeadCommit("tenant", "auth-fix", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"); err != nil {
		t.Fatal(err)
	}

	if err := switchLocalWorkspaceBranch(context.Background(), nil, rt, store, "tenant", "tenant", "auth-fix", "feat/auth-retry"); err != nil {
		t.Fatalf("switchLocalWorkspaceBranch() error = %v", err)
	}

	got, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if got.HeadCommit != "" {
		t.Fatalf("head_commit = %q, want empty after branch switch", got.HeadCommit)
	}
}

func TestSwitchLocalMachineBranchRefusesRunningMachine(t *testing.T) {
	t.Setenv("USER", "tenant")

	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	remotePath, workspacePath := createProjectRepo(t)

	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         remotePath,
		Branch:          "main",
		Mode:            "grace-period",
		WorkspacePath:   workspacePath,
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
		Initialized:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(&state.Session{
		User:          "tenant",
		Project:       "auth-fix",
		ContainerID:   "ctr-1",
		ContainerName: "podspawn-tenant-auth-fix",
		Image:         "ubuntu:24.04",
		Status:        state.StatusRunning,
		Connections:   1,
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
		MaxLifetime:   time.Now().UTC().Add(8 * time.Hour),
		Mode:          "grace-period",
	}); err != nil {
		t.Fatal(err)
	}
	rt.Containers["podspawn-tenant-auth-fix"] = true

	err := switchLocalWorkspaceBranch(context.Background(), nil, rt, store, "tenant", "tenant", "auth-fix", "feat/auth-retry")
	if err == nil {
		t.Fatal("expected running machine branch switch to fail")
	}
	if !strings.Contains(err.Error(), "stop it before switching branches") {
		t.Fatalf("error = %q, want running-machine hint", err)
	}
}

func TestSwitchActorResolvesHumanVsOperator(t *testing.T) {
	if got := switchActor("alice", "alice"); got.Kind != identity.KindHuman {
		t.Errorf("switchActor(alice, alice) kind = %q, want human", got.Kind)
	}
	if got := switchActor("root", "alice"); got.Kind != identity.KindOperator || got.OSUser != "root" {
		t.Errorf("switchActor(root, alice) = %+v, want operator:root", got)
	}
}

func TestSwitchLocalMachineBranchHonorsPolicyDeny(t *testing.T) {
	t.Setenv("USER", "tenant")

	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	remotePath, workspacePath := createProjectRepo(t)

	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         remotePath,
		Branch:          "main",
		Mode:            "grace-period",
		WorkspacePath:   workspacePath,
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
		Initialized:     true,
	}); err != nil {
		t.Fatal(err)
	}

	stub := &denyPolicy{reason: "branch switches blocked during freeze"}
	svc := session.New(session.Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Runtime:        rt,
		Policy:         stub,
	})

	err := switchLocalWorkspaceBranch(context.Background(), svc, rt, store, "tenant", "tenant", "auth-fix", "feat/auth-retry")
	if !errors.Is(err, policy.ErrPolicyDenied) {
		t.Fatalf("errors.Is(err, policy.ErrPolicyDenied) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "branch switches blocked during freeze") {
		t.Errorf("CLI error %q must include the policy reason", err.Error())
	}

	// Gate must have seen Workspace (with the current branch) and the
	// requested branch from args[1].
	if stub.captured.Workspace == nil || stub.captured.Workspace.Branch != "main" {
		t.Errorf("gate did not see current branch on workspace: %+v", stub.captured.Workspace)
	}
	if stub.captured.RequestedBranch != "feat/auth-retry" {
		t.Errorf("RequestedBranch = %q, want feat/auth-retry", stub.captured.RequestedBranch)
	}

	// Workspace row must be untouched: branch unchanged, no init flip.
	got, _ := store.GetWorkspace("tenant", "auth-fix")
	if got.Branch != "main" {
		t.Errorf("branch mutated on deny: %q", got.Branch)
	}
	if !got.Initialized {
		t.Error("initialized flipped on deny")
	}

	// The working tree should still be on main.
	if cur := gitOutput(t, workspacePath, "branch", "--show-current"); cur != "main" {
		t.Errorf("git branch mutated on deny: %q", cur)
	}
}

func TestSwitchLocalMachineBranchRefusesDirtyWorkspace(t *testing.T) {
	t.Setenv("USER", "tenant")

	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	remotePath, workspacePath := createProjectRepo(t)

	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         remotePath,
		Branch:          "main",
		Mode:            "grace-period",
		WorkspacePath:   workspacePath,
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
		Initialized:     true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(workspacePath, "README.md"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := switchLocalWorkspaceBranch(context.Background(), nil, rt, store, "tenant", "tenant", "auth-fix", "feat/auth-retry")
	if err == nil {
		t.Fatal("expected dirty workspace branch switch to fail")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("error = %q, want uncommitted-changes hint", err)
	}
}
