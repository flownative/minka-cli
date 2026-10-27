package cli

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// withLabels gives the project a plain label, Plain, the board two labels of
// the scope Stage, Ready and then Late, and the Docs & Files tool one, Draft.
func withLabels(e *testEnv) {
	f := e.fixture
	e.server.AddLabel(f.Project.ID, "", "Plain", "gray")
	e.server.AddLabel(f.Project.ID, f.Board.ID, "Stage::Ready", "red")
	e.server.AddLabel(f.Project.ID, f.Board.ID, "Stage::Late", "blue")
	e.server.AddLabel(f.Project.ID, f.Files.ID, "Draft", "orange")
}

func labelNamed(e *testEnv, name string) *fakeapi.Label {
	e.t.Helper()
	for _, label := range e.server.Labels {
		if label.Name == name {
			return label
		}
	}
	e.t.Fatalf("no label %s", name)
	return nil
}

func labelNamesOf(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var names []string
	for _, label := range listOfObjects(t, raw) {
		names = append(names, label["name"].(string))
	}
	return names
}

func namesOn(e *testEnv, itemID string) []string {
	var names []string
	for _, label := range e.server.LabelsOn(itemID) {
		names = append(names, label.Name)
	}
	return names
}

func TestLabelsListShowsAToolsOwnLabelsBeforeTheProjects(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	env := runOK(t, e, "labels", "list", "--board", e.fixture.Board.ID)
	if got := labelNamesOf(t, env.Data); !slices.Equal(got, []string{"Stage::Ready", "Stage::Late", "Plain"}) {
		t.Fatalf("labels %v", got)
	}
	r := e.run("", "labels", "list", "--board", e.fixture.Board.ID, "--styled")
	for _, want := range []string{"NAME", "Stage::Ready", "red", "tool", "Plain", "project"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

// What is wrong with the command line is said before anything is written: the
// place, an edit that changes nothing, a move to no place or next
// to itself, and an empty label name.
func TestALabelCommandRefusesWhatIsWrongBeforeWriting(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	for _, args := range [][]string{
		{"labels", "list"},
		{"labels", "list", "--board", f.Board.ID, "--project", f.Project.ID},
		{"labels", "list", "--board", "Board"},
		{"labels", "edit", "Plain", "--project", f.Project.ID},
		{"labels", "move", "Stage::Late", "--board", f.Board.ID},
		{"labels", "move", "Stage::Late", "--board", f.Board.ID, "--first", "--after", "Stage::Ready"},
		{"labels", "move", "Stage::Late", "--board", f.Board.ID, "--after", "stage :: LATE"},
		{"cards", "label", f.CardInDoing.ID},
		{"cards", "label", f.CardInDoing.ID, "Plain", "  "},
		{"cards", "unlabel", "a card", "Plain"},
		{"docs", "label", "no-document", "Plain"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if len(e.server.WritingRequests()) != 0 {
			t.Fatalf("%v wrote", args)
		}
	}
}

// A name is found the way Minka compares names, case and spacing aside, among
// the place's own labels; one of the project under a board is pointed there.
func TestALabelIsFoundByItsNameAmongThoseOfItsPlace(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture

	runOK(t, e, "labels", "edit", "stage  ::ready", "--board", f.Board.ID, "--color", "teal")
	if labelNamed(e, "Stage::Ready").Color != "teal" {
		t.Fatal("the label was not recoloured")
	}
	if body := e.lastWriteBody(); len(body) != 1 || body["color"] != "teal" {
		t.Fatalf("body %v", body)
	}

	env, r := e.runJSON("", "labels", "edit", "plain", "--board", f.Board.ID, "--name", "Other")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "--project "+f.Project.ID) {
		t.Errorf("hint %q", env.Hint)
	}
	env, r = e.runJSON("", "labels", "delete", "Nowhere", "--tool", f.Files.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "Nowhere") || !strings.Contains(env.Hint, "labels list --tool "+f.Files.ID) {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
	if len(e.server.WritingRequests()) != 1 {
		t.Fatal("a label that was not found was written")
	}
}

func TestALabelIsCreatedRenamedAndDeleted(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env := runOK(t, e, "labels", "create", "Priority :: High", "--project", f.Project.ID)
	created := decode(t, env.Data)
	if created["name"] != "Priority::High" || created["color"] != "blue" {
		t.Fatalf("created %v", created)
	}
	if body := e.lastWriteBody(); len(body) != 1 {
		t.Fatalf("a label without --color sent %v", body)
	}

	env, r := e.runJSON("", "labels", "create", "priority::high", "--board", f.Board.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelNameTaken" || !strings.Contains(env.Hint, "unique") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = e.runJSON("", "labels", "create", "a::b::c", "--board", f.Board.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelNameMalformed" || !strings.Contains(env.Hint, "::") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	runOK(t, e, "labels", "edit", "Priority::High", "--project", f.Project.ID, "--name", "Priority::Urgent", "--color", "red")
	if label := labelNamed(e, "Priority::Urgent"); label.Color != "red" {
		t.Fatalf("colour %s", label.Color)
	}
	env = runOK(t, e, "labels", "delete", "priority::urgent", "--project", f.Project.ID)
	if decode(t, env.Data)["deleted"] != true || len(e.server.Labels) != 0 {
		t.Fatalf("data %s, labels %v", env.Data, e.server.Labels)
	}
	_, r = e.runJSON("", "labels", "delete", created["id"].(string), "--project", f.Project.ID)
	expectCode(t, r, ExitNotFound)
}

// --first sends nothing, --after only the previous label, --before only the
// next: where both are sent Minka places by the previous one alone.
func TestALabelMovesByOneNeighbourAtMost(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	third := e.server.AddLabel(f.Project.ID, f.Board.ID, "Third", "lime")
	order := func() []string {
		return labelNamesOf(t, runOK(t, e, "labels", "list", "--board", f.Board.ID).Data)
	}
	for _, step := range []struct {
		args []string
		body map[string]any
		want []string
	}{
		{[]string{"Third", "--first"}, map[string]any{}, []string{"Third", "Stage::Ready", "Stage::Late", "Plain"}},
		{[]string{"Third", "--after", "stage::late"}, map[string]any{"previousLabelId": third.ID}, []string{"Stage::Ready", "Stage::Late", "Third", "Plain"}},
		{[]string{"Third", "--before", "Stage::Ready"}, map[string]any{"nextLabelId": ""}, []string{"Third", "Stage::Ready", "Stage::Late", "Plain"}},
	} {
		env := runOK(t, e, append([]string{"labels", "move", "--board", f.Board.ID}, step.args...)...)
		body := e.lastWriteBody()
		if len(body) != len(step.body) {
			t.Fatalf("%v sent %v", step.args, body)
		}
		for name := range step.body {
			if _, ok := body[name]; !ok {
				t.Fatalf("%v sent %v, want %s alone", step.args, body, name)
			}
		}
		if got := order(); !slices.Equal(got, step.want) {
			t.Fatalf("%v: order %v, want %v", step.args, got, step.want)
		}
		if decode(t, env.Data)["id"] != third.ID {
			t.Fatalf("the answer is not the moved label: %s", env.Data)
		}
	}
	env, r := e.runJSON("", "labels", "move", "Third", "--board", f.Board.ID, "--after", labelNamed(e, "Plain").ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelPlaceUnknown" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// An ID goes as labelId, a name as name; a scoped label replaces another of
// its scope; the answer is the card's labels afterwards.
func TestLabelsArePutOnACardByIDOrNameInTurn(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	f.CardInDoing.ShortID = "MNK-1A"
	ready := labelNamed(e, "Stage::Ready")

	env := runOK(t, e, "cards", "label", "MNK-1A", strings.ToUpper(ready.ID), "plain")
	if got := labelNamesOf(t, env.Data); !slices.Equal(got, []string{"Stage::Ready", "Plain"}) {
		t.Fatalf("labels %v", got)
	}
	writes := e.server.WritingRequests()
	if len(writes) != 2 || string(writes[0].Body) != `{"labelId":"`+ready.ID+`"}` || string(writes[1].Body) != `{"name":"plain"}` {
		t.Fatalf("writes %v", writes)
	}
	if writes[0].Path != "/api/v1/cards/"+f.CardInDoing.ID+"/labels" {
		t.Fatalf("path %s", writes[0].Path)
	}

	env = runOK(t, e, "cards", "label", f.CardInDoing.ID, "STAGE :: late")
	if got := labelNamesOf(t, env.Data); !slices.Equal(got, []string{"Stage::Late", "Plain"}) {
		t.Fatalf("a label of the scope Stage did not replace the other: %v", got)
	}

	r := e.run("", "cards", "unlabel", f.CardInDoing.ID, "Plain", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Labels: Stage::Late") {
		t.Fatalf("output:\n%s", r.stdout)
	}
	last := e.server.WritingRequests()[len(e.server.WritingRequests())-1]
	if last.Method != http.MethodDelete || last.Query != "name=Plain" {
		t.Fatalf("%s %s?%s", last.Method, last.Path, last.Query)
	}
	env = runOK(t, e, "cards", "unlabel", f.CardInDoing.ID, labelNamed(e, "Stage::Late").ID)
	if got := labelNamesOf(t, env.Data); len(got) != 0 {
		t.Fatalf("labels %v", got)
	}
}

// The first refusal ends the run; what was applied before it is named, and
// nothing after it is sent.
func TestPuttingOnSeveralLabelsStopsAtTheFirstRefusal(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture

	env, r := e.runJSON("", "docs", "label", f.Document.ID, "Draft", "Nowhere", "Plain")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelUnknown" || !strings.Contains(env.Error, "Nowhere") {
		t.Fatalf("apiCode %v, error %q", env.Meta["apiCode"], env.Error)
	}
	data := decode(t, env.Data)
	if !slices.Equal(stringsOf(data["applied"]), []string{"Draft"}) || !slices.Equal(stringsOf(data["notApplied"]), []string{"Nowhere", "Plain"}) {
		t.Fatalf("data %v", data)
	}
	if len(e.server.WritingRequests()) != 2 {
		t.Fatalf("%d writes, want 2", len(e.server.WritingRequests()))
	}
	if got := namesOn(e, f.Document.ID); !slices.Equal(got, []string{"Draft"}) {
		t.Fatalf("the document carries %v", got)
	}

	env, r = e.runJSON("", "docs", "uploads", "unlabel", f.FilesUpload.ID, "Nowhere")
	expectCode(t, r, ExitValidation)
	if data := decode(t, env.Data); len(stringsOf(data["applied"])) != 0 {
		t.Fatalf("data %v", data)
	}
}

func stringsOf(value any) []string {
	var list []string
	for _, entry := range value.([]any) {
		list = append(list, entry.(string))
	}
	return list
}

func TestAnArchivedItemKeepsItsLabels(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	env, r := e.runJSON("", "docs", "label", e.fixture.ArchivedDocument.ID, "Draft")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "documentArchived" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken
	env, r = e.runJSON("", "docs", "uploads", "label", e.fixture.FilesUpload.ID, "Draft")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "upload") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// The text views name the labels of an item: a line of their own on a single
// item, in brackets in a list.
func TestLabelsShowWhereTheirItemsShow(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	f.CardInDoing.AssigneeIDs = []string{f.Bruno.ID}
	e.server.PutLabelOn(f.CardInDoing.ID, labelNamed(e, "Plain"))
	e.server.PutLabelOn(f.CardInDoing.ID, labelNamed(e, "Stage::Ready"))
	e.server.PutLabelOn(f.Document.ID, labelNamed(e, "Draft"))
	e.server.PutLabelOn(f.FilesUpload.ID, labelNamed(e, "Plain"))

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"cards", "show", f.CardInDoing.ID}, "Labels: Stage::Ready, Plain\n"},
		{[]string{"boards", "show", f.Board.ID}, f.CardInDoing.Title + " [Stage::Ready] [Plain]"},
		{[]string{"cards", "mine"}, f.CardInDoing.Title + " [Stage::Ready] [Plain]"},
		{[]string{"docs", "show", f.Document.ID}, "Labels: Draft\n"},
		{[]string{"docs", "uploads", "show", f.FilesUpload.ID}, "Labels: Plain\n"},
		{[]string{"docs", "list", f.Files.ID}, f.Document.Title + " [Draft]"},
		{[]string{"docs", "list", f.Files.ID, "--folder", f.Folder.ID}, f.FilesUpload.Title + " [Plain]"},
	} {
		r := e.run("", append(c.args, "--styled")...)
		expectCode(t, r, ExitOK)
		if !strings.Contains(r.stdout, c.want) {
			t.Errorf("%v: missing %q in:\n%s", c.args, c.want, r.stdout)
		}
	}
	env := runOK(t, e, "cards", "show", f.CardInDoing.ID)
	if labels, ok := decode(t, env.Data)["labels"].([]any); !ok || len(labels) != 2 {
		t.Fatalf("JSON labels %v", decode(t, env.Data)["labels"])
	}
}

// A colour goes as typed, and Minka's refusal is answered with the twelve it
// takes.
func TestMinkaJudgesTheColourOfALabel(t *testing.T) {
	e := newTestEnv(t)
	for _, color := range []string{"Red", "", "aqua"} {
		before := len(e.server.WritingRequests())
		env, r := e.runJSON("", "labels", "create", "Fresh", "--board", e.fixture.Board.ID, "--color", color)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "labelColorUnknown" || !strings.Contains(env.Hint, "lime") {
			t.Fatalf("%q: apiCode %v, hint %q", color, env.Meta["apiCode"], env.Hint)
		}
		if writes := e.server.WritingRequests(); len(writes) != before+1 || decode(t, json.RawMessage(writes[len(writes)-1].Body))["color"] != color {
			t.Fatalf("%q was not sent as typed", color)
		}
	}
}

// Labels taken off stay off when they cannot be read back: the run says that
// every one was applied.
func TestLabelsTakenOffAreReportedAppliedWhenTheReadBackFails(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	e.server.PutLabelOn(f.Document.ID, labelNamed(e, "Draft"))
	e.server.FailNextReadWith(http.StatusBadGateway)

	env, r := e.runJSON("", "docs", "unlabel", f.Document.ID, "Draft")
	if r.code == ExitOK {
		t.Fatal("a read that failed passed for the labels of the document")
	}
	data := decode(t, env.Data)
	if !slices.Equal(stringsOf(data["applied"]), []string{"Draft"}) || len(data["notApplied"].([]any)) != 0 {
		t.Fatalf("data %v", data)
	}
	if !strings.Contains(env.Hint, "docs show "+f.Document.ID) || len(e.server.LabelsOn(f.Document.ID)) != 0 {
		t.Fatalf("hint %q, labels %v", env.Hint, namesOn(e, f.Document.ID))
	}
}

// A name goes into the query encoded, a space, "::" and "+" included, and
// Minka finds the label by it.
func TestANameIsEncodedInTheQueryOfAnUnlabel(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	label := e.server.AddLabel(f.Project.ID, "", "Team::C++ Core", "teal")
	e.server.PutLabelOn(f.FilesUpload.ID, label)

	runOK(t, e, "docs", "uploads", "unlabel", f.FilesUpload.ID, "team :: c++ core")
	last := e.server.WritingRequests()[len(e.server.WritingRequests())-1]
	if last.Query != "name=team+%3A%3A+c%2B%2B+core" {
		t.Fatalf("query %s", last.Query)
	}
	if len(e.server.LabelsOn(f.FilesUpload.ID)) != 0 {
		t.Fatal("the label stayed on")
	}
}

// A neighbour of a move that is a label of the project under a board is no
// label of that place, and the error says so instead of pointing elsewhere.
func TestANeighbourOfAnotherPlaceIsNamedAsSuch(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	env, r := e.runJSON("", "labels", "move", "Stage::Late", "--board", e.fixture.Board.ID, "--after", "Plain")
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Hint, "--project") || !strings.Contains(env.Hint, "own place") || !strings.Contains(env.Error, "--after") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("the move was sent")
	}
}

// labelReads counts the requests that read the labels of a place.
func labelReads(e *testEnv) int {
	reads := 0
	for _, r := range e.server.Requests() {
		if r.Method == http.MethodGet && strings.HasSuffix(r.Path, "/labels") {
			reads++
		}
	}
	return reads
}

// A new card takes labels by ID and by name, a name found among the board's
// and the project's labels the way Minka compares names; each goes once, and
// only IDs are sent.
func TestANewCardTakesLabelsByIDAndByName(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	plain, ready := labelNamed(e, "Plain"), labelNamed(e, "Stage::Ready")

	env := runOK(t, e, "cards", "create", "--board", f.Board.ID, "--title", "Labelled",
		"--label", strings.ToUpper(plain.ID), "--label", "stage :: READY", "--label", "plain")
	ids, _ := e.lastWriteBody()["labelIds"].([]any)
	if len(ids) != 2 || ids[0] != plain.ID || ids[1] != ready.ID {
		t.Fatalf("labelIds %v", ids)
	}
	if got := labelNamesOf(t, rawField(env.Data, "labels")); !slices.Equal(got, []string{"Stage::Ready", "Plain"}) {
		t.Fatalf("the card carries %v", got)
	}
	if labelReads(e) != 1 {
		t.Fatalf("the labels were read %d times", labelReads(e))
	}

	r := e.run("", "cards", "create", "--board", f.Board.ID, "--title", "Text", "--label", "Plain", "--styled")
	if !strings.Contains(r.stdout, "Labels: Plain") {
		t.Errorf("no labels line in:\n%s", r.stdout)
	}
}

// Without --label nothing about labels is sent or read; IDs alone are sent
// without reading the labels either.
func TestANewCardWithoutNamedLabelsReadsNoLabels(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	runOK(t, e, "cards", "create", "--board", f.Board.ID, "--title", "Bare")
	if _, sent := e.lastWriteBody()["labelIds"]; sent {
		t.Fatalf("labelIds sent without --label: %v", e.lastWriteBody())
	}
	runOK(t, e, "cards", "create", "--board", f.Board.ID, "--title", "By ID", "--label", labelNamed(e, "Plain").ID)
	if labelReads(e) != 0 {
		t.Fatalf("the labels were read %d times", labelReads(e))
	}
}

// A label that cannot be named is refused before the card is created: an
// empty name, a name found nowhere or ambiguous, a label of the Docs & Files
// tool; two of one scope are Minka's refusal, with a hint.
func TestANewCardsLabelsAreRefusedBeforeAnyWrite(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	create := func(labels ...string) (envelope, result) {
		args := []string{"cards", "create", "--board", f.Board.ID, "--title", "Refused"}
		for _, label := range labels {
			args = append(args, "--label", label)
		}
		return e.runJSON("", args...)
	}

	_, r := create("Plain", "  ")
	expectCode(t, r, ExitUsage)
	env, r := create("Plain", "Draft")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "Draft") || !strings.Contains(env.Hint, "labels list --board "+f.Board.ID) {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
	e.server.AddLabel(f.Project.ID, "", "PLAIN", "gray")
	_, r = create("plain")
	expectCode(t, r, ExitAmbiguous)
	if len(e.server.WritingRequests()) != 0 {
		t.Fatalf("a refused label wrote: %v", e.server.WritingRequests())
	}

	env, r = create("Stage::Ready", "stage::late")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelsShareAScope" || !strings.Contains(env.Hint, "one label of each scope") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = create(labelNamed(e, "Draft").ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "labelUnknown" || !strings.Contains(env.Hint, "--board "+f.Board.ID) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if got := len(f.Triage.Cards); got != 1 {
		t.Fatalf("%d cards in Triage after refusals only", got)
	}
}

// A scope is renamed for the place's own labels and the answer is those
// labels afterwards; a scope in use is joined, and an item carrying a label
// of each keeps both.
func TestAScopeIsRenamedAtItsPlace(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	phase := e.server.AddLabel(f.Project.ID, f.Board.ID, "Phase::Early", "lime")
	e.server.PutLabelOn(f.CardInDoing.ID, labelNamed(e, "Stage::Ready"))
	e.server.PutLabelOn(f.CardInDoing.ID, phase)

	env := runOK(t, e, "labels", "rename-scope", "stage", "Phase", "--board", f.Board.ID)
	if body := e.lastWriteBody(); len(body) != 2 || body["scope"] != "stage" || body["newScope"] != "Phase" {
		t.Fatalf("body %v", body)
	}
	if path := e.server.WritingRequests()[0].Path; path != "/api/v1/boards/"+f.Board.ID+"/labels/scope-renames" {
		t.Fatalf("path %s", path)
	}
	if got := labelNamesOf(t, env.Data); !slices.Equal(got, []string{"Phase::Ready", "Phase::Late", "Phase::Early"}) {
		t.Fatalf("answer %v", got)
	}
	if got := namesOn(e, f.CardInDoing.ID); !slices.Equal(got, []string{"Phase::Ready", "Phase::Early"}) {
		t.Fatalf("the card carries %v", got)
	}

	r := e.run("", "labels", "rename-scope", "phase", "Stufe", "--board", f.Board.ID, "--styled")
	for _, want := range []string{"Renamed scope phase to Stufe", "Stufe::Early", "NAME"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

// What Minka refuses comes back with its code and a hint; nothing is renamed.
func TestARefusedScopeRenameSaysWhy(t *testing.T) {
	e := newTestEnv(t)
	withLabels(e)
	f := e.fixture
	e.server.AddLabel(f.Project.ID, f.Files.ID, "Phase::Late", "gray")
	for _, c := range []struct {
		args          []string
		exit          int
		apiCode, hint string
	}{
		{[]string{"Stage", "Phase"}, ExitValidation, "labelNameTaken", "nothing was renamed"},
		{[]string{"Stage", "a::b"}, ExitValidation, "labelScopeMalformed", "part before"},
		{[]string{"Stage", " "}, ExitValidation, "labelScopeMissing", "part before"},
		{[]string{"Plain", "Other"}, ExitNotFound, "", `has the scope "Plain"`},
	} {
		env, r := e.runJSON("", append([]string{"labels", "rename-scope", "--board", f.Board.ID}, c.args...)...)
		expectCode(t, r, c.exit)
		if c.apiCode != "" && env.Meta["apiCode"] != c.apiCode || !strings.Contains(env.Hint, c.hint) {
			t.Errorf("%v: apiCode %v, hint %q", c.args, env.Meta["apiCode"], env.Hint)
		}
	}
	if labelNamed(e, "Stage::Ready").Name != "Stage::Ready" {
		t.Fatal("a refused rename renamed")
	}
	_, r := e.runJSON("", "labels", "rename-scope", "Stage", "--board", f.Board.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != 4 {
		t.Fatalf("%d writes", len(e.server.WritingRequests()))
	}
}
