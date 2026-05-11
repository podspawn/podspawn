package session

import (
	"context"
	"fmt"
)

// End stops a live session. Mirrors the current `podspawn stop` and
// `podspawn down` semantics: destroy the container + companion services +
// network and remove the sessions row. It is intentionally NOT a workspace
// removal path; preserved workspaces and named workspace rows survive End.
// `podspawn rm` remains the explicit workspace-removal flow.
//
// Returns ErrSessionNotFound when no live session exists for the Ref,
// regardless of whether a workspace with the same name exists. This
// preserves the legacy CLI contract: stop/down only act on live sessions.
func (s *Service) End(ctx context.Context, ref Ref) error {
	if ref.User == "" {
		return fmt.Errorf("%w: missing User", ErrInvalidRequest)
	}
	sess, err := s.sessions.GetSession(ref.User, ref.Name)
	if err != nil {
		return fmt.Errorf("looking up session: %w", err)
	}
	if sess == nil {
		return ErrSessionNotFound
	}
	if err := s.lifecycle.Stop(ctx, StopParams{
		Runtime: s.rt,
		Store:   s.sessions,
		Session: sess,
	}); err != nil {
		// Double-%w keeps the underlying runtime error unwrap-able while
		// still letting callers branch on session.ErrRuntimeFailure.
		return fmt.Errorf("%w: %w", ErrRuntimeFailure, err)
	}
	return nil
}
