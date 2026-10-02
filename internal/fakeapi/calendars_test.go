package fakeapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func eventsOf(f *Fixture) string {
	return "/calendars/" + f.Calendar.ID + "/events"
}

func idsOf(t *testing.T, list any) []string {
	t.Helper()
	entries, ok := list.([]any)
	if !ok {
		t.Fatalf("no list: %v", list)
	}
	ids := []string{}
	for _, entry := range entries {
		ids = append(ids, entry.(map[string]any)["id"].(string))
	}
	return ids
}

func TestTheCalendarsOfAProjectAreThoseInUse(t *testing.T) {
	s, f := New(t)
	answer := callSeries(t, s, f.Token, http.MethodGet, "/projects/"+f.Project.ID+"/calendars", nil)
	var list []any
	_ = json.Unmarshal([]byte(answer.raw), &list)
	if ids := idsOf(t, list); !reflect.DeepEqual(ids, []string{f.Calendar.ID}) {
		t.Errorf("calendars %v, want only the one in use", ids)
	}
	for _, id := range []string{f.Calendar.ID, f.CalendarOutOfUse.ID} {
		if answer := callSeries(t, s, f.Token, http.MethodGet, "/calendars/"+id, nil); answer.got != "200" || answer.body["projectId"] != f.Project.ID {
			t.Errorf("calendar %s: %s %s", id, answer.got, answer.raw)
		}
	}
	if answer := callSeries(t, s, f.Token, http.MethodGet, "/calendars/"+f.OtherCalendar.ID, nil); answer.raw != NotFoundBody {
		t.Errorf("a calendar of somebody else's project: %s %s", answer.got, answer.raw)
	}
}

// Summer time ends in Berlin between the first and the second Monday of the
// weekly series: 10:00 there stays 10:00, so the moment in UTC moves.
func TestASpanListsEveryEventOnceAndASeriesWithItsOccurrences(t *testing.T) {
	s, f := New(t)
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-10-19&to=2026-11-02", nil)
	if span.got != "200" {
		t.Fatalf("%s %s", span.got, span.raw)
	}
	expectPicked(t, "span", span.body, map[string]any{"from": "2026-10-19", "to": "2026-11-02", "timeZone": "Europe/Berlin"})
	events := span.body["events"].([]any)
	if ids := idsOf(t, events); !reflect.DeepEqual(ids, []string{f.WeeklyEvent.ID, f.AllDayEvent.ID, f.OnceEvent.ID}) {
		t.Fatalf("events %v, want by their first occurrence", ids)
	}
	series := events[0].(map[string]any)
	occurrence := func(startsAt, endsAt string) any {
		return map[string]any{"startsAt": startsAt, "endsAt": endsAt, "firstDay": nil, "lastDay": nil}
	}
	want := []any{
		occurrence("2026-10-19T08:00:00.000000Z", "2026-10-19T08:30:00.000000Z"),
		occurrence("2026-10-26T09:00:00.000000Z", "2026-10-26T09:30:00.000000Z"),
		occurrence("2026-11-02T09:00:00.000000Z", "2026-11-02T09:30:00.000000Z"),
	}
	if !reflect.DeepEqual(series["occurrences"], want) {
		t.Errorf("occurrences %v, want %v", series["occurrences"], want)
	}
	if series["startsAt"] != "2026-10-19T08:00:00.000000Z" || series["timeZone"] != "Europe/Berlin" || series["description"] != nil {
		t.Errorf("the line of the series: %v", series)
	}
	allDay := events[1].(map[string]any)["occurrences"]
	if !reflect.DeepEqual(allDay, []any{map[string]any{"startsAt": nil, "endsAt": nil, "firstDay": "2026-10-20", "lastDay": "2026-10-22"}}) {
		t.Errorf("the all-day event: %v", allDay)
	}

	later := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2027-03-01&to=2027-03-01", nil)
	if ids := idsOf(t, later.body["events"]); !reflect.DeepEqual(ids, []string{f.WeeklyEvent.ID}) {
		t.Errorf("a series is found from its occurrence, however long ago it began: %v", ids)
	}
	if none := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2027-03-02&to=2027-03-02", nil); len(none.body["events"].([]any)) != 0 {
		t.Errorf("a day without an occurrence: %s", none.raw)
	}
}

// The days of a span are the reader's: an event just after midnight in
// Berlin is on the day before in UTC.
func TestTheDaysOfASpanAreTheReadersOwn(t *testing.T) {
	s, f := New(t)
	late := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{"title": "Late", "startsAt": "2026-11-11T00:30:00+01:00", "endsAt": "2026-11-11T00:45:00+01:00"})
	if late.got != "201" {
		t.Fatalf("%s %s", late.got, late.raw)
	}
	if ids := idsOf(t, callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-11-11&to=2026-11-11", nil).body["events"]); len(ids) != 1 {
		t.Errorf("in Berlin it is the 11th: %v", ids)
	}
	f.Bruno.TimeZone = "UTC"
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-11-10&to=2026-11-10", nil)
	if ids := idsOf(t, span.body["events"]); len(ids) != 1 || span.body["timeZone"] != "UTC" {
		t.Errorf("in UTC it is the 10th: %s", span.raw)
	}
	s.TimeZone, f.Bruno.TimeZone = "CET", ""
	if span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-11-10&to=2026-11-10", nil); span.body["timeZone"] != "CET" {
		t.Errorf("an installation that keeps CET reads its days in it: %s", span.raw)
	}
}

func TestAnEventIsAddedReadChangedAndPutIntoTheTrash(t *testing.T) {
	s, f := New(t)
	created := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
		"title": "Planning", "startsAt": "2035-10-05T09:00:00+02:00", "endsAt": "2035-10-05T10:30:00.5+0200",
		"description": "<p>Come prepared.</p>", "joinUrl": "https://meet.example.com/x", "highlighted": true,
		"participantIds": []string{f.Anke.ID},
	})
	if created.got != "201" || created.location != s.URL+"/api/v1/events/"+created.body["id"].(string) {
		t.Fatalf("%s %s %s", created.got, created.location, created.raw)
	}
	expectPicked(t, "created", created.body, map[string]any{
		"toolId": f.Calendar.ID, "allDay": false, "startsAt": "2035-10-05T07:00:00.000000Z", "endsAt": "2035-10-05T08:30:00.500000Z",
		"firstDay": nil, "schedule": nil, "timeZone": nil, "joinUrl": "https://meet.example.com/x", "highlighted": true,
		"participants": []any{map[string]any{"id": f.Anke.ID, "name": "Anke"}}, "commentCount": float64(0),
	})
	id := created.body["id"].(string)

	changed := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+id, map[string]any{"title": "Planning 2036", "joinUrl": nil, "participantIds": []string{}})
	expectPicked(t, "changed", changed.body, map[string]any{"title": "Planning 2036", "joinUrl": nil, "participants": []any{}, "highlighted": true, "description": "<p>Come prepared.</p>"})
	allDay := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+id, map[string]any{"firstDay": "2035-10-03"})
	expectPicked(t, "all-day", allDay.body, map[string]any{"allDay": true, "startsAt": nil, "firstDay": "2035-10-03", "lastDay": "2035-10-03"})

	if gone := callSeries(t, s, f.Token, http.MethodDelete, "/events/"+id, nil); gone.got != "204" {
		t.Fatalf("%s %s", gone.got, gone.raw)
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if answer := callSeries(t, s, f.Token, method, "/events/"+id, nil); answer.raw != NotFoundBody {
			t.Errorf("%s of an event in the trash: %s %s", method, answer.got, answer.raw)
		}
	}
}

func TestAChangeSaysWhatBecomesOfTheRepetition(t *testing.T) {
	s, f := New(t)
	created := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
		"title": "Cleaning", "firstDay": "2035-10-01", "schedule": map[string]any{"rhythm": "weekly", "weekdays": []int{1, 4}, "endsOn": "2035-12-31"},
	})
	id := created.body["id"].(string)
	path := "/events/" + id

	if kept := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"title": "Kitchen"}); kept.body["timeZone"] != nil ||
		!reflect.DeepEqual(kept.body["schedule"].(map[string]any)["weekdays"], []any{float64(1), float64(4)}) {
		t.Errorf("a change without a schedule keeps it: %s", kept.raw)
	}
	for body, want := range map[string]string{
		`{"firstDay": "2035-10-02"}`:                                             "422 eventRepetitionKeepsItsFirstDay",
		`{"firstDay": "2035-10-02", "schedule": {}}`:                             "422 eventRepetitionDoesNotFit",
		`{"startsAt": "2035-10-01T08:00:00Z", "endsAt": "2035-10-01T09:00:00Z"}`: "422 eventRepetitionKeepsItsFirstDay",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPatch, path, body); answer.got != want {
			t.Errorf("%s: %s, want %s", body, answer.got, want)
		}
	}
	moved := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"firstDay": "2035-10-02", "schedule": map[string]any{"weekdays": []int{2}}})
	expectPicked(t, "moved", moved.body["schedule"].(map[string]any), map[string]any{"rhythm": "weekly", "weekdays": []any{float64(2)}, "startsOn": "2035-10-02", "endsOn": "2035-12-31"})

	stopped := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"schedule": nil})
	if stopped.body["schedule"] != nil || stopped.body["firstDay"] != "2035-10-02" {
		t.Errorf("stopped: %s", stopped.raw)
	}

	zoned := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{
		"startsAt": "2035-10-02T23:30:00Z", "endsAt": "2035-10-03T00:30:00Z",
		"schedule": map[string]any{"rhythm": "daily", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}}, "timeZone": "America/New_York",
	})
	if zoned.body["timeZone"] != "America/New_York" || zoned.body["schedule"].(map[string]any)["startsOn"] != "2035-10-02" {
		t.Errorf("the first day is the one of the zone the series keeps its time in: %s", zoned.raw)
	}
	if kept := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"schedule": map[string]any{"interval": 1}}); kept.body["timeZone"] != "America/New_York" {
		t.Errorf("a schedule without a zone keeps the one the series has: %s", kept.raw)
	}
}

func TestWhatMakesNoEventIsRefusedWithItsCase(t *testing.T) {
	s, f := New(t)
	moments := `"startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z"`
	for body, want := range map[string]string{
		`{` + moments + `}`: "422 eventTitleMissing",
		`{"title": "x"}`:    "422 eventTimeMissing",
		`{"title": "x", "endsAt": "2035-10-05T10:00:00Z"}`:                                          "422 eventTimeMissing",
		`{"title": "x", "lastDay": "2035-10-05"}`:                                                   "422 eventTimeMissing",
		`{"title": "x", "firstDay": "2035-10-05", ` + moments + `}`:                                 "422 eventTimeAmbiguous",
		`{"title": "x", "startsAt": "2035-10-05T09:00:00", "endsAt": "2035-10-05T10:00:00"}`:        "422 eventTimeNotReadable",
		`{"title": "x", "startsAt": "2035-10-05T09:00:00+15:00", "endsAt": "2035-10-05T10:00:00Z"}`: "422 eventTimeNotReadable",
		`{"title": "x", "startsAt": "0000-06-01T00:00:00Z", "endsAt": "2035-10-05T10:00:00Z"}`:      "422 eventTimeNotReadable",
		`{"title": "x", "startsAt": "0001-01-01T00:00:00+14:00", "endsAt": "2035-10-05T10:00:00Z"}`: "422 eventTimeNotReadable",
		`{"title": "x", "startsAt": "0000-06-01T00:00:00Z", "endsAt": "0000-05-01T00:00:00Z"}`:      "422 eventTimeNotReadable",
		`{"title": "x", "startsAt": "2035-02-30T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z"}`:      "422 eventTimeNotReadable",
		`{"title": "x", "firstDay": "2035-02-30"}`:                                                  "422 eventTimeNotReadable",
		`{"title": "x", "firstDay": "2035-10-05", "lastDay": "2035-10-04"}`:                         "422 eventEndsBeforeItStarts",
		`{"title": "x", "startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T08:00:00Z"}`:      "422 eventEndsBeforeItStarts",
		`{"title": "x", "joinUrl": "javascript:alert(1)", ` + moments + `}`:                         "422 eventJoinUrlNotAnAddress",
		`{"title": "x", "participantIds": ["nobody"], ` + moments + `}`:                             "422 personNotAProjectMember",
		`{"title": "x", "schedule": {"rhythm": "hourly"}, ` + moments + `}`:                         "422 eventRepetitionScheduleInvalid",
		`{"title": "x", "schedule": {"endsOn": "tomorrow"}, ` + moments + `}`:                       "422 eventRepetitionDatesNotADate",
		`{"title": "x", "schedule": {"endsOn": "2035-10-04"}, ` + moments + `}`:                     "422 eventRepetitionDatesOutOfOrder",
		`{"title": "x", "schedule": {"rhythm": "weekly", "weekdays": [2]}, ` + moments + `}`:        "422 eventRepetitionDoesNotFit",
		`{"title": "x", "schedule": {"rhythm": "daily"}, "timeZone": "+02:00", ` + moments + `}`:    "422 eventRepetitionTimeZoneUnknown",
		`{"title": "x", "schedule": {"rhythm": "daily"}, "timeZone": "CET", ` + moments + `}`:       "422 eventRepetitionTimeZoneUnknown",
		`{"title": "x", "schedule": "weekly", ` + moments + `}`:                                     "400 malformedBody",
		`{"title": "x", "participantIds": "nobody", ` + moments + `}`:                               "400 malformedBody",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), body); answer.got != want {
			t.Errorf("%s: %s, want %s", body, answer.got, want)
		}
	}
	if first := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "x", "startsAt": "0001-01-01T00:00:00Z", "endsAt": "0001-01-01T01:00:00Z"}`); first.got != "201" {
		t.Errorf("the year 1: %s %s", first.got, first.raw)
	}

	path := "/events/" + f.OnceEvent.ID
	for body, want := range map[string]string{
		`{}`:                                     "422 nothingToChange",
		`{"timeZone": "Europe/Berlin"}`:          "422 nothingToChange",
		`{"endsAt": "2035-10-01T18:00:00Z"}`:     "422 eventTimeMissing",
		`{"startsAt": null}`:                     "422 eventTimeMissing",
		`{"title": "Same", "title2": 1}`:         "200",
		`{"title": "Same", "highlighted": "no"}`: "400 malformedBody",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPatch, path, body); answer.got != want {
			t.Errorf("PATCH %s: %s, want %s", body, answer.got, want)
		}
	}

	for query, want := range map[string]string{
		"?from=2035-10-01":                  "422 eventsSpanNotADay",
		"?from=2035-10-02&to=2035-10-01":    "422 eventsSpanOutOfOrder",
		"?from=2035-01-01&to=2036-01-02":    "422 eventsSpanTooLong",
		"?from=2035-01-01&to=2036-01-01":    "200",
		"?from=%202035-01-01&to=2035-01-01": "200",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+query, nil); answer.got != want {
			t.Errorf("%s: %s, want %s", query, answer.got, want)
		}
	}
}

// The address is looked up before the body is read, the type of every field
// is checked before any value, and the values in the order the controller
// reads them; the archived project comes after them and before the people.
func TestTheRefusalsOfAnEventComeInTheOrderOfTheServer(t *testing.T) {
	s, f := New(t)
	if answer := callSeries(t, s, f.StrangerToken, http.MethodPost, eventsOf(f), `{"title": ""}`); answer.raw != NotFoundBody {
		t.Errorf("a stranger with a body that is wrong: %s %s", answer.got, answer.raw)
	}
	for body, want := range map[string]string{
		`{"title": "", "schedule": {"interval": "2"}}`:                                                                                                   "400 malformedBody",
		`{"title": "", "startsAt": "x"}`:                                                                                                                 "422 eventTitleMissing",
		`{"title": "x", "startsAt": "x", "joinUrl": "ftp://x"}`:                                                                                          "422 eventTimeMissing",
		`{"title": "x", "firstDay": "2035-10-05", "joinUrl": "ftp://x", "participantIds": ["nobody"]}`:                                                   "422 eventJoinUrlNotAnAddress",
		`{"title": "x", "firstDay": "2035-10-05", "participantIds": ["nobody"], "schedule": {"day": 40}}`:                                                "422 personNotAProjectMember",
		`{"title": "x", "startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z", "schedule": {"day": 40}, "timeZone": "Mars"}`:              "422 eventRepetitionScheduleInvalid",
		`{"title": "x", "startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z", "schedule": {"endsOn": "2035-01-01"}, "timeZone": "Mars"}`: "422 eventRepetitionTimeZoneUnknown",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), body); answer.got != want {
			t.Errorf("%s: %s, want %s", body, answer.got, want)
		}
	}
	tooLong := `{"title": "x", "firstDay": "2035-10-05", "schedule": {"rhythm": "weekly", "weekdays": [2]}, "description": "` + strings.Repeat("a", maxEventDescriptionLength+1) + `"}`
	if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), tooLong); answer.got != "422 eventRepetitionDoesNotFit" {
		t.Errorf("the schedule before the description: %s", answer.got)
	}

	f.Project.Archived = true
	if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "x", "firstDay": "2035-10-05", "participantIds": ["`+f.Stranger.ID+`"]}`); answer.got != "422 projectArchived" {
		t.Errorf("an archived project before the people: %s", answer.got)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.OnceEvent.ID, `{"title": "Acceptance"}`); answer.got != "422 projectArchived" {
		t.Errorf("an archived project, even for a change that changes nothing: %s", answer.got)
	}
	if answer := callSeries(t, s, f.Token, http.MethodGet, "/events/"+f.OnceEvent.ID, nil); answer.got != "200" {
		t.Errorf("reading in an archived project: %s", answer.got)
	}
	if answer := callSeries(t, s, f.StrangerToken, http.MethodDelete, "/events/"+f.OnceEvent.ID, nil); answer.raw != NotFoundBody {
		t.Errorf("only a member hears why: %s", answer.raw)
	}
}

func TestAClientReadsAndWritesOnlyWhatIsShownToClients(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	const client = "minka_pat_client"
	span := eventsOf(f) + "?from=2026-10-19&to=2026-10-25"
	if answer := callSeries(t, s, client, http.MethodGet, span, nil); answer.raw != NotFoundBody {
		t.Errorf("a calendar hidden from clients: %s %s", answer.got, answer.raw)
	}
	f.Calendar.ShownToClients = true
	if ids := idsOf(t, callSeries(t, s, client, http.MethodGet, span, nil).body["events"]); len(ids) != 0 {
		t.Errorf("the team's events are hidden from clients: %v", ids)
	}
	if answer := callSeries(t, s, client, http.MethodGet, "/events/"+f.OnceEvent.ID, nil); answer.raw != NotFoundBody {
		t.Errorf("an event of the team: %s", answer.raw)
	}

	own := callSeries(t, s, client, http.MethodPost, eventsOf(f), map[string]any{"title": "Question", "firstDay": "2026-10-20"})
	if own.got != "201" {
		t.Fatalf("%s %s", own.got, own.raw)
	}
	if ids := idsOf(t, callSeries(t, s, client, http.MethodGet, span, nil).body["events"]); len(ids) != 1 {
		t.Errorf("what a client adds is shown to clients: %v", ids)
	}
	if answer := callSeries(t, s, f.Token, http.MethodDelete, "/events/"+own.body["id"].(string), nil); answer.got != "204" {
		t.Errorf("the team puts away a client's event: %s", answer.got)
	}
	shown := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{"title": "Another", "firstDay": "2026-10-20"})
	s.Calendars[0].Events[len(s.Calendars[0].Events)-1].ShownToClients = true
	if answer := callSeries(t, s, client, http.MethodDelete, "/events/"+shown.body["id"].(string), nil); answer.raw != NotFoundBody {
		t.Errorf("a client puts away only an event of their own: %s", answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.OnceEvent.ID, map[string]any{"participantIds": []string{f.Client.ID}}); answer.got != "422 personCannotSeeIt" {
		t.Errorf("a client on an event hidden from clients: %s", answer.got)
	}
}

// Those who take part keep their place, and the newcomers of one request
// follow. Minka promises no order among them; its read sorts people put on
// at the same moment by their identifier, and so does the fake. They are
// named here against that order, so that the order named cannot pass for it.
func TestParticipantsKeepTheirPlaceAndNewcomersFollow(t *testing.T) {
	s, f := New(t)
	later, earlier := f.Bruno.ID, f.Agent.ID
	if later < earlier {
		later, earlier = earlier, later
	}
	answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.WeeklyEvent.ID, map[string]any{"participantIds": []string{later, f.Anke.ID, earlier, later}})
	if got := idsOf(t, answer.body["participants"]); !reflect.DeepEqual(got, []string{f.Anke.ID, earlier, later}) {
		t.Errorf("participants %v, want Anke first, then the newcomers once each by their identifier", got)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.WeeklyEvent.ID, map[string]any{"participantIds": []string{f.Stranger.ID}}); answer.got != "422 personNotAProjectMember" {
		t.Errorf("a stranger: %s", answer.got)
	}
}

// Every value is held before the archived project is asked, so a body that
// is wrong says what is wrong with it there as well.
func TestTheValuesOfAnEventComeBeforeTheArchivedProject(t *testing.T) {
	s, f := New(t)
	f.Project.Archived = true
	long := strings.Repeat("a", maxEventDescriptionLength+1)
	for body, want := range map[string]string{
		`{"title": "x", "startsAt": "2035-10-05T09:00:00", "endsAt": "2035-10-05T10:00:00"}`: "422 eventTimeNotReadable",
		`{"title": "x", "firstDay": "2035-10-05", "description": "` + long + `"}`:            "422 eventDescriptionTooLong",
		`{"title": "x", "firstDay": "2035-10-05", "schedule": {"endsOn": "2035-10-01"}}`:     "422 eventRepetitionDatesOutOfOrder",
		`{"title": "x", "firstDay": "2035-10-05", "schedule": {"endsOn": "tomorrow"}}`:       "422 eventRepetitionDatesNotADate",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), body); answer.got != want {
			t.Errorf("POST %s: %s, want %s", body[:min(len(body), 80)], answer.got, want)
		}
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.OnceEvent.ID, `{"description": "`+long+`"}`); answer.got != "422 eventDescriptionTooLong" {
		t.Errorf("PATCH with a description too long: %s", answer.got)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/events/"+f.OnceEvent.ID, `{"firstDay": "2035-02-30"}`); answer.got != "422 eventTimeNotReadable" {
		t.Errorf("PATCH with a day that is none: %s", answer.got)
	}
}

// An hour that comes twice is taken the first time, one that is skipped is
// moved forward, as PHP's setTime() does: measured with PHP 8.5, 02:30 in
// Berlin on 25 October 2026 is 00:30 UTC, and 02:30 in New York on 8 March
// 2026 is 07:30 UTC.
func TestAClockTimeAChangeOfSummerTimeMeetsIsPlacedAsPHPPlacesIt(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	newYork, _ := time.LoadLocation("America/New_York")
	for name, c := range map[string]struct {
		day  string
		zone *time.Location
		want string
	}{
		"the hour that comes twice": {"2026-10-25", berlin, "2026-10-25T00:30:00Z"},
		"the hour that is skipped":  {"2026-03-08", newYork, "2026-03-08T07:30:00Z"},
		"an ordinary hour":          {"2026-10-26", berlin, "2026-10-26T01:30:00Z"},
	} {
		clock := time.Date(2026, 1, 1, 2, 30, 0, 0, c.zone)
		if got := atTheClockTimeOf(clock, c.day, c.zone).UTC().Format(time.RFC3339); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

func TestASeriesIsReadInTheDaysOfItsReader(t *testing.T) {
	s, f := New(t)
	late := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
		"title": "Night shift", "startsAt": "2026-11-02T00:30:00+01:00", "endsAt": "2026-11-02T00:45:00+01:00",
		"schedule": map[string]any{"rhythm": "daily", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}}, "timeZone": "Europe/Berlin",
	})
	if late.got != "201" {
		t.Fatalf("%s %s", late.got, late.raw)
	}
	f.Bruno.TimeZone = "UTC"
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-11-09&to=2026-11-09", nil)
	var night map[string]any
	for _, entry := range span.body["events"].([]any) {
		if entry.(map[string]any)["id"] == late.body["id"] {
			night = entry.(map[string]any)
		}
	}
	want := []any{map[string]any{"startsAt": "2026-11-09T23:30:00.000000Z", "endsAt": "2026-11-09T23:45:00.000000Z", "firstDay": nil, "lastDay": nil}}
	if night == nil || !reflect.DeepEqual(night["occurrences"], want) {
		t.Errorf("00:30 in Berlin on the 10th is the 9th in UTC: %s", span.raw)
	}
}

// An occurrence that started the evening before reaches into the day.
func TestAnOccurrenceOfTheEveningBeforeReachesIntoASpan(t *testing.T) {
	s, f := New(t)
	shift := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
		"title": "Late shift", "startsAt": "2026-11-02T23:00:00+01:00", "endsAt": "2026-11-03T01:00:00+01:00",
		"schedule": map[string]any{"rhythm": "daily", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}},
	})
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-11-10&to=2026-11-10", nil)
	for _, entry := range span.body["events"].([]any) {
		if entry.(map[string]any)["id"] == shift.body["id"] {
			if got := len(entry.(map[string]any)["occurrences"].([]any)); got != 2 {
				t.Errorf("%d occurrences, want the one of the evening before and the one of the day", got)
			}
			return
		}
	}
	t.Errorf("the series is not in the span: %s", span.raw)
}

// A skipped hour may move the start past the end; the end then stays with it.
func TestAnOccurrenceAcrossASkippedHourEndsNotBeforeItStarts(t *testing.T) {
	s, f := New(t)
	early := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
		"title": "Early", "startsAt": "2026-03-22T02:30:00+01:00", "endsAt": "2026-03-22T03:10:00+01:00",
		"schedule": map[string]any{"rhythm": "weekly"},
	})
	span := callSeries(t, s, f.Token, http.MethodGet, eventsOf(f)+"?from=2026-03-29&to=2026-03-29", nil)
	want := []any{map[string]any{"startsAt": "2026-03-29T01:30:00.000000Z", "endsAt": "2026-03-29T01:30:00.000000Z", "firstDay": nil, "lastDay": nil}}
	if ids := idsOf(t, span.body["events"]); len(ids) != 1 || ids[0] != early.body["id"] ||
		!reflect.DeepEqual(span.body["events"].([]any)[0].(map[string]any)["occurrences"], want) {
		t.Errorf("02:30 to 03:10 on the day 02:00 to 03:00 is skipped: %s", span.raw)
	}
}

func TestATitleOfAnEventIsRefusedInMinkasWords(t *testing.T) {
	s, f := New(t)
	answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "", "firstDay": "2035-10-05"}`)
	if message := answer.body["error"].(map[string]any)["message"]; message != "An event needs a title." {
		t.Errorf("message %q", message)
	}
	answer = callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "`+strings.Repeat("a", 201)+`", "firstDay": "2035-10-05"}`)
	if message := answer.body["error"].(map[string]any)["message"]; message != "The title of an event is too long." {
		t.Errorf("message %q", message)
	}
}

// PHP holds a zone to its list exactly, whatever the file system ignores.
func TestAZoneIsReadInItsOwnCapitals(t *testing.T) {
	s, f := New(t)
	for zone, want := range map[string]string{"Europe/berlin": "422 eventRepetitionTimeZoneUnknown", "europe/Berlin": "422 eventRepetitionTimeZoneUnknown", "Europe/Berlin": "201"} {
		answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
			"title": "x", "startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z", "schedule": map[string]any{}, "timeZone": zone,
		})
		if answer.got != want {
			t.Errorf("%s: %s, want %s", zone, answer.got, want)
		}
	}
}

// The type of every field is held in the order the controller reads them:
// the people, then the zone, then the time.
func TestTheTypesOfTheFieldsOfAnEventAreHeldInMinkasOrder(t *testing.T) {
	s, f := New(t)
	for body, field := range map[string]string{
		`{"title": "x", "participantIds": "x", "timeZone": 5}`: "participantIds",
		`{"title": "x", "startsAt": 5, "timeZone": 5}`:         "timeZone",
		`{"title": "x", "startsAt": 5, "schedule": 5}`:         "startsAt",
	} {
		answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), body)
		message, _ := answer.body["error"].(map[string]any)["message"].(string)
		if answer.got != "400 malformedBody" || !strings.Contains(message, `"`+field+`"`) {
			t.Errorf("%s: %s %q, want %s refused first", body, answer.got, message, field)
		}
	}
}

func TestAnIntervalReachesUpTo99(t *testing.T) {
	s, f := New(t)
	for interval, want := range map[int]string{99: "201", 100: "422 eventRepetitionScheduleInvalid", 0: "422 eventRepetitionScheduleInvalid"} {
		answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), map[string]any{
			"title": "x", "firstDay": "2035-10-05", "schedule": map[string]any{"rhythm": "weekly", "interval": interval},
		})
		if answer.got != want {
			t.Errorf("interval %d: %s, want %s", interval, answer.got, want)
		}
	}
}

// Minka trims the days, the link and the zone of an event before it reads
// them, and neither the moments nor the days of a schedule.
func TestOnlyTheDaysTheLinkAndTheZoneOfAnEventAreTrimmed(t *testing.T) {
	s, f := New(t)
	allDay := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "x", "firstDay": " 2035-10-05 ", "lastDay": "\t2035-10-06\n", "joinUrl": " https://meet.example.com/x "}`)
	if allDay.got != "201" || allDay.body["firstDay"] != "2035-10-05" || allDay.body["lastDay"] != "2035-10-06" || allDay.body["joinUrl"] != "https://meet.example.com/x" {
		t.Errorf("trimmed fields: %s %s", allDay.got, allDay.raw)
	}
	moments := `"startsAt": "2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z"`
	if zone := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), `{"title": "x", "schedule": {}, "timeZone": " Europe/Berlin ", `+moments+`}`); zone.got != "201" || zone.body["timeZone"] != "Europe/Berlin" {
		t.Errorf("a trimmed zone: %s %s", zone.got, zone.raw)
	}
	for body, want := range map[string]string{
		`{"title": "x", "startsAt": " 2035-10-05T09:00:00Z", "endsAt": "2035-10-05T10:00:00Z"}`: "422 eventTimeNotReadable",
		`{"title": "x", "schedule": {"endsOn": " 2035-12-01 "}, ` + moments + `}`:               "422 eventRepetitionDatesNotADate",
		`{"title": "x", "schedule": {"startsOn": " 2035-10-05 "}, ` + moments + `}`:             "422 eventRepetitionDatesNotADate",
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, eventsOf(f), body); answer.got != want {
			t.Errorf("%s: %s, want %s", body, answer.got, want)
		}
	}
}
