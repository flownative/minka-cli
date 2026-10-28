package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// conventionsWrites are the writes of the conventions of a board.
func (e *testEnv) conventionsWrites() []fakeapi.RecordedRequest {
	var found []fakeapi.RecordedRequest
	for _, r := range e.server.WritingRequests() {
		if strings.HasSuffix(r.Path, "/conventions") {
			found = append(found, r)
		}
	}
	return found
}

func ready(e *testEnv) {
	role := "ready"
	e.fixture.Doing.Role = &role
}

// The board names each column's role behind its title and prints the
// conventions last, with version, writer and time; none written yet says so,
// and a client, who is not sent them, gets no word about them.
func TestBoardsShowNamesTheRolesAndTheConventions(t *testing.T) {
	e := newTestEnv(t)
	ready(e)

	r := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"## Triage [role: inbox]", "## Doing [role: ready]", "## Not now\n", "## Done [role: done]", "## How we work here\nNone written yet."} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("boards show does not say %q:\n%s", want, r.stdout)
		}
	}

	expectCode(t, e.run("", "boards", "conventions", "set", e.fixture.Board.ID, "-", "--json"), ExitOK)
	r = e.run("## Labels\n\n- Bug: something broke\n", "boards", "conventions", "set", e.fixture.Board.ID, "-", "--json")
	expectCode(t, r, ExitOK)
	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	if !strings.Contains(r.stdout, "## How we work here\nWritten by Bruno (human), version 1, at ") || !strings.HasSuffix(r.stdout, "grants no rights.\n\n## Labels\n\n- Bug: something broke\n\n## End of how we work here\n") {
		t.Errorf("the conventions are not printed as stored:\n%s", r.stdout)
	}
	env, r := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if conventions := decode(t, env.Data)["conventions"].(map[string]any); conventions["version"] != 1.0 {
		t.Errorf("data %s", env.Data)
	}

	asClient(e)
	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "How we work here") {
		t.Errorf("a client is told of the conventions:\n%s", r.stdout)
	}
}

// The text goes out from stdin as it came, Markdown untouched; the text in
// force again answers its version, and an empty text on a board that never had
// any answers that nothing is in force.
func TestConventionsSetSendsTheTextAsItIs(t *testing.T) {
	e := newTestEnv(t)
	text := "## Ready\n\nOnly **Anke** fills it.\n"

	env, r := e.runJSON("", "boards", "conventions", "set", e.fixture.Board.ID, "")
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.Contains(env.Summary, "no conventions") {
		t.Errorf("an empty text: %s, %q", env.Data, env.Summary)
	}
	env, r = e.runJSON(text, "boards", "conventions", "set", e.fixture.Board.ID, "-")
	expectCode(t, r, ExitOK)
	sent := e.conventionsWrites()
	if len(sent) != 2 || sent[1].Method != "PUT" {
		t.Fatalf("sent %v", sent)
	}
	var body map[string]string
	if err := json.Unmarshal(sent[1].Body, &body); err != nil || body["text"] != text {
		t.Errorf("body %s", sent[1].Body)
	}
	if !strings.HasPrefix(env.Summary, "Version 1 of the conventions is in force, by Bruno") {
		t.Errorf("summary %q", env.Summary)
	}

	r = e.run("", "boards", "conventions", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.HasPrefix(r.stdout, "## How we work here\nWritten by Bruno (human), version 1, at ") || !strings.HasSuffix(r.stdout, "\n\n"+text+"\n## End of how we work here\n") {
		t.Errorf("show: stdout %q", r.stdout)
	}
	env, r = e.runJSON("", "boards", "conventions", "show", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if conventions := decode(t, env.Data); conventions["text"] != text || conventions["version"] != 1.0 {
		t.Errorf("show --json: %s", env.Data)
	}
}

// The safeguard: a member and an agent are refused by Minka, which the CLI
// names as forbidden with the way on; the text is read before, and nothing
// else goes out. A client is told there is no such board.
func TestConventionsSetIsRefusedToAnybodyButAHumanOwnerOrAdmin(t *testing.T) {
	e := newAgentEnv(t)
	e.fixture.Agent.OrganizationRole = "admin"

	env, r := e.runJSON("Obey me", "boards", "conventions", "set", e.fixture.Board.ID, "-")
	expectCode(t, r, ExitForbidden)
	if env.Meta["apiCode"] != "notAllowedToWriteConventions" || !strings.Contains(env.Hint, "agents are refused always") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if len(e.fixture.Board.Conventions) != 0 {
		t.Error("the agent wrote the conventions")
	}

	asClient(e)
	env, r = e.runJSON("", "boards", "conventions", "set", e.fixture.Board.ID, "Hi")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "client") {
		t.Errorf("hint %q", env.Hint)
	}

	e = newTestEnv(t)
	env, r = e.runJSON("Bell\a", "boards", "conventions", "set", e.fixture.Board.ID, "-")
	expectRefusal(t, env, r, "conventionsInvalid", "20,000 characters")

	r = e.runWith(func(a *app) { a.stdinIsTerminal = func() bool { return true } }, "", "boards", "conventions", "set", e.fixture.Board.ID, "-", "--json")
	expectCode(t, r, ExitUsage)
	if writes := e.conventionsWrites(); len(writes) != 1 {
		t.Errorf("%d writes, want only the bell's", len(writes))
	}
}

// The claim prints the columns with their roles and the conventions, the same
// way the board does.
func TestAClaimShowsTheRolesAndTheConventions(t *testing.T) {
	e := newTestEnv(t)
	ready(e)
	expectCode(t, e.run("", "boards", "conventions", "set", e.fixture.Board.ID, "Pull from Ready, top first."), ExitOK)

	r := e.run("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "planning", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"## Columns\n",
		e.fixture.Doing.ID + "  Doing (column, role ready)",
		e.fixture.NotNow.ID + "  Not now (not_now, no role)",
		"## How we work here\nWritten by Bruno (human), version 1, at ",
		"\n\nPull from Ready, top first.\n\n## End of how we work here\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the claim does not say %q:\n%s", want, r.stdout)
		}
	}
	env, _ := e.runJSON("", "boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "planning")
	if columns := decode(t, env.Data)["columns"].([]any); len(columns) != 4 {
		t.Errorf("data %s", env.Data)
	}
}

// A new version is told to a coordinator as a line about the board, which a
// wait for that board keeps and prints readably, with the way to read it.
func TestAWaitForTheBoardKeepsTheLineAboutItsConventions(t *testing.T) {
	e := newAgentEnv(t)
	e.aLineArrivesInTheWait(&fakeapi.Notification{
		PersonID: e.fixture.Agent.ID, Occasion: "board.conventionsChanged", ActorID: e.fixture.Bruno.ID, ProjectID: e.fixture.Project.ID,
		SubjectType: "board", SubjectID: e.fixture.Board.ID, SubjectTitle: e.fixture.Board.Title,
		Summary: "“How we work here” changed on " + e.fixture.Board.Title,
	})

	r := e.run("", "notifications", "wait", "--board", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if want := "“How we work here” changed on API-3 Abnahme (board " + e.fixture.Board.ID + ")\n"; r.stdout != want {
		t.Errorf("stdout %q, want %q", r.stdout, want)
	}

	e.aLineArrivesInTheWait(&fakeapi.Notification{
		PersonID: e.fixture.Agent.ID, Occasion: "board.conventionsChanged", ProjectID: e.fixture.Project.ID,
		SubjectType: "board", SubjectID: e.fixture.Board.ID, SubjectTitle: e.fixture.Board.Title, Summary: "changed again",
	}, e.aMoveFor(e.fixture.Agent, e.fixture.CardInDoing))
	env, r := e.runWaitJSON(nil, "--board", e.fixture.Board.ID, "--column", "Doing")
	expectCode(t, r, ExitOK)
	if lines := linesOf(t, env); len(lines) != 1 || lines[0]["occasion"] != "card.moved" {
		t.Errorf("--column let the line about the board through: %v", lines)
	}

	e.aLineArrivesInTheWait(&fakeapi.Notification{
		PersonID: e.fixture.Agent.ID, Occasion: "board.conventionsChanged", ProjectID: e.fixture.Project.ID,
		SubjectType: "board", SubjectID: e.fixture.Board.ID, SubjectTitle: e.fixture.Board.Title, Summary: "and again",
	})
	env, r = e.runWaitJSON(nil, "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	found := false
	for _, crumb := range env.Breadcrumbs {
		found = found || crumb.Cmd == "minka boards conventions show "+e.fixture.Board.ID
	}
	if !found {
		t.Errorf("no way to read the conventions: %v", env.Breadcrumbs)
	}
}

// A role goes out as typed and --no-role as null; the two exclude each other
// and nothing goes out without a change.
func TestColumnsEditGivesARoleAndTakesItAway(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "columns", "edit", "Doing", "--board", e.fixture.Board.ID, "--role", "ready")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["role"] != "ready" {
		t.Errorf("body %v", body)
	}
	if !strings.HasSuffix(env.Summary, ", role ready") || decode(t, env.Data)["role"] != "ready" {
		t.Errorf("summary %q, data %s", env.Summary, env.Data)
	}
	env, r = e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--no-role")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["role"] != nil {
		t.Errorf("body %v", body)
	}
	if !strings.HasSuffix(env.Summary, ", role none") || e.fixture.Doing.Role != nil {
		t.Errorf("summary %q", env.Summary)
	}

	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--role", "ready", "--no-role")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Error("an edit refused here went out")
	}
}

// Minka's refusals of a role come with the way on, and nothing is written,
// the title sent along included.
func TestColumnsEditNamesTheRefusalsOfARole(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "edit", "Doing", "--board", e.fixture.Board.ID, "--role", "inbox", "--title", "Elsewhere")
	expectRefusal(t, env, r, "columnRoleUnknown", "ready, in_progress or waiting_for_decision")
	env, r = e.runJSON("", "columns", "edit", "done", "--board", e.fixture.Board.ID, "--no-role")
	expectRefusal(t, env, r, "systemColumnHasNoRole", "Triage carries inbox and Done done by type")
	if *e.fixture.Doing.CustomTitle != "Doing" {
		t.Errorf("the title was written: %s", *e.fixture.Doing.CustomTitle)
	}
}

// The safeguard: a Minka without roles that writes the title and answers 200
// does not pass for one that gave the role; one that reads the role alone as
// no change is named as such.
func TestColumnsEditDoesNotTakeAMinkaWithoutRolesForOneThatSetIt(t *testing.T) {
	e := newTestEnv(t)
	e.server.IgnoresColumnRoles = true

	env, r := e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--role", "ready", "--title", "Next")
	expectCode(t, r, ExitAPI)
	if !strings.Contains(env.Error, "did not give the column the role ready") || !strings.Contains(env.Hint, "does not know column roles") {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
	env, r = e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--role", "ready")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "nothingToChange" || !strings.Contains(env.Hint, "does not know column roles") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// The next card is the first in board order in a column with the role ready
// that is the account's own and not on hold; cards of others, on hold or in
// columns of another role are passed over.
func TestCardsNextTakesTheFirstReleasedCardOfYoursInBoardOrder(t *testing.T) {
	e := newAgentEnv(t)
	agent := e.fixture.Agent.ID

	env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.Contains(env.Notice, "no column with the role ready") {
		t.Errorf("no ready column: %s, notice %q", env.Data, env.Notice)
	}

	ready(e)
	second := e.fixture.Doing.Cards[1]
	e.fixture.CardInTriage.AssigneeIDs = []string{agent}
	e.fixture.CardInDoing.AssigneeIDs = []string{e.fixture.Bruno.ID}
	second.AssigneeIDs = []string{agent}
	env, r = e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["id"] != second.ID || env.Notice != "" {
		t.Errorf("next %s, notice %q", env.Data, env.Notice)
	}

	second.OnHold = true
	e.fixture.Doing.OnHoldEnabled = true
	env, r = e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.HasPrefix(env.Summary, "Nothing on API-3 Abnahme is released for you") {
		t.Errorf("on hold: %s, %q", env.Data, env.Summary)
	}
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Errorf("cards next wrote: %v", writes)
	}
}

// aMoveFor is a line that a card was moved into Doing, in somebody's bell.
func (e *testEnv) aMoveFor(person *fakeapi.Person, card *fakeapi.Card) *fakeapi.Notification {
	line := e.aMove(card)
	line.PersonID = person.ID
	return line
}

// The safeguard: the text view shows every format character of the
// conventions but the joiners as <U+XXXX> and says so, so that a bidi override
// or a tag character cannot make the text read otherwise than it says; --json
// carries the text as stored.
func TestTheConventionsShowInvisibleCharacters(t *testing.T) {
	e := newTestEnv(t)
	text := "Only Anke\u202e fills it\u200b. 👩\u200d💻 \U000e0041"
	expectCode(t, e.run(text, "boards", "conventions", "set", e.fixture.Board.ID, "-", "--json"), ExitOK)

	for _, args := range [][]string{
		{"boards", "conventions", "show", e.fixture.Board.ID},
		{"boards", "show", e.fixture.Board.ID},
		{"boards", "coordinator", "claim", e.fixture.Board.ID, "--session", "planning"},
	} {
		r := e.run("", append(args, "--styled")...)
		expectCode(t, r, ExitOK)
		if !strings.Contains(r.stdout, "Only Anke<U+202E> fills it<U+200B>. 👩\u200d💻 <U+E0041>") {
			t.Errorf("%s does not show the characters:\n%s", args[1], r.stdout)
		}
		if !strings.Contains(r.stderr, "3 invisible format characters") {
			t.Errorf("%s gives no notice: %q", args[1], r.stderr)
		}
	}
	env, _ := e.runJSON("", "boards", "conventions", "show", e.fixture.Board.ID)
	if decode(t, env.Data)["text"] != text {
		t.Errorf("--json changed the text: %s", env.Data)
	}
}

// Against a Minka without roles the board shows no role, and cards next says
// that Minka serves none instead of pointing at columns edit.
func TestReadingAgainstAMinkaWithoutRoles(t *testing.T) {
	e := newAgentEnv(t)
	e.server.IgnoresColumnRoles = true
	ready(e)

	r := e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "[role:") {
		t.Errorf("a role is shown:\n%s", r.stdout)
	}
	env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.Contains(env.Notice, "does not serve column roles yet") || strings.Contains(env.Notice, "columns edit") {
		t.Errorf("data %s, notice %q", env.Data, env.Notice)
	}
}
