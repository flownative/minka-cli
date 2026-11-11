package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writtenTo are the paths of the writes since the request numbered before.
func writtenTo(e *testEnv, before int) []string {
	var paths []string
	for _, request := range e.server.Requests()[before:] {
		if request.Method != http.MethodGet {
			paths = append(paths, request.Method+" "+request.Path)
		}
	}
	return paths
}

// Several sentences go out in one request with the fields they share, and
// come back in their order, each with its place on the agenda.
func TestSeveralTitlesAreWrittenDownAtOnceWithWhatTheyShare(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	label := e.server.AddLabel(f.Project.ID, "", "Budget", "red")
	before := len(e.server.Requests())

	env := runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "The budget passes", "--title", "The grant is approved",
		"--title", "December is off", "--decided-on", "2026-11-10", "--decided-by", "Council", "--status", "proposed", "--label", "budget")
	if writes := writtenTo(e, before); !slices.Equal(writes, []string{"POST /api/v1/decisions-tools/" + f.Decisions.ID + "/decisions/batch"}) {
		t.Fatalf("writes %v, want one to the batch", writes)
	}
	body := lastWrite(t, e)
	titles, _ := json.Marshal(body["titles"])
	labelIDs, _ := json.Marshal(body["labelIds"])
	if string(titles) != `["The budget passes","The grant is approved","December is off"]` || string(labelIDs) != `["`+label.ID+`"]` || body["decidedOn"] != "2026-11-10" || body["status"] != "proposed" {
		t.Fatalf("body %v", body)
	}
	if _, sent := body["title"]; sent {
		t.Fatal("a title went out beside the titles")
	}
	written := listOfObjects(t, env.Data)
	if len(written) != 3 || written[0]["agendaNumber"] != 1.0 || written[2]["agendaNumber"] != 3.0 || written[1]["title"] != "The grant is approved" {
		t.Fatalf("written %v", written)
	}

	text := e.run("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "One", "--title", "Two", "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Wrote 2 decisions down in Decisions:\n1. ") || !strings.Contains(text.stdout, "  Two\n") {
		t.Fatalf("text:\n%s", text.stdout)
	}
}

// A line of --titles-from is a sentence, an empty line none; one sentence
// is a single decision, which takes every flag.
func TestTitlesFromAFileLeaveOutEmptyLines(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	file := filepath.Join(t.TempDir(), "agenda.txt")
	if err := os.WriteFile(file, []byte("1. One\n\n   \nTwo\r\nThree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--titles-from", file)
	if titles, _ := json.Marshal(lastWrite(t, e)["titles"]); string(titles) != `["1. One","Two","Three"]` {
		t.Fatalf("titles %s", titles)
	}

	before := len(e.server.Requests())
	env, r := e.runJSON("\n  \nOnly one\n", "decisions", "create", "--tool", f.Decisions.ID, "--titles-from", "-", "--reason", "Alone it takes a reason.")
	expectCode(t, r, ExitOK)
	if writes := writtenTo(e, before); !slices.Equal(writes, []string{"POST /api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"}) {
		t.Fatalf("writes %v, want the single create", writes)
	}
	if decision := decode(t, env.Data); decision["title"] != "Only one" || decision["reason"] != "Alone it takes a reason." || decision["agendaNumber"] != nil {
		t.Fatalf("decision %v", decision)
	}

	before = len(e.server.Requests())
	for stdin, args := range map[string][]string{
		"\n \n": {"--titles-from", "-"},
		"One\n": {"--titles-from", "-", "--title", "Two"},
		"":      {"--reason", "No sentence"},
	} {
		_, r := e.runJSON(stdin, append([]string{"decisions", "create", "--tool", f.Decisions.ID}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

// Beside several sentences every flag that is not shared is refused before
// anything is sent.
func TestAFlagSeveralDecisionsDoNotShareIsRefusedBeforeAnythingIsSent(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	before := len(e.server.Requests())

	for _, flag := range [][]string{
		{"--reason", "Why"}, {"--body", "More"}, {"--body-format", "html"}, {"--votes", "5 : 1"}, {"--supersedes", f.Decision.ID},
		{"--revokes", f.Decision.ID}, {"--valid-until", "2027-01-01"}, {"--review-on", "2027-01-01"},
	} {
		env, r := e.runJSON("", append([]string{"decisions", "create", "--tool", f.Decisions.ID, "--title", "One", "--title", "Two"}, flag...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, flag[0]) {
			t.Errorf("%s: the error does not name it: %s", flag[0], env.Error)
		}
	}
	_, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "One", "--label", "Plain", "--from", "-")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

// A sentence Minka refuses refuses them all, and comes back with where it
// was given.
func TestRefusedSentencesComeBackWithWhereTheyWereGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	file := filepath.Join(t.TempDir(), "agenda.txt")
	if err := os.WriteFile(file, []byte("Good\n\nTab\there\n"+strings.Repeat("x", 201)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := len(f.Decisions.Decisions)

	env, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--titles-from", file)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionLinesRefused" || !strings.Contains(env.Error, "line 3 of "+file+": decisionTitleHasControlCharacters") {
		t.Fatalf("error %q, meta %v", env.Error, env.Meta)
	}
	lines := listOfObjects(t, rawOf(t, decode(t, env.Data), "lines"))
	if len(lines) != 2 || lines[0]["index"] != 1.0 || lines[0]["title"] != "Tab\there" || lines[0]["from"] != "line 3 of "+file ||
		lines[1]["index"] != 2.0 || lines[1]["code"] != "decisionTitleTooLong" || lines[1]["from"] != "line 4 of "+file {
		t.Fatalf("lines %v", lines)
	}

	env, r = e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "Good", "--title", " ")
	expectCode(t, r, ExitValidation)
	if lines := listOfObjects(t, rawOf(t, decode(t, env.Data), "lines")); len(lines) != 1 || lines[0]["from"] != "--title 2" || lines[0]["code"] != "decisionTitleMissing" {
		t.Fatalf("lines %v", lines)
	}
	if len(f.Decisions.Decisions) != before {
		t.Fatal("a refused batch wrote decisions")
	}
}

// The labels of new decisions are looked up among the project's before
// anything is written; one decision takes them right after it is written.
func TestTheLabelsOfNewDecisionsAreLookedUpBeforeAnythingIsWritten(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	plain := e.server.AddLabel(f.Project.ID, "", "Plain", "gray")
	high := e.server.AddLabel(f.Project.ID, "", "Priority::High", "red")
	e.server.AddLabel(f.Project.ID, "", "Priority::Low", "blue")
	boardLabel := e.server.AddLabel(f.Project.ID, f.Board.ID, "Board only", "red")

	before := len(e.server.Requests())
	env := runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "We buy a tent", "--label", "plain", "--label", up(high.ID), "--label", "Plain")
	decision := decode(t, env.Data)
	id, _ := decision["id"].(string)
	want := []string{"POST /api/v1/decisions-tools/" + f.Decisions.ID + "/decisions", "POST /api/v1/decisions/" + id + "/labels", "POST /api/v1/decisions/" + id + "/labels"}
	if writes := writtenTo(e, before); !slices.Equal(writes, want) {
		t.Fatalf("writes %v, want %v", writes, want)
	}
	var names []string
	for _, label := range listOfObjects(t, rawOf(t, decision, "labels")) {
		names = append(names, label["name"].(string))
	}
	if !slices.Equal(names, []string{plain.Name, high.Name}) {
		t.Fatalf("labels %v", names)
	}

	before = len(e.server.Requests())
	for _, c := range []struct {
		labels []string
		code   int
	}{
		{[]string{"Nowhere"}, ExitNotFound},
		{[]string{boardLabel.ID}, ExitNotFound},
		{[]string{"Priority::High", "priority :: low"}, ExitValidation},
	} {
		for _, titles := range [][]string{{"--title", "One"}, {"--title", "One", "--title", "Two"}} {
			args := append([]string{"decisions", "create", "--tool", f.Decisions.ID}, titles...)
			for _, label := range c.labels {
				args = append(args, "--label", label)
			}
			_, r := e.runJSON("", args...)
			expectCode(t, r, c.code)
		}
	}
	if writes := writtenTo(e, before); len(writes) != 0 {
		t.Fatalf("written for labels that cannot go on: %v", writes)
	}
}

// A label that fails to go on after one decision was written hands over the
// decision and the labels put on and not.
func TestALabelThatFailsAfterTheDecisionSaysWhatWasWritten(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.AddLabel(f.Project.ID, "", "Plain", "gray")
	e.server.AddLabel(f.Project.ID, "", "Other", "red")
	e.server.FailNextWriteWith("/labels", http.StatusBadGateway)

	env, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "We buy a tent", "--label", "Plain", "--label", "Other")
	if r.code == ExitOK {
		t.Fatal("a failed label went unreported")
	}
	data := decode(t, env.Data)
	if decode(t, rawOf(t, data, "created"))["title"] != "We buy a tent" || !strings.Contains(env.Error, "wrote decision") {
		t.Fatalf("error %q, data %v", env.Error, data)
	}
	if labelled, _ := json.Marshal(data["labelled"]); string(labelled) != `[]` {
		t.Fatalf("labelled %s", labelled)
	}
	if notLabelled, _ := json.Marshal(data["notLabelled"]); string(notLabelled) != `["Plain","Other"]` {
		t.Fatalf("notLabelled %s", notLabelled)
	}
}

// Two flags that read stdin are refused before anything is read or sent.
func TestTwoFlagsReadingStdinAreRefused(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	before := len(e.server.Requests())

	for _, args := range [][]string{
		{"--titles-from", "-", "--body", "-"},
		{"--from", "-", "--body", "-"},
	} {
		env, r := e.runJSON("Only one\n", append([]string{"decisions", "create", "--tool", f.Decisions.ID}, args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "--body") {
			t.Errorf("%v: %s", args, env.Error)
		}
	}
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

// Two labels of one scope refused by the CLI carry the code Minka refuses
// them with.
func TestLabelsOfOneScopeRefusedBeforehandCarryMinkasCode(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.AddLabel(f.Project.ID, "", "Priority::High", "red")
	e.server.AddLabel(f.Project.ID, "", "Priority::Low", "blue")

	env, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "One", "--label", "Priority::High", "--label", "Priority::Low")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelsShareAScope" {
		t.Fatalf("meta %v", env.Meta)
	}
}

// The place on the agenda shows in the list and in a decision.
func TestTheAgendaNumberIsShown(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env := runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "First", "--title", "Second")
	second := listOfObjects(t, env.Data)[1]["id"].(string)

	text := e.run("", "decisions", "show", second, "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Agenda item: 2 of the decisions written down with it") {
		t.Fatalf("show:\n%s", text.stdout)
	}
	text = e.run("", "decisions", "list", f.Decisions.ID, "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Second (In force, agenda item 2)") || strings.Contains(text.stdout, "Mondays (In force, decided 2026-11-02, agenda") {
		t.Fatalf("list:\n%s", text.stdout)
	}
}
