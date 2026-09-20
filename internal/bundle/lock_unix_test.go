//go:build unix

package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

// A run that opens the lock file just before the run that holds it removes it
// can lock what it opened, but that file is at no path any more.
func TestAFileThatWasRemovedMeanwhileIsNoLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bundle.lock")
	first, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	early, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	first.release()

	held, err := lockOpened(early, path)
	if held != nil || err != nil {
		t.Fatalf("got a lock on a file that is gone (%v, %v)", held, err)
	}

	// Opening the path again gives the lock the next run would meet.
	second, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.release()
	if third, err := acquireLock(path); err == nil {
		third.release()
		t.Error("two runs hold the lock")
	}
}

// The case the comparison is there for: by the time the early file is locked,
// the next run has made a new file at the path and holds it.
func TestAFileThatWasReplacedMeanwhileIsNoLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bundle.lock")
	first, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	early, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	first.release()
	next, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer next.release()

	held, err := lockOpened(early, path)
	if held != nil || err != nil {
		t.Fatalf("got a lock beside the one the next run holds (%v, %v)", held, err)
	}
	if another, err := acquireLock(path); err == nil {
		another.release()
		t.Error("the lock of the next run does not hold any more")
	}
}
