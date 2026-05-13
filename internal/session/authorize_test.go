package session

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
	"github.com/podspawn/podspawn/internal/state"
)

// fakePolicy is a service-level deny-stub. Mirror of the one in
// internal/policy/policy_test.go but exposed within the session package
// so the gating tests for Create/End don't need to construct one twice.
type fakePolicy struct {
	decision policy.Decision
	captured policy.Request
	calls    int
}

func (f *fakePolicy) Evaluate(_ context.Context, req policy.Request) policy.Decision {
	f.captured = req
	f.calls++
	return f.decision
}

func TestServiceAuthorizeAllowsByDefault(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{SessionStore: store, WorkspaceStore: store})
	err := svc.Authorize(context.Background(), policy.Request{
		Op:        policy.OpSessionCreate,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
	})
	if err != nil {
		t.Fatalf("default Authorize denied: %v", err)
	}
}

func TestServiceAuthorizeAllowsWhenAllowAllExplicit(t *testing.T) {
	store := state.NewFakeStore()
	svc := New(Options{SessionStore: store, WorkspaceStore: store, Policy: policy.AllowAll{}})
	err := svc.Authorize(context.Background(), policy.Request{
		Op:        policy.OpSessionAttach,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
	})
	if err != nil {
		t.Fatalf("AllowAll denied: %v", err)
	}
}

func TestServiceAuthorizeDeniesAndEmitsAudit(t *testing.T) {
	store := state.NewFakeStore()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	stub := &fakePolicy{decision: policy.Decision{Allow: false, Reason: "blocked in test"}}
	svc := New(Options{
		SessionStore:   store,
		WorkspaceStore: store,
		Audit:          logger,
		Policy:         stub,
	})

	err = svc.Authorize(context.Background(), policy.Request{
		Op:        policy.OpWorkspaceRemove,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
		Workspace: &state.Workspace{ID: "ws-1"},
	})
	_ = logger.Close()
	if err == nil {
		t.Fatal("Authorize returned nil on deny")
	}
	if !errors.Is(err, policy.ErrPolicyDenied) {
		t.Fatalf("errors.Is(err, policy.ErrPolicyDenied) = false; err = %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("policy calls = %d, want 1", stub.calls)
	}

	data, _ := os.ReadFile(auditPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d", len(lines))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != audit.EventPolicyDeny || entry["op"] != string(policy.OpWorkspaceRemove) {
		t.Errorf("audit entry mismatched: %+v", entry)
	}
	if entry["reason"] != "blocked in test" {
		t.Errorf("reason = %v, want \"blocked in test\"", entry["reason"])
	}
}
