package adduser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validKeyPrefixes = []string{
	"ssh-ed25519",
	"ssh-rsa",
	"ecdsa-sha2-nistp256",
	"ecdsa-sha2-nistp384",
	"ecdsa-sha2-nistp521",
	"sk-ssh-ed25519",
	"sk-ecdsa-sha2-nistp256",
	"ssh-dss",
}

var usernameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func ValidateUsername(username string) error {
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("invalid username %q: must be 1-32 chars, start with lowercase letter, contain only [a-z0-9_-]", username)
	}
	return nil
}

func ValidateSSHKey(key string) error {
	fields := strings.Fields(key)
	if len(fields) < 2 {
		return fmt.Errorf("SSH key must have at least type and data fields")
	}
	for _, prefix := range validKeyPrefixes {
		if fields[0] == prefix {
			return nil
		}
	}
	return fmt.Errorf("unrecognized key type %q", fields[0])
}

// WriteKeys appends SSH public keys to the user's key file at
// keyDir/username. Creates the directory and file if needed.
// Duplicates are not checked; sshd handles them fine.
func WriteKeys(keyDir, username string, keys []string) (int, error) {
	if err := ValidateUsername(username); err != nil {
		return 0, err
	}

	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return 0, fmt.Errorf("creating key directory: %w", err)
	}

	keyFile := filepath.Join(keyDir, username)
	f, err := os.OpenFile(keyFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("opening key file: %w", err)
	}
	defer f.Close() //nolint:errcheck // write errors caught below

	n := 0
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := ValidateSSHKey(key); err != nil {
			return n, fmt.Errorf("invalid key: %w", err)
		}
		if _, err := fmt.Fprintln(f, key); err != nil {
			return n, fmt.Errorf("writing key: %w", err)
		}
		n++
	}
	return n, nil
}

// FetchGitHubKeys downloads SSH public keys from GitHub's public
// endpoint for the given user. Returns validated keys only.
func FetchGitHubKeys(ctx context.Context, client *http.Client, githubUser string) ([]string, error) {
	url := fmt.Sprintf("https://github.com/%s.keys", githubUser)
	return fetchKeysFromURL(ctx, client, url)
}

func fetchKeysFromURL(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching keys: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for %s", resp.StatusCode, url)
	}

	return parseKeys(resp.Body)
}

func parseKeys(r io.Reader) ([]string, error) {
	var keys []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := ValidateSSHKey(line); err != nil {
			continue
		}
		keys = append(keys, line)
	}
	return keys, scanner.Err()
}

// ReadKeyFile reads SSH public keys from a file (e.g., ~/.ssh/id_ed25519.pub).
// Returns one key per non-empty, non-comment line.
func ReadKeyFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	return parseKeys(f)
}
