package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestTheSubtasksOfATodoAreListedInTheirOrder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "subtasks", "list", f.GroupedTodo.ID)
	expectCode(t, r, ExitOK)
	subtasks := listOfObjects(t, env.Data)
	if ids := idsIn(subtasks); !slices.Equal(ids, []string{f.GroupedTodo.Subtasks[0].ID, f.GroupedTodo.Subtasks[1].ID}) {
		t.Fatalf("subtasks %v", ids)
	}
	if !strings.Contains(env.Summary, "1 of 2 subtasks done") {
		t.Fatalf("summary %q", env.Summary)
	}
}

func TestASubtaskIsAddedAtTheEndWithItsPersonAndItsDay(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("Lay the cables", "todos", "subtasks", "create", f.GroupedTodo.ID, "-", "--assignee", "me", "--due", "2026-10-02")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["title"] != "Lay the cables" || body["assigneeId"] != f.Bruno.ID || body["dueOn"] != "2026-10-02" {
		t.Fatalf("sent %v", body)
	}
	todo := decode(t, env.Data)
	if ids := idsOf(t, todo, "subtasks"); len(ids) != 3 || f.GroupedTodo.Subtasks[2].Title != "Lay the cables" {
		t.Fatalf("a new subtask is not at the end: %v", ids)
	}

	_, r = e.runJSON("", "todos", "subtasks", "create", f.GroupedTodo.ID, "Plain")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 {
		t.Fatalf("sent %v", body)
	}
}

func TestASubtaskIsNamedByPositionTextOrID(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	cables := f.GroupedTodo.Subtasks[0]

	for _, name := range []string{"1", "find the CABLES", cables.ID} {
		_, r := e.runJSON("", "todos", "subtasks", "edit", f.GroupedTodo.ID, name, "--due", "2026-10-01")
		expectCode(t, r, ExitOK)
		writes := e.server.WritingRequests()
		if last := writes[len(writes)-1]; !strings.HasSuffix(last.Path, "/subtasks/"+cables.ID) {
			t.Fatalf("%q wrote %s", name, last.Path)
		}
	}

	writes := len(e.server.WritingRequests())
	for name, c := range map[string]struct {
		value string
		exit  int
	}{
		"a position beyond": {"3", ExitUsage},
		"a text of none":    {"Tents", ExitUsage},
	} {
		_, r := e.runJSON("", "todos", "subtasks", "complete", f.GroupedTodo.ID, c.value)
		expectCode(t, r, c.exit)
		if len(e.server.WritingRequests()) != writes {
			t.Fatalf("%s: a write was sent", name)
		}
	}

	f.GroupedTodo.Subtasks[1].Title = "Find the cables"
	env, r := e.runJSON("", "todos", "subtasks", "complete", f.GroupedTodo.ID, "Find the cables")
	expectCode(t, r, ExitAmbiguous)
	if !strings.Contains(env.Hint, "Give the position or the ID") {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestAnEditOfASubtaskTakesOffWhatIsGivenEmpty(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	cables := f.GroupedTodo.Subtasks[0]
	day := "2026-10-01"
	cables.DueOn = &day

	_, r := e.runJSON("", "todos", "subtasks", "edit", f.GroupedTodo.ID, "1", "--assignee", "", "--due", "")
	expectCode(t, r, ExitOK)
	raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body
	if string(raw) != `{"assigneeId":null,"dueOn":null}` {
		t.Fatalf("sent %s", raw)
	}
	if cables.AssigneeID != "" || cables.DueOn != nil || cables.Title != "Find the cables" {
		t.Fatalf("subtask %+v", cables)
	}

	_, r = e.runJSON("", "todos", "subtasks", "edit", f.GroupedTodo.ID, "1", "--assignee", "anke@example.com")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["assigneeId"] != f.Anke.ID {
		t.Fatalf("sent %v", body)
	}

	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "todos", "subtasks", "edit", f.GroupedTodo.ID, "1")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("an edit without a change was sent")
	}
	env, r := e.runJSON("", "todos", "subtasks", "edit", f.GroupedTodo.ID, "1", "--title", "")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "subtaskMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestASubtaskIsTickedOffTwiceWithoutAMistake(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "subtasks", "complete", f.GroupedTodo.ID, "1")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["completed"] != true || !f.GroupedTodo.Subtasks[0].Completed {
		t.Fatalf("sent %v", body)
	}
	if !strings.HasPrefix(env.Summary, "Completed") {
		t.Fatalf("summary %q", env.Summary)
	}
	env, r = e.runJSON("", "todos", "subtasks", "complete", f.GroupedTodo.ID, "1")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "already") {
		t.Fatalf("summary %q", env.Summary)
	}
	if f.GroupedTodo.CompletedAt != nil {
		t.Fatal("ticking off every subtask ticked off the to-do")
	}

	runOK(t, e, "todos", "subtasks", "reopen", f.GroupedTodo.ID, "1")
	if body := e.lastWriteBody(); body["completed"] != false || f.GroupedTodo.Subtasks[0].Completed {
		t.Fatalf("sent %v", body)
	}
}

func TestASubtaskMovesWhereItsPlacementSays(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	cables, van := f.GroupedTodo.Subtasks[0].ID, f.GroupedTodo.Subtasks[1].ID
	order := func() []string {
		return idsOf(t, decode(t, runOK(t, e, "todos", "show", f.GroupedTodo.ID).Data), "subtasks")
	}

	runOK(t, e, "todos", "subtasks", "move", f.GroupedTodo.ID, "2", "--top")
	if body := e.lastWriteBody(); len(body) != 0 || !slices.Equal(order(), []string{van, cables}) {
		t.Fatalf("--top sent %v", body)
	}
	runOK(t, e, "todos", "subtasks", "move", f.GroupedTodo.ID, van, "--after", "Find the cables")
	if body := e.lastWriteBody(); body["previousSubtaskId"] != cables || !slices.Equal(order(), []string{cables, van}) {
		t.Fatalf("--after sent %v", body)
	}
	runOK(t, e, "todos", "subtasks", "move", f.GroupedTodo.ID, van, "--before", "1")
	if body := e.lastWriteBody(); body["nextSubtaskId"] != cables || !slices.Equal(order(), []string{van, cables}) {
		t.Fatalf("--before sent %v", body)
	}

	env, r := e.runJSON("", "todos", "subtasks", "move", f.GroupedTodo.ID, van, "--after", van)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoPlaceUnknown || !strings.Contains(env.Hint, "the subtask itself") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	writes := len(e.server.WritingRequests())
	for _, args := range [][]string{
		{"todos", "subtasks", "move", f.GroupedTodo.ID, "1"},
		{"todos", "subtasks", "move", f.GroupedTodo.ID, "1", "--top", "--after", "2"},
		{"todos", "subtasks", "move", f.GroupedTodo.ID, "1", "--after", "Tents"},
	} {
		_, r = e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a move without one known place was sent")
	}
}

func TestASubtaskIsTakenOffAndOneNotThereIsNoMistake(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "subtasks", "delete", f.GroupedTodo.ID, "1")
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["wasOnTodo"] != true || len(f.GroupedTodo.Subtasks) != 1 {
		t.Fatalf("data %v", data)
	}

	env, r = e.runJSON("", "todos", "subtasks", "delete", f.GroupedTodo.ID, unknownID)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["wasOnTodo"] != false || !strings.Contains(env.Summary, "was not on") {
		t.Fatalf("data %v, summary %q", data, env.Summary)
	}

	// A subtask that is not there takes no change.
	env, r = e.runJSON("", "todos", "subtasks", "complete", f.GroupedTodo.ID, unknownID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != subtaskNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

// A subtask is named against the to-do the CLI reads first, so a to-do that is
// not there is said before anything about the subtask.
func TestASubtaskOfATodoThatIsNotThereIsNotFound(t *testing.T) {
	e := newTestEnv(t)

	for _, args := range [][]string{
		{"todos", "subtasks", "list", e.fixture.OtherTodo.ID},
		{"todos", "subtasks", "edit", unknownID, "1", "--title", ""},
		{"todos", "subtasks", "complete", unknownID, "1"},
		{"todos", "subtasks", "delete", unknownID, "1"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if env.Hint != todoNotFoundHint {
			t.Fatalf("%v: hint %q", args, env.Hint)
		}
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Fatalf("%d writes", n)
	}
}
