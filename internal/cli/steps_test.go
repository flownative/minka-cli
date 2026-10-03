package cli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// An edit of a step sends what was given and nothing else, so what was not
// given stays; a day given as empty goes out as null and takes the day off.
func TestStepsEditSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	step := withSteps(e)[0]
	step.AssigneeIDs = []string{e.fixture.Anke.ID}
	card := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "steps", "edit", card, "1", "--due", "2026-11-02")
	expectCode(t, r, ExitOK)
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"dueOn":"2026-11-02"}` {
		t.Fatalf("sent %s", raw)
	}
	if step.DueOn == nil || *step.DueOn != "2026-11-02" || !slices.Equal(step.AssigneeIDs, []string{e.fixture.Anke.ID}) {
		t.Fatalf("the step is due %v with %v", step.DueOn, step.AssigneeIDs)
	}
	shown := decode(t, env.Data)["steps"].([]any)[0].(map[string]any)
	if shown["dueOn"] != "2026-11-02" || len(shown["assignees"].([]any)) != 1 {
		t.Fatalf("data shows the step as %v", shown)
	}

	_, r = e.runJSON("", "steps", "edit", card, "1", "--due", "")
	expectCode(t, r, ExitOK)
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"dueOn":null}` || step.DueOn != nil {
		t.Fatalf("sent %s, the step is due %v", raw, step.DueOn)
	}
}

// People go as the whole set the step is to carry, looked up among the
// members of the card's project, and taking everybody off has to go out as an
// empty list: null names no change at all.
func TestStepsEditGivesTheWholeSetOfPeople(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	step := withSteps(e)[2]
	step.AssigneeIDs = []string{f.Anke.ID}

	_, r := e.runJSON("", "steps", "edit", f.CardInTriage.ID, "3", "--assignee", "me", "--assignee", "anke@example.com")
	expectCode(t, r, ExitOK)
	if people := stringsIn(e.lastWriteBody()["assigneeIds"]); !slices.Equal(people, []string{f.Bruno.ID, f.Anke.ID}) {
		t.Fatalf("sent %v", people)
	}
	if !slices.Equal(step.AssigneeIDs, []string{f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("the step carries %v", step.AssigneeIDs)
	}

	r = e.run("", "steps", "list", f.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "3. [ ] Book the tent (Anke, Bruno)") {
		t.Fatalf("the list does not name the people:\n%s", r.stdout)
	}

	_, r = e.runJSON("", "steps", "edit", f.CardInTriage.ID, "3", "--no-assignees")
	expectCode(t, r, ExitOK)
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"assigneeIds":[]}` || len(step.AssigneeIDs) != 0 {
		t.Fatalf("sent %s, the step carries %v", raw, step.AssigneeIDs)
	}
}

func TestStepsEditWithoutAChangeSendsNothing(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)
	card := e.fixture.CardInTriage.ID

	for name, args := range map[string][]string{
		"nothing":           {"steps", "edit", card, "1"},
		"people and nobody": {"steps", "edit", card, "1", "--assignee", "me", "--no-assignees"},
		"nobody known":      {"steps", "edit", card, "1", "--assignee", "Nobody Of That Name"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if n := len(e.server.WritingRequests()); n != 0 {
			t.Fatalf("%s: %d writes sent", name, n)
		}
	}
}

func TestStepsEditPassesMinkasRefusalsOn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withSteps(e)
	card := f.CardInTriage.ID

	env, r := e.runJSON("", "steps", "edit", card, "1", "--assignee", f.Stranger.ID, "--due", "2026-11-02")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" || env.Hint != stepPeopleHint {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if f.CardInTriage.Steps[0].DueOn != nil {
		t.Fatal("the day was written although the people were refused")
	}

	env, r = e.runJSON("", "steps", "edit", card, "1", "--due", "2026-02-30")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepDueDateNotADate" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	e.server.PutInTheArchive(f.CardInTriage)
	env, r = e.runJSON("", "steps", "edit", card, "1", "--due", "2026-11-02")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "cardArchived" || !strings.Contains(env.Hint, "minka cards unarchive") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// A Minka from before the people and the day of a step reads every change of
// a step as a tick, and refuses one without it as a missing tick, which says
// nothing about what was sent.
func TestAMinkaWithoutThePeopleOfAStepIsNamed(t *testing.T) {
	refusal := stepRefusal(&session{host: "minka.example"}, &api.Error{Method: "PATCH", StatusCode: 422, Code: "stepCompletedMissing", Message: "Changing a step needs the field \"completed\" as true or false."})
	if refusal.Code != CodeValidation || refusal.Hint != stepsOfAnOlderMinkaHint {
		t.Fatalf("code %s, hint %q", refusal.Code, refusal.Hint)
	}
	if other := stepRefusal(&session{host: "minka.example"}, errors.New("broken pipe")); other.Hint == stepsOfAnOlderMinkaHint {
		t.Fatal("a failure that is no refusal is named as an older Minka")
	}
}
