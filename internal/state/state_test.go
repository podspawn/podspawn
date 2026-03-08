package state

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testSessionData(user string) *Session {
	now := time.Now().UTC()
	return &Session{
		User:          user,
		Project:       "",
		ContainerID:   "abc123",
		ContainerName: "podspawn-" + user,
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	}
}

func TestOpenCreatesTable(t *testing.T) {
	store := openTestDB(t)

	_, err := store.GetSession("nonexistent", "")
	if err != nil {
		t.Fatalf("table should exist: %v", err)
	}
}

func TestCreateAndGetSession(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")

	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.ContainerID != "abc123" {
		t.Errorf("container_id = %q, want abc123", got.ContainerID)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.Connections != 1 {
		t.Errorf("connections = %d, want 1", got.Connections)
	}
}

func TestGetSessionMissing(t *testing.T) {
	store := openTestDB(t)

	got, err := store.GetSession("nobody", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestUpdateConnections(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	count, err := store.UpdateConnections("deploy", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("after +1: connections = %d, want 2", count)
	}

	count, err = store.UpdateConnections("deploy", "", -1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("after -1: connections = %d, want 1", count)
	}

	count, err = store.UpdateConnections("deploy", "", -1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("after -1: connections = %d, want 0", count)
	}
}

func TestUpdateConnectionsFloorAtZero(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")
	sess.Connections = 0
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	count, err := store.UpdateConnections("deploy", "", -1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("double decrement: connections = %d, want 0 (floored)", count)
	}
}

func TestSetAndCancelGracePeriod(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	expiry := time.Now().Add(60 * time.Second)
	if err := store.SetGracePeriod("deploy", "", expiry); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetSession("deploy", "")
	if got.Status != "grace_period" {
		t.Errorf("status = %q, want grace_period", got.Status)
	}
	if !got.GraceExpiry.Valid {
		t.Error("grace_expiry should be set")
	}

	if err := store.CancelGracePeriod("deploy", ""); err != nil {
		t.Fatal(err)
	}

	got, _ = store.GetSession("deploy", "")
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.GraceExpiry.Valid {
		t.Error("grace_expiry should be cleared")
	}
}

func TestDeleteSession(t *testing.T) {
	store := openTestDB(t)
	if err := store.CreateSession(testSessionData("deploy")); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteSession("deploy", ""); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetSession("deploy", "")
	if got != nil {
		t.Fatal("session should be deleted")
	}
}

func TestExpiredGracePeriods(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-10 * time.Second)
	if err := store.SetGracePeriod("deploy", "", past); err != nil {
		t.Fatal(err)
	}

	active := testSessionData("active")
	active.User = "active"
	if err := store.CreateSession(active); err != nil {
		t.Fatal(err)
	}

	expired, err := store.ExpiredGracePeriods()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].User != "deploy" {
		t.Errorf("expired user = %q, want deploy", expired[0].User)
	}
}

func TestExpiredLifetimes(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	sess.MaxLifetime = time.Now().Add(-1 * time.Hour)
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	expired, err := store.ExpiredLifetimes()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
}

func TestStaleZeroConnections(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	sess.Connections = 0
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	stale, err := store.StaleZeroConnections("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if stale == nil {
		t.Fatal("expected stale session")
	}

	other, err := store.StaleZeroConnections("nobody", "")
	if err != nil {
		t.Fatal(err)
	}
	if other != nil {
		t.Fatal("expected nil for nonexistent user")
	}
}

func TestReopenPreservesData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	store1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store1.CreateSession(testSessionData("deploy")); err != nil {
		t.Fatal(err)
	}
	_ = store1.Close()

	store2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store2.Close() }()

	got, err := store2.GetSession("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("session should survive reopen")
	}
}

func TestCompositeKeyIndependentSessions(t *testing.T) {
	store := openTestDB(t)

	now := time.Now().UTC()
	backend := &Session{
		User:          "deploy",
		Project:       "backend",
		ContainerID:   "backend-id",
		ContainerName: "podspawn-deploy-backend",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	}
	frontend := &Session{
		User:          "deploy",
		Project:       "frontend",
		ContainerID:   "frontend-id",
		ContainerName: "podspawn-deploy-frontend",
		Image:         "node:22",
		Status:        "running",
		Connections:   2,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	}

	if err := store.CreateSession(backend); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(frontend); err != nil {
		t.Fatal(err)
	}

	gotB, _ := store.GetSession("deploy", "backend")
	gotF, _ := store.GetSession("deploy", "frontend")

	if gotB.ContainerID != "backend-id" {
		t.Errorf("backend container = %q, want backend-id", gotB.ContainerID)
	}
	if gotF.ContainerID != "frontend-id" {
		t.Errorf("frontend container = %q, want frontend-id", gotF.ContainerID)
	}
	if gotF.Connections != 2 {
		t.Errorf("frontend connections = %d, want 2", gotF.Connections)
	}

	// Delete one, other survives
	if err := store.DeleteSession("deploy", "backend"); err != nil {
		t.Fatal(err)
	}
	gotB, _ = store.GetSession("deploy", "backend")
	gotF, _ = store.GetSession("deploy", "frontend")
	if gotB != nil {
		t.Error("backend session should be deleted")
	}
	if gotF == nil {
		t.Error("frontend session should survive")
	}
}

func TestNetworkAndServiceIDsRoundTrip(t *testing.T) {
	store := openTestDB(t)

	now := time.Now().UTC()
	sess := &Session{
		User:          "deploy",
		Project:       "backend",
		ContainerID:   "dev-id",
		ContainerName: "podspawn-deploy-backend",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
		NetworkID:     "net-abc123",
		ServiceIDs:    "svc-postgres,svc-redis",
	}

	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("deploy", "backend")
	if err != nil {
		t.Fatal(err)
	}
	if got.NetworkID != "net-abc123" {
		t.Errorf("network_id = %q, want net-abc123", got.NetworkID)
	}
	if got.ServiceIDs != "svc-postgres,svc-redis" {
		t.Errorf("service_ids = %q, want svc-postgres,svc-redis", got.ServiceIDs)
	}
}
