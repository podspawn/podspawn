package audit

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

// Event types for the audit log.
const (
	EventConnect    = "session.connect"
	EventDisconnect = "session.disconnect"
	EventCreate     = "container.create"
	EventDestroy    = "container.destroy"
	EventCommand    = "session.command"
	EventCleanup    = "cleanup.run"
)

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

// Convenience methods for common events

func (l *Logger) Connect(user, project, container string, connections int) {
	l.Log(EventConnect, user,
		slog.String("project", project),
		slog.String("container", container),
		slog.Int("connections", connections),
	)
}

func (l *Logger) Disconnect(user, project, container string, remainingConns int) {
	l.Log(EventDisconnect, user,
		slog.String("project", project),
		slog.String("container", container),
		slog.Int("remaining_connections", remainingConns),
	)
}

func (l *Logger) ContainerCreate(user, project, container, image string) {
	l.Log(EventCreate, user,
		slog.String("project", project),
		slog.String("container", container),
		slog.String("image", image),
	)
}

func (l *Logger) ContainerDestroy(user, project, container, reason string) {
	l.Log(EventDestroy, user,
		slog.String("project", project),
		slog.String("container", container),
		slog.String("reason", reason),
	)
}

func (l *Logger) Command(user, project, command string) {
	l.Log(EventCommand, user,
		slog.String("project", project),
		slog.String("command", command),
	)
}
