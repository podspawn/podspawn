package authkeys

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const keyOptions = "restrict,pty,agent-forwarding,port-forwarding,X11-forwarding"

// Lookup reads SSH public keys for username from keyDir and writes
// authorized_keys lines to w. Each line gets a command= directive
// that forces podspawn spawn. Returns the number of keys written.
//
// If the key file doesn't exist, writes nothing and returns 0 (not
// an error). sshd interprets empty output as "no keys, fall through."
func Lookup(username, keyDir, binaryPath string, w io.Writer) (int, error) {
	if strings.Contains(username, "/") || strings.Contains(username, "..") {
		return 0, fmt.Errorf("invalid username %q", username)
	}

	keyFile := filepath.Join(keyDir, username)
	f, err := os.Open(keyFile)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading keys for %s: %w", username, err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	n := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		directive := fmt.Sprintf("command=\"%s spawn --user %s\",%s",
			binaryPath, username, keyOptions)
		if _, err := fmt.Fprintf(w, "%s %s\n", directive, line); err != nil {
			return n, fmt.Errorf("writing key for %s: %w", username, err)
		}
		n++
	}
	if err := scanner.Err(); err != nil {
		return n, fmt.Errorf("reading keys for %s: %w", username, err)
	}
	return n, nil
}
