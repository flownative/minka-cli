package cli

import (
	"context"
	"encoding/json"
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
	if len(e.signals()) != 0 {
		t.Errorf("%d signals went out from a keepalive that got no lock", len(e.signals()))
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
func TestWhatOneTickOfTheKeepaliveDecides(t *testing.T) {
	woke := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	justBefore := woke.Add(-keepaliveTick)
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
			"a list that is empty",
			tick{now: woke, before: justBefore, read: true, beat: amoment, cards: 0},
			tickEndsOnAnEmptyList,
		},
		{
			"a store nobody has been at",
			tick{now: woke, before: justBefore, read: true, beat: woke.Add(-nobodyIsAtWorkAfter), cards: 1},
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
			"a clock that was put back",
			tick{now: woke, before: woke.Add(time.Hour), read: true, beat: amoment, cards: 1},
			tickEndsForTheClockJumped,
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
