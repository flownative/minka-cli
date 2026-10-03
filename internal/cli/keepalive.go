package cli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/config"
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
	tickEndsForTheModsReport
)

// tick is one wake-up of the keepalive for the list of one session: the moment
// it woke, the moment it woke the time before, and what the store said of that
// list — which is nothing where somebody else held the store for that moment.
type tick struct {
	now    time.Time
	before time.Time

	// read says whether the store answered, beat when the session was last
	// there, cards how many it has in hand.
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
	if t.clockJumped() {
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

// clockJumped says whether the two wake-ups stand further apart than a tick can
// account for. A step back of less than a tick is a clock being corrected by a
// second or two, which says nothing about the work: ending on that would kill
// the keepalive in the middle of the long tool call it is there for, where no
// beat is coming to start another.
func (t tick) clockJumped() bool {
	apart := t.now.Round(0).Sub(t.before.Round(0))
	return apart >= aJumpInTheClock || apart <= -keepaliveTick
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
	case tickEndsForTheModsReport:
		return "every session left is reported by the mod in its own Claude Code process"
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

// keepTilesAlive reports the cards of the sessions of this host on a timer of its
// own, each session's list under that session's client and for as long as that
// session is at work on them.
//
// A session makes no signal of its own while one tool call runs, and the server
// forgets a card three minutes after the last signal, so the tile would go dark
// while the work goes on. This is what reports in between, and what it is worth
// is twenty-five minutes from the last beat of the session: nothing is at the
// store while that one call runs, so the mark of a beat stands still with it and
// the keepalive stops reporting that session that long after it. A call that
// lasts longer outlives its tiles.
//
// Every session is decided on by its own beat and nobody else's. One timer for
// the host is enough for that, and it must not be more: a session that is busy
// says nothing about a session that died, so the cards of that one are reported
// only for as long as its own last beat allows.
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
		lists, err := a.whatTheStoreSays(path, host)
		// A store somebody else holds is a moment to pass over; a store that
		// cannot be read is the end. Otherwise a keepalive over a broken store
		// would tick until the machine is restarted, report nothing, and hold the
		// lock of its host against every beat that tried to start a live one.
		if err != nil && !errors.Is(err, work.ErrBusy) {
			return "the store cannot be read: " + err.Error()
		}
		if (tick{now: now, before: before}).clockJumped() {
			return whyItEnded(tickEndsForTheClockJumped)
		}
		if err == nil {
			if over := a.reportEverySessionAtWork(lists, now, before); over != tickReports {
				return whyItEnded(over)
			}
		}
		before = now

		select {
		case <-a.ctx.Done():
			return "it was told to stop"
		case <-ticker.C:
		}
	}
}

// reportEverySessionAtWork is one tick over the lists of the host: it reports
// every session that is at work, and takes back the set of every session that
// has nothing in hand any more. It answers tickReports while any session is
// still at work, and otherwise the verdict the keepalive ends on.
//
// An empty list is reported before the keepalive lets go of it, so that the
// tiles go out at once instead of after the three minutes the last signal counts
// for; the signal sends nothing where nothing was said before. A list whose
// session has not been at the store for too long is not reported at all: the
// cards may still be in hand, and the server forgets them by itself.
//
// Nor is a list the server has already forgotten while its session said
// nothing since: a keepalive that starts after a machine woke up, or after the
// last one stopped, would otherwise light the tiles of a session that is gone
// once more. Its next beat lights them again where it is not.
//
// A list whose session runs the mod is not touched at all. The mod beats for as
// long as the process of the session runs, so when its beats stop the process
// is gone, and the cards have to go dark three minutes after its last signal.
func (a *app) reportEverySessionAtWork(lists []sessionAtATick, now, before time.Time) verdict {
	reporting, nobodyAtWork, leftToTheMod := false, false, false
	for _, list := range lists {
		if list.mod {
			leftToTheMod = true
			continue
		}
		switch at := (tick{now: now, before: before, read: true, beat: list.beat, cards: list.cards}).decide(); at {
		case tickReports:
			if list.forgottenByTheServer(now) {
				nobodyAtWork = true
				continue
			}
			a.signal(list.session, aBeatWaitsForTheStore)
			reporting = true
		case tickEndsOnAnEmptyList:
			a.signal(list.session, aBeatWaitsForTheStore)
		case tickEndsForNobodyIsAtWork:
			nobodyAtWork = true
		}
	}
	switch {
	case reporting:
		return tickReports
	case nobodyAtWork:
		return tickEndsForNobodyIsAtWork
	case leftToTheMod:
		return tickEndsForTheModsReport
	}
	return tickEndsOnAnEmptyList
}

// sessionAtATick is what one tick knows of the list of one session: when the
// session was last at the store, how many cards it holds, when a signal of it
// last went out, and whether the mod of its process reports it.
type sessionAtATick struct {
	session string
	beat    time.Time
	cards   int
	sent    time.Time
	mod     bool
}

// forgottenByTheServer says that the server no longer counts the last signal
// of the list, and that its session has not been at the store since.
func (s sessionAtATick) forgottenByTheServer(now time.Time) bool {
	return now.Sub(s.sent) >= work.HeldFor && !s.beat.After(s.sent)
}

// whatTheStoreSays is what one tick decides on: the list of every session of
// the host, in the order of their names. ErrBusy says that somebody else held
// the store for that moment and nothing else is wrong with it; any other error
// is what it says.
//
// It gives up what ran out like every other writer: when a session dies the
// keepalive is often the last process left on the machine, and a card nobody
// gives up would be reported past the working day it is allowed.
func (a *app) whatTheStoreSays(path, hostName string) ([]sessionAtATick, error) {
	handle, err := work.Open(path, aBeatWaitsForTheStore)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	host, known := handle.File.Hosts[hostName]
	if !known {
		return nil, nil
	}
	if _, swept := host.ForgetWhatRanOut(work.NoSession, a.now()); swept {
		_ = handle.Save()
	}
	var lists []sessionAtATick
	for _, session := range slices.Sorted(maps.Keys(host.Sessions)) {
		list := host.Sessions[session]
		lists = append(lists, sessionAtATick{session: session, beat: list.BeatAt, cards: len(list.Cards), sent: list.SentAt, mod: list.Mod})
	}
	return lists, nil
}

// What heartbeat says became of the keepalive.
const (
	keepaliveStarted   = "started"
	keepaliveReporting = "already reporting"
)

// whatBecameOfTheKeepalive starts the process that reports while this session is
// busy with one long tool call, and says what became of it. Somebody looking
// into a tile that stays dark has nothing else to go on.
func (a *app) whatBecameOfTheKeepalive(session string) string {
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

	list, err := a.theListOf(path, host, session)
	switch {
	case errors.Is(err, work.ErrBusy):
		return notStarted("the store is busy")
	case err != nil:
		return notStarted("the store cannot be read: " + err.Error())
	case len(list.Cards) == 0:
		return notStarted("no card is in hand")
	case list.Mod:
		return notStarted(theModReports)
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

// theListOf is the list of one session on this host, an empty one where it
// holds nothing.
//
// The signal beside it says only what it carried, and the throttle holds most
// beats back, so whoever has to know whether anybody is at work asks the store.
// It waits for nobody: where somebody else holds the store, the next beat asks
// again.
func (a *app) theListOf(path, hostName, session string) (*work.List, error) {
	handle, err := work.Open(path, 0)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	host, known := handle.File.Hosts[hostName]
	if !known {
		return &work.List{}, nil
	}
	return listOf(host, session), nil
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
	// An argument is read by every process on the machine, which is why no token
	// comes through a flag either. Only a host that has been read may go there:
	// what the configuration holds reaches this seam through the commands that
	// refuse an unreadable value, and the seam holds that whether they do or not.
	// The refusal names nothing of the value; there is nothing in it to say.
	if _, err := config.NormalizeHost(host); err != nil {
		return errors.New("the host it would report to is not one that was read")
	}

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
