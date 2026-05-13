package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/identity"
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

// Regression: before this commit `buildLocalSession` left
// `spawn.Session.Actor` zero, so direct local attach paths
// (`podspawn run` without --project, `podspawn dev`, `podspawn shell`)
// drove `RunAndCleanup` with an Unknown actor and emitted audit events with
// `actor="unknown"` / empty `actor_kind`. The fix seeds the actor from the
// already-resolved invoking OS user.
func TestBuildLocalSessionSeedsHumanActor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("USER", "dev-user")
	oldCfg := cfg
	cfg = config.LocalDefaults()
	cfg.State.DBPath = filepath.Join(root, "state.db")
	cfg.ProjectsFile = filepath.Join(root, "projects.yaml")
	t.Cleanup(func() { cfg = oldCfg })

	ls, err := buildLocalSession("scratch")
	if err != nil {
		t.Fatalf("buildLocalSession error = %v", err)
	}
	defer ls.Close()

	got := ls.Session.Actor
	if !got.Valid() {
		t.Fatalf("Actor not Valid: %+v", got)
	}
	if got.Kind != identity.KindHuman {
		t.Fatalf("Actor.Kind = %q, want human", got.Kind)
	}
	if got.OSUser != "dev-user" {
		t.Fatalf("Actor.OSUser = %q, want dev-user", got.OSUser)
	}
	if got.String() != "human:dev-user" {
		t.Fatalf("Actor.String() = %q, want human:dev-user", got.String())
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
