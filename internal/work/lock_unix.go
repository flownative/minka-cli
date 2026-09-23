//go:build unix

package work

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// pause is how long a caller that waits for the lock sleeps between tries.
const pause = 10 * time.Millisecond

// lock keeps two processes from writing the store at once. It is an advisory
// lock of the kernel on a file beside the store, which the kernel drops when
// the process ends, however it ends.
//
// The file stays where it is: unlike a lock that stands for a run in progress,
// this one is only ever held for the moment a store is read and written, and a
// file that is never taken away cannot be replaced under a process that has
// just opened it.
type lock struct {
	file *os.File
}

func acquire(path string, wait time.Duration) (*lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &lock{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return nil, err
		}
		if !time.Now().Before(deadline) {
			_ = file.Close()
			return nil, ErrBusy
		}
		time.Sleep(pause)
	}
}

// release gives the lock up. It is safe on a lock that was never taken and on
// one already released.
func (l *lock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
}
