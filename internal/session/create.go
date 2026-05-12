package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/podspawn/podspawn/internal/spawn"
)

// Create resolves or creates a workspace, configures the supplied
// SessionTemplate, and (when req.Provision is true) drives the container
// provisioning step through Lifecycle.Provision.
//
// Behavior summary by request shape (preserves the existing CLI flows):
//
//   - Ephemeral=true, ProjectName!="" : matches `podspawn run --project`.
//     Clones a tmp workspace, configures template, no workspaces row.
//   - Ephemeral=false, ProjectName!="": matches `podspawn create --project`
//     (Provision=true) or `podspawn shell <name>`-style preflight.
//     Resolves or creates a workspace row, configures template.
//   - Ephemeral=false, ProjectName=="": no workspace work; template is
//     used as-is. Used by `podspawn create` with default image and by
//     `podspawn run` without a project.
//   - Provision=true: after template configuration, runs Lifecycle.Provision
//     so the container is ready. cmd/run.go and cmd/shell.go pass false
//     and call provisioning themselves via RunAndCleanup.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*CreateResult, error) {
	if req.SessionTemplate == nil {
		return nil, fmt.Errorf("%w: missing SessionTemplate", ErrInvalidRequest)
	}
	if req.User == "" {
		return nil, fmt.Errorf("%w: missing User", ErrInvalidRequest)
	}
	if !req.Actor.Valid() {
		return nil, fmt.Errorf("%w: missing or invalid Actor", ErrInvalidRequest)
	}
	if req.Actor.OSUser != req.User {
		return nil, fmt.Errorf("%w: Actor.OSUser %q != User %q", ErrInvalidRequest, req.Actor.OSUser, req.User)
	}

	tpl := req.SessionTemplate
	tpl.Username = req.User
	tpl.Actor = req.Actor
	if !req.Ephemeral && req.Name != "" {
		// For named flows, the spawn.Session's ProjectName drives container
		// naming and (user, project) keying in state. The eventual
		// ApplyWorkspaceToTemplate overwrites this when a workspace exists,
		// but seeding it here covers the no-project default-image case.
		tpl.ProjectName = req.Name
	} else if req.Ephemeral {
		tpl.ProjectName = req.Name
	}

	res := &CreateResult{session: tpl}

	switch {
	case req.Ephemeral && req.ProjectName != "":
		if err := s.setupEphemeralFlow(ctx, req); err != nil {
			return nil, err
		}
	case !req.Ephemeral && req.Name != "":
		ws, created, err := s.setupNamedFlow(ctx, req)
		if err != nil {
			return nil, err
		}
		res.Workspace = ws
		res.Created = created
	}

	if !req.Provision {
		return res, nil
	}

	provisioned, err := s.lifecycle.Provision(ctx, ProvisionParams{SessionTemplate: tpl})
	if err != nil {
		// Hook failures leave the workspace preserved (spawn handles it).
		// Other failures on a freshly-created workspace roll back so the
		// next CLI invocation doesn't trip over a dangling row.
		var hookErr *spawn.HookError
		if res.Created && !errors.As(err, &hookErr) {
			s.rollbackCreate(tpl)
		}
		// Double-%w preserves both: errors.Is(err, ErrRuntimeFailure) and
		// errors.As(err, &*spawn.HookError) — the latter is load-bearing
		// for cmd/create.go's wrapCreateError, which surfaces the
		// preserved-workspace recovery hint on on_create failures.
		return nil, fmt.Errorf("%w: %w", ErrRuntimeFailure, err)
	}
	res.ContainerName = provisioned.ContainerName
	return res, nil
}
