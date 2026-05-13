package session

import (
	"context"
	"fmt"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/policy"
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
//
// On success an audit `container.destroy` event with reason="stop" is
// emitted, attributed to req.Actor (the requester, e.g. an operator) while
// the `user` field stays the session owner (sess.User).
func (s *Service) End(ctx context.Context, req EndRequest) error {
	if req.Ref.User == "" {
		return fmt.Errorf("%w: missing User", ErrInvalidRequest)
	}
	if !req.Actor.Valid() {
		return fmt.Errorf("%w: missing or invalid Actor", ErrInvalidRequest)
	}
	sess, err := s.sessions.GetSession(req.Ref.User, req.Ref.Name)
	if err != nil {
		return fmt.Errorf("looking up session: %w", err)
	}
	if sess == nil {
		return ErrSessionNotFound
	}
	// Stage 8 gate: the existing session lookup already produced sess;
	// the gate consumes it. No new lookup, no new failure path under
	// the AllowAll default.
	if err := s.Authorize(ctx, policy.Request{
		Op:          policy.OpSessionEnd,
		Actor:       req.Actor,
		OwnerUser:   sess.User,
		Session:     sess,
		ContainerID: sess.ContainerID,
	}); err != nil {
		return err
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

	wsID := ""
	if sess.WorkspaceID.Valid {
		wsID = sess.WorkspaceID.String
	}
	s.audit.ContainerDestroy(audit.Subject{
		User:        sess.User,
		Actor:       req.Actor,
		WorkspaceID: wsID,
		SessionID:   sess.ID,
		ContainerID: sess.ContainerID,
	}, sess.Project, sess.ContainerName, "stop")
	return nil
}
