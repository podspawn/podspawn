package serversetup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	sshdConfig := filepath.Join(root, "sshd_config")
	return Paths{
		SSHDConfig:   sshdConfig,
		BinaryPath:   "/usr/local/bin/podspawn",
		PodspawnDir:  filepath.Join(root, "etc", "podspawn"),
		KeyDir:       filepath.Join(root, "etc", "podspawn", "keys"),
		StateDir:     filepath.Join(root, "var", "lib", "podspawn"),
		LockDir:      filepath.Join(root, "var", "lib", "podspawn", "locks"),
		EmergencyKey: filepath.Join(root, "etc", "podspawn", "emergency.keys"),
	}
}

func writeSSHDConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

var minimalSSHDConfig = "Port 22\nPermitRootLogin no\n"

func TestAppendsAuthKeysCommand(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(paths.SSHDConfig)
	content := string(data)
	if !strings.Contains(content, "AuthorizedKeysCommand /usr/local/bin/podspawn auth-keys %u %t %k") {
		t.Error("sshd_config missing AuthorizedKeysCommand line")
	}
	if !strings.Contains(content, "AuthorizedKeysCommandUser nobody") {
		t.Error("sshd_config missing AuthorizedKeysCommandUser line")
	}

	reloadCalled := false
	for _, call := range cmd.Calls {
		if len(call) >= 3 && call[0] == "systemctl" && call[1] == "reload" {
			reloadCalled = true
		}
	}
	if !reloadCalled {
		t.Error("systemctl reload was not called")
	}
}

func TestCreatesBackup(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(paths.SSHDConfig + ".podspawn.bak")
	if err != nil {
		t.Fatal("backup file not created")
	}
	if string(backup) != minimalSSHDConfig {
		t.Errorf("backup content = %q, want original config", string(backup))
	}
}

func TestRollbackOnValidationFailure(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)

	cmd := NewFakeCommander()
	cmd.ErrSequence["sshd"] = []error{nil, errors.New("validation failed")}
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err == nil {
		t.Fatal("expected error from second sshd -t")
	}

	restored, _ := os.ReadFile(paths.SSHDConfig)
	if string(restored) != minimalSSHDConfig {
		t.Error("sshd_config was not restored from backup")
	}

	if !strings.Contains(out.String(), "restored") {
		t.Error("output should mention restoration")
	}

	for _, call := range cmd.Calls {
		if len(call) >= 3 && call[0] == "systemctl" && call[1] == "reload" {
			t.Error("reload should not be called after validation failure")
		}
	}
}

func TestIdempotentAlreadyConfigured(t *testing.T) {
	paths := testPaths(t)
	config := minimalSSHDConfig + "AuthorizedKeysCommand /usr/local/bin/podspawn auth-keys %u %t %k\nAuthorizedKeysCommandUser nobody\n"
	writeSSHDConfig(t, paths.SSHDConfig, config)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(paths.SSHDConfig)
	if string(data) != config {
		t.Error("sshd_config should not have been modified")
	}

	if _, err := os.Stat(paths.SSHDConfig + ".podspawn.bak"); err == nil {
		t.Error("no backup should be created when already configured")
	}

	if _, err := os.Stat(paths.PodspawnDir); os.IsNotExist(err) {
		t.Error("directories should still be created even when already configured")
	}
}

func TestConflictOtherAuthKeysCommand(t *testing.T) {
	paths := testPaths(t)
	config := minimalSSHDConfig + "AuthorizedKeysCommand /usr/bin/other-tool %u\n"
	writeSSHDConfig(t, paths.SSHDConfig, config)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err == nil {
		t.Fatal("expected error for conflicting AuthorizedKeysCommand")
	}
	if !strings.Contains(err.Error(), "another AuthorizedKeysCommand") {
		t.Errorf("error should mention conflict, got: %v", err)
	}
}

func TestPreValidationFailure(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	cmd.Errors["sshd"] = errors.New("config broken")
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err == nil {
		t.Fatal("expected error for broken sshd config")
	}
	if !strings.Contains(err.Error(), "current sshd config is invalid") {
		t.Errorf("error should mention current config, got: %v", err)
	}
}

func TestLockoutWarning(t *testing.T) {
	paths := testPaths(t)
	config := "PasswordAuthentication no\nAuthorizedKeysFile none\n"
	writeSSHDConfig(t, paths.SSHDConfig, config)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	if !strings.Contains(output, "WARNING") {
		t.Error("should emit warning about lockout risk")
	}
}

func TestCreatesDirs(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{paths.PodspawnDir, paths.KeyDir, paths.StateDir, paths.LockDir} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("directory not created: %s", dir)
		}
	}
}

func TestEmergencyKeysNotOverwritten(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)

	os.MkdirAll(filepath.Dir(paths.EmergencyKey), 0755)                             //nolint:errcheck
	os.WriteFile(paths.EmergencyKey, []byte("ssh-ed25519 AAAA existing-key"), 0600) //nolint:errcheck

	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(paths.EmergencyKey)
	if !strings.Contains(string(data), "existing-key") {
		t.Error("emergency.keys content was overwritten")
	}
}

func TestDryRunModifiesNothing(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{DryRun: true}, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(paths.SSHDConfig)
	if string(data) != minimalSSHDConfig {
		t.Error("sshd_config should not be modified in dry-run")
	}

	if _, err := os.Stat(paths.PodspawnDir); err == nil {
		t.Error("directories should not be created in dry-run")
	}

	output := out.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Error("output should contain dry-run markers")
	}
}

func TestBinaryPathInAppendedLine(t *testing.T) {
	paths := testPaths(t)
	paths.BinaryPath = "/opt/custom/bin/podspawn"
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(paths.SSHDConfig)
	if !strings.Contains(string(data), "AuthorizedKeysCommand /opt/custom/bin/podspawn auth-keys") {
		t.Error("appended line should use the provided binary path")
	}
}

func TestServiceDetection(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)

	cmd := NewFakeCommander()
	cmd.Errors["systemctl is-active"] = errors.New("inactive")
	var out bytes.Buffer

	if err := Run(paths, cmd, Options{}, &out); err != nil {
		t.Fatal(err)
	}

	reloadedService := ""
	for _, call := range cmd.Calls {
		if len(call) >= 3 && call[0] == "systemctl" && call[1] == "reload" {
			reloadedService = call[2]
		}
	}
	if reloadedService != "sshd" {
		t.Errorf("should fall back to sshd, got %q", reloadedService)
	}
}

func TestReloadFailureNoRollback(t *testing.T) {
	paths := testPaths(t)
	writeSSHDConfig(t, paths.SSHDConfig, minimalSSHDConfig)

	cmd := NewFakeCommander()
	cmd.Errors["systemctl reload"] = errors.New("reload failed")
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err == nil {
		t.Fatal("expected error from reload failure")
	}
	if !strings.Contains(err.Error(), "reload manually") {
		t.Errorf("error should suggest manual reload, got: %v", err)
	}

	data, _ := os.ReadFile(paths.SSHDConfig)
	if !strings.Contains(string(data), "AuthorizedKeysCommand") {
		t.Error("config should NOT be rolled back on reload failure (config is valid)")
	}
}

func TestConfigNotFound(t *testing.T) {
	paths := testPaths(t)
	cmd := NewFakeCommander()
	var out bytes.Buffer

	err := Run(paths, cmd, Options{}, &out)
	if err == nil {
		t.Fatal("expected error for missing sshd_config")
	}
}
