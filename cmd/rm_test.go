package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/state"
)

// denyPolicy is a tiny in-test deny-stub. captured is the last request.
type denyPolicy struct {
	reason   string
	captured policy.Request
}

func (d *denyPolicy) Evaluate(_ context.Context, req policy.Request) policy.Decision {
	d.captured = req
	return policy.Decision{Allow: false, Reason: d.reason}
}

func TestRmActorResolvesHumanVsOperator(t *testing.T) {
	if got := rmActor("alice", "alice"); got.Kind != identity.KindHuman || got.OSUser != "alice" {
		t.Errorf("rmActor(alice, alice) = %+v, want human:alice", got)
	}
	if got := rmActor("", "alice"); got.Kind != identity.KindHuman || got.OSUser != "alice" {
		t.Errorf("rmActor(\"\", alice) = %+v, want human:alice (empty invoker collapses to owner)", got)
	}
	if got := rmActor("root", "alice"); got.Kind != identity.KindOperator || got.OSUser != "root" {
		t.Errorf("rmActor(root, alice) = %+v, want operator:root", got)
	}
}

func TestRemoveLocalMachineHonorsPolicyDeny(t *testing.T) {
	base := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	root := t.TempDir()
	workspacePath := filepath.Join(root, "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := base.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		WorkspacePath: workspacePath,
		Branch:        "main",
		Mode:          "grace-period",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(root, "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	stub := &denyPolicy{reason: "no rms during freeze"}
	svc := session.New(session.Options{
		SessionStore:   base,
		WorkspaceStore: base,
		Runtime:        rt,
		Audit:          logger,
		Policy:         stub,
	})

	err = removeLocalWorkspace(context.Background(), svc, rt, base, logger, "root", "tenant", "auth-fix", false)
	_ = logger.Close()

	// Guardrail: ErrPolicyDenied must survive cmd-side wrapping and the
	// reason must reach the CLI-visible message.
	if !errors.Is(err, policy.ErrPolicyDenied) {
		t.Fatalf("errors.Is(err, policy.ErrPolicyDenied) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "no rms during freeze") {
		t.Errorf("CLI-visible error %q must include policy reason", err.Error())
	}

	// Gate must have seen the loaded workspace and the operator actor.
	if stub.captured.Workspace == nil || stub.captured.Workspace.Name != "auth-fix" {
		t.Errorf("gate did not see workspace: %+v", stub.captured.Workspace)
	}
	if stub.captured.Actor.Kind != identity.KindOperator || stub.captured.Actor.OSUser != "root" {
		t.Errorf("gate actor = %+v, want operator:root", stub.captured.Actor)
	}

	// Workspace must survive deny.
	got, _ := base.GetWorkspace("tenant", "auth-fix")
	if got == nil {
		t.Fatal("workspace deleted on deny; rm must not have run")
	}
	if _, statErr := os.Stat(workspacePath); statErr != nil {
		t.Fatalf("workspace dir removed on deny: %v", statErr)
	}

	// Audit log must contain exactly the policy.deny record — no
	// workspace.delete on a denied rm.
	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d: %q", len(lines), string(data))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != audit.EventPolicyDeny {
		t.Errorf("event = %q, want %q", entry["event"], audit.EventPolicyDeny)
	}
	if entry["op"] != string(policy.OpWorkspaceRemove) {
		t.Errorf("op = %q, want %q", entry["op"], policy.OpWorkspaceRemove)
	}
	if entry["actor"] != "operator:root" {
		t.Errorf("actor = %q, want operator:root", entry["actor"])
	}
}

func TestRemoveLocalMachineDeletesStoppedWorkspace(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := removeLocalWorkspace(context.Background(), nil, rt, store, nil, "tenant", "tenant", "auth-fix", false); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}

	got, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("machine should be deleted, got %+v", got)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace should be removed, stat err = %v", err)
	}
}

func TestRemoveLocalMachineRefusesRunningMachineWithoutForce(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(&state.Session{
		User:          "tenant",
		Project:       "auth-fix",
		ContainerID:   "ctr-1",
		ContainerName: "podspawn-tenant-auth-fix",
		Image:         "ubuntu:24.04",
		Status:        state.StatusRunning,
		Connections:   1,
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
		MaxLifetime:   time.Now().UTC().Add(8 * time.Hour),
		Mode:          "grace-period",
	}); err != nil {
		t.Fatal(err)
	}
	rt.Containers["podspawn-tenant-auth-fix"] = true

	err := removeLocalWorkspace(context.Background(), nil, rt, store, nil, "tenant", "tenant", "auth-fix", false)
	if err == nil {
		t.Fatal("expected removeLocalWorkspace() to fail without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %q, want --force hint", err)
	}
}

func TestRemoveLocalMachineForceRemovesRunningMachine(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	workspacePath := filepath.Join(t.TempDir(), "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(&state.Session{
		User:          "tenant",
		Project:       "auth-fix",
		ContainerID:   "ctr-1",
		ContainerName: "podspawn-tenant-auth-fix",
		Image:         "ubuntu:24.04",
		Status:        state.StatusRunning,
		Connections:   1,
		CreatedAt:     time.Now().UTC(),
		LastActivity:  time.Now().UTC(),
		MaxLifetime:   time.Now().UTC().Add(8 * time.Hour),
		Mode:          "grace-period",
	}); err != nil {
		t.Fatal(err)
	}
	rt.Containers["podspawn-tenant-auth-fix"] = true

	if err := removeLocalWorkspace(context.Background(), nil, rt, store, nil, "tenant", "tenant", "auth-fix", true); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}

	gotMachine, err := store.GetWorkspace("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if gotMachine != nil {
		t.Fatalf("machine should be deleted, got %+v", gotMachine)
	}
	gotSession, err := store.GetSession("tenant", "auth-fix")
	if err != nil {
		t.Fatal(err)
	}
	if gotSession != nil {
		t.Fatalf("session should be deleted, got %+v", gotSession)
	}
	if _, exists := rt.Containers["podspawn-tenant-auth-fix"]; exists {
		t.Fatal("running container should be removed")
	}
}

func TestRemoveLocalMachineRejectsEphemeralNames(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()

	err := removeLocalWorkspace(context.Background(), nil, rt, store, nil, "tenant", "tenant", ".tmp-auth-fix-123", false)
	if err == nil {
		t.Fatal("expected removeLocalWorkspace() to reject ephemeral names")
	}
	if !strings.Contains(err.Error(), ".tmp-") {
		t.Fatalf("error = %q, want .tmp- hint", err)
	}
}

func TestRemoveLocalMachineLogsAuditEvent(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	root := t.TempDir()
	workspacePath := filepath.Join(root, "auth-fix")
	if err := os.MkdirAll(workspacePath, 0700); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateWorkspace(&state.Workspace{
		User:          "tenant",
		Name:          "auth-fix",
		Project:       "backend",
		RepoURL:       "https://github.com/podspawn/example.git",
		Branch:        "main",
		Mode:          "grace-period",
		WorkspacePath: workspacePath,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(root, "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := removeLocalWorkspace(context.Background(), nil, rt, store, logger, "tenant", "tenant", "auth-fix", false); err != nil {
		t.Fatalf("removeLocalWorkspace() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != "workspace.delete" {
		t.Fatalf("event = %q, want workspace.delete", entry["event"])
	}
	if entry["project"] != "backend" {
		t.Fatalf("project = %q, want backend", entry["project"])
	}
	if entry["branch"] != "main" {
		t.Fatalf("branch = %q, want main", entry["branch"])
	}
	if entry["reason"] != "rm" {
		t.Fatalf("reason = %q, want rm", entry["reason"])
	}
	if entry["actor"] != "human:tenant" {
		t.Fatalf("actor = %q, want human:tenant", entry["actor"])
	}
	if id, _ := entry["workspace_id"].(string); id == "" {
		t.Fatalf("workspace_id missing on workspace.delete event: %v", entry["workspace_id"])
	}
}
