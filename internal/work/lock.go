package work

import (
	"errors"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// lock is only ever held for the moment a store is read and written, or for
// as long as a keepalive reports.
type lock = localfile.Lock

func acquire(path string, wait time.Duration) (*lock, error) {
	held, err := localfile.AcquireWithin(path, wait)
	if errors.Is(err, localfile.ErrBusy) {
		return nil, ErrBusy
	}
	return held, err
}
