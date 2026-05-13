package policy

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
	"github.com/podspawn/podspawn/internal/state"
)

// stubPolicy lets tests assert what Enforce passed into Evaluate and
// shape the returned Decision. captured is the last Request seen.
type stubPolicy struct {
	decision Decision
	captured Request
	calls    int
}

func (s *stubPolicy) Evaluate(_ context.Context, req Request) Decision {
	s.captured = req
	s.calls++
	return s.decision
}

func TestAllowAllAllowsEveryOp(t *testing.T) {
	ops := []Op{
		OpSessionCreate,
		OpSessionAttach,
		OpSessionEnd,
		OpWorkspaceRemove,
		OpWorkspaceBranchSwitch,
	}
	for _, op := range ops {
		d := AllowAll{}.Evaluate(context.Background(), Request{Op: op})
		if !d.Allow {
			t.Errorf("AllowAll denied %s", op)
		}
	}
}

func TestPolicyErrorUnwrapsToSentinel(t *testing.T) {
	pe := &PolicyError{Op: OpWorkspaceRemove, Actor: identity.Operator("root"), Reason: "owner mismatch"}
	if !errors.Is(pe, ErrPolicyDenied) {
		t.Fatal("errors.Is(pe, ErrPolicyDenied) = false; want true")
	}
	if !strings.Contains(pe.Error(), "owner mismatch") {
		t.Errorf("Error() = %q, want it to include the reason", pe.Error())
	}
	if !strings.Contains(pe.Error(), string(OpWorkspaceRemove)) {
		t.Errorf("Error() = %q, want it to include the op", pe.Error())
	}
}

func TestEnforceAllowPathEmitsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := audit.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logger.Close() }()

	stub := &stubPolicy{decision: Decision{Allow: true}}
	got := Enforce(context.Background(), stub, logger, Request{
		Op:        OpSessionCreate,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
	})
	if got != nil {
		t.Fatalf("Enforce error = %v, want nil", got)
	}

	_ = logger.Close()
	data, _ := os.ReadFile(path)
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Fatalf("audit file not empty on allow: %q", string(data))
	}
}

func TestEnforceDenyReturnsTypedErrorAndEmitsAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := audit.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	stub := &stubPolicy{decision: Decision{Allow: false, Reason: "tenant cannot delete shared workspaces"}}
	req := Request{
		Op:        OpWorkspaceRemove,
		Actor:     identity.Operator("root"),
		OwnerUser: "alice",
		Workspace: &state.Workspace{ID: "ws-1", Name: "auth-fix"},
		Session:   &state.Session{ID: "sess-1", ContainerID: "ctr-1"},
		// ContainerID populated by the cmd-side helper, not derived here.
		ContainerID: "ctr-1",
	}
	err = Enforce(context.Background(), stub, logger, req)
	_ = logger.Close()

	if err == nil {
		t.Fatal("Enforce returned nil on deny")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("errors.Is(err, ErrPolicyDenied) = false; err = %v", err)
	}
	var pe *PolicyError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As(err, *PolicyError) = false; err = %v", err)
	}
	if pe.Op != OpWorkspaceRemove || pe.Reason != "tenant cannot delete shared workspaces" {
		t.Errorf("PolicyError = %+v", pe)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit line, got %d: %q", len(lines), string(data))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != audit.EventPolicyDeny {
		t.Errorf("event = %q, want %q", entry["event"], audit.EventPolicyDeny)
	}
	if entry["op"] != string(OpWorkspaceRemove) {
		t.Errorf("op = %q, want %q", entry["op"], OpWorkspaceRemove)
	}
	if entry["reason"] != "tenant cannot delete shared workspaces" {
		t.Errorf("reason mismatch: %v", entry["reason"])
	}
	if entry["user"] != "alice" {
		t.Errorf("user = %q, want alice (owner)", entry["user"])
	}
	if entry["actor"] != "operator:root" {
		t.Errorf("actor = %q, want operator:root", entry["actor"])
	}
	if entry["workspace_id"] != "ws-1" || entry["session_id"] != "sess-1" || entry["container_id"] != "ctr-1" {
		t.Errorf("identity ids missing: ws=%v sess=%v ctr=%v", entry["workspace_id"], entry["session_id"], entry["container_id"])
	}
	// RequestedBranch is decision input only — never audited.
	if _, present := entry["requested_branch"]; present {
		t.Errorf("RequestedBranch leaked into audit: %v", entry["requested_branch"])
	}
}

func TestEnforceNilPolicyDefaultsToAllow(t *testing.T) {
	err := Enforce(context.Background(), nil, nil, Request{
		Op:        OpSessionCreate,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
	})
	if err != nil {
		t.Fatalf("Enforce(nil policy) error = %v, want nil", err)
	}
}

func TestEnforceNilLoggerDoesNotPanicOnDeny(t *testing.T) {
	stub := &stubPolicy{decision: Decision{Allow: false, Reason: "blocked"}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Enforce panicked with nil logger: %v", r)
		}
	}()
	err := Enforce(context.Background(), stub, nil, Request{
		Op:    OpSessionCreate,
		Actor: identity.Human("tenant"),
	})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err = %v, want ErrPolicyDenied", err)
	}
}

func TestContainerIDOf(t *testing.T) {
	if got := ContainerIDOf(nil); got != "" {
		t.Errorf("ContainerIDOf(nil) = %q, want \"\"", got)
	}
	if got := ContainerIDOf(&state.Session{ContainerID: "ctr-9"}); got != "ctr-9" {
		t.Errorf("ContainerIDOf = %q, want ctr-9", got)
	}
}

func TestSubjectOfOmitsEmptyIDs(t *testing.T) {
	s := subjectOf(Request{
		Op:        OpSessionCreate,
		Actor:     identity.Human("tenant"),
		OwnerUser: "tenant",
	})
	if s.WorkspaceID != "" || s.SessionID != "" || s.ContainerID != "" {
		t.Fatalf("expected empty ids, got %+v", s)
	}
}
