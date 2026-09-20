//go:build unix

package bundle

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lock keeps a second run away from a target. It is an advisory lock of the
// kernel on a file next to the target: the kernel drops it when the process
// ends, however it ends, so a run that was killed locks nobody out.
type lock struct {
	file *os.File
}

func acquireLock(path string) (*lock, error) {
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		held, err := lockOpened(file, path)
		if err != nil || held != nil {
			return held, err
		}
	}
}

// lockOpened locks a file that was opened at path. It returns neither a lock
// nor an error when the file is no longer the one at the path: the run that
// held the lock removes the file when it is done, and a file opened just
// before that can be locked, but the next run would lock another one at the
// same path, and both would go on. The caller then opens the path again.
func lockOpened(file *os.File, path string) (*lock, error) {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w; it holds the lock %s", ErrLocked, path)
		}
		return nil, err
	}

	opened, err := file.Stat()
	if err == nil {
		var atPath os.FileInfo
		if atPath, err = os.Stat(path); err == nil && os.SameFile(opened, atPath) {
			return &lock{file: file}, nil
		}
	}
	_ = file.Close()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return nil, nil
}

// release gives the lock up and takes its file away. It is safe on a lock
// that was never taken and on one already released.
func (l *lock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = os.Remove(l.file.Name())
	_ = l.file.Close()
	l.file = nil
}
