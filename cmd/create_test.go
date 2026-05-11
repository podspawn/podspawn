package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/spawn"
)

func TestCreateCmdUsage(t *testing.T) {
	if createCmd.Use != "create <name>" {
		t.Errorf("create Use = %q, want 'create <name>'", createCmd.Use)
	}
	if createCmd.Args == nil {
		t.Error("create should require exactly 1 arg")
	}
}

func TestCreateCmdHasImageFlag(t *testing.T) {
	flag := createCmd.Flags().Lookup("image")
	if flag == nil {
		t.Fatal("create should have --image flag")
	}
	if flag.DefValue != "" {
		t.Errorf("--image default = %q, want empty", flag.DefValue)
	}
}

func TestCreateCmdHasProjectFlags(t *testing.T) {
	if createCmd.Flags().Lookup("project") == nil {
		t.Fatal("create should have --project flag")
	}
	if createCmd.Flags().Lookup("branch") == nil {
		t.Fatal("create should have --branch flag")
	}
}

func TestWrapCreateErrorAddsWorkspaceRecoveryHintForHookFailures(t *testing.T) {
	err := wrapCreateError(
		"backend-main",
		"/Users/tenant/.podspawn/workspaces/backend-main",
		&spawn.HookError{Hook: "on_create", Err: errors.New("exit status 9")},
	)
	if err == nil {
		t.Fatal("wrapCreateError() returned nil")
	}

	msg := err.Error()
	for _, want := range []string{
		"workspace preserved at",
		"podspawn create backend-main",
		"podspawn rm backend-main",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}
