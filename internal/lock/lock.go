//go:build !windows

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const pollInterval = 50 * time.Millisecond

// Acquire takes an exclusive flock on lockDir/<username>.lock.
// The returned function releases the lock and closes the file descriptor.
// Caller must defer the unlock function.
//
// Uses non-blocking flock with polling so the caller's context is respected.
func Acquire(ctx context.Context, lockDir, username string) (unlock func(), err error) {
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, username+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", lockPath, err)
	}

	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK {
			_ = f.Close()
			return nil, fmt.Errorf("acquiring lock for %s: %w", username, err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func Remove(lockDir, username string) error {
	lockPath := filepath.Join(lockDir, username+".lock")
	err := os.Remove(lockPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
