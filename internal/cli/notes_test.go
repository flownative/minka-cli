package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
)

// withNotes gives the card in triage notes Anke wrote.
func withNotes(e *testEnv) string {
	at := time.Date(2026, 9, 24, 11, 53, 14, 123456000, time.UTC)
	card := e.fixture.CardInTriage
	card.Notes, card.NotesUpdatedAt, card.NotesUpdatedByID = "## Plan\n\n- order the tent\n", &at, e.fixture.Anke.ID
	return "2026-09-24T11:53:14.123456Z"
}

func TestNotesShowPrintsTheMarkdownAndWhoChangedItLast(t *testing.T) {
	e := newTestEnv(t)
	updatedAt := withNotes(e)
	e.fixture.CardInTriage.ShortID = "MNK-3T"

	// What stands on the one stream is the notes and nothing else, so that
	// somebody can put them in a file and send them back unchanged.
	r := e.run("", "cards", "notes", "show", "mnk-3t", "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != "## Plan\n\n- order the tent\n" {
		t.Errorf("stdout is not the notes as they stand:\n%q", r.stdout)
	}
	for _, want := range []string{"Last changed by " + e.fixture.Anke.Name, "--expect " + updatedAt} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("the word about them lacks %q:\n%s", want, r.stderr)
		}
	}

	env, r := e.runJSON("", "cards", "notes", "show", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	var notes api.CardNotes
	if err := json.Unmarshal(env.Data, &notes); err != nil || notes.Body != "## Plan\n\n- order the tent\n" || notes.UpdatedAt == nil || *notes.UpdatedAt != updatedAt {
		t.Errorf("data %s", env.Data)
	}
	if len(env.Breadcrumbs) == 0 || !strings.Contains(env.Breadcrumbs[0].Cmd, "--expect "+updatedAt) {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
}

func TestNotesShowOfACardWithoutNotes(t *testing.T) {
	e := newTestEnv(t)

	// Nothing on the stream the notes have to themselves: an empty text is
	// what notes nobody has written are, and the word about it goes the other
	// way like every other word about them.
	r := e.run("", "cards", "notes", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != "" {
		t.Errorf("stdout:\n%q", r.stdout)
	}
	if strings.TrimSpace(r.stderr) != "No working notes yet." {
		t.Errorf("stderr:\n%s", r.stderr)
	}

	env, _ := e.runJSON("", "cards", "notes", "show", e.fixture.CardInTriage.ID)
	if !strings.Contains(env.Breadcrumbs[0].Cmd, "--expect none") {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
}

func TestNotesOfACardNobodyMaySeeAreNotFound(t *testing.T) {
	e := newTestEnv(t)
	_, r := e.runJSON("", "cards", "notes", "show", e.fixture.OtherCard.ID)
	expectCode(t, r, ExitNotFound)
}

func TestNotesSetNeedsToKnowWhatItMayReplace(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{},
		{"--expect", "yesterday"},
		{"--expect", "none", "--force"},
	} {
		r := e.run("text", append([]string{"cards", "notes", "set", e.fixture.CardInTriage.ID, "-"}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	// Minka reads six digits of a second and no more, and Go would read any
	// number of them, so a longer one has to be refused here.
	for _, expect := range []string{
		"2026-09-24T11:53:14.123456789Z",
		"2026-09-24 11:53:14Z",
		"yesterday",
		"2026-09-24T11:53:14",
	} {
		env, r := e.runJSON("text", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--expect", expect)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Hint, "cards notes show") {
			t.Errorf("--expect %s: hint %q does not point to cards notes show", expect, env.Hint)
		}
	}
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Errorf("%d writes were sent", len(writes))
	}
}

// Every form Minka reads a guard's time in goes through as it was typed:
// refusing here what the server would have taken would send an agent after a
// problem it does not have.
func TestNotesSetTakesEveryTimeMinkaReads(t *testing.T) {
	e := newTestEnv(t)
	for _, expect := range []string{
		"2026-09-24T11:53:14Z",
		"2026-09-24T11:53:14.123Z",
		"2026-09-24T11:53:14.123456Z",
		"2026-09-24T13:53:14.123456+02:00",
		"2026-09-24T13:53:14+0200",
	} {
		before := len(e.server.WritingRequests())
		expectCode(t, e.run("text", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--expect", expect, "--json"), ExitOK)
		writes := e.server.WritingRequests()
		if len(writes) != before+1 {
			t.Fatalf("--expect %s sent %d writes", expect, len(writes)-before)
		}
		var sent struct {
			ExpectedUpdatedAt *string `json:"expectedUpdatedAt"`
		}
		if err := json.Unmarshal(writes[len(writes)-1].Body, &sent); err != nil {
			t.Fatal(err)
		}
		if sent.ExpectedUpdatedAt == nil || *sent.ExpectedUpdatedAt != expect {
			t.Errorf("--expect %s went out as %v", expect, sent.ExpectedUpdatedAt)
		}
	}
}

// A guard on notes nobody has written yet guards nothing: the write creates
// them, because there is no earlier version anybody could have overwritten.
func TestAGuardOnNotesThatDoNotExistYetWrites(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "First word", "--expect", "2026-09-24T11:53:14.123456Z")
	expectCode(t, r, ExitOK)
	if env.Meta["apiCode"] != nil {
		t.Errorf("envelope %+v", env)
	}
	if e.fixture.CardInTriage.Notes != "First word" {
		t.Errorf("the notes are %q", e.fixture.CardInTriage.Notes)
	}
}

// A card nobody can see is a card nobody writes notes on.
func TestNotesOfACardThatIsNotThere(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"show", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
		{"set", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "text", "--force"},
	} {
		_, r := e.runJSON("", append([]string{"cards", "notes"}, args...)...)
		expectCode(t, r, ExitNotFound)
	}
}

func TestNotesSetSendsTheTextWithTheGuard(t *testing.T) {
	e := newTestEnv(t)
	updatedAt := withNotes(e)

	env, r := e.runJSON("## Plan\n\n- order the tent\n- ask Bruno\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--expect", updatedAt)
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if len(body) != 2 || body["body"] != "## Plan\n\n- order the tent\n- ask Bruno\n" || body["expectedUpdatedAt"] != updatedAt {
		t.Errorf("body sent %v", body)
	}
	var notes api.CardNotes
	if err := json.Unmarshal(env.Data, &notes); err != nil || !strings.HasSuffix(notes.Body, "- ask Bruno\n") || notes.UpdatedBy == nil || notes.UpdatedBy.ID != e.fixture.Bruno.ID {
		t.Errorf("data %s", env.Data)
	}
}

func TestNotesSetWithExpectNoneSendsNull(t *testing.T) {
	e := newTestEnv(t)

	_, r := e.runJSON("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "First notes", "--expect", "none")
	expectCode(t, r, ExitOK)
	if value, present := e.lastWriteBody()["expectedUpdatedAt"]; !present || value != nil {
		t.Errorf("expectedUpdatedAt sent %v (present %v), want null", value, present)
	}
}

func TestNotesSetWithForceSendsNoGuard(t *testing.T) {
	e := newTestEnv(t)
	withNotes(e)

	_, r := e.runJSON("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "", "--force")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if _, present := body["expectedUpdatedAt"]; present || body["body"] != "" {
		t.Errorf("body sent %v", body)
	}
	if e.fixture.CardInTriage.Notes != "" {
		t.Errorf("notes are %q, want them cleared", e.fixture.CardInTriage.Notes)
	}
}

func TestNotesChangedSinceTheyWereReadAreLeftAndShown(t *testing.T) {
	e := newTestEnv(t)
	withNotes(e)

	env, r := e.runJSON("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "Mine", "--expect", "2026-09-24T10:00:00.000000Z")
	expectCode(t, r, ExitValidation)
	if env.Code != CodeValidation || env.Meta["apiCode"] != "notesChanged" || env.Retryable {
		t.Errorf("envelope %+v", env)
	}
	var current api.CardNotes
	if err := json.Unmarshal(env.Data, &current); err != nil || current.Body != "## Plan\n\n- order the tent\n" {
		t.Errorf("data %s", env.Data)
	}
	if !strings.Contains(env.Hint, "--expect 2026-09-24T11:53:14.123456Z") {
		t.Errorf("hint %q", env.Hint)
	}
	if e.fixture.CardInTriage.Notes != "## Plan\n\n- order the tent\n" {
		t.Errorf("notes were written over: %q", e.fixture.CardInTriage.Notes)
	}

	r = e.run("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "Mine", "--expect", "none", "--styled")
	expectCode(t, r, ExitValidation)
	if !strings.Contains(r.stdout, "- order the tent") || !strings.Contains(r.stderr, "changed since they were read") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
}

func TestNotesLongerThanMinkaKeepsAreRefusedWithoutARequest(t *testing.T) {
	e := newTestEnv(t)

	// The same refusal the server gives, so the caller reads one exit code for
	// it whichever side decided.
	r := e.run(strings.Repeat("ä", maxNotesLength)+"\r\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectCode(t, r, ExitValidation)
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Errorf("%d writes were sent", len(writes))
	}

	_, r = e.runJSON(strings.Repeat("ä", maxNotesLength-1)+"\r\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectCode(t, r, ExitOK)
}

// The fake answers what the server answers, whether or not this CLI would ever
// ask it: Minka reads six digits of a second and no more, and Go reads as many
// as it is given whatever the layout says.
func TestTheFakeReadsAGuardsTimeNoWiderThanMinka(t *testing.T) {
	e := newTestEnv(t)
	for value, want := range map[string]int{
		"2026-09-24T11:53:14.123456Z":    http.StatusOK,
		"2026-09-24T11:53:14.123456789Z": http.StatusBadRequest,
		"2026-09-24T11:53:14+02":         http.StatusBadRequest,
	} {
		body := `{"body":"a text of its own for ` + value + `","expectedUpdatedAt":"` + value + `"}`
		request, err := http.NewRequest(http.MethodPut,
			e.server.URL+"/api/v1/cards/"+e.fixture.CardInTriage.ID+"/notes", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+e.fixture.Token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		answer, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("--expect %s answered %d, want %d: %s", value, response.StatusCode, want, answer)
		}
	}
}

// One byte order mark at the start of stdin is dropped, as an editor puts it
// in front of a file; one inside the text, or a second one, goes to Minka,
// which refuses it with the cause named. A text argument is sent as typed.
func TestNotesSetDropsTheByteOrderMarkOfAFileAndNothingElse(t *testing.T) {
	e := newTestEnv(t)

	_, r := e.runJSON("\ufeff## Plan\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["body"] != "## Plan\n" {
		t.Errorf("sent %q", body["body"])
	}

	env, r := e.runJSON("\ufeff\ufeff## Plan\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectRefusal(t, env, r, "cardNotesHasInvisibleCharacters", "byte order mark")
	if body := e.lastWriteBody(); body["body"] != "\ufeff## Plan\n" {
		t.Errorf("sent %q, want one mark kept", body["body"])
	}
	env, r = e.runJSON("## Plan\ufeff\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectRefusal(t, env, r, "cardNotesHasInvisibleCharacters", "The notes were not written")
	if body := e.lastWriteBody(); body["body"] != "## Plan\ufeff\n" {
		t.Errorf("sent %q", body["body"])
	}
	_, r = e.runJSON("", "cards", "notes", "set", e.fixture.CardInTriage.ID, "\ufeffon the command line", "--force")
	expectCode(t, r, ExitValidation)
}
