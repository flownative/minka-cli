// Package work keeps the cards this machine is working on.
//
// There is one store per machine, next to the configuration, and every session
// on the machine writes into it: what the server keeps hangs on the agent and
// its client, so two sessions with lists of their own would take each other's
// cards away with every signal. A signal therefore reports the union of the
// store, and every change to it is made while holding a lock of the kernel, so
// that a card somebody starts is never lost to a write somebody else began at
// the same moment.
package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// ForgottenAfter is how long a card stays in the store without being started
// again. A session that dies says nothing more about its cards, and nobody
// else knows they are over; a working day is long enough for the work itself
// and short enough that no tile glows through the night.
const ForgottenAfter = 8 * time.Hour

// ErrBusy means another process holds the lock on the store.
var ErrBusy = errors.New("another minka is writing the store")

// Card is a card this machine is at work on. The title and the short
// identifier are kept as they were read, so that the list can be shown without
// asking the server again.
type Card struct {
	ID        string    `json:"id"`
	ShortID   string    `json:"shortId,omitempty"`
	Title     string    `json:"title,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

// Host is what is remembered for one Minka host.
//
// SentAt and SentCards are the throttle: they say what the last signal carried
// and when it went out, whether it arrived or not. Every signal is a new
// process, so the throttle only works because it is written down here.
//
// Going out and arriving are two different moments. A signal that was turned
// away was at the host all the same, and whoever reads the list has to be able
// to tell "nothing has ever reported from here" from "it reports, and the host
// says no".
type Host struct {
	Cards []Card `json:"cards"`

	SentAt     time.Time `json:"sentAt,omitempty"`
	SentCards  []string  `json:"sentCards,omitempty"`
	ReportedAt time.Time `json:"reportedAt,omitempty"`
	Failure    string    `json:"failure,omitempty"`
	FailedAt   time.Time `json:"failedAt,omitempty"`
}

// File is the store as it lies on disk.
type File struct {
	Hosts map[string]*Host `json:"hosts"`
}

// Handle is an open store together with the lock that is held on it. The
// caller closes it, and saves before that when it changed anything.
type Handle struct {
	File *File

	path string
	lock *lock
}

// Path is where the store lies: beside the configuration file, so that it
// belongs to the machine and not to the directory a command was started in.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "work.json")
}

// Untouched says that nothing was ever remembered on this machine. It is what
// a signal asks before anything else, so that the common case costs one look
// at the directory and no lock.
func Untouched(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// Open takes the lock on the store and reads it. It waits at most that long
// for the lock; with no wait at all it gives up as soon as somebody else holds
// it, which is what a signal does rather than keep a session waiting.
func Open(path string, wait time.Duration) (*Handle, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	held, err := acquire(filepath.Join(dir, "work.lock"), wait)
	if err != nil {
		return nil, err
	}

	file := &File{Hosts: map[string]*Host{}}
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		held.release()
		return nil, fmt.Errorf("reading %s: %w", path, err)
	default:
		if err := json.Unmarshal(content, file); err != nil {
			held.release()
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if file.Hosts == nil {
			file.Hosts = map[string]*Host{}
		}
	}

	return &Handle{File: file, path: path, lock: held}, nil
}

func (h *Handle) Close() {
	h.lock.release()
}

// Save writes the store where it lies. It writes a temporary file next to the
// target and renames it, so that an interrupted write cannot leave a truncated
// store behind.
func (h *Handle) Save() error {
	path := h.path
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}

	content, err := json.MarshalIndent(h.File, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the store: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".work-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(content, '\n')); err != nil {
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

// Host is what this host holds, an empty one where it holds nothing yet.
func (f *File) Host(host string) *Host {
	entry, ok := f.Hosts[host]
	if !ok {
		entry = &Host{}
		f.Hosts[host] = entry
	}
	return entry
}

// Start remembers a card, or says again that this machine is still on one it
// remembers. It answers whether the card was new.
func (h *Host) Start(card Card) bool {
	at := slices.IndexFunc(h.Cards, func(c Card) bool { return c.ID == card.ID })
	if at < 0 {
		h.Cards = append(h.Cards, card)
		return true
	}
	h.Cards[at] = card
	return false
}

// Stop forgets a card and answers whether it was there.
func (h *Host) Stop(cardID string) bool {
	before := len(h.Cards)
	h.Cards = slices.DeleteFunc(h.Cards, func(c Card) bool { return c.ID == cardID })
	return len(h.Cards) < before
}

// ForgetWhatRanOut drops the cards of a session that never said it was done.
func (h *Host) ForgetWhatRanOut(now time.Time) int {
	before := len(h.Cards)
	h.Cards = slices.DeleteFunc(h.Cards, func(c Card) bool { return now.Sub(c.StartedAt) >= ForgottenAfter })
	return before - len(h.Cards)
}

// CardIDs is what a signal carries: every card of the store, sorted, so that
// two sets can be compared as they are.
func (h *Host) CardIDs() []string {
	ids := make([]string, 0, len(h.Cards))
	for _, card := range h.Cards {
		ids = append(ids, card.ID)
	}
	slices.Sort(ids)
	return ids
}
