package export

import (
	"os"
	"path/filepath"
	"testing"
)

// The lock of a run is the directory's, whatever name leads there: renamed,
// it keeps its lock, and on a file system that takes case as one, another case
// meets the same lock. Another directory has a lock of its own.
func TestALockBelongsToTheDirectoryNotToItsName(t *testing.T) {
	locks, base := t.TempDir(), t.TempDir()
	dir := filepath.Join(base, "Plans")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := LockPath(locks, dir)
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(base, "renamed")
	if err := os.Rename(dir, renamed); err != nil {
		t.Fatal(err)
	}
	if again, err := LockPath(locks, renamed); err != nil || again != first {
		t.Fatalf("the renamed directory has the lock %q, not %q: %v", again, first, err)
	}
	if _, err := os.Stat(filepath.Join(base, "RENAMED")); err == nil {
		if other, err := LockPath(locks, filepath.Join(base, "RENAMED")); err != nil || other != first {
			t.Fatalf("another case of the name has the lock %q: %v", other, err)
		}
	}
	other := filepath.Join(base, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if lock, err := LockPath(locks, other); err != nil || lock == first {
		t.Fatalf("another directory shares the lock: %v", err)
	}
}
