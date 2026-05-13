package cmd

import (
	"testing"

	"github.com/podspawn/podspawn/internal/identity"
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

// Regression: `podspawn shell` previously only set Username from the
// `user@` prefix, leaving Actor as whatever buildLocalSession seeded — i.e.
// the invoking OS user. With Stage 7's audit enrichment, that meant a
// cross-user attach (`podspawn shell alice@backend` as root) would emit
// the operator's identity as if it were the target user when buildLocalSession
// shipped without an actor seed, and as the invoker-as-human if it did. Both
// are wrong: the actor must reflect the cross-user nature explicitly.

func TestShellActorNoCrossUserIsHumanInvoker(t *testing.T) {
	got := shellActor("alice", "")
	if got.Kind != identity.KindHuman || got.OSUser != "alice" {
		t.Fatalf("shellActor(\"alice\",\"\") = %+v, want Human(alice)", got)
	}
	if got.String() != "human:alice" {
		t.Fatalf("actor = %q, want human:alice", got.String())
	}
}

func TestShellActorSameTargetIsHuman(t *testing.T) {
	got := shellActor("alice", "alice")
	if got.Kind != identity.KindHuman || got.OSUser != "alice" {
		t.Fatalf("shellActor(\"alice\",\"alice\") = %+v, want Human(alice)", got)
	}
}

func TestShellActorCrossUserIsOperatorOfInvoker(t *testing.T) {
	// `podspawn shell alice@backend` run by root.
	got := shellActor("root", "alice")
	if got.Kind != identity.KindOperator {
		t.Fatalf("kind = %q, want operator (target must NOT become the actor)", got.Kind)
	}
	if got.OSUser != "root" {
		t.Fatalf("OSUser = %q, want root (the invoker)", got.OSUser)
	}
	if got.ID != "root" {
		t.Fatalf("ID = %q, want root", got.ID)
	}
	if got.String() != "operator:root" {
		t.Fatalf("actor = %q, want operator:root", got.String())
	}
}
