package metrics

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

// Snapshot holds point-in-time metrics about the system.
type Snapshot struct {
	TotalSessions    int
	RunningSessions  int
	GraceSessions    int
	TotalConnections int
	DockerContainers int
	OldestSession    time.Duration
}

// Collect gathers metrics from the state DB and Docker.
func Collect(ctx context.Context, store state.SessionStore, rt runtime.Runtime) (*Snapshot, error) {
	sessions, err := store.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	snap := &Snapshot{TotalSessions: len(sessions)}

	for _, sess := range sessions {
		snap.TotalConnections += sess.Connections
		switch sess.Status {
		case "running":
			snap.RunningSessions++
		case "grace_period":
			snap.GraceSessions++
		}
		age := time.Since(sess.CreatedAt)
		if age > snap.OldestSession {
			snap.OldestSession = age
		}
	}

	if rt != nil {
		containers, err := rt.ListContainers(ctx, map[string]string{"managed-by": "podspawn"})
		if err == nil {
			snap.DockerContainers = len(containers)
		}
	}

	return snap, nil
}

// WritePrometheus writes metrics in Prometheus exposition format.
func WritePrometheus(w io.Writer, snap *Snapshot) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	p("# HELP podspawn_sessions_total Total tracked sessions\n")
	p("# TYPE podspawn_sessions_total gauge\n")
	p("podspawn_sessions_total %d\n", snap.TotalSessions)
	p("# HELP podspawn_sessions_running Sessions in running state\n")
	p("# TYPE podspawn_sessions_running gauge\n")
	p("podspawn_sessions_running %d\n", snap.RunningSessions)
	p("# HELP podspawn_sessions_grace Sessions in grace period\n")
	p("# TYPE podspawn_sessions_grace gauge\n")
	p("podspawn_sessions_grace %d\n", snap.GraceSessions)
	p("# HELP podspawn_connections_total Total active SSH connections\n")
	p("# TYPE podspawn_connections_total gauge\n")
	p("podspawn_connections_total %d\n", snap.TotalConnections)
	p("# HELP podspawn_containers_docker Docker containers with managed-by=podspawn\n")
	p("# TYPE podspawn_containers_docker gauge\n")
	p("podspawn_containers_docker %d\n", snap.DockerContainers)
	p("# HELP podspawn_oldest_session_seconds Age of the oldest session in seconds\n")
	p("# TYPE podspawn_oldest_session_seconds gauge\n")
	p("podspawn_oldest_session_seconds %.0f\n", snap.OldestSession.Seconds())
}

// WriteHuman writes a human-readable summary.
func WriteHuman(w io.Writer, snap *Snapshot) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	p("Sessions:    %d total (%d running, %d grace)\n", snap.TotalSessions, snap.RunningSessions, snap.GraceSessions)
	p("Connections: %d active\n", snap.TotalConnections)
	p("Containers:  %d in Docker\n", snap.DockerContainers)
	if snap.OldestSession > 0 {
		p("Oldest:      %s\n", snap.OldestSession.Truncate(time.Second))
	}
}
