package sshconfig

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const podBlock = `Host *.pod
    ProxyCommand podspawn connect %r %h %p
    UserKnownHostsFile /dev/null
    StrictHostKeyChecking no
`

// EnsurePodBlock appends the *.pod SSH config block to configPath.
// Creates the file and parent directory if they don't exist.
// Returns true if the block was added, false if it was already present.
func EnsurePodBlock(configPath string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return false, err
	}

	existing, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	if hasPodHost(existing) {
		return false, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return true, os.WriteFile(configPath, []byte(podBlock), 0600)
	}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return false, err
	}
	defer f.Close() //nolint:errcheck

	separator := "\n"
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = "\n\n"
	}

	if _, err := f.WriteString(separator + podBlock); err != nil {
		return false, err
	}

	return true, nil
}

func hasPodHost(data []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "Host *.pod" {
			return true
		}
	}
	return false
}
