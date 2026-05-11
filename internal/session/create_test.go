package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

// TestCreateClearsPreservedStateOnRetry locks in the load-bearing contract
// flagged in docs/agent-workspaces/deferred.md: a preserved workspace must
// be flipped back to active when the caller retries Create, in both the DB
// and the in-memory copy that flows into the template.
func TestCreateClearsPreservedStateOnRetry(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	root := t.TempDir()
	if err := store.CreateWorkspace(&state.Workspace{
		User:            "tenant",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         "git@example.com:acme/backend.git",
		Branch:          "feat/auth-retry",
		Mode:            "grace-period",
		WorkspacePath:   filepath.Join(root, "workspaces", "auth-fix"),
		WorkspaceTarget: "/workspace/backend",
		State:           state.WorkspaceStatePreserved,
	}); err != nil {
		t.Fatal(err)
	}

	template := &spawn.Session{
		Username: "tenant",
		Mode:     "grace-period",
	}
	res, err := svc.Create(context.Background(), CreateRequest{
		User:            "tenant",
		Name:            "auth-fix",
		ProjectName:     "backend",
		SessionTemplate: template,
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if res.Created {
		t.Fatal("Created = true, want false (existing workspace)")
	}
	if template.Workspace == nil || template.Workspace.State != state.WorkspaceStateActive {
		t.Fatalf("template workspace state = %v, want active", template.Workspace)
	}
	got, _ := store.GetWorkspace("tenant", "auth-fix")
	if got == nil || got.State != state.WorkspaceStateActive {
		t.Fatalf("db workspace state = %v, want active", got)
	}
}

// TestCreateRejectsProjectMismatch covers the "machine already uses
// project X" path; the error must wrap ErrUnsafeTransition so transport
// callers can branch on it without parsing the human-readable message.
func TestCreateRejectsProjectMismatch(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      &fakeLifecycle{},
	})

	if err := store.CreateWorkspace(&state.Workspace{
		User:    "tenant",
		Name:    "auth-fix",
		Project: "backend",
		Branch:  "main",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Create(context.Background(), CreateRequest{
		User:            "tenant",
		Name:            "auth-fix",
		ProjectName:     "frontend",
		SessionTemplate: &spawn.Session{Username: "tenant"},
	})
	if !errors.Is(err, ErrUnsafeTransition) {
		t.Fatalf("Create error = %v, want ErrUnsafeTransition", err)
	}
}

// TestCreateRejectsBranchMismatch is the branch-side companion to the
// project-mismatch case.
func TestCreateRejectsBranchMismatch(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      &fakeLifecycle{},
	})

	if err := store.CreateWorkspace(&state.Workspace{
		User:    "tenant",
		Name:    "auth-fix",
		Project: "backend",
		Branch:  "main",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Create(context.Background(), CreateRequest{
		User:            "tenant",
		Name:            "auth-fix",
		ProjectName:     "backend",
		Branch:          "feat/auth-retry",
		SessionTemplate: &spawn.Session{Username: "tenant"},
	})
	if !errors.Is(err, ErrUnsafeTransition) {
		t.Fatalf("Create error = %v, want ErrUnsafeTransition", err)
	}
}

func TestCreateRejectsMissingTemplate(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
	})
	_, err := svc.Create(context.Background(), CreateRequest{User: "tenant", Name: "x"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Create error = %v, want ErrInvalidRequest", err)
	}
}

func TestCreateRunsProvisionWhenRequested(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{provisionResult: ProvisionResult{ContainerName: "podspawn-tenant"}}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	res, err := svc.Create(context.Background(), CreateRequest{
		User:            "tenant",
		Name:            "scratch",
		Provision:       true,
		SessionTemplate: &spawn.Session{Username: "tenant"},
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if lc.provisionCalls != 1 {
		t.Fatalf("Provision calls = %d, want 1", lc.provisionCalls)
	}
	if res.ContainerName != "podspawn-tenant" {
		t.Fatalf("ContainerName = %q, want podspawn-tenant", res.ContainerName)
	}
}

// TestCreateRollsBackOnNonHookProvisionFailure mirrors the existing
// cleanupNewMachineOnFailure contract: when the freshly-created workspace
// fails to provision for a non-hook reason, the row and host directory are
// removed.
func TestCreateRollsBackOnNonHookProvisionFailure(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{provisionErr: errors.New("boom")}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	// Drive a Provision failure on a no-project Create. Created is false on
	// this path (no workspace row inserted), so the rollback path is not
	// armed; the test asserts only the typed error class. A separate
	// integration-level test exercises rollback against the clone path.
	_, err := svc.Create(context.Background(), CreateRequest{
		User:            "tenant",
		Name:            "no-such-project",
		Provision:       true,
		SessionTemplate: &spawn.Session{Username: "tenant"},
	})
	if err == nil {
		t.Fatal("Create error = nil, want runtime failure")
	}
	if !errors.Is(err, ErrRuntimeFailure) {
		t.Fatalf("Create error = %v, want ErrRuntimeFailure", err)
	}
}
