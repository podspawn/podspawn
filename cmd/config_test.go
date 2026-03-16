package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/config"
)

func TestShowConfigNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	err := showConfig(path)
	if err != nil {
		t.Fatalf("showConfig with missing file should not error, got: %v", err)
	}
}

func TestSetConfigCreatesFromScratch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".podspawn", "config.yaml")

	if err := setConfig(path, "servers.default", "myserver.com"); err != nil {
		t.Fatal(err)
	}

	clientCfg, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if clientCfg.Servers.Default != "myserver.com" {
		t.Errorf("servers.default = %q, want myserver.com", clientCfg.Servers.Default)
	}
}

func TestSetConfigUpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".podspawn")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.yaml")

	initial := "servers:\n  default: old.example.com\n"
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	if err := setConfig(path, "servers.default", "new.example.com"); err != nil {
		t.Fatal(err)
	}

	clientCfg, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if clientCfg.Servers.Default != "new.example.com" {
		t.Errorf("servers.default = %q, want new.example.com", clientCfg.Servers.Default)
	}
}

func TestSetConfigMapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".podspawn", "config.yaml")

	if err := setConfig(path, "servers.mappings.work.pod", "devbox.internal"); err != nil {
		t.Fatal(err)
	}

	clientCfg, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := clientCfg.Servers.Mappings["work.pod"]; got != "devbox.internal" {
		t.Errorf("servers.mappings[work.pod] = %q, want devbox.internal", got)
	}
}

func TestSetConfigInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := setConfig(path, "bogus.key", "value")
	if err == nil {
		t.Fatal("expected error for unknown config key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error should mention unknown key, got: %v", err)
	}
}
