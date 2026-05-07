package cmd

import (
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/state"
)

func TestParseShellTarget(t *testing.T) {
	tests := []struct {
		input       string
		wantUser    string
		wantProject string
	}{
		{"myproject", "", "myproject"},
		{"alice@backend", "alice", "backend"},
		{"deploy@", "deploy", ""},
		{"@orphan", "", "orphan"},
		{"alice@my-project", "alice", "my-project"},
	}

	for _, tt := range tests {
		user, project := parseShellTarget(tt.input)
		if user != tt.wantUser || project != tt.wantProject {
			t.Errorf("parseShellTarget(%q) = (%q, %q), want (%q, %q)",
				tt.input, user, project, tt.wantUser, tt.wantProject)
		}
	}
}

func TestRequireExistingShellTarget(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(store *state.FakeStore)
		wantErr string
	}{
		{
			name: "machine exists",
			setup: func(store *state.FakeStore) {
				if err := store.CreateWorkspace(&state.Workspace{
					User:    "tenant",
					Name:    "backend-main",
					Project: "backend",
				}); err != nil {
					t.Fatalf("create machine: %v", err)
				}
			},
		},
		{
			name: "session exists",
			setup: func(store *state.FakeStore) {
				if err := store.CreateSession(&state.Session{
					User:    "tenant",
					Project: "backend-main",
				}); err != nil {
					t.Fatalf("create session: %v", err)
				}
			},
		},
		{
			name:    "missing machine and session",
			setup:   func(store *state.FakeStore) {},
			wantErr: `no workspace or session named "backend-main" for user "tenant"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := state.NewFakeStore()
			tt.setup(store)

			err := requireExistingShellTarget(store, "tenant", "backend-main")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("requireExistingShellTarget() error = %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("requireExistingShellTarget() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("requireExistingShellTarget() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
