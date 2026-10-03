// Package readmark remembers which comments of a card this machine has shown,
// so that a command can tell the comments somebody has read from the ones that
// came in since.
//
// The marks lie in one file beside the configuration, per host and per reader,
// a reader being the token a command signs in with. A card's mark is the set of
// the comments known, not the newest one: a comment that was written while
// another one was being sent, or one that came in with an earlier time, is
// still unknown, and a removed comment leaves a name behind that harms nothing.
// Marks are only ever added to, so two processes writing at once lose nothing
// as long as each adds under the lock.
package readmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// ForgottenAfter is how long the mark of a card is kept after it was last
// written. Without it the file grows with every card ever read and is written
// whole with every command that reads one. A card that is forgotten has every
// comment unread again, which says too much rather than too little.
const ForgottenAfter = 90 * 24 * time.Hour

// RenewedAfter is how old the mark of a card has to be before a read that adds
// nothing to it writes it again. Every read of a card notes its comments, and
// rewriting the whole file for each would cost every read a write; the moment
// is read against a window of months, so a day changes nothing.
const RenewedAfter = 24 * time.Hour

// File is the marks as they lie on disk.
type File struct {
	Hosts map[string]map[string]map[string]*Card `json:"hosts"` // host, reader, card ID
}

// Card is what is known of the comments of one card.
type Card struct {
	Known     []string  `json:"known"` // the IDs of the comments shown or written here
	WrittenAt time.Time `json:"writtenAt"`
}

// Path is where the marks lie: beside the configuration file, so that they
// belong to the machine and not to the directory a command was started in.
func Path(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "comments-read.json")
}

// Reader names the person a token signs in as without the token itself. Two
// tokens of the same person are two readers, which costs at most comments
// shown again; one name for every token of a host would let one person's
// reading count for another.
func Reader(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:16])
}

// Known reads the comments known of a card. It takes no lock: the file is
// only ever replaced whole. A card without a mark knows nothing.
func Known(path, host, reader, cardID string) (map[string]bool, error) {
	file, err := read(path)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	if card := file.Hosts[host][reader][cardID]; card != nil {
		for _, id := range card.Known {
			known[id] = true
		}
	}
	return known, nil
}

// Remember adds comments to what is known of a card. It waits for the lock as
// long as the context allows. A holder keeps it for one read and one write of
// this file and never across a request, so a short wait is enough; a holder
// that is stuck would otherwise hold up whoever only wanted to read a card.
func Remember(ctx context.Context, path, host, reader, cardID string, ids []string, now time.Time) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	held, err := localfile.Acquire(ctx, filepath.Join(dir, "comments-read.lock"))
	if err != nil {
		return err
	}
	defer held.Release()

	file, err := read(path)
	if err != nil {
		return err
	}
	if file.knowsAlready(host, reader, cardID, ids, now) {
		return nil
	}
	file.forgetWhatIsOlderThan(now.Add(-ForgottenAfter))

	readers := file.Hosts[host]
	if readers == nil {
		readers = map[string]map[string]*Card{}
		file.Hosts[host] = readers
	}
	cards := readers[reader]
	if cards == nil {
		cards = map[string]*Card{}
		readers[reader] = cards
	}
	card := cards[cardID]
	if card == nil {
		card = &Card{}
		cards[cardID] = card
	}
	for _, id := range ids {
		if !slices.Contains(card.Known, id) {
			card.Known = append(card.Known, id)
		}
	}
	card.WrittenAt = now.UTC()

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the marks: %w", err)
	}
	return localfile.Write(path, ".comments-read-*.json", append(content, '\n'))
}

func read(path string) (*File, error) {
	file := &File{Hosts: map[string]map[string]map[string]*Card{}}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(content, file); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if file.Hosts == nil {
		file.Hosts = map[string]map[string]map[string]*Card{}
	}
	return file, nil
}

// knowsAlready says whether the mark of a card holds every one of the comments
// and was written less than RenewedAfter ago.
func (f *File) knowsAlready(host, reader, cardID string, ids []string, now time.Time) bool {
	card := f.Hosts[host][reader][cardID]
	if card == nil || now.Sub(card.WrittenAt) >= RenewedAfter {
		return false
	}
	for _, id := range ids {
		if !slices.Contains(card.Known, id) {
			return false
		}
	}
	return true
}

func (f *File) forgetWhatIsOlderThan(limit time.Time) {
	for host, readers := range f.Hosts {
		for reader, cards := range readers {
			for id, card := range cards {
				if card == nil || card.WrittenAt.Before(limit) {
					delete(cards, id)
				}
			}
			if len(cards) == 0 {
				delete(readers, reader)
			}
		}
		if len(readers) == 0 {
			delete(f.Hosts, host)
		}
	}
}
