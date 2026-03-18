package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Auth.KeyDir != "/etc/podspawn/keys" {
		t.Errorf("auth.key_dir = %q, want /etc/podspawn/keys", cfg.Auth.KeyDir)
	}
	if cfg.Defaults.Image != "ubuntu:24.04" {
		t.Errorf("defaults.image = %q, want ubuntu:24.04", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("defaults.shell = %q, want /bin/bash", cfg.Defaults.Shell)
	}
	if cfg.Defaults.CPUs != 2.0 {
		t.Errorf("defaults.cpus = %f, want 2.0", cfg.Defaults.CPUs)
	}
	if cfg.Defaults.Memory != "2g" {
		t.Errorf("defaults.memory = %q, want 2g", cfg.Defaults.Memory)
	}
	if cfg.Session.GracePeriod != "60s" {
		t.Errorf("session.grace_period = %q, want 60s", cfg.Session.GracePeriod)
	}
	if cfg.Session.MaxLifetime != "8h" {
		t.Errorf("session.max_lifetime = %q, want 8h", cfg.Session.MaxLifetime)
	}
	if cfg.Session.Mode != "grace-period" {
		t.Errorf("session.mode = %q, want grace-period", cfg.Session.Mode)
	}
	if cfg.Log.File != "" {
		t.Errorf("log.file = %q, want empty", cfg.Log.File)
	}
	if cfg.State.DBPath != "/var/lib/podspawn/state.db" {
		t.Errorf("state.db_path = %q, want /var/lib/podspawn/state.db", cfg.State.DBPath)
	}
	if cfg.State.LockDir != "/var/lib/podspawn/locks" {
		t.Errorf("state.lock_dir = %q, want /var/lib/podspawn/locks", cfg.State.LockDir)
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"2g", 2 * 1024 * 1024 * 1024},
		{"512m", 512 * 1024 * 1024},
		{"1024k", 1024 * 1024},
		{"4G", 4 * 1024 * 1024 * 1024},
		{"0", 0},
		{"", 0},
		{"1073741824", 1073741824},
		{"0.5g", 512 * 1024 * 1024},
	}
	for _, tt := range tests {
		got, err := ParseMemory(tt.input)
		if err != nil {
			t.Errorf("ParseMemory(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseMemory(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseMemoryErrors(t *testing.T) {
	invalid := []string{"abc", "-1g", "2x"}
	for _, s := range invalid {
		_, err := ParseMemory(s)
		if err == nil {
			t.Errorf("ParseMemory(%q) should error", s)
		}
	}
}

func TestLoadValidConfig(t *testing.T) {
	yaml := `
auth:
  key_dir: /opt/keys
defaults:
  image: "debian:12"
  shell: "/bin/zsh"
  cpus: 4.0
  memory: "8g"
session:
  grace_period: "30s"
  max_lifetime: "4h"
  mode: "destroy-on-disconnect"
log:
  file: "/var/log/podspawn.log"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Auth.KeyDir != "/opt/keys" {
		t.Errorf("auth.key_dir = %q, want /opt/keys", cfg.Auth.KeyDir)
	}
	if cfg.Defaults.Image != "debian:12" {
		t.Errorf("defaults.image = %q, want debian:12", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/zsh" {
		t.Errorf("defaults.shell = %q, want /bin/zsh", cfg.Defaults.Shell)
	}
	if cfg.Defaults.CPUs != 4.0 {
		t.Errorf("defaults.cpus = %f, want 4.0", cfg.Defaults.CPUs)
	}
	if cfg.Session.GracePeriod != "30s" {
		t.Errorf("session.grace_period = %q, want 30s", cfg.Session.GracePeriod)
	}
	if cfg.Session.Mode != "destroy-on-disconnect" {
		t.Errorf("session.mode = %q, want destroy-on-disconnect", cfg.Session.Mode)
	}
	if cfg.Log.File != "/var/log/podspawn.log" {
		t.Errorf("log.file = %q, want /var/log/podspawn.log", cfg.Log.File)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("missing file should not error, got: %v", err)
	}

	defaults := Defaults()
	if cfg.Defaults.Image != defaults.Defaults.Image {
		t.Errorf("missing file should return defaults, got image %q", cfg.Defaults.Image)
	}
}

func TestLoadPartialConfig(t *testing.T) {
	yaml := `
defaults:
  image: "alpine:3.20"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Defaults.Image != "alpine:3.20" {
		t.Errorf("image = %q, want alpine:3.20", cfg.Defaults.Image)
	}
	if cfg.Auth.KeyDir != "/etc/podspawn/keys" {
		t.Errorf("key_dir should default to /etc/podspawn/keys, got %q", cfg.Auth.KeyDir)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("shell should default to /bin/bash, got %q", cfg.Defaults.Shell)
	}
	if cfg.Session.GracePeriod != "60s" {
		t.Errorf("grace_period should default to 60s, got %q", cfg.Session.GracePeriod)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	yaml := `
session:
  grace_period: "60"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for grace_period without time unit")
	}
	if !strings.Contains(err.Error(), "session.grace_period") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestLoadRejectsInvalidMemory(t *testing.T) {
	yaml := `
defaults:
  memory: "lots"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid memory value")
	}
	if !strings.Contains(err.Error(), "defaults.memory") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTemp(t, "{{not yaml at all")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLocalDefaults(t *testing.T) {
	cfg := LocalDefaults()

	home, _ := os.UserHomeDir()
	wantDB := filepath.Join(home, ".podspawn", "state.db")
	wantLocks := filepath.Join(home, ".podspawn", "locks")

	if cfg.State.DBPath != wantDB {
		t.Errorf("state.db_path = %q, want %q", cfg.State.DBPath, wantDB)
	}
	if cfg.State.LockDir != wantLocks {
		t.Errorf("state.lock_dir = %q, want %q", cfg.State.LockDir, wantLocks)
	}
	if cfg.Session.MaxLifetime != "24h" {
		t.Errorf("session.max_lifetime = %q, want 24h", cfg.Session.MaxLifetime)
	}
	if cfg.Defaults.Image != "ubuntu:24.04" {
		t.Errorf("defaults.image = %q, want ubuntu:24.04", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("defaults.shell = %q, want /bin/bash", cfg.Defaults.Shell)
	}
}

func TestLocalDefaultsPathsAreUserLocal(t *testing.T) {
	cfg := LocalDefaults()

	if strings.HasPrefix(cfg.State.DBPath, "/var") {
		t.Errorf("local DB path should not be under /var: %s", cfg.State.DBPath)
	}
	if strings.HasPrefix(cfg.State.LockDir, "/var") {
		t.Errorf("local lock dir should not be under /var: %s", cfg.State.LockDir)
	}
	if !strings.Contains(cfg.State.DBPath, ".podspawn") {
		t.Errorf("local DB path should contain .podspawn: %s", cfg.State.DBPath)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
