package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/flownative/minka-cli/internal/work"
)

const (
	// keepaliveTick is how often the process that reports on a timer wakes up.
	// It is shorter than the minute the throttle reports on, because a ticker of
	// exactly a minute falls a hair short of the throttle every second or third
	// time and the gap grows past the three minutes the server counts a signal
	// for: the tile would flicker.
	keepaliveTick = 30 * time.Second

	// aJumpInTheClock is how far two wake-ups may stand apart before this counts
	// as a machine that was away rather than a tick that was late. The server
	// forgets a card three minutes after the last signal, so beyond that window
	// nothing that was reported is alive any more: the cards may have been let
	// go of while the machine slept, and reporting them again would say
	// something nobody said.
	aJumpInTheClock = 3 * time.Minute

	// nobodyIsAtWorkAfter is how long the keepalive keeps reporting a list that
	// nothing else comes near. A session at work is at the store after every
	// tool call, so a store nothing has touched for this long belongs to
	// sessions that are over, and the last process on the machine is nobody to
	// keep their tiles alight.
	nobodyIsAtWorkAfter = 25 * time.Minute
)

// verdict is what one tick of the keepalive comes to: it reports, it finds
// nothing to decide on, or it is the last tick and says why.
type verdict int

const (
	tickReports verdict = iota
	tickPassesOver
	tickEndsOnAnEmptyList
	tickEndsForNobodyIsAtWork
	tickEndsForTheClockJumped
)

// tick is one wake-up of the keepalive: the moment it woke, the moment it woke
// the time before, and what the store said — which is nothing where somebody
// else held the store for that moment.
type tick struct {
	now    time.Time
	before time.Time

	read  bool
	beat  time.Time
	cards int
}

// decide says what one tick does.
//
// A jump is looked for first, and on the wall clock: a machine that slept comes
// back with a monotonic reading that stood still through the sleep and shows no
// jump at all. The three endings differ on purpose. An empty list is reported
// before the keepalive goes, so that the tiles go out at once; a clock that
// jumped and a store nobody has been at are left unsaid, because the cards may
// still be in hand and the server forgets a signal by itself.
func (t tick) decide() verdict {
	if apart := t.now.Round(0).Sub(t.before.Round(0)); apart >= aJumpInTheClock || apart < 0 {
		return tickEndsForTheClockJumped
	}
	if !t.read {
		return tickPassesOver
	}
	if t.cards == 0 {
		return tickEndsOnAnEmptyList
	}
	if t.now.Sub(t.beat) >= nobodyIsAtWorkAfter {
		return tickEndsForNobodyIsAtWork
	}
	return tickReports
}

// whyItEnded is what the keepalive says of its own last tick. Nothing reads it
// while it runs detached; whoever starts one by hand to see what it does reads
// nothing else.
func whyItEnded(at verdict) string {
	switch at {
	case tickEndsOnAnEmptyList:
		return "no card is in hand any more"
	case tickEndsForNobodyIsAtWork:
		return fmt.Sprintf("nothing but the keepalive has been at the store for %.0f minutes", nobodyIsAtWorkAfter.Minutes())
	case tickEndsForTheClockJumped:
		return "the clock jumped, so the board has long forgotten these cards"
	}
	return ""
}

// reportWhileNobodyElseDoes is the keepalive as a command: it reports until a
// tick says it is over, and then says why. It keeps the manner of the beat it
// was started from, silent unless somebody asked.
func (a *app) reportWhileNobodyElseDoes() error {
	ended, err := a.keepTilesAlive()
	if err != nil {
		return err
	}
	if a.nobodyIsListening() {
		return nil
	}
	said := "The keepalive stopped: " + ended
	return a.ok(success{
		data:    map[string]any{"ended": ended},
		summary: said,
		text:    func(w io.Writer) { fmt.Fprintln(w, said) },
	})
}

// keepTilesAlive reports the cards of this machine on a timer of its own, for as
// long as somebody is at work on them.
//
// A session makes no signal of its own while one tool call runs — a test run, a
// build, an extraction of an hour, the time an answer from the person takes —
// and the server forgets a card three minutes after the last signal, so the tile
// would go dark while the work goes on. This is what reports in between.
func (a *app) keepTilesAlive() (string, error) {
	host, err := a.hostOfTheStore()
	if err != nil {
		return "", err
	}
	path, err := a.storePath()
	if err != nil {
		return "", err
	}

	held, err := work.HoldKeepalive(path, host)
	if err != nil {
		if errors.Is(err, work.ErrBusy) {
			return "another keepalive of " + host + " is reporting", nil
		}
		return "", &Error{Code: CodeConfig, Message: err.Error()}
	}
	defer held.Release()

	ticker := time.NewTicker(keepaliveTick)
	defer ticker.Stop()
	before := a.now()
	for {
		select {
		case <-a.ctx.Done():
			return "it was told to stop", nil
		case <-ticker.C:
		}

		now := a.now()
		one := tick{now: now, before: before}
		one.beat, one.cards, one.read = a.whatTheStoreSays(path, host)
		before = now

		switch at := one.decide(); at {
		case tickPassesOver:
		case tickReports:
			a.signal(waitForTheStore)
		default:
			if at == tickEndsOnAnEmptyList {
				// The empty set goes out before the keepalive does, so that the
				// tiles go out at once instead of after the three minutes the
				// last signal counts for.
				a.signal(waitForTheStore)
			}
			return whyItEnded(at), nil
		}
	}
}

// whatTheStoreSays is what one tick decides on, and nothing at all where
// somebody else holds the store for that moment.
//
// It gives up what ran out like every other writer: when a session dies the
// keepalive is often the last process left on the machine, and a card nobody
// gives up would be reported past the working day it is allowed.
func (a *app) whatTheStoreSays(path, host string) (beat time.Time, cards int, read bool) {
	handle, err := work.Open(path, waitForTheStore)
	if err != nil {
		return time.Time{}, 0, false
	}
	defer handle.Close()

	entry, known := handle.File.Hosts[host]
	if !known {
		return time.Time{}, 0, true
	}
	if entry.ForgetWhatRanOut(a.now()) > 0 {
		_ = handle.Save()
	}
	return entry.BeatAt, len(entry.Cards), true
}
