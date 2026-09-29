package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// seriesAnswer is what the server answered: the status with the error code
// behind it, the decoded body, the body as sent and the Location header.
type seriesAnswer struct {
	got      string
	body     map[string]any
	raw      string
	location string
}

func callSeries(t *testing.T, s *Server, token, method, path string, body any) seriesAnswer {
	t.Helper()
	encoded := "{}"
	switch value := body.(type) {
	case nil:
	case string:
		encoded = value
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		encoded = string(bytes)
	}
	request, err := http.NewRequest(method, s.URL+"/api/v1"+path, strings.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	answer := seriesAnswer{got: fmt.Sprint(response.StatusCode), raw: string(raw), location: response.Header.Get("Location")}
	_ = json.Unmarshal(raw, &answer.body)
	if refusal, ok := answer.body["error"].(map[string]any); ok && response.StatusCode != http.StatusNotFound {
		answer.got += " " + fmt.Sprint(refusal["code"])
	}
	return answer
}

// addTodoDue adds a to-do to the list and hands back its ID.
func addTodoDue(t *testing.T, s *Server, f *Fixture, title, dueOn string) string {
	t.Helper()
	answer := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.TodoList.ID+"/todos", map[string]any{"title": title, "dueOn": dueOn})
	if answer.got != "201" {
		t.Fatalf("adding a to-do: %s %s", answer.got, answer.raw)
	}
	return answer.body["id"].(string)
}

func repeat(t *testing.T, s *Server, f *Fixture, todoID string, schedule map[string]any) map[string]any {
	t.Helper()
	answer := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.TodoList.ID+"/series", map[string]any{"todoId": todoID, "schedule": schedule})
	if answer.got != "201" {
		t.Fatalf("repeating: %s %s", answer.got, answer.raw)
	}
	return answer.body
}

func pick(values map[string]any, keys ...string) map[string]any {
	picked := map[string]any{}
	for _, key := range keys {
		picked[key] = values[key]
	}
	return picked
}

func expectPicked(t *testing.T, name string, values map[string]any, want map[string]any) {
	t.Helper()
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	got := pick(values, keys...)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: %v, want %v", name, got, want)
	}
}

func TestATodoIsMadeToRepeatAndTheSeriesIsRead(t *testing.T) {
	s, f := New(t)
	todoID := addTodoDue(t, s, f, "Abrechnung Projekte", "2030-01-04")

	created := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.TodoList.ID+"/series", map[string]any{
		"todoId":   strings.ToUpper(todoID),
		"schedule": map[string]any{"rhythm": "monthlyOnWeekday", "weekday": 5, "instance": 1},
		"leadDays": 7,
	})
	if created.got != "201" {
		t.Fatalf("%s %s", created.got, created.raw)
	}
	seriesID := created.body["id"].(string)
	if !strings.HasSuffix(created.location, "/api/v1/todo-series/"+seriesID) {
		t.Errorf("Location %q", created.location)
	}
	expectPicked(t, "schedule", created.body["schedule"].(map[string]any), map[string]any{
		"rhythm": "monthlyOnWeekday", "weekdays": []any{}, "interval": 1.0, "day": nil, "weekday": 5.0,
		"instance": 1.0, "month": nil, "startsOn": "2030-01-04", "endsOn": nil,
	})
	expectPicked(t, "series", created.body, map[string]any{
		"listId": f.TodoList.ID, "title": "Abrechnung Projekte", "leadDays": 7.0, "state": "running", "endedAt": nil,
		"nextOccursOn": "2030-02-01", "nextShowsOn": "2030-01-25", "canShowNow": false,
	})
	if created.body["createdAt"] != created.body["updatedAt"] {
		t.Errorf("a new series was written once: %v, %v", created.body["createdAt"], created.body["updatedAt"])
	}

	todo := callSeries(t, s, f.Token, http.MethodGet, "/todos/"+todoID, nil)
	if !reflect.DeepEqual(todo.body["series"], map[string]any{"id": seriesID, "occursOn": "2030-01-04"}) {
		t.Errorf("the to-do names %v", todo.body["series"])
	}
	list := callSeries(t, s, f.Token, http.MethodGet, "/todo-lists/"+f.TodoList.ID, nil)
	lines := list.body["todos"].([]any)
	if last := lines[len(lines)-1].(map[string]any); last["seriesId"] != seriesID || lines[0].(map[string]any)["seriesId"] != nil {
		t.Errorf("the lines name %v and %v", lines[0], last)
	}
	var series []map[string]any
	all := callSeries(t, s, f.Token, http.MethodGet, "/todo-lists/"+f.TodoList.ID+"/series", nil)
	_ = json.Unmarshal([]byte(all.raw), &series)
	if len(series) != 2 || series[0]["id"] != f.Series.ID || series[1]["id"] != seriesID {
		t.Errorf("the series of the list, the one made first first: %s", all.raw)
	}
	if read := callSeries(t, s, f.Token, http.MethodGet, "/todo-series/"+seriesID, nil); read.raw != created.raw {
		t.Errorf("read %s\ncreated %s", read.raw, created.raw)
	}
	once := callSeries(t, s, f.Token, http.MethodGet, "/todos/"+f.LooseTodo.ID, nil)
	if value, named := once.body["series"]; !named || value != nil {
		t.Errorf("a to-do that is no occurrence names %v", value)
	}
}

func TestWhatCannotRepeatIsRefusedWithItsCase(t *testing.T) {
	s, f := New(t)
	todoID := addTodoDue(t, s, f, "Abrechnung Projekte", "2030-01-04")
	elsewhere := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.EmptyTodoList.ID+"/todos", map[string]any{"title": "Woanders", "dueOn": "2030-01-04"}).body["id"]
	path := "/todo-lists/" + f.TodoList.ID + "/series"

	for _, c := range []struct {
		want string
		body map[string]any
	}{
		{"422 todoSeriesNeedsDueDay", map[string]any{"todoId": f.LooseTodo.ID}},
		{"422 todoToRepeatUnknown", map[string]any{"todoId": elsewhere}},
		{"422 todoToRepeatUnknown", map[string]any{}},
		{"422 todoToRepeatUnknown", map[string]any{"todoId": "no identifier"}},
		{"422 todoSeriesScheduleInvalid", map[string]any{"todoId": todoID, "schedule": map[string]any{"rhythm": "hourly"}}},
		{"422 todoSeriesLeadDaysInvalid", map[string]any{"todoId": todoID, "leadDays": 400}},
		{"422 todoSeriesDatesOutOfOrder", map[string]any{"todoId": todoID, "schedule": map[string]any{"rhythm": "weekly", "endsOn": "2029-12-31"}}},
		{"422 todoSeriesDatesNotADate", map[string]any{"todoId": todoID, "schedule": map[string]any{"endsOn": "4.1.2030"}}},
		{"400 malformedBody", map[string]any{"todoId": todoID, "schedule": []any{"weekly"}}},
		{"422 todoRepeatsAlready", map[string]any{"todoId": f.GroupedTodo.ID}},
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, path, c.body); answer.got != c.want {
			t.Errorf("%v: %s, want %s", c.body, answer.got, c.want)
		}
	}
	if len(f.TodoList.Series) != 1 {
		t.Fatalf("a refused series was made: %d", len(f.TodoList.Series))
	}
	if got := callSeries(t, s, f.Token, http.MethodPost, path, map[string]any{"todoId": todoID}).got; got != "201" {
		t.Fatalf("repeating: %s", got)
	}
	if got := callSeries(t, s, f.Token, http.MethodPost, path, map[string]any{"todoId": todoID}).got; got != "422 todoRepeatsAlready" {
		t.Fatalf("repeating twice: %s", got)
	}
}

func TestAnEditKeepsTheDayDetailsWhileRhythmAndFirstDayStay(t *testing.T) {
	s, f := New(t)
	seriesID := repeat(t, s, f, addTodoDue(t, s, f, "Abrechnung Projekte", "2030-01-04"), map[string]any{"rhythm": "monthlyOnDay", "day": 20})["id"].(string)
	path := "/todo-series/" + seriesID

	edited := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"schedule": map[string]any{"interval": 2}, "assigneeIds": []string{f.Anke.ID}})
	expectPicked(t, "an interval alone", edited.body["schedule"].(map[string]any), map[string]any{"interval": 2.0, "day": 20.0})
	renamed := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Abrechnung", "leadDays": 3})
	expectPicked(t, "renamed", renamed.body, map[string]any{"title": "Abrechnung", "leadDays": 3.0})
	if !reflect.DeepEqual(renamed.body["schedule"], edited.body["schedule"]) || !reflect.DeepEqual(renamed.body["assignees"], edited.body["assignees"]) {
		t.Errorf("what the body leaves out stays: %v", renamed.body)
	}
	moved := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"schedule": map[string]any{"startsOn": "2030-01-10"}})
	expectPicked(t, "a new first day brings its own day", moved.body["schedule"].(map[string]any), map[string]any{"interval": 2.0, "day": 10.0, "startsOn": "2030-01-10"})

	for body, want := range map[string]string{
		`{}`: "422 nothingToChange",
		`{"assigneeIds": ["` + f.Stranger.ID + `"]}`: "422 personNotAProjectMember",
		`{"title": " "}`: "422 todoTitleMissing",
		`{"schedule": {"rhythm": "yearly", "month": 4, "day": 31}}`: "422 todoSeriesScheduleInvalid",
		`{"schedule": {"rhythm": "weekly", "weekdays": []}}`:        "422 todoSeriesScheduleInvalid",
		`{"schedule": {"endsOn": "2030-01-09"}}`:                    "422 todoSeriesDatesOutOfOrder",
	} {
		if got := callSeries(t, s, f.Token, http.MethodPatch, path, body).got; got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	ended := callSeries(t, s, f.Token, http.MethodPatch, path, `{"schedule": {"endsOn": "2030-06-30"}}`)
	unended := callSeries(t, s, f.Token, http.MethodPatch, path, `{"schedule": {"endsOn": ""}}`)
	if ended.body["schedule"].(map[string]any)["endsOn"] != "2030-06-30" || unended.body["schedule"].(map[string]any)["endsOn"] != nil {
		t.Errorf("an end is set and taken off again: %v, %v", ended.body["schedule"], unended.body["schedule"])
	}
	callSeries(t, s, f.Token, http.MethodPatch, path, `{"schedule": {"endsOn": "2030-06-30"}}`)
	unsetByNull := callSeries(t, s, f.Token, http.MethodPatch, path, `{"schedule": {"endsOn": null}}`)
	if unsetByNull.got != "200" || unsetByNull.body["schedule"].(map[string]any)["endsOn"] != nil {
		t.Errorf("null in endsOn takes the end off: %s %v", unsetByNull.got, unsetByNull.body["schedule"])
	}
}

// The fake's today is Sunday, 13 September 2026.
func TestASeriesIsShownNowStoppedAndResumed(t *testing.T) {
	s, f := New(t)
	seriesID := repeat(t, s, f, addTodoDue(t, s, f, "Kasse zählen", "2026-09-13"), map[string]any{"rhythm": "weekly"})["id"].(string)
	path := "/todo-series/" + seriesID

	shown := callSeries(t, s, f.Token, http.MethodPost, path+"/show-now", nil)
	if shown.got != "201" || shown.body["id"] != seriesID {
		t.Fatalf("%s %s", shown.got, shown.raw)
	}
	occurrence := callSeries(t, s, f.Token, http.MethodGet, strings.TrimPrefix(shown.location, s.URL+"/api/v1"), nil)
	if !reflect.DeepEqual(occurrence.body["series"], map[string]any{"id": seriesID, "occursOn": "2026-09-20"}) || occurrence.body["dueOn"] != "2026-09-20" {
		t.Fatalf("the occurrence at %s: %s", shown.location, occurrence.raw)
	}
	expectPicked(t, "after show now", shown.body, map[string]any{"nextOccursOn": "2026-09-27", "canShowNow": false})
	if got := callSeries(t, s, f.Token, http.MethodPost, path+"/show-now", nil).got; got != "422 todoSeriesOccurrenceAhead" {
		t.Errorf("at most one occurrence not yet due: %s", got)
	}

	if got := callSeries(t, s, f.Token, http.MethodDelete, path, nil).got; got != "404" {
		t.Errorf("DELETE stops nothing: %s", got)
	}
	stopped := callSeries(t, s, f.Token, http.MethodPost, path+"/stop", nil)
	expectPicked(t, "stopped", stopped.body, map[string]any{"state": "stopped", "nextOccursOn": nil, "nextShowsOn": nil, "canShowNow": false})
	if stopped.got != "200" || stopped.body["endedAt"] == nil {
		t.Fatalf("%s %s", stopped.got, stopped.raw)
	}
	if read := callSeries(t, s, f.Token, http.MethodGet, path, nil); read.raw != stopped.raw {
		t.Errorf("read after the stop: %s", read.raw)
	}
	for _, way := range []string{"/stop", "/show-now"} {
		if got := callSeries(t, s, f.Token, http.MethodPost, path+way, nil).got; got != "422 todoSeriesStopped" {
			t.Errorf("%s of a stopped series: %s", way, got)
		}
	}

	s.now = s.now.AddDate(0, 0, 20) // Saturday, 3 October
	resumed := callSeries(t, s, f.Token, http.MethodPost, path+"/resume", nil)
	expectPicked(t, "resumed", resumed.body, map[string]any{"state": "running", "endedAt": nil, "nextOccursOn": "2026-10-04"})
	if got := callSeries(t, s, f.Token, http.MethodPost, path+"/resume", nil).got; got != "422 todoSeriesNotStopped" {
		t.Errorf("resuming a running series: %s", got)
	}
}

func TestASeriesPastItsLastDayIsFinishedAndOneBehindOwesTheLatestDayOnly(t *testing.T) {
	s, f := New(t)
	finished := repeat(t, s, f, addTodoDue(t, s, f, "Kasse zählen", "2026-09-13"), map[string]any{"rhythm": "weekly", "endsOn": "2026-09-13"})
	expectPicked(t, "finished", finished, map[string]any{"state": "finished", "nextOccursOn": nil, "nextShowsOn": nil, "canShowNow": false})
	if got := callSeries(t, s, f.Token, http.MethodPost, "/todo-series/"+finished["id"].(string)+"/show-now", nil).got; got != "422 todoSeriesFinished" {
		t.Errorf("show now past the end: %s", got)
	}
	if got := callSeries(t, s, f.Token, http.MethodPost, "/todo-series/"+finished["id"].(string)+"/resume", nil).got; got != "422 todoSeriesNotStopped" {
		t.Errorf("a finished series is not stopped: %s", got)
	}

	behind := repeat(t, s, f, addTodoDue(t, s, f, "Abrechnung", "2020-01-06"), map[string]any{"rhythm": "weekly"})
	expectPicked(t, "behind", behind, map[string]any{"nextOccursOn": "2026-09-07", "canShowNow": true})
	path := "/todo-series/" + behind["id"].(string)
	callSeries(t, s, f.Token, http.MethodPost, path+"/stop", nil)
	resumed := callSeries(t, s, f.Token, http.MethodPost, path+"/resume", nil)
	expectPicked(t, "resumed", resumed.body, map[string]any{"nextOccursOn": "2026-09-14"})
}

func TestTheFixtureSeriesShowsItsNextOneAfterToday(t *testing.T) {
	s, f := New(t)
	series := callSeries(t, s, f.Token, http.MethodGet, "/todo-series/"+f.Series.ID, nil)
	expectPicked(t, "fixture", series.body, map[string]any{
		"state": "running", "nextOccursOn": "2026-09-17", "nextShowsOn": "2026-09-15", "canShowNow": true, "groupId": f.StageGroup.ID,
	})
	shown := callSeries(t, s, f.Token, http.MethodPost, "/todo-series/"+f.Series.ID+"/show-now", nil)
	todo := callSeries(t, s, f.Token, http.MethodGet, strings.TrimPrefix(shown.location, s.URL+"/api/v1"), nil)
	expectPicked(t, "the occurrence", todo.body, map[string]any{"title": "Fetch the speakers", "groupId": f.StageGroup.ID, "dueOn": "2026-09-17"})
	if !reflect.DeepEqual(todo.body["assignees"], []any{map[string]any{"id": f.Bruno.ID, "name": "Bruno"}}) || todo.body["creator"].(map[string]any)["id"] != f.Anke.ID {
		t.Errorf("the occurrence takes the people of the series and its creator: %s", todo.raw)
	}
}

func TestNobodyWhoMayNotReadTheListFindsItsSeries(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.TodoList.HiddenFromClients = true
	series := "/todo-series/" + f.Series.ID

	for _, token := range []string{f.StrangerToken, "minka_pat_client"} {
		for _, c := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/todo-lists/" + f.TodoList.ID + "/series", map[string]any{"todoId": f.LooseTodo.ID}},
			{http.MethodPatch, series, map[string]any{"title": "Übernommen"}},
			{http.MethodPost, series + "/show-now", nil},
			{http.MethodPost, series + "/resume", nil},
			{http.MethodPost, series + "/stop", nil},
			{http.MethodGet, "/todo-lists/" + f.TodoList.ID + "/series", nil},
			{http.MethodGet, series, nil},
			{http.MethodGet, "/todo-series/not-an-identifier", nil},
			{http.MethodGet, "/todo-series/01990000-0000-4000-8000-000000000000", nil},
		} {
			answer := callSeries(t, s, token, c.method, c.path, c.body)
			if answer.got != "404" || answer.raw != NotFoundBody {
				t.Errorf("%s %s %s: %s %s", token, c.method, c.path, answer.got, answer.raw)
			}
		}
	}
	if f.Series.Title != "Fetch the speakers" || f.Series.EndedAt != nil {
		t.Fatal("somebody who may not read the list changed its series")
	}
}

func TestEveryWayOfWritingIntoAnArchivedProjectOrListIsRefusedFirst(t *testing.T) {
	s, f := New(t)
	notYetRepeating := addTodoDue(t, s, f, "Kasse zählen", "2030-01-07")
	series := "/todo-series/" + f.Series.ID
	ways := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/todo-lists/" + f.TodoList.ID + "/series", map[string]any{"todoId": notYetRepeating}},
		{http.MethodPatch, series, map[string]any{"title": "Übernommen", "expectedUpdatedAt": "2020-01-01T00:00:00Z", "assigneeIds": []string{f.Stranger.ID}}},
		{http.MethodPost, series + "/show-now", nil},
		{http.MethodPost, series + "/resume", nil},
		{http.MethodPost, series + "/stop", nil},
	}

	f.Project.Archived = true
	for _, way := range ways {
		if got := callSeries(t, s, f.Token, way.method, way.path, way.body).got; got != "422 projectArchived" {
			t.Errorf("%s %s in an archived project: %s", way.method, way.path, got)
		}
	}
	if got := callSeries(t, s, f.Token, http.MethodGet, series, nil).got; got != "200" {
		t.Errorf("reading stays open: %s", got)
	}

	f.Project.Archived, f.TodoList.Archived = false, true
	for _, way := range ways {
		if got := callSeries(t, s, f.Token, way.method, way.path, way.body).got; got != "422 todoListArchived" {
			t.Errorf("%s %s in an archived list: %s", way.method, way.path, got)
		}
	}
	read := callSeries(t, s, f.Token, http.MethodGet, series, nil)
	expectPicked(t, "in an archived list", read.body, map[string]any{"state": "running", "nextOccursOn": "2026-09-17", "nextShowsOn": nil, "canShowNow": false})
	if f.Series.Title != "Fetch the speakers" || f.Series.EndedAt != nil || len(f.TodoList.Series) != 1 {
		t.Fatal("a refused write changed the series")
	}
}

func TestAnEditOverAVersionNotReadIsRefusedWithTheSeriesAsItStands(t *testing.T) {
	s, f := New(t)
	path := "/todo-series/" + f.Series.ID
	read := callSeries(t, s, f.Token, http.MethodGet, path, nil).body["updatedAt"].(string)

	first := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Abrechnung", "expectedUpdatedAt": read})
	if first.got != "200" || first.body["updatedAt"] == read {
		t.Fatalf("%s %s", first.got, first.raw)
	}
	second := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Kasse", "expectedUpdatedAt": read, "assigneeIds": []string{f.Stranger.ID}})
	if second.got != "409 todoSeriesChanged" || !reflect.DeepEqual(second.body["series"], first.body) {
		t.Fatalf("%s %s", second.got, second.raw)
	}
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Kasse", "expectedUpdatedAt": "yesterday"}).got; got != "400 malformedBody" {
		t.Errorf("a moment that is none: %s", got)
	}
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Kasse", "expectedUpdatedAt": nil}).got; got != "200" {
		t.Errorf("null expects nothing: %s", got)
	}
	stopped := callSeries(t, s, f.Token, http.MethodPost, path+"/stop", nil).body["updatedAt"]
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Abrechnung", "expectedUpdatedAt": first.body["updatedAt"]}).got; got != "409 todoSeriesChanged" || stopped == first.body["updatedAt"] {
		t.Errorf("a stop is a write too: %s", got)
	}
}

// The body is checked in full before the series is looked for: every case
// below is aimed at a series that does not exist.
func TestABodyIsCheckedBeforeTheSeriesIsLookedFor(t *testing.T) {
	s, f := New(t)
	invented := "/todo-series/01990000-0000-4000-8000-000000000000"
	for body, want := range map[string]string{
		`{"schedule": "weekly"}`:                                    "400 malformedBody",
		`{"schedule": null}`:                                        "400 malformedBody",
		`{"assigneeIds": null}`:                                     "400 malformedBody",
		`{"notifiedIds": [5]}`:                                      "400 malformedBody",
		`{"schedule": {"interval": 1.0}}`:                           "400 malformedBody",
		`{"schedule": {"weekdays": [1, "2"]}}`:                      "400 malformedBody",
		`{"leadDays": "7"}`:                                         "400 malformedBody",
		`{"title": "Kasse", "expectedUpdatedAt": 5}`:                "400 malformedBody",
		`{"leadDays": null}`:                                        "422 todoSeriesLeadDaysInvalid",
		`{"title": null}`:                                           "422 todoTitleMissing",
		`{"schedule": {}}`:                                          "422 nothingToChange",
		`{"schedule": {"rhythm": null}}`:                            "422 nothingToChange",
		`{"expectedUpdatedAt": "2026-09-13T14:02:11.123456Z"}`:      "422 nothingToChange",
		`{"schedule": {"rhythm": "daily", "interval": 0}}`:          "422 todoSeriesScheduleInvalid",
		`{"schedule": {"rhythm": "yearly", "interval": 500}}`:       "422 todoSeriesScheduleInvalid",
		`{"schedule": {"weekday": 8}}`:                              "422 todoSeriesScheduleInvalid",
		`{"schedule": {"instance": 5}}`:                             "422 todoSeriesScheduleInvalid",
		`{"schedule": {"endsOn": "2030-02-30"}}`:                    "422 todoSeriesDatesNotADate",
		`{"schedule": {"startsOn": ""}}`:                            "422 todoSeriesDatesNotADate",
		`{"assigneeIds": ["somebody"]}`:                             "422 personNotAProjectMember",
		`{"schedule": {"rhythm": "weekly", "weekdays": []}}`:        "404",
		`{"schedule": {"rhythm": "yearly", "month": 4, "day": 31}}`: "404",
		`{"title": "Kasse"}`:                                        "404",
	} {
		if got := callSeries(t, s, f.Token, http.MethodPatch, invented, body).got; got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if got := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/01990000-0000-4000-8000-000000000000/series", map[string]any{"todoId": f.LooseTodo.ID, "leadDays": "7"}).got; got != "400 malformedBody" {
		t.Errorf("lead days as text at a list that is not there: %s", got)
	}
	// The type of every field is held before any value: a 400 of a later
	// field comes before a 422 of an earlier one. These are Minka's own cases.
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/todo-lists/" + f.TodoList.ID + "/series", map[string]any{"todoId": "none", "leadDays": "7"}},
		{http.MethodPost, "/todo-lists/" + f.TodoList.ID + "/series", map[string]any{"schedule": map[string]any{"interval": "two"}}},
		{http.MethodPost, "/todo-lists/" + f.TodoList.ID + "/series", map[string]any{"todoId": f.LooseTodo.ID, "leadDays": 999, "schedule": map[string]any{"weekdays": "Monday"}}},
		{http.MethodPatch, invented, map[string]any{"title": "", "assigneeIds": "Anke"}},
		{http.MethodPatch, invented, map[string]any{"assigneeIds": []string{"none"}, "notifiedIds": []int{1}}},
		{http.MethodPatch, invented, map[string]any{"schedule": map[string]any{"interval": 0}, "expectedUpdatedAt": "yesterday"}},
		{http.MethodPatch, invented, map[string]any{"leadDays": 999, "schedule": map[string]any{"day": "1"}}},
	} {
		if got := callSeries(t, s, f.Token, c.method, c.path, c.body).got; got != "400 malformedBody" {
			t.Errorf("%s %v: %s", c.method, c.body, got)
		}
	}
	// Among the values, todoId is held first.
	if got := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.TodoList.ID+"/series", map[string]any{"todoId": "none", "leadDays": 999}).got; got != "422 todoToRepeatUnknown" {
		t.Errorf("no identifier beside a lead out of range: %s", got)
	}
	if got := callSeries(t, s, f.Token, http.MethodPost, "/todo-lists/"+f.TodoList.ID+"/series", map[string]any{"todoId": f.GroupedTodo.ID, "leadDays": nil}).got; got != "422 todoSeriesLeadDaysInvalid" {
		t.Errorf("lead days of null: %s", got)
	}

	request, _ := http.NewRequest(http.MethodPatch, s.URL+"/api/v1"+invented, strings.NewReader("title=Kasse"))
	request.Header.Set("Authorization", "Bearer "+f.Token)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("a form: %d", response.StatusCode)
	}
}

func TestPeopleAreSetAsAWholeAndStayWhereTheBodyNamesNone(t *testing.T) {
	s, f := New(t)
	path := "/todo-series/" + f.Series.ID
	twice := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"assigneeIds": []string{f.Anke.ID, f.Client.ID, f.Anke.ID}})
	if twice.got != "200" || len(twice.body["assignees"].([]any)) != 2 {
		t.Fatalf("a person named twice stands on it once: %s", twice.raw)
	}

	f.TodoList.HiddenFromClients = true
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"notifiedIds": []string{f.Client.ID}}).got; got != "422 personNotAProjectMember" {
		t.Errorf("a client the list is hidden from: %s", got)
	}
	kept := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Abrechnung"})
	if assignees := kept.body["assignees"].([]any); len(assignees) != 1 || len(f.Series.AssigneeIDs) != 2 {
		t.Errorf("the client stays stored and shows nowhere: %v, stored %v", assignees, f.Series.AssigneeIDs)
	}
	f.TodoList.HiddenFromClients = false
	if assignees := callSeries(t, s, f.Token, http.MethodGet, path, nil).body["assignees"].([]any); len(assignees) != 2 {
		t.Errorf("the client is back once the list is shown again: %v", assignees)
	}
}

func TestASeriesWhoseGroupIsInTheTrashMakesItsToDosLoose(t *testing.T) {
	s, f := New(t)
	f.StageGroup.Trashed = true
	series := callSeries(t, s, f.Token, http.MethodGet, "/todo-series/"+f.Series.ID, nil)
	expectPicked(t, "loose", series.body, map[string]any{"groupId": nil, "group": nil})
	shown := callSeries(t, s, f.Token, http.MethodPost, "/todo-series/"+f.Series.ID+"/show-now", nil)
	todo := callSeries(t, s, f.Token, http.MethodGet, strings.TrimPrefix(shown.location, s.URL+"/api/v1"), nil)
	if todo.got != "200" || todo.body["groupId"] != nil {
		t.Errorf("the occurrence stands loose in the list: %s %s", todo.got, todo.raw)
	}
}

func TestAWriteStandsAMomentAfterTheOneBefore(t *testing.T) {
	s, f := New(t)
	s.now = f.Series.UpdatedAt.Add(-time.Hour)
	before := f.Series.UpdatedAt
	callSeries(t, s, f.Token, http.MethodPatch, "/todo-series/"+f.Series.ID, map[string]any{"leadDays": 1})
	if !f.Series.UpdatedAt.After(before) {
		t.Fatalf("updatedAt %v, before %v", f.Series.UpdatedAt, before)
	}
}

// An edit that changes nothing writes nothing: the answer is the series as it
// stands, updatedAt stays, and a version not read is then no conflict.
func TestAnEditThatChangesNothingWritesNothing(t *testing.T) {
	s, f := New(t)
	path := "/todo-series/" + f.Series.ID
	read := callSeries(t, s, f.Token, http.MethodGet, path, nil)
	sameAgain := map[string]any{
		"title": " Fetch the speakers ", "assigneeIds": []string{f.Bruno.ID, f.Bruno.ID},
		"schedule": map[string]any{"rhythm": "weekly", "interval": 1}, "leadDays": 2,
	}

	s.now = s.now.Add(time.Hour)
	unchanged := callSeries(t, s, f.Token, http.MethodPatch, path, sameAgain)
	if unchanged.got != "200" || unchanged.raw != read.raw {
		t.Fatalf("an identical write: %s %s", unchanged.got, unchanged.raw)
	}
	callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"leadDays": 3})
	callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"leadDays": 2})
	stale := read.body["updatedAt"]
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"leadDays": 2, "expectedUpdatedAt": stale}).got; got != "200" {
		t.Errorf("an identical write over a version not read is no conflict: %s", got)
	}
	for name, body := range map[string]map[string]any{
		"other people":            {"assigneeIds": []string{f.Anke.ID}, "expectedUpdatedAt": stale},
		"somebody told when done": {"notifiedIds": []string{f.Anke.ID}, "expectedUpdatedAt": stale},
		"another title":           {"title": "Fetch the amps", "expectedUpdatedAt": stale},
		"another end":             {"schedule": map[string]any{"endsOn": "2030-01-01"}, "expectedUpdatedAt": stale},
	} {
		if got := callSeries(t, s, f.Token, http.MethodPatch, path, body).got; got != "409 todoSeriesChanged" {
			t.Errorf("%s is a change: %s", name, got)
		}
	}
	f.Series.AssigneeIDs = []string{f.Bruno.ID, f.Anke.ID}
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"assigneeIds": []string{f.Anke.ID, f.Bruno.ID}, "expectedUpdatedAt": stale}).got; got != "409 todoSeriesChanged" {
		t.Errorf("the same people in another order are a change: %s", got)
	}
	f.TodoList.Archived = true
	if got := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"leadDays": 2}).got; got != "422 todoListArchived" {
		t.Errorf("an identical write into an archived list: %s", got)
	}
}

// People compare as the series shows them: somebody stored and hidden from
// the list is no difference, stays where the people are left out or nothing
// changes, and goes with a write that names the people and changes anything.
func TestAPersonHiddenFromTheListStaysUntilAWriteNamesThePeople(t *testing.T) {
	s, f := New(t)
	path := "/todo-series/" + f.Series.ID
	f.Series.AssigneeIDs = []string{f.Bruno.ID, f.Client.ID}
	f.TodoList.HiddenFromClients = true

	callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"assigneeIds": []string{f.Bruno.ID}})
	callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"leadDays": 5})
	if len(f.Series.AssigneeIDs) != 2 {
		t.Fatalf("kept by an identical naming and by a write without people: %v", f.Series.AssigneeIDs)
	}
	callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"assigneeIds": []string{f.Bruno.ID}, "leadDays": 6})
	if len(f.Series.AssigneeIDs) != 1 || f.Series.AssigneeIDs[0] != f.Bruno.ID {
		t.Fatalf("taken off by a write that names the people and changes: %v", f.Series.AssigneeIDs)
	}
}
