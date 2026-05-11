package session

import (
	"context"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/state"
)

func newListService(t *testing.T) (*Service, *state.FakeStore) {
	t.Helper()
	store := state.NewFakeStore()
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
	})
	return svc, store
}

func TestListIncludesStoppedMachines(t *testing.T) {
	svc, store := newListService(t)
	now := time.Now().UTC()

	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "backend-main",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-main",
		Initialized:   true,
		CreatedAt:     now.Add(-2 * time.Hour),
	})
	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "backend-develop",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-develop",
		Initialized:   false,
		CreatedAt:     now.Add(-90 * time.Minute),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "backend-main",
		Image:        "ubuntu:24.04",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-45 * time.Minute),
		LastActivity: now.Add(-5 * time.Minute),
		MaxLifetime:  now.Add(7 * time.Hour),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "scratch",
		Image:        "golang:1.24",
		Status:       state.StatusGracePeriod,
		CreatedAt:    now.Add(-30 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		MaxLifetime:  now.Add(3 * time.Hour),
	})

	res, err := svc.List(context.Background(), ListRequest{User: "tenant"})
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(res.Rows))
	}
	if res.Rows[0].Name != "backend-develop" || res.Rows[0].Status != "uninitialized" {
		t.Fatalf("row 0 = %+v, want backend-develop/uninitialized", res.Rows[0])
	}
	if res.Rows[1].Name != "backend-main" || res.Rows[1].Status != "running" {
		t.Fatalf("row 1 = %+v, want backend-main/running", res.Rows[1])
	}
	if res.Rows[2].Name != "scratch" || res.Rows[2].Status != "grace" {
		t.Fatalf("row 2 = %+v, want scratch/grace", res.Rows[2])
	}
}

func TestListRegisteredOnlySkipsAdHocSessions(t *testing.T) {
	svc, store := newListService(t)
	now := time.Now().UTC()

	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "backend-main",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-main",
		Initialized:   true,
		CreatedAt:     now.Add(-2 * time.Hour),
	})
	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "backend-develop",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-develop",
		Initialized:   false,
		CreatedAt:     now.Add(-90 * time.Minute),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "backend-main",
		Image:        "ubuntu:24.04",
		Status:       state.StatusGracePeriod,
		CreatedAt:    now.Add(-45 * time.Minute),
		LastActivity: now.Add(-5 * time.Minute),
		MaxLifetime:  now.Add(7 * time.Hour),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "scratch",
		Image:        "golang:1.24",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-30 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		MaxLifetime:  now.Add(3 * time.Hour),
	})

	res, err := svc.List(context.Background(), ListRequest{User: "tenant", RegisteredOnly: true})
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
	if res.Rows[0].Name != "backend-develop" || res.Rows[0].Status != "uninitialized" {
		t.Fatalf("row 0 = %+v, want backend-develop/uninitialized", res.Rows[0])
	}
	if res.Rows[1].Name != "backend-main" || res.Rows[1].Status != "grace" {
		t.Fatalf("row 1 = %+v, want backend-main/grace", res.Rows[1])
	}
}

func TestListSurfacesPreserved(t *testing.T) {
	svc, store := newListService(t)
	now := time.Now().UTC()

	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		WorkspacePath: "/tmp/auth-fix",
		Initialized:   false,
		State:         state.WorkspaceStatePreserved,
		CreatedAt:     now.Add(-30 * time.Minute),
	})

	res, err := svc.List(context.Background(), ListRequest{User: "tenant"})
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "auth-fix" || res.Rows[0].Status != "preserved" {
		t.Fatalf("rows = %+v, want single auth-fix/preserved row", res.Rows)
	}
}

func TestListCarriesWorkspaceBranchAcrossSession(t *testing.T) {
	svc, store := newListService(t)
	now := time.Now().UTC()

	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		Branch:        "feat/auth-retry",
		WorkspacePath: "/tmp/auth-fix",
		Initialized:   true,
		CreatedAt:     now.Add(-time.Hour),
	})
	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "scratch",
		Project:       "backend",
		WorkspacePath: "/tmp/scratch",
		Initialized:   false,
		CreatedAt:     now.Add(-30 * time.Minute),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "auth-fix",
		Image:        "ubuntu:24.04",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-15 * time.Minute),
		LastActivity: now,
		MaxLifetime:  now.Add(time.Hour),
	})

	res, err := svc.List(context.Background(), ListRequest{User: "tenant"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("rows = %+v", res.Rows)
	}
	for _, row := range res.Rows {
		switch row.Name {
		case "auth-fix":
			if row.Branch != "feat/auth-retry" {
				t.Fatalf("auth-fix branch = %q, want feat/auth-retry", row.Branch)
			}
		case "scratch":
			if row.Branch != "" {
				t.Fatalf("scratch branch = %q, want empty", row.Branch)
			}
		default:
			t.Fatalf("unexpected row %q", row.Name)
		}
	}
}

func TestListKeepsPreservedOverStaleSession(t *testing.T) {
	svc, store := newListService(t)
	now := time.Now().UTC()

	mustCreateWorkspace(t, store, &state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		WorkspacePath: "/tmp/auth-fix",
		State:         state.WorkspaceStatePreserved,
		CreatedAt:     now.Add(-30 * time.Minute),
	})
	mustCreateSession(t, store, &state.Session{
		User:         "tenant",
		Project:      "auth-fix",
		Image:        "ubuntu:24.04",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-30 * time.Minute),
		LastActivity: now,
		MaxLifetime:  now.Add(time.Hour),
	})

	res, err := svc.List(context.Background(), ListRequest{User: "tenant"})
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Status != "preserved" {
		t.Fatalf("rows = %+v, want single auth-fix/preserved row", res.Rows)
	}
}

func mustCreateWorkspace(t *testing.T, store *state.FakeStore, ws *state.Workspace) {
	t.Helper()
	if err := store.CreateWorkspace(ws); err != nil {
		t.Fatalf("create workspace %q: %v", ws.Name, err)
	}
}

func mustCreateSession(t *testing.T, store *state.FakeStore, sess *state.Session) {
	t.Helper()
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create session %q: %v", sess.Project, err)
	}
}
