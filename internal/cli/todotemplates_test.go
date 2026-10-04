package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

func TestTheTemplatesInUseAreListedWithTheirLibrary(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "templates")
	expectCode(t, r, ExitOK)
	library := decode(t, env.Data)
	if library["projectId"] != f.TemplateLibrary.ID || library["toolId"] != f.Templates.ID {
		t.Fatalf("the library is named as project %v, tool %v", library["projectId"], library["toolId"])
	}
	if ids := idsOf(t, library, "templates"); !slices.Equal(ids, []string{f.Template.ID, f.EmptyTemplate.ID}) {
		t.Fatalf("the templates are %v: an archived or a trashed one is among them, or the order is lost", ids)
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Path != "/api/v1/templates/todo-lists" {
		t.Fatalf("asked %s", last.Path)
	}
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool {
		return b.Cmd == "minka todolists list "+f.Templates.ID+" --archived"
	}) {
		t.Fatalf("breadcrumbs %+v do not lead to the archived templates", env.Breadcrumbs)
	}

	r = e.run("", "todolists", "templates", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Library: project "+f.TemplateLibrary.ID+", tool "+f.Templates.ID) || !strings.Contains(r.stdout, "Monthly closing (mm-yyyy)") {
		t.Fatalf("text:\n%s", r.stdout)
	}

	// The library is no project anybody works in.
	env, r = e.runJSON("", "projects", "list")
	expectCode(t, r, ExitOK)
	if slices.Contains(idsIn(listOfObjects(t, env.Data)), f.TemplateLibrary.ID) {
		t.Fatal("the template library is listed among the projects")
	}

	// Nor in the list of a Minka from before projects could be archived.
	e.server.IgnoresProjectStatus = true
	env, r = e.runJSON("", "projects", "list")
	expectCode(t, r, ExitOK)
	if ids := idsIn(listOfObjects(t, env.Data)); len(ids) == 0 || slices.Contains(ids, f.TemplateLibrary.ID) {
		t.Fatalf("a Minka without project status lists %v", ids)
	}
}

func TestSomebodyInNoLibraryIsToldSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	outOfTheLibrary(f, f.Bruno)

	env, r := e.runJSON("", "todolists", "templates")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "no template library") || !strings.Contains(env.Hint, "client") {
		t.Fatalf("hint %q does not say who has templates", env.Hint)
	}

	env, r = e.runJSON("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.Template.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTemplateUnknown || !strings.Contains(env.Hint, "no template library") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestAListIsStartedFromATemplateWithThePeopleOfTheProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.Template.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["template"] != f.Template.ID {
		t.Fatalf("the body %v names more than the template, or not it", body)
	}
	list := decode(t, env.Data)
	if list["title"] != "Monthly closing (mm-yyyy)" || list["toolId"] != f.Todos.ID || list["description"] != "<p>Every month.</p>" {
		t.Fatalf("the new list is %v in %v with %v", list["title"], list["toolId"], list["description"])
	}
	if f.Todos.Lists[0].ID != list["id"] {
		t.Fatal("the new list is not on top of the tool")
	}
	var leftOff []struct{ ID, Name string }
	if err := json.Unmarshal(fieldAsJSON(t, list, "leftOff"), &leftOff); err != nil {
		t.Fatal(err)
	}
	// The assignee, the one told when done and the one of a subtask, by name:
	// not Olga, who is on no team, and nobody of a group or a to-do in the trash.
	want := []struct{ ID, Name string }{{f.Ada.ID, "Ada"}, {f.Stranger.ID, "Stranger"}, {f.Zora.ID, "Zora"}}
	if !slices.Equal(leftOff, want) {
		t.Fatalf("leftOff %+v, want %+v", leftOff, want)
	}
	if !strings.Contains(env.Summary, "left off 3 people, no members of Test-Projekt: Ada, Stranger, Zora") {
		t.Fatalf("summary %q does not name who was left off", env.Summary)
	}

	made := f.Todos.Lists[0]
	if !made.HiddenFromClients {
		t.Fatal("the clients of the project are shown the new list")
	}
	if len(made.Groups) != 1 || made.Groups[0].Title != "Bank" || made.Groups[0].Color != "blue" {
		t.Fatalf("groups %+v: want the blue one alone", made.Groups)
	}
	if len(made.Todos) != 2 || len(made.Comments) != 0 {
		t.Fatalf("%d to-dos and %d comments were carried", len(made.Todos), len(made.Comments))
	}
	receipts := made.Todos[0]
	if !slices.Equal(receipts.AssigneeIDs, []string{f.Bruno.ID}) || !slices.Equal(receipts.NotifiedIDs, []string{f.Anke.ID}) {
		t.Fatalf("the receipts carry %v and tell %v", receipts.AssigneeIDs, receipts.NotifiedIDs)
	}
	if receipts.Subtasks[0].AssigneeID != "" || receipts.Subtasks[1].AssigneeID != f.Anke.ID {
		t.Fatalf("the subtasks are %q's and %q's", receipts.Subtasks[0].AssigneeID, receipts.Subtasks[1].AssigneeID)
	}

	r = e.run("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.Template.ID, "--title", "Monthly closing (10-2026)", "--styled")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["title"] != "Monthly closing (10-2026)" {
		t.Fatalf("--title went out as %v", body["title"])
	}
	if !strings.Contains(r.stdout, "Left off, no members of Test-Projekt") || !strings.Contains(r.stdout, "- Stranger ("+f.Stranger.ID+")") {
		t.Fatalf("the text does not name who was left off:\n%s", r.stdout)
	}
	if f.Todos.Lists[0].Title != "Monthly closing (10-2026)" || f.Todos.Lists[1].ID != list["id"] {
		t.Fatal("a second run did not make a second list on top")
	}
}

func TestAListOfATemplateWithNobodyOnItLeavesNobodyOff(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.EmptyTemplate.ID)
	expectCode(t, r, ExitOK)
	if raw := string(fieldAsJSON(t, decode(t, env.Data), "leftOff")); raw != "[]" {
		t.Fatalf("leftOff is %s", raw)
	}
	if strings.Contains(env.Summary, "left off") {
		t.Fatalf("summary %q", env.Summary)
	}

	r = e.run("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.EmptyTemplate.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Left off: nobody") {
		t.Fatalf("text:\n%s", r.stdout)
	}

	// One person left off is no member, in the singular.
	f.EmptyTemplate.Todos = []*fakeapi.Todo{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", Title: "Say hello", AssigneeIDs: []string{f.Stranger.ID}}}
	env, r = e.runJSON("", "todolists", "start", "--tool", f.Todos.ID, "--template", f.EmptyTemplate.ID)
	expectCode(t, r, ExitOK)
	if !strings.HasSuffix(env.Summary, "; left off 1 person, no member of Test-Projekt: Stranger") {
		t.Fatalf("summary %q", env.Summary)
	}
}

func TestAListThatCannotBeMadeFromATemplateSaysWhy(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(e *testEnv)
		args    func(f *fakeapi.Fixture) []string
		exit    int
		apiCode string
		hint    string
	}{
		{name: "an unknown template", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"}
		}, exit: ExitValidation, apiCode: apiCodeTemplateUnknown, hint: "minka todolists templates"},
		{name: "a template in the trash", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.TrashedTemplate.ID}
		}, exit: ExitValidation, apiCode: apiCodeTemplateUnknown, hint: "in the trash"},
		{name: "a list of a project", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.EmptyTodoList.ID}
		}, exit: ExitValidation, apiCode: apiCodeTemplateUnknown, hint: "a list of a project"},
		{name: "an archived template", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.ArchivedTemplate.ID}
		}, exit: ExitValidation, apiCode: apiCodeTemplateArchived, hint: "minka todolists unarchive "},
		{name: "into the library", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Templates.ID, "--template", f.Template.ID}
		}, exit: ExitValidation, apiCode: apiCodeTemplateIntoLibrary, hint: "Duplicate"},
		{name: "an empty title", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.Template.ID, "--title", ""}
		}, exit: ExitValidation, apiCode: "todoListTitleMissing", hint: "Leave --title out"},
		{name: "an unknown tool", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--template", f.Template.ID}
		}, exit: ExitNotFound, apiCode: "notFound", hint: "no longer in use"},
		{name: "a tool no longer in use", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.TodosOutOfUse.ID, "--template", f.Template.ID}
		}, exit: ExitNotFound, apiCode: "notFound", hint: "no longer in use"},
		{name: "an archived template in an archived project", prepare: func(e *testEnv) { e.fixture.Project.Archived = true }, args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.ArchivedTemplate.ID}
		}, exit: ExitValidation, apiCode: apiCodeTemplateArchived},
		{name: "an archived template in an archived project, titled", prepare: func(e *testEnv) { e.fixture.Project.Archived = true }, args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.ArchivedTemplate.ID, "--title", "Titled"}
		}, exit: ExitValidation, apiCode: apiCodeProjectArchived},
		{name: "an archived template, titled", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.ArchivedTemplate.ID, "--title", "Titled"}
		}, exit: ExitValidation, apiCode: apiCodeTemplateArchived},
		{name: "an archived template, with an empty title", args: func(f *fakeapi.Fixture) []string {
			return []string{"--tool", f.Todos.ID, "--template", f.ArchivedTemplate.ID, "--title", ""}
		}, exit: ExitValidation, apiCode: "todoListTitleMissing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			f := e.fixture
			if c.prepare != nil {
				c.prepare(e)
			}
			before := len(f.Todos.Lists) + len(f.Templates.Lists)

			env, r := e.runJSON("", append([]string{"todolists", "start"}, c.args(f)...)...)
			expectCode(t, r, c.exit)
			if env.Meta["apiCode"] != c.apiCode {
				t.Fatalf("apiCode %v, want %q", env.Meta["apiCode"], c.apiCode)
			}
			if !strings.Contains(env.Hint, c.hint) {
				t.Fatalf("hint %q does not say %q", env.Hint, c.hint)
			}
			if len(f.Todos.Lists)+len(f.Templates.Lists) != before {
				t.Fatal("a list was made")
			}
		})
	}
}

func TestATemplateIsNamedByAnIDBeforeAnythingGoesOut(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	r := e.run("", "todolists", "start", "--tool", f.Todos.ID, "--template", "Monthly closing", "--json")
	expectCode(t, r, ExitUsage)
	r = e.run("", "todolists", "start", "--tool", f.Todos.ID, "--json")
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != 0 {
		t.Fatal("a request went out")
	}
}

func TestATemplateTakesNoDaysAndNoTicks(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	todo := f.TemplateTodo.ID

	for _, args := range [][]string{
		{"todos", "create", "--list", f.Template.ID, "--title", "Pay", "--due", "2026-10-01"},
		{"todos", "edit", todo, "--starts-on", "2026-10-01"},
		{"todos", "subtasks", "create", todo, "Count", "--due", "2026-10-01"},
		{"todos", "subtasks", "edit", todo, "1", "--due", "2026-10-01"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTemplateTakesNoDates || !strings.Contains(env.Hint, "takes no days") {
			t.Fatalf("%v: apiCode %v, hint %q", args, env.Meta["apiCode"], env.Hint)
		}
	}
	for _, args := range [][]string{
		{"todos", "complete", todo},
		{"todos", "subtasks", "complete", todo, "1"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeTemplateTakesNoTicks || !strings.Contains(env.Hint, "minka todolists start") {
			t.Fatalf("%v: apiCode %v, hint %q", args, env.Meta["apiCode"], env.Hint)
		}
	}

	// Taking a day off and opening again are no refusals.
	expectCode(t, e.run("", "todos", "edit", todo, "--due", "", "--json"), ExitOK)
	expectCode(t, e.run("", "todos", "edit", todo, "--starts-on", "", "--due", "", "--json"), ExitOK)
	expectCode(t, e.run("", "todos", "subtasks", "edit", todo, "1", "--due", "", "--json"), ExitOK)
	if body := e.lastWriteBody(); body["dueOn"] != nil {
		t.Fatalf("the subtask's day went out as %v, want null", body["dueOn"])
	}
	expectCode(t, e.run("", "todos", "reopen", todo, "--json"), ExitOK)
	expectCode(t, e.run("", "todos", "subtasks", "reopen", todo, "1", "--json"), ExitOK)
}

// outOfTheLibrary takes a person out of the template library, where every
// signed-in person of the fixture is.
func outOfTheLibrary(f *fakeapi.Fixture, person *fakeapi.Person) {
	f.TemplateLibrary.MemberIDs = slices.DeleteFunc(f.TemplateLibrary.MemberIDs, func(id string) bool { return id == person.ID })
}

func fieldAsJSON(t *testing.T, object map[string]any, key string) json.RawMessage {
	t.Helper()
	value, present := object[key]
	if !present {
		t.Fatalf("%q is missing", key)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
