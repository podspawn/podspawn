package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func TestRemoveLocalMachineDeletesStoppedWorkspace(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := removeLocalWorkspace(context.Background(), rt, store, nil, "tenant", "auth-fix", false); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}

	got, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("machine should be deleted, got %+v", got)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace should be removed, stat err = %v", err)
	}
}

func TestRemoveLocalMachineRefusesRunningMachineWithoutForce(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
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

	err := removeLocalWorkspace(context.Background(), rt, store, nil, "tenant", "auth-fix", false)
	if err == nil {
		t.Fatal("expected removeLocalWorkspace() to fail without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %q, want --force hint", err)
	}
}

func TestRemoveLocalMachineForceRemovesRunningMachine(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
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

	if err := removeLocalWorkspace(context.Background(), rt, store, nil, "tenant", "auth-fix", true); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}

	gotMachine, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if gotMachine != nil {
		t.Fatalf("machine should be deleted, got %+v", gotMachine)
	}
	gotSession, err := store.GetSession("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession != nil {
		t.Fatalf("session should be deleted, got %+v", gotSession)
	}
	if _, exists := rt.Containers["podspawn-tenant-auth-fix"]; exists {
		t.Fatal("running container should be removed")
	}
}

func TestRemoveLocalMachineRejectsEphemeralNames(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()

	err := removeLocalWorkspace(context.Background(), rt, store, nil, "tenant", ".tmp-auth-fix-123", false)
	if err == nil {
		t.Fatal("expected removeLocalWorkspace() to reject ephemeral names")
	}
	if !strings.Contains(err.Error(), ".tmp-") {
		t.Fatalf("error = %q, want .tmp- hint", err)
	}
}

func TestRemoveLocalMachineLogsAuditEvent(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	root := t.TempDir()
	workspacePath := filepath.Join(root, "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(root, "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := removeLocalWorkspace(context.Background(), rt, store, logger, "tenant", "auth-fix", false); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != "machine.delete" {
		t.Fatalf("event = %q, want machine.delete", entry["event"])
	}
	if entry["project"] != "backend" {
		t.Fatalf("project = %q, want backend", entry["project"])
	}
	if entry["branch"] != "main" {
		t.Fatalf("branch = %q, want main", entry["branch"])
	}
}
