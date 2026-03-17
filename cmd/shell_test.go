package cmd

import "testing"

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
