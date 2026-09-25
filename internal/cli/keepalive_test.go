package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/work"
)

// runAsTheKeepalive runs the CLI at a moment and in the mode of the process that
// keeps reporting while the session around it is busy.
func (e *testEnv) runAsTheKeepalive(at time.Time, args ...string) result {
	e.t.Helper()
	return e.runWith(func(a *app) {
		a.now = func() time.Time { return at }
		a.keepalive = true
	}, "", args...)
}

// storedHost is what the store holds for the host of these tests, as it lies on
// disk: what a command wrote, not what a command shows of it.
func (e *testEnv) storedHost() map[string]any {
	e.t.Helper()
	content, err := os.ReadFile(e.storePath())
	if err != nil {
		e.t.Fatal(err)
	}
	var file struct {
		Hosts map[string]map[string]any `json:"hosts"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		e.t.Fatalf("the store is not JSON: %v: %s", err, content)
	}
	entry, known := file.Hosts[e.vars["MINKA_HOST"]]
	if !known {
		e.t.Fatalf("the store holds nothing for %s: %s", e.vars["MINKA_HOST"], content)
	}
	return entry
}

// writeConfig puts a configuration file where the CLI of these tests reads one,
// with the permissions it insists on.
func (e *testEnv) writeConfig(content string) {
	e.t.Helper()
	path := e.vars["MINKA_CONFIG"]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

// theApp is the CLI as one run of it at a moment, for the few things a test asks
// of it directly instead of through a command line.
func (e *testEnv) theApp(at time.Time) *app {
	e.t.Helper()
	a := newApp(context.Background(), strings.NewReader(""), io.Discard, io.Discard,
		func(name string) string { return e.vars[name] })
	a.now = func() time.Time { return at }
	return a
}

// asStored is a moment the way the store writes it down.
func asStored(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

// runKeepaliveThatIsToldToStop runs the keepalive of a process that is told to
// stop straight away, so that what a test reads is what it decided before its
// first tick and never a timer.
func (e *testEnv) runKeepaliveThatIsToldToStop(at time.Time) (envelope, result) {
	e.t.Helper()
	told, stop := context.WithCancel(context.Background())
	stop()
	r := e.runWith(func(a *app) {
		a.now = func() time.Time { return at }
		a.ctx = told
	}, "", "heartbeat", "--keepalive", "--json")
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

// The safeguard from the side of the command: a second keepalive of the same
// host gives up before it has reported anything. Two of them would report the
// same list on timers of their own, neither hearing when the other stopped, and
// the one that outlived its session would report through the other's silence.
func TestASecondKeepaliveOfTheSameHostReportsNothing(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	held, err := work.HoldKeepalive(e.storePath(), e.vars["MINKA_HOST"])
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	env, r := e.runKeepaliveThatIsToldToStop(began.Add(time.Second))
	expectCode(t, r, ExitOK)
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "is reporting") {
		t.Errorf("the second keepalive ended for %q, want the one that is reporting", ended)
	}
	// It reports before its timer once it has the lock, so what says it did
	// nothing at all is the moment of a signal that was never claimed.
	if got := e.storedHost()["sentAt"]; got != asStored(time.Time{}) {
		t.Errorf("a keepalive that got no lock claimed the signal of %v", got)
	}
}

// The safeguard: the flag is what puts a run into the mode that is at no work.
// The first report comes before the timer, so a keepalive that is told to stop
// straight away has been at the store once — and what it left there is nothing:
// no mark of a beat, no sign of life, although the name of the session that
// spawned it stands in the environment around it.
func TestTheFlagPutsTheRunIntoTheModeThatIsAtNoWork(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	e.vars[sessionVariable] = "the session that spawned it"
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	// Two minutes on, where a beat of a session would write both marks.
	env, r := e.runKeepaliveThatIsToldToStop(began.Add(2 * time.Minute))
	expectCode(t, r, ExitOK)
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "told to stop") {
		t.Errorf("the keepalive ended for %q", ended)
	}
	entry := e.storedHost()
	// The moment of a signal is claimed in the store before the request goes out,
	// so this is what says that the keepalive reported at all, and that it did so
	// before its timer.
	if got := entry["sentAt"]; got != asStored(began.Add(2*time.Minute)) {
		t.Fatalf("the keepalive claimed the signal of %v, want the moment it started", got)
	}
	if got := entry["beatAt"]; got != asStored(began) {
		t.Errorf("the mark of a beat stands at %v, want the moment the card was taken up", got)
	}
	cards, _ := entry["cards"].([]any)
	if len(cards) != 1 {
		t.Fatalf("%d cards, want the one that was taken up", len(cards))
	}
	if seen := cards[0].(map[string]any)["seenAt"]; seen != asStored(time.Time{}) {
		t.Errorf("the keepalive wrote the sign of life %v of a card it is at no work on", seen)
	}
}

// The safeguard: only a store somebody else holds for the moment is a moment to
// pass over. One that cannot be read at all ends the keepalive, which otherwise
// would tick until the machine is restarted, report nothing, and hold the lock
// of its host against every beat that tried to start a live one.
func TestAKeepaliveOverAStoreItCannotReadStopsAndLetsGoOfTheLock(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	if err := os.WriteFile(e.storePath(), []byte("{ this is no store"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runKeepaliveThatIsToldToStop(began.Add(time.Second))
	expectCode(t, r, ExitOK)
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "cannot be read") {
		t.Errorf("the keepalive ended for %q, want the store it cannot read", ended)
	}
	if reporting, err := work.SomebodyKeepsAlive(e.storePath(), e.vars["MINKA_HOST"]); err != nil || reporting {
		t.Errorf("the lock is still held: %v, %v", reporting, err)
	}
}

// The keepalive holds the lock while it reports and gives it back when it stops,
// so that the next long tool call can start one again.
func TestAKeepaliveHoldsTheLockOnlyWhileItReports(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runKeepaliveThatIsToldToStop(began.Add(time.Second))
	expectCode(t, r, ExitOK)
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "told to stop") {
		t.Errorf("the keepalive ended for %q", ended)
	}
	if reporting, err := work.SomebodyKeepsAlive(e.storePath(), e.vars["MINKA_HOST"]); err != nil || reporting {
		t.Errorf("the lock is still held after the keepalive stopped: %v, %v", reporting, err)
	}
}

// Nothing the keepalive meets is a failure, the way nothing a beat meets is
// one: a machine that is signed in nowhere makes it stop and say why, and it
// exits 0 all the same.
func TestAKeepaliveWithNoHostToReportToStopsAndSaysSo(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")

	env, r := e.runKeepaliveThatIsToldToStop(began.Add(time.Second))
	expectCode(t, r, ExitOK)
	if !env.OK {
		t.Fatalf("the keepalive failed: %s", env.Error)
	}
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "no host") {
		t.Errorf("the keepalive ended for %q", ended)
	}
}

// A beat that finds a card in hand starts the keepalive, and says so where
// anybody asks: a tile that stays dark is diagnosed from that word and nothing
// else. Where one is reporting already, no second is started.
func TestABeatWithACardInHandStartsTheKeepalive(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runWithJSON(began, "heartbeat")
	expectCode(t, r, ExitOK)
	if became := decode(t, env.Data)["keepalive"]; became != keepaliveStarted {
		t.Errorf("the keepalive %v", became)
	}
	if started := e.keepalives.Load(); started != 1 {
		t.Fatalf("%d keepalives were started, want 1", started)
	}

	held, err := work.HoldKeepalive(e.storePath(), e.vars["MINKA_HOST"])
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	env, r = e.runWithJSON(began.Add(2*time.Minute), "heartbeat")
	expectCode(t, r, ExitOK)
	if became := decode(t, env.Data)["keepalive"]; became != keepaliveReporting {
		t.Errorf("the keepalive %v, want the one that is reporting", became)
	}
	if started := e.keepalives.Load(); started != 1 {
		t.Errorf("%d keepalives were started, want the one", started)
	}
}

// The keepalive is told which host it reports for. A beat that was given --host
// reports somewhere other than the host this machine is signed in to by default,
// and a child left to decide for itself would report the default one's cards
// while the tiles of the host that was named went dark — and, its lock being
// free, every beat after it would start another child.
func TestTheKeepaliveIsToldTheHostTheBeatDecidedOn(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	named, byDefault := e.server.URL, "https://elsewhere.example.com"

	// Signed in to two hosts, the other one of them by default.
	e.writeConfig(`{"defaultHost": "` + byDefault + `", "hosts": {
		"` + byDefault + `": {"token": "minka_pat_elsewhere"},
		"` + named + `": {"token": "` + e.fixture.AgentToken + `"}}}`)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")

	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--host", named, "--json"), ExitOK)
	env, r := e.runWithJSON(began, "heartbeat", "--host", named)
	expectCode(t, r, ExitOK)
	if became := decode(t, env.Data)["keepalive"]; became != keepaliveStarted {
		t.Fatalf("the keepalive %v", became)
	}
	if told, _ := e.keepaliveHost.Load().(string); told != named {
		t.Errorf("the keepalive was told %q, want %s", told, named)
	}

	// And that is how the child is called.
	if called := strings.Join(keepaliveArguments(named), " "); called != "heartbeat --keepalive --host "+named {
		t.Errorf("the child is called with %q", called)
	}
}

// A beat with nothing in hand starts nothing, and where nothing was ever
// remembered it does not so much as make the directory of the store.
func TestABeatWithNothingInHandStartsNoKeepalive(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)

	env, r := e.runWithJSON(began, "heartbeat")
	expectCode(t, r, ExitOK)
	if became, _ := decode(t, env.Data)["keepalive"].(string); !strings.HasPrefix(became, "not started") {
		t.Errorf("the keepalive %q", became)
	}
	if _, err := os.Stat(filepath.Dir(e.storePath())); err == nil {
		t.Error("the beat made a store although nothing was remembered")
	}

	// And with a list that was emptied: the cards are what it keeps alive.
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.runAt(began, "work", "stop", "--all", "--json"), ExitOK)
	env, r = e.runWithJSON(began.Add(time.Minute), "heartbeat")
	expectCode(t, r, ExitOK)
	if became, _ := decode(t, env.Data)["keepalive"].(string); !strings.Contains(became, "no card is in hand") {
		t.Errorf("the keepalive %q", became)
	}
	if started := e.keepalives.Load(); started != 0 {
		t.Errorf("%d keepalives were started with nothing in hand", started)
	}
}

// The keepalive is started with the environment of the beat that starts it, less
// the name of the session: a card binds itself to that name, and the keepalive is
// at no work of its own.
func TestTheKeepaliveIsGivenNoSessionToBeAtWorkAs(t *testing.T) {
	given := environmentWithoutTheSession([]string{
		"MINKA_HOST=https://minka.example.com",
		sessionVariable + "=the session that spawned it",
		sessionVariable + "X=something else entirely",
		"PATH=/usr/bin",
	})

	for _, entry := range given {
		if strings.HasPrefix(entry, sessionVariable+"=") {
			t.Errorf("the keepalive is given %q", entry)
		}
	}
	if len(given) != 3 {
		t.Errorf("the keepalive is given %v, want everything but the name of the session", given)
	}
}

// The keepalive is minka's own business: the catalog every agent reads offers no
// way to call it.
func TestTheKeepaliveIsInNoCatalog(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "commands", "--json")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "keepalive") {
		t.Error("the catalog offers the keepalive")
	}
}

// The safeguard, among the four a tick can come to: a tick whose clock jumped
// reports nothing. A machine that wakes from a sleep of hours holds cards whose
// tiles went out long ago, and a signal that raised them again would say that
// somebody is at work on them.
//
// The moments are written out rather than built from the constants they are to
// hold: a tick of a minute and a half, or a deadline of twenty-five hours, would
// pass a table that measures itself.
func TestWhatOneTickOfTheKeepaliveDecides(t *testing.T) {
	woke := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	justBefore := woke.Add(-30 * time.Second)
	amoment := woke.Add(-time.Minute)

	for _, one := range []struct {
		name string
		tick tick
		want verdict
	}{
		{
			"a card in hand and somebody at work",
			tick{now: woke, before: justBefore, read: true, beat: amoment, cards: 1},
			tickReports,
		},
		{
			"a store somebody was at twenty-four minutes ago",
			tick{now: woke, before: justBefore, read: true, beat: woke.Add(-24 * time.Minute), cards: 1},
			tickReports,
		},
		{
			"a list that is empty",
			tick{now: woke, before: justBefore, read: true, beat: amoment, cards: 0},
			tickEndsOnAnEmptyList,
		},
		{
			"a store nobody has been at for twenty-six minutes",
			tick{now: woke, before: justBefore, read: true, beat: woke.Add(-26 * time.Minute), cards: 1},
			tickEndsForNobodyIsAtWork,
		},
		{
			"a machine that slept",
			tick{now: woke, before: woke.Add(-8 * time.Hour), read: true, beat: amoment, cards: 1},
			tickEndsForTheClockJumped,
		},
		{
			"a machine that slept over an empty list",
			tick{now: woke, before: woke.Add(-8 * time.Hour), read: true, beat: amoment, cards: 0},
			tickEndsForTheClockJumped,
		},
		{
			"a tick four minutes late",
			tick{now: woke, before: woke.Add(-4 * time.Minute), read: true, beat: amoment, cards: 1},
			tickEndsForTheClockJumped,
		},
		{
			"a clock put back by an hour",
			tick{now: woke, before: woke.Add(time.Hour), read: true, beat: amoment, cards: 1},
			tickEndsForTheClockJumped,
		},
		{
			"a clock corrected by a second, which says nothing about the work",
			tick{now: woke, before: woke.Add(time.Second), read: true, beat: amoment, cards: 1},
			tickReports,
		},
		{
			"a store somebody else held for the moment",
			tick{now: woke, before: justBefore, read: false},
			tickPassesOver,
		},
	} {
		if got := one.tick.decide(); got != one.want {
			t.Errorf("%s: verdict %d, want %d", one.name, got, one.want)
		}
	}
}

// The tick has to be shorter than the throttle of the signal, or the keepalive
// would report on its own rate instead of the throttle's, fall short of the
// minute every second or third time, and let the gap grow past the three minutes
// the server counts a signal for: the tile would flicker.
func TestTheKeepaliveTicksOftenerThanItReports(t *testing.T) {
	if keepaliveTick >= signalEvery {
		t.Errorf("the keepalive ticks every %s and reports every %s", keepaliveTick, signalEvery)
	}
	if aJumpInTheClock <= signalEvery {
		t.Errorf("a jump is %s, which is not longer than the %s between two signals", aJumpInTheClock, signalEvery)
	}
}

// The safeguard: every tick gives up what ran out. When a session dies the
// keepalive is often the last process left on the machine, so without this it
// would report a card past the working day a card is allowed.
func TestATickOfTheKeepaliveGivesUpWhatRanOut(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	tooLate := began.Add(work.ForgottenAfter + time.Minute)
	beat, cards, err := e.theApp(tooLate).whatTheStoreSays(e.storePath(), e.vars["MINKA_HOST"])
	if err != nil {
		t.Fatal(err)
	}
	if cards != 0 {
		t.Errorf("the tick found %d cards, want none: the card ran out a minute ago", cards)
	}
	if !beat.Equal(began) {
		t.Errorf("the tick read the mark of a beat as %s, want %s", beat, began)
	}
	// And it is gone for every other reader too, not only for this tick.
	if left := e.storedHost()["cards"].([]any); len(left) != 0 {
		t.Errorf("%v is left in the store", left)
	}
}

// The safeguard: a beat of the keepalive is nobody at work. It writes neither
// the mark that says somebody was at the store nor the sign of life of a card,
// and it writes neither although the name of the session that spawned it stands
// in the environment around it. Left to the environment alone, one stray
// variable would be enough for a card to glow through a working day while
// nothing ever told the keepalive to stop.
func TestABeatOfTheKeepaliveIsNobodyAtWork(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	e.vars["CLAUDE_CODE_SESSION_ID"] = "the session that spawned it"
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	for minute := 30; minute <= 7*60; minute += 30 {
		expectCode(t, e.runAsTheKeepalive(began.Add(time.Duration(minute)*time.Minute), "heartbeat", "--json"), ExitOK)
	}

	entry := e.storedHost()
	if got := entry["beatAt"]; got != asStored(began) {
		t.Errorf("the mark of a beat stands at %v, want the moment the card was taken up", got)
	}
	cards, _ := entry["cards"].([]any)
	if len(cards) != 1 {
		t.Fatalf("%d cards, want the one that was taken up", len(cards))
	}
	if seen := cards[0].(map[string]any)["seenAt"]; seen != asStored(time.Time{}) {
		t.Errorf("the keepalive wrote the sign of life %v of a card it is at no work on", seen)
	}

	// So the card runs out on the working day it is allowed, however long the
	// keepalive keeps reporting it.
	env, r := e.runWithJSON(began.Add(work.ForgottenAfter+time.Minute), "work", "list")
	expectCode(t, r, ExitOK)
	if left := cardIDsOfTheList(t, env); len(left) != 0 {
		t.Errorf("%v is left after a working day of nothing but keepalive beats", left)
	}
}

// A beat of a session and a command somebody typed say that the work is going
// on. Looking at the list says nothing: looking is not working, and a command
// that only reads does not become a writer for this.
func TestWhatSaysThatSomebodyIsStillAtTheStore(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	e.vars["CLAUDE_CODE_SESSION_ID"] = "a session at work"

	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	if got := e.storedHost()["beatAt"]; got != asStored(began) {
		t.Errorf("work start left the mark at %v, want %s", got, asStored(began))
	}

	looked := began.Add(5 * time.Minute)
	expectCode(t, e.runAt(looked, "work", "list", "--json"), ExitOK)
	if got := e.storedHost()["beatAt"]; got != asStored(began) {
		t.Errorf("work list moved the mark to %v", got)
	}

	beat := began.Add(10 * time.Minute)
	expectCode(t, e.runAt(beat, "heartbeat", "--json"), ExitOK)
	if got := e.storedHost()["beatAt"]; got != asStored(beat) {
		t.Errorf("the beat of a session left the mark at %v, want %s", got, asStored(beat))
	}

	stopped := began.Add(20 * time.Minute)
	expectCode(t, e.runAt(stopped, "work", "stop", "--all", "--json"), ExitOK)
	if got := e.storedHost()["beatAt"]; got != asStored(stopped) {
		t.Errorf("work stop left the mark at %v, want %s", got, asStored(stopped))
	}
}
