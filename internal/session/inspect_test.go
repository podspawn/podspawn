package session

import (
	"context"
	"errors"
	"testing"

	"github.com/podspawn/podspawn/internal/state"
)

func TestInspectFindsWorkspace(t *testing.T) {
	svc, store := newListService(t)
	if err := store.CreateWorkspace(&state.Workspace{
		User:    "tenant",
		Name:    "backend-main",
		Project: "backend",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Inspect(context.Background(), Ref{User: "tenant", Name: "backend-main"})
	if err != nil {
		t.Fatalf("Inspect error = %v", err)
	}
	if res.Workspace == nil || res.Workspace.Name != "backend-main" {
		t.Fatalf("workspace = %+v, want backend-main", res.Workspace)
	}
	if res.Session != nil {
		t.Fatalf("session = %+v, want nil", res.Session)
	}
}

func TestInspectFindsLiveSessionWithoutWorkspace(t *testing.T) {
	svc, store := newListService(t)
	if err := store.CreateSession(&state.Session{
		User:    "tenant",
		Project: "backend-main",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Inspect(context.Background(), Ref{User: "tenant", Name: "backend-main"})
	if err != nil {
		t.Fatalf("Inspect error = %v", err)
	}
	if res.Workspace != nil {
		t.Fatalf("workspace = %+v, want nil", res.Workspace)
	}
	if res.Session == nil || res.Session.Project != "backend-main" {
		t.Fatalf("session = %+v, want backend-main", res.Session)
	}
}

func TestInspectReturnsErrWorkspaceNotFoundWhenBothAbsent(t *testing.T) {
	svc, _ := newListService(t)
	_, err := svc.Inspect(context.Background(), Ref{User: "tenant", Name: "backend-main"})
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("Inspect error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestInspectRejectsMissingUser(t *testing.T) {
	svc, _ := newListService(t)
	_, err := svc.Inspect(context.Background(), Ref{Name: "anything"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Inspect error = %v, want ErrInvalidRequest", err)
	}
}
