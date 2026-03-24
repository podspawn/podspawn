package spawn

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func devSession(fake *runtime.FakeRuntime, store *state.FakeStore, projectDir string) *Session {
	return &Session{
		Username:    "dev",
		ProjectName: "myapp-a3f8",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     projectDir,
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
	}
}

func TestDevSessionCreatesContainerWithWorkspaceMount(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.WorkspaceMounts = []runtime.Mount{{
		Source: projectDir,
		Target: "/workspace/myapp",
	}}
	sess.WorkingDir = "/workspace/myapp"
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "ls /workspace/myapp")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.CreateCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(fake.CreateCalls))
	}

	opts := fake.CreateCalls[0]
	foundMount := false
	for _, m := range opts.Mounts {
		if m.Target == "/workspace/myapp" && m.Source == projectDir {
			foundMount = true
			break
		}
	}
	if !foundMount {
		t.Errorf("workspace mount not found in container opts: %+v", opts.Mounts)
	}
}

func TestDevSessionPortBindingsPassedToContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.PortBindings = []runtime.PortBinding{
		{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
		{ContainerPort: 3000, HostPort: 3001, Protocol: "tcp"},
	}
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo ports")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	opts := fake.CreateCalls[0]
	if len(opts.PortBindings) != 2 {
		t.Fatalf("expected 2 port bindings, got %d", len(opts.PortBindings))
	}
	if opts.PortBindings[0].ContainerPort != 8080 {
		t.Errorf("first port = %d, want 8080", opts.PortBindings[0].ContainerPort)
	}
	if opts.PortBindings[1].HostPort != 3001 {
		t.Errorf("second host port = %d, want 3001", opts.PortBindings[1].HostPort)
	}
}

func TestDevSessionWorkingDirUsedForShell(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.WorkingDir = "/workspace/myapp"
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "pwd")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The command exec should use the working dir
	// First exec is user setup, second is the command
	if len(fake.ExecCalls) < 2 {
		t.Fatalf("expected at least 2 exec calls, got %d", len(fake.ExecCalls))
	}
	cmdCall := fake.ExecCalls[1]
	if cmdCall.Opts.WorkingDir != "/workspace/myapp" {
		t.Errorf("working dir = %q, want /workspace/myapp", cmdCall.Opts.WorkingDir)
	}
}

func TestDevSessionEphemeralMode(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.Mode = "destroy-on-disconnect"
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo ephemeral")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetSession("dev", "myapp-a3f8")
	if got == nil {
		t.Fatal("session should exist after run")
	}
	if got.Mode != "destroy-on-disconnect" {
		t.Errorf("mode = %q, want destroy-on-disconnect", got.Mode)
	}
}

func TestDevSessionReattachPreservesContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-dev-myapp-a3f8"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "dev",
		Project:       "myapp-a3f8",
		ContainerID:   "existing-abc",
		ContainerName: "podspawn-dev-myapp-a3f8",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
		Mode:          "grace-period",
	})

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\non_start: echo reattach\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo test")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should NOT create a new container
	if len(fake.CreateCalls) != 0 {
		t.Fatalf("expected 0 create calls on reattach, got %d", len(fake.CreateCalls))
	}

	// Should still have the same session
	got, _ := store.GetSession("dev", "myapp-a3f8")
	if got == nil {
		t.Fatal("session should still exist")
	}
	if got.Connections != 2 {
		t.Errorf("connections = %d, want 2 (incremented on reattach)", got.Connections)
	}
}

func TestDevSessionExtendsResolution(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	projectDir := t.TempDir()

	// Create a base and a child that extends it
	base := "base: ubuntu:24.04\npackages: [git]\n"
	child := "extends: ./base.yaml\npackages: [npm]\n"

	if err := os.WriteFile(filepath.Join(projectDir, "base.yaml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte(child), 0644); err != nil {
		t.Fatal(err)
	}

	sess := devSession(fake, store, projectDir)
	sess.LockDir = t.TempDir()

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo extends")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should have built an image (extends resolved successfully)
	if len(fake.BuildCalls) != 1 {
		t.Fatalf("expected 1 build call (extends resolved), got %d", len(fake.BuildCalls))
	}
}
