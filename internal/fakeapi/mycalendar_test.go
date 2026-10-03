package fakeapi

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func titlesOf(t *testing.T, list any) []string {
	t.Helper()
	entries, ok := list.([]any)
	if !ok {
		t.Fatalf("no list: %v", list)
	}
	titles := []string{}
	for _, entry := range entries {
		titles = append(titles, entry.(map[string]any)["title"].(string))
	}
	return titles
}

// withDatedItems gives the project a dated to-do of Anke's, a dated card of
// Anke's in Doing and a dated step of Anke's on a card of nobody's, all in
// April 2035, and things due then that no span shows.
func withDatedItems(f *Fixture) {
	day := func(value string) *string { return &value }
	f.LooseTodo.DueOn, f.LooseTodo.AssigneeIDs = day("2035-04-14"), []string{f.Anke.ID}
	f.DoneTodo.DueOn = day("2035-04-14")
	f.ArchivedListTodo.DueOn = day("2035-04-14")
	f.CardInDoing.DueOn, f.CardInDoing.AssigneeIDs = day("2035-04-16"), []string{f.Anke.ID}
	f.CardInTriage.Steps = []*Step{
		{ID: newID(), Title: "Get an offer", AssigneeIDs: []string{f.Anke.ID}, DueOn: day("2035-04-18")},
		{ID: newID(), Title: "Already got", Completed: true, AssigneeIDs: []string{f.Anke.ID}, DueOn: day("2035-04-18")},
	}
	f.Done.Cards = append(f.Done.Cards, &Card{ID: newID(), Title: "Done already", DueOn: day("2035-04-15"), AssigneeIDs: []string{f.Anke.ID}})
	// Completed and still in Doing: a calendar's span asks for the moment it
	// was completed, My calendar only for the column.
	completedAt := time.Date(2035, 4, 1, 9, 0, 0, 0, time.UTC)
	f.Doing.Cards = append(f.Doing.Cards, &Card{ID: newID(), Title: "Completed in Doing", DueOn: day("2035-04-17"), AssigneeIDs: []string{f.Anke.ID}, CompletedAt: &completedAt})
}

func asAnke(s *Server, f *Fixture) string {
	s.Tokens["minka_pat_anke"] = f.Anke.ID
	return "minka_pat_anke"
}

const april = "?from=2035-04-01&to=2035-04-30"

func TestACalendarShowsTheDatedItemsOfItsProjectUntilTheTeamChoosesEventsOnly(t *testing.T) {
	s, f := New(t)
	withDatedItems(f)
	calendar := "/calendars/" + f.Calendar.ID

	if answer := callSeries(t, s, f.Token, http.MethodGet, calendar, nil); answer.body["content"] != "eventsAndDatedItems" {
		t.Fatalf("a calendar nobody chose for: %s", answer.raw)
	}
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+april, nil)
	if span.body["content"] != "eventsAndDatedItems" {
		t.Errorf("the span says what the calendar shows: %s", span.raw)
	}
	items := span.body["datedItems"].([]any)
	if titles := titlesOf(t, items); !reflect.DeepEqual(titles, []string{"Count the tents", "Review the API"}) {
		t.Fatalf("dated items %v, want the open to-do and card of the project, whoever's, and no step", titles)
	}
	expectPicked(t, "the to-do", items[0].(map[string]any), map[string]any{
		"id": f.LooseTodo.ID, "kind": "todo", "projectId": f.Project.ID, "toolId": f.Todos.ID, "cardId": nil,
		"dueOn": "2035-04-14", "placeTitle": "Setting up", "appUrl": s.todoURL(f.Todos, f.LooseTodo),
	})
	expectPicked(t, "the card", items[1].(map[string]any), map[string]any{
		"kind": "card", "toolId": f.Board.ID, "placeTitle": "API-3 Abnahme", "appUrl": s.cardURL(f.Board, f.CardInDoing),
	})
	if _, named := items[0].(map[string]any)["project"]; named {
		t.Errorf("a dated item of a calendar names no project: %v", items[0])
	}

	chosen := callSeries(t, s, f.Token, http.MethodPatch, calendar, map[string]any{"content": "eventsOnly"})
	if chosen.got != "200" || chosen.body["content"] != "eventsOnly" || chosen.body["projectId"] != f.Project.ID {
		t.Fatalf("%s %s", chosen.got, chosen.raw)
	}
	span = callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+april, nil)
	if span.body["content"] != "eventsOnly" || len(span.body["datedItems"].([]any)) != 0 {
		t.Errorf("events only: %s", span.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, calendar, map[string]any{"content": "eventsOnly"}); answer.got != "200" {
		t.Errorf("the same again: %s", answer.got)
	}
	if answer := callSeries(t, s, asAnke(s, f), http.MethodGet, "/me/events"+april, nil); len(answer.body["datedItems"].([]any)) != 4 {
		t.Errorf("what a calendar shows does not touch My calendar: %s", answer.raw)
	}
	callSeries(t, s, f.Token, http.MethodPatch, calendar, map[string]any{"content": "eventsAndDatedItems"})
	if span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+april, nil); len(span.body["datedItems"].([]any)) != 2 {
		t.Errorf("back to both: %s", span.raw)
	}

	f.Project.Archived = true
	if span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+april, nil); len(span.body["datedItems"].([]any)) != 2 {
		t.Errorf("a span of an archived project still carries its dated items: %s", span.raw)
	}
}

func TestAChoiceOfWhatACalendarShowsIsRefusedInTheOrderOfTheServer(t *testing.T) {
	s, f := New(t)
	calendar := "/calendars/" + f.Calendar.ID
	if answer := callSeries(t, s, f.StrangerToken, http.MethodPatch, calendar, `[]`); answer.raw != NotFoundBody {
		t.Errorf("a stranger with a body that is wrong: %s %s", answer.got, answer.raw)
	}
	for body, want := range map[string]string{
		`{}`:                        "422 nothingToChange",
		`{"content": null}`:         "422 nothingToChange",
		`{"content": "everything"}`: "422 calendarContentUnknown",
		`{"content": ""}`:           "422 calendarContentUnknown",
		`{"content": "EventsOnly"}`: "422 calendarContentUnknown",
		`{"content": true}`:         "400 malformedBody",
		`[]`:                        "400 malformedBody",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPatch, calendar, body); answer.got != want {
			t.Errorf("%s: %s, want %s", body, answer.got, want)
		}
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/calendars/"+f.CalendarOutOfUse.ID, map[string]any{"content": "eventsOnly"}); answer.got != "200" {
		t.Errorf("a calendar no longer in use takes the choice: %s", answer.got)
	}

	s.Tokens["minka_pat_client"] = f.Client.ID
	const client = "minka_pat_client"
	if answer := callSeries(t, s, client, http.MethodPatch, calendar, map[string]any{"content": "eventsOnly"}); answer.raw != NotFoundBody {
		t.Errorf("a client the calendar is hidden from: %s", answer.raw)
	}
	f.Calendar.ShownToClients = true
	if answer := callSeries(t, s, client, http.MethodGet, calendar, nil); answer.body["content"] != "eventsAndDatedItems" {
		t.Errorf("a client reads the choice: %s", answer.raw)
	}
	if answer := callSeries(t, s, client, http.MethodPatch, calendar, map[string]any{"content": "everything"}); answer.got != "422 calendarContentUnknown" {
		t.Errorf("a client the calendar is shown to hears what is wrong with a body: %s", answer.got)
	}
	if answer := callSeries(t, s, client, http.MethodPatch, calendar, map[string]any{"content": "eventsOnly"}); answer.raw != NotFoundBody || f.Calendar.EventsOnly {
		t.Errorf("a client chooses nothing: %s", answer.raw)
	}

	f.Project.Archived = true
	for token, want := range map[string]string{f.Token: "422 projectArchived", client: "422 projectArchived", f.StrangerToken: "404"} {
		if answer := callSeries(t, s, token, http.MethodPatch, calendar, map[string]any{"content": "eventsOnly"}); answer.got != want {
			t.Errorf("an archived project, %s: %s, want %s", token, answer.got, want)
		}
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, calendar, map[string]any{"content": "everything"}); answer.got != "422 calendarContentUnknown" {
		t.Errorf("the value before the archive: %s", answer.got)
	}
	if f.Calendar.EventsOnly {
		t.Error("an archived project took the choice")
	}
}

func TestMyCalendarHoldsTheEventsTakenPartInAndWhatIsMineThatIsDue(t *testing.T) {
	s, f := New(t)
	withDatedItems(f)
	anke := asAnke(s, f)
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Anke.ID)
	f.OtherEvent.ParticipantIDs = []string{f.Anke.ID}
	f.AllDayEvent.ParticipantIDs = []string{f.Bruno.ID}
	f.GroupedTodo.Subtasks[0].DueOn = nil

	mine := callSeries(t, s, anke, http.MethodGet, "/me/events?from=2026-10-19&to=2026-10-25", nil)
	if mine.got != "200" {
		t.Fatalf("%s %s", mine.got, mine.raw)
	}
	if _, carries := mine.body["content"]; carries {
		t.Errorf("My calendar has no content of its own: %s", mine.raw)
	}
	events := mine.body["events"].([]any)
	if ids := idsOf(t, events); !reflect.DeepEqual(ids, []string{f.WeeklyEvent.ID, f.OtherEvent.ID}) {
		t.Fatalf("events %v, want those Anke takes part in across her projects, by their first occurrence", ids)
	}
	expectPicked(t, "the line", events[1].(map[string]any), map[string]any{
		"toolId": f.OtherCalendar.ID, "projectId": f.OtherProject.ID, "project": map[string]any{"id": f.OtherProject.ID, "name": "Secret"},
	})

	items := callSeries(t, s, anke, http.MethodGet, "/me/events"+april, nil).body["datedItems"].([]any)
	if titles := titlesOf(t, items); !reflect.DeepEqual(titles, []string{"Count the tents", "Review the API", "Completed in Doing", "Get an offer"}) {
		t.Fatalf("dated items %v", titles)
	}
	expectPicked(t, "the step", items[3].(map[string]any), map[string]any{
		"kind": "step", "toolId": f.Board.ID, "cardId": f.CardInTriage.ID, "placeTitle": "Write the CLI",
		"appUrl": s.cardURL(f.Board, f.CardInTriage), "project": map[string]any{"id": f.Project.ID, "name": "Test-Projekt"},
	})
	if titles := titlesOf(t, callSeries(t, s, f.Token, http.MethodGet, "/me/events"+april, nil).body["datedItems"]); len(titles) != 0 {
		t.Errorf("Bruno has nothing of his own due, and a calendar's items are not his: %v", titles)
	}

	// A to-do of which only a subtask is hers stays out, and so does the subtask.
	due := "2035-04-20"
	f.GroupedTodo.DueOn, f.GroupedTodo.Subtasks[0].DueOn = &due, &due
	if titles := titlesOf(t, callSeries(t, s, anke, http.MethodGet, "/me/events"+april, nil).body["datedItems"]); len(titles) != 4 {
		t.Errorf("a subtask counted: %v", titles)
	}

	f.Project.Archived = true
	archived := callSeries(t, s, anke, http.MethodGet, "/me/events?from=2026-10-19&to=2026-10-25", nil)
	if ids := idsOf(t, archived.body["events"]); !reflect.DeepEqual(ids, []string{f.OtherEvent.ID}) {
		t.Errorf("an archived project stays out of the events: %v", ids)
	}
	if titles := titlesOf(t, callSeries(t, s, anke, http.MethodGet, "/me/events"+april, nil).body["datedItems"]); len(titles) != 0 {
		t.Errorf("an archived project stays out of the dated items: %v", titles)
	}
}

// Minka reads the events in the order they were added, then orders them by
// their first occurrence; where two start together, the earlier one comes first
// whatever calendar it stands in.
func TestMyCalendarOrdersEventsThatStartTogetherByWhenTheyWereAdded(t *testing.T) {
	s, f := New(t)
	anke := asAnke(s, f)
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Anke.ID)
	f.OtherEvent.ParticipantIDs = []string{f.Anke.ID}
	f.AllDayEvent.ParticipantIDs, f.AllDayEvent.FirstDay = []string{f.Anke.ID}, "2026-10-20"
	f.AllDayEvent.CreatedAt = f.OtherEvent.CreatedAt.Add(time.Second)
	if ids := idsOf(t, callSeries(t, s, anke, http.MethodGet, "/me/events?from=2026-10-20&to=2026-10-20", nil).body["events"]); !reflect.DeepEqual(ids, []string{f.OtherEvent.ID, f.AllDayEvent.ID}) {
		t.Errorf("events %v, want the one added first first", ids)
	}
}

func TestASpanOfMyCalendarIsRefusedAsThatOfACalendar(t *testing.T) {
	s, f := New(t)
	for query, want := range map[string]string{
		"":                                  "422 eventsSpanNotADay",
		"?from=2035-04-01":                  "422 eventsSpanNotADay",
		"?from=2035-04-01&to=2035-02-30":    "422 eventsSpanNotADay",
		"?from=2035-04-02&to=2035-04-01":    "422 eventsSpanOutOfOrder",
		"?from=2035-01-01&to=2036-01-02":    "422 eventsSpanTooLong",
		"?from=2035-01-01&to=2036-01-01":    "200",
		"?from=%202035-04-01&to=2035-04-01": "200",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodGet, "/me/events"+query, nil); answer.got != want {
			t.Errorf("%q: %s, want %s", query, answer.got, want)
		}
	}
}

func TestAClientGetsOnlyTheDatedItemsShownToThem(t *testing.T) {
	s, f := New(t)
	withDatedItems(f)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.Calendar.ShownToClients = true
	f.TodoList.HiddenFromClients = true
	span := callSeries(t, s, "minka_pat_client", http.MethodGet, eventsOf(f)+april, nil)
	if titles := titlesOf(t, span.body["datedItems"]); !reflect.DeepEqual(titles, []string{"Review the API"}) {
		t.Errorf("a to-do of a list hidden from clients: %v", titles)
	}
	f.LooseTodo.AssigneeIDs = []string{f.Client.ID}
	if titles := titlesOf(t, callSeries(t, s, "minka_pat_client", http.MethodGet, "/me/events"+april, nil).body["datedItems"]); len(titles) != 0 {
		t.Errorf("My calendar of a client: %v", titles)
	}
}
