// Package localfile keeps the files minka writes beside its configuration:
// written whole or not at all, and changed by one process at a time.
package localfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrBusy means another process held the lock for as long as the caller was
// prepared to wait.
var ErrBusy = errors.New("another minka is writing the file")

// AcquireWithin takes the lock, waiting at most that long; with no wait at all
// it gives up as soon as somebody else holds it.
func AcquireWithin(path string, wait time.Duration) (*Lock, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	return Acquire(ctx, path)
}

// Write replaces the file at path with content. It writes a temporary file
// named after pattern next to the target and renames it, so that an
// interrupted write cannot leave a truncated file behind. A file that is a
// symlink is written where the link points and stays a link.
func Write(path, pattern string, content []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}

	// CreateTemp creates the file with 0600, before anything is written to it.
	temp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	// The rename itself has to reach the disk, or a machine that loses power
	// can come back with the file it held before and the new one nowhere.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
