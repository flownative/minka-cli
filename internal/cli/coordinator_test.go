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
// names it from then on: in data, in coordinator show and in boards show.
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
	if !strings.Contains(r.stdout, "Coordinator: Claude (Bruno) (agent, session "+sessionOfTheWait+")") {
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
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "two words"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if sent := e.coordinatorRequests(); len(sent) != 1 {
		t.Fatalf("%d claims went out, want only the first", len(sent))
	}
}

// The safeguard: while another person coordinates the board, a claim exits 9
// with the holder in data and in the hint, and takes nothing over.
func TestAClaimWhileAnotherPersonHoldsTheBoardNamesThem(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "agent-session", "--json"), ExitOK)

	e.vars["MINKA_TOKEN"] = e.fixture.Token
	env, r := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "brunos")
	expectCode(t, r, ExitValidation)
	if env.Code != CodeValidation || env.Meta["apiCode"] != "coordinatorTaken" {
		t.Fatalf("code %q, apiCode %v", env.Code, env.Meta["apiCode"])
	}
	if holder := decode(t, env.Data); holder["session"] != "agent-session" {
		t.Fatalf("data %s does not name the holder", env.Data)
	}
	if !strings.Contains(env.Hint, "Claude (Bruno) (agent, session agent-session)") || !strings.Contains(env.Hint, "minka boards coordinator release "+e.fixture.Board.ID) {
		t.Fatalf("hint %q", env.Hint)
	}
	if e.fixture.Board.Coordinator.Session != "agent-session" {
		t.Fatal("the claim took the board over")
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

// A release takes the entry off whoever holds it, and a second one succeeds
// as well; show then says nobody, with data null.
func TestAReleaseTakesTheCoordinatorOffAndCanBeRepeated(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "agent-session", "--json"), ExitOK)

	e.vars["MINKA_TOKEN"] = e.fixture.Token
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
	if r := e.run("", "boards", "coordinator", "show", e.fixture.Board.ID, "--styled"); !strings.Contains(r.stdout, "Nobody coordinates this board.") {
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
