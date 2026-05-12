package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/podspawn/podspawn/internal/identity"
)

func devSubject() Subject {
	return Subject{
		User:        "deploy",
		Actor:       identity.Human("deploy"),
		WorkspaceID: "ws-7",
		SessionID:   "sess-7",
		ContainerID: "ctr-7",
	}
}

func TestLogWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	s := devSubject()
	logger.Connect(s, "backend", "podspawn-deploy-backend", 1)
	logger.Command(s, "backend", "npm test", 0)
	logger.Disconnect(s, "backend", "podspawn-deploy-backend", 0)
	logger.ContainerDestroy(s, "backend", "podspawn-deploy-backend", "grace_expired")

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 audit lines, got %d", len(lines))
	}

	events := []string{EventConnect, EventCommand, EventDisconnect, EventDestroy}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
		if entry["event"] != events[i] {
			t.Errorf("line %d: event = %q, want %q", i, entry["event"], events[i])
		}
		if entry["user"] != "deploy" {
			t.Errorf("line %d: user = %q, want deploy", i, entry["user"])
		}
		if _, ok := entry["timestamp"]; !ok {
			t.Errorf("line %d: missing timestamp", i)
		}
		// Preserved payload keys. `container` rides on connect/disconnect/
		// destroy; `session.command` carries `command` instead.
		if entry["project"] != "backend" {
			t.Errorf("line %d: project = %q, want backend", i, entry["project"])
		}
		if events[i] == EventCommand {
			if entry["command"] != "npm test" {
				t.Errorf("line %d: command = %q, want npm test", i, entry["command"])
			}
		} else if entry["container"] != "podspawn-deploy-backend" {
			t.Errorf("line %d: container = %q, want podspawn-deploy-backend", i, entry["container"])
		}
		// New identity keys.
		if entry["actor"] != "human:deploy" {
			t.Errorf("line %d: actor = %q, want human:deploy", i, entry["actor"])
		}
		if entry["actor_kind"] != "human" {
			t.Errorf("line %d: actor_kind = %q, want human", i, entry["actor_kind"])
		}
		if entry["workspace_id"] != "ws-7" {
			t.Errorf("line %d: workspace_id = %q, want ws-7", i, entry["workspace_id"])
		}
		if entry["session_id"] != "sess-7" {
			t.Errorf("line %d: session_id = %q, want sess-7", i, entry["session_id"])
		}
		if entry["container_id"] != "ctr-7" {
			t.Errorf("line %d: container_id = %q, want ctr-7", i, entry["container_id"])
		}
	}
}

func TestCommandCarriesExitCodeAndOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.Command(devSubject(), "backend", "go test ./...", 0)
	logger.Command(devSubject(), "backend", "false", 1)
	_ = logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var ok0, fail1 map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &ok0); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &fail1); err != nil {
		t.Fatal(err)
	}
	if ok0["command"] != "go test ./..." {
		t.Errorf("command = %q", ok0["command"])
	}
	if ok0["exit_code"].(float64) != 0 || ok0["outcome"] != "success" {
		t.Errorf("exit 0: exit_code=%v outcome=%v", ok0["exit_code"], ok0["outcome"])
	}
	if fail1["exit_code"].(float64) != 1 || fail1["outcome"] != "failure" {
		t.Errorf("exit 1: exit_code=%v outcome=%v", fail1["exit_code"], fail1["outcome"])
	}
}

func TestOperatorActorDistinctFromOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.ContainerDestroy(Subject{
		User:        "alice",
		Actor:       identity.Operator("root"),
		SessionID:   "sess-1",
		ContainerID: "ctr-1",
	}, "backend", "podspawn-alice-backend", "stop")
	_ = logger.Close()

	entry := readSingle(t, path)
	if entry["user"] != "alice" {
		t.Errorf("user = %q, want alice (owner)", entry["user"])
	}
	if entry["actor"] != "operator:root" {
		t.Errorf("actor = %q, want operator:root", entry["actor"])
	}
	if entry["actor_kind"] != "operator" {
		t.Errorf("actor_kind = %q, want operator", entry["actor_kind"])
	}
	if entry["reason"] != "stop" {
		t.Errorf("reason = %q, want stop", entry["reason"])
	}
	if _, present := entry["workspace_id"]; present {
		t.Errorf("workspace_id should be omitted when empty, got %v", entry["workspace_id"])
	}
}

func TestEmptyIdentityFieldsOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Default-image / server-mode shape: no workspace, no session row yet.
	logger.Connect(Subject{User: "bob", Actor: identity.Human("bob")}, "", "podspawn-bob", 1)
	_ = logger.Close()

	entry := readSingle(t, path)
	for _, k := range []string{"workspace_id", "session_id", "container_id"} {
		if _, present := entry[k]; present {
			t.Errorf("%s should be omitted when empty, got %v", k, entry[k])
		}
	}
	if entry["actor"] != "human:bob" {
		t.Errorf("actor = %q, want human:bob", entry["actor"])
	}
}

func TestNilLoggerIsNoop(t *testing.T) {
	var logger *Logger
	s := Subject{User: "deploy", Actor: identity.Human("deploy")}
	// These should not panic.
	logger.Connect(s, "", "ctr", 1)
	logger.Command(s, "", "ls", 0)
	logger.Disconnect(s, "", "ctr", 0)
	logger.ContainerCreate(s, "", "ctr", "ubuntu:24.04")
	logger.ContainerDestroy(s, "", "ctr", "disconnect")
	logger.WorkspaceCreate(s, "dev", "backend", "main", "/srv/ws/dev")
	logger.WorkspaceDelete(s, "dev", "backend", "main", "/srv/ws/dev", "rm")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenEmptyPathReturnsNil(t *testing.T) {
	logger, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if logger != nil {
		t.Error("empty path should return nil logger")
	}
}

func TestConcurrentLogging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			user := fmt.Sprintf("worker-%d", n)
			logger.Connect(Subject{User: user, Actor: identity.Human(user)}, "proj", "ctr", 1)
		}(i)
	}
	wg.Wait()

	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != workers {
		t.Fatalf("expected %d audit lines, got %d", workers, len(lines))
	}

	seenWorkers := make(map[string]bool)
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
		if user, ok := entry["user"].(string); ok {
			seenWorkers[user] = true
		}
		if entry["event"] != EventConnect {
			t.Errorf("line %d: event = %v, want %s", i, entry["event"], EventConnect)
		}
	}
	if len(seenWorkers) != workers {
		t.Errorf("expected %d distinct workers, got %d", workers, len(seenWorkers))
	}
}

func TestContainerCreateEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	logger.ContainerCreate(Subject{
		User: "ci-agent", Actor: identity.Human("ci-agent"), SessionID: "s1", ContainerID: "c1",
	}, "pipeline", "podspawn-ci-pipeline", "node:22")
	_ = logger.Close()

	entry := readSingle(t, path)
	if entry["event"] != EventCreate {
		t.Errorf("event = %q, want %q", entry["event"], EventCreate)
	}
	if entry["image"] != "node:22" {
		t.Errorf("image = %q, want node:22", entry["image"])
	}
	if entry["project"] != "pipeline" {
		t.Errorf("project = %q, want pipeline", entry["project"])
	}
	if entry["session_id"] != "s1" || entry["container_id"] != "c1" {
		t.Errorf("ids: session_id=%v container_id=%v", entry["session_id"], entry["container_id"])
	}
}

func TestWorkspaceEventsKeepPayloadKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := Subject{User: "tenant", Actor: identity.Human("tenant"), WorkspaceID: "ws-9"}
	logger.WorkspaceCreate(s, "auth-fix", "backend", "feat/auth", "/srv/ws/auth-fix")
	logger.WorkspaceDelete(s, "auth-fix", "backend", "feat/auth", "/srv/ws/auth-fix", "rm")
	_ = logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	var create, del map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &create); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &del); err != nil {
		t.Fatal(err)
	}
	if create["event"] != EventWorkspaceCreate {
		t.Errorf("create event = %q, want %q", create["event"], EventWorkspaceCreate)
	}
	if del["event"] != EventWorkspaceDelete {
		t.Errorf("delete event = %q, want %q", del["event"], EventWorkspaceDelete)
	}
	for _, e := range []map[string]any{create, del} {
		if e["name"] != "auth-fix" || e["project"] != "backend" || e["branch"] != "feat/auth" || e["workspace"] != "/srv/ws/auth-fix" {
			t.Errorf("workspace payload keys not preserved: %v", e)
		}
		if e["workspace_id"] != "ws-9" {
			t.Errorf("workspace_id = %q, want ws-9", e["workspace_id"])
		}
		if e["actor"] != "human:tenant" {
			t.Errorf("actor = %q, want human:tenant", e["actor"])
		}
	}
	if del["reason"] != "rm" {
		t.Errorf("delete reason = %q, want rm", del["reason"])
	}
}

func readSingle(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	return entry
}
