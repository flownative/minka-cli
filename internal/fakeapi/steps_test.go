package fakeapi

import (
	"reflect"
	"strings"
	"testing"
)

func stepOnTheCard(t *testing.T, answer map[string]any) map[string]any {
	t.Helper()
	steps, _ := answer["steps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("steps of the card: %v", answer["steps"])
	}
	return steps[0].(map[string]any)
}

func assigneeIDsOf(step map[string]any) []string {
	ids := []string{}
	for _, assignee := range step["assignees"].([]any) {
		ids = append(ids, assignee.(map[string]any)["id"].(string))
	}
	return ids
}

// A step shows who is on it and when it is due, and a PATCH leaves alone
// what it does not name, people sent as null included.
func TestAStepCarriesItsPeopleAndItsDueDay(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Check"}
	f.CardInTriage.Steps = []*Step{step}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID

	got, answer := answerTo(t, s, f.Token, "PATCH", path, `{"dueOn":"2026-11-02","assigneeIds":["`+f.Anke.ID+`"]}`)
	if got != "200" {
		t.Fatalf("both: %s", got)
	}
	shown := stepOnTheCard(t, answer)
	if shown["dueOn"] != "2026-11-02" || !reflect.DeepEqual(shown["assignees"], []any{map[string]any{"id": f.Anke.ID, "name": f.Anke.Name}}) {
		t.Errorf("the step shows %v", shown)
	}

	_, answer = answerTo(t, s, f.Token, "PATCH", path, `{"assigneeIds":["`+f.Bruno.ID+`","`+f.Anke.ID+`"]}`)
	if shown := stepOnTheCard(t, answer); shown["dueOn"] != "2026-11-02" || !reflect.DeepEqual(assigneeIDsOf(shown), []string{f.Anke.ID, f.Bruno.ID}) {
		t.Errorf("naming the people took the day away or did not keep Anke's place: %v", shown)
	}

	_, answer = answerTo(t, s, f.Token, "PATCH", path, `{"dueOn":"2026-12-24","assigneeIds":null}`)
	if shown := stepOnTheCard(t, answer); shown["dueOn"] != "2026-12-24" || len(assigneeIDsOf(shown)) != 2 {
		t.Errorf("people sent as null changed the people: %v", shown)
	}

	_, answer = answerTo(t, s, f.Token, "PATCH", path, `{"completed":true,"dueOn":"","assigneeIds":[]}`)
	if shown := stepOnTheCard(t, answer); shown["dueOn"] != nil || len(assigneeIDsOf(shown)) != 0 || shown["completed"] != true {
		t.Errorf("an empty day and an empty set took nothing off, or the tick did not come: %v", shown)
	}

	answerTo(t, s, f.Token, "PATCH", path, `{"dueOn":"2026-12-24"}`)
	_, answer = answerTo(t, s, f.Token, "PATCH", path, `{"dueOn":null}`)
	if shown := stepOnTheCard(t, answer); shown["dueOn"] != nil {
		t.Errorf("null did not take the day off: %v", shown)
	}
}

// Somebody who left the project stays on a step and is named on it, and
// naming them again is no refusal; only who comes new has to be a member.
func TestSomebodyWhoLeftStaysOnAStep(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Check", AssigneeIDs: []string{f.Stranger.ID}}
	f.CardInTriage.Steps = []*Step{step}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID

	got, answer := answerTo(t, s, f.Token, "PATCH", path, `{"assigneeIds":["`+f.Stranger.ID+`","`+f.Anke.ID+`"]}`)
	if got != "200" || !reflect.DeepEqual(assigneeIDsOf(stepOnTheCard(t, answer)), []string{f.Stranger.ID, f.Anke.ID}) {
		t.Errorf("%s: %v", got, answer["steps"])
	}
}

// Minka reads the shape of the people first, then asks whether anything is
// named, then the tick, the day and identifiers that are none, then the
// archive, and only then whether a person is a member.
func TestTheRefusalsOfAStepComeInMinkasOrder(t *testing.T) {
	for _, c := range []struct {
		name, body, want string
		archived         func(s *Server, f *Fixture)
	}{
		{"nothing", `{}`, "422 nothingToChange", nil},
		{"people as null", `{"assigneeIds":null}`, "422 nothingToChange", nil},
		{"an unknown field", `{"name":"Other"}`, "422 nothingToChange", nil},
		{"people that are no list", `{"assigneeIds":"x"}`, "400 malformedBody", nil},
		{"people that are no list beside a word", `{"completed":"yes","assigneeIds":5}`, "400 malformedBody", nil},
		{"a tick that is a word", `{"completed":"yes","dueOn":"tomorrow"}`, "422 stepCompletedMissing", nil},
		{"a tick that is null", `{"completed":null}`, "422 stepCompletedMissing", nil},
		{"a line that is a number", `{"title":5,"completed":true}`, "400 malformedBody", nil},
		{"a line that is a number beside a word", `{"title":5,"completed":"yes"}`, "422 stepCompletedMissing", nil},
		{"a line that is null", `{"title":null}`, "422 stepMissing", nil},
		{"a line of blanks", `{"title":"  ","dueOn":"tomorrow"}`, "422 stepMissing", nil},
		{"a line too long", `{"title":"LONG","dueOn":"tomorrow"}`, "422 stepTooLong", nil},
		{"a line too long with a tab inside", `{"title":"LONG\tx"}`, "422 stepTooLong", nil},
		{"a line with a tab", `{"title":"Check\tagain"}`, "422 stepHasControlCharacters", archiveTheCard},
		{"a line that is none beside a stranger", `{"title":"","assigneeIds":["STRANGER"]}`, "422 stepMissing", nil},
		{"a line on an archived card", `{"title":"Other"}`, "422 cardArchived", archiveTheCard},
		{"the same line in an archived project", `{"title":"Check"}`, "422 projectArchived", archiveTheProject},
		{"a line beside a stranger", `{"title":"Other","assigneeIds":["STRANGER"]}`, "422 personNotAProjectMember", nil},
		{"a day that is a number", `{"dueOn":5}`, "400 malformedBody", nil},
		{"a day that is none", `{"dueOn":"2026-02-30","assigneeIds":["nobody"]}`, "422 stepDueDateNotADate", nil},
		{"a person that is no ID", `{"assigneeIds":["nobody"]}`, "422 personNotAProjectMember", archiveTheProject},
		{"a stranger in an archived project", `{"assigneeIds":["STRANGER"]}`, "422 projectArchived", archiveTheProject},
		{"a stranger on an archived card", `{"assigneeIds":["STRANGER"]}`, "422 cardArchived", archiveTheCard},
		{"a day on an archived card", `{"dueOn":"2026-11-02"}`, "422 cardArchived", archiveTheCard},
		{"a stranger", `{"assigneeIds":["STRANGER"],"dueOn":"2026-11-02","completed":true}`, "422 personNotAProjectMember", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			step := &Step{ID: newID(), Title: "Check"}
			f.CardInTriage.Steps = []*Step{step}
			path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID
			if c.archived != nil {
				c.archived(s, f)
			}
			body := strings.Replace(strings.Replace(c.body, "STRANGER", f.Stranger.ID, 1), "LONG", strings.Repeat("x", 501), 1)
			if got, _ := answerTo(t, s, f.Token, "PATCH", path, body); got != c.want {
				t.Errorf("%s: %s, want %s", body, got, c.want)
			}
			if step.DueOn != nil || len(step.AssigneeIDs) != 0 || step.Completed || step.Title != "Check" {
				t.Errorf("a refused write changed the step: %+v", step)
			}
		})
	}
}

func archiveTheProject(_ *Server, f *Fixture) { f.Project.Archived = true }

func archiveTheCard(s *Server, f *Fixture) { s.PutInTheArchive(f.CardInTriage) }

// A line is trimmed and kept as Minka keeps it, and it comes after the people
// and the day and before the tick, all in one request.
func TestAStepIsRenamedWithItsOtherChanges(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Check"}
	f.CardInTriage.Steps = []*Step{step}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID

	got, answer := answerTo(t, s, f.Token, "PATCH", path, `{"title":"  Check twice  ","completed":true,"dueOn":"2026-11-02"}`)
	if shown := stepOnTheCard(t, answer); got != "200" || shown["title"] != "Check twice" || shown["completed"] != true || shown["dueOn"] != "2026-11-02" {
		t.Errorf("%s: %v", got, shown)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", path, `{"title":"`+strings.Repeat("ä", 500)+`"}`); got != "200" || step.Title != strings.Repeat("ä", 500) {
		t.Errorf("500 characters of two bytes each: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", path, `{"title":"`+step.Title+`"}`); got != "200" {
		t.Errorf("the same line again: %s", got)
	}
}

func stepTitlesOf(card *Card) []string {
	titles := []string{}
	for _, step := range card.Steps {
		titles = append(titles, step.Title)
	}
	return titles
}

// A step moves as on the web: behind the previous step, in front of the next,
// to the top without either, and a previous step decides over a next one that
// says otherwise.
func TestAStepMovesBetweenItsNeighbours(t *testing.T) {
	s, f := New(t)
	first, second, third := &Step{ID: newID(), Title: "First"}, &Step{ID: newID(), Title: "Second"}, &Step{ID: newID(), Title: "Third"}
	f.CardInTriage.Steps = []*Step{first, second, third}
	moves := func(step *Step) string { return "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + step.ID + "/moves" }

	for _, c := range []struct {
		name  string
		step  *Step
		body  string
		order []string
	}{
		{"between two", first, `{"previousStepId":"` + second.ID + `","nextStepId":"` + third.ID + `"}`, []string{"Second", "First", "Third"}},
		{"to the end", second, `{"previousStepId":"` + third.ID + `"}`, []string{"First", "Third", "Second"}},
		{"in front of one", third, `{"nextStepId":"` + first.ID + `"}`, []string{"Third", "First", "Second"}},
		{"to the top", second, `{}`, []string{"Second", "Third", "First"}},
		{"where it stands", second, `{"previousStepId":null,"nextStepId":""}`, []string{"Second", "Third", "First"}},
		{"neighbours that disagree", second, `{"previousStepId":"` + first.ID + `","nextStepId":"` + third.ID + `"}`, []string{"Third", "First", "Second"}},
		{"in capitals", first, `{"nextStepId":"` + strings.ToUpper(third.ID) + `"}`, []string{"First", "Third", "Second"}},
	} {
		got, answer := answerTo(t, s, f.Token, "POST", moves(c.step), c.body)
		if got != "200" || !reflect.DeepEqual(stepTitlesOf(f.CardInTriage), c.order) {
			t.Errorf("%s: %s, %v", c.name, got, stepTitlesOf(f.CardInTriage))
		}
		if steps, _ := answer["steps"].([]any); len(steps) != 3 || steps[0].(map[string]any)["title"] != c.order[0] {
			t.Errorf("%s: the card answered %v", c.name, answer["steps"])
		}
	}
}

// Minka looks for the card and the step first, then reads the body, the
// previous neighbour before the next, tells the archive before a wrong
// neighbour, and the archive again before it moves.
func TestTheRefusalsOfAMoveComeInMinkasOrder(t *testing.T) {
	for _, c := range []struct {
		name, step, body, want string
		archived               func(s *Server, f *Fixture)
	}{
		{"a step that is none", "nonsense", `{}`, "404", nil},
		{"a step of another card", "OTHER", `{}`, "404", nil},
		{"a step that is gone in an archived project", "GONE", `nonsense`, "404", archiveTheProject},
		{"a body that is no JSON", "SELF", `nonsense`, "400 malformedBody", archiveTheProject},
		{"a neighbour that is a number", "SELF", `{"previousStepId":5,"nextStepId":"SELF"}`, "400 malformedBody", nil},
		{"the step itself", "SELF", `{"nextStepId":"SELF"}`, "422 stepPlaceUnknown", nil},
		{"a step of another card as neighbour", "SELF", `{"previousStepId":"OTHER"}`, "422 stepPlaceUnknown", nil},
		{"a neighbour that is none", "SELF", `{"previousStepId":"nonsense"}`, "422 stepPlaceUnknown", nil},
		{"a wrong previous beside a next that is a number", "SELF", `{"previousStepId":"SELF","nextStepId":5}`, "422 stepPlaceUnknown", nil},
		{"a wrong neighbour in an archived project", "SELF", `{"previousStepId":"GONE"}`, "422 projectArchived", archiveTheProject},
		{"a wrong neighbour on an archived card", "SELF", `{"previousStepId":"GONE"}`, "422 cardArchived", archiveTheCard},
		{"to the top on an archived card", "SELF", `{}`, "422 cardArchived", archiveTheCard},
		{"to the top in an archived project", "SELF", `{"nextStepId":""}`, "422 projectArchived", archiveTheProject},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			first, second := &Step{ID: newID(), Title: "First"}, &Step{ID: newID(), Title: "Second"}
			f.CardInTriage.Steps = []*Step{first, second}
			f.CardInDoing.Steps = []*Step{{ID: newID(), Title: "Elsewhere"}}
			if c.archived != nil {
				c.archived(s, f)
			}
			named := strings.NewReplacer("SELF", second.ID, "OTHER", f.CardInDoing.Steps[0].ID, "GONE", newID())
			path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + named.Replace(c.step) + "/moves"
			if got, _ := answerTo(t, s, f.Token, "POST", path, named.Replace(c.body)); got != c.want {
				t.Errorf("%s: %s, want %s", c.body, got, c.want)
			}
			if titles := stepTitlesOf(f.CardInTriage); !reflect.DeepEqual(titles, []string{"First", "Second"}) {
				t.Errorf("a refused move moved: %v", titles)
			}
		})
	}
}

// The step of the address is read in any case, as Minka reads it.
func TestAStepInCapitalsIsMoved(t *testing.T) {
	s, f := New(t)
	first, second := &Step{ID: newID(), Title: "First"}, &Step{ID: newID(), Title: "Second"}
	f.CardInTriage.Steps = []*Step{first, second}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + strings.ToUpper(second.ID) + "/moves"
	if got, _ := answerTo(t, s, f.Token, "POST", path, `{}`); got != "200" || !reflect.DeepEqual(stepTitlesOf(f.CardInTriage), []string{"Second", "First"}) {
		t.Errorf("%s: %v", got, stepTitlesOf(f.CardInTriage))
	}
}

// A card the person may not see answers a move as one that is not there.
func TestAMoveOnACardOfAnotherProjectIsNotFound(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Theirs"}
	f.OtherCard.Steps = []*Step{step}
	path := "/api/v1/cards/" + f.OtherCard.ID + "/steps/" + step.ID
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/moves", `{}`); got != "404" {
		t.Errorf("moving: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", path, `{"title":"Mine"}`); got != "404" || step.Title != "Theirs" {
		t.Errorf("renaming: %s, %q", got, step.Title)
	}
}
