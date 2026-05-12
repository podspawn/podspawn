package identity

import "testing"

func TestHuman(t *testing.T) {
	a := Human("alice")
	if a.Kind != KindHuman || a.ID != "alice" || a.OSUser != "alice" {
		t.Fatalf("Human(alice) = %+v", a)
	}
	if !a.Valid() {
		t.Fatal("Human(alice).Valid() = false")
	}
}

func TestActorValid(t *testing.T) {
	cases := []struct {
		name string
		a    Actor
		want bool
	}{
		{"zero", Actor{}, false},
		{"kind only", Actor{Kind: KindHuman}, false},
		{"os user only", Actor{OSUser: "alice"}, false},
		{"human", Human("alice"), true},
		{"agent with os projection", Actor{Kind: KindAgent, ID: "harness-7", OSUser: "agent-7"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Valid(); got != c.want {
				t.Fatalf("Valid() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestActorString(t *testing.T) {
	if got := (Actor{}).String(); got != "unknown" {
		t.Fatalf("zero actor String() = %q, want unknown", got)
	}
	if got := Human("alice").String(); got != "human:alice" {
		t.Fatalf("String() = %q, want human:alice", got)
	}
	if got := (Actor{Kind: KindOperator, ID: "ops"}).String(); got != "operator:ops" {
		t.Fatalf("String() = %q, want operator:ops", got)
	}
}

// TestKindConstants guards against an accidental rename of the wire-ish kind
// strings: later stages persist these in audit events.
func TestKindConstants(t *testing.T) {
	if KindUnknown != "" || KindHuman != "human" || KindAgent != "agent" || KindOperator != "operator" {
		t.Fatalf("Kind constants drifted: %q %q %q %q", KindUnknown, KindHuman, KindAgent, KindOperator)
	}
}
