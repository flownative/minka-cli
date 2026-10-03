package basecamp

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/bundle"
)

// repetitionSchedule is the schedule of a to-do or of a calendar entry that
// repeats, as Basecamp hands it out: always all ten fields, null the ones its
// frequency has no use for. Of a to-do only the newest occurrence of a series
// carries one, and nothing links it to the occurrences before it; an entry of a
// calendar is one record for all its days. Hour and Minute are a clock time in
// a zone the schedule does not name, Duration is seconds.
type repetitionSchedule struct {
	Frequency string `json:"frequency"`
	// Days are days of the week, Sunday 0, or of the month, by the frequency.
	Days []int `json:"days"`
	// WeekInstance is which of a weekday's days in the month, the n-th.
	WeekInstance  *int    `json:"week_instance"`
	WeekInterval  *int    `json:"week_interval"`
	MonthInterval *int    `json:"month_interval"`
	StartDate     *string `json:"start_date"`
	EndDate       *string `json:"end_date"`
	Duration      *int    `json:"duration"`
	Hour          *int    `json:"hour"`
	Minute        *int    `json:"minute"`
}

// The frequencies of Basecamp the extractor knows. The first four are a
// calendar's alone.
const (
	everyDay        = "every_day"
	everyWeekday    = "every_weekday"
	everyOtherWeek  = "every_other_week"
	customWeek      = "custom_week"
	everyWeek       = "every_week"
	everyMonth      = "every_month"
	customMonth     = "custom_month"
	everyDayOfMonth = "every_day_of_month"
	everyYear       = "every_year"
)

// maxRepetitionInterval is the largest interval a series of Minka takes.
const maxRepetitionInterval = 99

// scheduleReading collects the parts of a schedule that do not come along, each
// once. A part that is lost takes the series with it; one that is only dropped
// leaves the series without it.
type scheduleReading struct {
	notMapped []string
	lost      bool
}

func (r *scheduleReading) drop(part string) {
	if !slices.Contains(r.notMapped, part) {
		r.notMapped = append(r.notMapped, part)
	}
}

func (r *scheduleReading) lose(part string) {
	r.drop(part)
	r.lost = true
}

// repetitionOf is the series a to-do that repeats at the source brings, and the
// parts of its schedule the bundle does not carry, each named by Basecamp's
// field. dueOn is the day the to-do is due in Minka, nil where it has none.
//
// What is not mapped never holds the to-do back: a time of day is left off and
// the series still comes, because the days stay the same. Everything else that
// does not fit takes the series with it, and the to-do comes as one that does
// not repeat: a series on days the source never named would make to-dos nobody
// asked for, while one that is missing is named in the report. A part Basecamp
// was never seen to fill is such a part as well, rather than a guess at what it
// means.
func repetitionOf(raw json.RawMessage, dueOn *string) (*bundle.Repetition, []string) {
	var schedule repetitionSchedule
	if err := json.Unmarshal(raw, &schedule); err != nil {
		return nil, []string{"repetition_schedule"}
	}
	var reading scheduleReading
	for _, part := range []struct {
		name  string
		value *int
	}{{"hour", schedule.Hour}, {"minute", schedule.Minute}} {
		if part.value != nil && *part.value != 0 {
			reading.drop(part.name)
		}
	}
	if schedule.Duration != nil {
		reading.lose("duration")
	}

	// Minka makes a series from the to-do's due day, and a schedule on a to-do
	// without one stops its run.
	if dueOn == nil {
		reading.lose("due_on")
	}
	startsOn, endsOn, first := datesOf(schedule, dueOn, &reading)

	repetition := bundle.Repetition{StartsOn: startsOn, EndsOn: endsOn}
	switch schedule.Frequency {
	case everyWeek:
		repetition.Rhythm = bundle.RepeatsWeekly
		if weekdays, ok := weekdaysOfMinka(schedule.Days); ok {
			repetition.Weekdays = weekdays
		} else {
			reading.lose("days")
		}
		repetition.Interval = intervalOf(schedule.WeekInterval, "week_interval", &reading)
		unusedInstance(schedule, &reading)
		unusedInterval(schedule.MonthInterval, "month_interval", &reading)
	default:
		if !byTheMonthOrTheYear(schedule, first, &repetition, &reading) {
			reading.lose("frequency")
		}
	}

	if reading.lost {
		return nil, reading.notMapped
	}
	return &repetition, reading.notMapped
}

// datesOf are the first and the last day of a schedule as the bundle carries
// them, and first the day its series starts on: the start the schedule names,
// or fallback where it names none. An end before that day is a schedule no
// series can have.
func datesOf(schedule repetitionSchedule, fallback *string, reading *scheduleReading) (startsOn, endsOn, first *string) {
	startsOn = dayOfSchedule(schedule.StartDate)
	if schedule.StartDate != nil && strings.TrimSpace(*schedule.StartDate) != "" && startsOn == nil {
		reading.lose("start_date")
	}
	first = startsOn
	if first == nil {
		first = fallback
	}
	endsOn = dayOfSchedule(schedule.EndDate)
	if schedule.EndDate != nil && strings.TrimSpace(*schedule.EndDate) != "" && endsOn == nil {
		reading.lose("end_date")
	}
	if endsOn != nil && first != nil && *endsOn < *first {
		reading.lose("end_date")
	}
	return startsOn, endsOn, first
}

// byTheMonthOrTheYear maps the rhythms a to-do and an event share, those by the
// month and by the year, and says whether the frequency is one of them. first is
// the day the series starts on.
func byTheMonthOrTheYear(schedule repetitionSchedule, first *string, repetition *bundle.Repetition, reading *scheduleReading) bool {
	switch schedule.Frequency {
	case everyMonth, customMonth:
		repetition.Interval = intervalOf(schedule.MonthInterval, "month_interval", reading)
		unusedInterval(schedule.WeekInterval, "week_interval", reading)
		if schedule.WeekInstance == nil {
			// The day of the month is the day the series starts on. A day in
			// days that says another is not guessed between.
			repetition.Rhythm = bundle.RepeatsMonthlyOnDay
			if day, ok := firstDayOf(first); ok {
				repetition.Day = day.Day()
			}
			if len(schedule.Days) > 1 || (len(schedule.Days) == 1 && schedule.Days[0] != repetition.Day) {
				reading.lose("days")
			}
			return true
		}
		repetition.Rhythm = bundle.RepeatsMonthlyOnWeekday
		// How Basecamp says "the last" was never seen, so only the first to the
		// fourth are taken.
		if instance := *schedule.WeekInstance; instance >= 1 && instance <= 4 {
			repetition.Instance = instance
		} else {
			reading.lose("week_instance")
		}
		if len(schedule.Days) == 1 && schedule.Days[0] >= 0 && schedule.Days[0] <= 6 {
			repetition.Weekday = weekdayOfMinka(schedule.Days[0])
		} else {
			reading.lose("days")
		}
	case everyDayOfMonth:
		repetition.Rhythm = bundle.RepeatsMonthlyOnDay
		if len(schedule.Days) == 1 && schedule.Days[0] >= 1 && schedule.Days[0] <= 31 {
			repetition.Day = schedule.Days[0]
		} else {
			reading.lose("days")
		}
		repetition.Interval = intervalOf(schedule.MonthInterval, "month_interval", reading)
		unusedInstance(schedule, reading)
		unusedInterval(schedule.WeekInterval, "week_interval", reading)
	case everyYear:
		// The day of the year is the day the series starts on.
		repetition.Rhythm = bundle.RepeatsYearly
		if day, ok := firstDayOf(first); ok {
			repetition.Month, repetition.Day = int(day.Month()), day.Day()
		}
		if len(schedule.Days) > 0 {
			reading.lose("days")
		}
		unusedInstance(schedule, reading)
		unusedInterval(schedule.WeekInterval, "week_interval", reading)
		unusedInterval(schedule.MonthInterval, "month_interval", reading)
	default:
		return false
	}
	return true
}

// eventTimes is what the record of an event says of its first occurrence: the
// first day of an all-day one, the two moments of a timed one.
type eventTimes struct {
	allDay           bool
	startsOn         string
	startsAt, endsAt time.Time
}

const secondsPerDay = 24 * 60 * 60

// eventRepetitionOf is the schedule an event that repeats at the source brings,
// and the parts of it the bundle does not carry, each named by Basecamp's field.
// zoneName is the zone of whoever made the entry, which Basecamp does not name
// on the schedule itself.
//
// The rhythms by the month and by the year are a to-do's. Any part that does
// not fit takes the series with it and the event comes once, at the time of its
// record: every occurrence of a series is shown, so one on a day the source
// never had would be a meeting nobody called, while one that is missing is named
// in the report. That is why the schedule is held against the record before it
// is written: the clock time it names has to be the record's own in the zone,
// its first day the record's first day, and that day one of its days.
func eventRepetitionOf(raw json.RawMessage, times eventTimes, zoneName string) (*bundle.Repetition, []string) {
	var schedule repetitionSchedule
	if err := json.Unmarshal(raw, &schedule); err != nil {
		return nil, []string{"recurrence_schedule"}
	}
	var reading scheduleReading
	// The first day of an event's series is the first day of the event, and
	// only a schedule that says so can be proven to be the event's.
	if schedule.StartDate == nil || strings.TrimSpace(*schedule.StartDate) == "" {
		reading.lose("start_date")
	}
	startsOn, endsOn, first := datesOf(schedule, nil, &reading)
	repetition := bundle.Repetition{StartsOn: startsOn, EndsOn: endsOn}

	if times.allDay {
		if startsOn != nil && *startsOn != times.startsOn {
			reading.lose("start_date")
		}
		if schedule.Duration != nil && (*schedule.Duration <= 0 || *schedule.Duration%secondsPerDay != 0) {
			reading.lose("duration")
		}
	} else {
		if zone, known := zoneOf(zoneName); !known {
			reading.lose("time_zone")
		} else {
			repetition.TimeZone = &zoneName
			local := times.startsAt.In(zone)
			if startsOn != nil && (local.Format(dayFormat) != *startsOn || schedule.Hour == nil || schedule.Minute == nil ||
				local.Hour() != *schedule.Hour || local.Minute() != *schedule.Minute) {
				reading.lose("hour")
			}
		}
		if schedule.Duration != nil && time.Duration(*schedule.Duration)*time.Second != times.endsAt.Sub(times.startsAt) {
			reading.lose("duration")
		}
	}

	switch schedule.Frequency {
	case everyDay, everyWeekday:
		repetition.Rhythm = bundle.RepeatsDaily
		switch {
		case schedule.Frequency == everyWeekday:
			repetition.Weekdays = []int{1, 2, 3, 4, 5}
		case len(schedule.Days) == 0:
			repetition.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
		default:
			if weekdays, ok := weekdaysOfMinka(schedule.Days); ok {
				repetition.Weekdays = weekdays
			} else {
				reading.lose("days")
			}
		}
		unusedInstance(schedule, &reading)
		unusedInterval(schedule.WeekInterval, "week_interval", &reading)
		unusedInterval(schedule.MonthInterval, "month_interval", &reading)
	case everyWeek, everyOtherWeek, customWeek:
		repetition.Rhythm = bundle.RepeatsWeekly
		switch schedule.Frequency {
		case everyWeek:
			repetition.Interval = 1
			unusedInterval(schedule.WeekInterval, "week_interval", &reading)
		case everyOtherWeek:
			repetition.Interval = 2
			if schedule.WeekInterval != nil && *schedule.WeekInterval != 2 {
				reading.lose("week_interval")
			}
		default:
			repetition.Interval = intervalOf(schedule.WeekInterval, "week_interval", &reading)
		}
		repetition.Weekdays = weekdaysOfAWeek(schedule.Days, first, &reading)
		// Where Basecamp's weeks turn was never seen, so a rhythm of every other
		// week and beyond is taken on one weekday alone, where it cannot matter.
		if repetition.Interval > 1 && len(repetition.Weekdays) > 1 {
			reading.lose("days")
		}
		unusedInstance(schedule, &reading)
		unusedInterval(schedule.MonthInterval, "month_interval", &reading)
	default:
		if !byTheMonthOrTheYear(schedule, first, &repetition, &reading) {
			reading.lose("frequency")
		}
	}
	theFirstDayIsOneOfItsDays(repetition, first, &reading)

	if reading.lost {
		return nil, reading.notMapped
	}
	return &repetition, reading.notMapped
}

// weekdaysOfAWeek are the weekdays a weekly rhythm names, and the weekday of
// the first day where it names none.
func weekdaysOfAWeek(days []int, first *string, reading *scheduleReading) []int {
	if len(days) == 0 {
		if day, ok := firstDayOf(first); ok {
			return []int{weekdayOfMinka(int(day.Weekday()))}
		}
		reading.lose("days")
		return nil
	}
	weekdays, ok := weekdaysOfMinka(days)
	if !ok {
		reading.lose("days")
	}
	return weekdays
}

// theFirstDayIsOneOfItsDays holds a schedule to the day its series starts on:
// Minka refuses an event whose schedule does not fall on its first day. Only
// what the schedule names is held against it; a part that is missing is named
// where it went missing.
func theFirstDayIsOneOfItsDays(repetition bundle.Repetition, first *string, reading *scheduleReading) {
	day, ok := firstDayOf(first)
	if !ok {
		return
	}
	weekday := weekdayOfMinka(int(day.Weekday()))
	switch repetition.Rhythm {
	case bundle.RepeatsDaily, bundle.RepeatsWeekly:
		if len(repetition.Weekdays) > 0 && !slices.Contains(repetition.Weekdays, weekday) {
			reading.lose("days")
		}
	case bundle.RepeatsMonthlyOnDay:
		if repetition.Day != 0 && repetition.Day != day.Day() {
			reading.lose("days")
		}
	case bundle.RepeatsMonthlyOnWeekday:
		if repetition.Weekday != 0 && repetition.Weekday != weekday {
			reading.lose("days")
		}
		if repetition.Instance != 0 && repetition.Instance != (day.Day()-1)/7+1 {
			reading.lose("week_instance")
		}
	}
}

// zoneOf is the zone a name names, where it is one Minka takes and the zone
// database of this program knows it.
func zoneOf(name string) (*time.Location, bool) {
	if !zoneNamesMinkaTakes[name] {
		return nil, false
	}
	zone, err := time.LoadLocation(name)
	return zone, err == nil
}

// intervalOf is the interval a field counts in, 1 where it names none. One
// Minka cannot count in takes the series with it: every week instead of every
// hundredth would make to-dos nobody asked for.
func intervalOf(value *int, field string, reading *scheduleReading) int {
	if value == nil {
		return 1
	}
	if *value < 1 || *value > maxRepetitionInterval {
		reading.lose(field)
		return 0
	}
	return *value
}

// unusedInterval takes the series with it where a frequency that counts in
// something else names an interval other than every one: it would say what the
// frequency does not.
func unusedInterval(value *int, field string, reading *scheduleReading) {
	if value != nil && *value != 1 {
		reading.lose(field)
	}
}

// unusedInstance takes the series with it where a frequency that has no n-th
// weekday names one.
func unusedInstance(schedule repetitionSchedule, reading *scheduleReading) {
	if schedule.WeekInstance != nil {
		reading.lose("week_instance")
	}
}

// dayOfSchedule is the day a field of a schedule names, nil where it names none
// or none of the shape YYYY-MM-DD.
func dayOfSchedule(value *string) *string {
	if value == nil {
		return nil
	}
	if _, ok := aDay(*value); !ok {
		return nil
	}
	day := *value
	return &day
}

func firstDayOf(value *string) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	return aDay(*value)
}

// RepetitionNotMapped is a part of the source's schedules the bundle does not
// carry, by Basecamp's field, and the to-dos whose schedule held it.
// SeriesComes tells the to-dos whose series comes without the part from those
// that come as ones that do not repeat.
type RepetitionNotMapped struct {
	Part        string   `json:"part"`
	SeriesComes bool     `json:"seriesComes"`
	Todos       int      `json:"todos"`
	SourceIDs   []string `json:"sourceIds"`
}

// noteRepetitionNotMapped counts a to-do under each part of its schedule the
// bundle does not carry.
func (e *Extractor) noteRepetitionNotMapped(id int64, parts []string, seriesComes bool) {
	for _, part := range parts {
		found := false
		for i := range e.repetitionsNotMapped {
			one := &e.repetitionsNotMapped[i]
			if one.Part == part && one.SeriesComes == seriesComes {
				one.Todos++
				one.SourceIDs = append(one.SourceIDs, sourceID(id))
				found = true
				break
			}
		}
		if !found {
			e.repetitionsNotMapped = append(e.repetitionsNotMapped,
				RepetitionNotMapped{Part: part, SeriesComes: seriesComes, Todos: 1, SourceIDs: []string{sourceID(id)}})
		}
	}
	sort.SliceStable(e.repetitionsNotMapped, func(i, j int) bool {
		a, b := e.repetitionsNotMapped[i], e.repetitionsNotMapped[j]
		if a.Part != b.Part {
			return a.Part < b.Part
		}
		return a.SeriesComes && !b.SeriesComes
	})
}
