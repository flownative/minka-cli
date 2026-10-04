package cli

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/work"
)

// The mod of a Claude Code process names its session with --session: the
// commands it runs are not given $CLAUDE_CODE_SESSION_ID.
const sessionOfTheMod = "the-session-of-the-mod"

// beatOfTheMod is one beat as the mod sends it: after a step, or while a turn is
// under way.
func (e *testEnv) beatOfTheMod(at time.Time) map[string]any {
	e.t.Helper()
	env, r := e.runWithJSON(at, "heartbeat", "--session", sessionOfTheMod, "--from-mod")
	expectCode(e.t, r, ExitOK)
	return decode(e.t, env.Data)
}

// idleBeatOfTheMod is a beat of the mod while no turn is under way.
func (e *testEnv) idleBeatOfTheMod(at time.Time) map[string]any {
	e.t.Helper()
	env, r := e.runWithJSON(at, "heartbeat", "--session", sessionOfTheMod, "--from-mod", "--idle")
	expectCode(e.t, r, ExitOK)
	return decode(e.t, env.Data)
}

// startWorkInTheSessionOfTheMod takes a card up the way a session with the mod
// does: typed in its Bash tool, which carries the session in the environment,
// and followed by the beat the mod sends after the tool call.
func (e *testEnv) startWorkInTheSessionOfTheMod(at time.Time) {
	e.t.Helper()
	e.vars[sessionVariable] = sessionOfTheMod
	expectCode(e.t, e.runAt(at, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	delete(e.vars, sessionVariable)
	e.beatOfTheMod(at)
}

// listOfTheMod is what the store holds for the session of the mod.
func (e *testEnv) listOfTheMod() map[string]any {
	e.t.Helper()
	e.vars[sessionVariable] = sessionOfTheMod
	defer delete(e.vars, sessionVariable)
	return e.storedList()
}

// A beat of the mod reports the session it names, whatever the environment of
// the process says.
func TestABeatOfTheModReportsTheSessionItNames(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.vars[sessionVariable] = sessionOfTheMod
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	e.vars[sessionVariable] = "another-session"

	data := e.beatOfTheMod(began)
	if data["sent"] != true || data["session"] != sessionOfTheMod {
		t.Fatalf("the beat said %v", data)
	}
	if ids := e.cardIDsOfTheSignal(); !slices.Equal(ids, []string{e.fixture.CardInDoing.ID}) {
		t.Errorf("the signal carried %v", ids)
	}
}

// A beat while no turn is under way keeps the cards alight for twenty-five
// minutes after the last step, and says nothing about the work: a Claude Code
// left open over night does not keep its cards glowing until the morning, nor
// keep them past the working day a card is kept for.
func TestAnIdleBeatKeepsTheCardsOnlyForAWhileAfterTheLastStep(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	list := e.listOfTheMod()
	beatAt, seenAt := list["beatAt"], list["cards"].([]any)[0].(map[string]any)["seenAt"]

	lastSent := began
	for at := began.Add(30 * time.Second); at.Before(began.Add(nobodyIsAtWorkAfter)); at = at.Add(30 * time.Second) {
		if data := e.idleBeatOfTheMod(at); data["sent"] == true {
			lastSent = at
		}
	}
	if lastSent.Before(began.Add(nobodyIsAtWorkAfter - 2*time.Minute)) {
		t.Errorf("idle beats reported last at %s, want them to keep the card alight for %s", lastSent, nobodyIsAtWorkAfter)
	}
	list = e.listOfTheMod()
	if list["beatAt"] != beatAt || list["cards"].([]any)[0].(map[string]any)["seenAt"] != seenAt {
		t.Errorf("an idle beat said somebody is at work: %v", list)
	}

	sent := len(e.signals())
	for at := began.Add(nobodyIsAtWorkAfter); at.Before(began.Add(3 * time.Hour)); at = at.Add(30 * time.Second) {
		e.idleBeatOfTheMod(at)
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d idle beats reported after nobody was at work for %s", got-sent, nobodyIsAtWorkAfter)
	}
	if data := e.idleBeatOfTheMod(began.Add(3 * time.Hour)); !strings.Contains(data["reason"].(string), "nobody has been at work") {
		t.Errorf("the idle beat says %q", data["reason"])
	}

	// The next step is work again, and lights the card at once.
	if data := e.beatOfTheMod(began.Add(3 * time.Hour)); data["sent"] != true {
		t.Errorf("the next step sent nothing: %v", data)
	}
}

// An idle beat does not light again what the server forgot while nobody was at
// work: the last signal is older than the three minutes the server counts it
// for, and nobody has been at the store since, though that is well within the
// twenty-five minutes an idle session is kept alight. Only a step does.
func TestAnIdleBeatDoesNotRelightWhatTheServerForgot(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	e.startWorkInTheSessionOfTheMod(began)
	sent := len(e.signals())

	data := e.idleBeatOfTheMod(began.Add(work.HeldFor + time.Minute))
	if data["sent"] != false || !strings.Contains(data["reason"].(string), "forgotten") {
		t.Errorf("the idle beat said %v", data)
	}
	if got := len(e.signals()); got != sent {
		t.Errorf("%d signals went out", got-sent)
	}
	if data := e.beatOfTheMod(began.Add(work.HeldFor + time.Minute)); data["sent"] != true {
		t.Errorf("the next step sent nothing: %v", data)
	}
}

// The mod reports one session; a beat that names none reports nothing, so
// that the list of no session is never kept alight from inside a process.
func TestABeatOfTheModWithoutASessionReportsNothing(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runWithJSON(began, "heartbeat", "--from-mod")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["sent"] != false || data["reason"] != "the mod names no session" {
		t.Errorf("the beat said %v", data)
	}
	if len(e.signals()) != 0 {
		t.Error("a signal went out")
	}
}
