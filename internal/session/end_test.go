package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/identity"
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

func endReq(user, name string) EndRequest {
	return EndRequest{Ref: Ref{User: user, Name: name}, Actor: identity.Human(user)}
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

	if err := svc.End(context.Background(), endReq("tenant", "backend-main")); err != nil {
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

func TestEndEmitsContainerDestroyAttributedToActor(t *testing.T) {
	store := state.NewFakeStore()
	lc := &fakeLifecycle{}
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Lifecycle:      lc,
		Audit:          logger,
	})

	if err := store.CreateSession(&state.Session{
		User:          "bob",
		Project:       "backend-main",
		ContainerID:   "ctr-1",
		ContainerName: "podspawn-bob-backend-main",
		WorkspaceID:   sql.NullString{String: "ws-1", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	// An operator (root) stopping bob's session.
	if err := svc.End(context.Background(), EndRequest{
		Ref:   Ref{User: "bob", Name: "backend-main"},
		Actor: identity.Operator("root"),
	}); err != nil {
		t.Fatalf("End error = %v", err)
	}
	_ = logger.Close()

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("audit line not valid JSON: %v", err)
	}
	if entry["event"] != audit.EventDestroy {
		t.Errorf("event = %q, want %q", entry["event"], audit.EventDestroy)
	}
	if entry["reason"] != "stop" {
		t.Errorf("reason = %q, want stop", entry["reason"])
	}
	if entry["user"] != "bob" {
		t.Errorf("user = %q, want bob (owner)", entry["user"])
	}
	if entry["actor"] != "operator:root" {
		t.Errorf("actor = %q, want operator:root", entry["actor"])
	}
	if entry["actor_kind"] != "operator" {
		t.Errorf("actor_kind = %q, want operator", entry["actor_kind"])
	}
	if entry["project"] != "backend-main" {
		t.Errorf("project = %q, want backend-main", entry["project"])
	}
	if entry["container"] != "podspawn-bob-backend-main" {
		t.Errorf("container = %q, want podspawn-bob-backend-main", entry["container"])
	}
	if entry["container_id"] != "ctr-1" {
		t.Errorf("container_id = %q, want ctr-1", entry["container_id"])
	}
	if entry["workspace_id"] != "ws-1" {
		t.Errorf("workspace_id = %q, want ws-1", entry["workspace_id"])
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

	err := svc.End(context.Background(), endReq("tenant", "backend-main"))
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

	err := svc.End(context.Background(), endReq("tenant", "auth-fix"))
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
	err := svc.End(context.Background(), EndRequest{Ref: Ref{Name: "anything"}, Actor: identity.Human("tenant")})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("End error = %v, want ErrInvalidRequest", err)
	}
}

func TestEndRejectsInvalidActor(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
	})
	if err := store.CreateSession(&state.Session{User: "tenant", Project: "x"}); err != nil {
		t.Fatal(err)
	}
	err := svc.End(context.Background(), EndRequest{Ref: Ref{User: "tenant", Name: "x"}})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("End error = %v, want ErrInvalidRequest", err)
	}
}
