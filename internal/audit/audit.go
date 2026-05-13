package audit

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/podspawn/podspawn/internal/identity"
)

const (
	EventConnect         = "session.connect"
	EventDisconnect      = "session.disconnect"
	EventCreate          = "container.create"
	EventDestroy         = "container.destroy"
	EventCommand         = "session.command"
	EventCleanup         = "cleanup.run"
	EventWorkspaceCreate = "workspace.create"
	EventWorkspaceDelete = "workspace.delete"
	EventPolicyDeny      = "policy.deny"
)

// Subject is the identity bundle stamped onto every audit event: the session
// owner (User, also the in-container OS user), the actor performing the action
// (Actor), and the workspace / session / container the action touches. It
// carries identity only; event-specific context like project, container name,
// image, command, or reason stays as explicit parameters on the typed helpers
// below so no existing payload key is lost.
//
// WorkspaceID / SessionID / ContainerID are omitted from the event when empty:
// default-image and server-mode sessions have no workspace; the legacy
// no-state spawn path has no session row.
type Subject struct {
	User        string
	Actor       identity.Actor
	WorkspaceID string
	SessionID   string
	ContainerID string
}

func (s Subject) attrs() []slog.Attr {
	a := []slog.Attr{
		slog.String("actor", s.Actor.String()),
		slog.String("actor_kind", string(s.Actor.Kind)),
	}
	if s.WorkspaceID != "" {
		a = append(a, slog.String("workspace_id", s.WorkspaceID))
	}
	if s.SessionID != "" {
		a = append(a, slog.String("session_id", s.SessionID))
	}
	if s.ContainerID != "" {
		a = append(a, slog.String("container_id", s.ContainerID))
	}
	return a
}

// Logger writes structured audit events to a JSON-lines file.
// Safe for concurrent use. Nil Logger is a no-op.
type Logger struct {
	mu     sync.Mutex
	logger *slog.Logger
	file   *os.File
}

// Open creates an audit logger writing to the given path.
// Returns nil (no-op) if path is empty.
func Open(path string) (*Logger, error) {
	if path == "" {
		return nil, nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	return &Logger{
		logger: slog.New(handler),
		file:   f,
	}, nil
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Log writes an audit event. No-op if Logger is nil.
func (l *Logger) Log(event string, user string, attrs ...slog.Attr) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	base := []slog.Attr{
		slog.String("event", event),
		slog.String("user", user),
		slog.Time("timestamp", time.Now().UTC()),
	}
	base = append(base, attrs...)

	args := make([]any, len(base))
	for i, a := range base {
		args[i] = a
	}
	l.logger.Info("audit", args...)
}

func (l *Logger) logSubject(event string, s Subject, attrs ...slog.Attr) {
	l.Log(event, s.User, append(s.attrs(), attrs...)...)
}

func (l *Logger) Connect(s Subject, project, container string, connections int) {
	l.logSubject(EventConnect, s,
		slog.String("project", project),
		slog.String("container", container),
		slog.Int("connections", connections),
	)
}

func (l *Logger) Disconnect(s Subject, project, container string, remainingConns int) {
	l.logSubject(EventDisconnect, s,
		slog.String("project", project),
		slog.String("container", container),
		slog.Int("remaining_connections", remainingConns),
	)
}

func (l *Logger) ContainerCreate(s Subject, project, container, image string) {
	l.logSubject(EventCreate, s,
		slog.String("project", project),
		slog.String("container", container),
		slog.String("image", image),
	)
}

func (l *Logger) ContainerDestroy(s Subject, project, container, reason string) {
	l.logSubject(EventDestroy, s,
		slog.String("project", project),
		slog.String("container", container),
		slog.String("reason", reason),
	)
}

func (l *Logger) Command(s Subject, project, command string, exitCode int) {
	outcome := "success"
	if exitCode != 0 {
		outcome = "failure"
	}
	l.logSubject(EventCommand, s,
		slog.String("project", project),
		slog.String("command", command),
		slog.Int("exit_code", exitCode),
		slog.String("outcome", outcome),
	)
}

func (l *Logger) WorkspaceCreate(s Subject, name, project, branch, workspacePath string) {
	l.logSubject(EventWorkspaceCreate, s,
		slog.String("name", name),
		slog.String("project", project),
		slog.String("branch", branch),
		slog.String("workspace", workspacePath),
	)
}

func (l *Logger) WorkspaceDelete(s Subject, name, project, branch, workspacePath, reason string) {
	l.logSubject(EventWorkspaceDelete, s,
		slog.String("name", name),
		slog.String("project", project),
		slog.String("branch", branch),
		slog.String("workspace", workspacePath),
		slog.String("reason", reason),
	)
}

// PolicyDeny records a Stage 8 deny decision. Reason is whatever the
// Policy.Evaluate result carried; the typed PolicyError surfaces the
// same string back to the caller for CLI-visible error text.
func (l *Logger) PolicyDeny(s Subject, op, reason string) {
	l.logSubject(EventPolicyDeny, s,
		slog.String("op", op),
		slog.String("reason", reason),
	)
}
