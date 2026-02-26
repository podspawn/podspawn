package spawn

import (
	"context"
	"errors"
	"testing"

	"github.com/podspawn/podspawn/internal/runtime"
)

func TestRunCreatesContainerWhenMissing(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := &Session{Username: "deploy", Runtime: fake}

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo hello")

	exitCode, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Fatal("expected container podspawn-deploy to be created")
	}

	if len(fake.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(fake.ExecCalls))
	}

	call := fake.ExecCalls[0]
	if call.ContainerID != "podspawn-deploy" {
		t.Errorf("exec on wrong container: %s", call.ContainerID)
	}
	if call.Opts.TTY {
		t.Error("command execution should not use TTY")
	}
	if call.Opts.Cmd[0] != "sh" || call.Opts.Cmd[1] != "-c" || call.Opts.Cmd[2] != "echo hello" {
		t.Errorf("unexpected command: %v", call.Opts.Cmd)
	}
}

func TestRunReattachesToExistingContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true

	sess := &Session{Username: "deploy", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(fake.ExecCalls))
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.ExitCode = 42

	sess := &Session{Username: "deploy", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "exit 42")

	exitCode, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}

func TestRunInteractiveShellUsesTTY(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := &Session{Username: "deploy", Runtime: fake}

	t.Setenv("SSH_ORIGINAL_COMMAND", "")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(fake.ExecCalls))
	}
	call := fake.ExecCalls[0]
	if !call.Opts.TTY {
		t.Error("interactive shell should use TTY")
	}
	if call.Opts.Cmd[0] != "/bin/bash" {
		t.Errorf("expected /bin/bash, got %v", call.Opts.Cmd)
	}
}

func TestContainerNaming(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := &Session{Username: "ci-runner", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, _ = sess.Run(context.Background())

	if _, ok := fake.Containers["podspawn-ci-runner"]; !ok {
		t.Error("container should be named podspawn-ci-runner")
	}
}

func TestRunCreateContainerError(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.CreateErr = errors.New("image pull timeout")
	sess := &Session{Username: "deploy", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	exitCode, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failed container creation")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if len(fake.ExecCalls) != 0 {
		t.Fatalf("no exec should happen when creation fails, got %d calls", len(fake.ExecCalls))
	}
}

func TestRunStartContainerError(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.StartErr = errors.New("port already bound")
	sess := &Session{Username: "deploy", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	exitCode, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failed container start")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if len(fake.ExecCalls) != 0 {
		t.Fatalf("no exec should happen when start fails, got %d calls", len(fake.ExecCalls))
	}
}

func TestRunExecError(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.ExecErr = errors.New("container stopped unexpectedly")
	sess := &Session{Username: "deploy", Runtime: fake}
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	exitCode, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failed exec")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestCleanupRemovesContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true

	sess := &Session{Username: "deploy", Runtime: fake}
	sess.Cleanup(context.Background())

	if _, ok := fake.Containers["podspawn-deploy"]; ok {
		t.Error("container should be removed after cleanup")
	}
}
