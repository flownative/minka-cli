package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// coordinatorRequests are the writes on the coordinator of a board.
func (e *testEnv) coordinatorRequests() []fakeapi.RecordedRequest {
	var found []fakeapi.RecordedRequest
	for _, r := range e.server.WritingRequests() {
		if strings.HasSuffix(r.Path, "/coordinator") {
			found = append(found, r)
		}
	}
	return found
}

// A claim sends the session named with --session as written, and the board
// names it to its account from then on: in data, in coordinator show and in
// boards show.
func TestAClaimMakesTheSessionTheCoordinatorOfTheBoard(t *testing.T) {
	e := newAgentEnv(t)
	e.vars[sessionVariable] = "0199aaaa-0000-7000-8000-000000000000"

	env, r := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", sessionOfTheWait)
	expectCode(t, r, ExitOK)
	claimed := decode(t, env.Data)
	if claimed["session"] != sessionOfTheWait || claimed["person"].(map[string]any)["kind"] != "agent" {
		t.Fatalf("data %s", env.Data)
	}
	sent := e.coordinatorRequests()
	if len(sent) != 1 || sent[0].Method != http.MethodPut || string(sent[0].Body) != `{"session":"`+sessionOfTheWait+`"}` {
		t.Fatalf("sent %v", sent)
	}

	env, r = e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["session"] != sessionOfTheWait {
		t.Fatalf("show: %s", env.Data)
	}

	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Coordinated by Claude (Bruno) (agent, session "+sessionOfTheWait+") since ") {
		t.Fatalf("boards show does not name the coordinator:\n%s", r.stdout)
	}
}

// The safeguard: a claim takes the session from --session alone, a name
// somebody chose to be read on the board. The variable of Claude Code is never
// taken for it, and without the flag, or with a name a board cannot keep,
// nothing is sent.
func TestAClaimNeedsASessionTheBoardCanKeep(t *testing.T) {
	e := newTestEnv(t)
	e.vars[sessionVariable] = sessionOfTheWait
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "planning", "--json"), ExitOK)
	if body := string(e.coordinatorRequests()[0].Body); body != `{"session":"planning"}` {
		t.Fatalf("sent %s", body)
	}

	for _, args := range [][]string{
		{"boards", "coordinator", "claim", e.fixture.Board.ID},
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", strings.Repeat("a", 65)},
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", strings.Repeat("ä", 65)},
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "a\tb"},
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "a\u202eb"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if sent := e.coordinatorRequests(); len(sent) != 1 {
		t.Fatalf("%d claims went out, want only the first", len(sent))
	}
}

// A session is named as Minka takes a name: blanks and umlauts are part of it,
// at either end as well, and it is sent as written.
func TestASessionIsNamedAsMinkaTakesAName(t *testing.T) {
	e := newTestEnv(t)
	for _, session := range []string{"Planung März", " planning ", strings.Repeat("ä", 64)} {
		expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", session, "--json"), ExitOK)
		sent := e.coordinatorRequests()
		var body map[string]string
		if err := json.Unmarshal(sent[len(sent)-1].Body, &body); err != nil || body["session"] != session {
			t.Fatalf("sent %s for %q", sent[len(sent)-1].Body, session)
		}
	}
}

// A client of the project is not shown a coordinator: the board leaves the
// field out, show says so and hands over null with a notice, and boards show
// names none.
func TestAClientIsNotShownTheCoordinator(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "planning", "--json"), ExitOK)
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"

	env, r := e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.Contains(env.Notice, "does not show you") || strings.Contains(env.Summary, "No session") {
		t.Fatalf("envelope %+v", env)
	}
	if r := e.run("", "boards", "coordinator", "show", e.fixture.Board.ID, "--styled"); r.stdout != "This board does not show you a coordinator.\n" {
		t.Fatalf("stdout %q", r.stdout)
	}
	board := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, board, ExitOK)
	if strings.Contains(board.stdout, "coordinate") {
		t.Fatalf("boards show names a coordinator to a client:\n%s", board.stdout)
	}
	raw, _ := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	if _, named := decode(t, raw.Data)["coordinator"]; named {
		t.Fatal("the fake named the coordinator to a client")
	}
}

// The safeguard: two accounts coordinate one board side by side. Neither
// claim is refused, each account is shown its own entry alone, and a release
// takes off the own entry and leaves the other.
func TestTwoAccountsCoordinateABoardSideBySide(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "agent-session", "--json"), ExitOK)

	e.vars["MINKA_TOKEN"] = e.fixture.Token
	env, r := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "brunos")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["session"] != "brunos" {
		t.Fatalf("data %s", env.Data)
	}
	env, _ = e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	if decode(t, env.Data)["session"] != "brunos" {
		t.Fatalf("Bruno is shown %s", env.Data)
	}

	expectCode(t, e.run("", "boards", "coordinator", "release", e.fixture.Board.ID, "--json"), ExitOK)
	env, _ = e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	if string(env.Data) != "null" || !strings.Contains(env.Summary, "No session of yours") {
		t.Fatalf("after his release Bruno is shown %s, %q", env.Data, env.Summary)
	}

	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
	env, _ = e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	if decode(t, env.Data)["session"] != "agent-session" {
		t.Fatalf("Bruno's release took the agent's entry: %s", env.Data)
	}
}

// A Minka that still refuses a claim for somebody else's entry is reported as
// it answered: exit 9 with its code, and nothing of a takeover offered.
func TestAnOlderMinkaRefusingAClaimIsReportedAsItAnswered(t *testing.T) {
	e := newTestEnv(t)
	e.answeredBy(&bell{answers: []string{`409 {"error":{"code":"coordinatorTaken","message":"Somebody else coordinates this board."},"coordinator":{"person":{"id":"x","name":"Anke","kind":"human"},"session":"hers"}}`}})
	env, r := e.runJSON("", "boards", "coordinator", "claim", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--session", "planning")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "coordinatorTaken" || strings.Contains(env.Hint, "release") || strings.Contains(string(env.Data), "Anke") {
		t.Fatalf("envelope %+v", env)
	}
}

// Another session of the same person takes the board over at once.
func TestAnotherSessionOfThePersonTakesTheBoardOver(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "old", "--json"), ExitOK)
	env, r := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "new")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["session"] != "new" {
		t.Fatalf("data %s", env.Data)
	}
}

// A release takes the own entry off, and a second one succeeds as well; show
// then says that no session of yours coordinates the board, with data null.
func TestAReleaseTakesTheOwnEntryOffAndCanBeRepeated(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "agent-session", "--json"), ExitOK)

	for range 2 {
		env, r := e.runJSON("", "boards", "coordinator", "release", e.fixture.Board.ID)
		expectCode(t, r, ExitOK)
		if decode(t, env.Data)["coordinator"] != nil {
			t.Fatalf("data %s", env.Data)
		}
	}
	if sent := e.coordinatorRequests(); len(sent) != 3 || sent[2].Method != http.MethodDelete {
		t.Fatalf("sent %v", sent)
	}

	env, r := e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "" && string(env.Data) != "null" {
		t.Fatalf("data %s, want null", env.Data)
	}
	if r := e.run("", "boards", "coordinator", "show", e.fixture.Board.ID, "--styled"); !strings.Contains(r.stdout, "No session of yours coordinates this board.") {
		t.Fatalf("stdout %q", r.stdout)
	}
}

// The entry expires four hours after its last sign of life, and every wait of
// the same person is one.
func TestAWaitKeepsTheBoardAndSilenceLetsItExpire(t *testing.T) {
	e := newAgentEnv(t)
	began := e.server.Now()
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "agent-session", "--json"), ExitOK)

	e.server.SetNow(began.Add(3 * time.Hour))
	e.aLineArrivesInTheWait(&fakeapi.Notification{PersonID: e.fixture.Agent.ID, Occasion: "card.moved", Summary: "go on"})
	expectCode(t, e.run("", "notifications", "wait", "--json"), ExitOK)

	e.server.SetNow(began.Add(5 * time.Hour))
	env, r := e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	var shown map[string]any
	if err := json.Unmarshal(env.Data, &shown); err != nil || shown == nil {
		t.Fatalf("the board was let go although a wait kept it: %s", env.Data)
	}

	e.server.SetNow(began.Add(7*time.Hour + time.Second))
	env, _ = e.runJSON("", "boards", "coordinator", "show", e.fixture.Board.ID)
	if string(env.Data) != "" && string(env.Data) != "null" {
		t.Fatalf("data %s, want the entry expired", env.Data)
	}
}

// Somebody who may not edit the board, or cannot see it, is told there is no
// such board.
func TestOnlySomebodyWhoMayEditTheBoardCoordinatesIt(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	_, r := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "s")
	expectCode(t, r, ExitNotFound)
	_, r = e.runJSON("", "boards", "coordinator", "release", e.fixture.Board.ID)
	expectCode(t, r, ExitNotFound)
}
