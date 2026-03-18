//go:build !windows

package lock

import (
	"context"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Acquire(context.Background(), dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestConcurrentLock(t *testing.T) {
	dir := t.TempDir()

	// Goroutine 1 acquires lock
	unlock1, err := Acquire(context.Background(), dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})

	// Goroutine 2 should block until lock is released
	go func() {
		unlock2, err := Acquire(context.Background(), dir, "deploy")
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

func TestRemove(t *testing.T) {
	dir := t.TempDir()

	unlock, err := Acquire(context.Background(), dir, "stale-user")
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	if err := Remove(dir, "stale-user"); err != nil {
		t.Fatalf("Remove() returned error: %v", err)
	}

	_, err = Acquire(context.Background(), dir, "stale-user")
	if err != nil {
		t.Fatalf("Acquire after Remove should work, got: %v", err)
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()

	if err := Remove(dir, "ghost"); err != nil {
		t.Fatalf("Remove() on nonexistent lock should be a no-op, got: %v", err)
	}
}

func TestDifferentUsersNotBlocked(t *testing.T) {
	dir := t.TempDir()

	unlock1, err := Acquire(context.Background(), dir, "alice")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock1()

	// Different user should not be blocked
	unlock2, err := Acquire(context.Background(), dir, "bob")
	if err != nil {
		t.Fatal(err)
	}
	unlock2()
}

func TestAcquireRespectsContextTimeout(t *testing.T) {
	dir := t.TempDir()

	unlock1, err := Acquire(context.Background(), dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock1()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = Acquire(ctx, dir, "deploy")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from context timeout, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("returned too quickly (%v), expected ~150ms", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("took too long (%v), context should have cancelled around 150ms", elapsed)
	}
}

func TestAcquireRespectsContextCancellation(t *testing.T) {
	dir := t.TempDir()

	unlock1, err := Acquire(context.Background(), dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock1()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err = Acquire(ctx, dir, "deploy")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
