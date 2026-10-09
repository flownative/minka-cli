package cli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// The steps hang on a card alone and live under cards only: a top-level steps
// is an unknown command like any other.
func TestStepsLiveUnderCardsOnly(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage.ID

	_, r := e.runJSON("", "cards", "steps", "list", card)
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "steps", "list", card)
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Error, `unknown command "steps"`) {
		t.Errorf("code %q, error %q", env.Code, env.Error)
	}
}

// An edit of a step sends what was given and nothing else, so what was not
// given stays; a day given as empty goes out as null and takes the day off.
func TestStepsEditSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	step := withSteps(e)[0]
	step.AssigneeIDs = []string{e.fixture.Anke.ID}
	card := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "steps", "edit", card, "1", "--due", "2026-11-02")
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

	_, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--due", "")
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

	_, r := e.runJSON("", "cards", "steps", "edit", f.CardInTriage.ID, "3", "--assignee", "me", "--assignee", "anke@example.com")
	expectCode(t, r, ExitOK)
	if people := stringsIn(e.lastWriteBody()["assigneeIds"]); !slices.Equal(people, []string{f.Bruno.ID, f.Anke.ID}) {
		t.Fatalf("sent %v", people)
	}
	if !slices.Equal(step.AssigneeIDs, []string{f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("the step carries %v", step.AssigneeIDs)
	}

	r = e.run("", "cards", "steps", "list", f.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "3. [ ] Book the tent (Anke, Bruno)") {
		t.Fatalf("the list does not name the people:\n%s", r.stdout)
	}

	_, r = e.runJSON("", "cards", "steps", "edit", f.CardInTriage.ID, "3", "--no-assignees")
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
		"nothing":           {"cards", "steps", "edit", card, "1"},
		"people and nobody": {"cards", "steps", "edit", card, "1", "--assignee", "me", "--no-assignees"},
		"nobody known":      {"cards", "steps", "edit", card, "1", "--assignee", "Nobody Of That Name"},
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

	env, r := e.runJSON("", "cards", "steps", "edit", card, "1", "--assignee", f.Stranger.ID, "--due", "2026-11-02")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" || env.Hint != stepPeopleHint {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if f.CardInTriage.Steps[0].DueOn != nil {
		t.Fatal("the day was written although the people were refused")
	}

	env, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--due", "2026-02-30")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepDueDateNotADate" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	e.server.PutInTheArchive(f.CardInTriage)
	env, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--due", "2026-11-02")
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

// The text of a step goes in the same request as the people and the day, and
// an empty one goes out as it is for Minka to refuse, never as no change.
func TestStepsEditChangesTheTextWithTheRest(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	step := withSteps(e)[0]
	card := f.CardInTriage.ID

	env, r := e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "Call the caterer twice", "--due", "2026-11-02")
	expectCode(t, r, ExitOK)
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"dueOn":"2026-11-02","title":"Call the caterer twice"}` {
		t.Fatalf("sent %s", raw)
	}
	if step.Title != "Call the caterer twice" || step.DueOn == nil {
		t.Fatalf("the step is %+v", step)
	}
	if !strings.Contains(env.Summary, `"Call the caterer" to "Call the caterer twice"`) {
		t.Errorf("summary %q", env.Summary)
	}

	writes := len(e.server.WritingRequests())
	env, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepMissing" || len(e.server.WritingRequests()) != writes+1 {
		t.Fatalf("apiCode %v after %d writes", env.Meta["apiCode"], len(e.server.WritingRequests())-writes)
	}

	env, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "Somebody else's", "--assignee", f.Stranger.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" || step.Title != "Call the caterer twice" {
		t.Fatalf("apiCode %v, the step reads %q", env.Meta["apiCode"], step.Title)
	}
}

func stepTitles(steps []*fakeapi.Step) []string {
	titles := []string{}
	for _, step := range steps {
		titles = append(titles, step.Title)
	}
	return titles
}

// Each placement goes out as the neighbours Minka takes, the bottom as the
// last of the other steps, so that a step already last stays there.
func TestStepsMoveNamesTheNeighbours(t *testing.T) {
	const first, second, third = "55555555-5555-4555-8555-555555555551", "55555555-5555-4555-8555-555555555552", "55555555-5555-4555-8555-555555555553"
	for _, c := range []struct {
		name, moved string
		args        []string
		sent        string
		order       []string
	}{
		{"top", third, []string{"3", "--top"}, `{}`, []string{"Book the tent", "Call the caterer", "Book the tent"}},
		{"bottom", first, []string{"1", "--bottom"}, `{"previousStepId":"` + third + `"}`, []string{"Book the tent", "Book the tent", "Call the caterer"}},
		{"bottom where it stands", third, []string{"3", "--bottom"}, `{"previousStepId":"` + second + `"}`, []string{"Call the caterer", "Book the tent", "Book the tent"}},
		{"after", first, []string{"call the caterer", "--after", "2"}, `{"previousStepId":"` + second + `"}`, []string{"Book the tent", "Call the caterer", "Book the tent"}},
		{"before", third, []string{"3", "--before", first}, `{"nextStepId":"` + first + `"}`, []string{"Book the tent", "Call the caterer", "Book the tent"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			withSteps(e)
			card := e.fixture.CardInTriage

			env, r := e.runJSON("", append([]string{"cards", "steps", "move", card.ID}, c.args...)...)
			expectCode(t, r, ExitOK)
			writes := e.server.WritingRequests()
			if len(writes) != 1 || writes[0].Path != "/api/v1/cards/"+card.ID+"/steps/"+c.moved+"/moves" || string(writes[0].Body) != c.sent {
				t.Fatalf("sent %v", writes)
			}
			if titles := stepTitles(card.Steps); !slices.Equal(titles, c.order) {
				t.Errorf("the steps stand as %v", titles)
			}
			if steps, _ := decode(t, env.Data)["steps"].([]any); len(steps) != 3 {
				t.Errorf("data %s", env.Data)
			}
		})
	}
}

// A card with one step has no other to put it behind: the bottom is the top.
func TestStepsMoveToTheBottomOfOneStepNamesNoNeighbour(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInTriage.Steps = []*fakeapi.Step{{ID: "55555555-5555-4555-8555-555555555551", Title: "Alone"}}

	_, r := e.runJSON("", "cards", "steps", "move", e.fixture.CardInTriage.ID, "1", "--bottom")
	expectCode(t, r, ExitOK)
	if body := string(e.server.WritingRequests()[0].Body); body != `{}` {
		t.Fatalf("sent %s", body)
	}
}

func TestStepsMoveNeedsExactlyOnePlace(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)
	card := e.fixture.CardInTriage.ID

	for name, places := range map[string][]string{
		"none":             nil,
		"top and bottom":   {"--top", "--bottom"},
		"bottom and after": {"--bottom", "--after", "2"},
		"after and before": {"--after", "2", "--before", "3"},
	} {
		_, r := e.runJSON("", append([]string{"cards", "steps", "move", card, "1"}, places...)...)
		expectCode(t, r, ExitUsage)
		if n := len(e.server.WritingRequests()); n != 0 {
			t.Fatalf("%s: %d writes sent", name, n)
		}
	}
}

// Moving a step next to itself is Minka's to refuse, and the hint says where
// the order of the steps is read.
func TestStepsMovePassesMinkasRefusalsOn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withSteps(e)
	card := f.CardInTriage.ID

	env, r := e.runJSON("", "cards", "steps", "move", card, "1", "--after", "1")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepPlaceUnknown" || !strings.Contains(env.Hint, "minka cards steps list "+card) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	e.server.PutInTheArchive(f.CardInTriage)
	env, r = e.runJSON("", "cards", "steps", "move", card, "1", "--top")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "cardArchived" || !strings.Contains(env.Hint, "minka cards unarchive") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// A Minka from before renaming and moving steps answers a text alone with
// nothingToChange, writes the rest beside a text with a 200, and knows no
// move; none of it may pass for a step renamed or for a step that is gone.
func TestAMinkaThatNeitherRenamesNorMovesStepsIsNamed(t *testing.T) {
	e := newTestEnv(t)
	e.server.NeitherRenamesNorMovesSteps = true
	step := withSteps(e)[0]
	card := e.fixture.CardInTriage.ID

	env, r := e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "Call the caterer twice")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "nothingToChange" || env.Hint != stepsWithoutRenamingHint {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	env, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "  Call the caterer twice ", "--due", "2026-11-02")
	expectCode(t, r, ExitAPI)
	if !strings.Contains(env.Error, `still reads "Call the caterer"`) || env.Hint != stepsWithoutRenamingHint || step.DueOn == nil {
		t.Errorf("error %q, hint %q, due %v", env.Error, env.Hint, step.DueOn)
	}
	if steps, _ := decode(t, env.Data)["steps"].([]any); len(steps) != 3 {
		t.Errorf("data %s", env.Data)
	}

	env, r = e.runJSON("", "cards", "steps", "move", card, "1", "--bottom")
	expectCode(t, r, ExitNotFound)
	if env.Hint != stepsWithoutMovingHint {
		t.Errorf("hint %q", env.Hint)
	}

	e.server.NeitherRenamesNorMovesSteps = false
	_, r = e.runJSON("", "cards", "steps", "edit", card, "1", "--title", "  Call the caterer twice ", "--due", "2026-11-02")
	expectCode(t, r, ExitOK)
	env, r = e.runJSON("", "cards", "steps", "move", card, "55555555-5555-4555-8555-555555555559", "--top")
	expectCode(t, r, ExitNotFound)
	if env.Hint == stepsWithoutMovingHint {
		t.Error("a step that was not on the card is named as a Minka without moves")
	}
}

// A text is compared with the answer as Minka keeps it: trimmed of PHP's
// blanks only, so a no-break space stays, and with invalid UTF-8 replaced as
// it went out. Neither may pass for a Minka that does not rename.
func TestStepsEditTakesTheTextAsMinkaKeepsIt(t *testing.T) {
	for name, c := range map[string]struct{ sent, kept string }{
		"no-break spaces": {" Call the caterer twice ", " Call the caterer twice "},
		"PHP's blanks":    {" \x0bCall the caterer twice\x00 ", "Call the caterer twice"},
		"invalid UTF-8":   {"Call the caterer \xff\xfe twice", "Call the caterer �� twice"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			step := withSteps(e)[0]

			env, r := e.runJSON("", "cards", "steps", "edit", e.fixture.CardInTriage.ID, "1", "--title", c.sent)
			expectCode(t, r, ExitOK)
			if step.Title != c.kept || env.Error != "" {
				t.Errorf("the step reads %q, error %q", step.Title, env.Error)
			}
		})
	}
}

// Where the answer does not carry the text, the message claims nothing else
// written unless something else was sent.
func TestATextPassedOverClaimsOnlyWhatWasSent(t *testing.T) {
	title, due := "Other", "2026-11-02"
	alone := textPassedOver(api.Step{Title: "Call the caterer"}, api.UpdateStepInput{Title: &title}, nil)
	if alone.Code != CodeAPI || strings.Contains(alone.Message, "the rest") || alone.Hint != stepsWithoutRenamingHint {
		t.Errorf("alone: %+v", alone)
	}
	beside := textPassedOver(api.Step{Title: "Call the caterer"}, api.UpdateStepInput{Title: &title, DueOn: &due}, nil)
	if !strings.Contains(beside.Message, "the rest was written") {
		t.Errorf("beside a day: %q", beside.Message)
	}
}

// --bottom names the last step as read; where it left the card before the
// move, the hint says so instead of blaming --after or --before.
func TestStepsMoveToTheBottomSaysWhenTheLastStepIsGone(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)
	card := e.fixture.CardInTriage
	e.server.BeforeMovingStep = func(moved *fakeapi.Card) { moved.Steps = moved.Steps[:2] }

	env, r := e.runJSON("", "cards", "steps", "move", card.ID, "1", "--bottom")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepPlaceUnknown" || strings.Contains(env.Hint, "--after") || !strings.Contains(env.Hint, "last on the card") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// A neighbour is resolved like the step itself, and one that names no step
// or more than one stops the command before anything is written.
func TestStepsMoveRefusesANeighbourItCannotFind(t *testing.T) {
	for name, c := range map[string]struct {
		flags []string
		code  int
	}{
		"a text on two steps": {[]string{"--after", "book the tent"}, ExitAmbiguous},
		"a text on none":      {[]string{"--before", "Nothing like it"}, ExitUsage},
		"a position past all": {[]string{"--after", "4"}, ExitUsage},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			withSteps(e)

			_, r := e.runJSON("", append([]string{"cards", "steps", "move", e.fixture.CardInTriage.ID, "1"}, c.flags...)...)
			expectCode(t, r, c.code)
			if n := len(e.server.WritingRequests()); n != 0 {
				t.Errorf("%d writes sent", n)
			}
		})
	}
}

// The text view names the step where it stands now.
func TestStepsMoveShowsTheStepInItsNewPlace(t *testing.T) {
	e := newTestEnv(t)
	withSteps(e)

	r := e.run("", "cards", "steps", "move", e.fixture.CardInTriage.ID, "1", "--bottom", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, `Moved step "Call the caterer"`) || !strings.Contains(r.stdout, "3. [ ] Call the caterer") {
		t.Errorf("text view:\n%s", r.stdout)
	}
}
