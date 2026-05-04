package cmd

import (
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/state"
)

func TestCollectLocalMachineRowsIncludesStoppedMachines(t *testing.T) {
	store := state.NewFakeStore()
	now := time.Now().UTC()

	if err := store.CreateMachine(&state.Machine{
		User:          "tenant",
		Name:          "backend-main",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-main",
		Initialized:   true,
		CreatedAt:     now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create machine: %v", err)
	}
	if err := store.CreateMachine(&state.Machine{
		User:          "tenant",
		Name:          "backend-develop",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-develop",
		Initialized:   false,
		CreatedAt:     now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("create machine: %v", err)
	}
	if err := store.CreateSession(&state.Session{
		User:         "tenant",
		Project:      "backend-main",
		Image:        "ubuntu:24.04",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-45 * time.Minute),
		LastActivity: now.Add(-5 * time.Minute),
		MaxLifetime:  now.Add(7 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.CreateSession(&state.Session{
		User:         "tenant",
		Project:      "scratch",
		Image:        "golang:1.24",
		Status:       state.StatusGracePeriod,
		CreatedAt:    now.Add(-30 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		MaxLifetime:  now.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rows, err := collectLocalMachineRows(store, "tenant", false)
	if err != nil {
		t.Fatalf("collectLocalMachineRows() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if rows[0].Name != "backend-develop" || rows[0].Status != "uninitialized" {
		t.Fatalf("row 0 = %+v, want backend-develop/uninitialized", rows[0])
	}
	if rows[1].Name != "backend-main" || rows[1].Status != "running" {
		t.Fatalf("row 1 = %+v, want backend-main/running", rows[1])
	}
	if rows[2].Name != "scratch" || rows[2].Status != "grace" {
		t.Fatalf("row 2 = %+v, want scratch/grace", rows[2])
	}
}

func TestCollectRegisteredMachineRowsSkipsAdHocSessions(t *testing.T) {
	store := state.NewFakeStore()
	now := time.Now().UTC()

	if err := store.CreateMachine(&state.Machine{
		User:          "tenant",
		Name:          "backend-main",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-main",
		Initialized:   true,
		CreatedAt:     now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("create machine: %v", err)
	}
	if err := store.CreateMachine(&state.Machine{
		User:          "tenant",
		Name:          "backend-develop",
		Project:       "backend",
		WorkspacePath: "/tmp/backend-develop",
		Initialized:   false,
		CreatedAt:     now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("create machine: %v", err)
	}
	if err := store.CreateSession(&state.Session{
		User:         "tenant",
		Project:      "backend-main",
		Image:        "ubuntu:24.04",
		Status:       state.StatusGracePeriod,
		CreatedAt:    now.Add(-45 * time.Minute),
		LastActivity: now.Add(-5 * time.Minute),
		MaxLifetime:  now.Add(7 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.CreateSession(&state.Session{
		User:         "tenant",
		Project:      "scratch",
		Image:        "golang:1.24",
		Status:       state.StatusRunning,
		CreatedAt:    now.Add(-30 * time.Minute),
		LastActivity: now.Add(-2 * time.Minute),
		MaxLifetime:  now.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rows, err := collectRegisteredMachineRows(store, "tenant")
	if err != nil {
		t.Fatalf("collectRegisteredMachineRows() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].Name != "backend-develop" || rows[0].Status != "uninitialized" {
		t.Fatalf("row 0 = %+v, want backend-develop/uninitialized", rows[0])
	}
	if rows[1].Name != "backend-main" || rows[1].Status != "grace" {
		t.Fatalf("row 1 = %+v, want backend-main/grace", rows[1])
	}
}
