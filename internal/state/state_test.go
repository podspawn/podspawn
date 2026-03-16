package state

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
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

func TestListSessions(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC()

	sessions := []*Session{
		{
			User: "charlie", Project: "web", ContainerID: "c3", ContainerName: "podspawn-charlie-web",
			Image: "ubuntu:24.04", Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		},
		{
			User: "alice", Project: "api", ContainerID: "c1", ContainerName: "podspawn-alice-api",
			Image: "ubuntu:24.04", Status: "running", Connections: 2,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		},
		{
			User: "alice", Project: "frontend", ContainerID: "c2", ContainerName: "podspawn-alice-frontend",
			Image: "node:22", Status: "grace_period", Connections: 0,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		},
	}
	for _, sess := range sessions {
		if err := store.CreateSession(sess); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(got))
	}

	// ListSessions orders by user, project
	if got[0].User != "alice" || got[0].Project != "api" {
		t.Errorf("first session = %s/%s, want alice/api", got[0].User, got[0].Project)
	}
	if got[1].User != "alice" || got[1].Project != "frontend" {
		t.Errorf("second session = %s/%s, want alice/frontend", got[1].User, got[1].Project)
	}
	if got[2].User != "charlie" || got[2].Project != "web" {
		t.Errorf("third session = %s/%s, want charlie/web", got[2].User, got[2].Project)
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

func TestListSessionsByUser(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC()

	sessions := []*Session{
		{
			User: "deploy", Project: "backend", ContainerID: "d1", ContainerName: "podspawn-deploy-backend",
			Image: "ubuntu:24.04", Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		},
		{
			User: "deploy", Project: "api", ContainerID: "d2", ContainerName: "podspawn-deploy-api",
			Image: "ubuntu:24.04", Status: "running", Connections: 3,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		},
		{
			User: "ops", Project: "monitoring", ContainerID: "o1", ContainerName: "podspawn-ops-monitoring",
			Image: "grafana:latest", Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(4 * time.Hour),
		},
	}
	for _, sess := range sessions {
		if err := store.CreateSession(sess); err != nil {
			t.Fatal(err)
		}
	}

	deploySessions, err := store.ListSessionsByUser("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(deploySessions) != 2 {
		t.Fatalf("expected 2 deploy sessions, got %d", len(deploySessions))
	}
	// ListSessionsByUser orders by project
	if deploySessions[0].Project != "api" {
		t.Errorf("first deploy session project = %q, want api", deploySessions[0].Project)
	}
	if deploySessions[1].Project != "backend" {
		t.Errorf("second deploy session project = %q, want backend", deploySessions[1].Project)
	}

	opsSessions, err := store.ListSessionsByUser("ops")
	if err != nil {
		t.Fatal(err)
	}
	if len(opsSessions) != 1 {
		t.Fatalf("expected 1 ops session, got %d", len(opsSessions))
	}
	if opsSessions[0].ContainerID != "o1" {
		t.Errorf("ops container_id = %q, want o1", opsSessions[0].ContainerID)
	}

	// User with no sessions returns empty slice
	nobody, err := store.ListSessionsByUser("nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(nobody) != 0 {
		t.Fatalf("expected 0 sessions for nobody, got %d", len(nobody))
	}
}

func TestExpiredLifetimesWithMix(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC()

	expired1 := &Session{
		User: "deploy", Project: "old-api", ContainerID: "e1", ContainerName: "podspawn-deploy-old",
		Image: "ubuntu:24.04", Status: "running", Connections: 1,
		CreatedAt: now.Add(-10 * time.Hour), LastActivity: now.Add(-2 * time.Hour),
		MaxLifetime: now.Add(-1 * time.Hour),
	}
	expired2 := &Session{
		User: "ops", Project: "stale-job", ContainerID: "e2", ContainerName: "podspawn-ops-stale",
		Image: "ubuntu:24.04", Status: "grace_period", Connections: 0,
		CreatedAt: now.Add(-24 * time.Hour), LastActivity: now.Add(-12 * time.Hour),
		MaxLifetime: now.Add(-30 * time.Minute),
	}
	alive := &Session{
		User: "dev", Project: "fresh", ContainerID: "a1", ContainerName: "podspawn-dev-fresh",
		Image: "ubuntu:24.04", Status: "running", Connections: 2,
		CreatedAt: now, LastActivity: now,
		MaxLifetime: now.Add(6 * time.Hour),
	}

	for _, sess := range []*Session{expired1, expired2, alive} {
		if err := store.CreateSession(sess); err != nil {
			t.Fatal(err)
		}
	}

	expired, err := store.ExpiredLifetimes()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(expired))
	}

	users := map[string]bool{}
	for _, s := range expired {
		users[s.User] = true
	}
	if !users["deploy"] || !users["ops"] {
		t.Errorf("expected deploy and ops in expired set, got %v", users)
	}
}

func TestStaleZeroConnectionsWithGraceExpiry(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC()

	sess := &Session{
		User: "deploy", Project: "", ContainerID: "g1", ContainerName: "podspawn-deploy",
		Image: "ubuntu:24.04", Status: "running", Connections: 0,
		CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Without grace_expiry, should be stale
	stale, err := store.StaleZeroConnections("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if stale == nil {
		t.Fatal("expected stale session before grace period set")
	}

	// Set grace period; now it should NOT be returned as stale
	if err := store.SetGracePeriod("deploy", "", now.Add(60*time.Second)); err != nil {
		t.Fatal(err)
	}

	stale, err = store.StaleZeroConnections("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if stale != nil {
		t.Fatal("session with grace_expiry set should not be returned as stale")
	}

	// Cancel grace period; stale again
	if err := store.CancelGracePeriod("deploy", ""); err != nil {
		t.Fatal(err)
	}

	stale, err = store.StaleZeroConnections("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if stale == nil {
		t.Fatal("expected stale session after grace period cancelled")
	}
}

func TestCreateSessionDuplicate(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	dupe := testSessionData("deploy")
	dupe.ContainerID = "different-id"
	err := store.CreateSession(dupe)
	if err == nil {
		t.Fatal("expected error on duplicate user+project, got nil")
	}
}

func TestMigrateFromVersion1(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")

	// Simulate a v1 database: create schema_version with version=1
	// and a sessions table with the old schema (missing network_id, service_ids).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO schema_version (version) VALUES (1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sessions (
		user TEXT NOT NULL,
		project TEXT NOT NULL DEFAULT '',
		container_id TEXT NOT NULL,
		PRIMARY KEY (user, project)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO sessions (user, project, container_id) VALUES ('deploy', '', 'old-container')`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Reopen with the real Open function, which should migrate to v2
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open after v1 migration failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Old session is dropped during migration (sessions are ephemeral)
	got, err := store.GetSession("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("old v1 session should be dropped during migration")
	}

	// Verify the new schema works by inserting a full session
	sess := testSessionData("deploy")
	sess.NetworkID = "net-migrated"
	sess.ServiceIDs = "svc-pg"
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("insert after migration failed: %v", err)
	}

	got, err = store.GetSession("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.NetworkID != "net-migrated" {
		t.Errorf("network_id = %q, want net-migrated", got.NetworkID)
	}
}

func TestCancelGracePeriodRestoresRunning(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	expiry := time.Now().Add(5 * time.Minute)
	if err := store.SetGracePeriod("deploy", "", expiry); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetSession("deploy", "")
	if got.Status != "grace_period" {
		t.Fatalf("status = %q after SetGracePeriod, want grace_period", got.Status)
	}

	if err := store.CancelGracePeriod("deploy", ""); err != nil {
		t.Fatal(err)
	}

	got, _ = store.GetSession("deploy", "")
	if got.Status != "running" {
		t.Errorf("status = %q after CancelGracePeriod, want running", got.Status)
	}
	if got.GraceExpiry.Valid {
		t.Error("grace_expiry should be NULL after cancel")
	}
}

func TestUpdateConnectionsOnMissingSession(t *testing.T) {
	store := openTestDB(t)

	_, err := store.UpdateConnections("ghost", "", 1)
	if err == nil {
		t.Fatal("expected error updating connections on nonexistent session")
	}
}

func TestDeleteSessionIdempotent(t *testing.T) {
	store := openTestDB(t)

	// Deleting a nonexistent session should not error
	if err := store.DeleteSession("ghost", ""); err != nil {
		t.Fatalf("delete nonexistent session should not error: %v", err)
	}
}

func TestExpiredGracePeriodsExcludesFutureExpiry(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	futureExpiry := time.Now().Add(10 * time.Minute)
	if err := store.SetGracePeriod("deploy", "", futureExpiry); err != nil {
		t.Fatal(err)
	}

	expired, err := store.ExpiredGracePeriods()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 0 {
		t.Fatalf("expected 0 expired grace periods for future expiry, got %d", len(expired))
	}
}

func TestListSessionsEmpty(t *testing.T) {
	store := openTestDB(t)

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions on empty db, got %d", len(sessions))
	}
}

func TestStaleZeroConnectionsWithActiveConnections(t *testing.T) {
	store := openTestDB(t)

	sess := testSessionData("deploy")
	sess.Connections = 3
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	stale, err := store.StaleZeroConnections("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if stale != nil {
		t.Fatal("session with active connections should not be stale")
	}
}

// FakeStore tests: verify the in-memory implementation matches Store behavior.

func TestFakeStoreCreateAndGet(t *testing.T) {
	fs := NewFakeStore()
	sess := testSessionData("deploy")

	if err := fs.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := fs.GetSession("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ContainerID != "abc123" {
		t.Fatalf("expected session with container abc123, got %+v", got)
	}

	// Returns copy, not pointer to internal state
	got.ContainerID = "mutated"
	original, _ := fs.GetSession("deploy", "")
	if original.ContainerID == "mutated" {
		t.Fatal("GetSession should return a copy")
	}
}

func TestFakeStoreDuplicate(t *testing.T) {
	fs := NewFakeStore()
	sess := testSessionData("deploy")
	if err := fs.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	err := fs.CreateSession(testSessionData("deploy"))
	if err == nil {
		t.Fatal("expected error on duplicate")
	}
}

func TestFakeStoreGetMissing(t *testing.T) {
	fs := NewFakeStore()
	got, err := fs.GetSession("nobody", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for missing session")
	}
}

func TestFakeStoreUpdateConnections(t *testing.T) {
	fs := NewFakeStore()
	sess := testSessionData("deploy")
	_ = fs.CreateSession(sess)

	count, err := fs.UpdateConnections("deploy", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("connections = %d, want 3", count)
	}

	// Floor at zero
	count, _ = fs.UpdateConnections("deploy", "", -10)
	if count != 0 {
		t.Fatalf("connections = %d, want 0 (floored)", count)
	}

	// Missing session
	_, err = fs.UpdateConnections("ghost", "", 1)
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestFakeStoreGracePeriod(t *testing.T) {
	fs := NewFakeStore()
	_ = fs.CreateSession(testSessionData("deploy"))

	expiry := time.Now().Add(30 * time.Second)
	if err := fs.SetGracePeriod("deploy", "", expiry); err != nil {
		t.Fatal(err)
	}

	got, _ := fs.GetSession("deploy", "")
	if got.Status != "grace_period" || !got.GraceExpiry.Valid {
		t.Fatal("grace period not set correctly")
	}

	if err := fs.CancelGracePeriod("deploy", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = fs.GetSession("deploy", "")
	if got.Status != "running" || got.GraceExpiry.Valid {
		t.Fatal("grace period not cancelled correctly")
	}

	// Error on missing
	if err := fs.SetGracePeriod("ghost", "", expiry); err == nil {
		t.Fatal("expected error for missing session")
	}
	if err := fs.CancelGracePeriod("ghost", ""); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestFakeStoreDelete(t *testing.T) {
	fs := NewFakeStore()
	_ = fs.CreateSession(testSessionData("deploy"))
	_ = fs.DeleteSession("deploy", "")

	got, _ := fs.GetSession("deploy", "")
	if got != nil {
		t.Fatal("session should be deleted")
	}
}

func TestFakeStoreExpiredGracePeriods(t *testing.T) {
	fs := NewFakeStore()
	now := time.Now().UTC()

	sess := testSessionData("deploy")
	_ = fs.CreateSession(sess)
	_ = fs.SetGracePeriod("deploy", "", now.Add(-5*time.Second))

	active := testSessionData("active")
	active.User = "active"
	_ = fs.CreateSession(active)

	expired, err := fs.ExpiredGracePeriods()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].User != "deploy" {
		t.Fatalf("expected 1 expired (deploy), got %d", len(expired))
	}
}

func TestFakeStoreExpiredLifetimes(t *testing.T) {
	fs := NewFakeStore()
	now := time.Now().UTC()

	old := testSessionData("deploy")
	old.MaxLifetime = now.Add(-1 * time.Hour)
	_ = fs.CreateSession(old)

	fresh := testSessionData("active")
	fresh.User = "active"
	fresh.MaxLifetime = now.Add(6 * time.Hour)
	_ = fs.CreateSession(fresh)

	expired, err := fs.ExpiredLifetimes()
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].User != "deploy" {
		t.Fatalf("expected 1 expired (deploy), got %d", len(expired))
	}
}

func TestFakeStoreStaleZeroConnections(t *testing.T) {
	fs := NewFakeStore()

	sess := testSessionData("deploy")
	sess.Connections = 0
	_ = fs.CreateSession(sess)

	stale, _ := fs.StaleZeroConnections("deploy", "")
	if stale == nil {
		t.Fatal("expected stale session")
	}

	// With grace expiry, not stale
	_ = fs.SetGracePeriod("deploy", "", time.Now().Add(time.Minute))
	stale, _ = fs.StaleZeroConnections("deploy", "")
	if stale != nil {
		t.Fatal("session with grace expiry should not be stale")
	}

	// Missing user
	stale, _ = fs.StaleZeroConnections("ghost", "")
	if stale != nil {
		t.Fatal("expected nil for missing user")
	}
}

func TestFakeStoreListSessions(t *testing.T) {
	fs := NewFakeStore()
	now := time.Now().UTC()

	for _, u := range []struct{ user, project string }{
		{"charlie", "web"},
		{"alice", "frontend"},
		{"alice", "api"},
	} {
		s := &Session{
			User: u.user, Project: u.project, ContainerID: u.user + "-" + u.project,
			ContainerName: "c", Image: "ubuntu:24.04", Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		}
		_ = fs.CreateSession(s)
	}

	all, _ := fs.ListSessions()
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	if all[0].User != "alice" || all[0].Project != "api" {
		t.Errorf("first = %s/%s, want alice/api", all[0].User, all[0].Project)
	}
	if all[2].User != "charlie" {
		t.Errorf("last = %s, want charlie", all[2].User)
	}
}

func TestFakeStoreListSessionsByUser(t *testing.T) {
	fs := NewFakeStore()
	now := time.Now().UTC()

	for _, p := range []string{"zeta", "alpha"} {
		s := &Session{
			User: "deploy", Project: p, ContainerID: p,
			ContainerName: "c", Image: "ubuntu:24.04", Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		}
		_ = fs.CreateSession(s)
	}

	got, _ := fs.ListSessionsByUser("deploy")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Project != "alpha" {
		t.Errorf("first project = %q, want alpha", got[0].Project)
	}

	empty, _ := fs.ListSessionsByUser("nobody")
	if len(empty) != 0 {
		t.Fatalf("expected 0 for nobody, got %d", len(empty))
	}
}

func TestFakeStoreClose(t *testing.T) {
	fs := NewFakeStore()
	if err := fs.Close(); err != nil {
		t.Fatalf("Close should return nil: %v", err)
	}
}
