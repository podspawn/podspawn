package session

import "errors"

// Sentinel errors for the Stage 5 session-plane surface.
//
// These mirror the error classes the Stage 5 plan calls out, narrowed to
// what the four implemented methods (Create, Inspect, List, End) actually
// produce. Approval / policy / runtime-specific classes are intentionally
// absent; later stages add their own.
//
// Callers should use errors.Is for class membership and errors.As to reach
// the wrapped detail (e.g. *spawn.PreservedWorkspaceError under
// ErrUnsafeTransition).
var (
	ErrInvalidRequest    = errors.New("session: invalid request")
	ErrWorkspaceNotFound = errors.New("session: workspace not found")
	ErrSessionNotFound   = errors.New("session: live session not found")
	ErrUnsafeTransition  = errors.New("session: unsafe transition")
	ErrRuntimeFailure    = errors.New("session: runtime failure")
)
