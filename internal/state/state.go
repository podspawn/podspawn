package state

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const (
	StatusRunning     = "running"
	StatusGracePeriod = "grace_period"
)

type Session struct {
	// ID is storage-generated. Callers must not set this; CreateSession
	// overwrites any value present before INSERT.
	ID string
	// WorkspaceID points at the workspaces.id of the linked workspace, or is
	// invalid (Valid=false) when the session has no workspace (default-image
	// flows in server mode). Empty string is not a valid sentinel: NULL is.
	WorkspaceID   sql.NullString
	User          string
	Project       string
	ContainerID   string
	ContainerName string
	Image         string
	Status        string
	Connections   int
	GraceExpiry   sql.NullTime
	CreatedAt     time.Time
	LastActivity  time.Time
	MaxLifetime   time.Time
	NetworkID     string
	ServiceIDs    string
	Mode          string
}

type Workspace struct {
	// ID is storage-generated. Callers must not set this; CreateWorkspace
	// overwrites any value present before INSERT.
	ID              string
	User            string
	Name            string
	Project         string
	RepoURL         string
	Branch          string
	Mode            string
	WorkspacePath   string
	WorkspaceTarget string
	CreatedAt       time.Time
	Initialized     bool
}

type SessionStore interface {
	CreateSession(sess *Session) error
	GetSession(user, project string) (*Session, error)
	UpdateConnections(user, project string, delta int) (int, error)
	SetGracePeriod(user, project string, expiry time.Time) error
	CancelGracePeriod(user, project string) error
	DeleteSession(user, project string) error
	ExpiredGracePeriods() ([]*Session, error)
	ExpiredLifetimes() ([]*Session, error)
	StaleZeroConnections(user, project string) (*Session, error)
	ListSessions() ([]*Session, error)
	ListSessionsByUser(user string) ([]*Session, error)
	Close() error
}

type WorkspaceStore interface {
	CreateWorkspace(workspace *Workspace) error
	GetWorkspace(user, name string) (*Workspace, error)
	UpdateWorkspaceBranch(user, name, branch string) error
	UpdateWorkspaceInitialized(user, name string, initialized bool) error
	DeleteWorkspace(user, name string) error
	ListWorkspacesByUser(user string) ([]*Workspace, error)
}

type Store struct {
	db *sql.DB
}

var _ SessionStore = (*Store)(nil)
var _ WorkspaceStore = (*Store)(nil)

const schemaVersion = 6

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening state db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}

	// WAL mode creates -wal and -shm sidecar files. Match their permissions
	// to the DB file so other users (via sshd) can write to them.
	relaxSidecarPerms(dbPath)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	return &Store{db: db}, nil
}

// relaxSidecarPerms sets WAL/SHM file permissions to match the main DB file.
// Without this, the first user to open the DB creates sidecar files with their
// umask, blocking other users from writing.
func relaxSidecarPerms(dbPath string) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return
	}
	perm := info.Mode().Perm()
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Chmod(dbPath+suffix, perm)
	}
}

func migrate(db *sql.DB) error {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)

	var version int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for version < schemaVersion {
		switch version {
		case 0:
			if err := ensureSessionsTable(db); err != nil {
				return err
			}
			if err := ensureWorkspacesTable(db); err != nil {
				return err
			}
			version = 6
		case 1, 2:
			// Sessions are ephemeral; safe to recreate on upgrade from pre-v3 schemas.
			slog.Warn("dropping legacy sessions during schema migration; check for orphaned containers with docker ps -f label=managed-by=podspawn", "from_version", version)
			_, _ = db.Exec(`DROP TABLE IF EXISTS sessions`)
			if err := ensureSessionsTable(db); err != nil {
				return err
			}
			version = 3
		case 3:
			if err := ensureLegacyMachinesTable(db); err != nil {
				return err
			}
			version = 4
		case 4:
			hasMode, err := columnExists(db, "machines", "mode")
			if err != nil {
				return err
			}
			if !hasMode {
				if _, err := db.Exec(`ALTER TABLE machines ADD COLUMN mode TEXT NOT NULL DEFAULT 'grace-period'`); err != nil {
					return fmt.Errorf("adding machines.mode column: %w", err)
				}
				if _, err := db.Exec(`
					UPDATE machines
					SET mode = COALESCE(
						(SELECT mode FROM sessions WHERE sessions.user = machines.user AND sessions.project = machines.name),
						'grace-period'
					)`); err != nil {
					return fmt.Errorf("backfilling machines.mode: %w", err)
				}
			}
			version = 5
		case 5:
			if err := migrateV5toV6(db); err != nil {
				return err
			}
			version = 6
		default:
			return fmt.Errorf("unsupported schema version %d", version)
		}
	}

	if err := consolidateSchemaVersion(db, version); err != nil {
		return err
	}

	return nil
}

// migrateV5toV6 renames `machines` to `workspaces`, adds storage-generated `id`
// columns to both tables with unique indexes, and adds a nullable
// `workspace_id` column on sessions backfilled from the existing name-join
// (sessions.user/project ↔ workspaces.user/name).
//
// The migration is split into three transactions so each step can roll back
// independently. Each step is guarded by a sentinel so that if the process
// crashes between steps, a retry resumes at the right point rather than
// failing on a duplicate ALTER.
func migrateV5toV6(db *sql.DB) error {
	hasWorkspaces, err := tableExists(db, "workspaces")
	if err != nil {
		return fmt.Errorf("checking workspaces table: %w", err)
	}
	if !hasWorkspaces {
		if _, err := db.Exec(`ALTER TABLE machines RENAME TO workspaces`); err != nil {
			return fmt.Errorf("renaming machines to workspaces: %w", err)
		}
	}

	hasWsID, err := columnExists(db, "workspaces", "id")
	if err != nil {
		return fmt.Errorf("checking workspaces.id: %w", err)
	}
	if !hasWsID {
		if err := addIDColumnAndUniqueIndex(db, "workspaces", "idx_workspaces_id"); err != nil {
			return fmt.Errorf("adding workspaces.id: %w", err)
		}
	}

	hasSessID, err := columnExists(db, "sessions", "id")
	if err != nil {
		return fmt.Errorf("checking sessions.id: %w", err)
	}
	hasSessWsID, err := columnExists(db, "sessions", "workspace_id")
	if err != nil {
		return fmt.Errorf("checking sessions.workspace_id: %w", err)
	}
	if !hasSessID || !hasSessWsID {
		if err := addSessionIDsAndLink(db, hasSessID, hasSessWsID); err != nil {
			return fmt.Errorf("adding session ids: %w", err)
		}
	}

	return nil
}

// addIDColumnAndUniqueIndex adds an `id TEXT` column to a table, generates a
// UUIDv7 for every row, and creates a unique index, all in a single SQLite
// transaction so the column either gains its uniqueness invariant or rolls
// back to absent.
func addIDColumnAndUniqueIndex(db *sql.DB, table, indexName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN id TEXT`, table)); err != nil {
		return fmt.Errorf("add column: %w", err)
	}

	if err := backfillIDs(tx, table); err != nil {
		return err
	}

	if _, err := tx.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX %s ON %s(id)`, indexName, table)); err != nil {
		return fmt.Errorf("create unique index: %w", err)
	}

	return tx.Commit()
}

// addSessionIDsAndLink adds the missing of `id` and `workspace_id` columns to
// sessions, backfills both, and creates the unique index on id. Single
// transaction; either both columns land with their invariants or neither does.
func addSessionIDsAndLink(db *sql.DB, hasID, hasWsID bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if !hasID {
		if _, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN id TEXT`); err != nil {
			return fmt.Errorf("add sessions.id: %w", err)
		}
	}
	if !hasWsID {
		if _, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN workspace_id TEXT`); err != nil {
			return fmt.Errorf("add sessions.workspace_id: %w", err)
		}
	}

	if !hasID {
		if err := backfillIDs(tx, "sessions"); err != nil {
			return err
		}
	}
	if !hasWsID {
		if _, err := tx.Exec(`
			UPDATE sessions
			SET workspace_id = (
				SELECT id FROM workspaces
				WHERE workspaces.user = sessions.user AND workspaces.name = sessions.project
			)
			WHERE workspace_id IS NULL`); err != nil {
			return fmt.Errorf("backfilling sessions.workspace_id: %w", err)
		}
	}

	if !hasID {
		if _, err := tx.Exec(`CREATE UNIQUE INDEX idx_sessions_id ON sessions(id)`); err != nil {
			return fmt.Errorf("create unique index on sessions.id: %w", err)
		}
	}

	return tx.Commit()
}

// backfillIDs assigns a fresh UUIDv7 to every row whose id is NULL or empty.
// Caller is responsible for the surrounding transaction.
func backfillIDs(tx *sql.Tx, table string) error {
	rows, err := tx.Query(fmt.Sprintf(`SELECT rowid FROM %s WHERE id IS NULL OR id = ''`, table))
	if err != nil {
		return fmt.Errorf("scanning rowids: %w", err)
	}
	var rowIDs []int64
	for rows.Next() {
		var rid int64
		if err := rows.Scan(&rid); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan rowid: %w", err)
		}
		rowIDs = append(rowIDs, rid)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, rid := range rowIDs {
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET id = ? WHERE rowid = ?`, table), id, rid); err != nil {
			return fmt.Errorf("backfill row %d: %w", rid, err)
		}
	}
	return nil
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generating uuid: %w", err)
	}
	return id.String(), nil
}

func ensureSessionsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			user           TEXT NOT NULL,
			project        TEXT NOT NULL DEFAULT '',
			id             TEXT,
			workspace_id   TEXT,
			container_id   TEXT NOT NULL,
			container_name TEXT NOT NULL,
			image          TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'running',
			connections    INTEGER NOT NULL DEFAULT 1 CHECK(connections >= 0),
			grace_expiry   DATETIME,
			created_at     DATETIME NOT NULL,
			last_activity  DATETIME NOT NULL,
			max_lifetime   DATETIME NOT NULL,
			network_id     TEXT NOT NULL DEFAULT '',
			service_ids    TEXT NOT NULL DEFAULT '',
			mode           TEXT NOT NULL DEFAULT 'grace-period',
			PRIMARY KEY (user, project)
		)`)
	if err != nil {
		return fmt.Errorf("creating sessions table: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_id ON sessions(id) WHERE id IS NOT NULL`); err != nil {
		return fmt.Errorf("creating sessions id index: %w", err)
	}
	return nil
}

func ensureWorkspacesTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workspaces (
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
		)`)
	if err != nil {
		return fmt.Errorf("creating workspaces table: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_id ON workspaces(id) WHERE id IS NOT NULL`); err != nil {
		return fmt.Errorf("creating workspaces id index: %w", err)
	}
	return nil
}

// ensureLegacyMachinesTable creates the v3-era `machines` table for upgrades
// that pass through that version. v5→v6 renames it to `workspaces` afterward.
func ensureLegacyMachinesTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS machines (
			user             TEXT NOT NULL,
			name             TEXT NOT NULL,
			project          TEXT NOT NULL,
			repo_url         TEXT NOT NULL,
			branch           TEXT NOT NULL DEFAULT '',
			workspace_path   TEXT NOT NULL,
			workspace_target TEXT NOT NULL,
			created_at       DATETIME NOT NULL,
			initialized      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user, name)
		)`)
	if err != nil {
		return fmt.Errorf("creating machines table: %w", err)
	}
	return nil
}

func tableExists(db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, fmt.Errorf("reading %s schema: %w", table, err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scanning %s schema: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}

	return false, rows.Err()
}

// consolidateSchemaVersion ensures exactly one row exists with the given version.
// Handles both old tables (no constraints) and new databases.
func consolidateSchemaVersion(db *sql.DB, ver int) error {
	_, err := db.Exec(`DELETE FROM schema_version`)
	if err != nil {
		return fmt.Errorf("clearing schema_version: %w", err)
	}
	_, err = db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, ver)
	if err != nil {
		return fmt.Errorf("setting schema version to %d: %w", ver, err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateWorkspace(workspace *Workspace) error {
	id, err := newID()
	if err != nil {
		return err
	}
	workspace.ID = id

	initialized := 0
	if workspace.Initialized {
		initialized = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO workspaces (id, user, name, project, repo_url, branch, mode, workspace_path, workspace_target, created_at, initialized)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.User, workspace.Name, workspace.Project, workspace.RepoURL, workspace.Branch,
		workspace.Mode, workspace.WorkspacePath, workspace.WorkspaceTarget, workspace.CreatedAt.UTC(), initialized,
	)
	return err
}

const workspaceColumns = `id, user, name, project, repo_url, branch, mode, workspace_path, workspace_target, created_at, initialized`

func scanWorkspace(scanner interface{ Scan(...any) error }) (*Workspace, error) {
	workspace := &Workspace{}
	var id sql.NullString
	var initialized int
	err := scanner.Scan(
		&id, &workspace.User, &workspace.Name, &workspace.Project, &workspace.RepoURL, &workspace.Branch, &workspace.Mode,
		&workspace.WorkspacePath, &workspace.WorkspaceTarget, &workspace.CreatedAt, &initialized,
	)
	if id.Valid {
		workspace.ID = id.String
	}
	workspace.Initialized = initialized == 1
	return workspace, err
}

func (s *Store) GetWorkspace(user, name string) (*Workspace, error) {
	row := s.db.QueryRow(`SELECT `+workspaceColumns+` FROM workspaces WHERE user = ? AND name = ?`, user, name)

	workspace, err := scanWorkspace(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return workspace, nil
}

func (s *Store) UpdateWorkspaceInitialized(user, name string, initialized bool) error {
	value := 0
	if initialized {
		value = 1
	}
	_, err := s.db.Exec(`UPDATE workspaces SET initialized = ? WHERE user = ? AND name = ?`, value, user, name)
	return err
}

func (s *Store) UpdateWorkspaceBranch(user, name, branch string) error {
	_, err := s.db.Exec(`UPDATE workspaces SET branch = ? WHERE user = ? AND name = ?`, branch, user, name)
	return err
}

func (s *Store) DeleteWorkspace(user, name string) error {
	_, err := s.db.Exec(`DELETE FROM workspaces WHERE user = ? AND name = ?`, user, name)
	return err
}

func (s *Store) ListWorkspacesByUser(user string) ([]*Workspace, error) {
	rows, err := s.db.Query(`SELECT `+workspaceColumns+` FROM workspaces WHERE user = ? ORDER BY name`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var workspaces []*Workspace
	for rows.Next() {
		workspace, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

func (s *Store) CreateSession(sess *Session) error {
	id, err := newID()
	if err != nil {
		return err
	}
	sess.ID = id

	mode := sess.Mode
	if mode == "" {
		mode = "grace-period"
	}
	_, err = s.db.Exec(
		`INSERT INTO sessions (id, workspace_id, user, project, container_id, container_name, image, status, connections, created_at, last_activity, max_lifetime, network_id, service_ids, mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.WorkspaceID,
		sess.User, sess.Project, sess.ContainerID, sess.ContainerName, sess.Image,
		sess.Status, sess.Connections,
		sess.CreatedAt.UTC(), sess.LastActivity.UTC(), sess.MaxLifetime.UTC(),
		sess.NetworkID, sess.ServiceIDs, mode,
	)
	return err
}

const sessionColumns = `id, workspace_id, user, project, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime, network_id, service_ids, mode`

func scanSession(scanner interface{ Scan(...any) error }) (*Session, error) {
	sess := &Session{}
	var id sql.NullString
	err := scanner.Scan(
		&id, &sess.WorkspaceID,
		&sess.User, &sess.Project, &sess.ContainerID, &sess.ContainerName, &sess.Image,
		&sess.Status, &sess.Connections, &sess.GraceExpiry,
		&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
		&sess.NetworkID, &sess.ServiceIDs, &sess.Mode,
	)
	if id.Valid {
		sess.ID = id.String
	}
	return sess, err
}

func (s *Store) GetSession(user, project string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE user = ? AND project = ?`, user, project)

	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) UpdateConnections(user, project string, delta int) (int, error) {
	row := s.db.QueryRow(
		`UPDATE sessions SET connections = MAX(0, connections + ?), last_activity = ?
		 WHERE user = ? AND project = ? RETURNING connections`,
		delta, time.Now().UTC(), user, project,
	)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("updating connections for %s/%s: %w", user, project, err)
	}
	return count, nil
}

func (s *Store) SetGracePeriod(user, project string, expiry time.Time) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = 'grace_period', grace_expiry = ? WHERE user = ? AND project = ?`,
		expiry.UTC(), user, project,
	)
	return err
}

func (s *Store) CancelGracePeriod(user, project string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = 'running', grace_expiry = NULL WHERE user = ? AND project = ?`,
		user, project,
	)
	return err
}

func (s *Store) DeleteSession(user, project string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user = ? AND project = ?`, user, project)
	return err
}

func (s *Store) ExpiredGracePeriods() ([]*Session, error) {
	return s.queryMultiple(
		`SELECT `+sessionColumns+` FROM sessions WHERE status = 'grace_period' AND grace_expiry < ? AND mode != 'persistent'`,
		time.Now().UTC(),
	)
}

func (s *Store) ExpiredLifetimes() ([]*Session, error) {
	return s.queryMultiple(
		`SELECT `+sessionColumns+` FROM sessions WHERE max_lifetime < ? AND mode != 'persistent'`,
		time.Now().UTC(),
	)
}

func (s *Store) StaleZeroConnections(user, project string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE user = ? AND project = ? AND connections = 0 AND grace_expiry IS NULL AND mode != 'persistent'`,
		user, project)

	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) ListSessions() ([]*Session, error) {
	return s.queryMultiple(`SELECT ` + sessionColumns + ` FROM sessions ORDER BY user, project`)
}

func (s *Store) ListSessionsByUser(user string) ([]*Session, error) {
	return s.queryMultiple(`SELECT `+sessionColumns+` FROM sessions WHERE user = ? ORDER BY project`, user)
}

func (s *Store) queryMultiple(query string, args ...any) ([]*Session, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
