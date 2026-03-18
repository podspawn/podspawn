//go:build sshd

package sshd

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestRemoteCommand(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "cmd-user")

	client := h.Dial(t, "cmd-user")
	stdout, _, exitCode := RunCommand(t, client, "echo hello")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestExitCodePropagation(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "exit-user")

	client := h.Dial(t, "exit-user")
	_, _, exitCode := RunCommand(t, client, "exit 42")

	if exitCode != 42 {
		t.Fatalf("expected exit 42, got %d", exitCode)
	}
}

func TestSFTPRoundTrip(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "sftp-user")

	client := h.Dial(t, "sftp-user")

	// Install sftp-server in the user's container (ubuntu:24.04 doesn't include it)
	_, _, exitCode := RunCommand(t, client, "sudo apt-get update -qq && sudo apt-get install -y -qq openssh-sftp-server >/dev/null 2>&1")
	if exitCode != 0 {
		t.Skip("could not install openssh-sftp-server in container")
	}

	sc := SFTPClient(t, client)

	content := []byte("podspawn sftp test payload")

	f, err := sc.Create("/tmp/sftp-test.txt")
	if err != nil {
		t.Fatalf("creating remote file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("writing remote file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing remote file after write: %v", err)
	}

	f, err = sc.Open("/tmp/sftp-test.txt")
	if err != nil {
		t.Fatalf("opening remote file: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("reading remote file: %v", err)
	}
	f.Close()

	if string(buf[:n]) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", string(buf[:n]), string(content))
	}
}

func TestSCPUploadDownload(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "scp-user")

	client := h.Dial(t, "scp-user")

	content := []byte("scp round-trip payload")
	SCPUpload(t, client, "/tmp/scp-test.txt", content)

	downloaded := SCPDownload(t, client, "/tmp/scp-test.txt")
	if string(downloaded) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", string(downloaded), string(content))
	}
}

func TestSpecialCharacters(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "special-user")

	client := h.Dial(t, "special-user")
	stdout, _, exitCode := RunCommand(t, client, `echo "hello world" | wc -w`)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout); got != "2" {
		t.Fatalf("expected '2', got %q", got)
	}
}

func TestLargeFileTransfer(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "large-user")

	client := h.Dial(t, "large-user")

	// Install sftp-server in the user's container
	_, _, exitCode := RunCommand(t, client, "sudo apt-get update -qq && sudo apt-get install -y -qq openssh-sftp-server >/dev/null 2>&1")
	if exitCode != 0 {
		t.Skip("could not install openssh-sftp-server in container")
	}

	sc := SFTPClient(t, client)

	// 10MB file (not 100MB to keep tests fast)
	payload := make([]byte, 10*1024*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("generating random payload: %v", err)
	}
	localHash := sha256.Sum256(payload)

	f, err := sc.Create("/tmp/large-test.bin")
	if err != nil {
		t.Fatalf("creating remote file: %v", err)
	}
	if _, err := f.Write(payload); err != nil {
		t.Fatalf("writing large file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing remote file after write: %v", err)
	}

	// Read it back and verify hash
	stdout, _, exitCode := RunCommand(t, client, "sha256sum /tmp/large-test.bin")
	if exitCode != 0 {
		t.Fatalf("sha256sum failed with exit %d", exitCode)
	}
	fields := strings.Fields(strings.TrimSpace(stdout))
	if len(fields) == 0 {
		t.Fatalf("sha256sum returned empty output")
	}
	remoteHash := fields[0]
	expectedHash := fmt.Sprintf("%x", localHash)

	if remoteHash != expectedHash {
		t.Fatalf("hash mismatch: local=%s remote=%s", expectedHash, remoteHash)
	}
}

func TestLongRunningCommand(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "long-user")

	client := h.Dial(t, "long-user")
	stdout, _, exitCode := RunCommand(t, client, "sleep 5 && echo done")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout); got != "done" {
		t.Fatalf("expected 'done', got %q", got)
	}
}

func TestMultipleCommandsPerSession(t *testing.T) {
	h := getHarness(t)
	h.AddUser(t, "multi-user")

	client := h.Dial(t, "multi-user")

	// Each RunCommand opens a new SSH session on the same connection.
	// This tests that the container stays alive between sessions.
	RunCommand(t, client, "echo first > /tmp/multi-test.txt")

	stdout, _, exitCode := RunCommand(t, client, "cat /tmp/multi-test.txt")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout); got != "first" {
		t.Fatalf("expected 'first', got %q", got)
	}
}
