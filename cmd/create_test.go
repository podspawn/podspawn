package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/session"
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

// TestWrapCreateErrorSurfacesHintThroughServiceWrap is the cmd-side
// regression for the Stage 5 blocker: the service now wraps provisioning
// failures as fmt.Errorf("%w: %w", ErrRuntimeFailure, hookErr). cmd's
// wrapCreateError must still reach the inner *spawn.HookError via
// errors.As and emit the preserved-workspace recovery hint.
func TestWrapCreateErrorSurfacesHintThroughServiceWrap(t *testing.T) {
	hookErr := &spawn.HookError{Hook: "on_create", Err: errors.New("exit status 9")}
	wrapped := fmt.Errorf("%w: %w", session.ErrRuntimeFailure, hookErr)

	err := wrapCreateError(
		"backend-main",
		"/Users/tenant/.podspawn/workspaces/backend-main",
		wrapped,
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
	// And the typed-class assertion still holds, so transport callers can
	// branch on either side.
	if !errors.Is(err, session.ErrRuntimeFailure) {
		t.Fatalf("errors.Is(err, ErrRuntimeFailure) = false; err = %v", err)
	}
	var pinned *spawn.HookError
	if !errors.As(err, &pinned) || pinned.Hook != "on_create" {
		t.Fatalf("errors.As did not recover HookError; got %v", pinned)
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
