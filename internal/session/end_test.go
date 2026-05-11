package session

import (
	"context"
	"errors"
	"testing"

	"github.com/podspawn/podspawn/internal/state"
)

// fakeLifecycle records what Provision/Stop were called with so end_test
// and create_test can assert behavior without driving real containers.
type fakeLifecycle struct {
	provisionErr    error
	provisionResult ProvisionResult
	provisionCalls  int
	stopCalls       int
	stoppedSessions []*state.Session
}

func (f *fakeLifecycle) Provision(ctx context.Context, p ProvisionParams) (ProvisionResult, error) {
	f.provisionCalls++
	return f.provisionResult, f.provisionErr
}

func (f *fakeLifecycle) Stop(ctx context.Context, p StopParams) error {
	f.stopCalls++
	f.stoppedSessions = append(f.stoppedSessions, p.Session)
	// Mimic cleanup.DestroySession's row-removal side effect so List/End
	// behave like production after Stop.
	return p.Store.DeleteSession(p.Session.User, p.Session.Project)
}

func TestEndStopsLiveSession(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	if err := store.CreateSession(&state.Session{
		User:    "tenant",
		Project: "backend-main",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.End(context.Background(), Ref{User: "tenant", Name: "backend-main"}); err != nil {
		t.Fatalf("End error = %v", err)
	}
	if lc.stopCalls != 1 {
		t.Fatalf("Stop calls = %d, want 1", lc.stopCalls)
	}
	got, _ := store.GetSession("tenant", "backend-main")
	if got != nil {
		t.Fatalf("session row remained after End: %+v", got)
	}
}

func TestEndReturnsErrSessionNotFoundWhenAbsent(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	err := svc.End(context.Background(), Ref{User: "tenant", Name: "backend-main"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("End error = %v, want ErrSessionNotFound", err)
	}
	if lc.stopCalls != 0 {
		t.Fatalf("Stop calls = %d, want 0", lc.stopCalls)
	}
}

func TestEndLeavesWorkspaceUntouchedWhenNoSession(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
	})

	if err := store.CreateWorkspace(&state.Workspace{
		User:    "tenant",
		Name:    "auth-fix",
		Project: "backend",
		State:   state.WorkspaceStatePreserved,
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.End(context.Background(), Ref{User: "tenant", Name: "auth-fix"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("End error = %v, want ErrSessionNotFound", err)
	}
	got, _ := store.GetWorkspace("tenant", "auth-fix")
	if got == nil {
		t.Fatal("End must not delete workspaces")
	}
	if got.State != state.WorkspaceStatePreserved {
		t.Fatalf("workspace state = %q, want %q (End must not mutate)", got.State, state.WorkspaceStatePreserved)
	}
}

func TestEndRejectsMissingUser(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
	})
	err := svc.End(context.Background(), Ref{Name: "anything"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("End error = %v, want ErrInvalidRequest", err)
	}
}
