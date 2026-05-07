package state

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// seedV5DB creates a SQLite database at version 5 with the schema the v4→v5
// migration leaves behind. Returns the dbPath so the caller can reopen it
// through state.Open and trigger the v5→v6 migration.
func seedV5DB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "v5.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening seed db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (5)`,
		`CREATE TABLE sessions (
			user           TEXT NOT NULL,
			project        TEXT NOT NULL DEFAULT '',
			container_id   TEXT NOT NULL,
			container_name TEXT NOT NULL,
			image          TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'running',
			connections    INTEGER NOT NULL DEFAULT 1,
			grace_expiry   DATETIME,
			created_at     DATETIME NOT NULL,
			last_activity  DATETIME NOT NULL,
			max_lifetime   DATETIME NOT NULL,
			network_id     TEXT NOT NULL DEFAULT '',
			service_ids    TEXT NOT NULL DEFAULT '',
			mode           TEXT NOT NULL DEFAULT 'grace-period',
			PRIMARY KEY (user, project)
		)`,
		`CREATE TABLE machines (
			user             TEXT NOT NULL,
			name             TEXT NOT NULL,
			project          TEXT NOT NULL,
			repo_url         TEXT NOT NULL,
			branch           TEXT NOT NULL DEFAULT '',
			mode             TEXT NOT NULL DEFAULT 'grace-period',
			workspace_path   TEXT NOT NULL,
			workspace_target TEXT NOT NULL,
			created_at       DATETIME NOT NULL,
			initialized      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user, name)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed stmt failed: %v\nstmt: %s", err, s)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("closing seed db: %v", err)
	}
	return dbPath
}

func insertV5Machine(t *testing.T, db *sql.DB, m *Workspace) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO machines (user, name, project, repo_url, branch, mode, workspace_path, workspace_target, created_at, initialized)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.User, m.Name, m.Project, m.RepoURL, m.Branch, m.Mode,
		m.WorkspacePath, m.WorkspaceTarget, m.CreatedAt.UTC(), boolToInt(m.Initialized),
	)
	if err != nil {
		t.Fatalf("insert machine: %v", err)
	}
}

func insertV5Session(t *testing.T, db *sql.DB, s *Session) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO sessions (user, project, container_id, container_name, image, status, connections, created_at, last_activity, max_lifetime, network_id, service_ids, mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.User, s.Project, s.ContainerID, s.ContainerName, s.Image, s.Status, s.Connections,
		s.CreatedAt.UTC(), s.LastActivity.UTC(), s.MaxLifetime.UTC(), s.NetworkID, s.ServiceIDs, s.Mode,
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestMigrateFromVersion5RenamesMachinesTable(t *testing.T) {
	dbPath := seedV5DB(t)
	db, _ := sql.Open("sqlite", dbPath)
	insertV5Machine(t, db, &Workspace{
		User: "deploy", Name: "auth-fix", Project: "backend",
		RepoURL: "git@example.com:acme/backend.git", Branch: "feat/auth-retry",
		Mode: "persistent", WorkspacePath: "/ws/auth-fix", WorkspaceTarget: "/workspace/backend",
		CreatedAt: time.Now().UTC(), Initialized: true,
	})
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hasWorkspaces, err := tableExists(store.db, "workspaces")
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkspaces {
		t.Fatal("workspaces table should exist after migration")
	}

	hasMachines, err := tableExists(store.db, "machines")
	if err != nil {
		t.Fatal(err)
	}
	if hasMachines {
		t.Fatal("machines table should not exist after rename")
	}

	got, err := store.GetWorkspace("deploy", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected workspace row to survive rename")
	}
	if got.Branch != "feat/auth-retry" || got.Mode != "persistent" || !got.Initialized {
		t.Fatalf("workspace row corrupted by migration: %+v", got)
	}
}

func TestMigrateFromVersion5GeneratesUniqueIDs(t *testing.T) {
	dbPath := seedV5DB(t)
	db, _ := sql.Open("sqlite", dbPath)
	now := time.Now().UTC()

	for _, m := range []*Workspace{
		{User: "deploy", Name: "alpha", Project: "backend", RepoURL: "r", Mode: "grace-period",
			WorkspacePath: "/ws/alpha", WorkspaceTarget: "/workspace/backend", CreatedAt: now},
		{User: "deploy", Name: "beta", Project: "backend", RepoURL: "r", Mode: "grace-period",
			WorkspacePath: "/ws/beta", WorkspaceTarget: "/workspace/backend", CreatedAt: now},
		{User: "ops", Name: "gamma", Project: "monitoring", RepoURL: "r2", Mode: "persistent",
			WorkspacePath: "/ws/gamma", WorkspaceTarget: "/workspace/monitoring", CreatedAt: now},
	} {
		insertV5Machine(t, db, m)
	}

	for _, s := range []*Session{
		{User: "deploy", Project: "alpha", ContainerID: "c1", ContainerName: "n1", Image: "i",
			Status: "running", Connections: 1, CreatedAt: now, LastActivity: now,
			MaxLifetime: now.Add(time.Hour), Mode: "grace-period"},
		{User: "ops", Project: "gamma", ContainerID: "c2", ContainerName: "n2", Image: "i",
			Status: "running", Connections: 1, CreatedAt: now, LastActivity: now,
			MaxLifetime: now.Add(time.Hour), Mode: "persistent"},
	} {
		insertV5Session(t, db, s)
	}
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, table := range []string{"workspaces", "sessions"} {
		ids := collectColumn(t, store.db, table, "id")
		if len(ids) == 0 {
			t.Fatalf("%s: no rows returned", table)
		}
		seen := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if id == "" {
				t.Fatalf("%s: row has empty id after migration", table)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("%s: duplicate id %q after migration", table, id)
			}
			seen[id] = struct{}{}
		}
	}

	for _, idx := range []string{"idx_workspaces_id", "idx_sessions_id"} {
		var name string
		err := store.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&name)
		if err != nil {
			t.Fatalf("expected unique index %q: %v", idx, err)
		}
	}
}

func collectColumn(t *testing.T, db *sql.DB, table, column string) []string {
	t.Helper()
	rows, err := db.Query("SELECT " + column + " FROM " + table) //nolint:gosec // table/column come from test fixtures
	if err != nil {
		t.Fatalf("query %s.%s: %v", table, column, err)
	}
	defer rows.Close() //nolint:errcheck
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if s.Valid {
			out = append(out, s.String)
		} else {
			out = append(out, "")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}
	return out
}

func TestMigrateFromVersion5BackfillsWorkspaceIDsViaNameJoin(t *testing.T) {
	dbPath := seedV5DB(t)
	db, _ := sql.Open("sqlite", dbPath)
	now := time.Now().UTC()

	insertV5Machine(t, db, &Workspace{
		User: "deploy", Name: "auth-fix", Project: "backend", RepoURL: "r", Mode: "grace-period",
		WorkspacePath: "/ws/auth-fix", WorkspaceTarget: "/workspace/backend", CreatedAt: now,
	})
	insertV5Session(t, db, &Session{
		User: "deploy", Project: "auth-fix", ContainerID: "c1", ContainerName: "n1", Image: "i",
		Status: "running", Connections: 1, CreatedAt: now, LastActivity: now,
		MaxLifetime: now.Add(time.Hour), Mode: "grace-period",
	})
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ws, err := store.GetWorkspace("deploy", "auth-fix")
	if err != nil || ws == nil {
		t.Fatalf("get workspace: %v", err)
	}

	sess, err := store.GetSession("deploy", "auth-fix")
	if err != nil || sess == nil {
		t.Fatalf("get session: %v", err)
	}

	if !sess.WorkspaceID.Valid {
		t.Fatal("session.workspace_id should be NOT NULL after backfill")
	}
	if sess.WorkspaceID.String != ws.ID {
		t.Fatalf("workspace_id mismatch: session=%q workspace=%q", sess.WorkspaceID.String, ws.ID)
	}
}

func TestMigrateFromVersion5LeavesUnmatchedSessionsNull(t *testing.T) {
	dbPath := seedV5DB(t)
	db, _ := sql.Open("sqlite", dbPath)
	now := time.Now().UTC()

	// Session with no matching machine row (server-mode shape).
	insertV5Session(t, db, &Session{
		User: "alice", Project: "backend", ContainerID: "c1", ContainerName: "n1", Image: "i",
		Status: "running", Connections: 1, CreatedAt: now, LastActivity: now,
		MaxLifetime: now.Add(time.Hour), Mode: "grace-period",
	})
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sess, err := store.GetSession("alice", "backend")
	if err != nil || sess == nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.WorkspaceID.Valid {
		t.Fatalf("workspace_id should be NULL when no matching workspace exists, got %q", sess.WorkspaceID.String)
	}
}

func TestMigrateFromVersion5IsRestartableAfterPartialRun(t *testing.T) {
	dbPath := seedV5DB(t)

	// Pretend Transaction A committed (table renamed) but B did not run.
	db, _ := sql.Open("sqlite", dbPath)
	now := time.Now().UTC()
	insertV5Machine(t, db, &Workspace{
		User: "deploy", Name: "alpha", Project: "backend", RepoURL: "r", Mode: "grace-period",
		WorkspacePath: "/ws/alpha", WorkspaceTarget: "/workspace/backend", CreatedAt: now,
	})
	if _, err := db.Exec(`ALTER TABLE machines RENAME TO workspaces`); err != nil {
		t.Fatalf("simulating partial migration: %v", err)
	}
	_ = db.Close()

	// Reopen — migrator should resume at Transaction B.
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open after partial migration: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	got, err := store.GetWorkspace("deploy", "alpha")
	if err != nil || got == nil {
		t.Fatalf("workspace lost during resumed migration: %v", err)
	}
	if got.ID == "" {
		t.Fatal("workspace.id should be populated after resumed migration")
	}

	hasMachines, _ := tableExists(store.db, "machines")
	if hasMachines {
		t.Fatal("machines table should not reappear during resume")
	}
}

func TestCreateWorkspaceOverwritesCallerID(t *testing.T) {
	store := openTestDB(t)
	w := &Workspace{
		ID:              "caller-supplied-should-be-ignored",
		User:            "deploy",
		Name:            "auth-fix",
		Project:         "backend",
		RepoURL:         "r",
		Mode:            "grace-period",
		WorkspacePath:   "/ws/auth-fix",
		WorkspaceTarget: "/workspace/backend",
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.CreateWorkspace(w); err != nil {
		t.Fatal(err)
	}
	if w.ID == "caller-supplied-should-be-ignored" || w.ID == "" {
		t.Fatalf("ID should have been overwritten with a fresh uuid, got %q", w.ID)
	}

	got, err := store.GetWorkspace("deploy", "auth-fix")
	if err != nil || got == nil {
		t.Fatalf("get workspace: %v", err)
	}
	if got.ID != w.ID {
		t.Fatalf("stored id %q != returned id %q", got.ID, w.ID)
	}
}

func TestCreateSessionOverwritesCallerID(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")
	sess.ID = "caller-supplied-should-be-ignored"

	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if sess.ID == "caller-supplied-should-be-ignored" || sess.ID == "" {
		t.Fatalf("ID should have been overwritten, got %q", sess.ID)
	}

	got, err := store.GetSession("deploy", "")
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatalf("stored id %q != in-memory id %q", got.ID, sess.ID)
	}
}

func TestCreateSessionWithoutWorkspacePersistsNullWorkspaceID(t *testing.T) {
	store := openTestDB(t)
	sess := testSessionData("deploy")
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("deploy", "")
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorkspaceID.Valid {
		t.Fatalf("workspace_id should be NULL for default-image session, got %q", got.WorkspaceID.String)
	}
}

func TestCreateSessionWithWorkspacePersistsLinkedWorkspaceID(t *testing.T) {
	store := openTestDB(t)
	w := testMachineData("deploy", "auth-fix")
	if err := store.CreateWorkspace(w); err != nil {
		t.Fatal(err)
	}

	sess := testSessionData("deploy")
	sess.Project = "auth-fix"
	sess.WorkspaceID = sql.NullString{String: w.ID, Valid: true}
	if err := store.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetSession("deploy", "auth-fix")
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if !got.WorkspaceID.Valid {
		t.Fatal("workspace_id should be set when caller links a workspace")
	}
	if got.WorkspaceID.String != w.ID {
		t.Fatalf("workspace_id = %q, want %q", got.WorkspaceID.String, w.ID)
	}
}

// seedV6DB seeds a v6 schema (post-rename, post-IDs) so the v6→v7 migration
// has a realistic starting point.
func seedV6DB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "v6.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening seed db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (6)`,
		`CREATE TABLE sessions (
			user           TEXT NOT NULL,
			project        TEXT NOT NULL DEFAULT '',
			id             TEXT,
			workspace_id   TEXT,
			container_id   TEXT NOT NULL,
			container_name TEXT NOT NULL,
			image          TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'running',
			connections    INTEGER NOT NULL DEFAULT 1,
			grace_expiry   DATETIME,
			created_at     DATETIME NOT NULL,
			last_activity  DATETIME NOT NULL,
			max_lifetime   DATETIME NOT NULL,
			network_id     TEXT NOT NULL DEFAULT '',
			service_ids    TEXT NOT NULL DEFAULT '',
			mode           TEXT NOT NULL DEFAULT 'grace-period',
			PRIMARY KEY (user, project)
		)`,
		`CREATE UNIQUE INDEX idx_sessions_id ON sessions(id) WHERE id IS NOT NULL`,
		`CREATE TABLE workspaces (
			user             TEXT NOT NULL,
			name             TEXT NOT NULL,
			id               TEXT,
			project          TEXT NOT NULL,
			repo_url         TEXT NOT NULL,
			branch           TEXT NOT NULL DEFAULT '',
			mode             TEXT NOT NULL DEFAULT 'grace-period',
			workspace_path   TEXT NOT NULL,
			workspace_target TEXT NOT NULL,
			created_at       DATETIME NOT NULL,
			initialized      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user, name)
		)`,
		`CREATE UNIQUE INDEX idx_workspaces_id ON workspaces(id) WHERE id IS NOT NULL`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed stmt failed: %v\nstmt: %s", err, s)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("closing seed db: %v", err)
	}
	return dbPath
}

func TestMigrateFromVersion6AddsStateColumnWithActiveDefault(t *testing.T) {
	dbPath := seedV6DB(t)

	db, _ := sql.Open("sqlite", dbPath)
	now := time.Now().UTC()
	wsID := "01900000-0000-7000-8000-000000000001"
	if _, err := db.Exec(
		`INSERT INTO workspaces (user, name, id, project, repo_url, branch, mode, workspace_path, workspace_target, created_at, initialized)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"deploy", "alpha", wsID, "backend", "git@example.com:acme/backend.git",
		"main", "grace-period", "/ws/alpha", "/workspace/backend", now, 0,
	); err != nil {
		t.Fatalf("insert pre-migration row: %v", err)
	}
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hasState, err := columnExists(store.db, "workspaces", "state")
	if err != nil {
		t.Fatal(err)
	}
	if !hasState {
		t.Fatal("workspaces.state should exist after v6→v7 migration")
	}

	got, err := store.GetWorkspace("deploy", "alpha")
	if err != nil || got == nil {
		t.Fatalf("get workspace: %v", err)
	}
	if got.State != WorkspaceStateActive {
		t.Fatalf("expected pre-existing row to default to %q, got %q", WorkspaceStateActive, got.State)
	}

	var version int
	if err := store.db.QueryRow(`SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 7 {
		t.Fatalf("schema version = %d, want 7", version)
	}

	var rowCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 1 {
		t.Fatalf("schema_version row count = %d, want 1", rowCount)
	}
}

func TestMigrateFromVersion6IsIdempotentAfterPartialRun(t *testing.T) {
	dbPath := seedV6DB(t)

	db, _ := sql.Open("sqlite", dbPath)
	if _, err := db.Exec(`ALTER TABLE workspaces ADD COLUMN state TEXT NOT NULL DEFAULT 'active'`); err != nil {
		t.Fatalf("simulating partial migration: %v", err)
	}
	_ = db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open after partial migration: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	hasState, err := columnExists(store.db, "workspaces", "state")
	if err != nil {
		t.Fatal(err)
	}
	if !hasState {
		t.Fatal("state column should still exist after resumed migration")
	}
}

func TestCreateWorkspaceDefaultsStateToActive(t *testing.T) {
	store := openTestDB(t)
	w := testMachineData("deploy", "demo")
	w.State = ""
	if err := store.CreateWorkspace(w); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if w.State != WorkspaceStateActive {
		t.Fatalf("CreateWorkspace should default blank State to %q, got %q", WorkspaceStateActive, w.State)
	}

	got, err := store.GetWorkspace("deploy", "demo")
	if err != nil || got == nil {
		t.Fatalf("get workspace: %v", err)
	}
	if got.State != WorkspaceStateActive {
		t.Fatalf("stored state = %q, want %q", got.State, WorkspaceStateActive)
	}
}

func TestCreateWorkspaceRoundTripsPreservedState(t *testing.T) {
	store := openTestDB(t)
	w := testMachineData("deploy", "preserved-demo")
	w.State = WorkspaceStatePreserved
	if err := store.CreateWorkspace(w); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	got, err := store.GetWorkspace("deploy", "preserved-demo")
	if err != nil || got == nil {
		t.Fatalf("get workspace: %v", err)
	}
	if got.State != WorkspaceStatePreserved {
		t.Fatalf("stored state = %q, want %q", got.State, WorkspaceStatePreserved)
	}
}

func TestCreateWorkspaceRejectsUnknownState(t *testing.T) {
	store := openTestDB(t)
	w := testMachineData("deploy", "bogus")
	w.State = "garbage"
	if err := store.CreateWorkspace(w); err == nil {
		t.Fatal("CreateWorkspace should reject unknown state values")
	}
}

func TestUpdateWorkspaceStateRoundTrip(t *testing.T) {
	store := openTestDB(t)
	w := testMachineData("deploy", "flip")
	if err := store.CreateWorkspace(w); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateWorkspaceState("deploy", "flip", WorkspaceStatePreserved); err != nil {
		t.Fatalf("flip to preserved: %v", err)
	}
	got, err := store.GetWorkspace("deploy", "flip")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.State != WorkspaceStatePreserved {
		t.Fatalf("after flip got %q, want %q", got.State, WorkspaceStatePreserved)
	}

	if err := store.UpdateWorkspaceState("deploy", "flip", WorkspaceStateActive); err != nil {
		t.Fatalf("flip back to active: %v", err)
	}
	got, _ = store.GetWorkspace("deploy", "flip")
	if got.State != WorkspaceStateActive {
		t.Fatalf("after flip back got %q, want %q", got.State, WorkspaceStateActive)
	}

	if err := store.UpdateWorkspaceState("deploy", "flip", "nope"); err == nil {
		t.Fatal("UpdateWorkspaceState should reject unknown state values")
	}
}
