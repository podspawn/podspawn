//go:build integration

package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func mustDockerRuntime(t *testing.T) *DockerRuntime {
	t.Helper()
	rt, err := NewDockerRuntime()
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	return rt
}

func TestDockerExecEcho(t *testing.T) {
	rt := mustDockerRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name := "podspawn-integ-echo"

	t.Cleanup(func() {
		rt.RemoveContainer(context.Background(), name) //nolint:errcheck
	})

	_, err := rt.CreateContainer(ctx, ContainerOpts{
		Name:  name,
		Image: "ubuntu:24.04",
		Cmd:   []string{"sleep", "infinity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.StartContainer(ctx, name); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	exitCode, err := rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"echo", "hello from container"},
		TTY:    false,
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	got := strings.TrimSpace(stdout.String())
	if got != "hello from container" {
		t.Fatalf("expected 'hello from container', got %q", got)
	}
}

func TestDockerExitCodePropagation(t *testing.T) {
	rt := mustDockerRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name := "podspawn-integ-exitcode"

	t.Cleanup(func() {
		rt.RemoveContainer(context.Background(), name) //nolint:errcheck
	})

	_, err := rt.CreateContainer(ctx, ContainerOpts{
		Name:  name,
		Image: "ubuntu:24.04",
		Cmd:   []string{"sleep", "infinity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.StartContainer(ctx, name); err != nil {
		t.Fatal(err)
	}

	exitCode, err := rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"sh", "-c", "exit 42"},
		TTY:    false,
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}

func TestDockerContainerLifecycle(t *testing.T) {
	rt := mustDockerRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	name := "podspawn-integ-lifecycle"

	t.Cleanup(func() {
		rt.RemoveContainer(context.Background(), name) //nolint:errcheck
	})

	exists, _ := rt.ContainerExists(ctx, name)
	if exists {
		t.Fatal("container should not exist before creation")
	}

	_, err := rt.CreateContainer(ctx, ContainerOpts{
		Name:  name,
		Image: "ubuntu:24.04",
		Cmd:   []string{"sleep", "infinity"},
	})
	if err != nil {
		t.Fatal(err)
	}

	exists, _ = rt.ContainerExists(ctx, name)
	if !exists {
		t.Fatal("container should exist after creation")
	}

	if err := rt.StartContainer(ctx, name); err != nil {
		t.Fatal(err)
	}

	if err := rt.StopContainer(ctx, name, 5*time.Second); err != nil {
		t.Fatal(err)
	}

	if err := rt.RemoveContainer(ctx, name); err != nil {
		t.Fatal(err)
	}

	exists, _ = rt.ContainerExists(ctx, name)
	if exists {
		t.Fatal("container should not exist after removal")
	}
}
