package session

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

// This file owns the workspace-resolution logic that used to live in
// cmd/local_machine.go. It is unexported on purpose: callers go through
// Service.Create. The one exception is ApplyWorkspaceToTemplate, which is
// the small piece of "configure a *spawn.Session from a workspace row" that
// cmd/shell.go needs after a Service.Inspect lookup.

func loadResolvedPodfile(projectDir string) (*podfile.Podfile, error) {
	raw, err := podfile.FindAndRead(projectDir)
	if err != nil {
		return nil, err
	}
	rawPf, err := podfile.ParseRaw(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return podfile.ResolveExtends(rawPf, projectDir)
}

func (s *Service) loadRegisteredProject(name string) (config.ProjectConfig, error) {
	if s.projectsFile == "" {
		return config.ProjectConfig{}, fmt.Errorf("no projects file configured")
	}
	projects, err := config.LoadProjects(s.projectsFile)
	if err != nil {
		return config.ProjectConfig{}, fmt.Errorf("loading projects: %w", err)
	}
	project, ok := projects[name]
	if !ok {
		return config.ProjectConfig{}, fmt.Errorf("project %q is not registered", name)
	}
	return project, nil
}

func resolveProjectBranch(ctx context.Context, project config.ProjectConfig, cliBranch string) (string, error) {
	if cliBranch != "" {
		return cliBranch, nil
	}
	if project.LocalPath != "" {
		pf, err := loadResolvedPodfile(project.LocalPath)
		if err == nil && pf.Branch != "" {
			return pf.Branch, nil
		}
	}
	branch, err := podfile.DefaultBranch(ctx, project.Repo)
	if err != nil {
		slog.Warn("failed to resolve default branch, falling back to main", "repo", project.Repo, "error", err)
		return "main", nil
	}
	return branch, nil
}

func resolveWorkspaceMode(baseMode string, project config.ProjectConfig, userOverrides *config.UserOverrides) string {
	mode := baseMode
	if userOverrides != nil && config.ValidMode(userOverrides.Mode) {
		mode = userOverrides.Mode
	}
	if config.ValidMode(project.Mode) {
		mode = project.Mode
	}
	return mode
}

func applyWorkspacePodfileBranch(ctx context.Context, workspacePath, branch, cliBranch string) (*podfile.Podfile, string, error) {
	pf, err := loadResolvedPodfile(workspacePath)
	if err != nil {
		return nil, "", err
	}
	if cliBranch != "" || pf.Branch == "" || pf.Branch == branch {
		return pf, branch, nil
	}
	if err := podfile.CheckoutBranch(ctx, workspacePath, pf.Branch); err != nil {
		return nil, "", err
	}
	pf, err = loadResolvedPodfile(workspacePath)
	if err != nil {
		return nil, "", err
	}
	return pf, pf.Branch, nil
}

// ApplyWorkspaceToTemplate mutates a *spawn.Session template so it points
// at the given workspace. It is the one piece of cmd/local_machine.go's
// configureSessionFromWorkspace that cmd code still needs directly: after
// a Service.Inspect lookup (e.g. in cmd/shell.go), cmd uses this to wire
// the workspace into the template before calling RunAndCleanup.
//
// Exporting this is the minimum cost of not making Inspect a mutator.
// removeWorkspaceOnDestroy mirrors the existing parameter and is false for
// the shell flow (workspace survives a shell exit) and true for ephemeral
// project runs (workspace is a temp clone).
func ApplyWorkspaceToTemplate(template *spawn.Session, workspace *state.Workspace, removeWorkspaceOnDestroy bool) {
	if template == nil || workspace == nil {
		return
	}
	template.ProjectName = workspace.Name
	template.Project = &config.ProjectConfig{
		Repo:      workspace.RepoURL,
		LocalPath: workspace.WorkspacePath,
		Mode:      workspace.Mode,
	}
	template.Workspace = workspace
	if workspace.Mode != "" {
		template.Mode = workspace.Mode
	}
	template.RequireProjectImage = false
	template.WorkspacePath = workspace.WorkspacePath
	template.RemoveWorkspaceOnDestroy = removeWorkspaceOnDestroy
	template.WorkspaceMounts = []runtime.Mount{{
		Source: workspace.WorkspacePath,
		Target: workspace.WorkspaceTarget,
	}}
	template.WorkingDir = workspace.WorkspaceTarget
}

// createNamedWorkspace clones the project repo into a per-workspace
// directory and returns the constructed *state.Workspace ready for
// CreateWorkspace. The row is not inserted here; the caller decides
// whether to persist (named create) or not (ephemeral run).
func (s *Service) createNamedWorkspace(ctx context.Context, user, name, projectName, cliBranch, mode string, userOverrides *config.UserOverrides) (*state.Workspace, error) {
	if s.workspacesRoot == "" {
		return nil, fmt.Errorf("no workspaces root configured")
	}
	project, err := s.loadRegisteredProject(projectName)
	if err != nil {
		return nil, err
	}
	branch, err := resolveProjectBranch(ctx, project, cliBranch)
	if err != nil {
		return nil, fmt.Errorf("resolving branch for %s: %w", projectName, err)
	}
	if err := os.MkdirAll(s.workspacesRoot, 0700); err != nil {
		return nil, fmt.Errorf("creating workspace root: %w", err)
	}
	workspacePath := filepath.Join(s.workspacesRoot, name)
	if err := podfile.CloneRepo(ctx, project.Repo, workspacePath, branch); err != nil {
		_ = os.RemoveAll(workspacePath)
		return nil, err
	}
	pf, branch, err := applyWorkspacePodfileBranch(ctx, workspacePath, branch, cliBranch)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return nil, fmt.Errorf("resolving workspace podfile: %w", err)
	}
	target := pf.Workspace
	if target == "" {
		target = "/workspace/" + projectName
	}
	return &state.Workspace{
		User:            user,
		Name:            name,
		Project:         projectName,
		RepoURL:         project.Repo,
		Branch:          branch,
		Mode:            resolveWorkspaceMode(mode, project, userOverrides),
		WorkspacePath:   workspacePath,
		WorkspaceTarget: target,
		CreatedAt:       s.now().UTC(),
		Initialized:     false,
	}, nil
}

// setupNamedFlow handles the create flow's workspace step: lookup or clone
// + persist + configure template. Mirrors the old setupNamedMachine.
// Returns (created, error) where created reports whether a fresh
// workspaces row was inserted.
func (s *Service) setupNamedFlow(ctx context.Context, req CreateRequest) (*state.Workspace, bool, error) {
	tpl := req.SessionTemplate

	existing, err := s.workspaces.GetWorkspace(req.User, req.Name)
	if err != nil {
		return nil, false, fmt.Errorf("checking workspace registry: %w", err)
	}
	if existing != nil {
		if req.ProjectName != "" && existing.Project != req.ProjectName {
			return nil, false, fmt.Errorf("%w: machine %q already uses project %q", ErrUnsafeTransition, req.Name, existing.Project)
		}
		if req.Branch != "" && existing.Branch != req.Branch {
			return nil, false, fmt.Errorf("%w: machine %q already uses branch %q", ErrUnsafeTransition, req.Name, existing.Branch)
		}
		// Preserved-workspace retry contract: the deferred doc flags this as
		// load-bearing. Clear preserved → active (DB + in-memory) before
		// wiring the template so spawn's defensive refusal does not fire
		// on this legitimate retry path.
		if existing.State == state.WorkspaceStatePreserved {
			if err := s.workspaces.UpdateWorkspaceState(req.User, req.Name, state.WorkspaceStateActive); err != nil {
				return nil, false, fmt.Errorf("clearing preserved state on retry: %w", err)
			}
			existing.State = state.WorkspaceStateActive
		}
		ApplyWorkspaceToTemplate(tpl, existing, false)
		return existing, false, nil
	}

	active, err := s.sessions.GetSession(req.User, req.Name)
	if err != nil {
		return nil, false, fmt.Errorf("checking active sessions: %w", err)
	}
	if active != nil && tpl.Runtime != nil {
		alive, err := tpl.Runtime.ContainerExists(ctx, active.ContainerName)
		if err != nil {
			return nil, false, fmt.Errorf("checking stale session container: %w", err)
		}
		if !alive {
			if err := s.sessions.DeleteSession(req.User, req.Name); err != nil {
				return nil, false, fmt.Errorf("cleaning stale session: %w", err)
			}
			active = nil
		}
	}
	if active != nil {
		return nil, false, fmt.Errorf("%w: machine %q is already in use by a non-project session", ErrUnsafeTransition, req.Name)
	}

	if req.ProjectName == "" {
		return nil, false, nil
	}

	ws, err := s.createNamedWorkspace(ctx, req.User, req.Name, req.ProjectName, req.Branch, tpl.Mode, tpl.UserOverrides)
	if err != nil {
		return nil, false, fmt.Errorf("creating workspace: %w", err)
	}
	if err := s.workspaces.CreateWorkspace(ws); err != nil {
		_ = os.RemoveAll(ws.WorkspacePath)
		return nil, false, fmt.Errorf("recording machine: %w", err)
	}
	s.audit.MachineCreate(ws.User, ws.Name, ws.Project, ws.Branch, ws.WorkspacePath)
	ApplyWorkspaceToTemplate(tpl, ws, false)
	return ws, true, nil
}

// setupEphemeralFlow handles the run flow with a project: clone to a temp
// path, configure template, do NOT insert a workspaces row. Mirrors the
// old setupEphemeralProjectRun. Returns no workspace because none is
// persisted.
func (s *Service) setupEphemeralFlow(ctx context.Context, req CreateRequest) error {
	tpl := req.SessionTemplate

	active, err := s.sessions.GetSession(req.User, req.Name)
	if err != nil {
		return fmt.Errorf("checking active sessions: %w", err)
	}
	if active != nil {
		return fmt.Errorf("%w: machine %q is already running", ErrUnsafeTransition, req.Name)
	}
	existing, err := s.workspaces.GetWorkspace(req.User, req.Name)
	if err != nil {
		return fmt.Errorf("checking workspace registry: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("%w: machine %q already exists; use podspawn shell %s or podspawn create %s", ErrUnsafeTransition, req.Name, req.Name, req.Name)
	}

	project, err := s.loadRegisteredProject(req.ProjectName)
	if err != nil {
		return err
	}
	branch, err := resolveProjectBranch(ctx, project, req.Branch)
	if err != nil {
		return fmt.Errorf("resolving branch for %s: %w", req.ProjectName, err)
	}

	if s.workspacesRoot == "" {
		return fmt.Errorf("no workspaces root configured")
	}
	if err := os.MkdirAll(s.workspacesRoot, 0700); err != nil {
		return fmt.Errorf("creating workspace root: %w", err)
	}
	workspacePath := filepath.Join(s.workspacesRoot, fmt.Sprintf(".tmp-%s-%d", req.Name, time.Now().UnixNano()))
	if err := podfile.CloneRepo(ctx, project.Repo, workspacePath, branch); err != nil {
		_ = os.RemoveAll(workspacePath)
		return fmt.Errorf("creating workspace: %w", err)
	}
	pf, _, err := applyWorkspacePodfileBranch(ctx, workspacePath, branch, req.Branch)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return fmt.Errorf("resolving workspace podfile: %w", err)
	}
	target := pf.Workspace
	if target == "" {
		target = "/workspace/" + req.ProjectName
	}

	tpl.Project = &config.ProjectConfig{
		Repo:      project.Repo,
		LocalPath: workspacePath,
		Mode:      project.Mode,
	}
	tpl.WorkspacePath = workspacePath
	tpl.RemoveWorkspaceOnDestroy = true
	tpl.WorkspaceMounts = []runtime.Mount{{
		Source: workspacePath,
		Target: target,
	}}
	tpl.WorkingDir = target
	return nil
}

// rollbackCreate removes a freshly-inserted workspace row plus its host
// directory after a non-hook Provision failure. Mirrors the existing
// cleanupNewMachineOnFailure path in cmd/create.go: hook failures leave
// the workspace preserved (handled inside spawn.runHooks); everything
// else rolls back.
func (s *Service) rollbackCreate(tpl *spawn.Session) {
	if tpl == nil {
		return
	}
	if tpl.Workspace != nil {
		s.audit.MachineDelete(
			tpl.Username,
			tpl.Workspace.Name,
			tpl.Workspace.Project,
			tpl.Workspace.Branch,
			tpl.Workspace.WorkspacePath,
			"create_failed",
		)
		_ = s.workspaces.DeleteWorkspace(tpl.Username, tpl.Workspace.Name)
	}
	if tpl.WorkspacePath != "" {
		_ = os.RemoveAll(tpl.WorkspacePath)
	}
}
