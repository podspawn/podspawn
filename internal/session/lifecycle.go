package session

import (
	"context"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

// Lifecycle is the seam between the session service and code that actually
// drives containers. Stage 5's default implementation wraps the existing
// spawn package; tests substitute a fake. Keeping this an interface (rather
// than calling spawn directly from the service) is what lets us keep
// service tests cheap and lets future stages swap the container backend
// without touching the service.
type Lifecycle interface {
	// Provision brings the workspace's container to a ready-to-attach state
	// (creates if missing; reattaches if alive). Returns the container name
	// the caller would use to attach.
	Provision(ctx context.Context, p ProvisionParams) (ProvisionResult, error)

	// Stop tears down a live session. Mirrors cleanup.DestroySession.
	Stop(ctx context.Context, p StopParams) error
}

// ProvisionParams is what the service hands the Lifecycle. SessionTemplate
// is the configured *spawn.Session that the service has already wired with
// workspace / mounts / mode; Provision just drives it.
type ProvisionParams struct {
	SessionTemplate *spawn.Session
}

// ProvisionResult is what the Lifecycle hands back. ContainerName is the
// docker container the session is now attached to.
type ProvisionResult struct {
	ContainerName string
}

// StopParams carries the live session row plus the runtime and store needed
// to destroy it.
type StopParams struct {
	Runtime runtime.Runtime
	Store   state.SessionStore
	Session *state.Session
}

type defaultLifecycle struct{}

func newDefaultLifecycle() Lifecycle { return defaultLifecycle{} }

func (defaultLifecycle) Provision(ctx context.Context, p ProvisionParams) (ProvisionResult, error) {
	name, err := p.SessionTemplate.Ensure(ctx)
	if err != nil {
		return ProvisionResult{}, err
	}
	return ProvisionResult{ContainerName: name}, nil
}

func (defaultLifecycle) Stop(ctx context.Context, p StopParams) error {
	return cleanup.DestroySession(ctx, p.Runtime, p.Store, p.Session)
}
