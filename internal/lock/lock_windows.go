//go:build windows

package lock

import (
	"context"
	"fmt"
)

// Acquire is not supported on Windows (flock is Unix-only).
// Windows builds are client-only and don't use session locking.
func Acquire(_ context.Context, lockDir, username string) (unlock func(), err error) {
	return nil, fmt.Errorf("session locking not supported on Windows")
}

func Remove(lockDir, username string) error {
	return nil
}
