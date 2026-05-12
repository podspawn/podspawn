package spawn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podspawn/podspawn/internal/audit"
	"github.com/podspawn/podspawn/internal/identity"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/state"
)

func readAuditLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("audit line not JSON: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func auditedSession(t *testing.T, fake *runtime.FakeRuntime, store state.SessionStore, auditPath string) *Session {
	t.Helper()
	logger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return &Session{
		Username:    "dev",
		Actor:       identity.Human("dev"),
		Runtime:     fake,
		Image:       "ubuntu:24.04",
		Shell:       "/bin/bash",
		Store:       store,
		LockDir:     t.TempDir(),
		MaxLifetime: 8 * time.Hour,
		Mode:        "destroy-on-disconnect",
		Audit:       logger,
	}
}

func TestAuditEventsCarryActorAndIDs(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	// Exec order on a fresh container: 1) user-setup script (must be 0),
	// 2) the requested command. FakeRuntime does not parse "exit N".
	fake.ExitCodes = []int{0, 7}
	store := state.NewFakeStore()
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	sess := auditedSession(t, fake, store, auditPath)

	t.Setenv("SSH_ORIGINAL_COMMAND", "exit 7")
	exitCode := sess.RunAndCleanup(context.Background())
	if exitCode != 7 {
		t.Fatalf("exit code = %d, want 7", exitCode)
	}

	entries := readAuditLines(t, auditPath)
	byEvent := map[string]map[string]any{}
	cmdCount := 0
	for _, e := range entries {
		if ev, _ := e["event"].(string); ev == audit.EventCommand {
			cmdCount++
		}
		byEvent[e["event"].(string)] = e
	}

	for _, ev := range []string{audit.EventCreate, audit.EventConnect, audit.EventCommand, audit.EventDestroy} {
		e, ok := byEvent[ev]
		if !ok {
			t.Fatalf("missing audit event %q; got %v", ev, byEvent)
		}
		if e["actor"] != "human:dev" {
			t.Errorf("%s: actor = %q, want human:dev", ev, e["actor"])
		}
		if e["actor_kind"] != "human" {
			t.Errorf("%s: actor_kind = %q, want human", ev, e["actor_kind"])
		}
		if id, _ := e["session_id"].(string); id == "" {
			t.Errorf("%s: session_id missing", ev)
		}
		if id, _ := e["container_id"].(string); id == "" {
			t.Errorf("%s: container_id missing", ev)
		}
	}

	// Single post-exec command record, carrying the real exit code.
	if cmdCount != 1 {
		t.Fatalf("session.command emitted %d times, want exactly 1", cmdCount)
	}
	cmd := byEvent[audit.EventCommand]
	if cmd["command"] != "exit 7" {
		t.Errorf("command = %q, want \"exit 7\"", cmd["command"])
	}
	if cmd["exit_code"].(float64) != 7 || cmd["outcome"] != "failure" {
		t.Errorf("exit_code = %v outcome = %v, want 7 / failure", cmd["exit_code"], cmd["outcome"])
	}
	if byEvent[audit.EventDestroy]["reason"] != "disconnect" {
		t.Errorf("destroy reason = %q, want disconnect", byEvent[audit.EventDestroy]["reason"])
	}
}

func TestAuditConnectOnReattachCarriesIDs(t *testing.T) {
	fake := runtime.NewFakeRuntime()
	store := state.NewFakeStore()

	now := time.Now().UTC()
	if err := store.CreateSession(&state.Session{
		User:          "dev",
		Project:       "",
		ContainerID:   "ctr-existing",
		ContainerName: "podspawn-dev",
		Image:         "ubuntu:24.04",
		Status:        state.StatusRunning,
		Connections:   1,
		CreatedAt:     now,
		LastActivity:  now,
		MaxLifetime:   now.Add(8 * time.Hour),
		Mode:          "grace-period",
	}); err != nil {
		t.Fatal(err)
	}
	fake.Containers["podspawn-dev"] = true

	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	sess := auditedSession(t, fake, store, auditPath)
	sess.Mode = "persistent" // keep the container alive on disconnect

	t.Setenv("SSH_ORIGINAL_COMMAND", "true")
	if _, err := sess.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	entries := readAuditLines(t, auditPath)
	var connect map[string]any
	for _, e := range entries {
		if e["event"] == audit.EventConnect {
			connect = e
		}
	}
	if connect == nil {
		t.Fatalf("no session.connect event; got %v", entries)
	}
	if connect["container_id"] != "ctr-existing" {
		t.Errorf("container_id = %q, want ctr-existing", connect["container_id"])
	}
	if id, _ := connect["session_id"].(string); id == "" {
		t.Errorf("session_id missing on reattach connect")
	}
	if connect["actor"] != "human:dev" {
		t.Errorf("actor = %q, want human:dev", connect["actor"])
	}
}
