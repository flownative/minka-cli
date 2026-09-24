package cli

import (
	"encoding/json"
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

	r := e.run("", "cards", "notes", "show", "mnk-3t", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"## Plan\n\n- order the tent\n\n", "Last changed by " + e.fixture.Anke.Name, "--expect " + updatedAt} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("text lacks %q:\n%s", want, r.stdout)
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

	r := e.run("", "cards", "notes", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.TrimSpace(r.stdout) != "No working notes yet." {
		t.Errorf("text:\n%s", r.stdout)
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
	for _, expect := range []string{
		"2026-09-24T11:53:14Z",
		"2026-09-24T11:53:14.123Z",
		"2026-09-24T11:53:14.123456789Z",
		"2026-09-24T13:53:14.123456+02:00",
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
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || env.Meta["apiCode"] != "notesChanged" || env.Retryable {
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
	expectCode(t, r, ExitAPI)
	if !strings.Contains(r.stdout, "- order the tent") || !strings.Contains(r.stderr, "changed since they were read") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
}

func TestNotesLongerThanMinkaKeepsAreRefusedWithoutARequest(t *testing.T) {
	e := newTestEnv(t)

	r := e.run(strings.Repeat("ä", maxNotesLength)+"\r\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectCode(t, r, ExitUsage)
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Errorf("%d writes were sent", len(writes))
	}

	_, r = e.runJSON(strings.Repeat("ä", maxNotesLength-1)+"\r\n", "cards", "notes", "set", e.fixture.CardInTriage.ID, "-", "--force")
	expectCode(t, r, ExitOK)
}
