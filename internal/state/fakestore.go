package state

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"
)

type FakeStore struct {
	mu         sync.Mutex
	Sessions   map[string]*Session   // keyed by "user|project"
	Workspaces map[string]*Workspace // keyed by "user|name"
}

var _ SessionStore = (*FakeStore)(nil)
var _ WorkspaceStore = (*FakeStore)(nil)

func NewFakeStore() *FakeStore {
	return &FakeStore{
		Sessions:   make(map[string]*Session),
		Workspaces: make(map[string]*Workspace),
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
	id, err := newID()
	if err != nil {
		return err
	}
	sess.ID = id
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

func (f *FakeStore) CreateWorkspace(workspace *Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := sessionKey(workspace.User, workspace.Name)
	if _, exists := f.Workspaces[key]; exists {
		return fmt.Errorf("workspace already exists for %s/%s", workspace.User, workspace.Name)
	}
	id, err := newID()
	if err != nil {
		return err
	}
	workspace.ID = id
	cp := *workspace
	if cp.Mode == "" {
		cp.Mode = "grace-period"
	}
	f.Workspaces[key] = &cp
	return nil
}

func (f *FakeStore) GetWorkspace(user, name string) (*Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	workspace, ok := f.Workspaces[sessionKey(user, name)]
	if !ok {
		return nil, nil
	}
	cp := *workspace
	return &cp, nil
}

func (f *FakeStore) UpdateWorkspaceInitialized(user, name string, initialized bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	workspace, ok := f.Workspaces[sessionKey(user, name)]
	if !ok {
		return fmt.Errorf("no workspace for %s/%s", user, name)
	}
	workspace.Initialized = initialized
	return nil
}

func (f *FakeStore) UpdateWorkspaceBranch(user, name, branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	workspace, ok := f.Workspaces[sessionKey(user, name)]
	if !ok {
		return fmt.Errorf("no workspace for %s/%s", user, name)
	}
	workspace.Branch = branch
	return nil
}

func (f *FakeStore) DeleteWorkspace(user, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Workspaces, sessionKey(user, name))
	return nil
}

func (f *FakeStore) ListWorkspacesByUser(user string) ([]*Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Workspace
	for _, workspace := range f.Workspaces {
		if workspace.User == user {
			cp := *workspace
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}
