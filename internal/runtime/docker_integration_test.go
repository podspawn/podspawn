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

func TestDockerExecAsNonRootUser(t *testing.T) {
	rt := mustDockerRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	name := "podspawn-integ-nonroot"

	t.Cleanup(func() {
		rt.RemoveContainer(context.Background(), name) //nolint:errcheck
	})

	_, err := rt.CreateContainer(ctx, ContainerOpts{
		Name:     name,
		Image:    "ubuntu:24.04",
		Hostname: "testbox",
		Cmd:      []string{"sleep", "infinity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.StartContainer(ctx, name); err != nil {
		t.Fatal(err)
	}

	userSetup := `
if id -u testuser >/dev/null 2>&1; then exit 0; fi
EXISTING=$(getent passwd 1000 2>/dev/null | cut -d: -f1 || true)
if [ -n "$EXISTING" ] && [ "$EXISTING" != "testuser" ]; then
  userdel "$EXISTING" 2>/dev/null || true
fi
if command -v useradd >/dev/null 2>&1; then
  groupadd --gid 1000 testuser 2>/dev/null || true
  useradd --uid 1000 --gid 1000 -m -s /bin/bash testuser
else
  echo 'testuser:x:1000:1000::/home/testuser:/bin/bash' >> /etc/passwd
  echo 'testuser:x:1000:' >> /etc/group
  mkdir -p /home/testuser && chown 1000:1000 /home/testuser
fi
mkdir -p /etc/sudoers.d
echo 'testuser ALL=(root) NOPASSWD:ALL' > /etc/sudoers.d/testuser
chmod 0440 /etc/sudoers.d/testuser
`
	exitCode, err := rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"sh", "-c", userSetup},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("user setup exec failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("user setup exited %d", exitCode)
	}

	// Verify: whoami as testuser
	var whoami bytes.Buffer
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"whoami"},
		User:   "testuser",
		Stdout: &whoami,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("whoami exited %d", exitCode)
	}
	if got := strings.TrimSpace(whoami.String()); got != "testuser" {
		t.Errorf("whoami = %q, want testuser", got)
	}

	// Verify: home directory exists and is owned by testuser
	var homeStat bytes.Buffer
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"stat", "-c", "%U", "/home/testuser"},
		User:   "testuser",
		Stdout: &homeStat,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("stat home dir exited %d", exitCode)
	}
	if got := strings.TrimSpace(homeStat.String()); got != "testuser" {
		t.Errorf("home dir owner = %q, want testuser", got)
	}

	// Verify: user can write files in their home
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"touch", "/home/testuser/testfile"},
		User:   "testuser",
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("touch in home dir exited %d", exitCode)
	}

	// Verify: sudoers.d config exists (sudo binary only available in Podfile-built images)
	var sudoersCheck bytes.Buffer
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"cat", "/etc/sudoers.d/testuser"},
		Stdout: &sudoersCheck,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatal("sudoers.d config should exist")
	}
	if got := strings.TrimSpace(sudoersCheck.String()); !strings.Contains(got, "NOPASSWD") {
		t.Errorf("sudoers config should contain NOPASSWD, got %q", got)
	}

	// Verify: hostname is set correctly
	var hostname bytes.Buffer
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"hostname"},
		User:   "testuser",
		Stdout: &hostname,
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(hostname.String()); got != "testbox" {
		t.Errorf("hostname = %q, want testbox", got)
	}

	// Verify: WorkingDir on exec
	var pwd bytes.Buffer
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:        []string{"pwd"},
		User:       "testuser",
		WorkingDir: "/home/testuser",
		Stdout:     &pwd,
		Stderr:     &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(pwd.String()); got != "/home/testuser" {
		t.Errorf("pwd = %q, want /home/testuser", got)
	}

	// Verify: idempotent (running setup again doesn't fail)
	exitCode, err = rt.Exec(ctx, name, ExecOpts{
		Cmd:    []string{"sh", "-c", userSetup},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("second user setup failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("second user setup exited %d (should be idempotent)", exitCode)
	}
}
