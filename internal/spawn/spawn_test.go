package spawn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
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

	if _, err := sess.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, ok := fake.Containers["podspawn-ci-runner"]; !ok {
		t.Error("container should be named podspawn-ci-runner")
	}
}

func TestContainerNamingWithProject(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.ProjectName = "backend"
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	if _, err := sess.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

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

func TestRunWithProjectFailsWithoutPrebuiltImage(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
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
	if err == nil {
		t.Fatal("expected error for missing pre-built image")
	}
	// Should guide user to run update-project
	errMsg := err.Error()
	if !strings.Contains(errMsg, "not built") || !strings.Contains(errMsg, "update-project") {
		t.Errorf("error should guide user, got: %s", errMsg)
	}
}

func TestOnStartRunsOnReattach(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy-backend"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		Project:       "backend",
		ContainerID:   "existing-id",
		ContainerName: "podspawn-deploy-backend",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
	})

	projectDir := t.TempDir()
	podfileContent := "base: ubuntu:24.04\non_start: echo welcome\non_create: echo first-time\n"
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), []byte(podfileContent), 0644); err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
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

	// Should NOT have created a new container (reattach)
	if len(fake.CreateCalls) != 0 {
		t.Errorf("expected 0 create calls (reattach), got %d", len(fake.CreateCalls))
	}

	// Should have exec calls: on_start hook + the actual command
	// on_create should NOT run (not a new container)
	var hookCmds []string
	for _, call := range fake.ExecCalls {
		if len(call.Opts.Cmd) >= 3 && call.Opts.Cmd[0] == "sh" && call.Opts.Cmd[1] == "-c" {
			hookCmds = append(hookCmds, call.Opts.Cmd[2])
		}
	}

	foundOnStart := false
	foundOnCreate := false
	for _, cmd := range hookCmds {
		if strings.Contains(cmd, "welcome") {
			foundOnStart = true
		}
		if strings.Contains(cmd, "first-time") {
			foundOnCreate = true
		}
	}
	if !foundOnStart {
		t.Error("on_start should run on reattach")
	}
	if foundOnCreate {
		t.Error("on_create should NOT run on reattach")
	}
}

func TestDisconnectCleansUpServices(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy-backend"] = true
	fake.Containers["svc-pg"] = true
	fake.Containers["svc-redis"] = true
	store := state.NewFakeStore()

	now := time.Now().UTC()
	_ = store.CreateSession(&state.Session{
		User:          "deploy",
		Project:       "backend",
		ContainerID:   "abc123",
		ContainerName: "podspawn-deploy-backend",
		Image:         "ubuntu:24.04",
		Status:        "running",
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
		NetworkID:     "net-123",
		ServiceIDs:    "svc-pg,svc-redis",
	})

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Runtime:     fake,
		Store:       store,
		LockDir:     t.TempDir(),
		Mode:        "destroy-on-disconnect",
	}

	sess.Disconnect(context.Background())

	got, _ := store.GetSession("deploy", "backend")
	if got != nil {
		t.Error("session should be deleted")
	}
	if _, ok := fake.Containers["podspawn-deploy-backend"]; ok {
		t.Error("dev container should be removed")
	}
	if _, ok := fake.Containers["svc-pg"]; ok {
		t.Error("postgres service should be removed")
	}
	if _, ok := fake.Containers["svc-redis"]; ok {
		t.Error("redis service should be removed")
	}
	if len(fake.RemoveNetworkCalls) != 1 || fake.RemoveNetworkCalls[0] != "net-123" {
		t.Errorf("network should be removed, got calls: %v", fake.RemoveNetworkCalls)
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

func TestStartContainerFailureCleansUpContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.StartErr = errors.New("port conflict")
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
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from StartContainer failure")
	}

	// Container should have been cleaned up, not left orphaned
	if _, ok := fake.Containers["podspawn-deploy"]; ok {
		t.Error("container should be removed after StartContainer failure")
	}

	// No session should exist in the store
	got, _ := store.GetSession("deploy", "")
	if got != nil {
		t.Error("no session should be recorded when StartContainer fails")
	}
}

func TestApplyUserOverridesEnvMerge(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	projectDir := t.TempDir()
	podfileContent := []byte("base: ubuntu:24.04\nenv:\n  EDITOR: vim\n  DB_HOST: localhost\n")
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), podfileContent, 0644); err != nil {
		t.Fatal(err)
	}
	fake.Images[podfile.ComputeTag("backend", podfileContent)] = true

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
		UserOverrides: &config.UserOverrides{
			Env: map[string]string{
				"EDITOR":    "nvim",
				"EXTRA_VAR": "from-override",
			},
		},
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

	// Check that env vars were passed to the container
	opts := fake.CreateCalls[0]
	envMap := make(map[string]string)
	for _, e := range opts.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// User override should win over Podfile for EDITOR
	if envMap["EDITOR"] != "nvim" {
		t.Errorf("EDITOR = %q, want nvim (user override should win)", envMap["EDITOR"])
	}
	// Podfile value should survive if not overridden
	if envMap["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST = %q, want localhost (podfile value)", envMap["DB_HOST"])
	}
	// Override-only var should be present
	if envMap["EXTRA_VAR"] != "from-override" {
		t.Errorf("EXTRA_VAR = %q, want from-override", envMap["EXTRA_VAR"])
	}
}

func TestEnvExpansionIncludesProject(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	projectDir := t.TempDir()
	podfileContent := []byte("base: ubuntu:24.04\nenv:\n  GREETING: \"hello ${PODSPAWN_USER} on ${PODSPAWN_PROJECT}\"\n")
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), podfileContent, 0644); err != nil {
		t.Fatal(err)
	}
	fake.Images[podfile.ComputeTag("backend", podfileContent)] = true

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
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

	opts := fake.CreateCalls[0]
	found := false
	for _, e := range opts.Env {
		if e == "GREETING=hello deploy on backend" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GREETING with expanded user+project, got env: %v", opts.Env)
	}
}

func TestAgentForwardingCreatesSymlinkAndPassesEnv(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "agent.12345")
	// Create a dummy file to represent the SSH agent socket
	if err := os.WriteFile(sockPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SSH_AUTH_SOCK", sockPath)

	sess := &Session{Username: "deploy"}
	// Override agentHostDir by using a temp dir for the user
	t.Setenv("TMPDIR", tmpDir)

	env := sess.prepareAgentForwarding()

	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(env))
	}
	// Socket name includes PID to prevent concurrent session races
	expectedSock := fmt.Sprintf("%s/agent-%d.sock", containerAgentDir, os.Getpid())
	if env[0] != "SSH_AUTH_SOCK="+expectedSock {
		t.Errorf("env = %q, want SSH_AUTH_SOCK=%s", env[0], expectedSock)
	}

	// Verify symlink was created with PID-based name
	agentDir := sess.agentHostDir()
	sockName := fmt.Sprintf("agent-%d.sock", os.Getpid())
	linkTarget, err := os.Readlink(filepath.Join(agentDir, sockName))
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if linkTarget != sockPath {
		t.Errorf("symlink target = %q, want %q", linkTarget, sockPath)
	}
}

func TestAgentForwardingNoopWithoutSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	sess := &Session{Username: "deploy"}
	env := sess.prepareAgentForwarding()

	if env != nil {
		t.Errorf("expected nil env when SSH_AUTH_SOCK is empty, got %v", env)
	}
}

func TestAgentMountSkippedWithoutSocket(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")

	t.Setenv("SSH_ORIGINAL_COMMAND", "echo hello")
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	opts := fake.CreateCalls[0]
	for _, m := range opts.Mounts {
		if m.Target == containerAgentDir {
			t.Error("agent mount should not be included when SSH_AUTH_SOCK is empty")
		}
	}
}

func TestAgentEnvPassedToExec(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "agent.99")
	if err := os.WriteFile(sockPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	t.Setenv("SSH_ORIGINAL_COMMAND", "git clone repo")
	t.Setenv("SSH_AUTH_SOCK", sockPath)
	t.Setenv("TMPDIR", tmpDir)

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(fake.ExecCalls))
	}
	execEnv := fake.ExecCalls[0].Opts.Env
	found := false
	for _, e := range execEnv {
		if strings.HasPrefix(e, "SSH_AUTH_SOCK="+containerAgentDir+"/agent-") {
			found = true
		}
	}
	if !found {
		t.Errorf("SSH_AUTH_SOCK not passed to exec, got env: %v", execEnv)
	}
}

func TestSecurityOptsPassedToContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.Security = config.SecurityConfig{
		CapDrop:      []string{"ALL"},
		CapAdd:       []string{"CHOWN", "SETUID", "SETGID"},
		NoNewPrivs:   true,
		PidsLimit:    256,
		ReadonlyRoot: true,
		Tmpfs: map[string]string{
			"/tmp": "rw,noexec,nosuid,size=512m",
		},
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.CreateCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(fake.CreateCalls))
	}
	opts := fake.CreateCalls[0]

	if len(opts.CapDrop) != 1 || opts.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v, want [ALL]", opts.CapDrop)
	}
	if len(opts.CapAdd) != 3 {
		t.Errorf("CapAdd = %v, want 3 items", opts.CapAdd)
	}
	if len(opts.SecurityOpt) != 1 || opts.SecurityOpt[0] != "no-new-privileges:true" {
		t.Errorf("SecurityOpt = %v, want [no-new-privileges:true]", opts.SecurityOpt)
	}
	if opts.PidsLimit != 256 {
		t.Errorf("PidsLimit = %d, want 256", opts.PidsLimit)
	}
	if !opts.ReadonlyRootfs {
		t.Error("ReadonlyRootfs should be true")
	}
	if opts.Tmpfs["/tmp"] != "rw,noexec,nosuid,size=512m" {
		t.Errorf("Tmpfs = %v, want /tmp entry", opts.Tmpfs)
	}
}

func TestPerUserNetworkCreated(t *testing.T) {
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
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should have created a network
	if len(fake.CreateNetworkCalls) != 1 {
		t.Fatalf("expected 1 network creation, got %d", len(fake.CreateNetworkCalls))
	}
	if fake.CreateNetworkCalls[0] != "podspawn-deploy-net" {
		t.Errorf("network name = %q, want podspawn-deploy-net", fake.CreateNetworkCalls[0])
	}

	// Container should be on that network
	opts := fake.CreateCalls[0]
	if opts.NetworkID == "" {
		t.Error("container should be attached to a network")
	}

	// Session should record the network ID
	got, _ := store.GetSession("deploy", "")
	if got.NetworkID == "" {
		t.Error("session should record network ID")
	}
}

func TestPerUserNetworkWithProject(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	projectDir := t.TempDir()
	podfileContent := []byte("base: ubuntu:24.04\n")
	if err := os.WriteFile(filepath.Join(projectDir, "podfile.yaml"), podfileContent, 0644); err != nil {
		t.Fatal(err)
	}
	fake.Images[podfile.ComputeTag("backend", podfileContent)] = true

	sess := &Session{
		Username:    "deploy",
		ProjectName: "backend",
		Project: &config.ProjectConfig{
			LocalPath: projectDir,
		},
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

	if fake.CreateNetworkCalls[0] != "podspawn-deploy-backend-net" {
		t.Errorf("network name = %q, want podspawn-deploy-backend-net", fake.CreateNetworkCalls[0])
	}
}

func TestGVisorRuntimePassedToContainer(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	sess.Security = config.SecurityConfig{
		RuntimeName: "runsc",
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	opts := fake.CreateCalls[0]
	if opts.RuntimeName != "runsc" {
		t.Errorf("RuntimeName = %q, want runsc", opts.RuntimeName)
	}
}

func TestApplyUserOverridesWithoutPodfile(t *testing.T) {
	sess := &Session{
		Username: "deploy",
		Image:    "ubuntu:24.04",
		Shell:    "/bin/bash",
		CPUs:     2.0,
		Memory:   1024,
		UserOverrides: &config.UserOverrides{
			Image:  "node:22",
			Shell:  "/bin/zsh",
			CPUs:   4.0,
			Memory: "8g",
		},
	}

	sess.applyUserOverrides()

	if sess.Image != "node:22" {
		t.Errorf("Image = %q, want node:22", sess.Image)
	}
	if sess.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want /bin/zsh", sess.Shell)
	}
	if sess.CPUs != 4.0 {
		t.Errorf("CPUs = %f, want 4.0", sess.CPUs)
	}
	// Memory parsed from "8g"
	if sess.Memory != 8*1024*1024*1024 {
		t.Errorf("Memory = %d, want 8GiB", sess.Memory)
	}
}

func TestDisconnectLockAcquisitionFailure(t *testing.T) {
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
		LockDir:     "/dev/null/nope",
		GracePeriod: 60 * time.Second,
		Mode:        "grace-period",
	}

	// Should not panic; lock failure means early return, container survives
	sess.Disconnect(context.Background())

	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should still exist when lock acquisition fails")
	}
	if got.Connections != 1 {
		t.Errorf("connections = %d, want 1 (unchanged)", got.Connections)
	}
	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Error("container should survive lock failure")
	}
}

func TestDisconnectUpdateConnectionsError(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	store := state.NewFakeStore()

	// No session in store for "deploy", so UpdateConnections will fail
	sess := &Session{
		Username:    "deploy",
		Runtime:     fake,
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		Mode:        "destroy-on-disconnect",
	}

	// UpdateConnections returns error for missing session; should not panic
	sess.Disconnect(context.Background())

	// Container should be untouched since we bailed out on the error path
	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Error("container should survive when UpdateConnections fails")
	}
}

func TestDisconnectSessionNotFoundForCleanup(t *testing.T) {
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

	// Delete the session before Disconnect tries to look it up for cleanup
	_ = store.DeleteSession("deploy", "")

	// Should not panic; GetSession returns nil, early return with warning
	sess.Disconnect(context.Background())

	// Container should still exist because we couldn't find the session to clean up
	if _, ok := fake.Containers["podspawn-deploy"]; !ok {
		t.Error("container should survive when session is missing at cleanup time")
	}
}

func TestRunAndCleanup(t *testing.T) {
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

	exitCode := sess.RunAndCleanup(context.Background())
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	// Session should still exist (grace period mode), connections decremented to 0
	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should exist in grace period mode after RunAndCleanup")
	}
	if got.Connections != 0 {
		t.Errorf("connections = %d, want 0 after disconnect", got.Connections)
	}
	if got.Status != "grace_period" {
		t.Errorf("status = %q, want grace_period", got.Status)
	}
}

// errorOnExistsRuntime wraps FakeRuntime to return an error from ContainerExists.
type errorOnExistsRuntime struct {
	*runtime.FakeRuntime
	existsErr error
}

func (r *errorOnExistsRuntime) ContainerExists(_ context.Context, _ string) (bool, error) {
	return false, r.existsErr
}

func TestContainerExistsErrorOnReattach(t *testing.T) {
	fake := runtime.NewFakeRuntime()
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

	errRT := &errorOnExistsRuntime{
		FakeRuntime: fake,
		existsErr:   errors.New("docker daemon unreachable"),
	}

	sess := &Session{
		Username:    "deploy",
		Runtime:     errRT,
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

	// Session should NOT be deleted; we assume alive on ContainerExists error
	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should not be deleted when ContainerExists returns error")
	}
	if got.Connections != 2 {
		t.Errorf("connections = %d, want 2 (reattach should proceed)", got.Connections)
	}

	// Should not have created a new container
	if len(fake.CreateCalls) != 0 {
		t.Errorf("expected 0 create calls (assumed alive), got %d", len(fake.CreateCalls))
	}
}

func TestPerUserNetworkCleanupOnDisconnect(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	fake.Containers["podspawn-deploy"] = true
	fake.Networks["net-deploy-1"] = true
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
		NetworkID:     "net-deploy-1",
	})

	sess := &Session{
		Username: "deploy",
		Runtime:  fake,
		Store:    store,
		LockDir:  t.TempDir(),
		Mode:     "destroy-on-disconnect",
	}

	sess.Disconnect(context.Background())

	if _, ok := fake.Containers["podspawn-deploy"]; ok {
		t.Error("container should be removed")
	}
	if _, ok := fake.Networks["net-deploy-1"]; ok {
		t.Error("network should be removed on destroy-on-disconnect")
	}
	if len(fake.RemoveNetworkCalls) != 1 || fake.RemoveNetworkCalls[0] != "net-deploy-1" {
		t.Errorf("RemoveNetwork calls = %v, want [net-deploy-1]", fake.RemoveNetworkCalls)
	}
}

func TestSecurityOptsDefaultsEmpty(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	sess := testSession(fake, "deploy")
	// Security is zero-value config.SecurityConfig{}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	opts := fake.CreateCalls[0]
	if len(opts.CapDrop) != 0 {
		t.Errorf("CapDrop = %v, want empty", opts.CapDrop)
	}
	if len(opts.CapAdd) != 0 {
		t.Errorf("CapAdd = %v, want empty", opts.CapAdd)
	}
	if len(opts.SecurityOpt) != 0 {
		t.Errorf("SecurityOpt = %v, want empty", opts.SecurityOpt)
	}
	if opts.PidsLimit != 0 {
		t.Errorf("PidsLimit = %d, want 0", opts.PidsLimit)
	}
	if opts.ReadonlyRootfs {
		t.Error("ReadonlyRootfs should be false by default")
	}
	if len(opts.Tmpfs) != 0 {
		t.Errorf("Tmpfs = %v, want empty", opts.Tmpfs)
	}
	if opts.RuntimeName != "" {
		t.Errorf("RuntimeName = %q, want empty", opts.RuntimeName)
	}
}

func TestMaxPerUserEnforcement(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	now := time.Now().UTC()
	// Pre-create 2 sessions for this user (at the limit)
	for _, proj := range []string{"api", "web"} {
		fake.Containers["podspawn-deploy-"+proj] = true
		_ = store.CreateSession(&state.Session{
			User: "deploy", Project: proj, ContainerID: "podspawn-deploy-" + proj,
			ContainerName: "podspawn-deploy-" + proj, Image: "ubuntu:24.04",
			Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		})
	}

	sess := &Session{
		Username:    "deploy",
		ProjectName: "third",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
		MaxPerUser:  2,
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when max containers per user exceeded")
	}
	if !strings.Contains(err.Error(), "max 2") {
		t.Errorf("error should mention limit, got: %s", err.Error())
	}
	if len(fake.CreateCalls) != 0 {
		t.Error("should not create container when limit exceeded")
	}
}

func TestMaxPerUserZeroMeansUnlimited(t *testing.T) {
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
		MaxPerUser:  0, // unlimited
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatalf("MaxPerUser=0 should mean unlimited, got: %v", err)
	}
}

func TestResolveProjectNetworkCreated(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	// Default image session (no project), verify network is still created
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

	if len(fake.CreateNetworkCalls) != 1 {
		t.Fatalf("expected 1 network creation for default image session, got %d", len(fake.CreateNetworkCalls))
	}
	if fake.CreateNetworkCalls[0] != "podspawn-deploy-net" {
		t.Errorf("network name = %q, want podspawn-deploy-net", fake.CreateNetworkCalls[0])
	}

	// Session should record the network ID
	got, _ := store.GetSession("deploy", "")
	if got == nil {
		t.Fatal("session should exist")
	}
	if got.NetworkID == "" {
		t.Error("session should record network ID even for default image sessions")
	}

	// Container should reference that network
	opts := fake.CreateCalls[0]
	if opts.NetworkID == "" {
		t.Error("container should be attached to network for default image session")
	}
}

func TestMaxPerUserAllowsReattach(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	now := time.Now().UTC()
	// User already has 2 sessions (at the limit)
	for _, proj := range []string{"api", "web"} {
		fake.Containers["podspawn-deploy-"+proj] = true
		_ = store.CreateSession(&state.Session{
			User: "deploy", Project: proj, ContainerID: "podspawn-deploy-" + proj,
			ContainerName: "podspawn-deploy-" + proj, Image: "ubuntu:24.04",
			Status: "running", Connections: 1,
			CreatedAt: now, LastActivity: now, MaxLifetime: now.Add(8 * time.Hour),
		})
	}

	// Reattach to "api" (already exists) should work despite being at the limit
	sess := &Session{
		Username:    "deploy",
		ProjectName: "api",
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		GracePeriod: 60 * time.Second,
		MaxLifetime: 8 * time.Hour,
		Mode:        "grace-period",
		MaxPerUser:  2,
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "id")

	_, err := sess.Run(context.Background())
	if err != nil {
		t.Fatalf("reattach should succeed even at max session limit, got: %v", err)
	}

	// Should NOT have created a new container
	if len(fake.CreateCalls) != 0 {
		t.Errorf("expected 0 create calls (reattach), got %d", len(fake.CreateCalls))
	}

	// Connection count should be incremented
	got, _ := store.GetSession("deploy", "api")
	if got.Connections != 2 {
		t.Errorf("connections = %d, want 2 after reattach", got.Connections)
	}
}
