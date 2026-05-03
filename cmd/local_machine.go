package cmd

import (
	"bytes"
	"context"
	"fmt"
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
		return "", err
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

func configureSessionFromMachine(ls *localSession, machine *state.Machine, removeWorkspaceOnDestroy bool) {
	project := &config.ProjectConfig{
		Repo:      machine.RepoURL,
		LocalPath: machine.WorkspacePath,
		Mode:      machine.Mode,
	}

	ls.Session.ProjectName = machine.Name
	ls.Session.Project = project
	ls.Session.Machine = machine
	ls.Session.RequireProjectImage = false
	ls.Session.WorkspacePath = machine.WorkspacePath
	ls.Session.RemoveWorkspaceOnDestroy = removeWorkspaceOnDestroy
	ls.Session.WorkspaceMounts = []runtime.Mount{{
		Source: machine.WorkspacePath,
		Target: machine.WorkspaceTarget,
	}}
	ls.Session.WorkingDir = machine.WorkspaceTarget
}

func createMachineWorkspace(ctx context.Context, name, projectName, cliBranch, mode string, userOverrides *config.UserOverrides) (*state.Machine, error) {
	project, err := loadRegisteredProject(projectName)
	if err != nil {
		return nil, err
	}

	branch, err := resolveProjectBranch(ctx, project, cliBranch)
	if err != nil {
		return nil, fmt.Errorf("resolving branch for %s: %w", projectName, err)
	}

	root := localWorkspacesRoot()
	if err := os.MkdirAll(root, 0755); err != nil {
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

	return &state.Machine{
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
	existing, err := ls.Store.GetMachine(ls.Session.Username, name)
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
		configureSessionFromMachine(ls, existing, false)
		return false, nil
	}

	active, err := ls.Store.GetSession(ls.Session.Username, name)
	if err != nil {
		return false, fmt.Errorf("checking active sessions: %w", err)
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

	if err := ls.Store.CreateMachine(machine); err != nil {
		_ = os.RemoveAll(machine.WorkspacePath)
		return false, fmt.Errorf("recording machine: %w", err)
	}

	configureSessionFromMachine(ls, machine, false)
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

	existing, err := ls.Store.GetMachine(ls.Session.Username, name)
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
	if err := os.MkdirAll(root, 0755); err != nil {
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

	ls.Session.ProjectName = name
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
	if ls.Session.Machine != nil {
		_ = ls.Store.DeleteMachine(ls.Session.Username, ls.Session.Machine.Name)
	}
	if ls.Session.WorkspacePath != "" {
		_ = os.RemoveAll(ls.Session.WorkspacePath)
	}
}
