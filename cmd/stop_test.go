package cmd

import (
	"os"
	"testing"
)

func TestResolveStopArgServerMode(t *testing.T) {
	isLocalMode = false
	defer func() { isLocalMode = false }()
	tests := []struct {
		arg         string
		wantUser    string
		wantProject string
	}{
		{"alice", "alice", ""},
		{"alice@backend", "alice", "backend"},
		{"@project", "", "project"},
		{"alice@", "alice", ""},
	}
	for _, tt := range tests {
		user, project := resolveStopArg(tt.arg)
		if user != tt.wantUser || project != tt.wantProject {
			t.Errorf("resolveStopArg(%q) server mode = (%q, %q), want (%q, %q)",
				tt.arg, user, project, tt.wantUser, tt.wantProject)
		}
	}
}

func TestResolveStopArgLocalMode(t *testing.T) {
	isLocalMode = true
	defer func() { isLocalMode = false }()

	t.Setenv("USER", "karthik")

	tests := []struct {
		arg         string
		wantUser    string
		wantProject string
	}{
		{"mydev", "karthik", "mydev"},
		{"scratch", "karthik", "scratch"},
		{"alice@backend", "alice", "backend"}, // @ syntax still works
	}
	for _, tt := range tests {
		user, project := resolveStopArg(tt.arg)
		if user != tt.wantUser || project != tt.wantProject {
			t.Errorf("resolveStopArg(%q) local mode = (%q, %q), want (%q, %q)",
				tt.arg, user, project, tt.wantUser, tt.wantProject)
		}
	}
}

func TestResolveStopArgLocalModeFallsBackToUSER(t *testing.T) {
	isLocalMode = true
	defer func() { isLocalMode = false }()

	want := os.Getenv("USER")
	user, project := resolveStopArg("testmachine")
	if user != want {
		t.Errorf("user = %q, want $USER (%q)", user, want)
	}
	if project != "testmachine" {
		t.Errorf("project = %q, want testmachine", project)
	}
}
