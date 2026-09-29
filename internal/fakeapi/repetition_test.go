package fakeapi

import (
	"slices"
	"testing"
)

// The cases below are Minka's own RepetitionScheduleTest, day for day, so
// that the port reckons as the server does.

func mustSchedule(t *testing.T, schedule RepetitionSchedule, err error) RepetitionSchedule {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return schedule
}

// occurrences are the first days of a schedule from the year 2000 on.
func occurrences(schedule RepetitionSchedule, count int) []string {
	var days []string
	day := "2000-01-01"
	for len(days) < count {
		next, ok := schedule.nextOccurrenceAfter(day)
		if !ok {
			break
		}
		days, day = append(days, next), next
	}
	return days
}

func expectDays(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: %v, want %v", name, got, want)
	}
}

func TestTheDaysOfEachRhythmAreTheOnesMinkaReckons(t *testing.T) {
	workdays := []int{1, 2, 3, 4, 5}
	for _, c := range []struct {
		name     string
		schedule func() (RepetitionSchedule, error)
		count    int
		want     []string
	}{
		{"daily falls on each chosen weekday", func() (RepetitionSchedule, error) { return dailySchedule(workdays, "2026-10-01", "") },
			4, []string{"2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"}},
		{"weekly falls on the weekday of every week", func() (RepetitionSchedule, error) { return weeklySchedule([]int{5}, 1, "2026-10-01", "") },
			3, []string{"2026-10-02", "2026-10-09", "2026-10-16"}},
		{"an interval of weeks counts from the week of the first matching day", func() (RepetitionSchedule, error) {
			return weeklySchedule([]int{4, 2}, 2, "2026-10-01", "")
		}, 4, []string{"2026-10-01", "2026-10-13", "2026-10-15", "2026-10-27"}},
		{"the 31st falls on the last day of a shorter month", func() (RepetitionSchedule, error) { return monthlyOnDaySchedule(31, 1, "2026-01-31", "") },
			4, []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}},
		{"an interval of months counts from the first matching month", func() (RepetitionSchedule, error) {
			return monthlyOnDaySchedule(15, 3, "2026-01-20", "")
		}, 3, []string{"2026-02-15", "2026-05-15", "2026-08-15"}},
		{"the first Friday of every month", func() (RepetitionSchedule, error) { return monthlyOnWeekdaySchedule(5, 1, 1, "2026-10-02", "") },
			3, []string{"2026-10-02", "2026-11-06", "2026-12-04"}},
		{"the fourth weekday is never the fifth", func() (RepetitionSchedule, error) { return monthlyOnWeekdaySchedule(5, 4, 1, "2026-10-01", "") },
			2, []string{"2026-10-23", "2026-11-27"}},
		{"the last Friday is the fifth where a month has one", func() (RepetitionSchedule, error) {
			return monthlyOnWeekdaySchedule(5, LastWeekdayInstance, 1, "2026-10-01", "")
		}, 3, []string{"2026-10-30", "2026-11-27", "2026-12-25"}},
		{"the last Thursday", func() (RepetitionSchedule, error) {
			return monthlyOnWeekdaySchedule(4, LastWeekdayInstance, 1, "2026-11-01", "")
		}, 2, []string{"2026-11-26", "2026-12-31"}},
		{"a weekday of every other month", func() (RepetitionSchedule, error) { return monthlyOnWeekdaySchedule(5, 1, 2, "2026-10-03", "") },
			3, []string{"2026-11-06", "2027-01-01", "2027-03-05"}},
		{"yearly falls on its day once a year", func() (RepetitionSchedule, error) { return yearlySchedule(3, 15, "2026-06-01", "") },
			2, []string{"2027-03-15", "2028-03-15"}},
		{"29 February falls on the 28th outside a leap year", func() (RepetitionSchedule, error) { return yearlySchedule(2, 29, "2028-02-29", "") },
			5, []string{"2028-02-29", "2029-02-28", "2030-02-28", "2031-02-28", "2032-02-29"}},
	} {
		schedule, err := c.schedule()
		expectDays(t, c.name, occurrences(mustSchedule(t, schedule, err), c.count), c.want)
	}
}

func TestASeriesStartingLaterIsSearchedFromItsStart(t *testing.T) {
	schedule, err := dailySchedule([]int{1}, "2026-10-01", "")
	if next, _ := mustSchedule(t, schedule, err).nextOccurrenceAfter("2026-01-01"); next != "2026-10-05" {
		t.Fatalf("next %s", next)
	}
}

// A start centuries back counts its weeks all the same; the search asks from a
// day near today, as it does for a series that fell far behind.
func TestAnIntervalOfWeeksCountsFromAStartCenturiesAgo(t *testing.T) {
	for interval, want := range map[int]string{2: "2026-10-09", 3: "2026-10-02"} {
		schedule, err := weeklySchedule([]int{5}, interval, "1700-01-01", "")
		if next, _ := mustSchedule(t, schedule, err).nextOccurrenceAfter("2026-09-30"); next != want {
			t.Errorf("every %d weeks from 1700: %s, want %s", interval, next, want)
		}
	}
}

func TestNothingFallsAfterTheEndDate(t *testing.T) {
	schedule, err := weeklySchedule([]int{5}, 1, "2026-10-01", "2026-10-09")
	schedule = mustSchedule(t, schedule, err)
	if next, ok := schedule.nextOccurrenceAfter("2026-10-02"); !ok || next != "2026-10-09" {
		t.Errorf("the end day itself still counts: %s %v", next, ok)
	}
	if next, ok := schedule.nextOccurrenceAfter("2026-10-09"); ok {
		t.Errorf("after the end: %s", next)
	}
}

func TestAScheduleThatCannotHoldIsRefused(t *testing.T) {
	for name, attempt := range map[string]func() (RepetitionSchedule, error){
		"no weekday":              func() (RepetitionSchedule, error) { return dailySchedule(nil, "2026-10-01", "") },
		"no interval":             func() (RepetitionSchedule, error) { return weeklySchedule([]int{5}, 0, "2026-10-01", "") },
		"an interval of 100":      func() (RepetitionSchedule, error) { return weeklySchedule([]int{5}, 100, "2026-10-01", "") },
		"day 32":                  func() (RepetitionSchedule, error) { return monthlyOnDaySchedule(32, 1, "2026-10-01", "") },
		"30 February":             func() (RepetitionSchedule, error) { return yearlySchedule(2, 30, "2026-10-01", "") },
		"31 April":                func() (RepetitionSchedule, error) { return yearlySchedule(4, 31, "2026-10-01", "") },
		"month 13":                func() (RepetitionSchedule, error) { return yearlySchedule(13, 1, "2026-10-01", "") },
		"no date":                 func() (RepetitionSchedule, error) { return monthlyOnDaySchedule(1, 1, "2026-02-30", "") },
		"ending before the start": func() (RepetitionSchedule, error) { return monthlyOnDaySchedule(1, 1, "2026-10-01", "2026-09-30") },
	} {
		if _, err := attempt(); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if _, err := yearlySchedule(2, 29, "2026-10-01", ""); err != nil {
		t.Errorf("29 February is a day of the year: %v", err)
	}
}

// Minka asks occurrenceToShow for the round, which the fake does not run; the
// day it answers there is the upcoming one wherever the lead has come, so its
// cases hold for upcomingOccurrence too.
func TestTheUpcomingDayIsTheLatestOwedAndComesAfterAResume(t *testing.T) {
	weekly, err := weeklySchedule([]int{5}, 1, "2026-09-01", "")
	weekly = mustSchedule(t, weekly, err)
	if next, _ := weekly.upcomingOccurrence("2026-09-04", "", "2026-10-01"); next != "2026-09-25" {
		t.Errorf("a series that fell behind owes the latest day that has passed, not %s", next)
	}
	if next, _ := weekly.upcomingOccurrence("2026-09-04", "2026-10-03", "2026-10-03"); next != "2026-10-09" {
		t.Errorf("a resumed series continues after the day it was resumed, not on %s", next)
	}

	monthly, err := monthlyOnWeekdaySchedule(5, 1, 1, "2026-10-02", "")
	monthly = mustSchedule(t, monthly, err)
	if next, _ := monthly.upcomingOccurrence("", "", "2026-09-25"); next != "2026-10-02" {
		t.Errorf("the first occurrence is searched from the start, not %s", next)
	}
	if next, _ := monthly.upcomingOccurrence("2026-10-02", "", "2026-10-30"); next != "2026-11-06" || monthly.showsFrom(next, 7, "2026-10-02") != "2026-10-30" {
		t.Errorf("the next one after the first Friday: %s, showing from %s", next, monthly.showsFrom(next, 7, "2026-10-02"))
	}
}

func TestShowNowMakesTheNextDayNotYetMadeButNoneWhileOneIsAhead(t *testing.T) {
	daily, err := dailySchedule([]int{1, 2, 3, 4, 5}, "2026-10-01", "")
	daily = mustSchedule(t, daily, err)

	if day, ok := daily.occurrenceToShowNow("2026-10-06", "", "2026-10-06"); !ok || day != "2026-10-07" {
		t.Errorf("show now on the day of the last one: %s %v", day, ok)
	}
	if day, ok := daily.occurrenceToShowNow("2026-10-06", "", "2026-10-05"); ok {
		t.Errorf("a series holds at most one occurrence not yet due, and made %s", day)
	}
	if day, _ := daily.upcomingOccurrence("2026-10-06", "", "2026-10-05"); day != "2026-10-07" {
		t.Errorf("the line still names the day after the one ahead, not %s", day)
	}
	if day, _ := daily.occurrenceToShowNow("", "", "2026-09-20"); day != "2026-10-01" {
		t.Errorf("before the first day: %s", day)
	}
	ended, err := weeklySchedule([]int{5}, 1, "2026-10-01", "2026-10-02")
	if day, ok := mustSchedule(t, ended, err).occurrenceToShowNow("2026-10-02", "", "2026-10-02"); ok {
		t.Errorf("past its end a series shows nothing, and made %s", day)
	}
}

func TestAnOccurrenceShowsFromItsLeadButNotBeforeTheOneMadeLast(t *testing.T) {
	daily, err := dailySchedule([]int{1, 2, 3, 4, 5}, "2026-10-01", "")
	daily = mustSchedule(t, daily, err)
	if day := daily.showsFrom("2026-10-02", 7, ""); day != "2026-09-25" {
		t.Errorf("the lead ahead: %s", day)
	}
	if day := daily.showsFrom("2026-10-07", 7, "2026-10-06"); day != "2026-10-06" {
		t.Errorf("not before the one made last: %s", day)
	}
}

// What a request leaves out comes from the stored schedule while rhythm and
// first day stay, and from the first day otherwise, as Minka's
// RepetitionScheduleFields takes it.
func TestTheFieldsLeftOutComeFromTheStoredScheduleOrTheFirstDay(t *testing.T) {
	number := func(n int) *int { return &n }
	text := func(s string) *string { return &s }
	stored, err := monthlyOnDaySchedule(20, 1, "2030-01-04", "")
	stored = mustSchedule(t, stored, err)

	kept, err := scheduleFields{interval: number(2)}.scheduleFrom("2030-01-04", &stored)
	if err != nil || kept.DayOfMonth != 20 || kept.Interval != 2 {
		t.Errorf("an interval alone keeps the day: %+v %v", kept, err)
	}
	moved, err := scheduleFields{startsOn: text("2030-01-10")}.scheduleFrom("2030-01-10", &kept)
	if err != nil || moved.DayOfMonth != 10 || moved.Interval != 2 {
		t.Errorf("a new first day brings its own day and keeps the interval: %+v %v", moved, err)
	}
	fresh, err := scheduleFields{rhythm: text(RepeatsMonthlyOnWeekday)}.scheduleFrom("2030-01-31", &stored)
	if err != nil || fresh.Weekdays[0] != 4 || fresh.Instance != LastWeekdayInstance || fresh.Interval != 1 {
		t.Errorf("a day after the 28th is the last of its weekday, and a new rhythm starts at every month: %+v %v", fresh, err)
	}
	daily, err := scheduleFields{rhythm: text(RepeatsDaily)}.scheduleFrom("2030-01-05", nil)
	if err != nil || !slices.Equal(daily.Weekdays, []int{1, 2, 3, 4, 5}) {
		t.Errorf("a new daily series falls on the workdays: %+v %v", daily, err)
	}
	weekly, err := scheduleFields{}.scheduleFrom("2030-01-05", nil)
	if err != nil || weekly.Rhythm != RepeatsWeekly || !slices.Equal(weekly.Weekdays, []int{6}) {
		t.Errorf("without a rhythm a series is weekly on the weekday of its first day: %+v %v", weekly, err)
	}
	yearly, err := scheduleFields{rhythm: text(RepeatsYearly), weekdays: []int{1}, month: number(4), day: number(31)}.scheduleFrom("2030-01-05", nil)
	if err == nil {
		t.Errorf("31 April was taken: %+v", yearly)
	}
	ignoring, err := scheduleFields{rhythm: text(RepeatsMonthlyOnDay), weekdays: []int{1, 2}, month: number(3)}.scheduleFrom("2030-01-04", nil)
	if err != nil || ignoring.DayOfMonth != 4 || ignoring.Month != 0 || len(ignoring.Weekdays) != 0 {
		t.Errorf("a rhythm ignores the fields it does not use: %+v %v", ignoring, err)
	}
}
