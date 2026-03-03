//go:build sshd

package sshd

import (
	"fmt"
	"os"
	"testing"
)

var harness *SSHHarness

func TestMain(m *testing.M) {
	// We can't use testing.T in TestMain, so set up the harness using
	// a bare-bones approach: if setup fails, print and exit.
	h, err := SetupHarness()
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness setup: %v\n", err)
		os.Exit(1)
	}
	harness = h

	code := m.Run()

	harness.TeardownAll()
	os.Exit(code)
}

func getHarness(t *testing.T) *SSHHarness {
	t.Helper()
	if harness == nil {
		t.Fatal("harness not initialized")
	}
	return harness
}
