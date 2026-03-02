package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Acquire takes an exclusive flock on lockDir/<username>.lock.
// The returned function releases the lock and closes the file descriptor.
// Caller must defer the unlock function.
//
// Flock works across processes on the same host (local filesystem only).
func Acquire(lockDir, username string) (unlock func(), err error) {
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, username+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock for %s: %w", username, err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
