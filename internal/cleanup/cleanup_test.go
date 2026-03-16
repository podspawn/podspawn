package cleanup

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func TestDestroySession(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	ctx := context.Background()

	rt.Containers["podspawn-dev-api"] = true
	rt.Containers["svc-postgres-1"] = true
	_ = store.CreateSession(&state.Session{
		User:          "dev",
		Project:       "api",
		ContainerID:   "podspawn-dev-api",
		ContainerName: "podspawn-dev-api",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MaxLifetime:   time.Now().Add(8 * time.Hour),
		NetworkID:     "net-123",
		ServiceIDs:    "svc-postgres-1",
	})

	sess, _ := store.GetSession("dev", "api")
	if err := DestroySession(ctx, rt, store, sess); err != nil {
		t.Fatalf("DestroySession: %v", err)
	}

	if _, ok := rt.Containers["podspawn-dev-api"]; ok {
		t.Error("main container not removed")
	}
	if _, ok := rt.Containers["svc-postgres-1"]; ok {
		t.Error("service container not removed")
	}
	got, _ := store.GetSession("dev", "api")
	if got != nil {
		t.Error("session not deleted from store")
	}
}

func TestExpireGracePeriods(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	ctx := context.Background()

	rt.Containers["podspawn-dev-web"] = true
	_ = store.CreateSession(&state.Session{
		User:          "dev",
		Project:       "web",
		ContainerID:   "podspawn-dev-web",
		ContainerName: "podspawn-dev-web",
		Image:         "ubuntu:24.04",
		Status:        "grace_period",
		Connections:   0,
		GraceExpiry:   sql.NullTime{Time: time.Now().Add(-10 * time.Second), Valid: true},
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		LastActivity:  time.Now().Add(-5 * time.Minute),
		MaxLifetime:   time.Now().Add(7 * time.Hour),
	})

	count := ExpireGracePeriods(ctx, rt, store)
	if count != 1 {
		t.Fatalf("expected 1 expired, got %d", count)
	}
	if _, ok := rt.Containers["podspawn-dev-web"]; ok {
		t.Error("container not removed after grace expiry")
	}
}

func TestEnforceMaxLifetimes(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	ctx := context.Background()

	rt.Containers["podspawn-ci-build"] = true
	_ = store.CreateSession(&state.Session{
		User:          "ci",
		Project:       "build",
		ContainerID:   "podspawn-ci-build",
		ContainerName: "podspawn-ci-build",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     time.Now().Add(-9 * time.Hour),
		LastActivity:  time.Now().Add(-1 * time.Hour),
		MaxLifetime:   time.Now().Add(-30 * time.Minute), // expired 30 min ago
	})

	count := EnforceMaxLifetimes(ctx, rt, store)
	if count != 1 {
		t.Fatalf("expected 1 expired lifetime, got %d", count)
	}
}

func TestReconcileOrphans(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	ctx := context.Background()

	// Tracked container
	rt.Containers["podspawn-dev-api"] = true
	_ = store.CreateSession(&state.Session{
		User:          "dev",
		Project:       "api",
		ContainerID:   "podspawn-dev-api",
		ContainerName: "podspawn-dev-api",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MaxLifetime:   time.Now().Add(8 * time.Hour),
	})

	// Orphan: in Docker but not in DB
	rt.Containers["podspawn-old-stale"] = true

	count := ReconcileOrphans(ctx, rt, store)
	if count != 1 {
		t.Fatalf("expected 1 orphan removed, got %d", count)
	}
	if _, ok := rt.Containers["podspawn-old-stale"]; ok {
		t.Error("orphan container not removed")
	}
	if _, ok := rt.Containers["podspawn-dev-api"]; !ok {
		t.Error("tracked container should not be removed")
	}
}

func TestRunOnce(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	ctx := context.Background()

	// Mix of expired grace, expired lifetime, and orphan
	rt.Containers["podspawn-grace"] = true
	_ = store.CreateSession(&state.Session{
		User: "grace-user", Project: "", ContainerID: "podspawn-grace", ContainerName: "podspawn-grace",
		Image: "ubuntu:24.04", Status: "grace_period", Connections: 0,
		GraceExpiry: sql.NullTime{Time: time.Now().Add(-1 * time.Minute), Valid: true},
		CreatedAt:   time.Now().Add(-2 * time.Hour), LastActivity: time.Now().Add(-10 * time.Minute),
		MaxLifetime: time.Now().Add(6 * time.Hour),
	})

	rt.Containers["podspawn-orphan"] = true

	RunOnce(ctx, rt, store)

	if _, ok := rt.Containers["podspawn-grace"]; ok {
		t.Error("grace-period container not cleaned up")
	}
	if _, ok := rt.Containers["podspawn-orphan"]; ok {
		t.Error("orphan container not cleaned up")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
