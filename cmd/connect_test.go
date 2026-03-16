package cmd

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRelayBidirectional(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		conn.Write(buf[:n]) //nolint:errcheck
	}()

	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	relayDone := make(chan error, 1)
	go func() {
		relayDone <- relay(ln.Addr().String(), stdinR, &stdout)
	}()

	stdinW.Write([]byte("ping")) //nolint:errcheck
	_ = stdinW.Close()

	select {
	case err := <-relayDone:
		if err != nil {
			t.Fatalf("relay error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay timed out")
	}

	if got := stdout.String(); got != "ping" {
		t.Errorf("stdout = %q, want %q", got, "ping")
	}
}

func TestRelayClosesOnServerEOF(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() //nolint:errcheck

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close() //nolint:errcheck
	var stdout bytes.Buffer

	relayDone := make(chan error, 1)
	go func() {
		relayDone <- relay(ln.Addr().String(), stdinR, &stdout)
	}()

	select {
	case err := <-relayDone:
		if err != nil {
			t.Fatalf("relay error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay should return when server closes")
	}
}

func TestRelayDialFailure(t *testing.T) {
	err := relay("127.0.0.1:1", strings.NewReader(""), io.Discard)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error should mention address, got: %v", err)
	}
}

func TestResolveTargetDirectHostname(t *testing.T) {
	got, err := resolveTarget("devbox.company.com", "22")
	if err != nil {
		t.Fatal(err)
	}
	if got != "devbox.company.com:22" {
		t.Errorf("resolved = %q, want devbox.company.com:22", got)
	}
}

func TestResolveTargetPodHostname(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".podspawn")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	yaml := `
servers:
  default: fallback.example.com
  mappings:
    work.pod: devbox.company.com
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	got, err := resolveTarget("work.pod", "2222")
	if err != nil {
		t.Fatal(err)
	}
	if got != "devbox.company.com:2222" {
		t.Errorf("resolved = %q, want devbox.company.com:2222", got)
	}
}

func TestResolveTargetPodNoConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := resolveTarget("work.pod", "22")
	if err == nil {
		t.Fatal("expected error for .pod without config")
	}
}

func TestResolveTargetLocalhostPod(t *testing.T) {
	// localhost.pod resolves without any config file
	t.Setenv("HOME", t.TempDir())
	got, err := resolveTarget("localhost.pod", "22")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:22" {
		t.Errorf("resolved = %q, want 127.0.0.1:22", got)
	}
}

func TestResolveTargetLocalhostPodCustomPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := resolveTarget("localhost.pod", "2222")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:2222" {
		t.Errorf("resolved = %q, want 127.0.0.1:2222", got)
	}
}
