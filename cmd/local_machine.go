package cmd

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
	"github.com/podspawn/podspawn/internal/state"
)

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

func loadRegisteredProject(name string) (config.ProjectConfig, error) {
	projects, err := config.LoadProjects(cfg.ProjectsFile)
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

func resolveMachineMode(baseMode string, project config.ProjectConfig, userOverrides *config.UserOverrides) string {
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

func configureSessionFromWorkspace(ls *localSession, workspace *state.Workspace, removeWorkspaceOnDestroy bool) {
	project := &config.ProjectConfig{
		Repo:      workspace.RepoURL,
		LocalPath: workspace.WorkspacePath,
		Mode:      workspace.Mode,
	}

	ls.Session.ProjectName = workspace.Name
	ls.Session.Project = project
	ls.Session.Workspace = workspace
	if workspace.Mode != "" {
		ls.Session.Mode = workspace.Mode
	}
	ls.Session.RequireProjectImage = false
	ls.Session.WorkspacePath = workspace.WorkspacePath
	ls.Session.RemoveWorkspaceOnDestroy = removeWorkspaceOnDestroy
	ls.Session.WorkspaceMounts = []runtime.Mount{{
		Source: workspace.WorkspacePath,
		Target: workspace.WorkspaceTarget,
	}}
	ls.Session.WorkingDir = workspace.WorkspaceTarget
}

func createMachineWorkspace(ctx context.Context, name, projectName, cliBranch, mode string, userOverrides *config.UserOverrides) (*state.Workspace, error) {
	project, err := loadRegisteredProject(projectName)
	if err != nil {
		return nil, err
	}

	branch, err := resolveProjectBranch(ctx, project, cliBranch)
	if err != nil {
		return nil, fmt.Errorf("resolving branch for %s: %w", projectName, err)
	}

	root := localWorkspacesRoot()
	if err := ensureLocalWorkspaceRoot(root); err != nil {
		return nil, fmt.Errorf("creating workspace root: %w", err)
	}

	workspacePath := filepath.Join(root, name)
	if err := podfile.CloneRepo(ctx, project.Repo, workspacePath, branch); err != nil {
		_ = os.RemoveAll(workspacePath)
		return nil, err
	}

	pf, branch, err := applyWorkspacePodfileBranch(ctx, workspacePath, branch, cliBranch)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return nil, fmt.Errorf("resolving workspace podfile: %w", err)
	}

	workspaceTarget := pf.Workspace
	if workspaceTarget == "" {
		workspaceTarget = "/workspace/" + projectName
	}

	return &state.Workspace{
		User:            os.Getenv("USER"),
		Name:            name,
		Project:         projectName,
		RepoURL:         project.Repo,
		Branch:          branch,
		Mode:            resolveMachineMode(mode, project, userOverrides),
		WorkspacePath:   workspacePath,
		WorkspaceTarget: workspaceTarget,
		CreatedAt:       time.Now().UTC(),
		Initialized:     false,
	}, nil
}

func setupNamedMachine(ctx context.Context, ls *localSession, name, projectName, branch string) (bool, error) {
	existing, err := ls.Store.GetWorkspace(ls.Session.Username, name)
	if err != nil {
		return false, fmt.Errorf("checking machine registry: %w", err)
	}

	if existing != nil {
		if projectName != "" && existing.Project != projectName {
			return false, fmt.Errorf("machine %q already uses project %q", name, existing.Project)
		}
		if branch != "" && existing.Branch != branch {
			return false, fmt.Errorf("machine %q already uses branch %q", name, existing.Branch)
		}
		// Retry path for a workspace that was preserved after a fatal
		// on_create failure. Clear the preserved flag (DB + in-memory)
		// before configureSessionFromWorkspace, so spawn's defensive
		// refusal does not fire on this legitimate retry. If on_create
		// fails again, spawn re-marks it preserved.
		if existing.State == state.WorkspaceStatePreserved {
			if err := ls.Store.UpdateWorkspaceState(ls.Session.Username, name, state.WorkspaceStateActive); err != nil {
				return false, fmt.Errorf("clearing preserved state on retry: %w", err)
			}
			existing.State = state.WorkspaceStateActive
		}
		configureSessionFromWorkspace(ls, existing, false)
		return false, nil
	}

	active, err := ls.Store.GetSession(ls.Session.Username, name)
	if err != nil {
		return false, fmt.Errorf("checking active sessions: %w", err)
	}
	if active != nil {
		if ls.Session.Runtime != nil {
			alive, err := ls.Session.Runtime.ContainerExists(ctx, active.ContainerName)
			if err != nil {
				return false, fmt.Errorf("checking stale session container: %w", err)
			}
			if !alive {
				if err := ls.Store.DeleteSession(ls.Session.Username, name); err != nil {
					return false, fmt.Errorf("cleaning stale session: %w", err)
				}
				active = nil
			}
		}
	}
	if active != nil {
		return false, fmt.Errorf("machine %q is already in use by a non-project session", name)
	}

	if projectName == "" {
		return false, nil
	}

	machine, err := createMachineWorkspace(ctx, name, projectName, branch, ls.Session.Mode, ls.Session.UserOverrides)
	if err != nil {
		return false, fmt.Errorf("creating workspace: %w", err)
	}
	machine.User = ls.Session.Username

	if err := ls.Store.CreateWorkspace(machine); err != nil {
		_ = os.RemoveAll(machine.WorkspacePath)
		return false, fmt.Errorf("recording machine: %w", err)
	}
	ls.Session.Audit.MachineCreate(machine.User, machine.Name, machine.Project, machine.Branch, machine.WorkspacePath)

	configureSessionFromWorkspace(ls, machine, false)
	return true, nil
}

func setupEphemeralProjectRun(ctx context.Context, ls *localSession, name, projectName, branch string) error {
	active, err := ls.Store.GetSession(ls.Session.Username, name)
	if err != nil {
		return fmt.Errorf("checking active sessions: %w", err)
	}
	if active != nil {
		return fmt.Errorf("machine %q is already running", name)
	}

	existing, err := ls.Store.GetWorkspace(ls.Session.Username, name)
	if err != nil {
		return fmt.Errorf("checking machine registry: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("machine %q already exists; use podspawn shell %s or podspawn create %s", name, name, name)
	}

	project, err := loadRegisteredProject(projectName)
	if err != nil {
		return err
	}

	effectiveBranch, err := resolveProjectBranch(ctx, project, branch)
	if err != nil {
		return fmt.Errorf("resolving branch for %s: %w", projectName, err)
	}

	root := localWorkspacesRoot()
	if err := ensureLocalWorkspaceRoot(root); err != nil {
		return fmt.Errorf("creating workspace root: %w", err)
	}

	workspacePath := filepath.Join(root, ".tmp-"+name+"-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := podfile.CloneRepo(ctx, project.Repo, workspacePath, effectiveBranch); err != nil {
		_ = os.RemoveAll(workspacePath)
		return fmt.Errorf("creating workspace: %w", err)
	}

	pf, _, err := applyWorkspacePodfileBranch(ctx, workspacePath, effectiveBranch, branch)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return fmt.Errorf("resolving workspace podfile: %w", err)
	}

	workspaceTarget := pf.Workspace
	if workspaceTarget == "" {
		workspaceTarget = "/workspace/" + projectName
	}

	ls.Session.Project = &config.ProjectConfig{
		Repo:      project.Repo,
		LocalPath: workspacePath,
		Mode:      project.Mode,
	}
	ls.Session.WorkspacePath = workspacePath
	ls.Session.RemoveWorkspaceOnDestroy = true
	ls.Session.WorkspaceMounts = []runtime.Mount{{
		Source: workspacePath,
		Target: workspaceTarget,
	}}
	ls.Session.WorkingDir = workspaceTarget

	return nil
}

func cleanupNewMachineOnFailure(ls *localSession) {
	if ls.Session.Workspace != nil {
		ls.Session.Audit.MachineDelete(
			ls.Session.Username,
			ls.Session.Workspace.Name,
			ls.Session.Workspace.Project,
			ls.Session.Workspace.Branch,
			ls.Session.Workspace.WorkspacePath,
			"create_failed",
		)
		_ = ls.Store.DeleteWorkspace(ls.Session.Username, ls.Session.Workspace.Name)
	}
	if ls.Session.WorkspacePath != "" {
		_ = os.RemoveAll(ls.Session.WorkspacePath)
	}
}
