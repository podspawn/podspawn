package cmd

import "testing"

func TestParseSessionArg(t *testing.T) {
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
		user, project := parseSessionArg(tt.arg)
		if user != tt.wantUser || project != tt.wantProject {
			t.Errorf("parseSessionArg(%q) = (%q, %q), want (%q, %q)",
				tt.arg, user, project, tt.wantUser, tt.wantProject)
		}
	}
}
