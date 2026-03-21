package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusRunning     = "running"
	StatusGracePeriod = "grace_period"
)

type Session struct {
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
	NetworkID     string // Docker network for companion services
	ServiceIDs    string // comma-separated container IDs
	Mode          string // grace-period, destroy-on-disconnect, persistent
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

type Store struct {
	db *sql.DB
}

var _ SessionStore = (*Store)(nil)

const schemaVersion = 3

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

	if version >= schemaVersion {
		// Consolidate any duplicate rows left by older versions
		if err := consolidateSchemaVersion(db, version); err != nil {
			return err
		}
		return nil
	}

	// Sessions are ephemeral; safe to recreate on upgrade.
	_, _ = db.Exec(`DROP TABLE IF EXISTS sessions`)

	_, err = db.Exec(`
		CREATE TABLE sessions (
			user           TEXT NOT NULL,
			project        TEXT NOT NULL DEFAULT '',
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

	if err := consolidateSchemaVersion(db, schemaVersion); err != nil {
		return err
	}

	return nil
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

func (s *Store) CreateSession(sess *Session) error {
	mode := sess.Mode
	if mode == "" {
		mode = "grace-period"
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (user, project, container_id, container_name, image, status, connections, created_at, last_activity, max_lifetime, network_id, service_ids, mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.User, sess.Project, sess.ContainerID, sess.ContainerName, sess.Image,
		sess.Status, sess.Connections,
		sess.CreatedAt.UTC(), sess.LastActivity.UTC(), sess.MaxLifetime.UTC(),
		sess.NetworkID, sess.ServiceIDs, mode,
	)
	return err
}

const sessionColumns = `user, project, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime, network_id, service_ids, mode`

func scanSession(scanner interface{ Scan(...any) error }) (*Session, error) {
	sess := &Session{}
	err := scanner.Scan(
		&sess.User, &sess.Project, &sess.ContainerID, &sess.ContainerName, &sess.Image,
		&sess.Status, &sess.Connections, &sess.GraceExpiry,
		&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
		&sess.NetworkID, &sess.ServiceIDs, &sess.Mode,
	)
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
