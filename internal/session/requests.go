package session

import (
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

// Ref names a target by user and workspace/session name. Stage 5 keeps the
// existing (user, name) addressing scheme; ID-keyed Refs are deferred.
type Ref struct {
	User string
	Name string
}

// CreateRequest carries the inputs needed to resolve or create a workspace
// and configure the session for downstream provisioning. The service mutates
// SessionTemplate in place; for cmd/run and cmd/shell the same pointer is
// returned via CreateResult.Attach() so cmd code can drive the attach path
// without going through the service.
//
// The Provision flag controls whether the service itself runs the
// container-provisioning step (today: spawn.Session.Ensure) or leaves it to
// the caller's downstream RunAndCleanup. This split is a transition concession:
// the create flow has no attach and wants the service to provision; the run
// and shell flows attach immediately afterward and rely on RunAndCleanup's
// existing idempotent ensure path. Later stages collapse this.
type CreateRequest struct {
	User        string
	Name        string
	ProjectName string // empty = no project / default-image flow
	Branch      string // CLI override; empty = derived from project / podfile

	// Actor is who is establishing this session. Required: Service.Create
	// rejects a request whose Actor is not identity.Actor.Valid(), and one
	// whose Actor.OSUser disagrees with User. For local CLI flows the actor
	// is identity.Human(User). Stage 6 validates and holds it; forwarding it
	// onto spawn.Session and audit events is Stage 7.
	Actor identity.Actor

	// Ephemeral, when true, sets up a one-shot workspace (no workspaces row;
	// clone to a tmp dir; RemoveWorkspaceOnDestroy=true). Mirrors the
	// existing setupEphemeralProjectRun behavior driven by `podspawn run`.
	Ephemeral bool

	// Provision, when true, causes the service to call Lifecycle.Provision
	// after configuring the template. Used by `podspawn create`. The run
	// and shell flows pass false.
	Provision bool

	// SessionTemplate is the pre-built *spawn.Session that the service
	// mutates with the resolved workspace metadata. Required.
	SessionTemplate *spawn.Session
}

// CreateResult is the response to a successful Service.Create call.
type CreateResult struct {
	// Workspace is the resolved or newly-created workspace row.
	// May be nil for default-image / no-project flows.
	Workspace *state.Workspace

	// ContainerName is set when Provision=true and the container has been
	// created (or reattached). Empty otherwise.
	ContainerName string

	// Created reports whether this call inserted a fresh workspaces row.
	// Mirrors the bool return of the legacy setupNamedMachine.
	Created bool

	// session is the configured *spawn.Session, retained so cmd/run.go and
	// cmd/shell.go can pick it up via Attach(). Other callers must not
	// reach for it: the guardrail limits Attach() usage to those two CLI
	// entry points.
	session *spawn.Session
}

// Attach returns the configured *spawn.Session.
//
// Guardrail (Stage 5): only cmd/run.go and cmd/shell.go may call this. It
// exists as a tactical escape hatch so those two flows can drive the
// existing TTY/attach code in spawn without that code creeping into the
// service surface. Treat any other call site as scope drift.
func (r *CreateResult) Attach() *spawn.Session {
	if r == nil {
		return nil
	}
	return r.session
}

// InspectResult is the response to Service.Inspect. Both fields may be nil
// independently; callers must check.
//
//   - Workspace == nil && Session == nil → workspace and session both absent.
//   - Workspace != nil && Session == nil → workspace exists, no live session.
//   - Workspace == nil && Session != nil → server-mode / default-image session.
//   - both non-nil                       → active session backed by a workspace.
type InspectResult struct {
	Workspace *state.Workspace
	Session   *state.Session
}

// ListRequest narrows the listing to one user. RegisteredOnly mirrors the
// existing `--machines` flag on `podspawn list`: when true, sessions without
// a backing workspace row are suppressed.
type ListRequest struct {
	User           string
	RegisteredOnly bool
}

// MachineRow is the rendering-ready row type returned by Service.List.
// Field names match the existing cmd/list.go internal row type so cmd code
// can render without further translation.
type MachineRow struct {
	Name   string
	Status string
	Branch string
	Image  string
	Age    string
}

// ListResult is a thin envelope around the row slice. Kept as a struct so
// future fields (totals, filters) can be added without breaking callers.
type ListResult struct {
	Rows []MachineRow
}
