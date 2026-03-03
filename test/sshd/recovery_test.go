//go:build sshd

package sshd

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCrashRecoveryContainerGone(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "crash-gone")

	// Create a container via SSH
	client := h.Dial(t, "crash-gone")
	RunCommand(t, client, "echo setup")
	client.Close()

	// Wait a moment for disconnect to register
	time.Sleep(1 * time.Second)

	// Simulate crash: forcibly remove the container while DB record still exists
	h.RemoveUserContainer(t, "crash-gone")

	// Reconnect: podspawn should detect stale record, clean up, and create new
	client2 := h.Dial(t, "crash-gone")
	stdout, _, exitCode := RunCommand(t, client2, "echo recovered")

	if exitCode != 0 {
		t.Fatalf("expected exit 0 after recovery, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "recovered" {
		t.Fatalf("expected 'recovered', got %q", stdout)
	}
}

func TestMaxLifetimeEnforcement(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "lifetime-user")

	// Set a very short max_lifetime for this test
	code, _ := h.ExecInHarness(t, []string{"sh", "-c", `sed -i 's/max_lifetime: 30s/max_lifetime: 3s/' /etc/podspawn/config.yaml`})
	if code != 0 {
		t.Fatalf("sed config to 3s max_lifetime failed with exit %d", code)
	}
	t.Cleanup(func() {
		code, _ := h.ExecInHarness(t, []string{"sh", "-c", `sed -i 's/max_lifetime: 3s/max_lifetime: 30s/' /etc/podspawn/config.yaml`})
		if code != 0 {
			t.Errorf("cleanup: sed config restore to 30s max_lifetime failed with exit %d", code)
		}
	})

	client := h.Dial(t, "lifetime-user")
	RunCommand(t, client, "echo created")
	containerID := h.ContainerID(t, "lifetime-user")
	client.Close()

	// Wait for max_lifetime to expire
	time.Sleep(5 * time.Second)

	// Reconnect should get a new container (old one cleaned up by reconciliation)
	client2 := h.Dial(t, "lifetime-user")
	RunCommand(t, client2, "echo new")

	newID := h.ContainerID(t, "lifetime-user")
	if newID == containerID {
		t.Fatalf("expected new container after max_lifetime, got same: %s", containerID)
	}
}

func TestSQLiteConcurrency(t *testing.T) {
	h := getHarness(t)

	// Create 5 users for concurrent access
	users := []string{"sqlite-1", "sqlite-2", "sqlite-3", "sqlite-4", "sqlite-5"}
	for _, u := range users {
		h.AddUser(t, u)
	}

	// Rapidly connect all 5 users, run a command, disconnect.
	// We avoid passing t into goroutines since t.Fatal from a non-test
	// goroutine panics the runner.
	type result struct {
		user string
		err  error
	}
	results := make(chan result, len(users))

	for _, u := range users {
		go func(user string) {
			conn, err := h.dialE(user)
			if err != nil {
				results <- result{user: user, err: fmt.Errorf("dial: %w", err)}
				return
			}
			defer conn.Close()

			stdout, stderr, exitCode, err := runCommandE(conn, "echo "+user)
			if err != nil {
				results <- result{user: user, err: fmt.Errorf("run: %w", err)}
				return
			}
			if exitCode != 0 {
				results <- result{user: user, err: fmt.Errorf("exit %d, stdout: %q, stderr: %q", exitCode, stdout, stderr)}
				return
			}
			if strings.TrimSpace(stdout) != user {
				results <- result{user: user, err: fmt.Errorf("expected %q, got %q", user, strings.TrimSpace(stdout))}
				return
			}
			results <- result{user: user}
		}(u)
	}

	for range users {
		select {
		case r := <-results:
			if r.err != nil {
				t.Errorf("user %s failed: %v", r.user, r.err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("sqlite concurrency test timed out waiting for goroutines")
		}
	}
}
