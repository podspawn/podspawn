package cmd

import (
	"os"
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

func TestEnsureLocalWorkspaceRootUsesRestrictedPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")

	if err := ensureLocalWorkspaceRoot(root); err != nil {
		t.Fatalf("ensureLocalWorkspaceRoot() error = %v", err)
	}

	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("workspace root perms = %04o, want 0700", info.Mode().Perm())
	}
}
