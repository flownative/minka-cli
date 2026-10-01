package cli

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestATodoIsReadWithItsPlaceItsPeopleAndItsSubtasks(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "show", f.GroupedTodo.ID)
	expectCode(t, r, ExitOK)
	todo := decode(t, env.Data)
	if todo["listId"] != f.TodoList.ID || todo["groupId"] != f.StageGroup.ID || todo["dueOn"] != "2026-09-10" || todo["isOverdue"] != true {
		t.Fatalf("to-do %v", todo)
	}
	if ids := idsOf(t, todo, "assignees"); !slices.Equal(ids, []string{f.Bruno.ID}) {
		t.Fatalf("assignees %v", ids)
	}
	if ids := idsOf(t, todo, "subtasks"); len(ids) != 2 || todo["listArchived"] != false {
		t.Fatalf("subtasks %v, listArchived %v", ids, todo["listArchived"])
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "complete"), "todos complete "+f.GroupedTodo.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	text := e.run("", "todos", "show", f.GroupedTodo.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{
		"# Fetch the speakers",
		"Group: Stage (" + f.StageGroup.ID + ")",
		"Due: 2026-09-10 (overdue)",
		"Assigned: Bruno",
		"## Subtasks (1 of 2 done)",
		"1. [ ] Find the cables (Anke)",
		"2. [x] Book the van",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}
}

func TestATodoOfAnArchivedListIsReadAndSaysSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "show", f.ArchivedListTodo.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["listArchived"] != true || !strings.Contains(env.Summary, "archive") {
		t.Fatalf("summary %q", env.Summary)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "unarchive"), "todolists unarchive "+f.ArchivedList.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

func TestATodoNobodyMayReadIsAtNoAddress(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.KitchenGroup.Trashed = true
	f.LooseTodo.Trashed = true

	for name, id := range map[string]string{
		"of another project": f.OtherTodo.ID,
		"unknown":            unknownID,
		"in the trash":       f.LooseTodo.ID,
	} {
		env, r := e.runJSON("", "todos", "show", id)
		expectCode(t, r, ExitNotFound)
		if env.Hint != todoNotFoundHint {
			t.Fatalf("%s: hint %q", name, env.Hint)
		}
	}

	f.StageGroup.Trashed = true
	_, r := e.runJSON("", "todos", "show", f.GroupedTodo.ID)
	expectCode(t, r, ExitNotFound)

	requests := len(e.server.Requests())
	_, r = e.runJSON("", "todos", "show", "Fetch the speakers")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != requests {
		t.Fatal("a to-do was asked for by something that is no ID")
	}
}

func TestATodoIsAddedAtTheEndOfItsGroupWithEverythingGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("Bring **cables**.", "todos", "create", "--list", f.TodoList.ID, "--group", f.StageGroup.ID, "--title", " Hang the lights ",
		"--notes", "-", "--starts-on", "2026-10-01", "--due", "2026-10-03",
		"--assignee", "me", "--assignee", "anke@example.com", "--assignee", "me", "--notify-when-done", "Anke")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	want := map[string]any{
		"title": " Hang the lights ", "groupId": f.StageGroup.ID, "notes": "<p>Bring <strong>cables</strong>.</p>",
		"startsOn": "2026-10-01", "dueOn": "2026-10-03",
	}
	for key, value := range want {
		if body[key] != value {
			t.Fatalf("sent %s = %v, want %v", key, body[key], value)
		}
	}
	// A person named twice goes out twice: taking them once is Minka's rule.
	if people := stringsIn(body["assigneeIds"]); !slices.Equal(people, []string{f.Bruno.ID, f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("sent assignees %v", people)
	}
	if people := stringsIn(body["notifiedIds"]); !slices.Equal(people, []string{f.Anke.ID}) {
		t.Fatalf("sent the people told when done as %v", people)
	}

	created := decode(t, env.Data)
	if created["title"] != "Hang the lights" || created["groupId"] != f.StageGroup.ID || env.Notice != "" {
		t.Fatalf("created %v, notice %q", created, env.Notice)
	}
	if ids := idsOf(t, created, "assignees"); !slices.Equal(ids, []string{f.Bruno.ID, f.Anke.ID}) {
		t.Fatalf("assignees %v: Minka takes a person named twice once, in the order first named", ids)
	}
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPost || !strings.HasSuffix(last.Path, "/todo-lists/"+f.TodoList.ID+"/todos") {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}
	stage := decode(t, rawAt(t, decode(t, runOK(t, e, "todolists", "show", f.TodoList.ID).Data), "groups", 0))
	if ids := idsOf(t, stage, "todos"); !slices.Equal(ids, []string{f.GroupedTodo.ID, created["id"].(string)}) {
		t.Fatalf("a new to-do is not at the end of its group: %v", ids)
	}
}

func TestATodoOfOnlyATitleSendsNothingElseAndAsksForNoPeople(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	_, r := e.runJSON("", "todos", "create", "--list", f.TodoList.ID, "--title", "Plain", "--assignee", f.Anke.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 2 || body["title"] != "Plain" {
		t.Fatalf("sent %v", body)
	}
	for _, request := range e.server.Requests() {
		if request.Method == http.MethodGet {
			t.Fatalf("an ID was looked up although it goes out as it is: %s", request.Path)
		}
	}
}

func TestADayMinkaKeepsOtherwiseIsSaidOutLoud(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"a start alone":      {"--starts-on", "2026-10-01"},
		"a start on due day": {"--starts-on", "2026-10-01", "--due", "2026-10-01"},
	} {
		env, r := e.runJSON("", append([]string{"todos", "create", "--list", f.TodoList.ID, "--title", name}, args...)...)
		expectCode(t, r, ExitOK)
		todo := decode(t, env.Data)
		if todo["startsOn"] != nil || todo["dueOn"] != "2026-10-01" || !strings.Contains(env.Notice, "not kept as a start") {
			t.Fatalf("%s: startsOn %v, dueOn %v, notice %q", name, todo["startsOn"], todo["dueOn"], env.Notice)
		}
	}

	env, r := e.runJSON("", "todos", "create", "--list", f.TodoList.ID, "--title", "Both", "--starts-on", "2026-10-01", "--due", "2026-10-03")
	expectCode(t, r, ExitOK)
	both := decode(t, env.Data)
	if env.Notice != "" {
		t.Fatalf("a start before the due day drew the notice %q", env.Notice)
	}

	env, r = e.runJSON("", "todos", "edit", both["id"].(string), "--due", "")
	expectCode(t, r, ExitOK)
	if e.lastWriteBody()["dueOn"] != nil {
		t.Fatalf("taking the due day off sent %v", e.lastWriteBody())
	}
	if todo := decode(t, env.Data); todo["dueOn"] != "2026-10-01" || !strings.Contains(env.Notice, `--starts-on ""`) {
		t.Fatalf("dueOn %v, notice %q", todo["dueOn"], env.Notice)
	}
}

// A due day moved onto the start the to-do carries takes that start away,
// although no start was sent at all.
func TestAStartLostToANewDueDayIsSaidOutLoud(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	start, due := "2026-10-01", "2026-10-03"
	f.LooseTodo.StartsOn, f.LooseTodo.DueOn = &start, &due

	env, r := e.runJSON("", "todos", "edit", f.LooseTodo.ID, "--due", "2026-10-01")
	expectCode(t, r, ExitOK)
	if todo := decode(t, env.Data); todo["startsOn"] != nil || !strings.Contains(env.Notice, "the start 2026-10-01 was not kept") {
		t.Fatalf("startsOn %v, notice %q", todo["startsOn"], env.Notice)
	}

	f.LooseTodo.StartsOn = &start
	env, r = e.runJSON("", "todos", "edit", f.LooseTodo.ID, "--due", "2026-10-05")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Fatalf("a start that stayed drew the notice %q", env.Notice)
	}
}

// A start given together with an empty due day is meant: the one notice says
// what became of it, and nothing tells the caller to take off what they sent.
func TestAStartSentWithAnEmptyDueDayDrawsOneNotice(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, args := range [][]string{
		{"todos", "create", "--list", f.TodoList.ID, "--title", "Soon", "--starts-on", "2026-10-01", "--due", ""},
		{"todos", "edit", f.LooseTodo.ID, "--starts-on", "2026-10-01", "--due", ""},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitOK)
		if !strings.Contains(env.Notice, "not kept as a start") || strings.Contains(env.Notice, `--starts-on ""`) {
			t.Fatalf("%v: notice %q", args[:2], env.Notice)
		}
	}
}

func TestAnEditOfATodoSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	_, r := e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--title", "Fetch the big speakers")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "Fetch the big speakers" {
		t.Fatalf("sent %v", body)
	}
	if f.GroupedTodo.DueOn == nil || len(f.GroupedTodo.AssigneeIDs) != 1 {
		t.Fatal("what was not given changed with the title")
	}

	writes := len(e.server.WritingRequests())
	for name, args := range map[string][]string{
		"nothing":                   {"todos", "edit", f.GroupedTodo.ID},
		"people and nobody":         {"todos", "edit", f.GroupedTodo.ID, "--assignee", "me", "--no-assignees"},
		"told and nobody told":      {"todos", "edit", f.GroupedTodo.ID, "--notify-when-done", "me", "--no-notify-when-done"},
		"notes in an unknown shape": {"todos", "edit", f.GroupedTodo.ID, "--notes", "x", "--notes-format", "rtf"},
	} {
		_, r = e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if len(e.server.WritingRequests()) != writes {
			t.Fatalf("%s: a write was sent", name)
		}
	}

	env, r := e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--title", "")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// People go as the whole set the to-do is to carry, so taking everybody off
// has to go out as an empty list: null names no change at all.
func TestTakingEverybodyOffSendsAnEmptyList(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	_, r := e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--no-assignees")
	expectCode(t, r, ExitOK)
	raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body
	if string(raw) != `{"assigneeIds":[]}` {
		t.Fatalf("sent %s", raw)
	}
	if len(f.GroupedTodo.AssigneeIDs) != 0 {
		t.Fatalf("still assigned: %v", f.GroupedTodo.AssigneeIDs)
	}

	_, r = e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--no-notify-when-done")
	expectCode(t, r, ExitOK)
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"notifiedIds":[]}` {
		t.Fatalf("sent %s", raw)
	}
}

func TestTheAssigneesGivenAreTheWholeSet(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--assignee", "anke@example.com")
	expectCode(t, r, ExitOK)
	if people := stringsIn(e.lastWriteBody()["assigneeIds"]); !slices.Equal(people, []string{f.Anke.ID}) {
		t.Fatalf("sent %v", people)
	}
	if ids := idsOf(t, decode(t, env.Data), "assignees"); !slices.Equal(ids, []string{f.Anke.ID}) {
		t.Fatalf("Bruno, not named again, is still on the to-do: %v", ids)
	}

	_, r = e.runJSON("", "todos", "edit", f.GroupedTodo.ID, "--assignee", "Nobody Of That Name")
	expectCode(t, r, ExitUsage)
}

func TestNotesWithAttachmentsAreNotDroppedUnlessAsked(t *testing.T) {
	e := newTestEnv(t)
	todo := e.fixture.LooseTodo
	todo.Notes = `<p>Plan</p><action-text-attachment sgid="abc" content-type="image/png" url="https://minka.example/f/1.png" filename="plan.png"></action-text-attachment>`

	env, r := e.runJSON("", "todos", "edit", todo.ID, "--notes", "new text")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, "notesSource") {
		t.Fatalf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Fatalf("%d writes sent", n)
	}

	env, r = e.runJSON("", "todos", "edit", todo.ID, "--notes", "new text", "--drop-attachments")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "attachments") || todo.Notes != "<p>new text</p>" {
		t.Fatalf("notice %q, notes %q", env.Notice, todo.Notes)
	}
}

func TestNotesInHTMLWithoutMarkupAreSaidOutLoud(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "todos", "edit", e.fixture.LooseTodo.ID, "--notes", "# Heading", "--notes-format", "html")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "characters it consists of") || decode(t, env.Data)["notes"] != "<div># Heading</div>" {
		t.Fatalf("notice %q, notes %v", env.Notice, decode(t, env.Data)["notes"])
	}
}

func TestATodoIsTickedOffAndOpenedAgainTwiceWithoutAMistake(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for range 2 {
		env, r := e.runJSON("", "todos", "complete", f.GroupedTodo.ID)
		expectCode(t, r, ExitOK)
		if todo := decode(t, env.Data); todo["completed"] != true || todo["completedByName"] != "Bruno" {
			t.Fatalf("completed %v by %v", todo["completed"], todo["completedByName"])
		}
	}
	list := decode(t, runOK(t, e, "todolists", "show", f.TodoList.ID).Data)
	if list["openCount"] != float64(1) || list["completedCount"] != float64(2) {
		t.Fatalf("counts %v open, %v done", list["openCount"], list["completedCount"])
	}
	if f.GroupedTodo.Subtasks[0].Completed {
		t.Fatal("ticking off a to-do ticked off its subtasks")
	}

	for range 2 {
		env, r := e.runJSON("", "todos", "reopen", f.GroupedTodo.ID)
		expectCode(t, r, ExitOK)
		if todo := decode(t, env.Data); todo["completed"] != false || todo["completedAt"] != nil {
			t.Fatalf("todo %v", todo)
		}
	}
}

func TestATodoMovesWhereItsPlacementSays(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	loose := f.LooseTodo.ID
	second := decode(t, runOK(t, e, "todos", "create", "--list", f.TodoList.ID, "--title", "Second").Data)["id"].(string)

	for _, step := range []struct {
		args []string
		sent map[string]any
		want []string
	}{
		{[]string{"todos", "move", second, "--top"}, map[string]any{}, []string{second, loose}},
		{[]string{"todos", "move", second, "--after", loose}, map[string]any{"previousTodoId": loose}, []string{loose, second}},
		{[]string{"todos", "move", second, "--before", loose}, map[string]any{"nextTodoId": loose}, []string{second, loose}},
	} {
		runOK(t, e, step.args...)
		if body := e.lastWriteBody(); !mapsEqual(body, step.sent) {
			t.Fatalf("%v sent %v, want %v", step.args, body, step.sent)
		}
		if ids := idsOf(t, decode(t, runOK(t, e, "todolists", "show", f.TodoList.ID).Data), "todos"); !slices.Equal(ids, step.want) {
			t.Fatalf("%v left the list as %v, want %v", step.args, ids, step.want)
		}
	}
}

func TestATodoMovesIntoAGroupOutOfOneAndIntoAnotherList(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "todos", "move", f.LooseTodo.ID, "--group", f.StageGroup.ID, "--after", f.GroupedTodo.ID)
	if body := e.lastWriteBody(); body["groupId"] != f.StageGroup.ID || body["previousTodoId"] != f.GroupedTodo.ID {
		t.Fatalf("sent %v", body)
	}
	if decode(t, env.Data)["groupId"] != f.StageGroup.ID {
		t.Fatal("the to-do is not in the group")
	}

	// Within its list a to-do keeps its group unless told otherwise.
	runOK(t, e, "todos", "move", f.LooseTodo.ID, "--top")
	if f.LooseTodo.GroupID != f.StageGroup.ID {
		t.Fatal("a move without --group took the to-do out of its group")
	}

	runOK(t, e, "todos", "move", f.LooseTodo.ID, "--no-group", "--top")
	if raw := e.server.WritingRequests()[len(e.server.WritingRequests())-1].Body; string(raw) != `{"groupId":null}` {
		t.Fatalf("--no-group sent %s", raw)
	}
	if f.LooseTodo.GroupID != "" {
		t.Fatal("--no-group left the to-do in its group")
	}

	env = runOK(t, e, "todos", "move", f.GroupedTodo.ID, "--list", f.EmptyTodoList.ID, "--top")
	if moved := decode(t, env.Data); moved["listId"] != f.EmptyTodoList.ID || moved["groupId"] != nil {
		t.Fatalf("a to-do taken into another list stands in %v, group %v", moved["listId"], moved["groupId"])
	}
}

func TestATodoMoveNamesExactlyOnePlace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	todo := f.LooseTodo.ID

	for name, args := range map[string][]string{
		"no placement":        {"todos", "move", todo},
		"two placements":      {"todos", "move", todo, "--top", "--before", f.GroupedTodo.ID},
		"a group and none":    {"todos", "move", todo, "--top", "--group", f.StageGroup.ID, "--no-group"},
		"a neighbour no ID":   {"todos", "move", todo, "--after", "Count the tents"},
		"a list that no ID":   {"todos", "move", todo, "--top", "--list", "Tearing down"},
		"a group that no ID":  {"todos", "move", todo, "--top", "--group", "Stage"},
		"a to-do that no ID":  {"todos", "move", "Count the tents", "--top"},
		"a neighbour and top": {"todos", "move", todo, "--top", "--after", f.GroupedTodo.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if n := len(e.server.WritingRequests()); n != 0 {
			t.Fatalf("%s: %d writes sent", name, n)
		}
	}
}

// Where a to-do may stand is Minka's rule: what the CLI names goes out, and
// Minka's refusal reaches the caller with a hint about the place.
func TestATodoIsNotMovedWhereMinkaHasNoPlaceForIt(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"next to itself":               {"todos", "move", f.LooseTodo.ID, "--after", f.LooseTodo.ID},
		"next to one of another group": {"todos", "move", f.LooseTodo.ID, "--after", f.GroupedTodo.ID},
		"into a list of another tool":  {"todos", "move", f.LooseTodo.ID, "--list", f.OtherTodoList.ID, "--top"},
		"into a group that is not":     {"todos", "move", f.LooseTodo.ID, "--group", unknownID, "--top"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoPlaceUnknown || !strings.Contains(env.Hint, "--group") || !strings.Contains(env.Hint, "on the web") {
			t.Fatalf("%s: apiCode %v, hint %q", name, env.Meta["apiCode"], env.Hint)
		}
	}
	if ids := idsOf(t, decode(t, runOK(t, e, "todolists", "show", f.TodoList.ID).Data), "todos"); !slices.Equal(ids, []string{f.LooseTodo.ID}) {
		t.Fatalf("a refused move moved something: %v", ids)
	}

	env, r := e.runJSON("", "todos", "move", f.LooseTodo.ID, "--list", f.ArchivedList.ID, "--top")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.Contains(env.Hint, "--list") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestATodoGoesIntoTheTrashAndOutOfTheCounts(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "delete", f.LooseTodo.ID)
	expectCode(t, r, ExitOK)
	if data := decode(t, env.Data); data["id"] != f.LooseTodo.ID || data["deleted"] != true {
		t.Fatalf("data %v", data)
	}
	if list := decode(t, runOK(t, e, "todolists", "show", f.TodoList.ID).Data); list["openCount"] != float64(1) {
		t.Fatalf("a trashed to-do is still counted: %v open", list["openCount"])
	}
	env, r = e.runJSON("", "todos", "delete", f.LooseTodo.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestTheDoneTodosOfAListAreReadAPageAtATime(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	runOK(t, e, "todos", "complete", f.LooseTodo.ID)
	runOK(t, e, "todos", "complete", f.GroupedTodo.ID)

	env, r := e.runJSON("", "todos", "completed", f.TodoList.ID, "--limit", "2")
	expectCode(t, r, ExitOK)
	var first struct {
		Todos []map[string]any `json:"todos"`
		Next  *string          `json:"next"`
	}
	if err := json.Unmarshal(env.Data, &first); err != nil {
		t.Fatal(err)
	}
	if ids := idsIn(first.Todos); !slices.Equal(ids, []string{f.GroupedTodo.ID, f.LooseTodo.ID}) || first.Next == nil {
		t.Fatalf("the first page holds %v, next %v", ids, first.Next)
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Query != "limit=2" {
		t.Fatalf("asked with %q", last.Query)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "next"), "--before "+*first.Next) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	// Opening the last to-do of a page again moves nothing on the next one.
	runOK(t, e, "todos", "reopen", f.LooseTodo.ID)
	env, r = e.runJSON("", "todos", "completed", f.TodoList.ID, "--limit", "2", "--before", *first.Next)
	expectCode(t, r, ExitOK)
	second := decode(t, env.Data)
	if ids := idsOf(t, second, "todos"); !slices.Equal(ids, []string{f.DoneTodo.ID}) || second["next"] != nil {
		t.Fatalf("the second page holds %v, next %v", ids, second["next"])
	}

	env, r = e.runJSON("", "todos", "completed", f.TodoList.ID, "--before", f.DoneTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeCompletedTodosBeforeUnknown || !strings.Contains(env.Hint, "exactly as it came") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	// The size of a page is Minka's to refuse: a larger one is cut down, which
	// next then says.
	env, r = e.runJSON("", "todos", "completed", f.TodoList.ID, "--limit", "0")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodosLimitNotANumber {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestTheDoneTodosOfAnArchivedListAreRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	_, r := e.runJSON("", "todos", "completed", f.ArchivedList.ID)
	expectCode(t, r, ExitOK)
	env, r := e.runJSON("", "todos", "completed", f.OtherTodoList.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoListNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestMyTodosAreThoseLeftToDoInTheOrderOfMyPage(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	later := "2026-12-01"
	f.LooseTodo.AssigneeIDs, f.LooseTodo.DueOn = []string{f.Bruno.ID}, &later
	onlyASubtask := decode(t, runOK(t, e, "todos", "create", "--list", f.TodoList.ID, "--title", "A subtask of mine").Data)["id"].(string)
	runOK(t, e, "todos", "subtasks", "create", onlyASubtask, "Mine", "--assignee", "me")
	f.DoneTodo.AssigneeIDs = []string{f.Bruno.ID}
	f.ArchivedListTodo.AssigneeIDs = []string{f.Bruno.ID}

	env, r := e.runJSON("", "todos", "mine")
	expectCode(t, r, ExitOK)
	todos := listOfObjects(t, env.Data)
	if ids := idsIn(todos); !slices.Equal(ids, []string{f.GroupedTodo.ID, f.LooseTodo.ID, onlyASubtask}) {
		t.Fatalf("my to-dos are %v: a done one or one of an archived list is among them, or the order is lost", ids)
	}
	if todos[2]["isAssigned"] != false || todos[0]["isAssigned"] != true {
		t.Fatalf("isAssigned %v and %v", todos[0]["isAssigned"], todos[2]["isAssigned"])
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Query != "limit=50" {
		t.Fatalf("asked with %q: the page size goes out every time", last.Query)
	}

	f.Project.Archived = true
	env, r = e.runJSON("", "todos", "mine")
	expectCode(t, r, ExitOK)
	if n := len(listOfObjects(t, env.Data)); n != 0 {
		t.Fatalf("%d to-dos of an archived project", n)
	}
}

func TestNoTodoComesFromAToolInTheTrash(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Todos.Trashed = true

	env, r := e.runJSON("", "todos", "mine")
	expectCode(t, r, ExitOK)
	if n := len(listOfObjects(t, env.Data)); n != 0 {
		t.Fatalf("%d to-dos of a tool in the trash", n)
	}
	_, r = e.runJSON("", "todos", "show", f.GroupedTodo.ID)
	expectCode(t, r, ExitNotFound)
}

// Minka marks the end of my to-dos by nothing but a short page, and cuts a
// page above its largest without a word, so the CLI serves no such page.
func TestAPageOfMyTodosIsBoundedAndAFullOneSaysSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.LooseTodo.AssigneeIDs = []string{f.Bruno.ID}

	for _, limit := range []string{"0", "101"} {
		env, r := e.runJSON("", "todos", "mine", "--limit", limit)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "1 to 100") {
			t.Fatalf("error %q", env.Error)
		}
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Fatalf("%d requests for a page out of bounds", n)
	}

	env, r := e.runJSON("", "todos", "mine", "--limit", "1")
	expectCode(t, r, ExitOK)
	first := idsIn(listOfObjects(t, env.Data))
	if !strings.Contains(env.Notice, "--after "+first[0]) || !strings.Contains(crumbFor(t, env.Breadcrumbs, "next"), "--after "+first[0]) {
		t.Fatalf("notice %q", env.Notice)
	}
	env, r = e.runJSON("", "todos", "mine", "--limit", "1", "--after", first[0])
	expectCode(t, r, ExitOK)
	if ids := idsIn(listOfObjects(t, env.Data)); len(ids) != 1 || ids[0] == first[0] {
		t.Fatalf("the second page holds %v", ids)
	}
	env, r = e.runJSON("", "todos", "mine", "--limit", "3")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Fatalf("a page shorter than asked for says %q", env.Notice)
	}

	_, r = e.runJSON("", "todos", "mine", "--after", "last")
	expectCode(t, r, ExitUsage)
	env, r = e.runJSON("", "todos", "mine", "--after", f.OtherTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodosAfterUnknown || !strings.Contains(env.Hint, "minka todos mine") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestSomebodyOutsideTheProjectIsNotNamedOnATodo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"create":         {"todos", "create", "--list", f.TodoList.ID, "--title", "Theirs", "--assignee", f.Stranger.ID},
		"create, told":   {"todos", "create", "--list", f.TodoList.ID, "--title", "Theirs", "--notify-when-done", f.Stranger.ID},
		"edit":           {"todos", "edit", f.LooseTodo.ID, "--assignee", f.Stranger.ID},
		"edit, told":     {"todos", "edit", f.LooseTodo.ID, "--notify-when-done", f.Stranger.ID},
		"subtask create": {"todos", "subtasks", "create", f.LooseTodo.ID, "Theirs", "--assignee", f.Stranger.ID},
		"subtask edit":   {"todos", "subtasks", "edit", f.GroupedTodo.ID, "1", "--assignee", f.Stranger.ID},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePersonNotAProjectMember || env.Hint != todoPeopleHint {
			t.Fatalf("%s: apiCode %v, hint %q", name, env.Meta["apiCode"], env.Hint)
		}
	}
	if len(f.TodoList.Todos) != 3 || len(f.LooseTodo.AssigneeIDs) != 0 || len(f.LooseTodo.Subtasks) != 0 || f.GroupedTodo.Subtasks[0].AssigneeID != f.Anke.ID {
		t.Fatal("a refused write wrote something")
	}
}

func TestAGroupThatIsNotThereTakesNoNewTodo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "create", "--list", f.TodoList.ID, "--group", f.ArchivedGroup.ID, "--title", "Lost")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoPlaceUnknown || !strings.Contains(env.Hint, "todolists show "+f.TodoList.ID) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// The list or the to-do is looked up before what is wrong with what the
// command says is refused: aimed at nothing, each of these is not found, and
// aimed at a list or a to-do that is there, each is refused for what it says.
func TestWhatIsWrongWithTheRequestIsSaidAfterWhatItIsFor(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, c := range map[string]struct {
		args func(list, todo string) []string
		code string
	}{
		"create without a title": {func(list, _ string) []string { return []string{"todos", "create", "--list", list, "--title", " "} }, "todoTitleMissing"},
		"create with no day": {func(list, _ string) []string {
			return []string{"todos", "create", "--list", list, "--title", "A", "--due", "tomorrow"}
		}, "todoDatesNotADate"},
		"create with the days reversed": {func(list, _ string) []string {
			return []string{"todos", "create", "--list", list, "--title", "A", "--starts-on", "2026-10-05", "--due", "2026-10-01"}
		}, "todoDatesOutOfOrder"},
		"edit without a title":   {func(_, todo string) []string { return []string{"todos", "edit", todo, "--title", ""} }, "todoTitleMissing"},
		"edit with no day":       {func(_, todo string) []string { return []string{"todos", "edit", todo, "--due", "tomorrow"} }, "todoDatesNotADate"},
		"subtask without a text": {func(_, todo string) []string { return []string{"todos", "subtasks", "create", todo, " "} }, "subtaskMissing"},
		"subtask with no day": {func(_, todo string) []string {
			return []string{"todos", "subtasks", "create", todo, "Cables", "--due", "31.10."}
		}, "subtaskDueNotADate"},
	} {
		_, r := e.runJSON("", c.args(unknownID, unknownID)...)
		if r.code != ExitNotFound {
			t.Fatalf("%s aimed at nothing: exit %d, want %d", name, r.code, ExitNotFound)
		}
		env, r := e.runJSON("", c.args(f.TodoList.ID, f.LooseTodo.ID)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: apiCode %v, want %s", name, env.Meta["apiCode"], c.code)
		}
	}

	// A day of a to-do that is there is refused as a day.
	env, r := e.runJSON("", "todos", "edit", e.fixture.LooseTodo.ID, "--starts-on", "2026-10-05", "--due", "2026-10-01")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoDatesOutOfOrder" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// Where the CLI reads before it writes — the list or the to-do, for the project
// a person named by name, address or "me" is looked for in, or the start a due
// day is held against — something that is not there answers before what is
// wrong with the request.
func TestAReadBeforeTheWriteFindsNothingFirst(t *testing.T) {
	e := newTestEnv(t)

	for name, args := range map[string][]string{
		"create for me":     {"todos", "create", "--list", unknownID, "--title", " ", "--assignee", "me"},
		"create, me told":   {"todos", "create", "--list", unknownID, "--title", " ", "--notify-when-done", "anke@example.com"},
		"a subtask for me":  {"todos", "subtasks", "create", unknownID, " ", "--assignee", "me"},
		"edit for me":       {"todos", "edit", unknownID, "--title", "", "--assignee", "me"},
		"edit of the notes": {"todos", "edit", unknownID, "--title", "", "--notes", "More"},
		"edit of a due day": {"todos", "edit", unknownID, "--title", "", "--due", "2026-10-01"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if n := len(e.server.WritingRequests()); n != 0 {
			t.Fatalf("%s: %d writes", name, n)
		}
	}

	// Named by ID, nobody is looked for, and Minka's order holds: the list
	// first, then the title.
	_, r := e.runJSON("", "todos", "create", "--list", unknownID, "--title", " ", "--assignee", e.fixture.Anke.ID)
	expectCode(t, r, ExitNotFound)
	if n := len(e.server.WritingRequests()); n != 1 {
		t.Fatalf("%d writes, want the one sent", n)
	}
	env, r := e.runJSON("", "todos", "create", "--list", e.fixture.TodoList.ID, "--title", " ", "--assignee", e.fixture.Anke.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// todoWrites are the writes on a to-do and its subtasks, aimed at todo, whose
// first subtask each subtask command names by position.
func todoWrites(todo string) map[string][]string {
	return map[string][]string{
		"todos edit":              {"todos", "edit", todo, "--title", "Renamed"},
		"todos complete":          {"todos", "complete", todo},
		"todos reopen":            {"todos", "reopen", todo},
		"todos move":              {"todos", "move", todo, "--top"},
		"todos delete":            {"todos", "delete", todo},
		"todos subtasks create":   {"todos", "subtasks", "create", todo, "More"},
		"todos subtasks edit":     {"todos", "subtasks", "edit", todo, "1", "--title", "Renamed"},
		"todos subtasks complete": {"todos", "subtasks", "complete", todo, "1"},
		"todos subtasks reopen":   {"todos", "subtasks", "reopen", todo, "1"},
		"todos subtasks move":     {"todos", "subtasks", "move", todo, "1", "--top"},
		"todos subtasks delete":   {"todos", "subtasks", "delete", todo, "1"},
	}
}

func TestEveryWriteOnATodoOfAnArchivedListIsRefusedAndSaysWhy(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	writes := todoWrites(f.ArchivedListTodo.ID)
	writes["todos create"] = []string{"todos", "create", "--list", f.ArchivedList.ID, "--title", "Late"}
	writes["a subtask that is not there"] = []string{"todos", "subtasks", "delete", f.ArchivedListTodo.ID, unknownID}

	for name, args := range writes {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.Contains(env.Hint, "todolists unarchive") {
			t.Fatalf("%s: apiCode %v, hint %q", name, env.Meta["apiCode"], env.Hint)
		}
	}
	if len(f.ArchivedList.Todos) != 1 || len(f.ArchivedListTodo.Subtasks) != 1 || f.ArchivedListTodo.CompletedAt != nil || f.ArchivedListTodo.Trashed {
		t.Fatal("a refused write wrote something")
	}
}

// Minka asks why nothing was written in one order: the project, then the
// archive of the list, then the place or the people named.
func TestTheArchivedProjectAnswersBeforeTheArchivedListAndThePlace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.Archived = true
	writes := todoWrites(f.ArchivedListTodo.ID)
	for name, args := range map[string][]string{
		"todos create":                     {"todos", "create", "--list", f.ArchivedList.ID, "--title", "Late"},
		"todos create in no group":         {"todos", "create", "--list", f.TodoList.ID, "--group", unknownID, "--title", "Late"},
		"todos create for a stranger":      {"todos", "create", "--list", f.TodoList.ID, "--title", "Late", "--assignee", f.Stranger.ID},
		"todos edit for a stranger":        {"todos", "edit", f.LooseTodo.ID, "--assignee", f.Stranger.ID},
		"todos move next to itself":        {"todos", "move", f.LooseTodo.ID, "--after", f.LooseTodo.ID},
		"todos move into an archived list": {"todos", "move", f.LooseTodo.ID, "--list", f.ArchivedList.ID, "--top"},
		"a subtask for a stranger":         {"todos", "subtasks", "create", f.LooseTodo.ID, "More", "--assignee", f.Stranger.ID},
		"a subtask next to itself":         {"todos", "subtasks", "move", f.GroupedTodo.ID, "1", "--after", "1"},
		"a subtask that is not there":      {"todos", "subtasks", "delete", f.GroupedTodo.ID, unknownID},
		"a done to-do ticked off again":    {"todos", "complete", f.DoneTodo.ID},
		"an open subtask opened again":     {"todos", "subtasks", "reopen", f.GroupedTodo.ID, "1"},
	} {
		writes[name] = args
	}

	for name, args := range writes {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeProjectArchived {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
}

func TestTheArchivedListAnswersBeforeThePlaceAndThePeople(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	todo := f.ArchivedListTodo.ID

	for name, args := range map[string][]string{
		"create in no group":       {"todos", "create", "--list", f.ArchivedList.ID, "--group", unknownID, "--title", "Late"},
		"create for a stranger":    {"todos", "create", "--list", f.ArchivedList.ID, "--title", "Late", "--assignee", f.Stranger.ID},
		"edit for a stranger":      {"todos", "edit", todo, "--assignee", f.Stranger.ID},
		"move next to itself":      {"todos", "move", todo, "--after", todo},
		"move into another list":   {"todos", "move", todo, "--list", f.OtherTodoList.ID, "--top"},
		"a subtask for a stranger": {"todos", "subtasks", "create", todo, "More", "--assignee", f.Stranger.ID},
		"a subtask edit for one":   {"todos", "subtasks", "edit", todo, "1", "--assignee", f.Stranger.ID},
		"a subtask next to itself": {"todos", "subtasks", "move", todo, "1", "--after", "1"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoListArchived {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
}

// A to-do that is not there is not there in an archived project either.
func TestAnUnknownTodoIsNotFoundBeforeTheArchivedProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.Archived = true
	writes := todoWrites(unknownID)
	writes["todos create"] = []string{"todos", "create", "--list", unknownID, "--title", "Late"}

	for name, args := range writes {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if env.Meta["apiCode"] == apiCodeProjectArchived {
			t.Fatalf("%s: the archived project answered first", name)
		}
	}
}

func stringsIn(value any) []string {
	list, _ := value.([]any)
	texts := make([]string, len(list))
	for i, one := range list {
		texts[i], _ = one.(string)
	}
	return texts
}
