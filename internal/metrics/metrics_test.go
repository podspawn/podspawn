package metrics

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func TestCollect(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	ctx := context.Background()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User: "dev-1", Project: "api", ContainerID: "c1", ContainerName: "podspawn-dev-1-api",
		Image: "ubuntu:24.04", Status: "running", Connections: 2,
		CreatedAt: now.Add(-2 * time.Hour), LastActivity: now, MaxLifetime: now.Add(6 * time.Hour),
	})
	_ = store.CreateSession(&state.Session{
		User: "dev-2", Project: "", ContainerID: "c2", ContainerName: "podspawn-dev-2",
		Image: "ubuntu:24.04", Status: "grace_period", Connections: 0,
		CreatedAt: now.Add(-30 * time.Minute), LastActivity: now.Add(-5 * time.Minute), MaxLifetime: now.Add(7 * time.Hour),
	})

	rt.Containers["podspawn-dev-1-api"] = true
	rt.Containers["podspawn-dev-2"] = true
	rt.Containers["podspawn-orphan"] = true

	snap, err := Collect(ctx, store, rt)
	if err != nil {
		t.Fatal(err)
	}

	if snap.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", snap.TotalSessions)
	}
	if snap.RunningSessions != 1 {
		t.Errorf("RunningSessions = %d, want 1", snap.RunningSessions)
	}
	if snap.GraceSessions != 1 {
		t.Errorf("GraceSessions = %d, want 1", snap.GraceSessions)
	}
	if snap.TotalConnections != 2 {
		t.Errorf("TotalConnections = %d, want 2", snap.TotalConnections)
	}
	if snap.DockerContainers != 3 {
		t.Errorf("DockerContainers = %d, want 3", snap.DockerContainers)
	}
	if snap.OldestSession < time.Hour {
		t.Errorf("OldestSession = %v, want >= 1h", snap.OldestSession)
	}
}

func TestWritePrometheus(t *testing.T) {
	snap := &Snapshot{
		TotalSessions:    5,
		RunningSessions:  3,
		GraceSessions:    2,
		TotalConnections: 7,
		DockerContainers: 8,
		OldestSession:    3 * time.Hour,
	}

	var buf bytes.Buffer
	WritePrometheus(&buf, snap)
	out := buf.String()

	expected := []string{
		"podspawn_sessions_total 5",
		"podspawn_sessions_running 3",
		"podspawn_sessions_grace 2",
		"podspawn_connections_total 7",
		"podspawn_containers_docker 8",
		"podspawn_oldest_session_seconds 10800",
	}
	for _, want := range expected {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in prometheus output", want)
		}
	}
}

func TestWriteHuman(t *testing.T) {
	snap := &Snapshot{
		TotalSessions:    2,
		RunningSessions:  1,
		GraceSessions:    1,
		TotalConnections: 3,
		DockerContainers: 4,
		OldestSession:    90 * time.Minute,
	}

	var buf bytes.Buffer
	WriteHuman(&buf, snap)
	out := buf.String()

	if !strings.Contains(out, "2 total") {
		t.Errorf("missing session count: %s", out)
	}
	if !strings.Contains(out, "3 active") {
		t.Errorf("missing connection count: %s", out)
	}
}
