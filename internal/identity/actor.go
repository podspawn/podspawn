// Package identity defines who is acting in a Podspawn session: the Actor.
//
// It is a leaf package on purpose (imports only the standard library) so the
// session, spawn, and audit packages can all depend on it without an import
// cycle. Stage 6 introduces only the type and the contract; later stages
// forward the Actor onto spawn.Session and audit events.
package identity

import "fmt"

// Kind classifies an Actor. The zero value (KindUnknown) is not a valid actor;
// session.Service.Create rejects a CreateRequest whose Actor is not Valid.
type Kind string

const (
	KindUnknown  Kind = ""
	KindHuman    Kind = "human"
	KindAgent    Kind = "agent"
	KindOperator Kind = "operator"
)

// Actor is the external party an operation is attributed to.
//
//   - For a human (SSH attach or local CLI) ID and OSUser are both the OS
//     username.
//   - For an agent or harness caller (not yet wired) ID is the caller-supplied
//     identity and OSUser is the OS username it projects to inside the runtime.
//
// OSUser is the bridge to the OS execution identity: the in-container user that
// commands actually run as. Keeping that mapping on the Actor (and, later, on
// the session record) is what lets OS-level execution be attributed back to a
// concrete caller without changing the runtime.
type Actor struct {
	Kind   Kind
	ID     string
	OSUser string
	Label  string
}

// Human builds the common case: a person identified by their OS username.
func Human(osUser string) Actor {
	return Actor{Kind: KindHuman, ID: osUser, OSUser: osUser}
}

// Operator builds the actor for an admin acting on a session they do not own:
// the OS user running the command, tagged as an operator rather than the
// session's owner. Used by `podspawn stop` in server mode when the invoking
// OS user differs from the target session's user.
func Operator(osUser string) Actor {
	return Actor{Kind: KindOperator, ID: osUser, OSUser: osUser}
}

// Valid reports whether the actor is usable: it has a kind and an OS user to
// project to. It does not check ID, which agent callers populate differently.
func (a Actor) Valid() bool {
	return a.Kind != KindUnknown && a.OSUser != ""
}

func (a Actor) String() string {
	if a.Kind == KindUnknown && a.ID == "" {
		return "unknown"
	}
	return fmt.Sprintf("%s:%s", a.Kind, a.ID)
}
