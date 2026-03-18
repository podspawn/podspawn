package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadClientValidConfig(t *testing.T) {
	yaml := `
servers:
  default: fallback.example.com
  mappings:
    work.pod: devbox.company.com
    gpu.pod: gpu-server.company.com
`
	cfg, err := LoadClient(writeTemp(t, yaml))
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
	_, err := LoadClient(writeTemp(t, "{{not yaml"))
	if err == nil {
		t.Fatal("expected error for bad YAML")
	}
}

func TestLoadClientEmptyMappings(t *testing.T) {
	yaml := `
servers:
  default: solo.example.com
`
	cfg, err := LoadClient(writeTemp(t, yaml))
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

func TestLocalConfigApplyToOverridesFields(t *testing.T) {
	cfg := LocalDefaults()
	lc := LocalConfig{
		Image:       "alpine:3.20",
		Shell:       "/bin/zsh",
		CPUs:        8,
		Memory:      "16g",
		GracePeriod: "120s",
		MaxLifetime: "48h",
		Mode:        "destroy-on-disconnect",
	}
	lc.ApplyTo(cfg)

	if cfg.Defaults.Image != "alpine:3.20" {
		t.Errorf("image = %q, want alpine:3.20", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/zsh" {
		t.Errorf("shell = %q, want /bin/zsh", cfg.Defaults.Shell)
	}
	if cfg.Defaults.CPUs != 8 {
		t.Errorf("cpus = %f, want 8", cfg.Defaults.CPUs)
	}
	if cfg.Defaults.Memory != "16g" {
		t.Errorf("memory = %q, want 16g", cfg.Defaults.Memory)
	}
	if cfg.Session.GracePeriod != "120s" {
		t.Errorf("grace_period = %q, want 120s", cfg.Session.GracePeriod)
	}
	if cfg.Session.MaxLifetime != "48h" {
		t.Errorf("max_lifetime = %q, want 48h", cfg.Session.MaxLifetime)
	}
	if cfg.Session.Mode != "destroy-on-disconnect" {
		t.Errorf("mode = %q, want destroy-on-disconnect", cfg.Session.Mode)
	}
}

func TestLocalConfigApplyToSkipsZeroValues(t *testing.T) {
	cfg := LocalDefaults()
	lc := LocalConfig{Image: "debian:12"} // only image set
	lc.ApplyTo(cfg)

	if cfg.Defaults.Image != "debian:12" {
		t.Errorf("image = %q, want debian:12", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("shell should stay default, got %q", cfg.Defaults.Shell)
	}
	if cfg.Defaults.CPUs != 2.0 {
		t.Errorf("cpus should stay default, got %f", cfg.Defaults.CPUs)
	}
	if cfg.Session.MaxLifetime != "24h" {
		t.Errorf("max_lifetime should stay local default, got %q", cfg.Session.MaxLifetime)
	}
}

func TestLoadClientWithLocalSection(t *testing.T) {
	yaml := `
servers:
  default: myserver.com
local:
  image: fedora:41
  shell: /bin/fish
  cpus: 4
  memory: 4g
`
	cfg, err := LoadClient(writeTemp(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.Image != "fedora:41" {
		t.Errorf("local.image = %q, want fedora:41", cfg.Local.Image)
	}
	if cfg.Local.Shell != "/bin/fish" {
		t.Errorf("local.shell = %q, want /bin/fish", cfg.Local.Shell)
	}
	if cfg.Local.CPUs != 4 {
		t.Errorf("local.cpus = %f, want 4", cfg.Local.CPUs)
	}
	if cfg.Local.Memory != "4g" {
		t.Errorf("local.memory = %q, want 4g", cfg.Local.Memory)
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
