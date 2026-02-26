package authkeys

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKeyFile(t *testing.T, dir, username, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, username), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLookupSingleKey(t *testing.T) {
	dir := t.TempDir()
	writeKeyFile(t, dir, "deploy", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5 deploy@workstation\n")

	var buf bytes.Buffer
	n, err := Lookup("deploy", dir, "/usr/local/bin/podspawn", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 key, got %d", n)
	}

	got := buf.String()
	if !strings.Contains(got, `command="/usr/local/bin/podspawn spawn --user deploy"`) {
		t.Errorf("missing command directive in: %s", got)
	}
	if !strings.Contains(got, "restrict,pty,agent-forwarding") {
		t.Errorf("missing key options in: %s", got)
	}
	if !strings.Contains(got, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5") {
		t.Errorf("missing key data in: %s", got)
	}
}

func TestLookupMultipleKeys(t *testing.T) {
	dir := t.TempDir()
	keys := "ssh-ed25519 AAAA1 laptop\nssh-rsa AAAA2 desktop\n"
	writeKeyFile(t, dir, "deploy", keys)

	var buf bytes.Buffer
	n, err := Lookup("deploy", dir, "/opt/podspawn", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 keys, got %d", n)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines, got %d", len(lines))
	}
}

func TestLookupSkipsCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	content := "# my laptop key\n\nssh-ed25519 AAAA1 laptop\n\n# old key\n"
	writeKeyFile(t, dir, "deploy", content)

	var buf bytes.Buffer
	n, err := Lookup("deploy", dir, "/usr/local/bin/podspawn", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 key (skipping comments/blanks), got %d", n)
	}
}

func TestLookupMissingUser(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	n, err := Lookup("nobody", dir, "/usr/local/bin/podspawn", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 keys for missing user, got %d", n)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got %q", buf.String())
	}
}

func TestLookupEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeKeyFile(t, dir, "deploy", "")

	var buf bytes.Buffer
	n, err := Lookup("deploy", dir, "/usr/local/bin/podspawn", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 keys for empty file, got %d", n)
	}
}

func TestLookupPathTraversal(t *testing.T) {
	dir := t.TempDir()

	for _, bad := range []string{"../etc/shadow", "deploy/keys", "../../root"} {
		var buf bytes.Buffer
		_, err := Lookup(bad, dir, "/usr/local/bin/podspawn", &buf)
		if err == nil {
			t.Errorf("expected error for username %q, got nil", bad)
		}
	}
}

func TestLookupBinaryPath(t *testing.T) {
	dir := t.TempDir()
	writeKeyFile(t, dir, "deploy", "ssh-ed25519 AAAA1 laptop\n")

	var buf bytes.Buffer
	Lookup("deploy", dir, "/home/me/podspawn", &buf)

	if !strings.Contains(buf.String(), `command="/home/me/podspawn spawn`) {
		t.Errorf("binary path not reflected in output: %s", buf.String())
	}
}
