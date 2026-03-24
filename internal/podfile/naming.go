package podfile

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

// SessionName derives a session name from the working directory path and an
// optional Podfile name override. Format: <prefix>-<hash> where hash is 4 hex
// chars from SHA-256 of the absolute path.
//
// The prefix is the Podfile name field if set, otherwise the directory basename.
// The hash ensures uniqueness across directories with the same basename.
func SessionName(absPath string, podfileName string) string {
	prefix := podfileName
	if prefix == "" {
		prefix = filepath.Base(absPath)
	}

	prefix = sanitizeName(prefix)
	if prefix == "" {
		prefix = "dev"
	}

	hash := pathHash(absPath)
	return prefix + "-" + hash
}

func pathHash(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("%x", h[:2])
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 30 {
		s = s[:30]
	}
	s = strings.Trim(s, "-")
	return s
}
