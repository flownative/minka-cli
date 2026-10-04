package work

import (
	"errors"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// lock is only ever held for the moment a store is read and written.
type lock = localfile.Lock

func acquire(path string, wait time.Duration) (*lock, error) {
	held, err := localfile.AcquireWithin(path, wait)
	if errors.Is(err, localfile.ErrBusy) {
		return nil, ErrBusy
	}
	return held, err
}
