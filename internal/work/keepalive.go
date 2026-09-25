package work

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readableHostLetters is how much of a host stands readably in the name of its
// lock. What tells two hosts apart is the digest behind it, so this is for
// whoever looks in the directory and no more.
const readableHostLetters = 40

// Keepalive is the lock the one process that reports on a timer holds for as
// long as it reports. There is one per host, because the store keys its lists by
// host: a single lock for the whole machine would let the cards of a second host
// go dark without a word, which is worse than a second process.
//
// Unlike the lock on the store, which is held for the moment of a read and a
// write, this one stands for a process that is running. The kernel drops it when
// that process ends, however it ends, so one that was killed leaves nothing
// behind for a later one to clear away.
type Keepalive struct {
	held *lock
}

// HoldKeepalive takes the lock of one host, and answers ErrBusy where a process
// of that host already holds it.
func HoldKeepalive(storePath, host string) (*Keepalive, error) {
	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	held, err := acquire(KeepaliveLockPath(storePath, host), 0)
	if err != nil {
		return nil, err
	}
	return &Keepalive{held: held}, nil
}

// Release gives the lock up. It is safe on a lock already released.
func (k *Keepalive) Release() {
	k.held.release()
}

// SomebodyKeepsAlive says whether a process of this host is reporting on a
// timer. It asks by taking the lock and giving it back at once, so the answer is
// about the moment it was asked; a second one that starts on a stale answer
// gives up by itself, because what decides is the lock and not this.
func SomebodyKeepsAlive(storePath, host string) (bool, error) {
	held, err := HoldKeepalive(storePath, host)
	if errors.Is(err, ErrBusy) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	held.Release()
	return false, nil
}

// KeepaliveLockPath is where the lock of one host lies: beside the store, so
// that it belongs to the machine like the list it stands for.
func KeepaliveLockPath(storePath, host string) string {
	return filepath.Join(filepath.Dir(storePath), "keepalive-"+nameOfTheHost(host)+".lock")
}

// nameOfTheHost turns a host into the middle of a file name. What reads as a
// host name is kept and everything else becomes a hyphen, so that whoever looks
// into the directory sees whose lock this is; a short digest of the whole host
// stands behind it, so that two hosts which read alike once shortened — one of
// them on another port, say — do not come to share a lock and let a machine's
// cards go dark without a word.
//
// Nothing of the host reaches the file system as a path of its own this way: no
// separator, no name made of dots, nothing longer than a directory takes.
func nameOfTheHost(host string) string {
	digest := sha256.Sum256([]byte(host))
	readable := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if len(readable) > readableHostLetters {
		readable = readable[:readableHostLetters]
	}
	letters := make([]byte, 0, len(readable))
	for _, b := range []byte(readable) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '.', b == '-':
			letters = append(letters, b)
		default:
			letters = append(letters, '-')
		}
	}
	return string(letters) + "-" + hex.EncodeToString(digest[:4])
}
