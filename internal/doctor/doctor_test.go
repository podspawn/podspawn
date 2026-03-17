package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckDirExistsPass(t *testing.T) {
	dir := t.TempDir()
	r := checkDirExists(dir, "test dir")
	if r.Status != Pass {
		t.Errorf("expected Pass for existing dir, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckDirExistsFail(t *testing.T) {
	r := checkDirExists("/nonexistent/path/xyz", "test dir")
	if r.Status != Fail {
		t.Errorf("expected Fail for missing dir, got %v", r.Status)
	}
}

func TestCheckDirExistsNotADir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, []byte("x"), 0644)
	r := checkDirExists(f, "test dir")
	if r.Status != Fail {
		t.Errorf("expected Fail for file-not-dir, got %v", r.Status)
	}
}

func TestCheckKeyDirNormalPerms(t *testing.T) {
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "keys")
	_ = os.MkdirAll(keyDir, 0755) // readable but not world-writable is fine

	cfg := CheckConfig{KeyDir: keyDir}
	r := cfg.checkKeyDir(context.Background())
	if r.Status != Pass {
		t.Errorf("expected Pass for 0755 key dir, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckKeyDirCorrectPerms(t *testing.T) {
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "keys")
	_ = os.MkdirAll(keyDir, 0700)

	cfg := CheckConfig{KeyDir: keyDir}
	r := cfg.checkKeyDir(context.Background())
	if r.Status != Pass {
		t.Errorf("expected Pass for 0700 key dir, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckStateDirWritable(t *testing.T) {
	dir := t.TempDir()
	cfg := CheckConfig{StateDir: dir}
	r := cfg.checkStateDir(context.Background())
	if r.Status != Pass {
		t.Errorf("expected Pass for writable dir, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckAuthKeysCommandNotConfigured(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sshd_config")
	_ = os.WriteFile(f, []byte("Port 22\nPasswordAuthentication yes\n"), 0644)

	cfg := CheckConfig{SSHDConfigPath: f}
	r := cfg.checkAuthKeysCommand(context.Background())
	if r.Status != Warn {
		t.Errorf("expected Warn when not configured, got %v", r.Status)
	}
}

func TestCheckAuthKeysCommandConfigured(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sshd_config")
	content := "Port 22\nAuthorizedKeysCommand /usr/local/bin/podspawn auth-keys %u\nAuthorizedKeysCommandUser root\n"
	_ = os.WriteFile(f, []byte(content), 0644)

	cfg := CheckConfig{SSHDConfigPath: f}
	r := cfg.checkAuthKeysCommand(context.Background())
	if r.Status != Pass {
		t.Errorf("expected Pass when configured, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckDiskSpacePass(t *testing.T) {
	cfg := DefaultCheckConfig()
	r := cfg.checkDiskSpace(context.Background())
	// On any dev machine, we should have > 1GB free
	if r.Status == Fail {
		t.Errorf("disk space check failed on dev machine: %s", r.Detail)
	}
}

func TestCheckAuthKeysCommandCaseInsensitive(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sshd_config")
	// Lowercase "authorizedkeyscommand" should still match
	content := "Port 22\nauthorizedkeyscommand /usr/local/bin/podspawn auth-keys %u\nauthorizedkeyscommanduser root\n"
	_ = os.WriteFile(f, []byte(content), 0644)

	cfg := CheckConfig{SSHDConfigPath: f}
	r := cfg.checkAuthKeysCommand(context.Background())
	if r.Status != Pass {
		t.Errorf("expected Pass for lowercase AuthorizedKeysCommand, got %v: %s", r.Status, r.Detail)
	}
}

func TestCheckStateDirNotWritable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "readonly")
	_ = os.MkdirAll(dir, 0555)

	cfg := CheckConfig{StateDir: dir}
	r := cfg.checkStateDir(context.Background())
	// On macOS root can write to 0555 dirs, so we allow Fail or Pass
	// depending on whether we're running as root. The point is it doesn't panic.
	if r.Status == Warn {
		t.Errorf("expected Pass or Fail for read-only dir, got Warn: %s", r.Detail)
	}

	// If not root, we expect Fail
	if os.Getuid() != 0 && r.Status != Fail {
		t.Errorf("expected Fail for non-root user writing to 0555 dir, got %v: %s", r.Status, r.Detail)
	}
}

func TestRunAllOutput(t *testing.T) {
	dir := t.TempDir()
	cfg := CheckConfig{
		SSHDConfigPath: filepath.Join(dir, "nonexistent"),
		KeyDir:         filepath.Join(dir, "keys"),
		StateDir:       dir,
		LockDir:        filepath.Join(dir, "locks"),
		DefaultImage:   "nonexistent:image",
	}

	var buf bytes.Buffer
	passed, warned, failed := RunAll(context.Background(), cfg, &buf)

	output := buf.String()
	total := passed + warned + failed
	if total == 0 {
		t.Fatal("expected some checks to run")
	}
	// Output should contain [pass], [warn], or [FAIL] markers
	if len(output) == 0 {
		t.Error("expected non-empty output")
	}
}
