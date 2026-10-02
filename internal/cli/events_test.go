package cli

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTheCalendarsInUseOfAProjectAreListed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "events", "tools", f.Project.ID)
	calendars := listOfObjects(t, env.Data)
	if len(calendars) != 1 || calendars[0]["id"] != f.Calendar.ID {
		t.Fatalf("calendars %v: one no longer in use is listed, or the one in use is missing", calendars)
	}
	_, r := e.runJSON("", "events", "tools", f.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
}

func TestASpanIsListedWithTheOccurrencesMinkaAnswers(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-11-02")
	if query := e.lastRequestQuery(); query != "from=2026-10-19&to=2026-11-02" {
		t.Errorf("query %q", query)
	}
	span := decode(t, env.Data)
	if ids := idsOf(t, span, "events"); !slices.Equal(ids, []string{f.WeeklyEvent.ID, f.AllDayEvent.ID, f.OnceEvent.ID}) {
		t.Fatalf("events %v", ids)
	}
	series := decode(t, rawAt(t, span, "events", 0))
	if occurrences := series["occurrences"].([]any); len(occurrences) != 3 ||
		occurrences[1].(map[string]any)["startsAt"] != "2026-10-26T09:00:00.000000Z" {
		t.Errorf("the occurrences are not handed on as Minka answered them: %v", series["occurrences"])
	}
	if !strings.Contains(env.Summary, "3 events") || !strings.Contains(env.Summary, "Europe/Berlin") {
		t.Errorf("summary %q", env.Summary)
	}

	text := e.run("", "events", "list", f.Calendar.ID, "--from", "2026-10-19", "--to", "2026-11-02", "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"days of Europe/Berlin", "Weekly planning (repeats every week on Mon", "2026-10-20 to 2026-10-22, all day", "Acceptance (2 comments)"} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
	if strings.Count(text.stdout, " to ") < 5 {
		t.Errorf("not every occurrence has a line of its own:\n%s", text.stdout)
	}
}

// A span names both of its days; Minka would answer for a missing one with a
// refusal that names no flag.
func TestASpanWithoutBothDaysIsAUsageError(t *testing.T) {
	e := newTestEnv(t)
	before := len(e.server.Requests())
	for _, args := range [][]string{
		{"events", "list", e.fixture.Calendar.ID},
		{"events", "list", e.fixture.Calendar.ID, "--from", "2026-10-19"},
		{"events", "list", e.fixture.Calendar.ID, "--to", "2026-10-19"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if sent := len(e.server.Requests()) - before; sent != 0 {
		t.Fatalf("%d requests went out", sent)
	}
}

func TestTheRefusalsOfASpanSayWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for flags, want := range map[string]string{
		"--from=2026-10-19 --to=tomorrow":   apiCodeEventsSpanNotADay,
		"--from=2026-10-19 --to=2026-10-18": apiCodeEventsSpanOutOfOrder,
		"--from=2026-01-01 --to=2027-01-02": apiCodeEventsSpanTooLong,
	} {
		env, r := e.runJSON("", append([]string{"events", "list", f.Calendar.ID}, strings.Fields(flags)...)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want || env.Hint == "" {
			t.Errorf("%s: apiCode %v, hint %q", flags, env.Meta["apiCode"], env.Hint)
		}
	}
	env, r := e.runJSON("", "events", "list", f.OtherCalendar.ID, "--from", "2026-10-19", "--to", "2026-10-19")
	expectCode(t, r, ExitNotFound)
	if env.Hint != calendarNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestAnEventIsShownWithItsRepetitionAndItsPeople(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "events", "show", f.WeeklyEvent.ID)
	event := decode(t, env.Data)
	if event["timeZone"] != "Europe/Berlin" || event["descriptionSource"] == nil {
		t.Errorf("data is not the event as Minka answered it: %v", event)
	}
	text := e.run("", "events", "show", f.WeeklyEvent.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"# Weekly planning", "Repeats: every week on Mon, from 2026-10-19, at the clock time of Europe/Berlin", "Taking part: Anke", "Join: https://meet.example.com/planning", "Plan the **week**."} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, text.stdout)
		}
	}

	env, r := e.runJSON("", "events", "show", f.OtherEvent.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != eventNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestATimedSeriesIsAddedWithItsZoneAndItsPeopleByName(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "events", "create", "--tool", f.Calendar.ID, "--title", "Stand-up",
		"--starts-at", "2026-10-20T09:00:00+02:00", "--ends-at", "2026-10-20T09:15:00+02:00",
		"--repeat", "--time-zone", "Europe/Berlin", "--participant", "anke", "--participant", "me",
		"--description", "Say **what** you do.", "--join-url", "https://meet.example.com/standup", "--highlighted")
	body := e.lastWriteBody()
	want := map[string]any{
		"title": "Stand-up", "startsAt": "2026-10-20T09:00:00+02:00", "endsAt": "2026-10-20T09:15:00+02:00",
		"schedule": map[string]any{}, "timeZone": "Europe/Berlin", "participantIds": []any{f.Anke.ID, f.Bruno.ID},
		"description": "<p>Say <strong>what</strong> you do.</p>", "joinUrl": "https://meet.example.com/standup", "highlighted": true,
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body %v\nwant %v", body, want)
	}
	event := decode(t, env.Data)
	if event["timeZone"] != "Europe/Berlin" || event["schedule"].(map[string]any)["weekdays"].([]any)[0] != float64(2) {
		t.Errorf("event %v", event)
	}
	if !strings.Contains(env.Summary, "Added event Stand-up to Termine") {
		t.Errorf("summary %q", env.Summary)
	}

	runOK(t, e, "events", "create", "--tool", f.Calendar.ID, "--title", "Holiday", "--first-day", "2026-12-24")
	if body := e.lastWriteBody(); !reflect.DeepEqual(body, map[string]any{"title": "Holiday", "firstDay": "2026-12-24"}) {
		t.Errorf("only what was given goes out: %v", body)
	}
}

// What the flags say against each other, and a zone Minka would ignore
// without a word, is refused before anything goes out.
func TestTheFlagsOfAnEventThatContradictEachOtherAreUsageErrors(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	create := []string{"events", "create", "--tool", f.Calendar.ID, "--title", "x"}
	moments := []string{"--starts-at", "2026-10-20T09:00:00Z", "--ends-at", "2026-10-20T10:00:00Z"}
	before := len(e.server.Requests())
	for name, args := range map[string][]string{
		"no time":                      create,
		"moments and days":             append(slices.Clone(create), append(slices.Clone(moments), "--first-day", "2026-10-20")...),
		"a start alone":                append(slices.Clone(create), "--starts-at", "2026-10-20T09:00:00Z"),
		"a last day alone":             {"events", "edit", f.OnceEvent.ID, "--last-day", "2026-10-21"},
		"a zone without a schedule":    append(slices.Clone(create), append(slices.Clone(moments), "--time-zone", "Europe/Berlin")...),
		"a zone for an all-day series": append(slices.Clone(create), "--first-day", "2026-10-20", "--repeat", "--time-zone", "Europe/Berlin"),
		"a detail the rhythm ignores":  append(slices.Clone(create), append(slices.Clone(moments), "--rhythm", "daily", "--day", "3")...),
		"no repeat and a schedule":     {"events", "edit", f.WeeklyEvent.ID, "--no-repeat", "--interval", "2"},
		"participants and nobody":      {"events", "edit", f.OnceEvent.ID, "--participant", "me", "--no-participants"},
		"nothing to change":            {"events", "edit", f.OnceEvent.ID},
		"a description format unknown": {"events", "edit", f.OnceEvent.ID, "--description", "x", "--description-format", "rtf"},
		"a calendar that is no ID":     {"events", "create", "--tool", "termine", "--title", "x", "--first-day", "2026-10-20"},
	} {
		_, r := e.runJSON("", args...)
		if r.code != ExitUsage {
			t.Errorf("%s: exit %d, want %d\n%s", name, r.code, ExitUsage, r.stdout)
		}
	}
	if sent := len(e.server.Requests()) - before; sent != 0 {
		t.Fatalf("%d requests went out", sent)
	}
}

func TestTheRefusalsOfAnEventSayWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.Tokens["minka_pat_client"] = f.Client.ID
	create := []string{"events", "create", "--tool", f.Calendar.ID, "--title", "x"}
	for name, c := range map[string]struct {
		args []string
		code string
	}{
		"a moment without an offset": {append(slices.Clone(create), "--starts-at", "2026-10-20T09:00:00", "--ends-at", "2026-10-20T10:00:00"), apiCodeEventTimeNotReadable},
		"an end before the start":    {append(slices.Clone(create), "--first-day", "2026-10-20", "--last-day", "2026-10-19"), apiCodeEventEndsBeforeItStarts},
		"no title":                   {[]string{"events", "edit", f.OnceEvent.ID, "--title", ""}, apiCodeEventTitleMissing},
		"a link that is none":        {[]string{"events", "edit", f.OnceEvent.ID, "--join-url", "meet.example.com"}, apiCodeEventJoinURLNotAnAddress},
		"a schedule off the day":     {append(slices.Clone(create), "--first-day", "2026-10-20", "--rhythm", "weekly", "--weekdays", "mon"), apiCodeEventRepetitionDoesNotFit},
		"an abbreviation as a zone":  {append(slices.Clone(create), "--starts-at", "2026-10-20T09:00:00Z", "--ends-at", "2026-10-20T10:00:00Z", "--repeat", "--time-zone", "CET"), apiCodeEventRepetitionTimeZoneUnknown},
		"an end before the series":   {append(slices.Clone(create), "--first-day", "2026-10-20", "--repeat", "--ends-on", "2026-10-01"), apiCodeEventRepetitionDatesOutOfOrder},
		"a series moved without it":  {[]string{"events", "edit", f.WeeklyEvent.ID, "--starts-at", "2026-10-20T08:00:00Z", "--ends-at", "2026-10-20T08:30:00Z"}, apiCodeEventRepetitionKeepsItsFirstDay},
		"a stranger taking part":     {[]string{"events", "edit", f.OnceEvent.ID, "--participant", f.Stranger.ID}, apiCodePersonNotAProjectMember},
		"a client taking part":       {[]string{"events", "edit", f.OnceEvent.ID, "--participant", f.Client.ID}, apiCodePersonCannotSeeIt},
	} {
		env, r := e.runJSON("", c.args...)
		if r.code != ExitValidation || env.Meta["apiCode"] != c.code || env.Hint == "" {
			t.Errorf("%s: exit %d, apiCode %v, hint %q", name, r.code, env.Meta["apiCode"], env.Hint)
		}
	}

	env, r := e.runJSON("", "events", "create", "--tool", f.OtherCalendar.ID, "--title", "x", "--first-day", "2026-10-20")
	expectCode(t, r, ExitNotFound)
	if env.Hint != calendarNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestAChangeSendsOnlyWhatWasGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for name, c := range map[string]struct {
		args []string
		want map[string]any
	}{
		"the repetition stopped": {[]string{"--no-repeat"}, map[string]any{"schedule": nil}},
		"a series moved": {[]string{"--starts-at", "2026-10-20T10:00:00+02:00", "--ends-at", "2026-10-20T10:30:00+02:00", "--repeat", "--weekdays", "tue"},
			map[string]any{"startsAt": "2026-10-20T10:00:00+02:00", "endsAt": "2026-10-20T10:30:00+02:00", "schedule": map[string]any{"weekdays": []any{float64(2)}}}},
		"the link, the highlight and the people taken off": {[]string{"--join-url", "", "--highlighted=false", "--no-participants"},
			map[string]any{"joinUrl": "", "highlighted": false, "participantIds": []any{}}},
		"the description emptied": {[]string{"--description", ""}, map[string]any{"description": ""}},
	} {
		runOK(t, e, append([]string{"events", "edit", f.WeeklyEvent.ID}, c.args...)...)
		if body := e.lastWriteBody(); !reflect.DeepEqual(body, c.want) {
			t.Errorf("%s: body %v, want %v", name, body, c.want)
		}
	}
}

// Minka ignores what does not fit without a word; the answer tells, and the
// command says so.
func TestWhatMinkaIgnoredOfAChangeIsANotice(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env := runOK(t, e, "events", "edit", f.WeeklyEvent.ID, "--day", "5")
	if !strings.Contains(env.Notice, "Minka ignored --day") {
		t.Errorf("notice %q", env.Notice)
	}
	env = runOK(t, e, "events", "edit", f.AllDayEvent.ID, "--repeat", "--time-zone", "Europe/Berlin")
	if !strings.Contains(env.Notice, "Minka ignored --time-zone") {
		t.Errorf("notice %q", env.Notice)
	}
	env = runOK(t, e, "events", "edit", f.WeeklyEvent.ID, "--title", "Planning")
	if env.Notice != "" {
		t.Errorf("a notice where nothing was ignored: %q", env.Notice)
	}
}

func TestADescriptionWithFilesIsNotReplacedByOneWithout(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.OnceEvent.Description = "<div>Ask " + aMention + "</div>"

	env, r := e.runJSON("", "events", "edit", f.OnceEvent.ID, "--description", "Nothing attached")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || len(e.server.WritingRequests()) != 0 {
		t.Fatalf("hint %q, %d writes", env.Hint, len(e.server.WritingRequests()))
	}
	dropped := runOK(t, e, "events", "edit", f.OnceEvent.ID, "--description", "Nothing attached", "--drop-attachments")
	if !strings.Contains(dropped.Notice, "dropped") {
		t.Fatalf("notice %q", dropped.Notice)
	}
}

func TestAnEventGoesIntoTheTrash(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env := runOK(t, e, "events", "delete", f.OnceEvent.ID)
	var data map[string]any
	_ = json.Unmarshal(env.Data, &data)
	if data["id"] != f.OnceEvent.ID || data["deleted"] != true || !f.OnceEvent.Trashed {
		t.Fatalf("data %v, trashed %v", data, f.OnceEvent.Trashed)
	}
	env, r := e.runJSON("", "events", "trash", f.OnceEvent.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != eventNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}
