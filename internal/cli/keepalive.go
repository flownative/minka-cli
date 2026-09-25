package cli

import "time"

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
