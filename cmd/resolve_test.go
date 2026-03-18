package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMachineName(t *testing.T) {
	valid := []string{"dev", "my-project", "backend_v2", "a123"}
	for _, name := range valid {
		if err := validateMachineName(name); err != nil {
			t.Errorf("validateMachineName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "../etc", "my project", "name/slash", "UPPER", ".hidden", "-dash",
		"a b", "hello;world", "test$(cmd)", "name\nline"}
	for _, name := range invalid {
		if err := validateMachineName(name); err == nil {
			t.Errorf("validateMachineName(%q) = nil, want error", name)
		}
	}
}

func TestParseUserHost(t *testing.T) {
	tests := []struct {
		input    string
		wantUser string
		wantHost string
		wantErr  bool
	}{
		{"alice@backend", "alice", "backend", false},
		{"@backend", "", "", true},
		{"alice@", "", "", true},
		{"backend", "", "", true},
		{"a@b@c", "a", "b@c", false},
		{"", "", "", true},
		{"@", "", "", true},
		{"root@192.168.1.1", "root", "192.168.1.1", false},
	}
	for _, tt := range tests {
		user, host, err := parseUserHost(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseUserHost(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if user != tt.wantUser || host != tt.wantHost {
			t.Errorf("parseUserHost(%q) = (%q, %q), want (%q, %q)",
				tt.input, user, host, tt.wantUser, tt.wantHost)
		}
	}
}

func TestAppendPodSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"backend", "backend.pod"},
		{"server.com", "server.com"},
		{"host:22", "host:22"},
		{"x.pod", "x.pod"},
		{"192.168.1.1", "192.168.1.1"},
		{"[::1]:22", "[::1]:22"},
		{"myproject", "myproject.pod"},
	}
	for _, tt := range tests {
		got := appendPodSuffix(tt.input)
		if got != tt.want {
			t.Errorf("appendPodSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolvePodHost(t *testing.T) {
	t.Run("localhost.pod resolves without config", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		got, err := resolvePodHost("localhost.pod")
		if err != nil {
			t.Fatal(err)
		}
		if got != "127.0.0.1" {
			t.Errorf("got %q, want 127.0.0.1", got)
		}
	})

	t.Run("non-pod hostname passes through", func(t *testing.T) {
		got, err := resolvePodHost("devbox.company.com")
		if err != nil {
			t.Fatal(err)
		}
		if got != "devbox.company.com" {
			t.Errorf("got %q, want devbox.company.com", got)
		}
	})

	t.Run("pod hostname without config errors", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		_, err := resolvePodHost("work.pod")
		if err == nil {
			t.Fatal("expected error for .pod without config")
		}
	})

	t.Run("pod hostname resolves via mappings", func(t *testing.T) {
		dir := t.TempDir()
		configDir := filepath.Join(dir, ".podspawn")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			t.Fatal(err)
		}
		cfgYAML := "servers:\n  mappings:\n    staging.pod: staging.internal.net\n"
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(cfgYAML), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", dir)

		got, err := resolvePodHost("staging.pod")
		if err != nil {
			t.Fatal(err)
		}
		if got != "staging.internal.net" {
			t.Errorf("got %q, want staging.internal.net", got)
		}
	})

	t.Run("pod hostname falls back to default server", func(t *testing.T) {
		dir := t.TempDir()
		configDir := filepath.Join(dir, ".podspawn")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			t.Fatal(err)
		}
		cfgYAML := "servers:\n  default: fallback.example.com\n"
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(cfgYAML), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", dir)

		got, err := resolvePodHost("anything.pod")
		if err != nil {
			t.Fatal(err)
		}
		if got != "fallback.example.com" {
			t.Errorf("got %q, want fallback.example.com", got)
		}
	})
}
