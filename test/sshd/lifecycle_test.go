//go:build sshd

package sshd

import (
	"strings"
	"testing"
	"time"
)

func TestGracePeriodReconnect(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "grace-reconnect")

	// First connection creates the container
	client1 := h.Dial(t, "grace-reconnect")
	stdout, _, _ := RunCommand(t, client1, "echo alive")
	if strings.TrimSpace(stdout) != "alive" {
		t.Fatalf("first connection failed: got %q", stdout)
	}
	containerID := h.ContainerID(t, "grace-reconnect")
	client1.Close()

	// Reconnect within grace period (5s in test config)
	time.Sleep(2 * time.Second)

	client2 := h.Dial(t, "grace-reconnect")
	stdout, _, _ = RunCommand(t, client2, "echo still-alive")
	if strings.TrimSpace(stdout) != "still-alive" {
		t.Fatalf("reconnect failed: got %q", stdout)
	}

	// Same container should be reused
	newID := h.ContainerID(t, "grace-reconnect")
	if newID != containerID {
		t.Fatalf("container changed after reconnect within grace period: %s → %s", containerID, newID)
	}
}

func TestGracePeriodExpiry(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "grace-expire")

	client := h.Dial(t, "grace-expire")
	RunCommand(t, client, "echo setup")
	containerID := h.ContainerID(t, "grace-expire")
	client.Close()

	// Wait for grace period to expire (5s in test config + buffer)
	time.Sleep(7 * time.Second)

	// Reconnect should trigger reconciliation and create a new container
	client2 := h.Dial(t, "grace-expire")
	RunCommand(t, client2, "echo new-container")

	newID := h.ContainerID(t, "grace-expire")
	if newID == containerID {
		t.Fatalf("expected new container after grace period expiry, got same ID: %s", containerID)
	}
}

func TestConcurrentSessionsSameUser(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "concurrent-same")

	client1 := h.Dial(t, "concurrent-same")
	client2 := h.Dial(t, "concurrent-same")

	stdout1, _, _ := RunCommand(t, client1, "echo from-session-1")
	stdout2, _, _ := RunCommand(t, client2, "echo from-session-2")

	if strings.TrimSpace(stdout1) != "from-session-1" {
		t.Fatalf("session 1 failed: %q", stdout1)
	}
	if strings.TrimSpace(stdout2) != "from-session-2" {
		t.Fatalf("session 2 failed: %q", stdout2)
	}

	// Both sessions should share the same container
	// We can verify by writing a file in session 1 and reading in session 2
	RunCommand(t, client1, "echo shared > /tmp/concurrent-test.txt")
	stdout, _, _ := RunCommand(t, client2, "cat /tmp/concurrent-test.txt")
	if strings.TrimSpace(stdout) != "shared" {
		t.Fatalf("sessions don't share container: got %q from session 2", stdout)
	}
}

func TestConcurrentSessionsDifferentUsers(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "tenant-a")
	h.AddUser(t, "tenant-b")

	clientA := h.Dial(t, "tenant-a")
	clientB := h.Dial(t, "tenant-b")

	RunCommand(t, clientA, "echo tenant-a-was-here > /tmp/user-marker.txt")
	RunCommand(t, clientB, "echo tenant-b-was-here > /tmp/user-marker.txt")

	// Each user gets their own container, so reading the marker should
	// show their own value, not the other user's.
	stdoutA, _, _ := RunCommand(t, clientA, "cat /tmp/user-marker.txt")
	stdoutB, _, _ := RunCommand(t, clientB, "cat /tmp/user-marker.txt")

	if strings.TrimSpace(stdoutA) != "tenant-a-was-here" {
		t.Fatalf("tenant-a's container leaked: got %q", stdoutA)
	}
	if strings.TrimSpace(stdoutB) != "tenant-b-was-here" {
		t.Fatalf("tenant-b's container leaked: got %q", stdoutB)
	}

	// Container IDs should differ
	idA := h.ContainerID(t, "tenant-a")
	idB := h.ContainerID(t, "tenant-b")
	if idA == idB {
		t.Fatalf("tenant-a and tenant-b share a container: %s", idA)
	}
}

func TestDestroyOnDisconnect(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "destroy-user")

	// Change config to destroy-on-disconnect mode inside the sshd container
	code, _ := h.ExecInHarness(t, []string{"sh", "-c", `sed -i 's/mode: "grace-period"/mode: "destroy-on-disconnect"/' /etc/podspawn/config.yaml`})
	if code != 0 {
		t.Fatalf("sed config to destroy-on-disconnect failed with exit %d", code)
	}
	// Restore after test
	t.Cleanup(func() {
		code, _ := h.ExecInHarness(t, []string{"sh", "-c", `sed -i 's/mode: "destroy-on-disconnect"/mode: "grace-period"/' /etc/podspawn/config.yaml`})
		if code != 0 {
			t.Errorf("cleanup: sed config restore to grace-period failed with exit %d", code)
		}
	})

	client := h.Dial(t, "destroy-user")
	RunCommand(t, client, "echo doomed")
	client.Close()

	// Container should be gone almost immediately
	h.WaitContainerGone(t, "destroy-user", 10*time.Second)
}
