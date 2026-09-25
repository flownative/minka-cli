package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/work"
)

// newAgentEnv signs the CLI in as the agent of the fixture. Only an agent may
// say what it is working on.
func newAgentEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
	return e
}

// runAt runs the CLI with the clock standing still at a moment, so that a
// throttle that spans processes can be tested without waiting.
func (e *testEnv) runAt(at time.Time, args ...string) result {
	e.t.Helper()
	return e.runWith(func(a *app) { a.now = func() time.Time { return at } }, "", args...)
}

func (e *testEnv) storePath() string {
	return work.Path(e.vars["MINKA_CONFIG"])
}

// signals are the requests that said what this machine is working on.
func (e *testEnv) signals() []fakeapi.RecordedRequest {
	var found []fakeapi.RecordedRequest
	for _, r := range e.server.Requests() {
		if r.Method == http.MethodPut && r.Path == "/api/v1/me/work" {
			found = append(found, r)
		}
	}
	return found
}

func (e *testEnv) lastSignal() map[string]any {
	e.t.Helper()
	signals := e.signals()
	if len(signals) == 0 {
		e.t.Fatal("no signal was sent")
	}
	var body map[string]any
	if err := json.Unmarshal(signals[len(signals)-1].Body, &body); err != nil {
		e.t.Fatalf("the body of the signal is not JSON: %v", err)
	}
	return body
}

// cardIDsOfTheSignal is what the last signal named, as strings.
func (e *testEnv) cardIDsOfTheSignal() []string {
	e.t.Helper()
	var ids []string
	named, ok := e.lastSignal()["cardIds"].([]any)
	if !ok {
		e.t.Fatalf("cardIds is not a list: %v", e.lastSignal()["cardIds"])
	}
	for _, id := range named {
		ids = append(ids, fmt.Sprint(id))
	}
	return ids
}

func TestStartingACardReportsItWithTheNextHeartbeat(t *testing.T) {
	e := newAgentEnv(t)
	card := e.fixture.CardInDoing.ID

	env, r := e.runJSON("", "work", "start", card)
	expectCode(t, r, ExitOK)
	if len(e.signals()) != 0 {
		t.Error("work start reported by itself; the heartbeat does that")
	}
	if !strings.Contains(env.Notice, "hooks install") {
		t.Errorf("nothing says that no signal has gone out yet: %q", env.Notice)
	}

	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if len(e.signals()) != 1 {
		t.Fatalf("%d signals, want 1", len(e.signals()))
	}
	signal := e.signals()[0]
	if got := signal.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content type %q", got)
	}
	if ids := e.cardIDsOfTheSignal(); len(ids) != 1 || ids[0] != card {
		t.Errorf("the signal named %v, want %s", ids, card)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != card {
		t.Errorf("the server holds %v, want %s", got, card)
	}
}

func TestAHeartbeatWithNothingRememberedAsksNothingOfTheNetwork(t *testing.T) {
	e := newAgentEnv(t)

	r := e.run("", "heartbeat")
	expectCode(t, r, ExitOK)
	if len(e.server.Requests()) != 0 {
		t.Errorf("the heartbeat sent %d requests with nothing remembered", len(e.server.Requests()))
	}
	if r.stdout != "" || r.stderr != "" {
		t.Errorf("the heartbeat said something: %q %q", r.stdout, r.stderr)
	}
	// Not even the directory beside the configuration: with nothing
	// remembered, the command is a look at one path and no more.
	if _, err := os.Stat(filepath.Dir(e.storePath())); err == nil {
		t.Error("the heartbeat made a store although nothing was remembered")
	}
}

func TestTheHeartbeatReportsAtMostOnceAMinuteAndAtOnceOnAChange(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(now, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	expectCode(t, e.runAt(now, "heartbeat", "--json"), ExitOK)
	expectCode(t, e.runAt(now.Add(30*time.Second), "heartbeat", "--json"), ExitOK)
	if len(e.signals()) != 1 {
		t.Fatalf("%d signals within a minute, want 1", len(e.signals()))
	}

	expectCode(t, e.runAt(now.Add(61*time.Second), "heartbeat", "--json"), ExitOK)
	if len(e.signals()) != 2 {
		t.Fatalf("%d signals after a minute, want 2", len(e.signals()))
	}

	// A card that was given up must not wait for the next minute: the server
	// would show it as alive for three more.
	expectCode(t, e.runAt(now.Add(62*time.Second), "work", "stop", "--all", "--json"), ExitOK)
	expectCode(t, e.runAt(now.Add(63*time.Second), "heartbeat", "--json"), ExitOK)
	if len(e.signals()) != 3 {
		t.Fatalf("%d signals after the list changed, want 3", len(e.signals()))
	}
	if ids := e.cardIDsOfTheSignal(); len(ids) != 0 {
		t.Errorf("the signal named %v, want nothing", ids)
	}
	if got := e.server.CardsAtWork(); len(got) != 0 {
		t.Errorf("the server still holds %v", got)
	}

	// And with nothing left to take back, it goes quiet again.
	expectCode(t, e.runAt(now.Add(5*time.Minute), "heartbeat", "--json"), ExitOK)
	if len(e.signals()) != 3 {
		t.Errorf("%d signals, want 3: nothing is remembered any more", len(e.signals()))
	}
}

func TestAHeartbeatThatCannotReportDisturbsNobodyAndSaysWhyLater(t *testing.T) {
	e := newTestEnv(t)
	// A person's token: the server answers 403 agentsOnly, which no agent can
	// do anything about from inside a tool call.
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(now, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	r := e.runAt(now, "heartbeat")
	expectCode(t, r, ExitOK)
	if r.stdout != "" || r.stderr != "" {
		t.Errorf("the heartbeat said something: %q %q", r.stdout, r.stderr)
	}
	if len(e.signals()) != 1 {
		t.Fatalf("%d signals, want 1", len(e.signals()))
	}

	// The word a caller asking for it reads names the case, the way the list
	// keeps it: an agent is sent to the reason first of all.
	beat, asked := e.runWithJSON(now.Add(61*time.Second), "heartbeat")
	expectCode(t, asked, ExitOK)
	if reason, _ := decode(t, beat.Data)["reason"].(string); !strings.Contains(reason, "agentsOnly") {
		t.Errorf("the reason %q does not name the case", reason)
	}

	// At the same moment as the signals: with the clock of the day, the card
	// could have run out by now and an empty list would say nothing at all.
	env, listing := e.runWithJSON(now.Add(2*time.Minute), "work", "list")
	expectCode(t, listing, ExitOK)
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 1 {
		t.Fatalf("%d cards, want the one that is at work", len(cards))
	}
	if !strings.Contains(env.Notice, "did not arrive") {
		t.Errorf("the list says nothing about the failed signal: %q", env.Notice)
	}
	data := decode(t, env.Data)
	if failure, _ := data["failure"].(string); !strings.Contains(failure, "agentsOnly") {
		t.Errorf("the failure %v does not name the case", data["failure"])
	}

	// The two words must not contradict each other: a signal that went out and
	// was turned away is no reason to go and install a hook.
	if strings.Contains(env.Notice, "hooks install") {
		t.Errorf("the list advises the hook although a signal went out: %q", env.Notice)
	}
	if data["sentAt"] == nil {
		t.Error("nothing says that a signal went out")
	}
	if data["reportedAt"] != nil {
		t.Errorf("reportedAt is %v although nothing arrived", data["reportedAt"])
	}
}

func TestAHeartbeatWithoutASignInIsQuietAndExitsZero(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	delete(e.vars, "MINKA_TOKEN")
	delete(e.vars, "MINKA_HOST")

	env, r := e.runJSON("", "heartbeat")
	expectCode(t, r, ExitOK)
	if !env.OK {
		t.Errorf("the heartbeat failed: %s", env.Error)
	}
	if data := decode(t, env.Data); data["sent"] != false {
		t.Errorf("sent %v", data["sent"])
	}
	if len(e.signals()) != 0 {
		t.Error("a signal went out without a sign-in")
	}
}

func TestTheClientIsAMeaninglessNameThatIsMadeOnce(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)

	clientID, ok := e.lastSignal()["clientId"].(string)
	if !ok || !regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`).MatchString(clientID) {
		t.Fatalf("the client is %v", e.lastSignal()["clientId"])
	}
	host, _ := os.Hostname()
	user := os.Getenv("USER")
	for _, secret := range []string{host, user} {
		if secret != "" && strings.Contains(strings.ToLower(clientID), strings.ToLower(secret)) {
			t.Errorf("the client %q names this machine or its user", clientID)
		}
	}

	expectCode(t, e.run("", "work", "start", e.fixture.CardInTriage.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if again := e.lastSignal()["clientId"]; again != clientID {
		t.Errorf("the client changed from %q to %v", clientID, again)
	}

	content, err := os.ReadFile(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), clientID) {
		t.Error("the client is not kept in the configuration")
	}
}

func TestACardTheAgentCannotSeeIsRefusedWhereSomebodyCanReadIt(t *testing.T) {
	e := newAgentEnv(t)

	env, r := e.runJSON("", "work", "start", e.fixture.OtherCard.ID)
	expectCode(t, r, ExitNotFound)
	if env.Code != CodeNotFound {
		t.Errorf("code %q", env.Code)
	}
	if _, err := os.Stat(e.storePath()); err == nil {
		t.Error("a card that cannot be seen was remembered")
	}
}

func TestACardIsStartedAndStoppedUnderTheNameABoardShows(t *testing.T) {
	e := newAgentEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"
	card := e.fixture.CardInDoing.ID

	expectCode(t, e.run("", "work", "start", "mnk-1a", "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if ids := e.cardIDsOfTheSignal(); len(ids) != 1 || ids[0] != card {
		t.Errorf("the signal named %v, want the ID %s", ids, card)
	}

	// Stopping asks nobody: the short identifier was remembered with the card.
	before := len(e.server.Requests())
	env, r := e.runJSON("", "work", "stop", "MNK-1A")
	expectCode(t, r, ExitOK)
	if len(e.server.Requests()) != before {
		t.Error("work stop sent a request")
	}
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 0 {
		t.Errorf("%d cards left", len(cards))
	}
}

func TestStoppingWithoutACardLeavesTheListOfOthersAlone(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInTriage.ID, "--json"), ExitOK)

	env, r := e.runJSON("", "work", "stop")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Hint, "--all") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}

	listing, _ := e.runJSON("", "work", "list")
	if cards := decode(t, listing.Data)["cards"].([]any); len(cards) != 2 {
		t.Fatalf("%d cards left, want 2", len(cards))
	}

	// With one card left there is nothing to mix up.
	expectCode(t, e.run("", "work", "stop", e.fixture.CardInTriage.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "work", "stop", "--json"), ExitOK)
	listing, _ = e.runJSON("", "work", "list")
	if cards := decode(t, listing.Data)["cards"].([]any); len(cards) != 0 {
		t.Errorf("%d cards left, want none", len(cards))
	}
}

// What a command did has to stand where a person reads, not in a summary that
// only the JSON carries: the list it prints shows what is left.
func TestStoppingSaysWhichCardItStoppedOnATerminal(t *testing.T) {
	e := newAgentEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	r := e.run("", "work", "stop", "--all", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Stopped") || !strings.Contains(r.stdout, "MNK-1A") {
		t.Errorf("the text does not say what was stopped:\n%s", r.stdout)
	}

	r = e.run("", "work", "stop", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Nothing to stop") {
		t.Errorf("the text does not say that there was nothing to stop:\n%s", r.stdout)
	}
}

func TestOneSignalCarriesTheCardsOfEverySessionOnThisMachine(t *testing.T) {
	e := newAgentEnv(t)
	// Two sessions, each starting a card of its own: the server keeps one set
	// per agent and client, so a signal that left the other's card out would
	// take it away.
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInTriage.ID, "--json"), ExitOK)

	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	if ids := e.cardIDsOfTheSignal(); len(ids) != 2 {
		t.Errorf("the signal named %v, want both cards", ids)
	}
	if got := e.server.CardsAtWork(); len(got) != 2 {
		t.Errorf("the server holds %v, want both cards", got)
	}
}

// The safeguard: the store is read and written under a lock of the kernel.
// Without it, two sessions starting a card at the same moment read the same
// list and the one that writes last drops the other's card — the tile of
// somebody who is working goes dark.
func TestTwoSessionsStartingAtOnceLoseNoCard(t *testing.T) {
	e := newAgentEnv(t)
	cards := addCards(e, 12)

	var wg sync.WaitGroup
	for _, card := range cards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := e.run("", "work", "start", card, "--json"); r.code != ExitOK {
				t.Errorf("work start %s: exit %d: %s", card, r.code, r.stdout)
			}
		}()
	}
	wg.Wait()

	env, r := e.runJSON("", "work", "list")
	expectCode(t, r, ExitOK)
	remembered := map[string]bool{}
	for _, entry := range decode(t, env.Data)["cards"].([]any) {
		remembered[fmt.Sprint(entry.(map[string]any)["id"])] = true
	}
	for _, card := range cards {
		if !remembered[card] {
			t.Errorf("%s was lost; %d of %d cards are remembered", card, len(remembered), len(cards))
		}
	}
}

// addCards puts cards into the column the agent works in and answers with
// their IDs.
func addCards(e *testEnv, count int) []string {
	e.t.Helper()
	var ids []string
	for i := range count {
		id := fmt.Sprintf("0199e7b0-0000-4000-8000-%012d", i)
		e.fixture.Doing.Cards = append(e.fixture.Doing.Cards, &fakeapi.Card{
			ID: id, Title: fmt.Sprintf("Card %d", i), CreatedAt: time.Now(), CreatorID: e.fixture.Bruno.ID,
		})
		ids = append(ids, id)
	}
	return ids
}

func TestACardOfASessionThatNeverCameBackIsForgotten(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(now, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runWithJSON(now.Add(work.ForgottenAfter+time.Minute), "work", "list")
	expectCode(t, r, ExitOK)
	if cards := decode(t, env.Data)["cards"].([]any); len(cards) != 0 {
		t.Errorf("%d cards after a working day, want none", len(cards))
	}
	// A list that is suddenly shorter says why, and what to do about a card
	// somebody is still at work on.
	if !strings.Contains(env.Notice, "given up") || !strings.Contains(env.Notice, "work start") {
		t.Errorf("nothing says that a card was given up: %q", env.Notice)
	}
}

// runWithJSON runs a command with --json at a moment and decodes the envelope.
func (e *testEnv) runWithJSON(at time.Time, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.runAt(at, append(args, "--json")...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s", err, r.stdout)
	}
	return env, r
}

func TestTheStoreLivesBesideTheConfigurationAndNotInTheProject(t *testing.T) {
	e := newAgentEnv(t)
	expectCode(t, e.run("", "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)

	if _, err := os.Stat(e.storePath()); err != nil {
		t.Fatalf("no store at %s: %v", e.storePath(), err)
	}
	if filepath.Dir(e.storePath()) != filepath.Dir(e.vars["MINKA_CONFIG"]) {
		t.Errorf("the store lies at %s, the configuration at %s", e.storePath(), e.vars["MINKA_CONFIG"])
	}
}

// say sends a body to the address an agent reports through, the way a client
// that is not this CLI might, and answers with the status and the body.
func (e *testEnv) say(token, contentType, body string) (int, string) {
	e.t.Helper()
	request, err := http.NewRequest(http.MethodPut, e.server.URL+"/api/v1/me/work", strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", contentType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		e.t.Fatal(err)
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(answer)
}

// The contract of the address, in the order the server checks it. The CLI
// sends none of these itself, and the fake has to answer them the way Minka
// does, or a test of the CLI against it would prove nothing.
func TestTheAddressRefusesInTheOrderTheContractGives(t *testing.T) {
	e := newAgentEnv(t)
	agent, person := e.fixture.AgentToken, e.fixture.Token
	card := e.fixture.CardInDoing.ID

	for _, one := range []struct {
		name        string
		token       string
		contentType string
		body        string
		status      int
		code        string
	}{
		{"no token", "", "application/json", `{}`, http.StatusUnauthorized, "unauthenticated"},
		{"a person, before the body is looked at", person, "application/json", `{"clientId": "no spaces"`, http.StatusForbidden, "agentsOnly"},
		{"another content type", agent, "text/plain", `{}`, http.StatusUnsupportedMediaType, "unsupportedMediaType"},
		{"a body that is no object", agent, "application/json", `[]`, http.StatusBadRequest, "malformedBody"},
		{"a client that is no string", agent, "application/json", `{"clientId": 17}`, http.StatusBadRequest, "malformedBody"},
		{"a client with a space in it", agent, "application/json", `{"clientId": "mac studio"}`, http.StatusBadRequest, "malformedBody"},
		{"a client that is too long", agent, "application/json", `{"clientId": "` + strings.Repeat("a", 65) + `"}`, http.StatusBadRequest, "malformedBody"},
		{"cards that are no list", agent, "application/json", `{"cardIds": "` + card + `"}`, http.StatusBadRequest, "malformedBody"},
		{"cards that are no list of strings", agent, "application/json", `{"cardIds": [17]}`, http.StatusBadRequest, "malformedBody"},
	} {
		t.Run(one.name, func(t *testing.T) {
			status, body := e.say(one.token, one.contentType, one.body)
			if status != one.status {
				t.Fatalf("status %d, want %d: %s", status, one.status, body)
			}
			if !strings.Contains(body, one.code) {
				t.Errorf("the body is %s, want the code %s", body, one.code)
			}
			if len(e.server.CardsAtWork()) != 0 {
				t.Error("a refused body left a mark")
			}
		})
	}
}

func TestTheServerKeepsOneSetPerClientAndPassesOverWhatItDoesNotKnow(t *testing.T) {
	e := newAgentEnv(t)
	agent := e.fixture.AgentToken
	mine, theirs := e.fixture.CardInDoing.ID, e.fixture.OtherCard.ID

	// A card in capitals is the same card; one the agent may not see and one
	// that is no identifier at all are passed over without a word.
	status, body := e.say(agent, "application/json",
		`{"clientId": "one", "cardIds": ["`+strings.ToUpper(mine)+`", "`+theirs+`", "not an id"]}`)
	if status != http.StatusNoContent || body != "" {
		t.Fatalf("status %d, body %q", status, body)
	}
	if got := e.server.AtWorkOf(e.fixture.Agent.ID, "one"); len(got) != 1 || got[0] != mine {
		t.Fatalf("the client holds %v, want %s", got, mine)
	}

	// A second client of the same agent keeps its own set, and a client that
	// stops calms its own cards alone.
	if status, _ := e.say(agent, "application/json", `{"cardIds": ["`+e.fixture.CardInTriage.ID+`"]}`); status != http.StatusNoContent {
		t.Fatalf("status %d", status)
	}
	if status, _ := e.say(agent, "application/json", `{"clientId": "one", "cardIds": []}`); status != http.StatusNoContent {
		t.Fatalf("status %d", status)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != e.fixture.CardInTriage.ID {
		t.Errorf("the server holds %v, want the card of the client with no name", got)
	}
}

// cardIDsOfTheList is what a listing of this machine's cards names.
func cardIDsOfTheList(t *testing.T, env envelope) []string {
	t.Helper()
	var ids []string
	for _, card := range decode(t, env.Data)["cards"].([]any) {
		ids = append(ids, fmt.Sprint(card.(map[string]any)["id"]))
	}
	return ids
}

// A card somebody moves on is one nobody is at work on any more. The list is
// the machine's and holds the cards of every session on it, so the one card
// leaves and the rest stay, and the board is told before the next tool call
// rather than after it.
func TestMovingACardOnTakesItOutOfTheListAtOnce(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	moved, kept := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	expectCode(t, e.runAt(now, "work", "start", moved, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "work", "start", kept, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "heartbeat", "--json"), ExitOK)
	if got := e.server.CardsAtWork(); len(got) != 2 {
		t.Fatalf("the server holds %v, want both cards", got)
	}

	env, r := e.runWithJSON(now, "cards", "move", moved, "--column", "not_now")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no longer among the cards") || !strings.Contains(env.Notice, "told at once") {
		t.Errorf("the move said nothing about the list: %q", env.Notice)
	}
	listing, _ := e.runWithJSON(now, "work", "list")
	if left := cardIDsOfTheList(t, listing); len(left) != 1 || left[0] != kept {
		t.Errorf("%v is left in the list, want the card nobody moved", left)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != kept {
		t.Errorf("the server holds %v, want %s", got, kept)
	}

	// A card nobody said they were at work on changes nothing, and a move that
	// changes nothing asks nothing of the network — two minutes on, where no
	// throttle would hold a signal back.
	signals := len(e.signals())
	env, r = e.runWithJSON(now.Add(2*time.Minute), "cards", "move", moved, "--column", "Doing")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Errorf("a card nobody is working on: %q", env.Notice)
	}
	if sent := len(e.signals()); sent != signals {
		t.Errorf("%d signals for a move that changed no list", sent-signals)
	}
}

// A card that only changes place within its column is one nobody has let go
// of; one that is put aside has been let go of, the column notwithstanding.
func TestACardThatOnlyChangesPlaceStaysInTheList(t *testing.T) {
	e := newAgentEnv(t)
	card := e.fixture.CardInDoing.ID
	expectCode(t, e.run("", "columns", "on-hold", e.fixture.Doing.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "work", "start", card, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "move", card, "--column", "Doing", "--bottom")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Errorf("a card that stayed where it was: %q", env.Notice)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != card {
		t.Fatalf("the server holds %v, want %s", got, card)
	}

	env, r = e.runJSON("", "cards", "move", card, "--column", "Doing", "--on-hold")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("a card put aside: %q", env.Notice)
	}
	if got := e.server.CardsAtWork(); len(got) != 0 {
		t.Errorf("the server still holds %v", got)
	}

	// Taken up again and pushed about among the cards on hold: it was aside
	// before and is aside after, so nothing has been let go of here.
	expectCode(t, e.run("", "work", "start", card, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)
	env, r = e.runJSON("", "cards", "move", card, "--column", "Doing", "--on-hold", "--bottom")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Errorf("a card that was aside before and after: %q", env.Notice)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 {
		t.Errorf("the server holds %v, want the card", got)
	}
}

// Done is the end of a card whichever command puts it there, and a card that
// lies there already is no exception.
func TestAMoveIntoDoneLetsGoOfTheCard(t *testing.T) {
	e := newAgentEnv(t)
	card := e.fixture.CardInDoing.ID
	expectCode(t, e.run("", "work", "start", card, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "move", card, "--column", "done")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("a card moved into Done: %q", env.Notice)
	}

	// Again, on a card that is in Done already: `cards done` takes such a card
	// out of the list, and a move has no reason to do otherwise.
	expectCode(t, e.run("", "work", "start", card, "--json"), ExitOK)
	env, r = e.runJSON("", "cards", "move", card, "--column", "done")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("a card that was in Done already: %q", env.Notice)
	}
	if got := e.server.CardsAtWork(); len(got) != 0 {
		t.Errorf("the server still holds %v", got)
	}
}

// A card that ran out while the command was running is not the card the
// command moved, and the word about it belongs to the card that was moved.
func TestAMoveSaysNothingAboutACardItDidNotMove(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	ranOut, moved := e.fixture.CardInTriage.ID, e.fixture.CardInDoing.ID
	expectCode(t, e.runAt(began, "work", "start", ranOut, "--json"), ExitOK)

	// A working day on, the one card is given up by the sweep and the other
	// was never in the list at all.
	env, r := e.runWithJSON(began.Add(work.ForgottenAfter+time.Minute), "cards", "move", moved, "--column", "not_now")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Errorf("a move that let go of nothing: %q", env.Notice)
	}
}

// The move is done and stays done whatever becomes of the signal, and the word
// about it promises no more than happened.
func TestAMoveWhoseSignalIsRefusedStandsAllTheSame(t *testing.T) {
	// A person's token: the server answers the signal 403 agentsOnly, and
	// everything else as usual.
	e := newTestEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID
	expectCode(t, e.runAt(now, "work", "start", card, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "heartbeat"), ExitOK)

	env, r := e.runWithJSON(now, "cards", "move", card, "--column", "not_now")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "next signal") {
		t.Errorf("the notice promises more than happened: %q", env.Notice)
	}
	listing, _ := e.runWithJSON(now, "work", "list")
	if left := cardIDsOfTheList(t, listing); len(left) != 0 {
		t.Errorf("%v is left in the list", left)
	}
	// The one the heartbeat sent, and the one the move sent after it: a list
	// that was reported once has to be taken back even where nobody listens.
	if len(e.signals()) != 2 {
		t.Errorf("%d signals, want the two that were refused", len(e.signals()))
	}
}

// Completing a card is the other way a card leaves the hand, and a card that
// was done already is done all the same.
func TestCompletingACardTakesItOutOfTheListAtOnce(t *testing.T) {
	e := newAgentEnv(t)
	done, kept := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	expectCode(t, e.run("", "work", "start", done, "--json"), ExitOK)
	expectCode(t, e.run("", "work", "start", kept, "--json"), ExitOK)
	expectCode(t, e.run("", "heartbeat", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "done", done)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("done said nothing about the list: %q", env.Notice)
	}
	listing, _ := e.runJSON("", "work", "list")
	if left := cardIDsOfTheList(t, listing); len(left) != 1 || left[0] != kept {
		t.Errorf("%v is left in the list, want the card nobody completed", left)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != kept {
		t.Errorf("the server holds %v, want %s", got, kept)
	}

	// Somebody who says it again of a card that is done takes it out too.
	expectCode(t, e.run("", "work", "start", done, "--json"), ExitOK)
	env, r = e.runJSON("", "cards", "done", done)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already done") {
		t.Fatalf("summary %q", env.Summary)
	}
	if !strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("a card that was done already: %q", env.Notice)
	}
	listing, _ = e.runJSON("", "work", "list")
	if left := cardIDsOfTheList(t, listing); len(left) != 1 || left[0] != kept {
		t.Errorf("%v is left in the list, want the card nobody completed", left)
	}
}

// Work that lasts longer than a working day keeps its card, because the
// session says after every tool call that it is still at it. The session
// beside it that died keeps nothing, however busy the machine.
func TestTheSessionThatKeepsWorkingKeepsItsCard(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	kept, lost := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one that dies"
	expectCode(t, e.runAt(began, "work", "start", lost, "--json"), ExitOK)
	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one that works on"
	expectCode(t, e.runAt(began, "work", "start", kept, "--json"), ExitOK)

	// Ten hours of tool calls, all of them from the session that is still at
	// its card.
	for hour := 0; hour <= 10; hour++ {
		expectCode(t, e.runAt(began.Add(time.Duration(hour)*time.Hour), "heartbeat", "--json"), ExitOK)
	}

	env, r := e.runWithJSON(began.Add(10*time.Hour+time.Minute), "work", "list")
	expectCode(t, r, ExitOK)
	if left := cardIDsOfTheList(t, env); len(left) != 1 || left[0] != kept {
		t.Fatalf("%v is left, want the card of the session that kept working", left)
	}
	if got := e.server.CardsAtWork(); len(got) != 1 || got[0] != kept {
		t.Errorf("the server holds %v, want %s", got, kept)
	}
}

// A card taken up where nothing says who is asking goes by the moment it was
// started, and a machine that is busy around it changes nothing about that.
func TestACardWithoutASessionGoesByItsBeginning(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInTriage.ID, "--json"), ExitOK)

	e.vars["CLAUDE_CODE_SESSION_ID"] = "somebody else entirely"
	for hour := 0; hour <= 9; hour++ {
		expectCode(t, e.runAt(began.Add(time.Duration(hour)*time.Hour), "heartbeat", "--json"), ExitOK)
	}

	// It is gone by the sweep of the signal that ran past the window, so the
	// list has nothing left to give up and nothing to say about it.
	env, r := e.runWithJSON(began.Add(9*time.Hour+time.Minute), "work", "list")
	expectCode(t, r, ExitOK)
	if left := cardIDsOfTheList(t, env); len(left) != 0 {
		t.Errorf("%v is left, want none", left)
	}
}

// The safeguard: the sign of life is written before the throttle decides
// whether to send. A tool call says that the session is at its cards whether
// or not a signal goes out from it, and the throttle looks at the whole list
// of the machine — so a session whose beats fall into the minute of another
// session's signal would lose its card under its own hands.
func TestTheSignOfLifeIsWrittenWhereNoSignalGoesOut(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	atWork, beside := e.fixture.CardInDoing.ID, e.fixture.CardInTriage.ID

	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one that keeps at it"
	expectCode(t, e.runAt(began, "work", "start", atWork, "--json"), ExitOK)
	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one beside it"
	expectCode(t, e.runAt(began, "work", "start", beside, "--json"), ExitOK)

	// The session beside it reports first, so the minute of the throttle is
	// spent on its signal.
	expectCode(t, e.runAt(began.Add(10*time.Second), "heartbeat", "--json"), ExitOK)

	// The other session beats within that minute: the list is the same one,
	// nothing goes out, and the card is being worked on all the same.
	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one that keeps at it"
	beat, r := e.runWithJSON(began.Add(40*time.Second), "heartbeat")
	expectCode(t, r, ExitOK)
	if sent, _ := decode(t, beat.Data)["sent"].(bool); sent {
		t.Fatal("the beat within the minute sent a signal, so the throttle was not what held it")
	}
	if len(e.signals()) != 1 {
		t.Fatalf("%d signals, want the one of the session beside it", len(e.signals()))
	}

	// A working day after the two cards were taken up: the one whose session
	// beat is the one that is left.
	env, r := e.runWithJSON(began.Add(work.ForgottenAfter+20*time.Second), "work", "list")
	expectCode(t, r, ExitOK)
	if left := cardIDsOfTheList(t, env); len(left) != 1 || left[0] != atWork {
		t.Fatalf("%v is left, want the card of the session whose beat sent nothing", left)
	}
}

// The safeguard: what has run out is given up before the sign of life is
// written. A session that comes back after a day away was not at its card, so
// the beat it brings must not raise a card that is over — and the board has to
// hear that it is over.
func TestABeatThatComesTooLateRaisesNothing(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing.ID

	e.vars["CLAUDE_CODE_SESSION_ID"] = "the one that goes away"
	expectCode(t, e.runAt(began, "work", "start", card, "--json"), ExitOK)
	expectCode(t, e.runAt(began.Add(time.Minute), "heartbeat", "--json"), ExitOK)
	if got := e.server.CardsAtWork(); len(got) != 1 {
		t.Fatalf("the server holds %v, want the card that was taken up", got)
	}

	// A day later the same session beats again, its card long run out.
	tooLate := began.Add(24 * time.Hour)
	expectCode(t, e.runAt(tooLate, "heartbeat", "--json"), ExitOK)
	if got := e.server.CardsAtWork(); len(got) != 0 {
		t.Errorf("the server still holds %v after the card ran out", got)
	}

	env, r := e.runWithJSON(tooLate.Add(time.Minute), "work", "list")
	expectCode(t, r, ExitOK)
	if left := cardIDsOfTheList(t, env); len(left) != 0 {
		t.Errorf("%v was raised by a beat that came a day late", left)
	}
}
