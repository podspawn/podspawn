package spawn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func testSession(fake *runtime.FakeRuntime, username string) *Session {
	return &Session{
		Username: username,
		Runtime:  fake,
		Image:    "ubuntu:24.04",
		Shell:    "/bin/bash",
	}
}

func TestRunCreatesContainerWhenMissing(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")

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

	sess := testSession(fake, "deploy")
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

	sess := testSession(fake, "deploy")
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
	sess := testSession(fake, "deploy")

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

func TestRunInteractiveShellUsesConfiguredShell(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.Shell = "/bin/zsh"

	t.Setenv("SSH_ORIGINAL_COMMAND", "")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	call := fake.ExecCalls[0]
	if call.Opts.Cmd[0] != "/bin/zsh" {
		t.Errorf("expected /bin/zsh, got %v", call.Opts.Cmd)
	}
}

func TestRunUsesConfiguredImage(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.Image = "debian:12"

	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.CreateCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(fake.CreateCalls))
	}
	if fake.CreateCalls[0].Image != "debian:12" {
		t.Errorf("expected image debian:12, got %s", fake.CreateCalls[0].Image)
	}
}

func TestRunPassesResourceLimitsAndLabels(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.CPUs = 4.0
	sess.Memory = 8 * 1024 * 1024 * 1024

	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.CreateCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(fake.CreateCalls))
	}
	opts := fake.CreateCalls[0]
	if opts.CPUs != 4.0 {
		t.Errorf("CPUs = %f, want 4.0", opts.CPUs)
	}
	if opts.Memory != 8*1024*1024*1024 {
		t.Errorf("Memory = %d, want 8GiB", opts.Memory)
	}
	if opts.Labels["managed-by"] != "podspawn" {
		t.Errorf("missing managed-by label")
	}
	if opts.Labels["podspawn-user"] != "deploy" {
		t.Errorf("podspawn-user label = %q, want deploy", opts.Labels["podspawn-user"])
	}
}

func TestContainerNaming(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "ci-runner")
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, _ = sess.Run(context.Background())

	if _, ok := fake.Containers["podspawn-ci-runner"]; !ok {
		t.Error("container should be named podspawn-ci-runner")
	}
}

func TestContainerNamingWithProject(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.ProjectName = "backend"
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, _ = sess.Run(context.Background())

	if _, ok := fake.Containers["podspawn-deploy-backend"]; !ok {
		t.Error("container should be named podspawn-deploy-backend")
	}
}

func TestRunCreateContainerError(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.CreateErr = errors.New("image pull timeout")
	sess := testSession(fake, "deploy")
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
	sess := testSession(fake, "deploy")
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
	sess := testSession(fake, "deploy")
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	exitCode, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failed exec")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestRunWithStateCreatesSession(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()
	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "whoami")

	exitCode, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should exist in store")
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.Connections != 1 {
		t.Errorf("connections = %d, want 1", got.Connections)
	}
}

func TestRunReattachesViaState(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "existing-id",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})

	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should not have created a new container
	if len(fake.CreateCalls) != 0 {
		t.Errorf("expected 0 create calls, got %d", len(fake.CreateCalls))
	}

	got, _ := store.GetSession("deploy", "")
	if got.Connections != 2 {
		t.Errorf("connections = %d, want 2 (incremented)", got.Connections)
	}
}

func TestRunCancelsGracePeriod(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "existing-id",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   0,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})
	_ = store.SetGracePeriod("deploy", "", now.Add(30*time.Second))

	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetSession("deploy", "")
	if got.Status != "running" {
		t.Errorf("status = %q, want running (grace cancelled)", got.Status)
	}
	if got.GraceExpiry.Valid {
		t.Error("grace_expiry should be cleared")
	}
}

func TestDisconnectStartsGracePeriod(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "abc123",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})

	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		Mode:        "grace-period",
	}

	sess.Disconnect(context.Background())

	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should still exist")
	}
	if got.Status != "grace_period" {
		t.Errorf("status = %q, want grace_period", got.Status)
	}
	if !got.GraceExpiry.Valid {
		t.Error("grace_expiry should be set")
	}
	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Error("container should not be removed during grace period")
	}
}

func TestDisconnectDestroysInDestroyMode(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "abc123",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})

	sess := &Session{
		Username: "deploy",
		Runtime:  fake,
		Store:    store,
		LockDir:  t.TempDir(),
		Mode:     "destroy-on-disconnect",
	}

	sess.Disconnect(context.Background())

	got, _ := store.GetSession("deploy", "")
	if got != nil {
		t.Error("session should be deleted in destroy mode")
	}
	if _, ok := fake.Containers["podspawn-deploy"]; ok {
		t.Error("container should be removed in destroy mode")
	}
}

func TestDisconnectDecrementsButKeepsAlive(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "abc123",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   2,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})

	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		Mode:        "grace-period",
	}

	sess.Disconnect(context.Background())

	got, _ := store.GetSession("deploy", "")
	if got.Connections != 1 {
		t.Errorf("connections = %d, want 1", got.Connections)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running (still active)", got.Status)
	}
}

// SFTP detection tests

func TestIsSFTP(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"/usr/lib/openssh/sftp-server", true},
		{"/usr/libexec/openssh/sftp-server", true},
		{"internal-sftp", true},
		{"echo hello", false},
		{"cat sftp-server.log", false},
		{"scp -t /tmp/file", false},
		{"rsync --server --sender .", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isSFTP(tt.cmd)
		if got != tt.want {
			t.Errorf("isSFTP(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestReconcileExpiredGracePeriod(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	past := time.Now().Add(-10 * time.Second)
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		ContainerID:   "old-id",
		ContainerName: "podspawn-deploy",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   0,
		CreatedAt:     past,
		LastActivity:  past,
		MaxLifetime:   time.Now().Add(8 * time.Hour),
	})
	_ = store.SetGracePeriod("deploy", "", past)

	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Old container should be removed, new one created
	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Error("new container should exist")
	}
	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should exist after reconnect")
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.ContainerID == "old-id" {
		t.Error("should have a new container ID, not the old one")
	}
}

func TestDisconnectRemovesContainerWithoutStore(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true

	sess := testSession(fake, "deploy")
	sess.Disconnect(context.Background())

	if _, ok := fake.Containers["podspawn-deploy"]; ok {
		t.Error("container should be removed after disconnect (Phase 0 mode)")
	}
}
