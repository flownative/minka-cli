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
		{"an unknown field", `{"title":"Other"}`, "422 nothingToChange", nil},
		{"people that are no list", `{"assigneeIds":"x"}`, "400 malformedBody", nil},
		{"people that are no list beside a word", `{"completed":"yes","assigneeIds":5}`, "400 malformedBody", nil},
		{"a tick that is a word", `{"completed":"yes","dueOn":"tomorrow"}`, "422 stepCompletedMissing", nil},
		{"a tick that is null", `{"completed":null}`, "422 stepCompletedMissing", nil},
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
			body := strings.Replace(c.body, "STRANGER", f.Stranger.ID, 1)
			if got, _ := answerTo(t, s, f.Token, "PATCH", path, body); got != c.want {
				t.Errorf("%s: %s, want %s", body, got, c.want)
			}
			if step.DueOn != nil || len(step.AssigneeIDs) != 0 || step.Completed {
				t.Errorf("a refused write changed the step: %+v", step)
			}
		})
	}
}

func archiveTheProject(_ *Server, f *Fixture) { f.Project.Archived = true }

func archiveTheCard(s *Server, f *Fixture) { s.PutInTheArchive(f.CardInTriage) }
