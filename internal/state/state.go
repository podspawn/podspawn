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
	ContainerID   string
	ContainerName string
	Image         string
	Status        string // "running" | "grace_period"
	Connections   int
	GraceExpiry   sql.NullTime
	CreatedAt     time.Time
	LastActivity  time.Time
	MaxLifetime   time.Time
}

// SessionStore is the interface for session persistence.
// Implemented by Store (SQLite) and FakeStore (tests).
type SessionStore interface {
	CreateSession(sess *Session) error
	GetSession(user string) (*Session, error)
	UpdateConnections(user string, delta int) (int, error)
	SetGracePeriod(user string, expiry time.Time) error
	CancelGracePeriod(user string) error
	DeleteSession(user string) error
	ExpiredGracePeriods() ([]*Session, error)
	ExpiredLifetimes() ([]*Session, error)
	StaleZeroConnections(user string) (*Session, error)
	Close() error
}

type Store struct {
	db *sql.DB
}

var _ SessionStore = (*Store)(nil)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	user           TEXT PRIMARY KEY,
	container_id   TEXT NOT NULL,
	container_name TEXT NOT NULL,
	image          TEXT NOT NULL,
	status         TEXT NOT NULL DEFAULT 'running',
	connections    INTEGER NOT NULL DEFAULT 1 CHECK(connections >= 0),
	grace_expiry   DATETIME,
	created_at     DATETIME NOT NULL,
	last_activity  DATETIME NOT NULL,
	max_lifetime   DATETIME NOT NULL
);`

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

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (user, container_id, container_name, image, status, connections, created_at, last_activity, max_lifetime)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.User, sess.ContainerID, sess.ContainerName, sess.Image,
		sess.Status, sess.Connections,
		sess.CreatedAt.UTC(), sess.LastActivity.UTC(), sess.MaxLifetime.UTC(),
	)
	return err
}

func (s *Store) GetSession(user string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT user, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime
		 FROM sessions WHERE user = ?`, user)

	sess := &Session{}
	err := row.Scan(
		&sess.User, &sess.ContainerID, &sess.ContainerName, &sess.Image,
		&sess.Status, &sess.Connections, &sess.GraceExpiry,
		&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// UpdateConnections atomically adjusts the connection count and returns the new value.
func (s *Store) UpdateConnections(user string, delta int) (int, error) {
	row := s.db.QueryRow(
		`UPDATE sessions SET connections = MAX(0, connections + ?), last_activity = ?
		 WHERE user = ? RETURNING connections`,
		delta, time.Now().UTC(), user,
	)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("updating connections for %s: %w", user, err)
	}
	return count, nil
}

func (s *Store) SetGracePeriod(user string, expiry time.Time) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = 'grace_period', grace_expiry = ? WHERE user = ?`,
		expiry.UTC(), user,
	)
	return err
}

func (s *Store) CancelGracePeriod(user string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET status = 'running', grace_expiry = NULL WHERE user = ?`,
		user,
	)
	return err
}

func (s *Store) DeleteSession(user string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user = ?`, user)
	return err
}

func (s *Store) ExpiredGracePeriods() ([]*Session, error) {
	return s.queryExpired(
		`SELECT user, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime
		 FROM sessions WHERE status = 'grace_period' AND grace_expiry < ?`,
		time.Now().UTC(),
	)
}

func (s *Store) ExpiredLifetimes() ([]*Session, error) {
	return s.queryExpired(
		`SELECT user, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime
		 FROM sessions WHERE max_lifetime < ?`,
		time.Now().UTC(),
	)
}

// StaleZeroConnections finds a session for the given user with
// connections=0 and no grace_expiry set, indicating a crash between
// decrement and grace-set.
func (s *Store) StaleZeroConnections(user string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT user, container_id, container_name, image, status, connections, grace_expiry, created_at, last_activity, max_lifetime
		 FROM sessions WHERE user = ? AND connections = 0 AND grace_expiry IS NULL`, user)

	sess := &Session{}
	err := row.Scan(
		&sess.User, &sess.ContainerID, &sess.ContainerName, &sess.Image,
		&sess.Status, &sess.Connections, &sess.GraceExpiry,
		&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) queryExpired(query string, args ...any) ([]*Session, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows closed on function exit

	var sessions []*Session
	for rows.Next() {
		sess := &Session{}
		if err := rows.Scan(
			&sess.User, &sess.ContainerID, &sess.ContainerName, &sess.Image,
			&sess.Status, &sess.Connections, &sess.GraceExpiry,
			&sess.CreatedAt, &sess.LastActivity, &sess.MaxLifetime,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
