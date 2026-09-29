package cli

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// sentJSON is the body of the last writing request, compared as JSON.
func sentJSON(t *testing.T, e *testEnv) string {
	t.Helper()
	encoded, err := json.Marshal(e.lastWriteBody())
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestARepeatSendsOnlyTheToDoWhereNothingElseIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	due := "2026-09-18"
	f.LooseTodo.DueOn = &due

	env, r := e.runJSON("", "todos", "repeat", strings.ToUpper(f.LooseTodo.ID))
	expectCode(t, r, ExitOK)
	if got := sentJSON(t, e); got != `{"todoId":"`+f.LooseTodo.ID+`"}` {
		t.Fatalf("sent %s", got)
	}
	writes := e.server.WritingRequests()
	if path := writes[len(writes)-1].Path; path != "/api/v1/todo-lists/"+f.TodoList.ID+"/series" {
		t.Fatalf("the series went to %s", path)
	}
	series := decode(t, env.Data)
	if series["nextOccursOn"] != "2026-09-25" || series["state"] != "running" {
		t.Fatalf("series %v", series)
	}
	if !strings.Contains(env.Summary, "every week on Fri, from 2026-09-18") {
		t.Fatalf("summary %q", env.Summary)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "edit"), "--expect "+series["updatedAt"].(string)) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

func TestARepeatSendsTheScheduleAsGivenWithWeekdaysAsNumbers(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	due := "2026-10-02"
	f.LooseTodo.DueOn = &due

	env, r := e.runJSON("", "todos", "repeat", f.LooseTodo.ID, "--rhythm", "monthlyOnWeekday", "--weekday", "fri", "--instance", "last",
		"--interval", "2", "--ends-on", "2027-12-31", "--lead-days", "0")
	expectCode(t, r, ExitOK)
	want := `{"leadDays":0,"schedule":{"endsOn":"2027-12-31","instance":-1,"interval":2,"rhythm":"monthlyOnWeekday","weekday":5},"todoId":"` + f.LooseTodo.ID + `"}`
	if got := sentJSON(t, e); got != want {
		t.Fatalf("sent %s\nwant %s", got, want)
	}
	if schedule := decode(t, env.Data)["schedule"].(map[string]any); schedule["instance"] != -1.0 || schedule["weekday"] != 5.0 {
		t.Fatalf("schedule %v", schedule)
	}

	due = "2026-10-05"
	f.DoneTodo.DueOn = &due
	_, r = e.runJSON("", "todos", "repeat", f.DoneTodo.ID, "--rhythm", "daily", "--weekdays", "mon,wed", "--weekdays", "5")
	expectCode(t, r, ExitOK)
	if got := sentJSON(t, e); got != `{"schedule":{"rhythm":"daily","weekdays":[1,3,5]},"todoId":"`+f.DoneTodo.ID+`"}` {
		t.Fatalf("sent %s", got)
	}
}

// A detail the rhythm given does not use would be ignored by Minka without a
// word; the CLI refuses it before anything goes out, and says so afterwards
// where the rhythm is only known from the answer.
func TestADetailTheRhythmDoesNotUseIsNotSentSilently(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	due := "2026-09-18"
	f.LooseTodo.DueOn = &due

	for _, args := range [][]string{
		{"todos", "repeat", f.LooseTodo.ID, "--rhythm", "monthlyOnDay", "--weekdays", "mon"},
		{"todos", "repeat", f.LooseTodo.ID, "--rhythm", "daily", "--interval", "2"},
		{"todos", "repeat", f.LooseTodo.ID, "--rhythm", "yearly", "--weekday", "fri"},
		{"todos", "series", "edit", f.Series.ID, "--rhythm", "weekly", "--day", "3", "--force"},
		{"todos", "repeat", f.LooseTodo.ID, "--weekdays", "someday"},
		{"todos", "repeat", f.LooseTodo.ID, "--instance", "fifth"},
	} {
		before := len(e.server.Requests())
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if len(e.server.Requests()) != before {
			t.Fatalf("%v sent a request", args)
		}
	}

	env, r := e.runJSON("", "todos", "series", "edit", f.Series.ID, "--day", "3", "--force")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "Minka ignored --day: a weekly series does not use it") {
		t.Fatalf("notice %q", env.Notice)
	}
}

func TestWhatCannotRepeatSaysWhy(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "repeat", f.LooseTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesNeedsDueDay || !strings.Contains(env.Hint, "minka todos edit "+f.LooseTodo.ID+" --due") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = e.runJSON("", "todos", "repeat", f.GroupedTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoRepeatsAlready || !strings.Contains(env.Hint, "minka todos show "+f.GroupedTodo.ID) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = e.runJSON("", "todos", "repeat", f.OtherTodo.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
	due := "2026-09-18"
	f.LooseTodo.DueOn = &due
	env, r = e.runJSON("", "todos", "repeat", f.LooseTodo.ID, "--rhythm", "yearly", "--month", "4", "--day", "31")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesScheduleInvalid || env.Hint != todoSeriesScheduleHint {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// An edit sends what was given and nothing of what the series holds: the
// details Minka keeps are Minka's to keep.
func TestAnEditSendsOnlyWhatWasGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	read := decode(t, func() json.RawMessage {
		env, r := e.runJSON("", "todos", "series", "show", f.Series.ID)
		expectCode(t, r, ExitOK)
		return env.Data
	}())["updatedAt"].(string)

	before := len(e.server.Requests())
	env, r := e.runJSON("", "todos", "series", "edit", f.Series.ID, "--interval", "2", "--expect", read)
	expectCode(t, r, ExitOK)
	if got := sentJSON(t, e); got != `{"expectedUpdatedAt":"`+read+`","schedule":{"interval":2}}` {
		t.Fatalf("sent %s", got)
	}
	if requests := e.server.Requests()[before:]; len(requests) != 1 || requests[0].Method != http.MethodPatch {
		t.Fatalf("an edit that names nobody reads nothing first: %d requests", len(requests))
	}
	if schedule := decode(t, env.Data)["schedule"].(map[string]any); schedule["interval"] != 2.0 || !reflect.DeepEqual(schedule["weekdays"], []any{4.0}) {
		t.Fatalf("schedule %v", schedule)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--ends-on", "", "--force"}, `{"schedule":{"endsOn":null}}`},
		{[]string{"--rhythm", "monthlyOnDay", "--starts-on", "2026-10-31", "--force"}, `{"schedule":{"rhythm":"monthlyOnDay","startsOn":"2026-10-31"}}`},
		{[]string{"--title", "Fetch the amps", "--lead-days", "3", "--no-assignees", "--no-notify-when-done", "--force"},
			`{"assigneeIds":[],"leadDays":3,"notifiedIds":[],"title":"Fetch the amps"}`},
		{[]string{"--assignee", "me", "--assignee", "anke@example.com", "--force"}, `{"assigneeIds":["` + f.Bruno.ID + `","` + f.Anke.ID + `"]}`},
	} {
		_, r := e.runJSON("", append([]string{"todos", "series", "edit", f.Series.ID}, c.args...)...)
		expectCode(t, r, ExitOK)
		if got := sentJSON(t, e); got != c.want {
			t.Errorf("%v sent %s, want %s", c.args, got, c.want)
		}
	}
}

func TestAnEditSaysWhichSeriesItMayWriteOver(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	before := len(e.server.Requests())
	for _, args := range [][]string{
		{"--title", "New"},
		{"--title", "New", "--expect", "yesterday"},
		{"--title", "New", "--expect", "none"},
		{"--title", "New", "--expect", "2026-09-13T14:02:11.123456Z", "--force"},
		{"--force"},
	} {
		_, r := e.runJSON("", append([]string{"todos", "series", "edit", f.Series.ID}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != before {
		t.Fatal("a refused edit sent a request")
	}
}

func TestAnEditOverASeriesWrittenSinceIsRefusedWithTheSeriesAsItStands(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	read := "2026-09-13T14:02:11.123456Z"
	_, r := e.runJSON("", "todos", "series", "stop", f.Series.ID)
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "todos", "series", "edit", f.Series.ID, "--title", "Fetch the amps", "--expect", read)
	expectCode(t, r, ExitValidation)
	current := decode(t, env.Data)
	if env.Meta["apiCode"] != apiCodeTodoSeriesChanged || current["state"] != "stopped" || current["title"] != "Fetch the speakers" {
		t.Fatalf("apiCode %v, data %v", env.Meta["apiCode"], current)
	}
	if !strings.Contains(env.Hint, "--expect "+current["updatedAt"].(string)) {
		t.Fatalf("hint %q", env.Hint)
	}
	if f.Series.Title != "Fetch the speakers" {
		t.Fatal("the series was written over")
	}
}

func TestASeriesIsShownNowAndTheNewToDoIsNamed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "series", "show-now", f.Series.ID)
	expectCode(t, r, ExitOK)
	var data struct {
		Series map[string]any `json:"series"`
		TodoID string         `json:"todoId"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	made := f.TodoList.Todos[len(f.TodoList.Todos)-1]
	if data.TodoID != made.ID || made.SeriesID != f.Series.ID || data.Series["id"] != f.Series.ID || data.Series["canShowNow"] != false {
		t.Fatalf("data %s, the to-do made %s", env.Data, made.ID)
	}
	if crumbFor(t, env.Breadcrumbs, "todo") != "minka todos show "+made.ID || !strings.Contains(env.Summary, made.ID) {
		t.Fatalf("summary %q, breadcrumbs %+v", env.Summary, env.Breadcrumbs)
	}

	env, r = e.runJSON("", "todos", "series", "show-now", f.Series.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesOccurrenceAhead || !strings.Contains(env.Hint, "not yet due") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestTheNewToDoIsReadFromLocationAlone(t *testing.T) {
	id := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	for location, want := range map[string]string{
		"https://minka.example/api/v1/todos/" + id:                  id,
		"https://minka.example/api/v1/todos/" + strings.ToUpper(id): id,
		"/api/v1/todos/" + id:                                       id,
		"https://minka.example/api/v1/todo-series/" + id:            "",
		"https://minka.example/api/v1/todos/" + id + "/subtasks":    "",
		"https://minka.example/api/v1/todos/not-an-id":              "",
		"": "",
		id: "",
	} {
		got, found := todoOfLocation(location)
		if want == "" && found || want != "" && got != want {
			t.Errorf("%q: %q %v, want %q", location, got, found, want)
		}
	}
}

func TestASeriesIsStoppedAndResumedAndSaysSoTwice(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "series", "stop", f.Series.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["state"] != "stopped" || !strings.Contains(crumbFor(t, env.Breadcrumbs, "resume"), f.Series.ID) {
		t.Fatalf("stopped %s", env.Data)
	}
	env, r = e.runJSON("", "todos", "series", "stop", f.Series.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesStopped || !strings.Contains(env.Hint, "stopped already") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = e.runJSON("", "todos", "series", "show-now", f.Series.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesStopped || !strings.Contains(env.Hint, "minka todos series resume "+f.Series.ID) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	env, r = e.runJSON("", "todos", "series", "resume", f.Series.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["state"] != "running" {
		t.Fatalf("resumed %s", env.Data)
	}
	env, r = e.runJSON("", "todos", "series", "resume", f.Series.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoSeriesNotStopped || !strings.Contains(env.Hint, "--ends-on") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestASeriesIsReadAsMinkaCountsItsDays(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	text := e.run("", "todos", "series", "show", f.Series.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{
		"# Fetch the speakers",
		"Group: Stage (" + f.StageGroup.ID + ")",
		"Repeats: every week on Thu, from 2026-09-10",
		"Lead: in the list 2 days ahead",
		"State: running",
		"Next: 2026-09-17, in the list from 2026-09-15",
		"Show now: possible",
		"Assigned: Bruno",
		"To write over it: --expect 2026-09-13T14:02:11.123456Z",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}

	env, r := e.runJSON("", "todos", "series", "list", f.TodoList.ID)
	expectCode(t, r, ExitOK)
	var all []map[string]any
	if err := json.Unmarshal(env.Data, &all); err != nil || len(all) != 1 || all[0]["id"] != f.Series.ID {
		t.Fatalf("list %s", env.Data)
	}

	env, r = e.runJSON("", "todos", "series", "show", unknownID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoSeriesNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
	env, r = e.runJSON("", "todos", "series", "list", unknownID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != todoListNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestAnOccurrenceNamesItsSeriesWhereItIsRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "todos", "show", f.GroupedTodo.ID)
	expectCode(t, r, ExitOK)
	if crumbFor(t, env.Breadcrumbs, "series") != "minka todos series show "+f.Series.ID {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
	text := e.run("", "todos", "show", f.GroupedTodo.ID, "--styled")
	if !strings.Contains(text.stdout, "Repeats: the occurrence of 2026-09-10 of series "+f.Series.ID) {
		t.Fatalf("text:\n%s", text.stdout)
	}

	due := "2026-09-18"
	f.LooseTodo.DueOn = &due
	env, r = e.runJSON("", "todos", "show", f.LooseTodo.ID)
	expectCode(t, r, ExitOK)
	if crumbFor(t, env.Breadcrumbs, "repeat") != "minka todos repeat "+f.LooseTodo.ID {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	list := e.run("", "todolists", "show", f.TodoList.ID, "--styled")
	if !strings.Contains(list.stdout, "Fetch the speakers (Bruno, due 2026-09-10, overdue, repeats,") {
		t.Fatalf("list:\n%s", list.stdout)
	}
	env, _ = e.runJSON("", "todolists", "show", f.TodoList.ID)
	if crumbFor(t, env.Breadcrumbs, "series") != "minka todos series list "+f.TodoList.ID {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

func TestAWriteOnASeriesOfAnArchivedListSaysSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.TodoList.Archived = true

	due := "2026-09-18"
	f.LooseTodo.DueOn = &due
	env, r := e.runJSON("", "todos", "repeat", f.LooseTodo.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.HasSuffix(env.Hint, "minka todolists unarchive "+f.TodoList.ID) {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	env, r = e.runJSON("", "todos", "series", "stop", f.Series.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeTodoListArchived || !strings.Contains(env.Hint, "todolists unarchive") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestATitleASeriesCannotTakeSaysWhy(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for title, code := range map[string]string{
		"":                        apiCodeTodoTitleMissing,
		strings.Repeat("x", 1000): apiCodeTodoTitleTooLong,
		"Fetch\tthe amps":         apiCodeTodoTitleHasControlCharacters,
	} {
		env, r := e.runJSON("", "todos", "series", "edit", f.Series.ID, "--title", title, "--force")
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != code || !strings.Contains(env.Hint, "--title") {
			t.Errorf("%q: apiCode %v, hint %q", title, env.Meta["apiCode"], env.Hint)
		}
	}
}

func TestAnOccurrenceWithoutItsDayNamesItsSeriesAlone(t *testing.T) {
	var out strings.Builder
	renderTodo(&out, api.Todo{ID: unknownID, Series: &api.TodoSeriesReference{ID: unknownID}})
	if !strings.Contains(out.String(), "Repeats: an occurrence of series "+unknownID+"\n") || strings.Contains(out.String(), "occurrence of  ") {
		t.Fatalf("text:\n%s", out.String())
	}
}

func TestAnEditThatChangesNothingSaysSo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	read := "2026-09-13T14:02:11.123456Z"

	env, r := e.runJSON("", "todos", "series", "edit", f.Series.ID, "--lead-days", "2", "--expect", read)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "nothing changed") || !strings.Contains(env.Summary, "nothing was written") {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}
	env, r = e.runJSON("", "todos", "series", "edit", f.Series.ID, "--lead-days", "3", "--expect", read)
	expectCode(t, r, ExitOK)
	if strings.Contains(env.Notice, "nothing changed") || strings.Contains(env.Summary, "nothing was written") {
		t.Fatalf("a change: summary %q, notice %q", env.Summary, env.Notice)
	}
}
