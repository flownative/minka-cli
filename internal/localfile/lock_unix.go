//go:build unix

package localfile

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// pause is how long a caller that waits for a lock sleeps between tries.
const pause = 10 * time.Millisecond

// Lock keeps two processes from writing the same file at once. It is an
// advisory lock of the kernel on a file beside it, which the kernel drops when
// the process ends, however it ends.
//
// The file of the lock stays where it is: a file that is never taken away
// cannot be replaced under a process that has just opened it.
type Lock struct {
	file *os.File
}

// Acquire takes the lock, trying until the context is done. A context that is
// done before the lock is free answers ErrBusy where its deadline ran out, and
// its own error where it was cancelled.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for tries := 1; ; tries++ {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &Lock{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return nil, err
		}
		if BusyTried != nil {
			BusyTried(tries)
		}
		select {
		case <-ctx.Done():
			// One last try, so that a lock given up while the caller slept
			// is still taken in time.
			if syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
				return &Lock{file: file}, nil
			}
			_ = file.Close()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, ErrBusy
			}
			return nil, ctx.Err()
		case <-time.After(pause):
		}
	}
}

// Release gives the lock up. It is safe on a lock that was never taken and on
// one already released.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
}
