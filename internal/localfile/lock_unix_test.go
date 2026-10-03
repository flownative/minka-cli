//go:build unix

package localfile

import (
	"context"
	"path/filepath"
	"testing"
)

// A lock given up in the moment the caller stops waiting is still taken: the
// last try comes after the wait has run out. The holder lets go and the wait
// ends in the same moment, after the caller found the lock taken once.
func TestALockFreedAsTheWaitEndsIsStillTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.lock")
	held, err := Acquire(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	BusyTried = func() {
		held.Release()
		cancel()
	}
	t.Cleanup(func() { BusyTried = nil })

	taken, err := Acquire(ctx, path)
	if err != nil {
		t.Fatalf("the lock freed as the wait ended was not taken: %v", err)
	}
	taken.Release()
}
