package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// The first time is an addition, every other one says the to-do was a step
// already; the body names the to-do alone, and the data is the step.
func TestATodoStepSaysWhetherItWasNew(t *testing.T) {
	e := newTestEnv(t)
	todo := e.fixture.LooseTodo
	parent := e.fixture.CardInTriage
	parent.ShortID = "MNK-12"

	env, r := e.runJSON("", "cards", "steps", "create", parent.ID, "--todo", todo.ID)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	linked, _ := data["todo"].(map[string]any)
	if env.Summary != `Added to-do "`+todo.Title+`" as a step of card `+parent.ID || data["id"] != parent.Steps[0].ID || linked["id"] != todo.ID {
		t.Fatalf("summary %q, data %s", env.Summary, env.Data)
	}
	if body := e.lastWriteBody(); len(body) != 1 || body["todoId"] != todo.ID {
		t.Errorf("the body went out as %v", body)
	}
	for _, cmd := range []string{"minka todos show " + todo.ID, "minka todos complete " + todo.ID} {
		if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Cmd == cmd }) {
			t.Errorf("breadcrumbs %v lack %q", env.Breadcrumbs, cmd)
		}
	}

	r = e.run("", "cards", "steps", "create", "mnk-12", "--todo", todo.ID, "--styled")
	expectCode(t, r, ExitOK)
	want := `To-do "` + todo.Title + `" was a step of card #MNK-12 already.` + "\n↳ to-do [ ] " + todo.Title + "  " + e.server.URL
	if !strings.HasPrefix(r.stdout, want) || !strings.HasSuffix(r.stdout, "/todos/"+todo.ID+"\n") || len(parent.Steps) != 1 {
		t.Errorf("text %q, want it to start with %q", r.stdout, want)
	}
}

// A step is a text, a card or a to-do: two of them, and a to-do named by
// anything but an ID, are refused before any request, since Minka would take
// the card and pass over the to-do without a word.
func TestATodoStepBesideATextOrACardIsAUsageError(t *testing.T) {
	e := newTestEnv(t)
	parent, todo := e.fixture.CardInTriage, e.fixture.LooseTodo
	before := len(e.server.Requests())

	for _, c := range []struct {
		want string
		args []string
	}{
		{"give either --card or --todo, not both", []string{"--card", e.fixture.CardInDoing.ID, "--todo", todo.ID}},
		{"give either --card or --todo, not both", []string{"A text", "--card", e.fixture.CardInDoing.ID, "--todo", todo.ID}},
		{"give either a text or --todo, not both", []string{"A text", "--todo", todo.ID}},
		{`--todo "Buy pegs" is not an ID`, []string{"--todo", "Buy pegs"}},
		{`--todo "" is not an ID`, []string{"--todo", ""}},
		{`--todo "MNK-1A" is not an ID`, []string{"--todo", "MNK-1A"}},
	} {
		env, r := e.runJSON("", append([]string{"cards", "steps", "create", parent.ID}, c.args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, c.want) {
			t.Errorf("%v: error %q, want %q", c.args, env.Error, c.want)
		}
	}
	if sent := e.server.Requests()[before:]; len(sent) != 0 || len(parent.Steps) != 0 {
		t.Fatalf("%d requests went out, the card carries %v", len(sent), parent.Steps)
	}
}

// A to-do nobody here can see is refused by Minka, and the hint says how a
// to-do is named.
func TestATodoStepOfAnUnseenTodoIsRefused(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "cards", "steps", "create", e.fixture.CardInTriage.ID, "--todo", e.fixture.OtherTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepTodoUnknown" || env.Hint != stepTodoUnknownHint {
		t.Errorf("%v, hint %q", env.Meta, env.Hint)
	}
}

// A to-do step reads as its to-do, with the to-do's tick and address; one
// the reader cannot see says so; the summary names it by its to-do or by its
// position; and --json hands the todo of every step on unchanged.
func TestTheTextViewsOfATodoStep(t *testing.T) {
	e := newTestEnv(t)
	todo, done := e.fixture.LooseTodo, e.fixture.DoneTodo
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Measure"}, {ID: newTestID(), TodoID: todo.ID}, {ID: newTestID(), TodoID: done.ID}, {ID: newTestID(), TodoID: e.fixture.OtherTodo.ID}}

	for _, command := range [][]string{{"cards", "steps", "list"}, {"cards", "show"}} {
		r := e.run("", append(command, parent.ID, "--styled")...)
		expectCode(t, r, ExitOK)
		for _, line := range []string{
			"1. [ ] Measure",
			"2. ↳ to-do [ ] " + todo.Title + "  " + e.server.URL,
			"/todos/" + todo.ID,
			"3. ↳ to-do [x] " + done.Title + "  " + e.server.URL,
			"4. ↳ (a to-do you cannot open)",
		} {
			if !strings.Contains(r.stdout, line) {
				t.Errorf("%s lacks %q:\n%s", command[len(command)-1], line, r.stdout)
			}
		}
	}
	r := e.run("", "cards", "show", parent.ID, "--styled")
	if !strings.Contains(r.stdout, "## Steps (1/4)") {
		t.Errorf("the card counts its steps as:\n%s", r.stdout)
	}

	env, r := e.runJSON("", "cards", "steps", "list", parent.ID)
	expectCode(t, r, ExitOK)
	var steps []map[string]any
	if err := json.Unmarshal(env.Data, &steps); err != nil || len(steps) != 4 {
		t.Fatalf("the data reads %s", env.Data)
	}
	if linked, _ := steps[1]["todo"].(map[string]any); linked["listId"] != e.fixture.TodoList.ID || linked["toolId"] != e.fixture.Todos.ID || steps[0]["todo"] != nil {
		t.Errorf("the data reads %s", env.Data)
	}
	if unseen, _ := steps[3]["todo"].(map[string]any); len(unseen) != 1 || unseen["kind"] != "todo" {
		t.Errorf("the unseen to-do reads %v", steps[3]["todo"])
	}

	env, r = e.runJSON("", "cards", "steps", "move", parent.ID, "2", "--top")
	expectCode(t, r, ExitOK)
	if want := `Moved to-do step "` + todo.Title + `" on card ` + parent.Title; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	env, r = e.runJSON("", "cards", "steps", "move", parent.ID, "4", "--bottom")
	expectCode(t, r, ExitOK)
	if want := "Moved step 4, a to-do you cannot open on card " + parent.Title; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
}

// A to-do step is named by its to-do's title, comes off without the to-do,
// and one the reader cannot see comes off by its position.
func TestATodoStepComesOffAndTheTodoStays(t *testing.T) {
	e := newTestEnv(t)
	todo := e.fixture.LooseTodo
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), TodoID: todo.ID}, {ID: newTestID(), TodoID: e.fixture.OtherTodo.ID}}

	env, r := e.runJSON("", "cards", "steps", "delete", parent.ID, strings.ToUpper(todo.Title))
	expectCode(t, r, ExitOK)
	if want := `Took to-do "` + todo.Title + `" off the steps of card ` + parent.Title + "; the to-do itself stays"; env.Summary != want || len(parent.Steps) != 1 {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	env, r = e.runJSON("", "cards", "steps", "delete", parent.ID, "1")
	expectCode(t, r, ExitOK)
	if want := "Took a to-do you cannot open off the steps of card " + parent.Title + "; the to-do itself stays"; env.Summary != want || len(parent.Steps) != 0 {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	_, r = e.runJSON("", "todos", "show", todo.ID)
	expectCode(t, r, ExitOK)
}

// A to-do step carries nothing of its own, and the refusal names the command
// that does on the to-do what was asked of the step. Ticking the to-do is
// what ticks the step.
func TestATodoStepIsChangedOnItsTodo(t *testing.T) {
	e := newTestEnv(t)
	todo := e.fixture.LooseTodo
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), TodoID: todo.ID}, {ID: newTestID(), TodoID: e.fixture.OtherTodo.ID}}

	for _, c := range []struct {
		command string
		args    []string
	}{
		{"minka todos complete " + todo.ID, []string{"cards", "steps", "complete", parent.ID, "1"}},
		{"minka todos reopen " + todo.ID, []string{"cards", "steps", "reopen", parent.ID, "1"}},
		{"minka todos edit " + todo.ID, []string{"cards", "steps", "edit", parent.ID, "1", "--due", "2026-12-01"}},
	} {
		env, r := e.runJSON("", c.args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "stepIsATodo" || !strings.HasSuffix(env.Hint, c.command) {
			t.Errorf("%s: %v, hint %q", c.args[2], env.Meta, env.Hint)
		}
	}
	if todo.CompletedAt != nil {
		t.Fatal("a refused step ticked the to-do")
	}

	for _, args := range [][]string{
		{"cards", "steps", "complete", parent.ID, "2"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if !strings.Contains(env.Hint, "minka cards steps delete "+parent.ID+" 2") || strings.Contains(env.Hint, "minka todos") {
			t.Errorf("%s on an unseen to-do: hint %q", args[2], env.Hint)
		}
	}

	_, r := e.runJSON("", "todos", "complete", todo.ID)
	expectCode(t, r, ExitOK)
	r = e.run("", "cards", "steps", "list", parent.ID, "--styled")
	if !strings.Contains(r.stdout, "1. ↳ to-do [x] "+todo.Title) {
		t.Errorf("the step does not follow its to-do:\n%s", r.stdout)
	}
}

// A Minka without to-do steps answers --todo with the missing text: the
// command says the host does not know to-do steps, as a feature that is not
// there, and the absence of todo breaks no view.
func TestAMinkaWithoutTodoSteps(t *testing.T) {
	e := newTestEnv(t)
	e.server.KnowsNoTodoSteps = true
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Measure"}}

	env, r := e.runJSON("", "cards", "steps", "create", parent.ID, "--todo", e.fixture.LooseTodo.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "does not know a to-do as the step of a card") || env.Hint != todoStepsUnknownHint || len(parent.Steps) != 1 {
		t.Errorf("%q, hint %q", env.Error, env.Hint)
	}
	r = e.run("", "cards", "show", parent.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "1. [ ] Measure") {
		t.Errorf("the card reads:\n%s", r.stdout)
	}
}

// A to-do does not become a card, and the refusal says so without sending
// anybody to tick the to-do off, seen or not.
func TestATodoStepDoesNotBecomeACard(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), TodoID: e.fixture.LooseTodo.ID}, {ID: newTestID(), TodoID: e.fixture.OtherTodo.ID}}

	for _, step := range []string{"1", "2"} {
		env, r := e.runJSON("", "cards", "steps", "make-card", parent.ID, step)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "stepIsATodo" || !strings.Contains(env.Hint, "a to-do does not become a card") || strings.Contains(env.Hint, "minka todos") {
			t.Errorf("step %s: %v, hint %q", step, env.Meta, env.Hint)
		}
	}
	if parent.Steps[0].CardID != "" || e.fixture.LooseTodo.CompletedAt != nil {
		t.Fatal("the refusal changed the step or the to-do")
	}
}

// A card in the archive of its board takes no to-do step, and the hint names
// the command that brings it back.
func TestATodoStepOnAnArchivedCardSaysHowToBringItBack(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	e.server.PutInTheArchive(parent)

	env, r := e.runJSON("", "cards", "steps", "create", parent.ID, "--todo", e.fixture.LooseTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "cardArchived" || !strings.HasSuffix(env.Hint, "minka cards unarchive "+parent.ID) {
		t.Errorf("%v, hint %q", env.Meta, env.Hint)
	}
}
