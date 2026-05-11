package cmd

import (
	"context"
	"log/slog"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/state"
)

// setupNamedMachine is the cmd-side adapter that older tests (and the
// branch-workspace integration suite) drive directly. Production code paths
// in cmd/create.go and cmd/run.go go through ls.Service.Create themselves;
// this helper exists so the workspace-row contract those tests pin can be
// exercised without rebuilding the test fixtures around the new service
// surface.
//
// Returns (created, error): created=true when a fresh workspaces row was
// inserted. Mirrors the old signature exactly so test assertions keep
// working.
func setupNamedMachine(ctx context.Context, ls *localSession, name, projectName, branch string) (bool, error) {
	svc := ls.Service
	if svc == nil {
		svc = session.New(session.Options{
			SessionStore:   ls.Store,
			WorkspaceStore: ls.Store,
			Runtime:        ls.Session.Runtime,
			Audit:          ls.Session.Audit,
			ProjectsFile:   cfg.ProjectsFile,
			WorkspacesRoot: localWorkspacesRoot(),
		})
		ls.Service = svc
	}
	res, err := svc.Create(ctx, session.CreateRequest{
		User:            ls.Session.Username,
		Name:            name,
		ProjectName:     projectName,
		Branch:          branch,
		Provision:       false,
		SessionTemplate: ls.Session,
	})
	if err != nil {
		return false, err
	}
	return res.Created, nil
}

// configureSessionFromWorkspace is a cmd-side adapter for tests; it forwards
// to session.ApplyWorkspaceToTemplate, which is the canonical home for the
// template-from-workspace mutation.
func configureSessionFromWorkspace(ls *localSession, workspace *state.Workspace, removeWorkspaceOnDestroy bool) {
	session.ApplyWorkspaceToTemplate(ls.Session, workspace, removeWorkspaceOnDestroy)
}

// resolveProjectBranch is preserved because cmd/local_machine_test.go's
// fallback test pins its behavior against an unreachable repo. The
// equivalent logic also lives in internal/session/workspaces.go; this is
// a thin transitional wrapper, not a second source of truth.
func resolveProjectBranch(ctx context.Context, project config.ProjectConfig, cliBranch string) (string, error) {
	if cliBranch != "" {
		return cliBranch, nil
	}
	branch, err := podfile.DefaultBranch(ctx, project.Repo)
	if err != nil {
		slog.Warn("failed to resolve default branch, falling back to main", "repo", project.Repo, "error", err)
		return "main", nil
	}
	return branch, nil
}
