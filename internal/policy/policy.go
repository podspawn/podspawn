// Package policy is the Stage 8 in-process policy decision point.
//
// It is deny-only: every Policy.Evaluate returns either an allow or a
// deny; approvals (Stage 9) are not modeled here. The package is a leaf
// over identity, state, and audit so internal/session can depend on it
// without a cycle.
//
// The shipping default is AllowAll. There is no rule format, no config
// surface, and no per-workspace/per-caller/per-profile attachment in
// Stage 8 — those land with approvals.
//
// Field population on policy.Request is asymmetric on purpose: each gate
// carries only the context its callsite already has on hand. See the
// table on Request for the Stage 8 producer matrix.
package policy

import (
	"context"
	"errors"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/state"
)

// Op names the action a policy is deciding on.
type Op string

const (
	OpSessionCreate         Op = "session.create"
	OpSessionAttach         Op = "session.attach"          // cmd/shell.go in Stage 8
	OpSessionEnd            Op = "session.end"             // stop + down
	OpWorkspaceRemove       Op = "workspace.remove"        // rm
	OpWorkspaceBranchSwitch Op = "workspace.branch_switch" // switch-branch
)

// Request carries the inputs a policy decides on.
//
// Stage 8 producer matrix:
//
//	Op                        Workspace  Session  ContainerID  RequestedBranch
//	OpSessionCreate           ~          ~        ~            yes (req.Branch)
//	OpSessionAttach (shell)   yes*       yes*     yes*         ~
//	OpSessionEnd              ~          yes      yes          ~
//	OpWorkspaceRemove         yes        yes*     yes*         ~
//	OpWorkspaceBranchSwitch   yes        yes*     yes*         yes (args[1])
//
//	yes  = always populated when reachable
//	yes* = populated when the callsite's existing lookup returned non-nil
//	~    = intentionally empty in Stage 8
//
// Command / Path / NetworkDest / ServiceName are deliberately absent: no
// producer exists in Stage 8 (Exec / ReadFile / WriteFile / Glob and
// service operations are deferred). The struct grows only when a real
// producer lands in the same stage as the field.
type Request struct {
	Op              Op
	Actor           identity.Actor
	OwnerUser       string
	Workspace       *state.Workspace
	Session         *state.Session
	ContainerID     string
	RequestedBranch string
}

// Decision is the result of Policy.Evaluate. Reason is required when
// Allow is false; it surfaces in both the wrapped error and the audit
// record.
type Decision struct {
	Allow  bool
	Reason string
}

// Policy is the single interface a runtime decision point implements.
type Policy interface {
	Evaluate(ctx context.Context, req Request) Decision
}

// AllowAll is the Stage 8 default. Every request is permitted.
type AllowAll struct{}

func (AllowAll) Evaluate(context.Context, Request) Decision {
	return Decision{Allow: true}
}

// ErrPolicyDenied is the sentinel returned (wrapped in PolicyError) on
// any deny decision. Callers branch on errors.Is(err, ErrPolicyDenied).
var ErrPolicyDenied = errors.New("policy: denied")

// PolicyError is the typed wrapper Enforce returns on a deny decision.
// Its Unwrap returns ErrPolicyDenied so errors.Is keeps working through
// any further cmd-side fmt.Errorf wraps.
type PolicyError struct {
	Op     Op
	Actor  identity.Actor
	Reason string
}

func (e *PolicyError) Error() string {
	return "policy denied " + string(e.Op) + ": " + e.Reason
}

func (e *PolicyError) Unwrap() error { return ErrPolicyDenied }

// Enforce is the single denial path. Nil policy defaults to AllowAll.
// *audit.Logger is nil-safe (every audit method short-circuits on a
// nil receiver). Stage 8 has exactly one production caller
// (session.Service.Authorize); the free function is kept because
// isolating denial-shape construction in one place is independently
// useful for testing and avoids forcing future callers (MCP / HTTP) to
// re-implement the wrap+audit dance.
func Enforce(ctx context.Context, p Policy, log *audit.Logger, req Request) error {
	if p == nil {
		p = AllowAll{}
	}
	d := p.Evaluate(ctx, req)
	if d.Allow {
		return nil
	}
	log.PolicyDeny(subjectOf(req), string(req.Op), d.Reason)
	return &PolicyError{Op: req.Op, Actor: req.Actor, Reason: d.Reason}
}

func subjectOf(req Request) audit.Subject {
	var wsID, sessID string
	if req.Workspace != nil {
		wsID = req.Workspace.ID
	}
	if req.Session != nil {
		sessID = req.Session.ID
	}
	return audit.Subject{
		User:        req.OwnerUser,
		Actor:       req.Actor,
		WorkspaceID: wsID,
		SessionID:   sessID,
		ContainerID: req.ContainerID,
	}
}

// ContainerIDOf is the nil-safe accessor used by callers that have a
// *state.Session in hand but may be looking at the "no live session"
// case. Lives here so the cmd helpers don't grow their own nil checks.
func ContainerIDOf(sess *state.Session) string {
	if sess == nil {
		return ""
	}
	return sess.ContainerID
}
