package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeClientTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadClientValidConfig(t *testing.T) {
	yaml := `
servers:
  default: fallback.example.com
  mappings:
    work.pod: devbox.company.com
    gpu.pod: gpu-server.company.com
`
	cfg, err := LoadClient(writeClientTemp(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers.Default != "fallback.example.com" {
		t.Errorf("default = %q, want fallback.example.com", cfg.Servers.Default)
	}
	if len(cfg.Servers.Mappings) != 2 {
		t.Errorf("mappings count = %d, want 2", len(cfg.Servers.Mappings))
	}
	if cfg.Servers.Mappings["work.pod"] != "devbox.company.com" {
		t.Errorf("work.pod mapping = %q, want devbox.company.com", cfg.Servers.Mappings["work.pod"])
	}
}

func TestLoadClientMissingFile(t *testing.T) {
	_, err := LoadClient(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestLoadClientMalformedYAML(t *testing.T) {
	_, err := LoadClient(writeClientTemp(t, "{{not yaml"))
	if err == nil {
		t.Fatal("expected error for bad YAML")
	}
}

func TestLoadClientEmptyMappings(t *testing.T) {
	yaml := `
servers:
  default: solo.example.com
`
	cfg, err := LoadClient(writeClientTemp(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers.Default != "solo.example.com" {
		t.Errorf("default = %q, want solo.example.com", cfg.Servers.Default)
	}
	if len(cfg.Servers.Mappings) != 0 {
		t.Errorf("mappings should be empty, got %v", cfg.Servers.Mappings)
	}
}

func TestResolveHostMapped(t *testing.T) {
	cfg := &ClientConfig{
		Servers: ServerRouting{
			Default:  "fallback.example.com",
			Mappings: map[string]string{"work.pod": "devbox.company.com"},
		},
	}
	got, err := cfg.ResolveHost("work.pod")
	if err != nil {
		t.Fatal(err)
	}
	if got != "devbox.company.com" {
		t.Errorf("resolved = %q, want devbox.company.com", got)
	}
}

func TestResolveHostFallsBackToDefault(t *testing.T) {
	cfg := &ClientConfig{
		Servers: ServerRouting{
			Default:  "fallback.example.com",
			Mappings: map[string]string{"work.pod": "devbox.company.com"},
		},
	}
	got, err := cfg.ResolveHost("mystery.pod")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback.example.com" {
		t.Errorf("resolved = %q, want fallback.example.com", got)
	}
}

func TestResolveHostNoDefaultNoMapping(t *testing.T) {
	cfg := &ClientConfig{
		Servers: ServerRouting{
			Mappings: map[string]string{"work.pod": "devbox.company.com"},
		},
	}
	_, err := cfg.ResolveHost("unknown.pod")
	if err == nil {
		t.Fatal("expected error for unresolvable hostname")
	}
	if !strings.Contains(err.Error(), "unknown.pod") {
		t.Errorf("error should mention hostname, got: %v", err)
	}
}

func TestResolveHostNonPodPassthrough(t *testing.T) {
	cfg := &ClientConfig{
		Servers: ServerRouting{
			Default: "fallback.example.com",
		},
	}
	got, err := cfg.ResolveHost("devbox.company.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "devbox.company.com" {
		t.Errorf("non-.pod host should pass through, got %q", got)
	}
}

func TestResolveHostBarePodSuffix(t *testing.T) {
	cfg := &ClientConfig{
		Servers: ServerRouting{Default: "fallback.example.com"},
	}
	got, err := cfg.ResolveHost(".pod")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback.example.com" {
		t.Errorf("bare .pod should fall back to default, got %q", got)
	}
}
