package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/work"
)

// signalBody is one signal as it went out: the client that said it and the
// cards it named.
type signalBody struct {
	ClientID string   `json:"clientId"`
	CardIDs  []string `json:"cardIds"`
}

// signalsSince are the signals sent after the first n, decoded.
func (e *testEnv) signalsSince(n int) []signalBody {
	e.t.Helper()
	var bodies []signalBody
	for _, signal := range e.signals()[n:] {
		var body signalBody
		if err := json.Unmarshal(signal.Body, &body); err != nil {
			e.t.Fatalf("the body of the signal is not JSON: %v", err)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

// The safeguard: a session reports its own cards and nobody else's. The server
// keeps a card alight for as long as any client keeps naming it, so a session
// that carried the cards of another would keep them glowing after that one
// died — for as long as it works on, and up to the hours a card is kept. Session
// A takes a card up, says so once and is gone; session B works on for hours, and
// not one of its signals names A's card.
func TestASessionNeverReportsTheCardsOfAnother(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	ofA, ofB := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars[sessionVariable] = "session-a"
	expectCode(t, e.runAt(began, "work", "start", ofA, "--json"), ExitOK)
	expectCode(t, e.runAt(began, "heartbeat", "--json"), ExitOK)
	fromA := e.signalsSince(0)
	if len(fromA) != 1 || !slices.Equal(fromA[0].CardIDs, []string{ofA}) {
		t.Fatalf("session A sent %+v, want its own card", fromA)
	}

	e.vars[sessionVariable] = "session-b"
	expectCode(t, e.runAt(began.Add(time.Minute), "work", "start", ofB, "--json"), ExitOK)
	for minute := 1; minute <= 6*60; minute += 7 {
		expectCode(t, e.runAt(began.Add(time.Duration(minute)*time.Minute), "heartbeat", "--json"), ExitOK)
	}

	fromB := e.signalsSince(1)
	if len(fromB) == 0 {
		t.Fatal("session B sent nothing")
	}
	for _, signal := range fromB {
		if slices.Contains(signal.CardIDs, ofA) {
			t.Fatalf("session B reported the card of session A: %+v", signal)
		}
		if signal.ClientID == fromA[0].ClientID {
			t.Fatalf("session B reported as the client of session A: %+v", signal)
		}
	}
}

// The safeguard beside it: every session reports as a client of its own. The
// server replaces the set of a client with every signal, so two sessions under
// one client would take each other's cards off the board.
func TestEverySessionReportsAsAClientOfItsOwn(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	ofA, ofB := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars[sessionVariable] = "session-a"
	expectCode(t, e.runAt(now, "work", "start", ofA, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "heartbeat", "--json"), ExitOK)
	e.vars[sessionVariable] = "session-b"
	expectCode(t, e.runAt(now, "work", "start", ofB, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "heartbeat", "--json"), ExitOK)

	if got := e.server.CardsAtWork(); len(got) != 2 {
		t.Errorf("the server holds %v, want the cards of both sessions", got)
	}
}

// A client of a session is the name of the installation and a digest of the
// session: the same with every signal, another for every session, the plain
// name for the list of no session, and never longer or otherwise lettered than
// the server takes.
func TestTheClientOfASessionIsMadeFromTheInstallationAndTheSession(t *testing.T) {
	const base = "0123456789abcdef0123456789abcdef"
	allowed := regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

	if got := clientIDOf(base, work.NoSession); got != base {
		t.Errorf("the list of no session reports as %q, want %q", got, base)
	}
	one := clientIDOf(base, "2cf0c501-2428-4e88-bb99-2aeb454d5618")
	if one != clientIDOf(base, "2cf0c501-2428-4e88-bb99-2aeb454d5618") {
		t.Error("the same session reports as two clients")
	}
	if one == clientIDOf(base, "another-session") || one == base {
		t.Error("two sessions report as one client")
	}
	if !strings.HasPrefix(one, base+".") || strings.Contains(one, "2cf0c501") {
		t.Errorf("the client %q does not hide the session behind the installation", one)
	}
	for _, name := range []string{base, strings.Repeat("x", 64)} {
		if got := clientIDOf(name, "a-session"); !allowed.MatchString(got) {
			t.Errorf("the client %q is not one the server takes", got)
		}
	}
}

// hookInput is what Claude Code gives a hook on standard input, naming a
// session.
func hookInput(session string) string {
	return `{"session_id": "` + session + `", "hook_event_name": "PostToolUse", "tool_response": {"stdout": "x"}}`
}

// beatOfTheHook runs a heartbeat the way the hook runs it, with the input of a
// hook on standard input, and decodes what it says.
func (e *testEnv) beatOfTheHook(at time.Time, input string) map[string]any {
	e.t.Helper()
	r := e.runWith(func(a *app) { a.now = func() time.Time { return at } }, input, "heartbeat", "--json")
	expectCode(e.t, r, ExitOK)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\n%s", err, r.stdout)
	}
	return decode(e.t, env.Data)
}

// The safeguard: a typed command and the hook after it speak for the same
// list. Where the environment names no session, a typed command cannot know the
// session the input of the hook names, so a hook that went by the input would
// report a list nobody writes into, and the card would never be reported.
func TestWithoutASessionInTheEnvironmentTheHookReportsWhatWasTyped(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID

	expectCode(t, e.runAt(now, "work", "start", card, "--json"), ExitOK)
	data := e.beatOfTheHook(now, hookInput("a-session-only-the-hook-knows"))
	if data["session"] != nil || data["sent"] != true {
		t.Fatalf("the beat spoke for %v and sent %v: %s", data["session"], data["sent"], data["reason"])
	}
	if ids := e.cardIDsOfTheSignal(); !slices.Equal(ids, []string{card}) {
		t.Errorf("the signal named %v, want %s", ids, card)
	}
}

// The safeguard beside it: where the environment and the input of the hook name
// different sessions, the hook goes by the environment, which is what the typed
// command went by.
func TestTheHookGoesByTheSessionOfTheEnvironment(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID

	e.vars[sessionVariable] = "the-session-of-the-environment"
	expectCode(t, e.runAt(now, "work", "start", card, "--json"), ExitOK)
	data := e.beatOfTheHook(now, hookInput("another-session"))
	if data["session"] != "the-session-of-the-environment" || data["sent"] != true {
		t.Fatalf("the beat spoke for %v and sent %v: %s", data["session"], data["sent"], data["reason"])
	}
	signal := e.signalsSince(0)
	if len(signal) != 1 || !slices.Equal(signal[0].CardIDs, []string{card}) || !strings.HasSuffix(signal[0].ClientID, clientIDOf("", "the-session-of-the-environment")) {
		t.Errorf("the signals %+v, want the card under the client of the environment's session", signal)
	}
}

// Inside Claude Code a card that lands in the list of no session is shared with
// every other session that names none, and work start says so.
func TestStartingWithoutASessionInsideClaudeCodeSaysSo(t *testing.T) {
	e := newAgentEnv(t)
	env, r := e.runJSON("", "work", "start", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitOK)
	if strings.Contains(env.Notice, "names no session") {
		t.Errorf("outside Claude Code the notice says %q", env.Notice)
	}

	e.vars[claudeCodeVariable] = "1"
	env, r = e.runJSON("", "work", "start", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "names no session") || !strings.Contains(env.Notice, sessionVariable) {
		t.Errorf("the notice %q does not say that no session was named", env.Notice)
	}

	e.vars[sessionVariable] = "a-session"
	env, _ = e.runJSON("", "work", "start", e.fixture.CardInDoing.ID)
	if strings.Contains(env.Notice, "names no session") {
		t.Errorf("with a session the notice says %q", env.Notice)
	}
}

// Whatever reports from inside a session is told its name: --session outweighs
// the environment, and one that cannot be a session is
// refused by a command and passed over by a beat, which exits 0 as always.
func TestTheSessionFlagNamesTheSession(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID
	e.vars[sessionVariable] = "the-environment"

	expectCode(t, e.runAt(now, "work", "start", card, "--session", "the-mod", "--json"), ExitOK)
	env, r := e.runWithJSON(now, "heartbeat", "--session", "the-mod")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["session"] != "the-mod" || data["sent"] != true {
		t.Fatalf("the beat spoke for %v and sent %v", data["session"], data["sent"])
	}
	listed, r := e.runWithJSON(now, "work", "list", "--session", "the-mod")
	expectCode(t, r, ExitOK)
	if ids := cardIDsOfTheList(t, listed); !slices.Equal(ids, []string{card}) {
		t.Errorf("the session holds %v, want %s", ids, card)
	}

	refused, r := e.runWithJSON(now, "work", "start", card, "--session", "not a session")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(refused.Error, "--session") {
		t.Errorf("the refusal %q does not name the flag", refused.Error)
	}
	before := len(e.signals())
	beat, r := e.runWithJSON(now.Add(time.Hour), "heartbeat", "--session", "not a session")
	expectCode(t, r, ExitOK)
	if sent, _ := decode(t, beat.Data)["sent"].(bool); sent || len(e.signals()) != before {
		t.Error("a beat reported under a name that is no session")
	}
}

// A command shows the list of its own session, and what the other sessions of
// the machine hold beside it, so that a card somebody else reports is not taken
// for one nobody holds.
func TestTheListShowsThisSessionAndTheOthersBesideIt(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	mine, theirs := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars[sessionVariable] = "another-session"
	expectCode(t, e.runAt(now, "work", "start", theirs, "--json"), ExitOK)
	e.vars[sessionVariable] = "this-session"
	expectCode(t, e.runAt(now, "work", "start", mine, "--json"), ExitOK)

	env, r := e.runWithJSON(now, "work", "list")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if ids := cardIDsOfTheList(t, env); !slices.Equal(ids, []string{mine}) || data["session"] != "this-session" {
		t.Errorf("the list of %v holds %v, want %s", data["session"], ids, mine)
	}
	elsewhere, _ := data["elsewhere"].([]any)
	if len(elsewhere) != 1 {
		t.Fatalf("elsewhere is %v, want the card of the other session", data["elsewhere"])
	}
	if row := elsewhere[0].(map[string]any); row["id"] != theirs || row["session"] != "another-session" {
		t.Errorf("elsewhere names %v", row)
	}

	// Stopping without a card is about this session alone, whatever else the
	// machine holds.
	expectCode(t, e.runAt(now, "work", "stop", "--json"), ExitOK)
	e.vars[sessionVariable] = "another-session"
	env, _ = e.runWithJSON(now, "work", "list")
	if ids := cardIDsOfTheList(t, env); !slices.Equal(ids, []string{theirs}) {
		t.Errorf("the other session holds %v, want %s", ids, theirs)
	}
}

// A card named that this session does not hold is stopped where it is held:
// whoever stops a card has let go of it, and a list of a dead session is
// nowhere else to be cleared.
func TestStoppingACardOfAnotherSessionTakesItOutOfThatList(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID

	e.vars[sessionVariable] = "the-one-that-died"
	expectCode(t, e.runAt(now, "work", "start", card, "--json"), ExitOK)
	e.vars[sessionVariable] = "the-one-that-cleans-up"
	env, r := e.runWithJSON(now, "work", "stop", card)
	expectCode(t, r, ExitOK)
	if stopped, _ := decode(t, env.Data)["stopped"].([]any); len(stopped) != 1 {
		t.Fatalf("stopped %v, want the card", decode(t, env.Data)["stopped"])
	}
	if elsewhere, _ := decode(t, env.Data)["elsewhere"].([]any); len(elsewhere) != 0 {
		t.Errorf("%v is still held elsewhere", elsewhere)
	}
}

// A card that is moved on leaves every list it is in, and the board is told at
// once only by the session that held it: reporting the list of another session
// from here would keep that session's other cards alight.
func TestMovingACardOfAnotherSessionDoesNotReportThatSession(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	moved, kept := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars[sessionVariable] = "the-one-that-holds-it"
	expectCode(t, e.runAt(now, "work", "start", moved, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "work", "start", kept, "--json"), ExitOK)

	e.vars[sessionVariable] = "the-one-that-moves-it"
	before := len(e.signals())
	env, r := e.runWithJSON(now, "cards", "move", moved, "--column", "not_now")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "next signal") {
		t.Errorf("the notice %q does not say the board hears of it later", env.Notice)
	}
	for _, signal := range e.signalsSince(before) {
		if slices.Contains(signal.CardIDs, kept) {
			t.Errorf("the move reported the other session's list: %+v", signal)
		}
	}
	e.vars[sessionVariable] = "the-one-that-holds-it"
	listed, _ := e.runWithJSON(now, "work", "list")
	if ids := cardIDsOfTheList(t, listed); !slices.Equal(ids, []string{kept}) {
		t.Errorf("the session holds %v, want only %s", ids, kept)
	}
}

// The keepalive reports every session by its own last beat: one that is still
// at work is reported, one that has said nothing for longer than the keepalive
// waits is not, however busy the session beside it is. Otherwise the keepalive
// would do what a shared list did — keep a dead session's cards alight on the
// busyness of another.
func TestTheKeepaliveReportsOnlyTheSessionsThatAreAtWork(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	ofTheDead, ofTheBusy := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars[sessionVariable] = "the-one-that-died"
	expectCode(t, e.runAt(began, "work", "start", ofTheDead, "--json"), ExitOK)
	e.vars[sessionVariable] = "the-busy-one"
	expectCode(t, e.runAt(began, "work", "start", ofTheBusy, "--json"), ExitOK)
	busy := began.Add(nobodyIsAtWorkAfter + time.Minute)
	expectCode(t, e.runAt(busy, "heartbeat", "--json"), ExitOK)

	ticked := busy.Add(2 * time.Minute)
	env, r := e.runKeepaliveThatIsToldToStop(ticked)
	expectCode(t, r, ExitOK)
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "told to stop") {
		t.Fatalf("the keepalive ended for %q", ended)
	}
	// A keepalive that is told to stop has its requests cut off, so what it
	// set out to send is what it claimed in the store before the request.
	if got := e.storedList()["sentAt"]; got != asStored(ticked) {
		t.Errorf("the busy session was last sent at %v, want the tick at %s", got, asStored(ticked))
	}
	e.vars[sessionVariable] = "the-one-that-died"
	if got := e.storedList()["sentAt"]; got != asStored(time.Time{}) {
		t.Errorf("the keepalive reported the session that died, at %v", got)
	}
}

// A store of one list for the machine, each card naming the session that took
// it up, is read without losing a card: every card lands in the list of its
// session with its sign of life, and the list of no session keeps what the
// machine was last sent, so that its client can take that back. A list goes by
// the moment its own cards were last seen, not by the last beat of the machine,
// so that a session that died long ago does not pass for one at work.
func TestAStoreOfTheMachineIsHandedOutToItsSessions(t *testing.T) {
	e := newAgentEnv(t)
	host := e.vars["MINKA_HOST"]
	stored := fmt.Sprintf(`{"hosts": {%q: {
		"cards": [
			{"id": "card-a", "title": "Of A", "startedAt": "2026-09-23T09:00:00Z", "session": "session-a", "seenAt": "2026-09-23T10:00:00Z"},
			{"id": "card-b", "title": "Of B", "startedAt": "2026-09-23T09:30:00Z", "session": "session-b"},
			{"id": "card-c", "title": "Of nobody", "startedAt": "2026-09-23T09:45:00Z"}
		],
		"letGo": [{"id": "card-d", "reason": "cardArchived", "at": "2026-09-23T09:50:00Z"}],
		"sentAt": "2026-09-23T10:00:00Z",
		"sentCards": ["card-a", "card-b", "card-c"],
		"beatAt": "2026-09-23T16:00:00Z"
	}}}`, host)
	if err := os.MkdirAll(dirOf(e.storePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.storePath(), []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}

	handle, err := work.Open(e.storePath(), waitForTheStore)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	sessions := handle.File.Hosts[host].Sessions
	for session, want := range map[string]struct {
		card string
		beat time.Time
	}{
		"session-a":    {"card-a", time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)},
		"session-b":    {"card-b", time.Date(2026, 9, 23, 9, 30, 0, 0, time.UTC)},
		work.NoSession: {"card-c", time.Date(2026, 9, 23, 9, 45, 0, 0, time.UTC)},
	} {
		list, held := sessions[session]
		if !held || !slices.Equal(list.CardIDs(), []string{want.card}) {
			t.Errorf("the session %q holds %+v, want %s", session, list, want.card)
			continue
		}
		if !list.BeatAt.Equal(want.beat) {
			t.Errorf("the session %q has the mark of a beat at %s, want %s", session, list.BeatAt, want.beat)
		}
	}
	if t.Failed() {
		return
	}
	if seen := sessions["session-a"].Cards[0].SeenAt; !seen.Equal(time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("the card of session A lost its sign of life: %s", seen)
	}
	machine := sessions[work.NoSession]
	if len(machine.SentCards) != 3 || len(machine.LetGo) != 1 {
		t.Errorf("the list of no session does not keep what the machine was sent: %+v", machine)
	}

	// Written again, it is the new shape alone.
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(e.storePath()); strings.Contains(string(content), `"session"`) {
		t.Errorf("the store still names a session per card:\n%s", content)
	}
}

func dirOf(path string) string {
	return path[:strings.LastIndex(path, string(os.PathSeparator))]
}

// The safeguard: a keepalive that starts after the server forgot a list — after
// a machine woke up, say — does not light it again while its session has said
// nothing since. Its last beat may be young enough to report on, but the
// session may be gone, and only a beat of its own says otherwise.
func TestAKeepaliveDoesNotRelightWhatTheServerForgot(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	e.vars[sessionVariable] = "the-one-that-went-quiet"
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.runAt(began, "heartbeat", "--json"), ExitOK)
	if got := e.storedList()["sentAt"]; got != asStored(began) {
		t.Fatalf("the beat claimed the signal of %v, want %s", got, asStored(began))
	}

	env, r := e.runKeepaliveThatIsToldToStop(began.Add(10 * time.Minute))
	expectCode(t, r, ExitOK)
	if got := e.storedList()["sentAt"]; got != asStored(began) {
		t.Errorf("the keepalive lit a list the server forgot, at %v", got)
	}
	if ended, _ := decode(t, env.Data)["ended"].(string); !strings.Contains(ended, "nothing but the keepalive") {
		t.Errorf("the keepalive ended for %q, want nobody at work", ended)
	}
}

// The safeguard: the beat of a session never carries the list of no session.
// That list belongs to whoever names no session, and a session that reported it
// would keep a person's card alight for as long as the session works on.
func TestABeatOfASessionDoesNotCarryTheListOfNoSession(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	ofNobody, ofTheSession := e.fixture.CardInTriage.ID, e.fixture.CardInDoing.ID

	expectCode(t, e.runAt(began, "work", "start", ofNobody, "--json"), ExitOK)
	e.vars[sessionVariable] = "a-session"
	expectCode(t, e.runAt(began, "work", "start", ofTheSession, "--json"), ExitOK)
	for minute := 0; minute <= 3*60; minute += 7 {
		expectCode(t, e.runAt(began.Add(time.Duration(minute)*time.Minute), "heartbeat", "--json"), ExitOK)
	}

	sent := e.signalsSince(0)
	if len(sent) == 0 {
		t.Fatal("the session sent nothing")
	}
	for _, signal := range sent {
		if slices.Contains(signal.CardIDs, ofNobody) {
			t.Fatalf("the session reported the card of no session: %+v", signal)
		}
	}
}
