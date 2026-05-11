package session

import (
	"context"
	"fmt"
)

// Inspect resolves a Ref to the current workspace + live session pair.
// Either or both may be nil in the result:
//
//   - Workspace == nil && Session == nil → ErrWorkspaceNotFound (the
//     caller asked for a target that does not exist on either side).
//   - Workspace != nil && Session == nil → workspace exists, no session.
//   - Workspace == nil && Session != nil → server-mode / default-image
//     session with no workspace row (workspace_id IS NULL).
//   - both non-nil                       → live session backed by a workspace.
//
// Inspect is read-only. It does not mutate the workspace, does not start
// or stop containers, and does not call audit.
func (s *Service) Inspect(ctx context.Context, ref Ref) (*InspectResult, error) {
	if ref.User == "" {
		return nil, fmt.Errorf("%w: missing User", ErrInvalidRequest)
	}

	ws, err := s.workspaces.GetWorkspace(ref.User, ref.Name)
	if err != nil {
		return nil, fmt.Errorf("looking up workspace: %w", err)
	}
	sess, err := s.sessions.GetSession(ref.User, ref.Name)
	if err != nil {
		return nil, fmt.Errorf("looking up session: %w", err)
	}

	if ws == nil && sess == nil {
		return nil, ErrWorkspaceNotFound
	}
	return &InspectResult{Workspace: ws, Session: sess}, nil
}
