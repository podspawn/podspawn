// Package session is the internal, in-process session control plane for
// Podspawn. It is the Stage 5 deliverable: a transport-neutral Go API that
// owns workspace resolution, session inspection, listing, and live-session
// teardown — the operations that other stages will surface through MCP,
// HTTP, or harness adapters.
//
// Stage 5 scope: Create, Inspect, List, End. Exec, ReadFile, WriteFile,
// Glob, GitState, Snapshot, Resume, and approval flows are explicitly
// out of scope and tracked in docs/agent-workspaces/deferred.md.
//
// The package does not own TTY/stdio. Attach paths stay in cmd, which
// reaches the configured *spawn.Session via CreateResult.Attach() — a
// guardrailed escape hatch limited to cmd/run.go and cmd/shell.go.
package session

import (
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

// Service is the in-process entry point for the session plane. Construct
// one per process and reuse it; methods are safe for concurrent use as
// long as the underlying Store, Runtime, and Lifecycle implementations are.
type Service struct {
	sessions       state.SessionStore
	workspaces     state.WorkspaceStore
	rt             runtime.Runtime
	audit          *audit.Logger
	lifecycle      Lifecycle
	policy         policy.Policy
	projectsFile   string
	workspacesRoot string
	now            func() time.Time
}

// Options configures a Service. Required fields: SessionStore, WorkspaceStore.
// Everything else has a sensible zero / default.
type Options struct {
	SessionStore   state.SessionStore
	WorkspaceStore state.WorkspaceStore

	// Runtime is the container runtime backing provisioning and teardown.
	// May be nil in tests that exercise only workspace bookkeeping.
	Runtime runtime.Runtime

	// Audit is the JSON-lines audit logger. Nil is a no-op.
	Audit *audit.Logger

	// Lifecycle is the container-driving seam. nil → newDefaultLifecycle,
	// which delegates to spawn.Session.{Ensure,Disconnect} via cleanup.
	Lifecycle Lifecycle

	// Policy is the Stage 8 deny-only decision point. nil → policy.AllowAll{},
	// which preserves pre-Stage-8 behavior. Tests inject deny-stubs here to
	// exercise the gate without touching production cmd code.
	Policy policy.Policy

	// ProjectsFile is the path to projects.yaml. Required for project-backed
	// Create requests; left empty for pure default-image flows.
	ProjectsFile string

	// WorkspacesRoot is the host directory that backs new clones. Required
	// for project-backed Create.
	WorkspacesRoot string

	// Clock injects time for tests. nil → time.Now.
	Clock func() time.Time
}

// New constructs a Service. Required Options fields are not validated here;
// the operations they back will return ErrInvalidRequest if they are missing
// at call time. This matches the rest of the codebase's "fail at use, not
// at construction" pattern.
func New(opts Options) *Service {
	lc := opts.Lifecycle
	if lc == nil {
		lc = newDefaultLifecycle()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	pol := opts.Policy
	if pol == nil {
		pol = policy.AllowAll{}
	}
	return &Service{
		sessions:       opts.SessionStore,
		workspaces:     opts.WorkspaceStore,
		rt:             opts.Runtime,
		audit:          opts.Audit,
		lifecycle:      lc,
		policy:         pol,
		projectsFile:   opts.ProjectsFile,
		workspacesRoot: opts.WorkspacesRoot,
		now:            clock,
	}
}
