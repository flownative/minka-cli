package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// withTwoDecisionEntries puts two links into the history of the decision in
// force.
func withTwoDecisionEntries(e *testEnv) {
	for _, link := range []string{"https://example.org/one", "https://example.org/two"} {
		runOK(e.t, e, "decisions", "history", "add", e.fixture.Decision.ID, link, "--description", "A link")
	}
}

func asTheClient(e *testEnv) {
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"
}

// lastWrite is the body of the last request that was not a read.
func lastWrite(t *testing.T, e *testEnv) map[string]any {
	t.Helper()
	writes := e.server.WritingRequests()
	if len(writes) == 0 {
		t.Fatal("nothing was written")
	}
	var body map[string]any
	if err := json.Unmarshal(writes[len(writes)-1].Body, &body); err != nil {
		t.Fatalf("body %s: %v", writes[len(writes)-1].Body, err)
	}
	return body
}

func TestTheDecisionsOfAToolAreListedTheLastDecidedFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	tools := listOfObjects(t, runOK(t, e, "decisions", "tools", f.Project.ID).Data)
	if len(tools) != 1 || tools[0]["id"] != f.Decisions.ID {
		t.Fatalf("tools %v", tools)
	}
	statuses := listOfObjects(t, runOK(t, e, "decisions", "statuses", f.Decisions.ID).Data)
	if len(statuses) != 7 || statuses[2]["role"] != "holds" || statuses[2]["name"] != "In force" {
		t.Fatalf("statuses %v", statuses)
	}

	env := runOK(t, e, "decisions", "list", f.Decisions.ID)
	var ids []string
	for _, line := range listOfObjects(t, env.Data) {
		ids = append(ids, line["id"].(string))
	}
	want := []string{f.DraftDecision.ID, f.ProposedDecision.ID, f.Decision.ID}
	if !slices.Equal(ids, want) {
		t.Fatalf("decisions %v, want %v", ids, want)
	}
	if e.lastRequestQuery() != "" {
		t.Fatalf("query %q: every status is listed unless one is named", e.lastRequestQuery())
	}
	// A decision of the archive is listed nowhere and still read.
	runOK(t, e, "decisions", "show", f.ArchivedDecision.ID)

	text := e.run("", "decisions", "list", f.Decisions.ID, "--holds", "--styled")
	expectCode(t, text, ExitOK)
	if e.lastRequestQuery() != "status=holds" {
		t.Fatalf("query %q, want status=holds", e.lastRequestQuery())
	}
	for _, want := range []string{f.Decision.ID + "  We meet on Mondays (In force, decided 2026-11-02)", "  Then everybody has time."} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
	if strings.Contains(text.stdout, "We buy a tent") {
		t.Fatalf("a proposed decision is listed as in force:\n%s", text.stdout)
	}

	_, r := e.runJSON("", "decisions", "list", f.OtherDecisions.ID)
	expectCode(t, r, ExitNotFound)
}

// Minka compares a role exactly as it spells it, so a role goes out as it was
// typed and an ID in lowercase.
func TestARoleGoesOutAsTypedAndAnIDOfAStatusInLowercase(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "decisions", "list", f.Decisions.ID, "--status", "Holds")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeDecisionStatusUnknown || !strings.Contains(env.Hint, "holds") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if e.lastRequestQuery() != "status=Holds" {
		t.Fatalf("query %q: the role is to go out as typed", e.lastRequestQuery())
	}

	runOK(t, e, "decisions", "status", f.ProposedDecision.ID, up(f.Decisions.StatusOfRole("rejected").ID))
	if status := lastWrite(t, e)["status"]; status != f.Decisions.StatusOfRole("rejected").ID {
		t.Fatalf("status %v went out", status)
	}
	if f.ProposedDecision.StatusID != f.Decisions.StatusOfRole("rejected").ID {
		t.Fatal("the decision did not move")
	}
}

func TestAClientMeetsNoDraftAndNoVersion(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	asTheClient(e)

	env := runOK(t, e, "decisions", "list", f.Decisions.ID)
	for _, line := range listOfObjects(t, env.Data) {
		if line["id"] != f.Decision.ID {
			t.Fatalf("a client is shown %v", line["title"])
		}
	}
	for _, args := range [][]string{
		{"decisions", "show", f.DraftDecision.ID},
		{"decisions", "versions", "list", f.Decision.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
	}
	statuses := listOfObjects(t, runOK(t, e, "decisions", "statuses", f.Decisions.ID).Data)
	if slices.ContainsFunc(statuses, func(status map[string]any) bool { return status["role"] == "draft" }) {
		t.Fatal("a client is offered draft")
	}
}

// A label is filtered by its ID, so a name is looked up among the labels of
// the project before the list is read, and one found nowhere reads no list.
func TestALabelNamedForTheListIsLookedUpInTheProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	label := e.server.AddLabel(f.Project.ID, "", "Board", "red")
	e.server.PutLabelOn(f.ProposedDecision.ID, label)

	env := runOK(t, e, "decisions", "list", f.Decisions.ID, "--label", "board")
	lines := listOfObjects(t, env.Data)
	if len(lines) != 1 || lines[0]["id"] != f.ProposedDecision.ID || e.lastRequestQuery() != "label="+label.ID {
		t.Fatalf("lines %v, query %q", lines, e.lastRequestQuery())
	}

	before := len(e.server.Requests())
	_, r := e.runJSON("", "decisions", "list", f.Decisions.ID, "--label", "Nowhere")
	expectCode(t, r, ExitNotFound)
	for _, request := range e.server.Requests()[before:] {
		if strings.HasSuffix(request.Path, "/decisions") {
			t.Fatal("the list was read for a label that is none")
		}
	}
}

func TestADecisionAppliesToACardNamedByItsShortIdentifier(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInDoing.ShortID = "MNK-9"

	env := runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "The stage stands left", "--applies-to-card", "mnk-9")
	subject := decode(t, rawOf(t, decode(t, env.Data), "appliesTo"))
	if subject["kind"] != "card" || subject["id"] != f.CardInDoing.ID {
		t.Fatalf("appliesTo %v", subject)
	}
	runOK(t, e, "decisions", "list", f.Decisions.ID, "--applies-to-card", "MNK-9")
	if e.lastRequestQuery() != "appliesTo="+f.CardInDoing.ID {
		t.Fatalf("query %q", e.lastRequestQuery())
	}

	_, r := e.runJSON("", "decisions", "list", f.Decisions.ID, "--applies-to-card", "MNK-9", "--applies-to-event", f.OnceEvent.ID)
	expectCode(t, r, ExitUsage)
}

func TestADecisionIsShownWithWhatHoldsAndWhy(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	text := e.run("", "decisions", "show", f.Decision.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"# We meet on Mondays", "Status: In force (holds)", "Reason: Then everybody has time.",
		"Decided on: 2026-11-02", "## Body\n\nFrom **December** on.", "Fine by me"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
	_, r := e.runJSON("", "decisions", "show", f.OtherDecision.ID)
	expectCode(t, r, ExitNotFound)
}

func TestANewDecisionSendsTheFieldsGivenAndHoldsWithoutAStatus(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "decisions", "create", "--tool", f.Decisions.ID, "--title", "We buy benches", "--reason", "The old ones broke.",
		"--body", "Twenty, **wooden**.", "--decided-by", "me", "--decided-by", f.Anke.ID, "--decided-by", "Board",
		"--event", f.OnceEvent.ID, "--votes", "5 : 1", "--review-on", "2027-03-01")
	body := lastWrite(t, e)
	if body["body"] != "<p>Twenty, <strong>wooden</strong>.</p>" || body["decidedInEventId"] != f.OnceEvent.ID || body["reviewOn"] != "2027-03-01" {
		t.Fatalf("body %v", body)
	}
	if _, sent := body["status"]; sent {
		t.Fatal("a status went out that was not given")
	}
	deciders, _ := json.Marshal(body["decidedBy"])
	if string(deciders) != `[{"personId":"`+f.Bruno.ID+`"},{"personId":"`+f.Anke.ID+`"},{"name":"Board"}]` {
		t.Fatalf("decidedBy %s", deciders)
	}
	decision := decode(t, env.Data)
	if decode(t, rawOf(t, decision, "status"))["role"] != "holds" || decision["decidedOn"] != "2026-10-21" {
		t.Fatalf("status %v, decidedOn %v: want in force on the day of the event", decision["status"], decision["decidedOn"])
	}

	before := len(e.server.Requests())
	_, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--reason", "No sentence")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Fatal("a decision without a sentence was sent")
	}
}

// Several decisions are written one after another; the first refusal stops
// the run and hands over what was written before it.
func TestDecisionsFromAFileStopAtTheFirstRefusal(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	file := filepath.Join(t.TempDir(), "decisions.json")
	list := `[{"title": "We meet at seven", "body": "In the **hall**."}, {"title": "", "reason": "No sentence"}, {"title": "Never written"}]`
	if err := os.WriteFile(file, []byte(list), 0o600); err != nil {
		t.Fatal(err)
	}
	before := len(f.Decisions.Decisions)

	env, r := e.runJSON("", "decisions", "create", "--tool", f.Decisions.ID, "--from", file)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "decisionTitleMissing" || !strings.Contains(env.Error, "decision 2") {
		t.Fatalf("error %q, apiCode %v", env.Error, env.Meta["apiCode"])
	}
	data := decode(t, env.Data)
	created := listOfObjects(t, rawOf(t, data, "created"))
	if len(created) != 1 || created[0]["title"] != "We meet at seven" || created[0]["body"] != "<p>In the <strong>hall</strong>.</p>" {
		t.Fatalf("created %v", created)
	}
	if notCreated, _ := json.Marshal(data["notCreated"]); string(notCreated) != `["","Never written"]` {
		t.Fatalf("notCreated %s", notCreated)
	}
	if len(f.Decisions.Decisions) != before+1 {
		t.Fatalf("%d decisions written, want one", len(f.Decisions.Decisions)-before)
	}
}

func TestDecisionsFromStdinAreWrittenInTheirOrder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON(`[{"title": "One"}, {"title": "Two", "status": "proposed", "decidedBy": [{"personId": "`+up(f.Anke.ID)+`"}]}]`,
		"decisions", "create", "--tool", f.Decisions.ID, "--from", "-")
	expectCode(t, r, ExitOK)
	created := listOfObjects(t, env.Data)
	if len(created) != 2 || created[0]["title"] != "One" || created[1]["title"] != "Two" {
		t.Fatalf("created %v", created)
	}
	if strings.Contains(string(e.server.WritingRequests()[1].Body), up(f.Anke.ID)) {
		t.Fatal("an ID of the list went out in capitals")
	}
}

func TestAFieldMinkaDoesNotTakeIsRefusedBeforeAnythingIsSent(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	before := len(e.server.Requests())

	for stdin, args := range map[string][]string{
		`[{"title": "One"}, {"title": "Two", "decided": "yesterday"}]`: {"decisions", "create", "--tool", f.Decisions.ID, "--from", "-"},
		`[{"title": "One"}]`: {"decisions", "create", "--tool", f.Decisions.ID, "--from", "-", "--status", "proposed"},
	} {
		_, r := e.runJSON(stdin, args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

func TestAChangeSendsOnlyWhatIsGivenAndNullForWhatIsCleared(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	runOK(t, e, "decisions", "edit", f.ProposedDecision.ID, "--supersedes", f.Decision.ID, "--votes", "3 : 0")

	runOK(t, e, "decisions", "edit", f.ProposedDecision.ID, "--no-reason", "--no-earlier", "--no-votes", "--no-applies-to")
	body := lastWrite(t, e)
	for _, field := range []string{"reason", "supersedes", "revokes", "votes", "appliesTo"} {
		if value, sent := body[field]; !sent || value != nil {
			t.Errorf("%s: %v, sent %v; want null", field, value, sent)
		}
	}
	if len(body) != 5 {
		t.Fatalf("body %v names more than was given", body)
	}
	if f.ProposedDecision.EarlierID != "" || f.ProposedDecision.Votes != "" {
		t.Fatal("the decision keeps what was cleared")
	}

	for _, args := range [][]string{
		{"decisions", "edit", f.ProposedDecision.ID},
		{"decisions", "edit", f.ProposedDecision.ID, "--reason", "Why", "--no-reason"},
		{"decisions", "edit", f.ProposedDecision.ID, "--supersedes", f.Decision.ID, "--revokes", f.Decision.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
}

// The earlier decision moves only while the new one holds, and the answer
// says where it stays.
func TestTheEarlierDecisionMovesOnlyWhileTheNewOneHolds(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "decisions", "supersede", f.Decision.ID, "--title", "We meet on Tuesdays", "--status", "proposed")
	if lastWrite(t, e)["supersedes"] != f.Decision.ID || !strings.Contains(env.Notice, "stays In force") {
		t.Fatalf("body %v, notice %q", lastWrite(t, e), env.Notice)
	}
	if f.Decision.StatusID != f.Decisions.StatusOfRole("holds").ID {
		t.Fatal("the earlier decision moved before the new one holds")
	}
	later := decode(t, env.Data)["id"].(string)

	env = runOK(t, e, "decisions", "status", later, "holds")
	if f.Decision.StatusID != f.Decisions.StatusOfRole("superseded").ID || env.Notice != "" {
		t.Fatalf("status %s, notice %q", f.Decision.StatusID, env.Notice)
	}

	text := e.run("", "decisions", "revoke", f.ProposedDecision.ID, "--title", "No tent", "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Revoked: We buy a tent, moved into Revoked") {
		t.Fatalf("the text does not say where the earlier one went:\n%s", text.stdout)
	}
}

func TestADecisionGoesIntoTheTrash(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	runOK(t, e, "decisions", "delete", f.ProposedDecision.ID)
	_, r := e.runJSON("", "decisions", "show", f.ProposedDecision.ID)
	expectCode(t, r, ExitNotFound)

	asTheClient(e)
	env, r := e.runJSON("", "decisions", "delete", f.Decision.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != decisionNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestTheVersionsOfADecisionAreRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	runOK(t, e, "decisions", "edit", f.Decision.ID, "--reason", "The hall is free then.")
	runOK(t, e, "decisions", "status", f.Decision.ID, "done")

	versions := listOfObjects(t, runOK(t, e, "decisions", "versions", "list", f.Decision.ID).Data)
	if len(versions) != 2 || versions[1]["current"] != true {
		t.Fatalf("versions %v: a status makes no version", versions)
	}
	first := decode(t, runOK(t, e, "decisions", "versions", "show", f.Decision.ID, versions[0]["id"].(string)).Data)
	if first["reason"] != "Then everybody has time." {
		t.Fatalf("first version %v", first)
	}
}

func TestTheHistoryAndTheConsequencesOfADecision(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInDoing.ShortID = "MNK-9"

	env, r := e.runJSON("", "decisions", "history", "add", f.Decision.ID, "https://example.org/halls")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeLinkDescriptionMissing || !strings.Contains(env.Hint, "--description") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	withTwoDecisionEntries(e)
	card := decode(t, runOK(t, e, "decisions", "consequences", "add", f.Decision.ID, "#MNK-9").Data)
	if card["kind"] != "card" || card["done"] != false {
		t.Fatalf("consequence %v", card)
	}
	_, r = e.runJSON("", "decisions", "consequences", "add", f.Decision.ID, "https://example.org/halls")
	expectCode(t, r, ExitValidation)

	first, second := f.Decision.History[0].ID, f.Decision.History[1].ID
	runOK(t, e, "decisions", "entries", "move", f.Decision.ID, first, "--bottom")
	if f.Decision.History[1].ID != first || lastWrite(t, e)["previousEntryId"] != second {
		t.Fatalf("the entry is not last: %v", lastWrite(t, e))
	}
	runOK(t, e, "decisions", "entries", "delete", f.Decision.ID, first)
	if len(f.Decision.History) != 1 {
		t.Fatalf("history %v", f.Decision.History)
	}

	text := e.run("", "decisions", "show", f.Decision.ID, "--styled")
	if !strings.Contains(text.stdout, "## Consequences (1)") || !strings.Contains(text.stdout, "[ ] ") {
		t.Fatalf("the text lacks the consequences:\n%s", text.stdout)
	}
}

func TestADecisionCarriesLabelsAndComments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withLabels(e)

	labels := listOfObjects(t, runOK(t, e, "decisions", "label", f.Decision.ID, "Plain").Data)
	if len(labels) != 1 || labels[0]["name"] != "Plain" {
		t.Fatalf("labels %v", labels)
	}
	env, r := e.runJSON("", "decisions", "label", f.ArchivedDecision.ID, "Plain")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeDecisionArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	runOK(t, e, "decisions", "comments", "create", f.Decision.ID, "Agreed, **Monday**.")
	comments := listOfObjects(t, runOK(t, e, "decisions", "comments", "list", f.Decision.ID).Data)
	if len(comments) != 2 {
		t.Fatalf("comments %v", comments)
	}
}

func TestASearchForDecisionsLeadsToDecisionsShow(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	// Minka's search takes no decision of the archive, and the comments under
	// one, marked archived.
	f.ArchivedDecision.Comments = []*fakeapi.Comment{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f0", AuthorID: f.Anke.ID, Body: "<div>Tuesdays were better</div>"}}
	archived := listOfObjects(t, rawOf(t, decode(t, runOK(t, e, "search", "Tuesdays", "--kind", "decisions").Data), "hits"))
	if len(archived) != 1 || archived[0]["kind"] != "comment" || archived[0]["archived"] != true || archived[0]["targetId"] != f.ArchivedDecision.ID {
		t.Fatalf("hits %v: want the comment under the decision of the archive alone, marked archived", archived)
	}
	env := runOK(t, e, "search", "Mondays", "--kind", "decisions")
	hits := decodeSearch(t, env.Data).Hits
	if len(hits) != 1 || hits[0].Kind != "decision" || hits[0].ID != f.Decision.ID {
		t.Fatalf("hits %v", hits)
	}
	if !strings.Contains(e.lastRequestQuery(), "kinds=decisions") {
		t.Fatalf("query %q", e.lastRequestQuery())
	}
	if crumb := crumbFor(t, env.Breadcrumbs, "show"); crumb != "minka decisions show "+f.Decision.ID {
		t.Fatalf("breadcrumb %q", crumb)
	}
}

// A body in a list of --from is taken as written: "-" reads no stdin.
func TestABodyOfAListIsTakenAsWritten(t *testing.T) {
	e := newTestEnv(t)
	file := filepath.Join(t.TempDir(), "decisions.json")
	if err := os.WriteFile(file, []byte(`[{"title": "A dash", "body": "-"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, r := e.runJSON("Injected from stdin", "decisions", "create", "--tool", e.fixture.Decisions.ID, "--from", file)
	expectCode(t, r, ExitOK)
	if body, _ := lastWrite(t, e)["body"].(string); strings.Contains(body, "Injected") {
		t.Fatalf("body %q was read from stdin", body)
	}
}

func TestANewBodyWithoutTheFilesOfTheOldOneNeedsDropAttachments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Decision.Body = "<div>See " + aMention + "</div>"

	env, r := e.runJSON("", "decisions", "edit", f.Decision.ID, "--body", "Nothing attached")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || len(e.server.WritingRequests()) != 0 {
		t.Fatalf("hint %q, %d writes", env.Hint, len(e.server.WritingRequests()))
	}
	runOK(t, e, "decisions", "edit", f.Decision.ID, "--body", "Nothing attached", "--drop-attachments")
	if strings.Contains(f.Decision.Body, "action-text-attachment") {
		t.Fatal("the body was not replaced")
	}
}

// Flags that exclude each other send nothing; before, an event beside a card
// won silently.
func TestFlagsThatExcludeEachOtherSendNothing(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	before := len(e.server.Requests())

	for _, args := range [][]string{
		{"decisions", "list", f.Decisions.ID, "--holds", "--status", "proposed"},
		{"decisions", "create", "--tool", f.Decisions.ID, "--title", "T", "--applies-to-card", f.CardInDoing.ID, "--applies-to-event", f.OnceEvent.ID},
		{"decisions", "edit", f.Decision.ID, "--applies-to-card", f.CardInDoing.ID, "--applies-to-event", f.OnceEvent.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

// Minka answers a change of a decision of the archive with 404, and the hint
// names the archive among the reasons.
func TestAChangeOfADecisionOfTheArchiveNamesTheArchive(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"decisions", "edit", e.fixture.ArchivedDecision.ID, "--title", "Renamed"},
		{"decisions", "status", e.fixture.ArchivedDecision.ID, "done"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "archive") {
			t.Errorf("%s: hint %q", args[1], env.Hint)
		}
	}
}
