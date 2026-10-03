package cli

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// --calendars brings the calendars and says what became of them: a bundle with
// a calendar is one only a Minka that applies events takes, and the run says
// so, beside what of the calendar it could not carry.
func TestImportBasecampBringsTheCalendarsWhereItIsAskedTo(t *testing.T) {
	e := newImportEnv(t)
	e.account.AddCalendarProject()
	project := strconv.Itoa(fakebasecamp.CalendarProject)

	env, r := e.runJSON("import", "basecamp", "--project", project, "--calendars", "--out", filepath.Join(t.TempDir(), "bundle"))
	expectCode(t, r, ExitOK)
	var data struct {
		Counts    map[string]int  `json:"counts"`
		Calendars json.RawMessage `json:"calendars"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Counts["events"] != 6 || data.Counts["tools"] != 1 {
		t.Errorf("counts %v", data.Counts)
	}
	var calendars map[string]any
	if err := json.Unmarshal(data.Calendars, &calendars); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"calendars": []any{map[string]any{
			"projectSourceId": project, "toolSourceId": "9601", "written": float64(6), "trashed": float64(1),
			"thatRepeat": float64(2), "withASeries": float64(1), "allDayInInstants": []any{"9619"}, "timedInDays": []any{},
			"repetitionNotMapped":  []any{map[string]any{"part": "time_zone", "events": float64(1), "sourceIds": []any{"9615"}}},
			"longerThanMinkaTakes": float64(0),
		}},
		"commentsUnderSingleDays": []any{map[string]any{"projectSourceId": project, "comments": float64(3), "daySourceIds": []any{"9701", "9702"}}},
	}
	if !reflect.DeepEqual(calendars, want) {
		t.Errorf("calendars\n got %v\nwant %v", calendars, want)
	}
	for _, said := range []string{"only a Minka whose import applies events takes", "1 event repeats at the source and comes once",
		"1 entry of a calendar is left out", "3 comments were written under single days"} {
		if !strings.Contains(env.Notice, said) {
			t.Errorf("the notice does not say %q: %s", said, env.Notice)
		}
	}

	styled := e.run("import", "basecamp", "--project", project, "--calendars", "--dry-run", "--styled")
	expectCode(t, styled, ExitOK)
	for _, said := range []string{"Calendar 9601 of project " + project + ": 6 events, 1 trashed; 2 repeat at the source, 1 of them with their series",
		"time_zone of a schedule, not mapped, the event comes once: 1 — 9615", "all-day entries said in moments, left out: 1 — 9619",
		"Comments under single days of a series in project " + project + ", not carried: 3"} {
		if !strings.Contains(styled.stdout, said) {
			t.Errorf("the text does not say %q:\n%s", said, styled.stdout)
		}
	}
}

// Without --calendars a calendar is a tool the run skips, and the report says
// nothing of calendars at all.
func TestImportBasecampLeavesTheCalendarsOutUnlessAsked(t *testing.T) {
	e := newImportEnv(t)
	e.account.AddCalendarProject()
	env, r := e.runJSON("import", "basecamp", "--project", strconv.Itoa(fakebasecamp.CalendarProject), "--dry-run")
	expectCode(t, r, ExitOK)
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	if _, said := data["calendars"]; said {
		t.Errorf("a run without --calendars reports %v", data["calendars"])
	}
	if counts := data["counts"].(map[string]any); counts["events"] != nil || counts["tools"] != nil {
		t.Errorf("counts %v", counts)
	}
	if strings.Contains(env.Notice, "calendar") {
		t.Errorf("the notice speaks of calendars: %s", env.Notice)
	}
}
