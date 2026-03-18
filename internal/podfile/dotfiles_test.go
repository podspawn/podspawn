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
	err := CloneDotfiles(context.Background(), rt, "dev-ctr", "deploy", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	call := rt.ExecCalls[0]
	if call.Opts.Cmd[0] != "git" || call.Opts.Cmd[1] != "clone" {
		t.Errorf("expected git clone, got %v", call.Opts.Cmd)
	}
	if call.Opts.User != "deploy" {
		t.Errorf("exec user = %q, want deploy", call.Opts.User)
	}
	// Should clone to /home/deploy/dotfiles, not /root/dotfiles
	cloneDest := call.Opts.Cmd[len(call.Opts.Cmd)-1]
	if cloneDest != "/home/deploy/dotfiles" {
		t.Errorf("clone dest = %q, want /home/deploy/dotfiles", cloneDest)
	}
}

func TestCloneDotfilesWithInstall(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	cfg := &DotfilesConfig{
		Repo:    "https://github.com/user/dots",
		Install: "./install.sh",
	}
	err := CloneDotfiles(context.Background(), rt, "dev-ctr", "deploy", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 2 {
		t.Fatalf("expected 2 exec calls (clone + install), got %d", len(rt.ExecCalls))
	}
	// Both should run as the user
	for i, call := range rt.ExecCalls {
		if call.Opts.User != "deploy" {
			t.Errorf("exec %d user = %q, want deploy", i, call.Opts.User)
		}
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
	err := CloneRepoInContainer(context.Background(), rt, "dev-ctr", "deploy", repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	call := rt.ExecCalls[0]
	if call.Opts.User != "deploy" {
		t.Errorf("exec user = %q, want deploy", call.Opts.User)
	}
	found := false
	for i, arg := range call.Opts.Cmd {
		if arg == "--branch" && i+1 < len(call.Opts.Cmd) && call.Opts.Cmd[i+1] == "develop" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --branch develop in %v", call.Opts.Cmd)
	}
}

func TestRunHookEmpty(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	RunHook(context.Background(), rt, "dev-ctr", "deploy", "on_create", "")
	if len(rt.ExecCalls) != 0 {
		t.Error("empty hook should not exec")
	}
}

func TestRunHookExecutes(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["dev-ctr"] = true

	RunHook(context.Background(), rt, "dev-ctr", "deploy", "on_create", "make setup")
	if len(rt.ExecCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(rt.ExecCalls))
	}
	call := rt.ExecCalls[0]
	if call.Opts.Cmd[0] != "sh" || call.Opts.Cmd[1] != "-c" || call.Opts.Cmd[2] != "make setup" {
		t.Errorf("unexpected command: %v", call.Opts.Cmd)
	}
	if call.Opts.User != "deploy" {
		t.Errorf("hook user = %q, want deploy", call.Opts.User)
	}
}
