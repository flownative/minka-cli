package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// waitDeadline ends every run of a notifications command in a test, so that a
// wait whose guard broke fails the test instead of hanging it.
const waitDeadline = 5 * time.Second

// runPausingJSON runs a wait whose pauses move the clock instead of waiting,
// and records them.
func (e *testEnv) runPausingJSON(clock *waitClock, pauses *[]time.Duration, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.runWith(func(a *app) {
		a.now = clock.Now
		a.pause = func(_ context.Context, d time.Duration) error {
			clock.mu.Lock()
			clock.now = clock.now.Add(d)
			clock.mu.Unlock()
			*pauses = append(*pauses, d)
			return nil
		}
	}, "", append([]string{"notifications", "wait", "--json"}, args...)...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

// The safeguard: an ask that comes back empty sooner than it asked to be held
// is followed by a pause that grows, so a server or a proxy that does not hold
// a request is not asked again at once for ever.
func TestAnAskThatComesBackEarlyAndEmptyIsFollowedByAPause(t *testing.T) {
	e := newTestEnv(t)
	clock := &waitClock{now: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)}
	e.server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- clock.Now()
		return passed
	}

	var pauses []time.Duration
	_, r := e.runPausingJSON(clock, &pauses, "--timeout", "3m")
	expectCode(t, r, ExitTimeout)
	if len(pauses) < 3 {
		t.Fatalf("pauses %v, want one after every early answer", pauses)
	}
	if pauses[0] < earlyAnswerPause || pauses[0] > earlyAnswerPause*5/4 {
		t.Fatalf("the first pause is %s, want %s and a quarter at most", pauses[0], earlyAnswerPause)
	}
	if pauses[1] < 2*earlyAnswerPause || pauses[2] < 4*earlyAnswerPause {
		t.Fatalf("pauses %v do not grow", pauses)
	}
	for _, pause := range pauses {
		if pause > earlyAnswerPauseMax*5/4 {
			t.Fatalf("pause %s is longer than the most", pause)
		}
	}
	if asked := len(e.waitRequests()); asked > 12 {
		t.Fatalf("%d asks in three minutes of early answers", asked)
	}
}

// The safeguard: a position Minka refuses right after handing it out ends the
// wait as an error of the server instead of starting anew for ever.
func TestAPositionRefusedRightAfterItWasHandedOutEndsTheWait(t *testing.T) {
	e := newTestEnv(t)
	e.server.ForgottenCursors = []string{"0"}

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || env.Retryable || env.Meta["apiCode"] != "cursorUnknown" {
		t.Fatalf("envelope %+v", env)
	}
	if asked := e.waitRequests(); len(asked) != 2 {
		t.Fatalf("%d asks, want the start and one ask", len(asked))
	}
}

// bell is a server that answers the list of notifications from a script, one
// answer per ask, and a board from a fixed answer: it gives the odd answers
// the fake never gives.
type bell struct {
	mu      sync.Mutex
	answers []string // a status, a space, a body
	asked   []string // the query of every ask
	board   string
}

func (e *testEnv) answeredBy(b *bell) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/boards/") {
			_, _ = w.Write([]byte(b.board))
			return
		}
		b.asked = append(b.asked, r.URL.RawQuery)
		if len(b.answers) == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"scriptEnded","message":"no answer left"}}`))
			return
		}
		status, body, _ := strings.Cut(b.answers[0], " ")
		b.answers = b.answers[1:]
		var code int
		_, _ = fmt.Sscan(status, &code)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	e.t.Cleanup(server.Close)
	e.vars["MINKA_HOST"] = server.URL
}

func (e *testEnv) keepMark(session, cursor string, at time.Time) {
	e.t.Helper()
	path := newsmark.Path(e.vars["MINKA_CONFIG"])
	if _, err := newsmark.MoveOn(e.t.Context(), path, e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), session, "", cursor, at); err != nil {
		e.t.Fatal(err)
	}
}

func page(cursor string, lineCursors ...string) string {
	lines := make([]string, len(lineCursors))
	for i, c := range lineCursors {
		lines[i] = fmt.Sprintf(`{"id":"n%d","cursor":%q,"occasion":"comment.created","summary":"line %d","subject":{"type":"card","id":"x","title":"T"}}`, i, c, i)
	}
	return fmt.Sprintf(`200 {"notifications":[%s],"cursor":%q}`, strings.Join(lines, ","), cursor)
}

const unknown = `422 {"error":{"code":"cursorUnknown","message":"unknown"}}`

// Where Minka forgets the position more than once in a run, the notice says
// so once.
func TestANoticeIsGivenOnceHoweverOftenItHappens(t *testing.T) {
	e := newTestEnv(t)
	b := &bell{answers: []string{unknown, page("p1"), page("p1"), unknown, page("p2"), page("l1", "l1")}}
	e.answeredBy(b)
	e.keepMark("", "stale", time.Now())

	clock := &waitClock{now: time.Now()}
	var pauses []time.Duration
	env, r := e.runPausingJSON(clock, &pauses)
	expectCode(t, r, ExitOK)
	if got := strings.Count(env.Notice, "no longer knows"); got != 1 {
		t.Fatalf("notice %q says it %d times", env.Notice, got)
	}
}

// Odd answers end the wait as an error of the server, before anything is
// printed that would be printed again in every run after it; an answer that
// names no position after its lines goes on from its last line.
func TestOddAnswersOfTheListAreRefusedOrMadeSenseOf(t *testing.T) {
	full := make([]string, 200)
	for i := range full {
		full[i] = fmt.Sprintf("f%d", i)
	}
	for _, c := range []struct {
		name    string
		answers []string
		code    int
		mark    string
	}{
		{"a start without a position", []string{page("")}, ExitAPI, ""},
		{"an empty answer without a position", []string{page("p"), page("")}, ExitAPI, "p"},
		{"lines that do not lead past the position", []string{page("p"), page("p", "p")}, ExitAPI, "p"},
		{"a full page leading back", []string{page("p"), page("f", full...), page("g", full...), page("f", full...)}, ExitAPI, "p"},
		{"lines without a position behind them", []string{page("p"), page("", "l1", "l2")}, ExitOK, "l2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			e.answeredBy(&bell{answers: c.answers})
			env, r := e.runWaitJSON(nil)
			expectCode(t, r, c.code)
			if c.code != ExitOK && (env.Code != CodeAPI || env.Retryable || len(env.Data) > 0 && string(env.Data) != "null") {
				t.Fatalf("envelope %+v", env)
			}
			if mark, _ := e.markOf(""); mark != c.mark {
				t.Fatalf("mark %q, want %q", mark, c.mark)
			}
		})
	}
}

// The last ask of a timeout is rounded up to a whole second, so that what is
// left is asked for and not a wait of nothing over and over.
func TestTheLastAskOfATimeoutIsRoundedUp(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()
	_, r := e.runWaitJSON(clock, "--timeout", "110500ms")
	expectCode(t, r, ExitTimeout)
	var waits []string
	for _, asked := range e.waitRequests()[1:] {
		waits = append(waits, asked.Get("wait"))
	}
	if got := strings.Join(waits, ","); got != "50,50,11" {
		t.Fatalf("waits %s, want 50,50,11", got)
	}
}

// A mark older than the days it is kept is no position: the wait starts from
// now and says so.
func TestAMarkKeptTooLongStartsTheWaitFromNow(t *testing.T) {
	e := newTestEnv(t)
	e.keepMark("", "0", time.Now().Add(-newsmark.ForgottenAfter-time.Hour))
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "forgotten") {
		t.Fatalf("notice %q", env.Notice)
	}
	if asked := e.waitRequests(); asked[0].Has("after") {
		t.Fatalf("the wait went on from the old mark: %v", asked[0])
	}
}

// A file of positions that cannot be read costs the positions, not the wait:
// it starts from now, names the file and writes it anew.
func TestAnUnreadableFileOfPositionsIsWrittenAnew(t *testing.T) {
	e := newTestEnv(t)
	path := newsmark.Path(e.vars["MINKA_CONFIG"])
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	line := e.aMove(e.fixture.CardInDoing)
	e.aLineArrivesInTheWait(line)

	env, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, path) {
		t.Fatalf("notice %q does not name the file", env.Notice)
	}
	if mark, _ := e.markOf(""); mark != fakeapi.CursorOf(line) {
		t.Fatalf("mark %q", mark)
	}
}

// The wait the breadcrumb offers has the session and the filters of this one.
func TestTheNextWaitOfferedKeepsTheSessionAndFilters(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))

	env, r := e.runWaitJSON(nil, "--board", up(e.fixture.Board.ID), "--column", "doing", "--column", "In Arbeit", "--session", "planning", "--timeout", "1h")
	expectCode(t, r, ExitOK)
	want := "minka notifications wait --board " + e.fixture.Board.ID + " --column doing --column 'In Arbeit' --session planning --timeout 1h0m0s --json"
	if len(env.Breadcrumbs) == 0 || env.Breadcrumbs[0].Cmd != want {
		t.Fatalf("breadcrumbs %+v, want %q", env.Breadcrumbs, want)
	}
}

// One notification is one line of text, whatever its words hold.
func TestANotificationIsOneLineOfText(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(&fakeapi.Notification{PersonID: e.fixture.Bruno.ID, Occasion: "comment.created", CardID: e.fixture.CardInDoing.ID, Summary: "Anke wrote:\nfirst\r\nsecond"})

	r := e.run("", "notifications", "wait", "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != "Review the API: Anke wrote: first second\n" {
		t.Fatalf("stdout %q", r.stdout)
	}
}

// A board whose coordinator is null shows nobody, as null and without a
// notice.
func TestABoardWithANullCoordinatorShowsNobody(t *testing.T) {
	e := newTestEnv(t)
	e.answeredBy(&bell{board: `{"id":"0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6","projectId":"p","title":"B","columns":[],"coordinator":null}`})
	env, r := e.runJSON("", "boards", "coordinator", "show", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6")
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" && len(env.Data) != 0 {
		t.Fatalf("data %s, want null", env.Data)
	}
	if !strings.Contains(env.Summary, "Nobody") || env.Notice != "" {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}
}

// The safeguard: a wait moves the mark on only from the position it read.
// Where another wait of the session moved it meanwhile, that one's mark stays,
// and this run moves it no further.
func TestAMarkMovedByAnotherWaitIsLeftToIt(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	read, _ := e.markOf("")

	e.server.WhileWaiting = func(*fakeapi.Person) {
		path := newsmark.Path(e.vars["MINKA_CONFIG"])
		if _, err := newsmark.MoveOn(context.Background(), path, e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), "", read, "moved-by-another", time.Now()); err != nil {
			t.Error(err)
		}
		e.server.Notify(e.aMove(e.fixture.CardInTriage))
	}
	_, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if mark, _ := e.markOf(""); mark != "moved-by-another" {
		t.Fatalf("mark %q, want the one the other wait kept", mark)
	}
}

// The safeguard: a wait that read a mark nearly as old as it is kept moves it
// on after waiting for days. Forgetting it on the way would print the lines
// and start the next wait from now, past them.
func TestAWaitOfDaysKeepsTheMarkItStartedFrom(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()
	e.keepMark("", "0", clock.Now().Add(-newsmark.ForgottenAfter+time.Hour))
	asks := 0
	var line *fakeapi.Notification
	e.server.WhileWaiting = func(*fakeapi.Person) {
		asks++
		if asks == 1800 {
			line = e.server.Notify(e.aMove(e.fixture.CardInDoing))
		}
	}

	_, r := e.runWaitJSON(clock)
	expectCode(t, r, ExitOK)
	if mark, _ := e.markOf(""); line == nil || mark != fakeapi.CursorOf(line) {
		t.Fatalf("mark %q after a wait of a day, want it behind the line", mark)
	}
	if strings.Contains(r.stderr, "gone") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// A mark that another wait took out of the file by writing it anew is
// written again by the wait that stood at it.
func TestAMarkTakenOutByARewrittenFileIsWrittenAgain(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)

	path := newsmark.Path(e.vars["MINKA_CONFIG"])
	var line *fakeapi.Notification
	e.server.WhileWaiting = func(*fakeapi.Person) {
		if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
			t.Error(err)
		}
		if _, err := newsmark.MoveOn(context.Background(), path, e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), "other", "", "x", time.Now()); err != nil {
			t.Error(err)
		}
		line = e.server.Notify(e.aMove(e.fixture.CardInTriage))
	}
	_, r := e.runWaitJSON(nil)
	expectCode(t, r, ExitOK)
	if mark, _ := e.markOf(""); line == nil || mark != fakeapi.CursorOf(line) {
		t.Fatalf("mark %q, want it behind the line printed", mark)
	}
}

// The safeguard: the write of another session prunes marks older than they
// are kept, the one a running wait stands at included. That wait writes its
// mark again, so its next wait goes on behind the lines it printed.
func TestAMarkPrunedUnderARunningWaitIsWrittenAgain(t *testing.T) {
	e := newTestEnv(t)
	clock := e.waitsPassAtOnce()
	e.keepMark("", "0", clock.Now().Add(-newsmark.ForgottenAfter+time.Hour))
	path := newsmark.Path(e.vars["MINKA_CONFIG"])
	asks := 0
	var line *fakeapi.Notification
	e.server.WhileWaiting = func(*fakeapi.Person) {
		asks++
		if asks == 2400 {
			if _, err := newsmark.MoveOn(context.Background(), path, e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), "b", "", "x", clock.Now()); err != nil {
				t.Error(err)
			}
			if _, found := e.markOf(""); found {
				t.Error("the write of b did not prune the mark; the test proves nothing")
			}
			line = e.server.Notify(e.aMove(e.fixture.CardInDoing))
		}
	}

	_, r := e.runWaitJSON(clock)
	expectCode(t, r, ExitOK)
	if mark, _ := e.markOf(""); line == nil || mark != fakeapi.CursorOf(line) {
		t.Fatalf("mark %q, want it behind the line printed", mark)
	}
}

// A pause never reaches past the end of --timeout.
func TestAPauseEndsWithTheTimeout(t *testing.T) {
	e := newTestEnv(t)
	clock := &waitClock{now: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)}
	e.server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- clock.Now()
		return passed
	}
	var pauses []time.Duration
	_, r := e.runPausingJSON(clock, &pauses, "--timeout", "7s")
	expectCode(t, r, ExitTimeout)
	var all time.Duration
	for _, pause := range pauses {
		all += pause
	}
	if len(pauses) != 2 || all != 7*time.Second {
		t.Fatalf("pauses %v, want two that end with the timeout", pauses)
	}
}

// The safeguard: a run that lost the mark to another wait moves it no
// further, even where it later reads on from the very position the other
// one wrote.
func TestARunThatLostTheMarkMovesItNoFurther(t *testing.T) {
	e := newTestEnv(t)
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing))
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)
	read, _ := e.markOf("")

	path := newsmark.Path(e.vars["MINKA_CONFIG"])
	var other string
	asks := 0
	e.server.WhileWaiting = func(*fakeapi.Person) {
		asks++
		switch asks {
		case 1:
			left := e.server.Notify(e.aMove(e.fixture.CardInDoing))
			other = fakeapi.CursorOf(left)
			if _, err := newsmark.MoveOn(context.Background(), path, e.vars["MINKA_HOST"], readmark.Reader(e.vars["MINKA_TOKEN"]), "", read, other, time.Now()); err != nil {
				t.Error(err)
			}
		case 2:
			e.server.Notify(e.aMove(e.fixture.CardInTriage))
		}
	}
	_, r := e.runWaitJSON(nil, "--column", "Triage")
	expectCode(t, r, ExitOK)
	if mark, _ := e.markOf(""); mark != other {
		t.Fatalf("mark %q, want the other wait's %q", mark, other)
	}
}

// A pause starts anew after an answer that brought lines, even lines the
// filters left out.
func TestAPauseStartsAnewAfterLines(t *testing.T) {
	e := newTestEnv(t)
	kept := `200 {"notifications":[{"id":"k","cursor":"k","occasion":"card.moved","summary":"kept","subject":{"type":"card","id":"x","title":"T"},"card":{"boardId":"0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6","column":{"id":"c","title":"Doing"}}}],"cursor":"k"}`
	e.answeredBy(&bell{answers: []string{page("p"), page("p"), page("p"), page("l", "l"), page("l"), kept}})
	clock := &waitClock{now: time.Now()}
	var pauses []time.Duration
	_, r := e.runPausingJSON(clock, &pauses, "--board", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6")
	expectCode(t, r, ExitOK)
	if len(pauses) != 3 || pauses[1] < 2*earlyAnswerPause || pauses[2] > earlyAnswerPause*5/4 {
		t.Fatalf("pauses %v, want the third to start anew", pauses)
	}
}

// An ask of a single second that comes back at once is early too: without a
// pause the last ask of a timeout would be repeated at once for ever.
func TestAnAskOfOneSecondThatComesBackAtOnceIsEarly(t *testing.T) {
	e := newTestEnv(t)
	clock := &waitClock{now: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)}
	e.server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- clock.Now()
		return passed
	}
	var pauses []time.Duration
	_, r := e.runPausingJSON(clock, &pauses, "--timeout", "1s")
	expectCode(t, r, ExitTimeout)
	if len(pauses) != 1 || pauses[0] != time.Second {
		t.Fatalf("pauses %v, want one of the second that is left", pauses)
	}
}

// The safeguard: an installation that holds a request for less than it was
// asked holds all the same. An empty answer after it is no early one, and no
// pause follows it.
func TestAnAskHeldShorterThanAskedIsNotEarly(t *testing.T) {
	e := newTestEnv(t)
	e.server.HoldsAtMost = 20 * time.Second
	clock := e.waitsPassAtOnce()
	var pauses []time.Duration
	_, r := e.runPausingJSON(clock, &pauses, "--timeout", "2m")
	expectCode(t, r, ExitTimeout)
	if len(pauses) != 0 {
		t.Fatalf("pauses %v after answers that were held", pauses)
	}
	if asked := len(e.waitRequests()) - 1; asked != 6 {
		t.Fatalf("%d asks, want one per 20 seconds held", asked)
	}
}
