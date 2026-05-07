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
