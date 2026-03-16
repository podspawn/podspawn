package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	User          string
	Project       string // together with User forms composite PK
	ContainerID   string
	ContainerName string
	Image         string
	Status        string // "running" | "grace_period"
	Connections   int
	GraceExpiry   sql.NullTime
	CreatedAt     time.Time
	LastActivity  time.Time
	MaxLifetime   time.Time
	NetworkID     string // Docker network for companion services
	ServiceIDs    string // comma-separated container IDs
}

// SessionStore is the interface for session persistence.
// Implemented by Store (SQLite) and FakeStore (tests).
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
	Close() error
}

type Store struct {
	db *sql.DB
}

var _ SessionStore = (*Store)(nil)

const schemaVersion = 2

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
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

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)

	var version int
	err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version)
	if err == sql.ErrNoRows {
		version = 0
	} else if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if version >= schemaVersion {
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
			PRIMARY KEY (user, project)
		)`)
	if err != nil {
		return fmt.Errorf("creating sessions table: %w", err)
	}

	if version == 0 {
		_, err = db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion)
	} else {
		_, err = db.Exec(`UPDATE schema_version SET version = ?`, schemaVersion)
	}
	if err != nil {
		return fmt.Errorf("updating schema version: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (user, project, container_id, container_name, image, status, connections, created_at, last_activity, max_lifetime, network_id, service_ids)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.User, sess.Project, sess.ContainerID, sess.ContainerName, sess.Image,
		sess.Status, sess.Connections,
		sess.CreatedAt.UTC(), sess.LastActivity.UTC(), sess.MaxLifetime.UTC(),
		sess.NetworkID, sess.ServiceIDs,
	)
	return err
}

const sessionColumns = `user, project, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime, network_id, service_ids`

func scanSession(scanner interface{ Scan(...any) error }) (*Session, error) {
	sess := &Session{}
	err := scanner.Scan(
		&sess.User, &sess.Project, &sess.ContainerID, &sess.ContainerName, &sess.Image,
		&sess.Status, &sess.Connections, &sess.GraceExpiry,
		&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
		&sess.NetworkID, &sess.ServiceIDs,
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
		`SELECT `+sessionColumns+` FROM sessions WHERE status = 'grace_period' AND grace_expiry < ?`,
		time.Now().UTC(),
	)
}

func (s *Store) ExpiredLifetimes() ([]*Session, error) {
	return s.queryMultiple(
		`SELECT `+sessionColumns+` FROM sessions WHERE max_lifetime < ?`,
		time.Now().UTC(),
	)
}

func (s *Store) StaleZeroConnections(user, project string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE user = ? AND project = ? AND connections = 0 AND grace_expiry IS NULL`,
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
