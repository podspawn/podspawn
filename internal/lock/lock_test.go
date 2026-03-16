//go:build !windows

package lock

import (
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Acquire(dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestConcurrentLock(t *testing.T) {
	dir := t.TempDir()

	// Goroutine 1 acquires lock
	unlock1, err := Acquire(dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})

	// Goroutine 2 should block until lock is released
	go func() {
		unlock2, err := Acquire(dir, "deploy")
		if err != nil {
			t.Error(err)
			return
		}
		close(acquired)
		unlock2()
	}()

	// Give goroutine 2 a moment to attempt acquisition
	select {
	case <-acquired:
		t.Fatal("second lock acquired while first is held")
	case <-time.After(50 * time.Millisecond):
		// expected: goroutine 2 is blocked
	}

	// Release first lock
	unlock1()

	// Goroutine 2 should now acquire
	select {
	case <-acquired:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("second lock not acquired after first released")
	}
}

func TestDifferentUsersNotBlocked(t *testing.T) {
	dir := t.TempDir()

	unlock1, err := Acquire(dir, "alice")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock1()

	// Different user should not be blocked
	unlock2, err := Acquire(dir, "bob")
	if err != nil {
		t.Fatal(err)
	}
	unlock2()
}
