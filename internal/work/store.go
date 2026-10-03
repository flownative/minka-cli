// Package work keeps the cards the sessions on this machine are working on.
//
// There is one store per machine, next to the configuration, and in it one list
// per host and session. Every session reports its own list and nothing else,
// under a client of its own: the server keeps one set per client, so a session
// that reported the cards of another would keep them alight after that one
// died. Every change to the store is made while holding a lock of the kernel, so
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
	"regexp"
	"slices"
	"time"

	"github.com/flownative/minka-cli/internal/localfile"
)

// ForgottenAfter is how long a card stays in the store without a sign of life.
// A session that dies says nothing more about its cards, and nobody else knows
// they are over; a working day is long enough for the work itself and short
// enough that no tile glows through the night.
const ForgottenAfter = 8 * time.Hour

// SeenEvery is how often a session writes down that it is still at its cards.
// The moment is read against a window of hours, so writing it oftener would
// cost the store something and say nothing new.
const SeenEvery = time.Minute

// BeatEvery is how often the moment of a beat is written down. It is read
// against a window of many minutes, so writing it oftener would cost the store
// something and say nothing new.
const BeatEvery = time.Minute

// HeldFor is how long the server counts a signal. A list that has sent nothing
// for that long has nothing left on the server to take back.
const HeldFor = 3 * time.Minute

// NoSession is the key of the list of whoever does not say which session they
// are: a person at a terminal, or an agent that knows nothing of sessions.
const NoSession = ""

// sessionPattern is what a session may be called. A session of Claude Code
// goes by a UUID; the letters are those the server takes for a client, so that
// a name never has to be mangled on its way anywhere.
var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// IsSession says whether a name can stand for a session.
func IsSession(name string) bool {
	return sessionPattern.MatchString(name)
}

// ErrBusy means another process holds the lock on the store.
var ErrBusy = errors.New("another minka is writing the store")

// Card is a card a session is at work on. The title and the short identifier
// are kept as they were read, so that the list can be shown without asking the
// server again.
type Card struct {
	ID        string    `json:"id"`
	ShortID   string    `json:"shortId,omitempty"`
	Title     string    `json:"title,omitempty"`
	StartedAt time.Time `json:"startedAt"`

	// SeenAt is the last moment the session of the list said it was still at
	// the card. A card in the list of no session is never seen again and goes
	// by the moment it was started.
	SeenAt time.Time `json:"seenAt,omitempty"`
}

// LetGoCard is a card the host did not take into the list at work: the word it
// gave for that, and when it said so. The note outlives the card, because a
// card that is gone from the list is the one thing the list itself cannot say.
//
// Where the word is one this machine acts on, the card is out of the list as
// well. Where it is not, the card stays in it: no word of a host takes a card
// out of a list by itself.
type LetGoCard struct {
	ID      string    `json:"id"`
	ShortID string    `json:"shortId,omitempty"`
	Title   string    `json:"title,omitempty"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// Host is what is remembered for one Minka host: one list per session, keyed by
// the name of the session, and NoSession for whoever gave none.
type Host struct {
	Sessions map[string]*List `json:"sessions"`
}

// List is what one session has in hand on one host, and what it last reported.
//
// SentAt and SentCards are the throttle: they say what the last signal carried
// and when it went out, whether it arrived or not. Every signal is a new
// process, so the throttle only works because it is written down here.
//
// Going out and arriving are two different moments. A signal that was turned
// away was at the host all the same, and whoever reads the list has to be able
// to tell "nothing has ever reported from here" from "it reports, and the host
// says no".
//
// BeatAt is the last moment somebody who is at work was at the store: a beat
// out of a session, or a command somebody typed. It hangs on the invocation and
// not on a request going out, because a signal is held back for a minute at a
// time and a busy session would otherwise look silent.
type List struct {
	Cards []Card `json:"cards"`

	// LetGo are the cards the host did not take, newest last, kept as long as a
	// card of the list is kept without a sign of life.
	LetGo []LetGoCard `json:"letGo,omitempty"`

	SentAt     time.Time `json:"sentAt,omitempty"`
	SentCards  []string  `json:"sentCards,omitempty"`
	ReportedAt time.Time `json:"reportedAt,omitempty"`
	Failure    string    `json:"failure,omitempty"`
	FailedAt   time.Time `json:"failedAt,omitempty"`
	BeatAt     time.Time `json:"beatAt,omitempty"`

	// Mod says that the session reports from inside its own Claude Code
	// process, every thirty seconds for as long as that process runs. Nothing
	// else reports such a list: when the process ends, its beats end, and the
	// server forgets its cards three minutes after the last signal.
	Mod bool `json:"mod,omitempty"`
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
		held.Release()
		return nil, fmt.Errorf("reading %s: %w", path, err)
	default:
		if err := json.Unmarshal(content, file); err != nil {
			held.Release()
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if file.Hosts == nil {
			file.Hosts = map[string]*Host{}
		}
	}

	return &Handle{File: file, path: path, lock: held}, nil
}

func (h *Handle) Close() {
	h.lock.Release()
}

// Save writes the store where it lies. It writes a temporary file next to the
// target and renames it, so that an interrupted write cannot leave a truncated
// store behind.
func (h *Handle) Save() error {
	content, err := json.MarshalIndent(h.File, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the store: %w", err)
	}
	return localfile.Write(h.path, ".work-*.json", append(content, '\n'))
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

// List is the list of one session, an empty one where it holds nothing yet.
func (h *Host) List(session string) *List {
	if h.Sessions == nil {
		h.Sessions = map[string]*List{}
	}
	list, ok := h.Sessions[session]
	if !ok {
		list = &List{}
		h.Sessions[session] = list
	}
	return list
}

// StillAtIt says again that a session is at work on the cards it took up, and
// answers whether that was worth writing down. A session that gave no name
// says nothing here: its cards go by the moment they were started.
func (h *Host) StillAtIt(session string, now time.Time) bool {
	list, known := h.Sessions[session]
	if session == NoSession || !known {
		return false
	}
	written := false
	for i := range list.Cards {
		if now.Sub(list.Cards[i].SeenAt) < SeenEvery {
			continue
		}
		list.Cards[i].SeenAt = now
		written = true
	}
	return written
}

// ForgetWhatRanOut sweeps every list of the host, and drops a list that has
// nothing left to hold, to say or to take back: no card, no note, and nothing
// sent that the server could still be counting. It answers how many cards it
// forgot in the list of the session named, which is the one a command speaks
// for, and whether it changed anything at all.
func (h *Host) ForgetWhatRanOut(session string, now time.Time) (int, bool) {
	forgottenHere, changed := 0, false
	for name, list := range h.Sessions {
		forgotten, swept := list.ForgetWhatRanOut(now)
		if name == session {
			forgottenHere = forgotten
		}
		changed = changed || swept
		if len(list.Cards) == 0 && len(list.LetGo) == 0 && (len(list.SentCards) == 0 || now.Sub(list.SentAt) >= HeldFor) {
			delete(h.Sessions, name)
			changed = true
		}
	}
	return forgottenHere, changed
}

// UnmarshalJSON reads a host in either of two shapes: one list per session, and
// one list for the whole machine whose cards each name the session that took
// them up. The second is handed out to those sessions, so that no card is lost
// and every one keeps its sign of life. What the host said about that list and
// what it was last sent stay with the list of no session, which reports under
// the client the whole list went by, so that the next signal of that client
// takes back what is not its own.
//
// A list is given the moment its own cards were last seen as the mark of its
// beat, not the mark of the machine: a session that died long ago would
// otherwise pass for one at work as long as any other was.
func (h *Host) UnmarshalJSON(content []byte) error {
	var stored struct {
		Sessions map[string]*List `json:"sessions"`
		List
		Cards []struct {
			Card
			Session string `json:"session"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(content, &stored); err != nil {
		return err
	}
	h.Sessions = stored.Sessions
	if len(stored.Cards) == 0 && len(stored.LetGo) == 0 && len(stored.SentCards) == 0 {
		return nil
	}

	machine := h.List(NoSession)
	machine.LetGo = append(machine.LetGo, stored.LetGo...)
	machine.SentAt, machine.SentCards = stored.SentAt, stored.SentCards
	machine.ReportedAt, machine.Failure, machine.FailedAt = stored.ReportedAt, stored.Failure, stored.FailedAt
	for _, card := range stored.Cards {
		session := card.Session
		if !IsSession(session) {
			session = NoSession
		}
		list := h.List(session)
		list.Start(card.Card)
		if seen := lastSeen(card.Card); seen.After(list.BeatAt) {
			list.BeatAt = seen
		}
	}
	return nil
}

// Start remembers a card, or says again that the session is still on one it
// remembers. It answers whether the card was new.
func (h *List) Start(card Card) bool {
	at := slices.IndexFunc(h.Cards, func(c Card) bool { return c.ID == card.ID })
	if at < 0 {
		h.Cards = append(h.Cards, card)
		return true
	}
	h.Cards[at] = card
	return false
}

// Stop forgets a card and answers whether it was there.
func (h *List) Stop(cardID string) bool {
	before := len(h.Cards)
	h.Cards = slices.DeleteFunc(h.Cards, func(c Card) bool { return c.ID == cardID })
	return len(h.Cards) < before
}

// LetGoOf takes a card out of the list because the host will not have it, and
// leaves the note that says why.
func (h *List) LetGoOf(cardID, reason string, now time.Time) LetGoCard {
	note := h.note(cardID, reason, now)
	h.Stop(cardID)
	return note
}

// NoteNotTaken keeps the note about a card the host did not take and leaves the
// list as it is, for a word this machine does not act on.
func (h *List) NoteNotTaken(cardID, reason string, now time.Time) LetGoCard {
	return h.note(cardID, reason, now)
}

// note writes down what the host said about one card, under the name the card
// goes by here, and replaces what it said about the same card before: a
// condition that lasts is one note, not one per signal.
func (h *List) note(cardID, reason string, now time.Time) LetGoCard {
	note := LetGoCard{ID: cardID, Reason: reason, At: now}
	if at := slices.IndexFunc(h.Cards, func(c Card) bool { return c.ID == cardID }); at >= 0 {
		note.ShortID = h.Cards[at].ShortID
		note.Title = h.Cards[at].Title
	}
	h.LetGo = slices.DeleteFunc(h.LetGo, func(n LetGoCard) bool { return n.ID == cardID })
	h.LetGo = append(h.LetGo, note)
	return note
}

// Beat writes down that somebody who is at work was here, and answers whether
// that was worth writing down. Whoever is at no work says nothing here: the
// mark is what tells a machine where somebody is working from one where nothing
// but minka itself is left running.
func (h *List) Beat(now time.Time) bool {
	if now.Sub(h.BeatAt) < BeatEvery {
		return false
	}
	h.BeatAt = now
	return true
}

// ForgetWhatRanOut drops the cards of a session that never said it was done,
// and the notes about cards the host did not take that have stood as long: one
// window for everything the store keeps without being told again. It answers
// how many cards it forgot and whether it changed anything at all.
func (h *List) ForgetWhatRanOut(now time.Time) (int, bool) {
	before := len(h.Cards)
	h.Cards = slices.DeleteFunc(h.Cards, func(c Card) bool { return now.Sub(lastSeen(c)) >= ForgottenAfter })
	notes := len(h.LetGo)
	h.LetGo = slices.DeleteFunc(h.LetGo, func(n LetGoCard) bool { return now.Sub(n.At) >= ForgottenAfter })
	forgotten := before - len(h.Cards)
	return forgotten, forgotten > 0 || notes > len(h.LetGo)
}

// lastSeen is when the session that took a card up was last known to be at it:
// the moment it said so, or the moment the card was taken up where it has said
// nothing since and where it never gave a name to say it under. A card that is
// taken up anew is taken up whole, so it has said nothing yet.
func lastSeen(card Card) time.Time {
	if card.SeenAt.IsZero() {
		return card.StartedAt
	}
	return card.SeenAt
}

// CardIDs is what a signal carries: every card of the list, sorted, so that two
// sets can be compared as they are.
func (h *List) CardIDs() []string {
	ids := make([]string, 0, len(h.Cards))
	for _, card := range h.Cards {
		ids = append(ids, card.ID)
	}
	slices.Sort(ids)
	return ids
}
