//go:build sshd

package sshd

import (
	"testing"
)

func TestCPULimitsApplied(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "cpu-user")

	client := h.Dial(t, "cpu-user")
	RunCommand(t, client, "echo trigger-container")

	cpus, _ := h.ContainerResources(t, "cpu-user")
	if cpus != 1.0 {
		t.Fatalf("expected 1.0 CPUs, got %f", cpus)
	}
}

func TestMemoryLimitsApplied(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "mem-user")

	client := h.Dial(t, "mem-user")
	RunCommand(t, client, "echo trigger-container")

	_, memBytes := h.ContainerResources(t, "mem-user")
	// 256m = 256 * 1024 * 1024 = 268435456
	expected := int64(268435456)
	if memBytes != expected {
		t.Fatalf("expected %d bytes memory, got %d", expected, memBytes)
	}
}
