package session

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/state"
)

// List enumerates one user's workspaces and live sessions, merged into the
// rendering-ready row shape used by `podspawn list`. The merge precedence
// matches the legacy cmd/list.go behavior, including the preserved-beats-
// stale-session precedence the deferred doc flags as load-bearing.
//
// RegisteredOnly mirrors the existing `--machines` flag: when true, ad-hoc
// sessions (no backing workspace row) are suppressed.
func (s *Service) List(ctx context.Context, req ListRequest) (*ListResult, error) {
	_ = ctx
	if req.User == "" {
		return nil, fmt.Errorf("%w: missing User", ErrInvalidRequest)
	}
	sessions, err := s.sessions.ListSessionsByUser(req.User)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	workspaces, err := s.workspaces.ListWorkspacesByUser(req.User)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	rows := mergeRows(workspaces, sessions, req.RegisteredOnly, s.now())
	return &ListResult{Rows: rows}, nil
}

// mergeRows produces the same row set as cmd/list.go's collectMachineRows
// did, lifted here so the merge has one home. now is taken as a parameter
// so the service's injected clock controls "age" rendering in tests.
func mergeRows(workspaces []*state.Workspace, sessions []*state.Session, registeredOnly bool, now time.Time) []MachineRow {
	rowsByName := make(map[string]MachineRow, len(workspaces)+len(sessions))
	for _, ws := range workspaces {
		status := "stopped"
		if !ws.Initialized {
			status = "uninitialized"
		}
		if ws.State == state.WorkspaceStatePreserved {
			status = "preserved"
		}
		rowsByName[ws.Name] = MachineRow{
			Name:   ws.Name,
			Status: status,
			Branch: ws.Branch,
			Age:    cleanup.FormatDuration(now.Sub(ws.CreatedAt)),
		}
	}
	for _, sess := range sessions {
		name := sess.Project
		if name == "" {
			name = "(default)"
		}
		existing, hasWorkspace := rowsByName[name]
		if registeredOnly && !hasWorkspace {
			continue
		}
		// Preserved beats stale session row even if both briefly coexist
		// (e.g. between mark-preserved and session-row cleanup on fatal
		// on_create). Matches cmd/list.go's existing precedence.
		if hasWorkspace && existing.Status == "preserved" {
			continue
		}
		status := sess.Status
		if status == state.StatusGracePeriod {
			status = "grace"
		}
		rowsByName[name] = MachineRow{
			Name:   name,
			Status: status,
			Branch: existing.Branch,
			Image:  sess.Image,
			Age:    cleanup.FormatDuration(now.Sub(sess.CreatedAt)),
		}
	}
	names := make([]string, 0, len(rowsByName))
	for name := range rowsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]MachineRow, 0, len(names))
	for _, name := range names {
		out = append(out, rowsByName[name])
	}
	return out
}
