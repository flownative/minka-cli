package fakeapi

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// A to-do becomes a step by its ID in any case, reads as it stands with
// exactly the keys Minka hands out, follows the to-do's tick, and a second
// time is 200 without a second step.
func TestATodoBecomesAStepAndReadsAsItStands(t *testing.T) {
	s, f := New(t)
	todo := f.LooseTodo
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"

	got, step := answerTo(t, s, f.Token, "POST", path, `{"todoId":" `+strings.ToUpper(todo.ID)+` "}`)
	if got != "201" {
		t.Fatalf("the to-do step: %s %v", got, step)
	}
	want := map[string]any{
		"id": f.CardInTriage.Steps[0].ID, "title": todo.Title, "completed": false, "assignees": []any{}, "dueOn": nil, "card": nil,
		"todo": map[string]any{
			"kind": "todo", "id": todo.ID, "title": todo.Title, "projectId": f.Project.ID, "toolId": f.Todos.ID,
			"listId": f.TodoList.ID, "completed": false, "appUrl": s.todoURL(f.Todos, todo),
		},
	}
	if !reflect.DeepEqual(step, want) {
		t.Fatalf("the step reads %v, want %v", step, want)
	}

	got, again := answerTo(t, s, f.Token, "POST", path, `{"todoId":"`+todo.ID+`"}`)
	if got != "200" || !reflect.DeepEqual(again, step) || len(f.CardInTriage.Steps) != 1 {
		t.Fatalf("again: %s, %v", got, again)
	}

	done := time.Now()
	todo.CompletedAt = &done
	_, card := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	shown := stepOnTheCard(t, card)
	if shown["completed"] != true || shown["todo"].(map[string]any)["completed"] != true {
		t.Errorf("the step of a done to-do reads %v", shown)
	}
	_, board := answerTo(t, s, f.Token, "GET", "/api/v1/boards/"+f.Board.ID, "")
	if tile := tileOf(board, f.CardInTriage.ID); tile["stepCount"] != 1.0 || tile["completedStepCount"] != 1.0 {
		t.Errorf("the tile counts %v of %v", tile["completedStepCount"], tile["stepCount"])
	}
}

// Minka refuses a body naming a card and a to-do before it looks at either,
// then words beside a to-do, the shape of the name, and a to-do the person
// may not see; only then is the archive told.
func TestATodoStepIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"
	trashed := *f.DoneTodo
	trashed.ID, trashed.Trashed = newID(), true
	f.TodoList.Todos = append(f.TodoList.Todos, &trashed)

	for body, want := range map[string]string{
		`{"todoId":"` + f.LooseTodo.ID + `","title":"x"}`:  "422 stepTitleAndTodo",
		`{"todoId":"` + f.LooseTodo.ID + `","title":null}`: "422 stepTitleAndTodo",
		`{"todoId":1,"title":"x"}`:                         "422 stepTitleAndTodo",
		`{"todoId":1}`:                                     "400 malformedBody",
		`{"todoId":null}`:                                  "422 stepTodoUnknown",
		`{"todoId":""}`:                                    "422 stepTodoUnknown",
		`{"todoId":"Buy pegs"}`:                            "422 stepTodoUnknown",
		`{"todoId":"` + newID() + `"}`:                     "422 stepTodoUnknown",
		`{"todoId":"` + f.OtherTodo.ID + `"}`:              "422 stepTodoUnknown",
		`{"todoId":"` + trashed.ID + `"}`:                  "422 stepTodoUnknown",
		`{"todoId":"` + f.LooseTodo.ID + `","cardId":"` + f.CardInDoing.ID + `"}`: "422 stepCardAndTodo",
		`{"todoId":null,"cardId":null,"title":"x"}`:                               "422 stepCardAndTodo",
	} {
		if got, _ := answerTo(t, s, f.Token, "POST", path, body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if len(f.CardInTriage.Steps) != 0 {
		t.Fatalf("a refusal wrote %v", f.CardInTriage.Steps)
	}

	s.PutInTheArchive(f.CardInTriage)
	if got, _ := answerTo(t, s, f.Token, "POST", path, `{"todoId":"`+newID()+`"}`); got != "422 stepTodoUnknown" {
		t.Errorf("an unknown to-do on an archived card: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path, `{"todoId":"`+f.LooseTodo.ID+`"}`); got != "422 cardArchived" {
		t.Errorf("on an archived card: %s", got)
	}
}

// The to-do of another project the person is a member of becomes a step, and
// so does one in a list in the archive and one that is done.
func TestATodoOfAnotherProjectBecomesAStep(t *testing.T) {
	s, f := New(t)
	elsewhere := &Project{ID: newID(), Name: "Shopping", MemberIDs: []string{f.Bruno.ID}}
	s.Projects = append(s.Projects, elsewhere)
	pegs := &Todo{ID: newID(), Title: "Buy pegs", CreatedAt: time.Now(), CreatorID: f.Bruno.ID}
	s.TodosTools = append(s.TodosTools, &TodosTool{ID: newID(), ProjectID: elsewhere.ID, Title: "To-dos", Lists: []*TodoList{{ID: newID(), Title: "Camp", Todos: []*Todo{pegs}}}})
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps"

	for _, c := range []struct {
		name string
		todo *Todo
	}{{"another project", pegs}, {"a list in the archive", f.ArchivedListTodo}, {"a done to-do", f.DoneTodo}} {
		got, step := answerTo(t, s, f.Token, "POST", path, `{"todoId":"`+c.todo.ID+`"}`)
		linked, _ := step["todo"].(map[string]any)
		if got != "201" || linked["id"] != c.todo.ID || step["completed"] != (c.todo.CompletedAt != nil) {
			t.Errorf("%s: %s %v", c.name, got, step)
		}
	}
	_, card := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	if first := card["steps"].([]any)[0].(map[string]any)["todo"].(map[string]any); first["projectId"] != elsewhere.ID {
		t.Errorf("the to-do does not name its own project: %v", first)
	}
}

// A to-do the reader may not see, and one in the trash for everybody, is only
// a to-do: no title, no tick, even where the to-do was done.
func TestAnUnseenOrTrashedTodoIsOnlyATodo(t *testing.T) {
	s, f := New(t)
	done := time.Now()
	f.LooseTodo.CompletedAt = &done
	f.LooseTodo.Trashed = true
	for name, todo := range map[string]*Todo{"unseen": f.OtherTodo, "in the trash": f.LooseTodo} {
		step := &Step{ID: newID(), TodoID: todo.ID}
		f.CardInTriage.Steps = []*Step{step}

		_, card := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
		want := map[string]any{"id": step.ID, "title": "", "completed": false, "assignees": []any{}, "dueOn": nil, "card": nil, "todo": map[string]any{"kind": "todo"}}
		if shown := stepOnTheCard(t, card); !reflect.DeepEqual(shown, want) {
			t.Errorf("%s: the step reads %v", name, shown)
		}
		_, board := answerTo(t, s, f.Token, "GET", "/api/v1/boards/"+f.Board.ID, "")
		if tile := tileOf(board, f.CardInTriage.ID); tile["completedStepCount"] != 0.0 {
			t.Errorf("%s: the tile counts %v done", name, tile["completedStepCount"])
		}
	}
}

// A to-do step takes no change of its own, refused before the body is read,
// seen or not; it does not become a card, which the archive of the card is
// told before; it moves and comes off as any step, and the to-do stays.
func TestATodoStepIsNeitherTickedNorRenamedNorMadeACard(t *testing.T) {
	s, f := New(t)
	words := &Step{ID: newID(), Title: "Measure"}
	linked := &Step{ID: newID(), TodoID: f.LooseTodo.ID}
	unseen := &Step{ID: newID(), TodoID: f.OtherTodo.ID}
	f.CardInTriage.Steps = []*Step{words, linked, unseen}
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + linked.ID

	for _, address := range []string{path, "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + unseen.ID} {
		for _, body := range []string{`{"completed":true}`, `not even JSON`} {
			if got, _ := answerTo(t, s, f.Token, "PATCH", address, body); got != "422 stepIsATodo" {
				t.Errorf("%s: %s", body, got)
			}
		}
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/card", ""); got != "422 stepIsATodo" || linked.CardID != "" {
		t.Errorf("made into a card: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", path+"/moves", `{}`); got != "200" || f.CardInTriage.Steps[0] != linked {
		t.Errorf("the move: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "DELETE", path, ""); got != "204" || len(f.CardInTriage.Steps) != 2 {
		t.Errorf("the removal: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "GET", "/api/v1/todos/"+f.LooseTodo.ID, ""); got != "200" {
		t.Errorf("the to-do went with the link: %s", got)
	}

	s.PutInTheArchive(f.CardInTriage)
	address := "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + unseen.ID
	if got, _ := answerTo(t, s, f.Token, "PATCH", address, `{"completed":true}`); got != "422 stepIsATodo" {
		t.Errorf("a change on an archived card: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", address+"/card", ""); got != "422 cardArchived" {
		t.Errorf("made into a card on an archived card: %s", got)
	}
}

// A Minka from before to-do steps passes over the to-do and asks for words,
// and its steps carry no to-do.
func TestAMinkaWithoutTodoSteps(t *testing.T) {
	s, f := New(t)
	s.KnowsNoTodoSteps = true
	f.CardInTriage.Steps = []*Step{{ID: newID(), Title: "Measure"}}

	if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/steps", `{"todoId":"`+f.LooseTodo.ID+`"}`); got != "422 stepMissing" {
		t.Errorf("a to-do step: %s", got)
	}
	_, card := answerTo(t, s, f.Token, "GET", "/api/v1/cards/"+f.CardInTriage.ID, "")
	shown := stepOnTheCard(t, card)
	if _, there := shown["todo"]; there {
		t.Error("a step carries todo")
	}
	if _, there := shown["card"]; !there {
		t.Error("a step carries no card")
	}
}
