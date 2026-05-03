package state

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"
)

type FakeStore struct {
	mu       sync.Mutex
	Sessions map[string]*Session // keyed by "user|project"
	Machines map[string]*Machine // keyed by "user|name"
}

var _ SessionStore = (*FakeStore)(nil)
var _ MachineStore = (*FakeStore)(nil)

func NewFakeStore() *FakeStore {
	return &FakeStore{
		Sessions: make(map[string]*Session),
		Machines: make(map[string]*Machine),
	}
}

func sessionKey(user, project string) string {
	return user + "|" + project
}

func (f *FakeStore) CreateSession(sess *Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := sessionKey(sess.User, sess.Project)
	if _, exists := f.Sessions[key]; exists {
		return fmt.Errorf("session already exists for %s/%s", sess.User, sess.Project)
	}
	cp := *sess
	f.Sessions[key] = &cp
	return nil
}

func (f *FakeStore) GetSession(user, project string) (*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[sessionKey(user, project)]
	if !ok {
		return nil, nil
	}
	cp := *sess
	return &cp, nil
}

func (f *FakeStore) UpdateConnections(user, project string, delta int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[sessionKey(user, project)]
	if !ok {
		return 0, fmt.Errorf("no session for %s/%s", user, project)
	}
	sess.Connections += delta
	if sess.Connections < 0 {
		sess.Connections = 0
	}
	sess.LastActivity = time.Now().UTC()
	return sess.Connections, nil
}

func (f *FakeStore) SetGracePeriod(user, project string, expiry time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[sessionKey(user, project)]
	if !ok {
		return fmt.Errorf("no session for %s/%s", user, project)
	}
	sess.Status = StatusGracePeriod
	sess.GraceExpiry = sql.NullTime{Time: expiry, Valid: true}
	return nil
}

func (f *FakeStore) CancelGracePeriod(user, project string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[sessionKey(user, project)]
	if !ok {
		return fmt.Errorf("no session for %s/%s", user, project)
	}
	sess.Status = StatusRunning
	sess.GraceExpiry = sql.NullTime{}
	return nil
}

func (f *FakeStore) DeleteSession(user, project string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Sessions, sessionKey(user, project))
	return nil
}

func (f *FakeStore) ExpiredGracePeriods() ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	var out []*Session
	for _, sess := range f.Sessions {
		if sess.Mode == "persistent" {
			continue
		}
		if sess.Status == StatusGracePeriod && sess.GraceExpiry.Valid && sess.GraceExpiry.Time.Before(now) {
			cp := *sess
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *FakeStore) ExpiredLifetimes() ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	var out []*Session
	for _, sess := range f.Sessions {
		if sess.Mode == "persistent" {
			continue
		}
		if sess.MaxLifetime.Before(now) {
			cp := *sess
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *FakeStore) StaleZeroConnections(user, project string) (*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.Sessions[sessionKey(user, project)]
	if !ok {
		return nil, nil
	}
	if sess.Mode == "persistent" {
		return nil, nil
	}
	if sess.Connections == 0 && !sess.GraceExpiry.Valid {
		cp := *sess
		return &cp, nil
	}
	return nil, nil
}

func (f *FakeStore) ListSessions() ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Session
	for _, sess := range f.Sessions {
		cp := *sess
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		return out[i].Project < out[j].Project
	})
	return out, nil
}

func (f *FakeStore) ListSessionsByUser(user string) ([]*Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Session
	for _, sess := range f.Sessions {
		if sess.User == user {
			cp := *sess
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Project < out[j].Project
	})
	return out, nil
}

func (f *FakeStore) Close() error { return nil }

func (f *FakeStore) CreateMachine(machine *Machine) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := sessionKey(machine.User, machine.Name)
	if _, exists := f.Machines[key]; exists {
		return fmt.Errorf("machine already exists for %s/%s", machine.User, machine.Name)
	}
	cp := *machine
	f.Machines[key] = &cp
	return nil
}

func (f *FakeStore) GetMachine(user, name string) (*Machine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	machine, ok := f.Machines[sessionKey(user, name)]
	if !ok {
		return nil, nil
	}
	cp := *machine
	return &cp, nil
}

func (f *FakeStore) UpdateMachineInitialized(user, name string, initialized bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	machine, ok := f.Machines[sessionKey(user, name)]
	if !ok {
		return fmt.Errorf("no machine for %s/%s", user, name)
	}
	machine.Initialized = initialized
	return nil
}

func (f *FakeStore) DeleteMachine(user, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Machines, sessionKey(user, name))
	return nil
}

func (f *FakeStore) ListMachinesByUser(user string) ([]*Machine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Machine
	for _, machine := range f.Machines {
		if machine.User == user {
			cp := *machine
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}
