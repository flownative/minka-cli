package basecamp_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// everyWeekList is the list of the second To-dos tool of the fake, which holds
// the to-do that repeats every Monday.
func everyWeekList(a *fakebasecamp.Account) *fakebasecamp.TodoList {
	return &a.Projects[7].Dock[1].TodoLists[0]
}

// withRepeatingTodo adds a to-do to the list of the second To-dos tool, written
// by whoever wrote the one already there.
func withRepeatingTodo(a *fakebasecamp.Account, id int64, dueOn string, schedule *fakebasecamp.Repetition) *fakebasecamp.Todo {
	list := everyWeekList(a)
	model := list.Todos[0]
	list.Todos = append(list.Todos, fakebasecamp.Todo{ID: id, Title: "Repeating", Creator: model.Creator,
		CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt, DueOn: dueOn, Repetition: schedule})
	return &list.Todos[len(list.Todos)-1]
}

func number(value int) *int { return &value }

// The account sends a schedule the way Basecamp does, or the tests below would
// hold the extractor against a shape no real account has: all ten fields, the
// days as a list even where they are empty, the time of day 0, and no key at all
// on a to-do that does not repeat.
func TestTheAccountSendsAScheduleOnlyOnAToDoThatRepeats(t *testing.T) {
	account := fakebasecamp.New()
	withRepeatingTodo(account, 9450, "2025-03-15", &fakebasecamp.Repetition{Frequency: "every_year", StartDate: "2025-03-15"})

	shown := todosTheAccountShows(t, account, 7008, 9405, false)
	if len(shown) != 2 {
		t.Fatalf("the account holds %v in the list of the second tool, and the test needs the two that repeat", shown)
	}
	for _, one := range shown {
		schedule, said := one["repetition_schedule"].(map[string]any)
		if !said {
			t.Fatalf("to-do %v sends no schedule", one["id"])
		}
		var fields []string
		for field := range schedule {
			fields = append(fields, field)
		}
		slices.Sort(fields)
		want := []string{"days", "duration", "end_date", "frequency", "hour", "minute", "month_interval", "start_date", "week_instance", "week_interval"}
		if !slices.Equal(fields, want) {
			t.Fatalf("the account sends a schedule of %v, want the %v Basecamp holds", fields, want)
		}
		if _, aList := schedule["days"].([]any); !aList || schedule["hour"] != float64(0) || schedule["minute"] != float64(0) {
			t.Errorf("the account sends days %v, hour %v and minute %v, and Basecamp sends a list and 0", schedule["days"], schedule["hour"], schedule["minute"])
		}
	}
	for _, one := range todosTheAccountShows(t, account, 7008, 9401, false) {
		if _, said := one["repetition_schedule"]; said {
			t.Fatalf("the account sends a schedule for to-do %v, which comes once: then the key is no test", one["id"])
		}
	}
}

// Each frequency the account was seen to hold becomes the rhythm Minka has for
// it, with the values it held: Sunday 0 becomes 7, the n-th weekday of the
// month its instance, and the day of the year the day the series starts on.
func TestEachFrequencyOfBasecampBecomesMinkasRhythm(t *testing.T) {
	due := "2026-10-02"
	cases := []struct {
		name     string
		dueOn    string
		schedule fakebasecamp.Repetition
		want     map[string]any
	}{
		{"every_week on a Friday", due, fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due},
			map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(1), "startsOn": due, "endsOn": nil}},
		{"every_week on Sunday and Friday, every other week", due,
			fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5, 0}, WeekInterval: number(2), StartDate: due},
			map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5), float64(7)}, "interval": float64(2), "startsOn": due, "endsOn": nil}},
		{"every_month on the first Friday", due, fakebasecamp.Repetition{Frequency: "every_month", Days: []int{5}, WeekInstance: number(1), StartDate: due},
			map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(5), "instance": float64(1), "interval": float64(1), "startsOn": due, "endsOn": nil}},
		{"every_month on the second Tuesday", "2026-10-13", fakebasecamp.Repetition{Frequency: "every_month", Days: []int{2}, WeekInstance: number(2), StartDate: "2026-10-13"},
			map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(2), "instance": float64(2), "interval": float64(1), "startsOn": "2026-10-13", "endsOn": nil}},
		{"every_month on the first Sunday", "2026-10-04", fakebasecamp.Repetition{Frequency: "every_month", Days: []int{0}, WeekInstance: number(1), StartDate: "2026-10-04"},
			map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(7), "instance": float64(1), "interval": float64(1), "startsOn": "2026-10-04", "endsOn": nil}},
		{"every_month without an instance, on the day it starts", "2026-10-17", fakebasecamp.Repetition{Frequency: "every_month", StartDate: "2026-10-17"},
			map[string]any{"rhythm": "monthlyOnDay", "day": float64(17), "interval": float64(1), "startsOn": "2026-10-17", "endsOn": nil}},
		{"every_month without an instance, on the day it names and starts", "2026-10-17",
			fakebasecamp.Repetition{Frequency: "every_month", Days: []int{17}, StartDate: "2026-10-17"},
			map[string]any{"rhythm": "monthlyOnDay", "day": float64(17), "interval": float64(1), "startsOn": "2026-10-17", "endsOn": nil}},
		{"every_day_of_month on the fifth of every third month", "2026-10-05",
			fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{5}, MonthInterval: number(3), StartDate: "2026-10-05"},
			map[string]any{"rhythm": "monthlyOnDay", "day": float64(5), "interval": float64(3), "startsOn": "2026-10-05", "endsOn": nil}},
		{"a schedule that ends on the day it starts", due, fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due, EndDate: due},
			map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(1), "startsOn": due, "endsOn": due}},
		{"every_day_of_month on the fifth", "2026-10-05", fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{5}, StartDate: "2026-10-05"},
			map[string]any{"rhythm": "monthlyOnDay", "day": float64(5), "interval": float64(1), "startsOn": "2026-10-05", "endsOn": nil}},
		{"custom_month on the first Monday of every other month", "2026-10-05",
			fakebasecamp.Repetition{Frequency: "custom_month", Days: []int{1}, WeekInstance: number(1), MonthInterval: number(2), StartDate: "2026-10-05"},
			map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(1), "instance": float64(1), "interval": float64(2), "startsOn": "2026-10-05", "endsOn": nil}},
		{"custom_month on the second Monday of every sixth month", "2026-10-12",
			fakebasecamp.Repetition{Frequency: "custom_month", Days: []int{1}, WeekInstance: number(2), MonthInterval: number(6), StartDate: "2026-10-12"},
			map[string]any{"rhythm": "monthlyOnWeekday", "weekday": float64(1), "instance": float64(2), "interval": float64(6), "startsOn": "2026-10-12", "endsOn": nil}},
		{"every_year on the day it starts", "2027-03-15", fakebasecamp.Repetition{Frequency: "every_year", StartDate: "2027-03-15"},
			map[string]any{"rhythm": "yearly", "month": float64(3), "day": float64(15), "startsOn": "2027-03-15", "endsOn": nil}},
		{"every_year on the day it starts, not the day it is due", "2027-03-15", fakebasecamp.Repetition{Frequency: "every_year", StartDate: "2027-03-14"},
			map[string]any{"rhythm": "yearly", "month": float64(3), "day": float64(14), "startsOn": "2027-03-14", "endsOn": nil}},
		{"every_year without a start, on the day it is due", "2027-02-28", fakebasecamp.Repetition{Frequency: "every_year"},
			map[string]any{"rhythm": "yearly", "month": float64(2), "day": float64(28), "endsOn": nil}},
		{"a schedule that runs out", due, fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due, EndDate: "2026-12-31"},
			map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)}, "interval": float64(1), "startsOn": due, "endsOn": "2026-12-31"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			account := fakebasecamp.New()
			withRepeatingTodo(account, 9450, c.dueOn, &c.schedule)

			dir, result := extract(t, account, todoProject)

			record := recordWithID(t, records(t, dir, bundle.Todos), "9450")
			if !reflect.DeepEqual(record["repetition"], c.want) {
				t.Errorf("repetition:\n got %v\nwant %v", record["repetition"], c.want)
			}
			if said, there := record["repetitionNotMapped"]; there {
				t.Errorf("a schedule that fits names %v as not mapped", said)
			}
			if result.TodosThatRepeat != 3 || result.TodosWithASeries != 3 || len(result.RepetitionsNotMapped) != 0 {
				t.Errorf("repeating %d, with a series %d, not mapped %v; want the three of the account, each with its series",
					result.TodosThatRepeat, result.TodosWithASeries, result.RepetitionsNotMapped)
			}
		})
	}
}

// What a series cannot hold is named by Basecamp's field and never holds the
// to-do back. A time of day is left off the series; everything else takes the
// series with it, so the to-do comes as one that does not repeat rather than on
// days nobody chose.
func TestWhatASeriesCannotHoldIsNamedAndNeverHoldsTheToDoBack(t *testing.T) {
	due := "2026-10-02"
	weekly := func(change func(*fakebasecamp.Repetition)) *fakebasecamp.Repetition {
		schedule := &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due}
		change(schedule)
		return schedule
	}
	cases := []struct {
		name        string
		dueOn       string
		schedule    *fakebasecamp.Repetition
		odd         map[string]any
		notMapped   []any
		seriesComes bool
	}{
		{"an hour", due, weekly(func(r *fakebasecamp.Repetition) { r.Hour = number(9) }), nil, []any{"hour"}, true},
		{"a minute", due, weekly(func(r *fakebasecamp.Repetition) { r.Minute = number(30) }), nil, []any{"minute"}, true},
		{"a duration", due, weekly(func(r *fakebasecamp.Repetition) { r.Duration = number(60) }), nil, []any{"duration"}, false},
		{"a time of day and a duration", due, weekly(func(r *fakebasecamp.Repetition) { r.Hour, r.Duration = number(9), number(60) }), nil,
			[]any{"hour", "duration"}, false},
		{"an end before the start", due, weekly(func(r *fakebasecamp.Repetition) { r.EndDate = "2026-10-01" }), nil, []any{"end_date"}, false},
		{"an end of no day", due, weekly(func(r *fakebasecamp.Repetition) { r.EndDate = "someday" }), nil, []any{"end_date"}, false},
		{"a start of no day", due, weekly(func(r *fakebasecamp.Repetition) { r.StartDate = "2026-10-2" }), nil, []any{"start_date"}, false},
		{"a start in the year 0", due, weekly(func(r *fakebasecamp.Repetition) { r.StartDate = "0000-01-01" }), nil, []any{"start_date"}, false},
		{"no due day", "", weekly(func(r *fakebasecamp.Repetition) { r.StartDate = "" }), nil, []any{"due_on"}, false},
		{"a frequency nobody saw", due, weekly(func(r *fakebasecamp.Repetition) { r.Frequency = "every_day" }), nil, []any{"frequency"}, false},
		{"no weekday", due, weekly(func(r *fakebasecamp.Repetition) { r.Days = nil }), nil, []any{"days"}, false},
		{"a weekday of no week", due, weekly(func(r *fakebasecamp.Repetition) { r.Days = []int{7} }), nil, []any{"days"}, false},
		{"a week interval Minka cannot count", due, weekly(func(r *fakebasecamp.Repetition) { r.WeekInterval = number(100) }), nil,
			[]any{"week_interval"}, false},
		{"an instance on a weekly rhythm", due, weekly(func(r *fakebasecamp.Repetition) { r.WeekInstance = number(1) }), nil,
			[]any{"week_instance"}, false},
		{"a month interval on a weekly rhythm", due, weekly(func(r *fakebasecamp.Repetition) { r.MonthInterval = number(2) }), nil,
			[]any{"month_interval"}, false},
		{"several weekdays of a month", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{2, 4}, WeekInstance: number(1), StartDate: due}, nil, []any{"days"}, false},
		{"several days without an instance", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{2, 4}, StartDate: due}, nil, []any{"days"}, false},
		{"no n-th weekday of a month", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{5}, WeekInstance: number(0), StartDate: due}, nil, []any{"week_instance"}, false},
		{"a weekday of a month after Saturday", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{7}, WeekInstance: number(1), StartDate: due}, nil, []any{"days"}, false},
		{"a weekday of a month before Sunday", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{-1}, WeekInstance: number(1), StartDate: due}, nil, []any{"days"}, false},
		{"a day of the month without an instance other than the first day's", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{5}, StartDate: due}, nil, []any{"days"}, false},
		{"day 0 of a month", due,
			&fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{0}, StartDate: due}, nil, []any{"days"}, false},
		{"a week interval on a day of the month", due,
			&fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{5}, WeekInterval: number(2), StartDate: due}, nil,
			[]any{"week_interval"}, false},
		{"an instance on a yearly rhythm", due,
			&fakebasecamp.Repetition{Frequency: "every_year", WeekInstance: number(1), StartDate: due}, nil, []any{"week_instance"}, false},
		{"a fifth weekday of a month", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{5}, WeekInstance: number(5), StartDate: due}, nil, []any{"week_instance"}, false},
		{"a last weekday said as Minka says it", due,
			&fakebasecamp.Repetition{Frequency: "custom_month", Days: []int{5}, WeekInstance: number(-1), MonthInterval: number(2), StartDate: due}, nil,
			[]any{"week_instance"}, false},
		{"a month interval Minka cannot count", due,
			&fakebasecamp.Repetition{Frequency: "custom_month", Days: []int{5}, WeekInstance: number(1), MonthInterval: number(0), StartDate: due}, nil,
			[]any{"month_interval"}, false},
		{"a week interval on a monthly rhythm", due,
			&fakebasecamp.Repetition{Frequency: "every_month", Days: []int{5}, WeekInstance: number(1), WeekInterval: number(2), StartDate: due}, nil,
			[]any{"week_interval"}, false},
		{"several days of a month", due,
			&fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{5, 20}, StartDate: due}, nil, []any{"days"}, false},
		{"a day of no month", due,
			&fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{32}, StartDate: due}, nil, []any{"days"}, false},
		{"an instance on a day of the month", due,
			&fakebasecamp.Repetition{Frequency: "every_day_of_month", Days: []int{5}, WeekInstance: number(1), StartDate: due}, nil,
			[]any{"week_instance"}, false},
		{"days of a year", due,
			&fakebasecamp.Repetition{Frequency: "every_year", Days: []int{5}, StartDate: due}, nil, []any{"days"}, false},
		{"a month interval on a yearly rhythm", due,
			&fakebasecamp.Repetition{Frequency: "every_year", MonthInterval: number(2), StartDate: due}, nil, []any{"month_interval"}, false},
		{"a schedule of another shape", due, nil,
			map[string]any{"frequency": "every_week", "days": []int{5}, "hour": "nine", "start_date": due}, []any{"repetition_schedule"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			account := fakebasecamp.New()
			todo := withRepeatingTodo(account, 9450, c.dueOn, c.schedule)
			todo.OddRepetition = c.odd

			dir, result := extract(t, account, todoProject)

			record := recordWithID(t, records(t, dir, bundle.Todos), "9450")
			if !reflect.DeepEqual(record["repetitionNotMapped"], c.notMapped) {
				t.Errorf("not mapped: got %v, want %v", record["repetitionNotMapped"], c.notMapped)
			}
			if _, carried := record["repetition"]; carried != c.seriesComes {
				t.Errorf("the series comes: %v, want %v (%v)", carried, c.seriesComes, record["repetition"])
			}
			if c.seriesComes && !reflect.DeepEqual(record["repetition"], map[string]any{"rhythm": "weekly", "weekdays": []any{float64(5)},
				"interval": float64(1), "startsOn": due, "endsOn": nil}) {
				t.Errorf("the series that comes without its time of day: %v", record["repetition"])
			}
			var want []basecamp.RepetitionNotMapped
			for _, part := range c.notMapped {
				want = append(want, basecamp.RepetitionNotMapped{Part: part.(string), SeriesComes: c.seriesComes, Todos: 1, SourceIDs: []string{"9450"}})
			}
			slices.SortFunc(want, func(a, b basecamp.RepetitionNotMapped) int {
				if a.Part < b.Part {
					return -1
				}
				return 1
			})
			if !reflect.DeepEqual(result.RepetitionsNotMapped, want) {
				t.Errorf("the report names\n%+v\nwant\n%+v", result.RepetitionsNotMapped, want)
			}
			withASeries := 2
			if c.seriesComes {
				withASeries = 3
			}
			if result.TodosThatRepeat != 3 || result.TodosWithASeries != withASeries {
				t.Errorf("repeating %d, with a series %d; want 3 and %d", result.TodosThatRepeat, result.TodosWithASeries, withASeries)
			}
		})
	}
}

// The report counts a part once per to-do and names every to-do that held it,
// apart for the to-dos whose series comes and those that come without one.
func TestTheReportNamesEachPartWithEveryToDoThatHeldIt(t *testing.T) {
	account := fakebasecamp.New()
	due := "2026-10-02"
	withRepeatingTodo(account, 9450, due, &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due, Hour: number(9)})
	withRepeatingTodo(account, 9452, due, &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due, Hour: number(8)})
	withRepeatingTodo(account, 9453, due, &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: due, Hour: number(9), Duration: number(30)})

	_, result := extract(t, account, todoProject)

	want := []basecamp.RepetitionNotMapped{
		{Part: "duration", SeriesComes: false, Todos: 1, SourceIDs: []string{"9453"}},
		{Part: "hour", SeriesComes: true, Todos: 2, SourceIDs: []string{"9450", "9452"}},
		{Part: "hour", SeriesComes: false, Todos: 1, SourceIDs: []string{"9453"}},
	}
	if !reflect.DeepEqual(result.RepetitionsNotMapped, want) {
		t.Errorf("the report names\n%+v\nwant\n%+v", result.RepetitionsNotMapped, want)
	}
}

// Only the newest occurrence at the source carries a schedule: an older one and
// one that is done come as to-dos that do not repeat, as they are at the source,
// and nothing ties them to the series. A done one that still carries its
// schedule carries it into the bundle, and Minka brings it without a series, so
// the report counts no series for it and a part it leaves off as costing one.
func TestOnlyTheOccurrenceWithASchedulesCarriesOne(t *testing.T) {
	account := fakebasecamp.New()
	list := everyWeekList(account)
	model := list.Todos[0]
	list.Todos = append(list.Todos,
		fakebasecamp.Todo{ID: 9450, Title: model.Title, Creator: model.Creator, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
			DueOn: "2025-03-31", Completion: &fakebasecamp.Completion{At: model.UpdatedAt, By: model.Creator}},
		fakebasecamp.Todo{ID: 9452, Title: model.Title, Creator: model.Creator, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
			DueOn: "2025-03-24"},
		fakebasecamp.Todo{ID: 9453, Title: "Done and still repeating", Creator: model.Creator, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
			DueOn: "2025-04-14", Completion: &fakebasecamp.Completion{At: model.UpdatedAt, By: model.Creator},
			Repetition: &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{1}, StartDate: "2025-04-14", Hour: number(9)}},
	)
	for _, one := range append(todosTheAccountShows(t, account, 7008, 9405, false), todosTheAccountShows(t, account, 7008, 9405, true)...) {
		if id := one["id"].(float64); (id == 9450 || id == 9452) && one["repetition_schedule"] != nil {
			t.Fatalf("the account sends a schedule for the older occurrence %v", id)
		}
	}

	dir, result := extract(t, account, todoProject)

	written := records(t, dir, bundle.Todos)
	for _, id := range []string{"9450", "9452"} {
		record := recordWithID(t, written, id)
		if _, said := record["repetition"]; said {
			t.Errorf("the older occurrence %s carries a series: %v", id, record["repetition"])
		}
		if _, said := record["repetitionNotMapped"]; said {
			t.Errorf("the older occurrence %s names parts not mapped: %v", id, record["repetitionNotMapped"])
		}
	}
	done := recordWithID(t, written, "9453")
	if done["repetition"] == nil || done["completedAt"] == nil {
		t.Errorf("the done to-do with a schedule: %v", done)
	}
	if result.TodosThatRepeat != 3 || result.TodosWithASeries != 2 {
		t.Errorf("repeating %d, with a series %d; want the three with a schedule, two of them open", result.TodosThatRepeat, result.TodosWithASeries)
	}
	want := []basecamp.RepetitionNotMapped{{Part: "hour", SeriesComes: false, Todos: 1, SourceIDs: []string{"9453"}}}
	if !reflect.DeepEqual(result.RepetitionsNotMapped, want) {
		t.Errorf("the report names\n%+v\nwant\n%+v", result.RepetitionsNotMapped, want)
	}
}

// A to-do that repeats on a To-dos tool itself stays left out and named, as any
// to-do there does: the bundle has no list to hang it or its series on.
func TestARepeatingToDoInNoListStaysLeftOut(t *testing.T) {
	account := fakebasecamp.New()
	todoSet(account).Todos[0].Repetition = &fakebasecamp.Repetition{Frequency: "every_week", Days: []int{5}, StartDate: "2026-10-02"}
	todoSet(account).Todos[0].DueOn = "2026-10-02"

	dir, result := extract(t, account, todoProject)

	if written := sourceIDs(records(t, dir, bundle.Todos)); slices.Contains(written, "9431") {
		t.Errorf("the to-do in no list came along: %v", written)
	}
	if !slices.Contains(result.TodosWithoutAList, basecamp.TodoWithoutAList{SourceID: "9431", ProjectSourceID: "7008"}) {
		t.Errorf("the to-do in no list is not named: %v", result.TodosWithoutAList)
	}
	if result.TodosThatRepeat != 2 || result.TodosWithASeries != 2 {
		t.Errorf("repeating %d, with a series %d; the one left out counts in neither", result.TodosThatRepeat, result.TodosWithASeries)
	}
}

// A to-do the source gives a start alone is due on that day in Minka, so its
// schedule has the due day a series needs.
func TestAStartAloneIsTheDueDayOfASeries(t *testing.T) {
	account := fakebasecamp.New()
	todo := withRepeatingTodo(account, 9450, "", &fakebasecamp.Repetition{Frequency: "every_year"})
	todo.StartsOn = "2027-03-15"

	dir, _ := extract(t, account, todoProject)

	record := recordWithID(t, records(t, dir, bundle.Todos), "9450")
	want := map[string]any{"rhythm": "yearly", "month": float64(3), "day": float64(15), "endsOn": nil}
	if !reflect.DeepEqual(record["repetition"], want) || record["repetitionNotMapped"] != nil {
		t.Errorf("repetition %v, not mapped %v; want %v", record["repetition"], record["repetitionNotMapped"], want)
	}
}
