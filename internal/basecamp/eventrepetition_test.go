package basecamp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// berlinFriday is the first occurrence of the one series the account was seen
// to hold: Friday, 2 October 2026, 18:00 to 18:30 in Berlin, as the list of its
// calendar gives it.
var berlinFriday = eventTimes{
	startsAt: time.Date(2026, 10, 2, 16, 0, 0, 0, time.UTC),
	endsAt:   time.Date(2026, 10, 2, 16, 30, 0, 0, time.UTC),
}

// sampledSeries is that series' schedule as Basecamp sent it, with what a case
// changes of it.
func sampledSeries(change func(map[string]any)) json.RawMessage {
	schedule := map[string]any{
		"frequency": "every_other_week", "days": []int{5}, "hour": 18, "minute": 0, "duration": 1800,
		"start_date": "2026-10-02", "end_date": nil, "week_instance": nil, "week_interval": nil, "month_interval": nil,
	}
	if change != nil {
		change(schedule)
	}
	raw, err := json.Marshal(schedule)
	if err != nil {
		panic(err)
	}
	return raw
}

func set(values map[string]any) func(map[string]any) {
	return func(schedule map[string]any) {
		for key, value := range values {
			schedule[key] = value
		}
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	if reflect.ValueOf(value).IsNil() {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Each frequency a calendar knows becomes the rhythm Minka has for it, the
// clock time and the length staying the record's own: the schedule carries the
// first day, the zone and the days, and nothing of a time.
func TestEachFrequencyOfACalendarBecomesMinkasRhythm(t *testing.T) {
	berlin := "Europe/Berlin"
	weekly := func(interval int, weekdays ...any) map[string]any {
		return map[string]any{"rhythm": "weekly", "weekdays": weekdays, "interval": float64(interval),
			"startsOn": "2026-10-02", "endsOn": nil, "timeZone": berlin}
	}
	daily := func(weekdays ...any) map[string]any {
		return map[string]any{"rhythm": "daily", "weekdays": weekdays, "startsOn": "2026-10-02", "endsOn": nil, "timeZone": berlin}
	}
	monthly := func(fields map[string]any) map[string]any {
		answer := map[string]any{"startsOn": "2026-10-02", "endsOn": nil, "timeZone": berlin, "interval": float64(1)}
		for key, value := range fields {
			answer[key] = value
		}
		return answer
	}
	cases := []struct {
		name     string
		schedule json.RawMessage
		want     map[string]any
	}{
		{"every_other_week as sampled", sampledSeries(nil), weekly(2, float64(5))},
		{"every_day without days", sampledSeries(set(map[string]any{"frequency": "every_day", "days": []int{}})),
			daily(float64(1), float64(2), float64(3), float64(4), float64(5), float64(6), float64(7))},
		{"every_day with days, Sunday among them", sampledSeries(set(map[string]any{"frequency": "every_day", "days": []int{5, 0, 1}})),
			daily(float64(1), float64(5), float64(7))},
		{"every_weekday", sampledSeries(set(map[string]any{"frequency": "every_weekday", "days": []int{}})),
			daily(float64(1), float64(2), float64(3), float64(4), float64(5))},
		{"every_week", sampledSeries(set(map[string]any{"frequency": "every_week"})), weekly(1, float64(5))},
		{"every_week with several days", sampledSeries(set(map[string]any{"frequency": "every_week", "days": []int{1, 5}})),
			weekly(1, float64(1), float64(5))},
		{"every_week without days takes the first day's", sampledSeries(set(map[string]any{"frequency": "every_week", "days": []int{}})),
			weekly(1, float64(5))},
		{"every_other_week without days", sampledSeries(set(map[string]any{"days": []int{}})), weekly(2, float64(5))},
		{"custom_week", sampledSeries(set(map[string]any{"frequency": "custom_week", "week_interval": 3})), weekly(3, float64(5))},
		{"custom_week every week on several days", sampledSeries(set(map[string]any{"frequency": "custom_week", "week_interval": 1, "days": []int{1, 5}})),
			weekly(1, float64(1), float64(5))},
		{"every_month on the first Friday", sampledSeries(set(map[string]any{"frequency": "every_month", "week_instance": 1})),
			monthly(map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(5), "instance": float64(1)})},
		{"every_month on the day", sampledSeries(set(map[string]any{"frequency": "every_month", "days": []int{}})),
			monthly(map[string]any{"rhythm": "monthlyOnDay", "day": float64(2)})},
		{"every_day_of_month", sampledSeries(set(map[string]any{"frequency": "every_day_of_month", "days": []int{2}})),
			monthly(map[string]any{"rhythm": "monthlyOnDay", "day": float64(2)})},
		{"custom_month on the day", sampledSeries(set(map[string]any{"frequency": "custom_month", "days": []int{}, "month_interval": 2})),
			monthly(map[string]any{"rhythm": "monthlyOnDay", "day": float64(2), "interval": float64(2)})},
		{"every_year", sampledSeries(set(map[string]any{"frequency": "every_year", "days": []int{}})),
			map[string]any{"rhythm": "yearly", "month": float64(10), "day": float64(2), "startsOn": "2026-10-02", "endsOn": nil, "timeZone": berlin}},
		{"with an end", sampledSeries(set(map[string]any{"end_date": "2026-12-31"})),
			map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(2), "startsOn": "2026-10-02", "endsOn": "2026-12-31", "timeZone": berlin}},
		{"without a length", sampledSeries(set(map[string]any{"duration": nil})), weekly(2, float64(5))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repetition, notMapped := eventRepetitionOf(c.schedule, berlinFriday, berlin)
			if len(notMapped) > 0 {
				t.Fatalf("not mapped: %v", notMapped)
			}
			if got := asMap(t, repetition); !reflect.DeepEqual(got, c.want) {
				t.Errorf("repetition\n got %v\nwant %v", got, c.want)
			}
		})
	}
}

// What of a schedule does not fit is named by Basecamp's field, each part once,
// and the event comes once: a series on days the source never had would be
// shown to everybody on it.
func TestWhatOfACalendarsScheduleDoesNotFitLeavesTheEventSingle(t *testing.T) {
	cases := []struct {
		name     string
		schedule json.RawMessage
		times    eventTimes
		zone     string
		want     []string
	}{
		{"a frequency nobody knows", sampledSeries(set(map[string]any{"frequency": "every_fortnight"})), berlinFriday, "Europe/Berlin", []string{"frequency"}},
		{"a schedule of another shape", json.RawMessage(`{"frequency":"every_week","days":"5"}`), berlinFriday, "Europe/Berlin", []string{"recurrence_schedule"}},
		{"several days every other week", sampledSeries(set(map[string]any{"days": []int{1, 5}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		// Two reasons to name the same part name it once.
		{"other days every other week", sampledSeries(set(map[string]any{"days": []int{1, 2}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"every other week counted in threes", sampledSeries(set(map[string]any{"week_interval": 3})), berlinFriday, "Europe/Berlin", []string{"week_interval"}},
		{"several days every third week", sampledSeries(set(map[string]any{"frequency": "custom_week", "week_interval": 3, "days": []int{1, 5}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"a weekday the first day is not", sampledSeries(set(map[string]any{"frequency": "every_week", "days": []int{1}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"weekdays the first day is not", sampledSeries(set(map[string]any{"frequency": "every_weekday", "start_date": "2026-10-03"})),
			eventTimes{startsAt: time.Date(2026, 10, 3, 16, 0, 0, 0, time.UTC), endsAt: time.Date(2026, 10, 3, 16, 30, 0, 0, time.UTC)}, "Europe/Berlin", []string{"days"}},
		{"another day of the month", sampledSeries(set(map[string]any{"frequency": "every_day_of_month", "days": []int{3}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"another day of the month in custom_month", sampledSeries(set(map[string]any{"frequency": "custom_month", "days": []int{3}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"several days a month", sampledSeries(set(map[string]any{"frequency": "every_month", "days": []int{2, 3}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"the last week of the month", sampledSeries(set(map[string]any{"frequency": "every_month", "week_instance": -1})), berlinFriday, "Europe/Berlin", []string{"week_instance"}},
		{"a week the first day is not in", sampledSeries(set(map[string]any{"frequency": "every_month", "week_instance": 2})), berlinFriday, "Europe/Berlin", []string{"week_instance"}},
		{"another weekday of the month", sampledSeries(set(map[string]any{"frequency": "every_month", "week_instance": 1, "days": []int{4}})), berlinFriday, "Europe/Berlin", []string{"days"}},
		{"an end before the start", sampledSeries(set(map[string]any{"end_date": "2026-10-01"})), berlinFriday, "Europe/Berlin", []string{"end_date"}},
		{"no start", sampledSeries(set(map[string]any{"start_date": nil})), berlinFriday, "Europe/Berlin", []string{"start_date"}},
		{"a start of another shape", sampledSeries(set(map[string]any{"start_date": "2.10.2026"})), berlinFriday, "Europe/Berlin", []string{"start_date"}},
		{"another hour", sampledSeries(set(map[string]any{"hour": 16})), berlinFriday, "Europe/Berlin", []string{"hour"}},
		{"another minute", sampledSeries(set(map[string]any{"minute": 30})), berlinFriday, "Europe/Berlin", []string{"hour"}},
		{"no hour", sampledSeries(set(map[string]any{"hour": nil})), berlinFriday, "Europe/Berlin", []string{"hour"}},
		// A list that names a later day of the series than the first.
		{"a later occurrence than the first", sampledSeries(set(map[string]any{"start_date": "2026-09-18"})), berlinFriday, "Europe/Berlin", []string{"hour"}},
		{"the hour of another zone", sampledSeries(nil), berlinFriday, "Europe/London", []string{"hour"}},
		{"another length", sampledSeries(set(map[string]any{"duration": 3600})), berlinFriday, "Europe/Berlin", []string{"duration"}},
		{"nobody's zone", sampledSeries(nil), berlinFriday, "", []string{"time_zone"}},
		{"an abbreviation", sampledSeries(nil), berlinFriday, "CEST", []string{"time_zone"}},
		{"an offset", sampledSeries(nil), berlinFriday, "+02:00", []string{"time_zone"}},
		{"a zone of another case", sampledSeries(nil), berlinFriday, "europe/berlin", []string{"time_zone"}},
		{"the zone of the machine", sampledSeries(nil), berlinFriday, "Local", []string{"time_zone"}},
		{"several parts", sampledSeries(set(map[string]any{"hour": 17, "frequency": "every_week", "days": []int{1}, "duration": 60})), berlinFriday, "Europe/Berlin",
			[]string{"hour", "duration", "days"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repetition, notMapped := eventRepetitionOf(c.schedule, c.times, c.zone)
			if repetition != nil {
				t.Errorf("the event comes with the series %+v", *repetition)
			}
			if !reflect.DeepEqual(notMapped, c.want) {
				t.Errorf("not mapped %v, want %v", notMapped, c.want)
			}
		})
	}
}

// The clock time of a series holds in its zone, so a series that starts in
// winter is proven at another instant than one in summer, and an older name of
// a zone counts as the zone it names.
func TestTheClockTimeOfASeriesIsProvenInTheZoneOfItsDay(t *testing.T) {
	winter := eventTimes{startsAt: time.Date(2027, 1, 8, 17, 0, 0, 0, time.UTC), endsAt: time.Date(2027, 1, 8, 17, 30, 0, 0, time.UTC)}
	if repetition, notMapped := eventRepetitionOf(sampledSeries(set(map[string]any{"start_date": "2027-01-08"})), winter, "Europe/Berlin"); repetition == nil {
		t.Errorf("18:00 in Berlin in winter is not proven: %v", notMapped)
	}
	for _, zone := range []struct {
		name string
		hour int
	}{{"Europe/Kiev", 19}, {"US/Pacific", 9}} {
		repetition, notMapped := eventRepetitionOf(sampledSeries(set(map[string]any{"hour": zone.hour})), berlinFriday, zone.name)
		if repetition == nil || repetition.TimeZone == nil || *repetition.TimeZone != zone.name {
			t.Errorf("%s: %+v, not mapped %v", zone.name, repetition, notMapped)
		}
	}
}

// An all-day series repeats by plain days: it carries no zone, whoever made it,
// and its proof is the first day alone.
func TestAnAllDaySeriesIsProvenByItsFirstDay(t *testing.T) {
	friday := eventTimes{allDay: true, startsOn: "2026-10-02"}
	allDay := func(values map[string]any) json.RawMessage {
		return sampledSeries(set(map[string]any{"hour": nil, "minute": nil, "duration": values["duration"], "start_date": values["start_date"]}))
	}
	repetition, notMapped := eventRepetitionOf(allDay(map[string]any{"start_date": "2026-10-02"}), friday, "")
	want := map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(2), "startsOn": "2026-10-02", "endsOn": nil}
	if got := asMap(t, repetition); !reflect.DeepEqual(got, want) || len(notMapped) > 0 {
		t.Errorf("an all-day series is %v, not mapped %v", got, notMapped)
	}
	if repetition, _ := eventRepetitionOf(allDay(map[string]any{"start_date": "2026-10-02", "duration": 2 * 86400}), friday, ""); repetition == nil {
		t.Error("a length of whole days is not taken")
	}
	for _, c := range []struct {
		values map[string]any
		want   []string
	}{
		{map[string]any{"start_date": "2026-10-09"}, []string{"start_date"}},
		{map[string]any{"start_date": "2026-10-02", "duration": 3600}, []string{"duration"}},
	} {
		if repetition, notMapped := eventRepetitionOf(allDay(c.values), friday, ""); repetition != nil || !reflect.DeepEqual(notMapped, c.want) {
			t.Errorf("%v: %+v, not mapped %v, want %v", c.values, repetition, notMapped, c.want)
		}
	}
}

// Every name of a zone a series can carry is one this program can read as a
// zone, so no name Minka takes is named as no zone here.
func TestEveryZoneNameMinkaTakesIsAZoneHere(t *testing.T) {
	for name := range zoneNamesMinkaTakes {
		if _, known := zoneOf(name); !known {
			t.Errorf("%s is a zone Minka takes and no zone here", name)
		}
	}
	if !zoneNamesMinkaTakes["Europe/Berlin"] || !zoneNamesMinkaTakes["Europe/Kiev"] || zoneNamesMinkaTakes["CEST"] {
		t.Error("the list of zones is not PHP's")
	}
}

type countingSource struct{ asked []string }

func (c *countingSource) Get(_ context.Context, path string) ([]byte, error) {
	c.asked = append(c.asked, path)
	return []byte(`[]`), nil
}

// A single day of a series and the entry of a series are addresses Basecamp
// answers by writing: no part of a run reaches them, whatever asks.
func TestARunNeverAsksForAnAddressThatWritesAtTheSource(t *testing.T) {
	source := &countingSource{}
	guarded := readsOnly{source}
	for _, path := range []string{
		"buckets/1/schedule_entries/2/occurrences/20261002.json",
		"buckets/1/schedule_entries/2.json",
		"buckets/1/schedule_entries/2/occurrences/20261002.json?page=1",
	} {
		_, err := guarded.Get(t.Context(), path)
		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API || !strings.Contains(failed.Message, path) {
			t.Errorf("%s: %v", path, err)
		}
	}
	if len(source.asked) > 0 {
		t.Errorf("Basecamp was asked for %v", source.asked)
	}
	if _, err := guarded.Get(t.Context(), "buckets/1/schedules/2/entries.json?page=1"); err != nil || len(source.asked) != 1 {
		t.Errorf("the list of a calendar is not asked for: %v, %v", err, source.asked)
	}
}
