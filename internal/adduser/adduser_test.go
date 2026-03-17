package adduser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	valid := []string{"deploy", "ci-runner", "alice", "user_123"}
	for _, name := range valid {
		if err := ValidateUsername(name); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "../etc", "foo/bar", "a..b", " spaces", "--flag", "UPPER", ".hidden", strings.Repeat("a", 33)}
	for _, name := range invalid {
		if err := ValidateUsername(name); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want error", name)
		}
	}
}

func TestValidateSSHKey(t *testing.T) {
	valid := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz deploy@workstation",
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB user@host",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHA test",
		"sk-ssh-ed25519 AAAAGnNrLXNzaC1lZDI1NTE5 security-key",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz",
	}
	for _, key := range valid {
		if err := ValidateSSHKey(key); err != nil {
			t.Errorf("ValidateSSHKey(%q) = %v, want nil", key, err)
		}
	}

	invalid := []string{
		"",
		"not-a-key",
		"ssh-ed25519",
		"garbage AAAA data extra",
	}
	for _, key := range invalid {
		if err := ValidateSSHKey(key); err == nil {
			t.Errorf("ValidateSSHKey(%q) = nil, want error", key)
		}
	}
}

func TestWriteKeys(t *testing.T) {
	dir := t.TempDir()
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz deploy@work",
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB deploy@laptop",
	}

	n, err := WriteKeys(dir, "deploy", keys)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("wrote %d keys, want 2", n)
	}

	data, err := os.ReadFile(filepath.Join(dir, "deploy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if !strings.Contains(string(data), key) {
			t.Errorf("key file missing %q", key)
		}
	}
}

func TestWriteKeysAppends(t *testing.T) {
	dir := t.TempDir()
	first := []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz first-key"}
	second := []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB second-key"}

	if _, err := WriteKeys(dir, "deploy", first); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteKeys(dir, "deploy", second); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "deploy"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "first-key") || !strings.Contains(content, "second-key") {
		t.Errorf("expected both keys in file, got:\n%s", content)
	}
}

func TestWriteKeysCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "keys")
	keys := []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz test"}

	n, err := WriteKeys(dir, "deploy", keys)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("wrote %d keys, want 1", n)
	}
}

func TestWriteKeysRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	keys := []string{"not-a-valid-key"}

	_, err := WriteKeys(dir, "deploy", keys)
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestWriteKeysSkipsBlanks(t *testing.T) {
	dir := t.TempDir()
	keys := []string{"", "  ", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz test", ""}

	n, err := WriteKeys(dir, "deploy", keys)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("wrote %d keys, want 1 (blanks skipped)", n)
	}
}

func TestWriteKeysFileReadableByOthers(t *testing.T) {
	dir := t.TempDir()
	keys := []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz test-key"}

	if _, err := WriteKeys(dir, "deploy", keys); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dir, "deploy"))
	if err != nil {
		t.Fatal(err)
	}

	// Key files must be world-readable (0644) so AuthorizedKeysCommandUser nobody can read them
	perm := info.Mode().Perm()
	if perm&0044 == 0 {
		t.Errorf("key file should be readable by others (need 0644 for nobody), got %04o", perm)
	}
}

func TestFetchGitHubKeys(t *testing.T) {
	body := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz gh-key-1\nssh-rsa AAAAB3NzaC1yc2EAAAADAQAB gh-key-2\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body)) //nolint:errcheck
	}))
	defer srv.Close()

	// Override the URL by using a client that redirects to our test server
	keys, err := fetchKeysFromURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
}

func TestFetchGitHubKeysNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchKeysFromURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

func TestFetchGitHubKeysMixedContent(t *testing.T) {
	body := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGrz valid-key\n\n# comment\nnot-a-key\nssh-rsa AAAAB3NzaC1yc2EAAAADAQAB another-valid\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body)) //nolint:errcheck
	}))
	defer srv.Close()

	keys, err := fetchKeysFromURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2 (blanks/comments/invalid filtered)", len(keys))
	}
}
