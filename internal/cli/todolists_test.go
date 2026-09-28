package cli

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestTheTodosToolsInUseOfAProjectAreListed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "tools", f.Project.ID)
	expectCode(t, r, ExitOK)
	var tools []struct{ ID, Title, AppURL string }
	if err := json.Unmarshal(env.Data, &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].ID != f.Todos.ID || tools[0].AppURL == "" {
		t.Fatalf("tools %+v: a tool no longer in use is among them, or the one in use is missing", tools)
	}

	// A tool the project no longer names is still read at its own address.
	_, r = e.runJSON("", "todolists", "list", f.TodosOutOfUse.ID)
	expectCode(t, r, ExitOK)

	env, r = e.runJSON("", "todolists", "tools", f.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
	env, r = e.runJSON("", "todolists", "list", f.OtherTodos.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "minka todolists tools <projectId>") {
		t.Fatalf("hint %q does not say where the tools are listed", env.Hint)
	}
}

func TestTheListsOfAToolAndItsArchiveAreListedApart(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "list", f.Todos.ID)
	expectCode(t, r, ExitOK)
	lists := listOfObjects(t, env.Data)
	if ids := idsIn(lists); !slices.Equal(ids, []string{f.TodoList.ID, f.EmptyTodoList.ID}) {
		t.Fatalf("the lists in use are %v: the archived one is among them, or the order is lost", ids)
	}
	first := lists[0]
	if first["openCount"] != float64(2) || first["completedCount"] != float64(1) || first["archived"] != false {
		t.Fatalf("the first list counts %v open and %v done, archived %v", first["openCount"], first["completedCount"], first["archived"])
	}
	if first["description"] != "<p>Everything for <strong>Saturday</strong>.</p>" {
		t.Fatalf("description %v, want the whole stored HTML", first["description"])
	}

	env, r = e.runJSON("", "todolists", "list", f.Todos.ID, "--archived")
	expectCode(t, r, ExitOK)
	archived := listOfObjects(t, env.Data)
	if ids := idsIn(archived); !slices.Equal(ids, []string{f.ArchivedList.ID}) {
		t.Fatalf("the archive holds %v", ids)
	}
	if archived[0]["archived"] != true || !strings.Contains(env.Summary, "archive") {
		t.Fatalf("a list of the archive says archived %v, summary %q", archived[0]["archived"], env.Summary)
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; !strings.HasSuffix(last.Path, "/todos-tools/"+f.Todos.ID+"/archive") {
		t.Fatalf("--archived asked %s", last.Path)
	}
}

func TestTheArchiveListsTheListArchivedLastFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	expectCode(t, e.run("", "todolists", "archive", f.TodoList.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "todolists", "archive", f.EmptyTodoList.ID, "--json"), ExitOK)

	env, r := e.runJSON("", "todolists", "list", f.Todos.ID, "--archived")
	expectCode(t, r, ExitOK)
	if ids := idsIn(listOfObjects(t, env.Data)); !slices.Equal(ids, []string{f.EmptyTodoList.ID, f.TodoList.ID, f.ArchivedList.ID}) {
		t.Fatalf("the archive reads %v", ids)
	}

	// A list brought back takes the place it had on the page.
	expectCode(t, e.run("", "todolists", "unarchive", f.TodoList.ID, "--json"), ExitOK)
	expectCode(t, e.run("", "todolists", "unarchive", f.EmptyTodoList.ID, "--json"), ExitOK)
	env, r = e.runJSON("", "todolists", "list", f.Todos.ID)
	expectCode(t, r, ExitOK)
	if ids := idsIn(listOfObjects(t, env.Data)); !slices.Equal(ids, []string{f.TodoList.ID, f.EmptyTodoList.ID}) {
		t.Fatalf("the lists brought back read %v", ids)
	}
}

func TestAListIsReadWithItsGroupsAndItsOpenTodos(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "show", f.TodoList.ID)
	expectCode(t, r, ExitOK)
	list := decode(t, env.Data)
	if ids := idsOf(t, list, "todos"); !slices.Equal(ids, []string{f.LooseTodo.ID}) {
		t.Fatalf("the to-dos standing in the list are %v: a grouped or a done one is among them", ids)
	}
	if ids := idsOf(t, list, "groups"); !slices.Equal(ids, []string{f.StageGroup.ID, f.KitchenGroup.ID}) {
		t.Fatalf("groups %v", ids)
	}
	stage := decode(t, rawAt(t, list, "groups", 0))
	if ids := idsOf(t, stage, "todos"); !slices.Equal(ids, []string{f.GroupedTodo.ID}) || stage["listId"] != f.TodoList.ID {
		t.Fatalf("the stage group holds %v and names list %v", ids, stage["listId"])
	}
	if list["openCount"] != float64(2) || list["completedCount"] != float64(1) || list["commentCount"] != float64(1) {
		t.Fatalf("counts %v open, %v done, %v comments", list["openCount"], list["completedCount"], list["commentCount"])
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "group"), "todogroups create --list "+f.TodoList.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	text := e.run("", "todolists", "show", f.TodoList.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{
		"Everything for **Saturday**.",
		"To-dos: 2 open, 1 done",
		f.LooseTodo.ID + "  Count the tents",
		"## Stage (group " + f.StageGroup.ID + ", 1 open)",
		"## Kitchen (group " + f.KitchenGroup.ID + ", 0 open)",
		"I bring the van",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}
	if strings.Contains(text.stdout, f.DoneTodo.Title) {
		t.Fatalf("a done to-do is listed:\n%s", text.stdout)
	}
}

func TestAnArchivedListIsReadAndSaysSo(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "todolists", "show", e.fixture.ArchivedList.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["archived"] != true || !strings.Contains(env.Summary, "archive") {
		t.Fatalf("summary %q", env.Summary)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "unarchive"), "todolists unarchive "+e.fixture.ArchivedList.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

func TestAListNobodyMayReadIsAtNoAddress(t *testing.T) {
	e := newTestEnv(t)

	for _, id := range []string{e.fixture.OtherTodoList.ID, "01990000-0000-4000-8000-000000000000"} {
		env, r := e.runJSON("", "todolists", "show", id)
		expectCode(t, r, ExitNotFound)
		if env.Hint != todoListNotFoundHint {
			t.Fatalf("hint %q", env.Hint)
		}
	}
}

func TestAListIsStartedOnTopWithItsDescriptionAsHTML(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("# Plan\n\nBring **chairs**.", "todolists", "create", "--tool", f.Todos.ID, "--title", " Clearing up ", "--description", "-")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["title"] != " Clearing up " || body["description"] != "<h1>Plan</h1>\n<p>Bring <strong>chairs</strong>.</p>" {
		t.Fatalf("sent %v", body)
	}
	created := decode(t, env.Data)
	if created["title"] != "Clearing up" {
		t.Fatalf("title %v", created["title"])
	}

	env, r = e.runJSON("", "todolists", "list", f.Todos.ID)
	expectCode(t, r, ExitOK)
	if ids := idsIn(listOfObjects(t, env.Data)); len(ids) != 3 || ids[0] != created["id"] {
		t.Fatalf("a new list is not on top: %v", ids)
	}

	// Without --description there is none to send, not an empty one.
	_, r = e.runJSON("", "todolists", "create", "--tool", f.Todos.ID, "--title", "Only a title")
	expectCode(t, r, ExitOK)
	if _, sent := e.lastWriteBody()["description"]; sent {
		t.Fatal("a description was sent although none was given")
	}
}

func TestADescriptionInHTMLWithoutMarkupIsSaidOutLoud(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "todolists", "create", "--tool", e.fixture.Todos.ID, "--title", "Plain", "--description", "# Heading", "--description-format", "html")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "characters it consists of") {
		t.Fatalf("notice %q", env.Notice)
	}
	if description := decode(t, env.Data)["description"]; description != "<div># Heading</div>" {
		t.Fatalf("the server kept %v", description)
	}
}

func TestStartingAListIsRefusedWhereTheServerRefusesIt(t *testing.T) {
	e := newTestEnv(t)

	// The title is held before the tool is looked up.
	env, r := e.runJSON("", "todolists", "create", "--tool", "01990000-0000-4000-8000-000000000000", "--title", " ")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoListTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	env, r = e.runJSON("", "todolists", "create", "--tool", "01990000-0000-4000-8000-000000000000", "--title", "Somewhere")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "todolists tools") {
		t.Fatalf("hint %q", env.Hint)
	}

	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "todolists", "create", "--title", "Nowhere")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a list was sent without a tool")
	}
}

func TestAnEditSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	_, r := e.runJSON("", "todolists", "edit", f.TodoList.ID, "--title", "Setting up on Saturday")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "Setting up on Saturday" {
		t.Fatalf("sent %v", body)
	}
	if f.TodoList.Description != "<p>Everything for <strong>Saturday</strong>.</p>" {
		t.Fatalf("the description changed with the title: %q", f.TodoList.Description)
	}

	_, r = e.runJSON("", "todolists", "edit", f.TodoList.ID, "--description", "")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["description"] != "" {
		t.Fatalf("sent %v", body)
	}
	if f.TodoList.Description != "" {
		t.Fatalf("an empty description did not empty it: %q", f.TodoList.Description)
	}

	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "todolists", "edit", f.TodoList.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("an edit without a change was sent")
	}

	env, r := e.runJSON("", "todolists", "edit", f.TodoList.ID, "--title", "")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoListTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestADescriptionWithAttachmentsIsNotDroppedUnlessAsked(t *testing.T) {
	e := newTestEnv(t)
	list := e.fixture.TodoList
	list.Description = `<p>Plan</p><action-text-attachment sgid="abc" content-type="image/png" url="https://minka.example/f/1.png" filename="plan.png"></action-text-attachment>`

	env, r := e.runJSON("", "todolists", "edit", list.ID, "--description", "new text")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, "todolists show --json") {
		t.Fatalf("hint %q", env.Hint)
	}
	if n := len(e.server.WritingRequests()); n != 0 {
		t.Fatalf("%d writes sent", n)
	}

	env, r = e.runJSON("", "todolists", "edit", list.ID, "--description", "new text", "--drop-attachments")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "attachments") || list.Description != "<p>new text</p>" {
		t.Fatalf("notice %q, description %q", env.Notice, list.Description)
	}
}

func TestAListGoesIntoTheArchiveAndComesBack(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	list := f.TodoList.ID

	env, r := e.runJSON("", "todolists", "archive", list)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["archived"] != true {
		t.Fatal("the answer does not say that the list is archived")
	}

	for name, args := range map[string][]string{
		"archive again":  {"todolists", "archive", list},
		"edit":           {"todolists", "edit", list, "--title", "Renamed"},
		"a group for it": {"todogroups", "create", "--list", list, "--title", "Late"},
	} {
		env, r = e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.Contains(env.Hint, "minka todolists unarchive "+list) {
			t.Fatalf("%s: apiCode %v, hint %q", name, env.Meta["apiCode"], env.Hint)
		}
	}

	env, r = e.runJSON("", "todolists", "unarchive", list)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["archived"] != false {
		t.Fatal("the answer does not say that the list is back")
	}

	env, r = e.runJSON("", "todolists", "unarchive", list)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoListNotArchived || !strings.Contains(env.Hint, "minka todolists archive "+list) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestAnArchivedProjectAnswersBeforeTheArchiveOfTheList(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.Archived = true

	for name, args := range map[string][]string{
		"archiving an archived list":   {"todolists", "archive", f.ArchivedList.ID},
		"bringing back one in use":     {"todolists", "unarchive", f.TodoList.ID},
		"changing an archived list":    {"todolists", "edit", f.ArchivedList.ID, "--title", "Renamed"},
		"a neighbour of another list":  {"todogroups", "move", f.StageGroup.ID, "--after", f.ArchivedGroup.ID},
		"a group for an archived list": {"todogroups", "create", "--list", f.ArchivedList.ID, "--title", "Late"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeProjectArchived {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
}

func TestAGroupOfAnArchivedListIsAtNoAddress(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, args := range map[string][]string{
		"edit":   {"todogroups", "edit", f.ArchivedGroup.ID, "--title", "Renamed"},
		"move":   {"todogroups", "move", f.ArchivedGroup.ID, "--top"},
		"delete": {"todogroups", "delete", f.ArchivedGroup.ID},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "archive") || !strings.Contains(env.Hint, "todolists unarchive") {
			t.Fatalf("%s: hint %q does not name the archive of the list", name, env.Hint)
		}
	}
}

func TestAListGoesIntoTheTrashFromThePageAndFromTheArchive(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, list := range []string{f.TodoList.ID, f.ArchivedList.ID} {
		env, r := e.runJSON("", "todolists", "delete", list)
		expectCode(t, r, ExitOK)
		if data := decode(t, env.Data); data["id"] != list || data["deleted"] != true {
			t.Fatalf("data %v", data)
		}
		writes := e.server.WritingRequests()
		if last := writes[len(writes)-1]; last.Method != http.MethodDelete || !strings.HasSuffix(last.Path, "/todo-lists/"+list) {
			t.Fatalf("sent %s %s", last.Method, last.Path)
		}

		env, r = e.runJSON("", "todolists", "show", list)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "trash") {
			t.Fatalf("hint %q", env.Hint)
		}
		_, r = e.runJSON("", "todolists", "delete", list)
		expectCode(t, r, ExitNotFound)
	}
	if !f.TodoList.Trashed || !f.ArchivedList.Trashed {
		t.Fatal("a list is not in the trash")
	}
}

func TestAGroupIsAddedAtTheEndRenamedAndTrashedWithItsTodos(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todogroups", "create", "--list", f.TodoList.ID, "--title", " Light ")
	expectCode(t, r, ExitOK)
	group := decode(t, env.Data)
	if group["title"] != "Light" || group["listId"] != f.TodoList.ID {
		t.Fatalf("group %v", group)
	}
	if ids := groupsOfTheList(t, e); !slices.Equal(ids, []string{f.StageGroup.ID, f.KitchenGroup.ID, group["id"].(string)}) {
		t.Fatalf("a new group is not at the end: %v", ids)
	}

	env, r = e.runJSON("", "todogroups", "edit", f.StageGroup.ID, "--title", "Stage and light")
	expectCode(t, r, ExitOK)
	if e.lastWriteBody()["title"] != "Stage and light" || f.StageGroup.Title != "Stage and light" {
		t.Fatalf("title %q", f.StageGroup.Title)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "show"), "todolists show "+f.TodoList.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "todogroups", "edit", f.StageGroup.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a rename without a title was sent")
	}
	env, r = e.runJSON("", "todogroups", "edit", f.StageGroup.ID, "--title", "")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoGroupTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	_, r = e.runJSON("", "todogroups", "delete", f.StageGroup.ID)
	expectCode(t, r, ExitOK)
	env, r = e.runJSON("", "todolists", "show", f.TodoList.ID)
	expectCode(t, r, ExitOK)
	if list := decode(t, env.Data); list["openCount"] != float64(1) {
		t.Fatalf("the to-do of a trashed group is still counted: %v open", list["openCount"])
	}
	env, r = e.runJSON("", "todogroups", "delete", f.StageGroup.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoGroupNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestAGroupMovesWhereItsPlacementSays(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	stage, kitchen := f.StageGroup.ID, f.KitchenGroup.ID
	light := decode(t, runOK(t, e, "todogroups", "create", "--list", f.TodoList.ID, "--title", "Light").Data)["id"].(string)

	for _, step := range []struct {
		args []string
		sent map[string]any
		want []string
	}{
		{[]string{"todogroups", "move", light, "--top"}, map[string]any{}, []string{light, stage, kitchen}},
		{[]string{"todogroups", "move", light, "--after", kitchen}, map[string]any{"previousGroupId": kitchen}, []string{stage, kitchen, light}},
		{[]string{"todogroups", "move", light, "--before", kitchen}, map[string]any{"nextGroupId": kitchen}, []string{stage, light, kitchen}},
	} {
		runOK(t, e, step.args...)
		if body := e.lastWriteBody(); !mapsEqual(body, step.sent) {
			t.Fatalf("%v sent %v, want %v", step.args, body, step.sent)
		}
		if ids := groupsOfTheList(t, e); !slices.Equal(ids, step.want) {
			t.Fatalf("%v left the groups as %v, want %v", step.args, ids, step.want)
		}
	}
}

func TestAGroupMoveNamesExactlyOnePlace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	stage, kitchen := f.StageGroup.ID, f.KitchenGroup.ID

	for name, args := range map[string][]string{
		"no placement":      {"todogroups", "move", stage},
		"two placements":    {"todogroups", "move", stage, "--top", "--after", kitchen},
		"a neighbour no ID": {"todogroups", "move", stage, "--after", "Kitchen"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if n := len(e.server.WritingRequests()); n != 0 {
			t.Fatalf("%s: %d writes sent", name, n)
		}
	}
}

// A group named as its own neighbour is Minka's to refuse, so what it says
// reaches the caller: todoPlaceUnknown, and in an archived project that it is
// archived.
func TestAGroupIsNotMovedNextToItself(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, flag := range []string{"--after", "--before"} {
		env, r := e.runJSON("", "todogroups", "move", f.StageGroup.ID, flag, f.StageGroup.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTodoPlaceUnknown || !strings.Contains(env.Hint, "the group itself") {
			t.Fatalf("%s: apiCode %v, hint %q", flag, env.Meta["apiCode"], env.Hint)
		}
	}
	if ids := groupsOfTheList(t, e); !slices.Equal(ids, []string{f.StageGroup.ID, f.KitchenGroup.ID}) {
		t.Fatalf("a refused move moved something: %v", ids)
	}

	f.Project.Archived = true
	env, r := e.runJSON("", "todogroups", "move", f.StageGroup.ID, "--after", f.StageGroup.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeProjectArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// The title is held before the list or the group is looked up, so an empty one
// is refused as empty wherever it is aimed.
func TestATitleIsRefusedBeforeWhatItIsFor(t *testing.T) {
	e := newTestEnv(t)
	unknown := "01990000-0000-4000-8000-000000000000"

	for name, c := range map[string]struct {
		args []string
		code string
	}{
		"a list changed":  {[]string{"todolists", "edit", unknown, "--title", ""}, "todoListTitleMissing"},
		"a group added":   {[]string{"todogroups", "create", "--list", unknown, "--title", ""}, "todoGroupTitleMissing"},
		"a group renamed": {[]string{"todogroups", "edit", unknown, "--title", ""}, "todoGroupTitleMissing"},
	} {
		env, r := e.runJSON("", c.args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: apiCode %v, want %s", name, env.Meta["apiCode"], c.code)
		}
	}
}

// Escaping makes a plain text longer, and the server holds the length of what
// it would store as well as of what was sent.
func TestADescriptionThatGrowsBeyondTheLimitWhenStoredIsRefused(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON(strings.Repeat("&", 30000), "todolists", "create", "--tool", e.fixture.Todos.ID, "--title", "Ampersands", "--description", "-", "--description-format", "html")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "todoListDescriptionTooLong" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// The server escapes a text it keeps as characters without touching quotes and
// keeps each line break behind the <br> it adds, so a text full of quotes
// stays within the limit and is stored.
func TestAPlainDescriptionIsEscapedAsTheServerEscapesIt(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON(strings.Repeat(`"'`, 20000)+"\n<b> & more", "todolists", "create", "--tool", e.fixture.Todos.ID, "--title", "Quotes", "--description", "-", "--description-format", "html")
	expectCode(t, r, ExitOK)
	want := "<div>" + strings.Repeat(`"'`, 20000) + "<br>\n&lt;b&gt; &amp; more</div>"
	if description := decode(t, env.Data)["description"]; description != want {
		t.Fatalf("the server kept a description of %d characters that differs from what Minka keeps", len(description.(string)))
	}
}

func TestAGroupIsNotMovedNextToAGroupOfAnotherList(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	elsewhere := decode(t, runOK(t, e, "todogroups", "create", "--list", f.EmptyTodoList.ID, "--title", "Elsewhere").Data)["id"].(string)

	env, r := e.runJSON("", "todogroups", "move", f.KitchenGroup.ID, "--before", elsewhere)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoPlaceUnknown || !strings.Contains(env.Hint, "todolists show") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if ids := groupsOfTheList(t, e); !slices.Equal(ids, []string{f.StageGroup.ID, f.KitchenGroup.ID}) {
		t.Fatalf("a refused move moved something: %v", ids)
	}
}

func TestAGroupIsAddedOnlyToAListSomebodyMayRead(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "todogroups", "create", "--list", e.fixture.OtherTodoList.ID, "--title", "Mine")
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoListNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func runOK(t *testing.T, e *testEnv, args ...string) envelope {
	t.Helper()
	env, r := e.runJSON("", args...)
	expectCode(t, r, ExitOK)
	return env
}

// groupsOfTheList reads the groups of the fixture's list in their order, as
// the CLI hands them over.
func groupsOfTheList(t *testing.T, e *testEnv) []string {
	t.Helper()
	return idsOf(t, decode(t, runOK(t, e, "todolists", "show", e.fixture.TodoList.ID).Data), "groups")
}

func listOfObjects(t *testing.T, raw json.RawMessage) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("data is not a list of objects: %v: %s", err, raw)
	}
	return list
}

func idsIn(list []map[string]any) []string {
	ids := make([]string, len(list))
	for i, one := range list {
		ids[i], _ = one["id"].(string)
	}
	return ids
}

func mapsEqual(one, other map[string]any) bool {
	if len(one) != len(other) {
		return false
	}
	for key, value := range one {
		if other[key] != value {
			return false
		}
	}
	return true
}
