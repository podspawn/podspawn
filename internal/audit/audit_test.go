package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLogWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	logger.Connect("deploy", "backend", "podspawn-deploy-backend", 1)
	logger.Command("deploy", "backend", "npm test")
	logger.Disconnect("deploy", "backend", "podspawn-deploy-backend", 0)
	logger.ContainerDestroy("deploy", "backend", "podspawn-deploy-backend", "grace_expired")

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

	// Verify each line is valid JSON with expected fields
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
	}
}

func TestNilLoggerIsNoop(t *testing.T) {
	var logger *Logger
	// These should not panic
	logger.Connect("deploy", "", "ctr", 1)
	logger.Command("deploy", "", "ls")
	logger.Disconnect("deploy", "", "ctr", 0)
	logger.ContainerCreate("deploy", "", "ctr", "ubuntu:24.04")
	logger.ContainerDestroy("deploy", "", "ctr", "disconnect")
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
			logger.Connect(user, "proj", "ctr", 1)
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

	logger.ContainerCreate("ci-agent", "pipeline", "podspawn-ci-pipeline", "node:22")
	_ = logger.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry["event"] != EventCreate {
		t.Errorf("event = %q, want %q", entry["event"], EventCreate)
	}
	if entry["image"] != "node:22" {
		t.Errorf("image = %q, want node:22", entry["image"])
	}
}
