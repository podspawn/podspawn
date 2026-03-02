package state

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

type FakeStore struct {
	mu       sync.Mutex
	Sessions map[string]*Session
}

var _ SessionStore = (*FakeStore)(nil)

func NewFakeStore() *FakeStore {
	return &FakeStore{Sessions: make(map[string]*Session)}
}

func (f *FakeStore) CreateSession(sess *Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.Sessions[sess.User]; exists {
		return fmt.Errorf("session already exists for %s", sess.User)
	}
	cp := *sess
	f.Sessions[sess.User] = &cp
	return nil
}

func (f *FakeStore) GetSession(user string) (*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[user]
	if !ok {
		return nil, nil
	}
	cp := *sess
	return &cp, nil
}

func (f *FakeStore) UpdateConnections(user string, delta int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[user]
	if !ok {
		return 0, fmt.Errorf("no session for %s", user)
	}
	sess.Connections += delta
	if sess.Connections < 0 {
		sess.Connections = 0
	}
	sess.LastActivity = time.Now().UTC()
	return sess.Connections, nil
}

func (f *FakeStore) SetGracePeriod(user string, expiry time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[user]
	if !ok {
		return fmt.Errorf("no session for %s", user)
	}
	sess.Status = "grace_period"
	sess.GraceExpiry = sql.NullTime{Time: expiry, Valid: true}
	return nil
}

func (f *FakeStore) CancelGracePeriod(user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[user]
	if !ok {
		return fmt.Errorf("no session for %s", user)
	}
	sess.Status = "running"
	sess.GraceExpiry = sql.NullTime{}
	return nil
}

func (f *FakeStore) DeleteSession(user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Sessions, user)
	return nil
}

func (f *FakeStore) ExpiredGracePeriods() ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var out []*Session
	for _, sess := range f.Sessions {
		if sess.Status == "grace_period" && sess.GraceExpiry.Valid && sess.GraceExpiry.Time.Before(now) {
			cp := *sess
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *FakeStore) ExpiredLifetimes() ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var out []*Session
	for _, sess := range f.Sessions {
		if sess.MaxLifetime.Before(now) {
			cp := *sess
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *FakeStore) StaleZeroConnections(user string) (*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[user]
	if !ok {
		return nil, nil
	}
	if sess.Connections == 0 && !sess.GraceExpiry.Valid {
		cp := *sess
		return &cp, nil
	}
	return nil, nil
}

func (f *FakeStore) Close() error { return nil }
