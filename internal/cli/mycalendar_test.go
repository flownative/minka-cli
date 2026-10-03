package cli

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// withThingsDue gives the project a to-do and a card of Anke's due in the
// week of 19 October 2026, and a step of hers on a card of nobody's.
func withThingsDue(f *fakeapi.Fixture) {
	day := func(value string) *string { return &value }
	f.LooseTodo.DueOn, f.LooseTodo.AssigneeIDs = day("2026-10-20"), []string{f.Anke.ID}
	f.CardInDoing.DueOn, f.CardInDoing.AssigneeIDs = day("2026-10-21"), []string{f.Anke.ID}
	f.CardInTriage.Steps = []*fakeapi.Step{{ID: openStepID, Title: "Get an offer", AssigneeIDs: []string{f.Anke.ID}, DueOn: day("2026-10-22")}}
}

func asAnke(e *testEnv) {
	e.server.Tokens["minka_pat_anke"] = e.fixture.Anke.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_anke"
}

func TestASpanShowsWhatIsDueApartFromItsEvents(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withThingsDue(f)

	env := runOK(t, e, "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25")
	span := decode(t, env.Data)
	if span["content"] != "eventsAndDatedItems" || !slices.Equal(idsOf(t, span, "datedItems"), []string{f.LooseTodo.ID, f.CardInDoing.ID}) {
		t.Fatalf("data is not the span as Minka answered it: %v", span)
	}
	if !strings.Contains(env.Summary, "3 events and 2 items due") {
		t.Errorf("summary %q", env.Summary)
	}

	text := e.run("", "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25", "--styled")
	expectCode(t, text, ExitOK)
	events, due, found := strings.Cut(text.stdout, "Due on these days (not events):")
	if !found {
		t.Fatalf("what is due has no heading of its own:\n%s", text.stdout)
	}
	if strings.Contains(events, "Count the tents") || !strings.Contains(events, "Acceptance") {
		t.Errorf("a dated item stands among the events:\n%s", text.stdout)
	}
	for _, want := range []string{"The calendar shows events and the to-dos and cards due.", "2026-10-20  to-do  Count the tents", "in list Setting up", "2026-10-21  card", "on board API-3 Abnahme", f.CardInDoing.ID} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
	if strings.Contains(due, "Acceptance") {
		t.Errorf("an event stands among what is due:\n%s", text.stdout)
	}

	f.Calendar.EventsOnly = true
	env = runOK(t, e, "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25")
	if !strings.Contains(env.Summary, "events only") || strings.Contains(env.Summary, "due") {
		t.Errorf("summary %q", env.Summary)
	}
	text = e.run("", "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25", "--styled")
	if !strings.Contains(text.stdout, "The calendar shows events only.") || strings.Contains(text.stdout, "Due on these days") {
		t.Errorf("a calendar of events only:\n%s", text.stdout)
	}
}

func TestMyCalendarShowsMyEventsAndWhatIsMineThatIsDue(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	withThingsDue(f)
	asAnke(e)

	env := runOK(t, e, "events", "mine", "--from", "2026-10-19", "--to", "2026-10-25")
	requests := e.server.Requests()
	if last := requests[len(requests)-1]; last.Path != "/api/v1/me/events" || e.lastRequestQuery() != "from=2026-10-19&to=2026-10-25" {
		t.Errorf("request %s?%s", last.Path, last.Query)
	}
	mine := decode(t, env.Data)
	if !slices.Equal(idsOf(t, mine, "events"), []string{f.WeeklyEvent.ID}) {
		t.Errorf("events %v, want those Anke takes part in", idsOf(t, mine, "events"))
	}
	if ids := idsOf(t, mine, "datedItems"); len(ids) != 3 || ids[2] != f.CardInTriage.Steps[0].ID {
		t.Errorf("dated items %v", ids)
	}
	if !strings.Contains(env.Summary, "1 event and 3 items due on your calendar") {
		t.Errorf("summary %q", env.Summary)
	}

	text := e.run("", "events", "mine", "--from", "2026-10-19", "--to", "2026-10-25", "--styled")
	expectCode(t, text, ExitOK)
	events, due, found := strings.Cut(text.stdout, "Due on these days (not events):")
	if !found || !strings.Contains(events, "Weekly planning (repeats") || !strings.Contains(events, "[Test-Projekt]") || strings.Contains(events, "Get an offer") {
		t.Fatalf("the events and what is due are not apart:\n%s", text.stdout)
	}
	for _, want := range []string{"2026-10-22  step   Get an offer", "on card Write the CLI, Test-Projekt", "in list Setting up, Test-Projekt"} {
		if !strings.Contains(due, want) {
			t.Errorf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
}

func TestMyCalendarNeedsBothDaysAndSaysWhatIsWrongWithThem(t *testing.T) {
	e := newTestEnv(t)
	before := len(e.server.Requests())
	for _, args := range [][]string{{"events", "mine"}, {"events", "mine", "--from", "2026-10-19"}, {"events", "mine", "--to", "2026-10-19"}} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if sent := len(e.server.Requests()) - before; sent != 0 {
		t.Fatalf("%d requests went out", sent)
	}
	for flags, want := range map[string]string{
		"--from=2026-10-19 --to=tomorrow":   apiCodeEventsSpanNotADay,
		"--from=2026-10-19 --to=2026-10-18": apiCodeEventsSpanOutOfOrder,
		"--from=2026-01-01 --to=2027-01-02": apiCodeEventsSpanTooLong,
	} {
		env, r := e.runJSON("", append([]string{"events", "mine"}, strings.Fields(flags)...)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want || env.Hint == "" {
			t.Errorf("%s: apiCode %v, hint %q", flags, env.Meta["apiCode"], env.Hint)
		}
	}
}

// A Minka without the address answers with a page of its web; there is no
// calendar of one's own that could be missing, so the host is at fault.
func TestMyCalendarOfAnOlderMinkaSaysSo(t *testing.T) {
	e := newTestEnv(t)
	e.vars["MINKA_HOST"] = olderMinka(t)

	env, r := e.runJSON("", "events", "mine", "--from", "2026-10-19", "--to", "2026-10-25")
	expectCode(t, r, ExitNotFound)
	if env.Hint != myCalendarUnknownHint || !strings.Contains(env.Error, "has no My calendar") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}

	env, r = e.runJSON("", "events", "settings", e.fixture.Calendar.ID, "--content", "eventsOnly")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "cannot choose what a calendar shows") || !strings.Contains(env.Hint, "older") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}

// A span that names no content says nothing of what is due, so the text view
// claims nothing about it either.
func TestASpanWithoutContentSaysNothingOfWhatIsDue(t *testing.T) {
	e := newTestEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"from":"2026-10-19","to":"2026-10-25","timeZone":"Europe/Berlin","events":[]}`))
	}))
	t.Cleanup(server.Close)
	e.vars["MINKA_HOST"] = server.URL

	r := e.run("", "events", "list", e.fixture.Calendar.ID, "--from", "2026-10-19", "--to", "2026-10-25", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Due on these days") || strings.Contains(r.stdout, "The calendar shows") {
		t.Fatalf("the text speaks of what the span never said:\n%s", r.stdout)
	}
}

// Minka's own 404 on My calendar is no sign of an older Minka.
func TestMyCalendarCallsOnlyAPageOfTheWebAnOlderMinka(t *testing.T) {
	e := newTestEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(fakeapi.NotFoundBody))
	}))
	t.Cleanup(server.Close)
	e.vars["MINKA_HOST"] = server.URL

	env, r := e.runJSON("", "events", "mine", "--from", "2026-10-19", "--to", "2026-10-25")
	expectCode(t, r, ExitNotFound)
	if env.Hint == myCalendarUnknownHint || strings.Contains(env.Error, "has no My calendar") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}

func olderMinka(t *testing.T) string {
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Not found</body></html>"))
	}))
	t.Cleanup(older.Close)
	return older.URL
}

func TestTheTeamChoosesWhatACalendarShows(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "events", "settings", f.Calendar.ID, "--content", "eventsOnly")
	requests := e.server.Requests()
	last := requests[len(requests)-1]
	if last.Method != http.MethodPatch || last.Path != "/api/v1/calendars/"+f.Calendar.ID || string(last.Body) != `{"content":"eventsOnly"}` {
		t.Errorf("request %s %s %s", last.Method, last.Path, last.Body)
	}
	if calendar := decode(t, env.Data); calendar["content"] != "eventsOnly" || !f.Calendar.EventsOnly {
		t.Errorf("data %v", calendar)
	}
	if env.Summary != "Calendar Termine shows events only" {
		t.Errorf("summary %q", env.Summary)
	}

	before := len(e.server.Requests())
	_, r := e.runJSON("", "events", "settings", f.Calendar.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Error("a choice of nothing went out")
	}

	env, r = e.runJSON("", "events", "settings", f.Calendar.ID, "--content", "events-only")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeCalendarContentUnknown || !strings.Contains(env.Hint, "eventsAndDatedItems or eventsOnly") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	env, r = e.runJSON("", "events", "settings", f.OtherCalendar.ID, "--content", "eventsOnly")
	expectCode(t, r, ExitNotFound)
	if env.Hint != calendarSettingsNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestAClientIsToldThatOnlyTheTeamChooses(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Calendar.ShownToClients = true
	e.server.Tokens["minka_pat_client"] = f.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"

	env, r := e.runJSON("", "events", "settings", f.Calendar.ID, "--content", "eventsOnly")
	expectCode(t, r, ExitNotFound)
	if env.Hint != calendarSettingsNotFoundHint || !strings.Contains(env.Hint, "only its team chooses") {
		t.Errorf("hint %q", env.Hint)
	}
	if f.Calendar.EventsOnly {
		t.Error("a client chose")
	}
}
