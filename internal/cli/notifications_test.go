package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/newsmark"
	"github.com/flownative/minka-cli/internal/readmark"
)

const sessionOfTheWait = "0199aaaa-1111-7222-8333-444455556666"

// waitClock is the time of a wait under test: a held request that runs out
// moves it on by the wait it was given, and nothing really waits.
type waitClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *waitClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// waitsPassAtOnce makes every held request of the fake run out at once, the
// clock of the CLI moved on by its wait.
func (e *testEnv) waitsPassAtOnce() *waitClock {
	clock := &waitClock{now: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)}
	e.server.WaitsOut = func(wait time.Duration) <-chan time.Time {
		clock.mu.Lock()
		clock.now = clock.now.Add(wait)
		now := clock.now
		clock.mu.Unlock()
		passed := make(chan time.Time, 1)
		passed <- now
		return passed
	}
	return clock
}

func (e *testEnv) runWaitJSON(clock *waitClock, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.runWith(func(a *app) {
		if clock != nil {
			a.now = clock.Now
		}
	}, "", append([]string{"notifications", "wait", "--json"}, args...)...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

// aMove is the line Bruno's bell gets when Anke moves a card of his into
// Doing, with the summary the fake answers for it.
func (e *testEnv) aMove(card *fakeapi.Card) *fakeapi.Notification {
	name := card.ShortID
	if name == "" {
		name = "“" + card.Title + "”"
	}
	return &fakeapi.Notification{
		PersonID: e.fixture.Bruno.ID, Occasion: "card.moved", ActorID: e.fixture.Anke.ID, CardID: card.ID,
		MovedToColumnID: e.fixture.Doing.ID, Summary: "Anke moved " + name + " to “Doing”",
	}
}

// aLineArrivesInTheWait lets a line arrive while the first request of a
// wait is held.
func (e *testEnv) aLineArrivesInTheWait(lines ...*fakeapi.Notification) {
	var once sync.Once
	e.server.WhileWaiting = func(*fakeapi.Person) {
		once.Do(func() {
			for _, line := range lines {
				e.server.Notify(line)
			}
		})
	}
}

// waitRequests are the asks of the list of notifications, by their query.
func (e *testEnv) waitRequests() []url.Values {
	var asked []url.Values
	for _, r := range e.server.Requests() {
		if r.Path == "/api/v1/me/notifications" {
			query, _ := url.ParseQuery(r.Query)
			asked = append(asked, query)
		}
	}
	return asked
}

func (e *testEnv) markOf(session string) (string, bool) {
	e.t.Helper()
	// The zero moment forgets no mark, however old.
	position, err := newsmark.Read(newsmark.Path(e.vars["MINKA_CONFIG"]), e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), session, time.Time{})
	if err != nil {
		e.t.Fatal(err)
	}
	return position.Cursor, position.Found
}

func linesOf(t *testing.T, env envelope) []map[string]any {
	t.Helper()
	var lines []map[string]any
	if err := json.Unmarshal(env.Data, &lines); err != nil {
		t.Fatalf("data is not a list: %v: %s", err, env.Data)
	}
	return lines
}

// The first wait of a session starts from now: it asks where the bell stands
// without a position, keeps that, and waits from there. A line that arrives
// in the wait is printed as Minka sent it, and the mark stands behind it.
func TestTheFirstWaitStartsFromNowAndPrintsWhatArrives(t *testing.T) {
	e := newTestEnv(t)
	e.vars[sessionVariable] = sessionOfTheWait
	e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.written", Summary: "from before"})
	arriving := e.aMove(e.fixture.CardInDoing)
	e.aLineArrivesInTheWait(arriving)

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	lines := linesOf(t, env)
	if len(lines) != 1 || lines[0]["summary"] != arriving.Summary || lines[0]["cursor"] != fakeapi.CursorOf(arriving) {
		t.Fatalf("lines %v", lines)
	}
	if card, _ := lines[0]["card"].(map[string]any); card["boardId"] != e.fixture.Board.ID {
		t.Fatalf("the line lost what Minka sent: %v", lines[0])
	}

	asked := e.waitRequests()
	if len(asked) != 2 || asked[0].Has("after") || asked[0].Get("wait") != "0" {
		t.Fatalf("the wait did not start by asking where the bell stands: %v", asked)
	}
	if asked[1].Get("after") == "" || asked[1].Get("wait") != "50" || asked[1].Get("limit") != "200" {
		t.Fatalf("the wait asked %v", asked[1])
	}
	if mark, _ := e.markOf(sessionOfTheWait); mark != fakeapi.CursorOf(arriving) {
		t.Fatalf("mark %q, want %q", mark, fakeapi.CursorOf(arriving))
	}
}

// The next wait goes on from the mark, so a line that came in between two
// waits is printed at once and nothing is missed.
func TestTheNextWaitGoesOnFromWhereTheLastOneStopped(t *testing.T) {
	e := newTestEnv(t)
	e.vars[sessionVariable] = sessionOfTheWait
	first := e.aMove(e.fixture.CardInDoing)
	e.aLineArrivesInTheWait(first)
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)

	e.server.WhileWaiting = nil
	between := e.server.Notify(e.aMove(e.fixture.CardInTriage))
	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != between.ID {
		t.Fatalf("lines %v, want the one that came in between", lines)
	}
	if asked := e.waitRequests(); asked[len(asked)-1].Get("after") != fakeapi.CursorOf(first) {
		t.Fatalf("the wait asked from %q, not from the mark", asked[len(asked)-1].Get("after"))
	}
}

// The text view says one line per notification: what it is about, then what
// happened and who did it.
func TestAWaitPrintsOneLinePerNotification(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-B4"
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing), e.aMove(e.fixture.CardInTriage))

	r := e.run("", "notifications", "wait", "--styled")
	expectCode(t, r, ExitOK)
	want := "MNK-B4 Review the API: Anke moved MNK-B4 to “Doing”\nWrite the CLI: Anke moved “Write the CLI” to “Doing”\n"
	if r.stdout != want {
		t.Fatalf("stdout %q, want %q", r.stdout, want)
	}
}

// The safeguard: the mark moves on only after the lines are out. Where it
// cannot be kept, the lines are printed all the same, a warning says so, and
// the next wait prints them again rather than losing them.
func TestTheLinesArePrintedBeforeTheMarkMovesOn(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	before, _ := e.markOf("")

	lock := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "notifications-read.lock")
	if err := os.RemoveAll(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	e.server.WhileWaiting = nil
	line := e.server.Notify(e.aMove(e.fixture.CardInTriage))

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != line.ID {
		t.Fatalf("lines %v", lines)
	}
	if !strings.Contains(r.stderr, "shows these lines again") {
		t.Fatalf("stderr %q does not say that the mark was not kept", r.stderr)
	}
	if mark, _ := e.markOf(""); mark != before {
		t.Fatalf("mark %q, want it unmoved at %q", mark, before)
	}

	if err := os.RemoveAll(lock); err != nil {
		t.Fatal(err)
	}
	env, r = e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != line.ID {
		t.Fatalf("the next wait did not print the line again: %v", lines)
	}
}

// Every session has a mark of its own: --session, else the variable of Claude
// Code, else the one mark of no session. And every account has its own.
func TestEverySessionAndAccountWaitsFromAMarkOfItsOwn(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--session", "planning", "--json"), ExitOK)

	e.vars[sessionVariable] = sessionOfTheWait
	e.server.WhileWaiting = nil
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInTriage))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)

	planning, _ := e.markOf("planning")
	ofTheVariable, _ := e.markOf(sessionOfTheWait)
	if planning == "" || ofTheVariable == "" || planning == ofTheVariable {
		t.Fatalf("marks %q and %q, want one per session", planning, ofTheVariable)
	}
	if _, found := e.markOf(""); found {
		t.Fatal("a wait of a session moved the mark of no session")
	}

	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
	e.server.WhileWaiting = nil
	e.aLineArrivesInTheWait(&fakeapi.Notification{PersonID: e.fixture.Agent.ID, Occasion: "comment.written", Summary: "for the agent"})
	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["summary"] != "for the agent" {
		t.Fatalf("the agent was shown %v", lines)
	}
	if mark, _ := e.markOf(sessionOfTheWait); mark == ofTheVariable {
		t.Fatal("the agent's wait did not keep a mark of its own")
	}
}

// --board and --column keep the lines about cards there, by the column a card
// stands in now; a column is named by its ID or its title in any case. A line
// they leave out is read all the same, and one about no card is left out
// under either.
func TestFiltersKeepTheLinesOfABoardOrColumnAndReadTheRest(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()
	expectCode(t, e.runWith(func(a *app) { a.now = clock.Now }, "", "notifications", "wait", "--timeout", "1s", "--json"), ExitTimeout)

	inTriage := e.server.Notify(e.aMove(e.fixture.CardInTriage))
	e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "person.mentionedPrivately", Summary: "about no card"})
	inDoing := e.server.Notify(e.aMove(e.fixture.CardInDoing))

	env, r := e.runWaitJSON(clock, "--board", up(e.fixture.Board.ID), "--column", "doing", "--timeout", "1h")
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != inDoing.ID {
		t.Fatalf("lines %v, want the one in Doing", lines)
	}
	if !strings.Contains(env.Notice, "2 notifications left out") {
		t.Fatalf("notice %q", env.Notice)
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(inDoing) {
		t.Fatalf("mark %q, want it behind every line read", mark)
	}

	later := e.server.Notify(e.aMove(e.fixture.CardInTriage))
	env, r = e.runWaitJSON(clock, "--column", e.fixture.Triage.ID)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != later.ID {
		t.Fatalf("lines %v, want only the line after the last wait, not %s again", lines, inTriage.ID)
	}
}

// A filter that lets nothing through reads what it left out and goes on
// waiting; the next wait does not go through those lines again.
func TestAWaitWhoseFilterLetsNothingThroughGoesOnWaiting(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()
	left := e.aMove(e.fixture.CardInDoing)
	e.aLineArrivesInTheWait(left)

	env, r := e.runWaitJSON(clock, "--board", e.fixture.OtherBoard.ID, "--timeout", "10m")
	expectCode(t, r, ExitTimeout)
	if env.Code != CodeTimeout || !strings.Contains(env.Notice, "1 notification left out") {
		t.Fatalf("code %q, notice %q", env.Code, env.Notice)
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(left) {
		t.Fatalf("mark %q, want it behind the line left out", mark)
	}
}

// --timeout waits in asks of at most 50 seconds, as long as Minka holds one,
// the last one for what is left, and ends with exit 12 when nothing arrived.
// Without it every ask waits the longest Minka holds one.
func TestATimeoutWaitsInAsksOfAtMostFiftySeconds(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()

	env, r := e.runWaitJSON(clock, "--timeout", "2m")
	expectCode(t, r, ExitTimeout)
	if env.OK || env.Code != CodeTimeout || !env.Retryable || !strings.Contains(env.Error, "2m0s") {
		t.Fatalf("envelope %+v", env)
	}
	var waits []string
	for _, asked := range e.waitRequests()[1:] {
		waits = append(waits, asked.Get("wait"))
	}
	if got := strings.Join(waits, ","); got != "50,50,20" {
		t.Fatalf("waits %s, want 50,50,20", got)
	}

	for _, timeout := range []string{"0s", "-1m"} {
		_, r = e.runWaitJSON(clock, "--timeout", timeout)
		expectCode(t, r, ExitUsage)
	}
}

// Where Minka no longer knows the mark, the wait starts from now, says so,
// keeps the new position and goes on waiting.
func TestAMarkMinkaForgotStartsTheWaitFromNow(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	forgotten, _ := e.markOf("")
	e.server.ForgottenCursors = []string{forgotten}

	e.server.WhileWaiting = nil
	e.server.Notify(e.aMove(e.fixture.CardInTriage))
	arriving := e.aMove(e.fixture.CardInDoing)
	e.aLineArrivesInTheWait(arriving)

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["id"] != arriving.ID {
		t.Fatalf("lines %v, want only the one that arrived after starting anew", lines)
	}
	if !strings.Contains(env.Notice, "started from now") {
		t.Fatalf("notice %q does not say that the wait started anew", env.Notice)
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(arriving) {
		t.Fatalf("mark %q", mark)
	}
}

// More lines than a page holds are all read before the wait ends, without
// waiting between the pages.
func TestAWaitReadsEveryPageThatPiledUp(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)

	e.server.WhileWaiting = nil
	var last *fakeapi.Notification
	for i := range 250 {
		last = e.server.Notify(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.written", Summary: fmt.Sprintf("line %d", i)})
	}
	before := len(e.waitRequests())
	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 250 || lines[249]["id"] != last.ID {
		t.Fatalf("%d lines, want 250", len(lines))
	}
	asked := e.waitRequests()[before:]
	if len(asked) != 2 || asked[1].Get("wait") != "0" {
		t.Fatalf("asked %v, want a second page without a wait", asked)
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(last) {
		t.Fatalf("mark %q, want behind the last line", mark)
	}
}

// A failed ask ends the wait as a read does, retryable, and the mark stays
// where it was.
func TestAFailedAskLeavesTheMarkWhereItWas(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	before, _ := e.markOf("")

	e.server.WhileWaiting = nil
	e.server.FailNextWith(502)
	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitAPI)
	if !env.Retryable {
		t.Fatal("a failed wait is not retryable")
	}
	if mark, _ := e.markOf(""); mark != before {
		t.Fatalf("mark %q, want %q", mark, before)
	}
}

// A session named with --session that cannot be one is refused before
// anything is asked.
func TestAWaitRefusesASessionThatCannotBeOne(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runWaitJSON(nil, "--session", "not a session")
	expectCode(t, r, ExitUsage)
	if asked := e.waitRequests(); len(asked) != 0 {
		t.Fatalf("asked %v", asked)
	}
}

// Moving a card tells its assignees but the mover, with a line that names
// the card by its short identifier, else its title in quotes, and the column
// it stands in now.
func TestAMovedCardTellsItsAssigneesButTheMover(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInTriage.AssigneeIDs = []string{e.fixture.Bruno.ID, e.fixture.Anke.ID}
	expectCode(t, e.run("", "cards", "move", e.fixture.CardInTriage.ID, "--column", e.fixture.Doing.ID, "--json"), ExitOK)

	if len(e.server.Notifications) != 1 {
		t.Fatalf("%d lines, want one for Anke", len(e.server.Notifications))
	}
	line := e.server.Notifications[0]
	if line.PersonID != e.fixture.Anke.ID || line.Occasion != "card.moved" || line.Summary != "Bruno moved “Write the CLI” to “Doing”" {
		t.Fatalf("line %+v", line)
	}

	// A move into Done is told as the completion alone, to the subscribers.
	e.fixture.CardInTriage.SubscriberIDs = []string{e.fixture.Bruno.ID, e.fixture.Theo.ID}
	expectCode(t, e.run("", "cards", "move", e.fixture.CardInTriage.ID, "--column", e.fixture.Done.ID, "--json"), ExitOK)
	if len(e.server.Notifications) != 2 {
		t.Fatalf("%d lines, want one more for Theo", len(e.server.Notifications))
	}
	if done := e.server.Notifications[1]; done.PersonID != e.fixture.Theo.ID || done.Occasion != "card.completed" || done.Summary != "Completed: Write the CLI" {
		t.Fatalf("line %+v", done)
	}
}
