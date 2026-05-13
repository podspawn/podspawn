package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/policy"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/session"
	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/state"
)

// TestGateLocalCreateNilLocalSessionIsNoop locks in the contract that
// gateLocalCreate tolerates a nil localSession (defensive — should not
// happen in production but the helper must not panic).
func TestGateLocalCreateNilLocalSessionIsNoop(t *testing.T) {
	if err := gateLocalCreate(context.Background(), nil); err != nil {
		t.Fatalf("gateLocalCreate(nil) = %v, want nil", err)
	}
}

// TestGateLocalCreateNilServiceIsNoop covers the case where a test
// localSession was constructed without ls.Service set. Production code
// always populates Service via buildLocalSession; this is purely for
// the test-fallback seam.
func TestGateLocalCreateNilServiceIsNoop(t *testing.T) {
	ls := &localSession{Session: &spawn.Session{Username: "tenant", Actor: identity.Human("tenant")}}
	if err := gateLocalCreate(context.Background(), ls); err != nil {
		t.Fatalf("gateLocalCreate(no Service) = %v, want nil", err)
	}
}

func TestGateLocalCreateAllowsByDefault(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	ls := &localSession{
		Session: &spawn.Session{
			Username: "tenant",
			Actor:    identity.Human("tenant"),
			Runtime:  rt,
			Store:    store,
		},
		Service: session.New(session.Options{
			SessionStore:   store,
			WorkspaceStore: store,
			Runtime:        rt,
		}),
	}
	if err := gateLocalCreate(context.Background(), ls); err != nil {
		t.Fatalf("AllowAll default denied: %v", err)
	}
}

// TestGateLocalCreateDeniesAndCarriesReason is the load-bearing
// regression: it proves the two Stage 8 bypass paths
// (cmd/dev.go, cmd/run.go without --project) now go through the gate,
// preserve errors.Is(err, policy.ErrPolicyDenied), and surface the
// reason on the CLI-visible error text.
func TestGateLocalCreateDeniesAndCarriesReason(t *testing.T) {
	store := state.NewFakeStore()
	rt := runtime.NewFakeRuntime()
	root := t.TempDir()
	auditPath := filepath.Join(root, "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}

	stub := &denyPolicy{reason: "no defaults during change freeze"}
	svc := session.New(session.Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Runtime:        rt,
		Audit:          logger,
		Policy:         stub,
	})

	ls := &localSession{
		Session: &spawn.Session{
			Username: "tenant",
			Actor:    identity.Human("tenant"),
			Runtime:  rt,
			Store:    store,
			Audit:    logger,
		},
		Service: svc,
	}

	err = gateLocalCreate(context.Background(), ls)
	_ = logger.Close()

	if !errors.Is(err, policy.ErrPolicyDenied) {
		t.Fatalf("errors.Is(err, policy.ErrPolicyDenied) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "no defaults during change freeze") {
		t.Errorf("CLI error %q must include the policy reason", err.Error())
	}

	// Gate must have seen the actor + owner, and the op must be
	// session.create (matching what Service.Create's coarse gate
	// produces).
	if stub.captured.Op != policy.OpSessionCreate {
		t.Errorf("op = %q, want %q", stub.captured.Op, policy.OpSessionCreate)
	}
	if stub.captured.Actor.Kind != identity.KindHuman || stub.captured.Actor.OSUser != "tenant" {
		t.Errorf("actor = %+v, want human:tenant", stub.captured.Actor)
	}
	if stub.captured.OwnerUser != "tenant" {
		t.Errorf("OwnerUser = %q, want tenant", stub.captured.OwnerUser)
	}
	// Coarse gate: no workspace/session/branch.
	if stub.captured.Workspace != nil || stub.captured.Session != nil || stub.captured.RequestedBranch != "" {
		t.Errorf("coarse gate should not carry context: %+v", stub.captured)
	}

	// Audit log: exactly one policy.deny record.
	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d: %q", len(lines), string(data))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != audit.EventPolicyDeny || entry["op"] != string(policy.OpSessionCreate) {
		t.Errorf("audit entry mismatch: %+v", entry)
	}
	if entry["reason"] != "no defaults during change freeze" {
		t.Errorf("audit reason = %v, want \"no defaults during change freeze\"", entry["reason"])
	}
}
