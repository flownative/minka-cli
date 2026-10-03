package basecamp_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// calendarProject is the project of the fake that holds a calendar, which only
// a test that adds it meets.
var calendarProject = strconv.Itoa(fakebasecamp.CalendarProject)

// withCalendar is the account of the fake with the project of the calendar.
func withCalendar() *fakebasecamp.Account {
	account := fakebasecamp.New()
	account.AddCalendarProject()
	return account
}

// extractWithCalendars is an extraction that brings the calendars.
func extractWithCalendars(t *testing.T, account *fakebasecamp.Account, projects ...string) (string, basecamp.Result) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Calendars: true,
		Now: func() time.Time { return runDay }}
	result, err := extractor.Extract(t.Context(), projects, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return dir, result
}

// entriesTheAccountShows are the entries of a list of the calendar as the
// account answers them.
func entriesTheAccountShows(t *testing.T, account *fakebasecamp.Account, status string) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for page := 1; ; page++ {
		path := "buckets/7012/schedules/9601/entries.json?page=" + strconv.Itoa(page)
		if status != "" {
			path = "buckets/7012/schedules/9601/entries.json?status=" + status + "&page=" + strconv.Itoa(page)
		}
		content, err := account.Get(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		var listed []map[string]any
		if err := json.Unmarshal(content, &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed) == 0 {
			return entries
		}
		entries = append(entries, listed...)
	}
}

// The account answers an entry the way Basecamp's list does, or the tests below
// would hold the extractor against a shape no real account has: a schedule with
// all ten fields on an entry that repeats and no key at all on one that does
// not, an all-day entry in plain days and a timed one in moments of UTC.
func TestTheAccountListsAnEntryAsBasecampDoes(t *testing.T) {
	account := withCalendar()
	for _, entry := range entriesTheAccountShows(t, account, "") {
		schedule, repeats := entry["recurrence_schedule"].(map[string]any)
		switch entry["id"] {
		case float64(9614), float64(9615):
			if !repeats || len(schedule) != 10 {
				t.Errorf("entry %v sends the schedule %v", entry["id"], entry["recurrence_schedule"])
			}
		default:
			if _, said := entry["recurrence_schedule"]; said {
				t.Errorf("entry %v, which does not repeat, sends a schedule", entry["id"])
			}
		}
		starts := entry["starts_at"].(string)
		if entry["id"] == float64(9611) && (starts != "2020-02-01" || entry["ends_at"] != "2020-02-01") {
			t.Errorf("an all-day entry of one day says %v to %v", starts, entry["ends_at"])
		}
		if entry["id"] == float64(9613) && starts != "2026-06-16T13:00:00.000Z" {
			t.Errorf("a timed entry says %v", starts)
		}
	}
}

// A calendar comes only into a run that asks for it: a Minka that does not
// apply events stops at the first tool of that kind. Without it the dock entry
// is skipped like a kind Minka has nothing for, and nothing of it is asked for.
func TestACalendarStaysOutOfARunThatIsNotAskedForIt(t *testing.T) {
	account := withCalendar()
	dir, result := extract(t, account, calendarProject)

	if events := recordsOrNone(t, dir, bundle.Events); events != nil {
		t.Errorf("a run that was not asked for calendars wrote %v", events)
	}
	for _, tool := range recordsOrNone(t, dir, bundle.Tools) {
		t.Errorf("a run that was not asked for calendars wrote the tool %v", tool)
	}
	if want := []basecamp.SkippedTools{{Kind: "schedule", Enabled: true, Count: 1}}; !reflect.DeepEqual(result.Skipped, want) {
		t.Errorf("skipped %v, want %v", result.Skipped, want)
	}
	for _, path := range account.Requests {
		if strings.Contains(path, "schedules/9601/") || strings.Contains(path, "recordings/96") {
			t.Errorf("a run that was not asked for calendars asked for %s", path)
		}
	}
	if result.Calendars.Calendars == nil || len(result.Calendars.Calendars) > 0 {
		t.Errorf("a run without calendars reports %+v", result.Calendars)
	}
}

// A calendar is a tool of its own kind, and each entry an event with the times
// the source gives it: two moments of a timed one as they stand, the first and
// the last day of an all-day one, never one made of the other.
func TestACalendarComesAsAToolWithItsEvents(t *testing.T) {
	dir, _ := extractWithCalendars(t, withCalendar(), calendarProject)

	tools := records(t, dir, bundle.Tools)
	if len(tools) != 1 || tools[0]["kind"] != "calendar" || tools[0]["name"] != "Schedule" || tools[0]["includeDueDates"] != true {
		t.Fatalf("tools: %v", tools)
	}

	want := map[string]map[string]any{
		"9611": {"allDay": true, "startsOn": "2020-02-01", "endsOn": "2020-02-01", "title": "Company day", "createdBy": "101",
			"description":  `<div>Bring <import-mention ref="102">Grace Sample</import-mention></div><import-attachment ref="attachment-5071"></import-attachment>`,
			"participants": []any{"101"}, "subscribers": []any{"101", "102"}, "joinUrl": nil, "highlighted": false, "status": "active"},
		"9612": {"allDay": true, "startsOn": "2018-12-24", "endsOn": "2018-12-28", "title": "Holidays", "createdBy": "101", "description": nil,
			"participants": []any{"101", "102"}, "subscribers": []any{}, "joinUrl": "https://meet.example.test/holidays", "highlighted": true,
			"status": "active", "visibleToClients": true},
		"9613": {"allDay": false, "startsAt": "2026-06-16T13:00:00.000Z", "endsAt": "2026-06-16T16:00:00.000Z", "title": "Workshop", "createdBy": "101",
			"description": nil, "participants": []any{}, "subscribers": []any{}, "joinUrl": nil, "highlighted": false, "status": "active"},
		"9614": {"allDay": false, "startsAt": "2026-10-02T16:00:00.000Z", "endsAt": "2026-10-02T16:30:00.000Z", "title": "Review", "createdBy": "101",
			"description": nil, "participants": []any{"101", "104"}, "subscribers": []any{"101", "104"}, "joinUrl": nil, "highlighted": false, "status": "active",
			"repetition": map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(2), "startsOn": "2026-10-02",
				"endsOn": nil, "timeZone": "Europe/Berlin"}},
		"9615": {"allDay": false, "startsAt": "2026-10-02T07:00:00.000Z", "endsAt": "2026-10-02T07:15:00.000Z", "title": "Stand-up", "createdBy": "102",
			"description": nil, "participants": []any{}, "subscribers": []any{}, "joinUrl": nil, "highlighted": false, "status": "active",
			"repetitionNotMapped": []any{"time_zone"}},
		"9616": {"allDay": false, "startsAt": "2026-07-01T08:00:00.000Z", "endsAt": "2026-07-01T09:00:00.000Z", "title": "Cancelled", "createdBy": "101",
			"description": nil, "participants": []any{}, "joinUrl": nil, "highlighted": false, "status": "trashed"},
	}
	events := records(t, dir, bundle.Events)
	var order []string
	for _, event := range events {
		id := event["sourceId"].(string)
		order = append(order, id)
		expected, known := want[id]
		if !known {
			t.Errorf("event %s is in the bundle", id)
			continue
		}
		got := map[string]any{}
		for key, value := range event {
			switch key {
			case "sourceId", "createdAt", "updatedAt", "projectSourceId", "toolSourceId":
			default:
				got[key] = value
			}
		}
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("event %s\n got %v\nwant %v", id, got, expected)
		}
		if event["projectSourceId"] != calendarProject || event["toolSourceId"] != "9601" {
			t.Errorf("event %s stands in %v, %v", id, event["projectSourceId"], event["toolSourceId"])
		}
	}
	// The order of the lines decides nothing; it is the order they were made in,
	// so that two runs write the same bytes.
	if wantOrder := []string{"9611", "9612", "9613", "9614", "9615", "9616"}; !slices.Equal(order, wantOrder) {
		t.Errorf("events %v, want %v", order, wantOrder)
	}
}

// What hangs on an event comes with it: the files of its description, its
// comments, its boosts and the boosts of its comments, a trashed event's
// comments included, which Minka passes over with the event.
func TestWhatHangsOnAnEventComesWithIt(t *testing.T) {
	dir, _ := extractWithCalendars(t, withCalendar(), calendarProject)

	var comments []string
	for _, comment := range records(t, dir, bundle.Comments) {
		comments = append(comments, comment["sourceId"].(string)+" on "+comment["parentKind"].(string)+" "+comment["parentSourceId"].(string))
	}
	if want := []string{"9631 on event 9611", "9632 on event 9616"}; !slices.Equal(comments, want) {
		t.Errorf("comments %v, want %v", comments, want)
	}
	var reactions []string
	for _, reaction := range records(t, dir, bundle.Reactions) {
		reactions = append(reactions, reaction["sourceId"].(string)+" on "+reaction["targetKind"].(string)+" "+reaction["targetSourceId"].(string))
	}
	if want := []string{"9641 on event 9611", "9642 on comment 9631"}; !slices.Equal(reactions, want) {
		t.Errorf("reactions %v, want %v", reactions, want)
	}
	var files []string
	for _, file := range records(t, dir, bundle.Attachments) {
		if file["ownerKind"] != bundle.OwnerPerson {
			files = append(files, file["sourceId"].(string)+" of "+file["ownerKind"].(string)+" "+file["ownerSourceId"].(string))
		}
	}
	if want := []string{"attachment-5071 of event 9611"}; !slices.Equal(files, want) {
		t.Errorf("files %v, want %v", files, want)
	}
}

// An entry of a status a bundle has no word for is left out and counted by its
// status, whichever list names it, and nothing that hangs on it is asked for.
// One in the trash is written as trashed; who follows it is not asked, as for
// every record in the trash.
func TestAnEntryOfAStatusABundleHasNoWordForIsLeftOutAndCounted(t *testing.T) {
	account := withCalendar()
	_, result := extractWithCalendars(t, account, calendarProject)

	want := []basecamp.LeftOutRecords{
		{Record: "event", Field: "status", Value: "drafted", In: "calendar", InSourceID: "9601", LeftOut: 1},
		{Record: "event", Field: "status", Value: "archived", In: "calendar", InSourceID: "9601", LeftOut: 1},
	}
	if !reflect.DeepEqual(result.RecordsLeftOut, want) {
		t.Errorf("left out %+v, want %+v", result.RecordsLeftOut, want)
	}
	for _, path := range account.Requests {
		for _, gone := range []string{"9617", "9618", "9619", "9633", "9634"} {
			if strings.Contains(path, "recordings/"+gone+"/") {
				t.Errorf("a run asked for %s of an entry it leaves out", path)
			}
		}
		if strings.Contains(path, "recordings/9616/subscription.json") {
			t.Error("a run asked who follows an entry in the trash")
		}
	}
	if !slices.Contains(account.Requests, "buckets/7012/recordings/9616/comments.json?page=1") {
		t.Error("the comments of an entry in the trash were not asked for")
	}
}

// An all-day entry the source says in moments is left out and named: a day
// made of a moment is the day of one zone, and the event would move by one for
// somebody. Nothing that hangs on it is asked for.
func TestAnAllDayEntrySaidInMomentsIsLeftOutAndNamed(t *testing.T) {
	account := withCalendar()
	_, result := extractWithCalendars(t, account, calendarProject)

	if got := result.Calendars.Calendars[0].AllDayInInstants; !slices.Equal(got, []string{"9619"}) {
		t.Errorf("all-day entries said in moments: %v", got)
	}
	if got := result.Calendars.Calendars[0].TimedInDays; len(got) > 0 {
		t.Errorf("timed entries said in days: %v", got)
	}

	timed := withCalendar()
	timed.Calendar().Entries[2].StartsAt, timed.Calendar().Entries[2].EndsAt = "2026-06-16", "2026-06-16"
	dir, result := extractWithCalendars(t, timed, calendarProject)
	if got := result.Calendars.Calendars[0].TimedInDays; !slices.Equal(got, []string{"9613"}) {
		t.Errorf("timed entries said in days: %v", got)
	}
	if slices.ContainsFunc(records(t, dir, bundle.Events), func(event map[string]any) bool { return event["sourceId"] == "9613" }) {
		t.Error("a timed entry said in days is in the bundle")
	}
}

// A time that is neither a day nor a moment, and an end before the start, end
// the run: Minka would refuse the event, and nobody can put such a time right
// afterwards.
func TestATimeNoEventCanHaveEndsTheRun(t *testing.T) {
	for name, change := range map[string]func(*fakebasecamp.Entry){
		"no time":             func(e *fakebasecamp.Entry) { e.StartsAt = "" },
		"a day nobody has":    func(e *fakebasecamp.Entry) { e.AllDay, e.StartsAt, e.EndsAt = true, "2026-02-30", "2026-02-30" },
		"a day of the year 0": func(e *fakebasecamp.Entry) { e.AllDay, e.StartsAt, e.EndsAt = true, "0000-01-01", "0000-01-01" },
		"a moment of the year 0": func(e *fakebasecamp.Entry) {
			e.StartsAt, e.EndsAt = "0000-06-16T12:00:00.000Z", "0000-06-16T13:00:00.000Z"
		},
		"the year 0 in UTC": func(e *fakebasecamp.Entry) {
			e.StartsAt, e.EndsAt = "0001-01-01T00:30:00.000+01:00", "0001-01-01T01:30:00.000+01:00"
		},
		"an end before the start": func(e *fakebasecamp.Entry) { e.EndsAt = "2026-06-16T12:00:00.000Z" },
		"an end before the day":   func(e *fakebasecamp.Entry) { e.AllDay, e.StartsAt, e.EndsAt = true, "2026-06-16", "2026-06-15" },
		"a day and a moment":      func(e *fakebasecamp.Entry) { e.EndsAt = "2026-06-16" },
	} {
		t.Run(name, func(t *testing.T) {
			account := withCalendar()
			change(&account.Calendar().Entries[2])
			writer := bundle.Discard()
			defer writer.Abort()
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Calendars: true, Now: func() time.Time { return runDay }}
			_, err := extractor.Extract(t.Context(), []string{calendarProject}, writer)
			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.Contains(failed.Message, "event 9613") {
				t.Errorf("the run ended with %v", err)
			}
		})
	}
}

// The report of a calendar counts what the run wrote and names what it could
// not map of a schedule; the comments written under single days of a series,
// which a bundle has no place for, are counted from the list of every comment
// of the project, the one place that names them.
func TestTheReportOfACalendarCountsWhatTheRunWrote(t *testing.T) {
	dir, result := extractWithCalendars(t, withCalendar(), calendarProject)

	want := basecamp.CalendarCounts{
		Calendars: []basecamp.CalendarCount{{
			ProjectSourceID: calendarProject, ToolSourceID: "9601",
			Written: 6, Trashed: 1, ThatRepeat: 2, WithASeries: 1,
			AllDayInInstants: []string{"9619"}, TimedInDays: []string{},
			RepetitionNotMapped: []basecamp.EventRepetitionNotMapped{{Part: "time_zone", Events: 1, SourceIDs: []string{"9615"}}},
		}},
		CommentsUnderSingleDays: []basecamp.SingleDayComments{{ProjectSourceID: calendarProject, Comments: 3, DaySourceIDs: []string{"9701", "9702"}}},
	}
	if !reflect.DeepEqual(result.Calendars, want) {
		t.Errorf("calendars\n got %+v\nwant %+v", result.Calendars, want)
	}
	for _, comment := range records(t, dir, bundle.Comments) {
		if id := comment["sourceId"]; id == "9661" || id == "9662" || id == "9663" {
			t.Errorf("a comment under a single day is in the bundle: %v", comment)
		}
	}
}

// A part of a schedule names up to ten events, and counts them all.
func TestAPartOfAScheduleNamesTenEventsAndCountsThemAll(t *testing.T) {
	account := withCalendar()
	model := account.Calendar().Entries[4]
	for i := range 11 {
		one := model
		one.ID = 9800 + int64(i)
		account.Calendar().Entries = append(account.Calendar().Entries, one)
	}
	_, result := extractWithCalendars(t, account, calendarProject)
	parts := result.Calendars.Calendars[0].RepetitionNotMapped
	if len(parts) != 1 || parts[0].Events != 12 || len(parts[0].SourceIDs) != 10 || parts[0].SourceIDs[0] != "9615" {
		t.Errorf("not mapped %+v", parts)
	}
}

// Only a project with a series is asked for the list of all its comments,
// which is long, and a single day hangs on nothing else.
func TestAProjectWithoutASeriesIsNotAskedForEveryComment(t *testing.T) {
	account := withCalendar()
	entries := account.Calendar().Entries
	account.Calendar().Entries = append(entries[:3:3], entries[5:]...)
	_, result := extractWithCalendars(t, account, calendarProject)
	for _, path := range account.Requests {
		if strings.HasPrefix(path, "projects/recordings.json?type=Comment") {
			t.Errorf("a project without a series was asked for %s", path)
		}
	}
	if len(result.Calendars.CommentsUnderSingleDays) > 0 {
		t.Errorf("comments under single days: %+v", result.Calendars.CommentsUnderSingleDays)
	}
}

// A calendar that is switched off is left out like every tool the source has
// switched off: no tool, and none of its entries asked for.
func TestACalendarThatIsSwitchedOffIsSkipped(t *testing.T) {
	account := withCalendar()
	account.Calendar().Enabled = false
	dir, result := extractWithCalendars(t, account, calendarProject)
	if events := recordsOrNone(t, dir, bundle.Events); events != nil {
		t.Errorf("events of a calendar that is switched off: %v", events)
	}
	if want := []basecamp.SkippedTools{{Kind: "schedule", Enabled: false, Count: 1}}; !reflect.DeepEqual(result.Skipped, want) {
		t.Errorf("skipped %v, want %v", result.Skipped, want)
	}
	for _, path := range account.Requests {
		if strings.Contains(path, "/entries.json") {
			t.Errorf("a calendar that is switched off was asked for %s", path)
		}
	}
}

// A run of a project with a calendar asks for exactly these and nothing else:
// the lists of the calendar, and of each entry it writes what hangs on it. No
// single day of a series and no entry on its own is ever asked for, because
// Basecamp makes a record of the day where somebody asks for it.
func TestARunReadsACalendarFromItsListsAlone(t *testing.T) {
	account := withCalendar()
	extractWithCalendars(t, account, calendarProject)

	if len(account.ReadsThatWrite) > 0 {
		t.Fatalf("the run wrote at the source by asking for %v", account.ReadsThatWrite)
	}
	var calendar []string
	for _, path := range account.Requests {
		if strings.Contains(path, "schedule_entries") {
			t.Errorf("the run asked for %s", path)
		}
		if !strings.HasPrefix(path, "people.json") && !strings.HasPrefix(path, "projects/7012") {
			calendar = append(calendar, path)
		}
	}
	want := []string{
		"buckets/7012/schedules/9601.json",
		"buckets/7012/schedules/9601/entries.json?page=1", "buckets/7012/schedules/9601/entries.json?page=2",
		"buckets/7012/schedules/9601/entries.json?page=3", "buckets/7012/schedules/9601/entries.json?page=4",
		"buckets/7012/schedules/9601/entries.json?page=5",
		"buckets/7012/schedules/9601/entries.json?status=trashed&page=1", "buckets/7012/schedules/9601/entries.json?status=trashed&page=2",
		"buckets/7012/schedules/9601/entries.json?status=archived&page=1", "buckets/7012/schedules/9601/entries.json?status=archived&page=2",
		"buckets/7012/recordings/9611/subscription.json",
		"buckets/7012/recordings/9611/comments.json?page=1", "buckets/7012/recordings/9611/comments.json?page=2",
		"buckets/7012/recordings/9611/boosts.json?page=1", "buckets/7012/recordings/9611/boosts.json?page=2",
		"buckets/7012/recordings/9631/boosts.json?page=1", "buckets/7012/recordings/9631/boosts.json?page=2",
		"buckets/7012/recordings/9612/subscription.json",
		"buckets/7012/recordings/9613/subscription.json",
		"buckets/7012/recordings/9614/subscription.json",
		"buckets/7012/recordings/9615/subscription.json",
		"buckets/7012/recordings/9616/comments.json?page=1", "buckets/7012/recordings/9616/comments.json?page=2",
		"projects/recordings.json?type=Comment&bucket=7012&page=1", "projects/recordings.json?type=Comment&bucket=7012&page=2",
		"projects/recordings.json?type=Comment&bucket=7012&page=3", "projects/recordings.json?type=Comment&bucket=7012&page=4",
		"projects/recordings.json?type=Comment&bucket=7012&page=5",
	}
	if !slices.Equal(calendar, want) {
		t.Errorf("the run asked for\n%s\nwant\n%s", strings.Join(calendar, "\n"), strings.Join(want, "\n"))
	}
}

// A run that asked for a single day would end before anything else is asked
// for, whatever part of it asked: the guard stands between every request and
// the source.
func TestARunThatWouldAskForASingleDayEndsInstead(t *testing.T) {
	account := withCalendar()
	account.Calendar().URL = "https://3.basecampapi.test/4242/buckets/7012/schedule_entries/9614.json"
	writer := bundle.Discard()
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Calendars: true, Now: func() time.Time { return runDay }}
	_, err := extractor.Extract(t.Context(), []string{calendarProject}, writer)
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.Contains(failed.Message, "schedule_entries/9614.json") {
		t.Errorf("the run ended with %v", err)
	}
	if len(account.ReadsThatWrite) > 0 {
		t.Errorf("the run wrote at the source by asking for %v", account.ReadsThatWrite)
	}
}

// A walk is no snapshot: an entry two lists of its calendar name is written
// once, as the first list says it, with what hangs on it once.
func TestAnEntryTwoListsNameIsWrittenOnce(t *testing.T) {
	account := withCalendar()
	account.Calendar().Entries[0].AlsoListedIn = "trashed"
	dir, result := extractWithCalendars(t, account, calendarProject)

	var written []string
	for _, event := range records(t, dir, bundle.Events) {
		if event["sourceId"] == "9611" {
			written = append(written, event["status"].(string))
		}
	}
	if !slices.Equal(written, []string{"active"}) {
		t.Errorf("entry 9611 is written as %v", written)
	}
	comments := 0
	for _, comment := range records(t, dir, bundle.Comments) {
		if comment["sourceId"] == "9631" {
			comments++
		}
	}
	if comments != 1 || result.Calendars.Calendars[0].Written != 6 {
		t.Errorf("its comment is written %d times, %d events counted", comments, result.Calendars.Calendars[0].Written)
	}
}

// Everybody an event names is a person of the bundle: who made it, who it is
// for and who follows it.
func TestEveryPersonAnEventNamesIsAPersonOfTheBundle(t *testing.T) {
	dir, _ := extractWithCalendars(t, withCalendar(), calendarProject)
	people := map[string]bool{}
	for _, person := range records(t, dir, bundle.People) {
		people[person["sourceId"].(string)] = true
	}
	for _, event := range records(t, dir, bundle.Events) {
		named := []any{event["createdBy"]}
		named = append(named, event["participants"].([]any)...)
		if subscribers, said := event["subscribers"].([]any); said {
			named = append(named, subscribers...)
		}
		for _, one := range named {
			if !people[one.(string)] {
				t.Errorf("event %v names %v, who is no person of the bundle", event["sourceId"], one)
			}
		}
	}
}
