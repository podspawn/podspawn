package cmd

import (
	"path/filepath"
	"testing"

	"github.com/podspawn/podspawn/internal/config"
)

func TestLocalWorkspacesRootUsesPodspawnHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(t.TempDir(), "state", "state.db")
	t.Cleanup(func() { cfg = oldCfg })

	got := localWorkspacesRoot()
	want := filepath.Join(filepath.Dir(cfg.ProjectsFile), "workspaces")
	if got != want {
		t.Fatalf("localWorkspacesRoot() = %q, want %q", got, want)
	}
}
