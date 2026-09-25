package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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

	// read says whether the store answered, beat when somebody at work was last
	// there, cards how many are in hand.
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
	ended := a.keepTilesAlive()
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
//
// Nothing it meets is a failure, the way nothing a beat meets is one: it says
// why it stopped and leaves it at that.
func (a *app) keepTilesAlive() string {
	host, err := a.hostOfTheStore()
	if err != nil {
		return "it has no host to report to: " + err.Error()
	}
	path, err := a.storePath()
	if err != nil {
		return "it has no store to read: " + err.Error()
	}

	held, err := work.HoldKeepalive(path, host)
	if err != nil {
		if errors.Is(err, work.ErrBusy) {
			return "another keepalive of " + host + " is reporting"
		}
		return "the lock beside the store cannot be taken: " + err.Error()
	}
	defer held.Release()

	ticker := time.NewTicker(keepaliveTick)
	defer ticker.Stop()

	// The first tick is the moment the lock was taken, not a tick away from it:
	// whoever holds the reporting from here on says so at once, and a keepalive
	// that can do nothing for its host finds that out before it settles into a
	// timer.
	before := a.now()
	for {
		now := a.now()
		beat, cards, err := a.whatTheStoreSays(path, host)
		// A store somebody else holds is a moment to pass over; a store that
		// cannot be read is the end. Otherwise a keepalive over a broken store
		// would tick until the machine is restarted, report nothing, and hold the
		// lock of its host against every beat that tried to start a live one.
		if err != nil && !errors.Is(err, work.ErrBusy) {
			return "the store cannot be read: " + err.Error()
		}
		at := tick{now: now, before: before, read: err == nil, beat: beat, cards: cards}.decide()
		before = now

		switch at {
		case tickPassesOver:
		case tickReports:
			a.signal(waitForTheStore)
		case tickEndsOnAnEmptyList:
			// The empty set goes out before the keepalive does, so that the tiles
			// go out at once instead of after the three minutes the last signal
			// counts for.
			a.signal(waitForTheStore)
			return whyItEnded(at)
		default:
			return whyItEnded(at)
		}

		select {
		case <-a.ctx.Done():
			return "it was told to stop"
		case <-ticker.C:
		}
	}
}

// whatTheStoreSays is what one tick decides on. ErrBusy says that somebody else
// held the store for that moment and nothing else is wrong with it; any other
// error is what it says.
//
// It gives up what ran out like every other writer: when a session dies the
// keepalive is often the last process left on the machine, and a card nobody
// gives up would be reported past the working day it is allowed.
func (a *app) whatTheStoreSays(path, host string) (beat time.Time, cards int, err error) {
	handle, err := work.Open(path, waitForTheStore)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer handle.Close()

	entry, known := handle.File.Hosts[host]
	if !known {
		return time.Time{}, 0, nil
	}
	if entry.ForgetWhatRanOut(a.now()) > 0 {
		_ = handle.Save()
	}
	return entry.BeatAt, len(entry.Cards), nil
}

// What heartbeat says became of the keepalive.
const (
	keepaliveStarted   = "started"
	keepaliveReporting = "already reporting"
)

// whatBecameOfTheKeepalive starts the process that reports while this session is
// busy with one long tool call, and says what became of it. Somebody looking
// into a tile that stays dark has nothing else to go on.
func (a *app) whatBecameOfTheKeepalive() string {
	path, err := a.storePath()
	if err != nil {
		return notStarted(err.Error())
	}
	if work.Untouched(path) {
		return notStarted("nothing is remembered on this machine")
	}
	host, err := a.hostOfTheStore()
	if err != nil {
		return notStarted(err.Error())
	}

	cards, err := a.cardsInTheList(path, host)
	switch {
	case errors.Is(err, work.ErrBusy):
		return notStarted("the store is busy")
	case err != nil:
		return notStarted("the store cannot be read: " + err.Error())
	case cards == 0:
		return notStarted("no card is in hand")
	}

	reporting, err := work.SomebodyKeepsAlive(path, host)
	if err != nil {
		return notStarted(err.Error())
	}
	if reporting {
		return keepaliveReporting
	}
	if err := a.startKeepalive(host); err != nil {
		return notStarted(err.Error())
	}
	return keepaliveStarted
}

func notStarted(why string) string {
	return "not started: " + why
}

// cardsInTheList is how many cards this host holds.
//
// The signal beside it says only what it carried, and the throttle holds most
// beats back, so whoever has to know whether anybody is at work asks the store.
// It waits for nobody: where somebody else holds the store, the next beat asks
// again.
func (a *app) cardsInTheList(path, host string) (int, error) {
	handle, err := work.Open(path, 0)
	if err != nil {
		return 0, err
	}
	defer handle.Close()

	entry, known := handle.File.Hosts[host]
	if !known {
		return 0, nil
	}
	return len(entry.Cards), nil
}

// spawnKeepalive starts the keepalive as a process of its own, for the host the
// beat that starts it decided on.
//
// The host travels as an argument because the child decides nothing for itself:
// a beat told another host with --host would otherwise start a keepalive for the
// default one, which reports cards nobody asked about while the tiles of the
// host that was named go dark.
//
// Claude Code reads the output of a hook through a pipe, and a child that holds
// the write end of it keeps that pipe from ever reaching its end: the tool call
// would then wait for as long as the keepalive lives, which is the opposite of
// what it is for. So all three streams go to the null device and nobody waits for
// the child. It gets a session of its own, which is also what keeps the Ctrl-C
// meant for the agent away from it, and a directory that holds no project, so
// that a working tree somebody deletes does not take it along.
func spawnKeepalive(host string, environment []string) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()

	started := exec.Command(program, keepaliveArguments(host)...)
	started.Stdin, started.Stdout, started.Stderr = null, null, null
	started.Dir = os.TempDir()
	started.Env = environment
	detach(started)
	return started.Start()
}

// keepaliveArguments is how the child is called: as the keepalive of one host,
// named so that it reports where the beat that started it reports and nowhere
// else.
func keepaliveArguments(host string) []string {
	return []string{"heartbeat", "--keepalive", "--host", host}
}

// environmentWithoutTheSession is what the keepalive is started with: the
// environment of the beat that starts it, less the name of the session.
//
// A card binds itself to the session it finds in that variable, and the
// keepalive is at no work of its own. Its beats say nothing about the work
// whatever they find in the environment; not being given the name is the other
// half of the same rule.
func environmentWithoutTheSession(environment []string) []string {
	kept := make([]string, 0, len(environment))
	for _, entry := range environment {
		if strings.HasPrefix(entry, sessionVariable+"=") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
