package podfile

import (
	"context"
	"testing"

	"github.com/podspawn/podspawn/internal/runtime"
)

func TestCloneDotfiles(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	cfg := &DotfilesConfig{
		Repo: "https://github.com/user/dots",
	}
	err := CloneDotfiles(context.Background(), rt, "dev-ctr", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	cmd := rt.ExecCalls[0].Opts.Cmd
	if cmd[0] != "git" || cmd[1] != "clone" {
		t.Errorf("expected git clone, got %v", cmd)
	}
}

func TestCloneDotfilesWithInstall(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	cfg := &DotfilesConfig{
		Repo:    "https://github.com/user/dots",
		Install: "./install.sh",
	}
	err := CloneDotfiles(context.Background(), rt, "dev-ctr", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 2 {
		t.Fatalf("expected 2 exec calls (clone + install), got %d", len(rt.ExecCalls))
	}
}

func TestCloneRepoInContainer(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	repo := RepoConfig{
		URL:    "https://github.com/company/backend",
		Path:   "/workspace/backend",
		Branch: "develop",
	}
	err := CloneRepoInContainer(context.Background(), rt, "dev-ctr", repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	cmd := rt.ExecCalls[0].Opts.Cmd
	// Should include --branch develop
	found := false
	for i, arg := range cmd {
		if arg == "--branch" && i+1 < len(cmd) && cmd[i+1] == "develop" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --branch develop in %v", cmd)
	}
}

func TestRunHookEmpty(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	RunHook(context.Background(), rt, "dev-ctr", "on_create", "")
	if len(rt.ExecCalls) != 0 {
		t.Error("empty hook should not exec")
	}
}

func TestRunHookExecutes(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	RunHook(context.Background(), rt, "dev-ctr", "on_create", "make setup")
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	cmd := rt.ExecCalls[0].Opts.Cmd
	if cmd[0] != "sh" || cmd[1] != "-c" || cmd[2] != "make setup" {
		t.Errorf("unexpected command: %v", cmd)
	}
}
