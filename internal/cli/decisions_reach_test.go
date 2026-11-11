package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The audience goes out only where it is given, the people to tell by ID
// whatever named them, and Minka's default stays Minka's.
func TestTheAudienceOfADecisionGoesOutWhereItIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "We buy a tent", "--status", "proposed")
	body := lastWrite(t, e)
	for _, field := range []string{"audience", "notifyIds"} {
		if _, sent := body[field]; sent {
			t.Fatalf("%s went out without its flag: %v", field, body)
		}
	}

	runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "We buy a stage", "--status", "proposed",
		"--audience", "chosen", "--notify", f.Anke.Name, "--notify", "me")
	body = lastWrite(t, e)
	if body["audience"] != "chosen" || !slices.Equal(stringsOf(body["notifyIds"]), []string{f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("body %v", body)
	}
	decision := f.Decisions.Decisions[len(f.Decisions.Decisions)-1]
	if len(decision.Announcements) != 1 || decision.Announcements[0].Audience != "chosen" {
		t.Fatalf("announcements %+v", decision.Announcements)
	}

	runOK(t, e, "decisions", "status", decision.ShortID, "holds", "--audience", "nobody")
	if body := lastWrite(t, e); body["status"] != "holds" || body["audience"] != "nobody" {
		t.Fatalf("body %v", body)
	}
	runOK(t, e, "decisions", "supersede", decision.ShortID, "--title", "We rent a stage", "--audience", "everybody")
	if body := lastWrite(t, e); body["audience"] != "everybody" || body["supersedes"] != decision.ID {
		t.Fatalf("body %v", body)
	}
}

// An audience that is none, and an audience beside a change that names no
// status, which tells nobody, are refused before anything is sent.
func TestAnAudienceThatCannotTakeEffectIsRefusedBeforeAnythingIsSent(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, args := range [][]string{
		{"decisions", "create", "--tool", f.Decisions.ID, "--title", "T", "--audience", "all"},
		{"decisions", "status", f.ProposedDecision.ID, "holds", "--audience", "Everybody"},
		{"decisions", "edit", f.ProposedDecision.ID, "--title", "T", "--audience", "nobody"},
		{"decisions", "edit", f.ProposedDecision.ID, "--title", "T", "--notify", f.Anke.ID},
	} {
		before := len(e.server.Requests())
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if len(e.server.Requests()) != before {
			t.Errorf("%v: something was sent", args)
		}
		if !strings.Contains(env.Error, "--audience") {
			t.Errorf("%v: %q", args, env.Error)
		}
	}
	runOK(t, e, "decisions", "edit", f.ProposedDecision.ID, "--status", "holds", "--audience", "nobody")
}

// Every refusal of the audience says what to change.
func TestARefusedAudienceSaysWhatToChange(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, c := range []struct {
		args       []string
		code, hint string
	}{
		{[]string{"--audience", "chosen"}, apiCodeNotifyIDsMissing, "--notify"},
		{[]string{"--notify", f.Anke.ID}, apiCodeNotifyIDsWithoutChosenAudience, "--audience chosen"},
		{[]string{"--audience", "chosen", "--notify", f.Stranger.ID}, apiCodePersonNotAProjectMember, "--notify"},
		{[]string{"--audience", "chosen", "--notify", f.Client.ID}, apiCodePersonCannotSeeIt, "--visible-to-clients"},
	} {
		args := append([]string{"decisions", "create", "--tool", f.Decisions.ID, "--title", "T"}, c.args...)
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code || !strings.Contains(env.Hint, c.hint) {
			t.Errorf("%v: %v %q", c.args, env.Meta["apiCode"], env.Hint)
		}
	}
}

// A decision is named by its short identifier wherever it is named; the
// identifier is looked up in capitals, and the requests that follow name the
// ID.
func TestADecisionIsNamedByItsShortIdentifierEverywhere(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInDoing.ShortID = "MNK-9"
	withTwoDecisionEntries(e)
	entry := f.Decision.History[0].ID
	id := f.Decision.ID

	for _, c := range []struct {
		args []string
		path string // of the last request
	}{
		{[]string{"decisions", "show", "dec-2"}, "/api/v1/decisions/by-short-id/DEC-2"},
		{[]string{"decisions", "edit", "dec-2", "--votes", "5 : 1"}, "/api/v1/decisions/" + id},
		{[]string{"decisions", "versions", "list", "DEC-2"}, "/api/v1/decisions/" + id + "/versions"},
		{[]string{"decisions", "versions", "show", "DEC-2", f.Decision.Versions[0].ID}, "/api/v1/decisions/" + id + "/versions/" + f.Decision.Versions[0].ID},
		{[]string{"decisions", "history", "add", "DEC-2", "#DEC-1"}, "/api/v1/decisions/" + id + "/history"},
		{[]string{"decisions", "consequences", "add", "DEC-2", "#" + f.CardInDoing.ShortID}, "/api/v1/decisions/" + id + "/consequences"},
		{[]string{"decisions", "entries", "move", "DEC-2", entry, "--bottom"}, "/api/v1/decisions/" + id + "/entries/" + entry + "/moves"},
		{[]string{"decisions", "entries", "delete", "DEC-2", entry}, "/api/v1/decisions/" + id + "/entries/" + entry},
		{[]string{"decisions", "comments", "list", "DEC-2"}, "/api/v1/decisions/" + id + "/comments"},
		{[]string{"decisions", "comments", "create", "DEC-2", "Agreed."}, "/api/v1/decisions/" + id + "/comments"},
		{[]string{"decisions", "status", "DEC-2", "holds"}, "/api/v1/decisions/" + id},
		{[]string{"decisions", "create", "--tool", f.Decisions.ID, "--title", "Fridays", "--revokes", "dec-3", "--status", "proposed"}, "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"},
		{[]string{"decisions", "delete", "DEC-4"}, "/api/v1/decisions/" + f.DraftDecision.ID},
	} {
		runOK(t, e, c.args...)
		requests := e.server.Requests()
		if last := requests[len(requests)-1]; last.Path != c.path {
			t.Errorf("%v: %s %s, want %s", c.args, last.Method, last.Path, c.path)
		}
	}
	if body := lastWriteBefore(t, e, "/api/v1/decisions-tools/"); body["revokes"] != f.ProposedDecision.ID {
		t.Errorf("--revokes went out as %v", body["revokes"])
	}
	runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "Saturdays", "--supersedes", "dec-2", "--status", "proposed")
	if body := lastWrite(t, e); body["supersedes"] != f.Decision.ID {
		t.Errorf("--supersedes of a new decision went out as %v", body["supersedes"])
	}
	runOK(t, e, "decisions", "edit", "DEC-3", "--supersedes", "DEC-2")
	if body := lastWrite(t, e); body["supersedes"] != f.Decision.ID {
		t.Errorf("--supersedes of a change went out as %v", body["supersedes"])
	}
	history := f.Decision.History
	if len(history) == 0 || history[len(history)-1].Kind != "decision" {
		t.Errorf("#DEC-1 was not passed through to Minka: %+v", history)
	}

	env, r := e.runJSON("", "decisions", "show", "DEC-99")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "no decision you can see goes by DEC-99") {
		t.Errorf("error %q", env.Error)
	}
	before := len(e.server.Requests())
	_, r = e.runJSON("", "decisions", "show", "Mondays")
	expectCode(t, r, ExitUsage)
	_, r = e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--title", "T", "--supersedes", "Mondays")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Error("what names no decision was sent")
	}
}

// The comments and the labels of a decision named by its short identifier go
// to the decision's ID, in every command that writes them.
func TestTheCommentsAndLabelsOfADecisionTakeItsShortIdentifier(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withLabels(e)
	id := f.Decision.ID
	comment := decode(t, runOK(t, e, "decisions", "comments", "create", "DEC-2", "Agreed.").Data)["id"].(string)

	for _, c := range []struct {
		args []string
		path string
	}{
		{[]string{"decisions", "comments", "edit", "dec-2", comment, "Agreed, Monday."}, "/api/v1/decisions/" + id + "/comments/" + comment},
		{[]string{"decisions", "comments", "react", "DEC-2", comment, "👍"}, "/api/v1/decisions/" + id + "/comments/" + comment + "/reactions"},
		{[]string{"decisions", "comments", "unreact", "DEC-2", comment, "👍"}, "/api/v1/decisions/" + id + "/comments/" + comment + "/reactions"},
		{[]string{"decisions", "comments", "delete", "DEC-2", comment}, "/api/v1/decisions/" + id + "/comments/" + comment},
		{[]string{"decisions", "label", "dec-2", "Plain"}, "/api/v1/decisions/" + id + "/labels"},
		{[]string{"decisions", "unlabel", "DEC-2", "Plain"}, "/api/v1/decisions/" + id + "/labels"},
	} {
		runOK(t, e, c.args...)
		writes := e.server.WritingRequests()
		if last := writes[len(writes)-1]; last.Path != c.path {
			t.Errorf("%v: %s %s, want %s", c.args, last.Method, last.Path, c.path)
		}
	}
}

// A decision named by its short identifier is read once: the lookup gives
// the project whose members are told and the body the guard holds against.
func TestADecisionNamedByItsShortIdentifierIsReadOnce(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	withTwoDecisionEntries(e)
	for _, args := range [][]string{
		{"decisions", "entries", "move", "DEC-2", f.Decision.History[0].ID, "--bottom"},
		{"decisions", "edit", "DEC-3", "--status", "holds", "--audience", "chosen", "--notify", f.Anke.Name, "--body", "Green, with **poles**."},
		{"decisions", "status", "DEC-3", "proposed", "--audience", "chosen", "--notify", f.Anke.Name},
	} {
		before := len(e.server.Requests())
		runOK(t, e, args...)
		reads := 0
		for _, request := range e.server.Requests()[before:] {
			if request.Method == "GET" && strings.HasPrefix(request.Path, "/api/v1/decisions/") {
				reads++
			}
		}
		if reads != 1 {
			t.Errorf("%v: %d reads of the decision, want 1", args, reads)
		}
		if !slices.Contains(args, "--notify") {
			continue
		}
		if body := lastWrite(t, e); !slices.Equal(stringsOf(body["notifyIds"]), []string{f.Anke.ID}) {
			t.Errorf("%v: body %v", args, body)
		}
	}
}

// An answer to a short identifier that carries no ID stops the command
// before anything is written.
func TestADecisionAnsweredWithoutAnIDStopsTheCommand(t *testing.T) {
	e := newTestEnv(t)
	e.server.AnswersDecisionsWithoutID = true

	env, r := e.runJSON("", "decisions", "comments", "create", "DEC-2", "Agreed.")
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || len(e.server.WritingRequests()) != 0 {
		t.Fatalf("code %q, %d writes", env.Code, len(e.server.WritingRequests()))
	}
}

// lastWriteBefore is the body of the last write to a path beginning with
// prefix.
func lastWriteBefore(t *testing.T, e *testEnv, prefix string) map[string]any {
	t.Helper()
	writes := e.server.WritingRequests()
	for i := len(writes) - 1; i >= 0; i-- {
		if strings.HasPrefix(writes[i].Path, prefix) {
			var body map[string]any
			if err := json.Unmarshal(writes[i].Body, &body); err != nil {
				t.Fatalf("body %s: %v", writes[i].Body, err)
			}
			return body
		}
	}
	t.Fatalf("nothing was written to %s", prefix)
	return nil
}

// A short identifier of an earlier decision in a list of new ones is looked
// up before the first write, so that one nobody can find writes nothing.
func TestAnEarlierDecisionOfAListIsLookedUpBeforeAnythingIsWritten(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	file := filepath.Join(t.TempDir(), "decisions.json")
	write := func(content string) {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(`[{"title":"Tuesdays","supersedes":"dec-2","audience":"chosen","notifyIds":["` + strings.ToUpper(f.Anke.ID) + `"]}]`)
	runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--from", file)
	if body := lastWrite(t, e); body["supersedes"] != f.Decision.ID || !slices.Equal(stringsOf(body["notifyIds"]), []string{f.Anke.ID}) {
		t.Fatalf("body %v", body)
	}

	writes := len(e.server.WritingRequests())
	write(`[{"title":"One"},{"title":"Two","revokes":"DEC-99"}]`)
	_, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--from", file)
	expectCode(t, r, ExitNotFound)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a list naming an earlier decision nobody can find wrote something")
	}
}

// The text leads with the short identifier, names a decision by its short
// address and says who set its status.
func TestTheTextNamesADecisionByItsShortIdentifierAndWhoSetItsStatus(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.ArchivedDecision.ShortID = ""

	text := e.run("", "decisions", "list", f.Decisions.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{
		"- DEC-2  " + f.Decision.ID + "  We meet on Mondays (In force, decided 2026-11-02)",
		"- DEC-3  " + f.ProposedDecision.ID + "  We buy a tent (Proposed, set by an agent)",
		"- DEC-4  " + f.DraftDecision.ID + "  The fee rises to 40 euros (Draft)",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("the list lacks %q:\n%s", want, text.stdout)
		}
	}

	text = e.run("", "decisions", "show", f.ProposedDecision.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"# We buy a tent\nShort ID: DEC-3\n", "Status set: ", "by " + f.Agent.Name + " (" + f.Agent.ID + "), an agent",
		"URL: " + e.server.URL + "/go/decision/DEC-3"} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("the decision lacks %q:\n%s", want, text.stdout)
		}
	}

	f.Decision.StatusSetBy.Minka = true
	text = e.run("", "decisions", "list", f.Decisions.ID, "--styled")
	if !strings.Contains(text.stdout, "We meet on Mondays (In force, decided 2026-11-02, set by Minka)") {
		t.Errorf("the list does not say Minka set the status:\n%s", text.stdout)
	}
	text = e.run("", "decisions", "show", "DEC-2", "--styled")
	if !strings.Contains(text.stdout, "by Minka, on behalf of "+f.Bruno.Name) {
		t.Errorf("Minka's move is not said:\n%s", text.stdout)
	}

	f.ArchivedDecision.Archived = false
	text = e.run("", "decisions", "show", f.ArchivedDecision.ID, "--styled")
	if strings.Contains(text.stdout, "Short ID") || !strings.Contains(text.stdout, "URL: "+e.server.URL+"/projects/") {
		t.Errorf("a decision without a number:\n%s", text.stdout)
	}
}

// --set-by-agent asks Minka for the decisions whose status an agent set.
func TestTheListNarrowsToTheStatusesAnAgentSet(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "decisions", "list", f.Decisions.ID, "--set-by-agent")
	if e.lastRequestQuery() != "setBy=agent" {
		t.Fatalf("query %q", e.lastRequestQuery())
	}
	lines := listOfObjects(t, env.Data)
	if len(lines) != 1 || lines[0]["id"] != f.ProposedDecision.ID {
		t.Fatalf("lines %v", lines)
	}
	runOK(t, e, "decisions", "list", f.Decisions.ID)
	if e.lastRequestQuery() != "" {
		t.Fatalf("query %q without the flag", e.lastRequestQuery())
	}
}
