package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func DestroySession(ctx context.Context, rt runtime.Runtime, store state.SessionStore, sess *state.Session) error {
	_ = rt.RemoveContainer(ctx, sess.ContainerName)

	if sess.ServiceIDs != "" {
		for _, id := range strings.Split(sess.ServiceIDs, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			_ = rt.RemoveContainer(ctx, id)
		}
	}
	if sess.NetworkID != "" {
		_ = rt.RemoveNetwork(ctx, sess.NetworkID)
	}

	return store.DeleteSession(sess.User, sess.Project)
}

func ExpireGracePeriods(ctx context.Context, rt runtime.Runtime, store state.SessionStore) int {
	sessions, err := store.ExpiredGracePeriods()
	if err != nil {
		slog.Error("failed to query expired grace periods", "error", err)
		return 0
	}
	for _, sess := range sessions {
		slog.Info("grace period expired, destroying", "user", sess.User, "project", sess.Project, "container", sess.ContainerName)
		if err := DestroySession(ctx, rt, store, sess); err != nil {
			slog.Error("failed to destroy expired session", "user", sess.User, "error", err)
		}
	}
	return len(sessions)
}

func EnforceMaxLifetimes(ctx context.Context, rt runtime.Runtime, store state.SessionStore) int {
	sessions, err := store.ExpiredLifetimes()
	if err != nil {
		slog.Error("failed to query expired lifetimes", "error", err)
		return 0
	}
	for _, sess := range sessions {
		slog.Info("max lifetime exceeded, destroying", "user", sess.User, "project", sess.Project, "container", sess.ContainerName)
		if err := DestroySession(ctx, rt, store, sess); err != nil {
			slog.Error("failed to destroy expired session", "user", sess.User, "error", err)
		}
	}
	return len(sessions)
}

func ReconcileOrphans(ctx context.Context, rt runtime.Runtime, store state.SessionStore) int {
	containers, err := rt.ListContainers(ctx, map[string]string{"managed-by": "podspawn"})
	if err != nil {
		slog.Error("failed to list podspawn containers", "error", err)
		return 0
	}

	sessions, err := store.ListSessions()
	if err != nil {
		slog.Error("failed to list sessions", "error", err)
		return 0
	}

	tracked := make(map[string]bool)
	for _, sess := range sessions {
		tracked[sess.ContainerName] = true
		if sess.ServiceIDs != "" {
			for _, id := range strings.Split(sess.ServiceIDs, ",") {
				tracked[strings.TrimSpace(id)] = true
			}
		}
	}

	removed := 0
	for _, c := range containers {
		if tracked[c.Name] || tracked[c.ID] {
			continue
		}
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		slog.Info("removing orphaned container", "name", c.Name, "id", shortID)
		if err := rt.RemoveContainer(ctx, c.ID); err != nil {
			slog.Error("failed to remove orphan", "name", c.Name, "error", err)
			continue
		}
		removed++
	}
	return removed
}

// Grace periods checked first so max-lifetime doesn't double-destroy.
func RunOnce(ctx context.Context, rt runtime.Runtime, store state.SessionStore) {
	graced := ExpireGracePeriods(ctx, rt, store)
	expired := EnforceMaxLifetimes(ctx, rt, store)
	orphans := ReconcileOrphans(ctx, rt, store)
	if graced+expired+orphans > 0 {
		slog.Info("cleanup pass complete", "grace_expired", graced, "lifetime_expired", expired, "orphans_removed", orphans)
	}
}

func RunDaemon(ctx context.Context, rt runtime.Runtime, store state.SessionStore, interval time.Duration) error {
	slog.Info("cleanup daemon started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	RunOnce(ctx, rt, store)

	for {
		select {
		case <-ctx.Done():
			slog.Info("cleanup daemon stopped")
			return nil
		case <-ticker.C:
			RunOnce(ctx, rt, store)
		}
	}
}

func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}
