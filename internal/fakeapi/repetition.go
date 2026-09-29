package fakeapi

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The rhythms a to-do series repeats in, as Minka names them.
const (
	RepeatsDaily            = "daily"            // on each of the chosen weekdays
	RepeatsWeekly           = "weekly"           // on the chosen weekdays of every n-th week
	RepeatsMonthlyOnDay     = "monthlyOnDay"     // on one day of every n-th month
	RepeatsMonthlyOnWeekday = "monthlyOnWeekday" // on the first to fourth or the last of one weekday of every n-th month
	RepeatsYearly           = "yearly"           // on one day of one month
)

// LastWeekdayInstance is the last of a weekday in a month: the fifth where the
// month has one.
const LastWeekdayInstance = -1

// maxRepetitionInterval keeps the search for the next day short.
const maxRepetitionInterval = 99

const dayLayout = "2006-01-02"

// RepetitionSchedule is on which calendar days a to-do series falls, as
// Minka's RepetitionSchedule reckons them: days carry no time and no zone, an
// interval counts from the first matching day on or after the start, a day
// past the end of a month falls on its last day and 29 February on the 28th.
// A field the rhythm does not use is its zero value.
type RepetitionSchedule struct {
	Rhythm     string
	Weekdays   []int // ISO weekdays in the order of the week, each once; the one weekday of monthlyOnWeekday
	Interval   int   // in weeks or months, 1 for every one
	DayOfMonth int
	Instance   int // 1 to 4, or LastWeekdayInstance
	Month      int
	StartsOn   string
	EndsOn     string // the last day an occurrence may fall on, empty for none
}

var errNoSchedule = errors.New("the fields make no schedule")

func newRepetitionSchedule(schedule RepetitionSchedule) (RepetitionSchedule, error) {
	if schedule.Interval < 1 || schedule.Interval > maxRepetitionInterval {
		return RepetitionSchedule{}, errNoSchedule
	}
	start, ok := midnightOf(schedule.StartsOn)
	if !ok {
		return RepetitionSchedule{}, errNoSchedule
	}
	if schedule.EndsOn != "" {
		end, ok := midnightOf(schedule.EndsOn)
		if !ok || end.Before(start) {
			return RepetitionSchedule{}, errNoSchedule
		}
	}
	return schedule, nil
}

func dailySchedule(weekdays []int, startsOn, endsOn string) (RepetitionSchedule, error) {
	days, err := someWeekdays(weekdays)
	if err != nil {
		return RepetitionSchedule{}, err
	}
	return newRepetitionSchedule(RepetitionSchedule{Rhythm: RepeatsDaily, Weekdays: days, Interval: 1, StartsOn: startsOn, EndsOn: endsOn})
}

func weeklySchedule(weekdays []int, interval int, startsOn, endsOn string) (RepetitionSchedule, error) {
	days, err := someWeekdays(weekdays)
	if err != nil {
		return RepetitionSchedule{}, err
	}
	return newRepetitionSchedule(RepetitionSchedule{Rhythm: RepeatsWeekly, Weekdays: days, Interval: interval, StartsOn: startsOn, EndsOn: endsOn})
}

func monthlyOnDaySchedule(dayOfMonth, interval int, startsOn, endsOn string) (RepetitionSchedule, error) {
	if dayOfMonth < 1 || dayOfMonth > 31 {
		return RepetitionSchedule{}, errNoSchedule
	}
	return newRepetitionSchedule(RepetitionSchedule{Rhythm: RepeatsMonthlyOnDay, Interval: interval, DayOfMonth: dayOfMonth, StartsOn: startsOn, EndsOn: endsOn})
}

func monthlyOnWeekdaySchedule(weekday, instance, interval int, startsOn, endsOn string) (RepetitionSchedule, error) {
	return newRepetitionSchedule(RepetitionSchedule{Rhythm: RepeatsMonthlyOnWeekday, Weekdays: []int{weekday}, Interval: interval, Instance: instance, StartsOn: startsOn, EndsOn: endsOn})
}

func yearlySchedule(month, dayOfMonth int, startsOn, endsOn string) (RepetitionSchedule, error) {
	// Held against a leap year, so that 29 February is a day of the year.
	if month < 1 || month > 12 || dayOfMonth < 1 || dayOfMonth > daysIn(2000, month) {
		return RepetitionSchedule{}, errNoSchedule
	}
	return newRepetitionSchedule(RepetitionSchedule{Rhythm: RepeatsYearly, Interval: 1, DayOfMonth: dayOfMonth, Month: month, StartsOn: startsOn, EndsOn: endsOn})
}

// someWeekdays are the weekdays each once in the order of the week; none at
// all make no schedule.
func someWeekdays(weekdays []int) ([]int, error) {
	unique := slices.Clone(weekdays)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	if len(unique) == 0 {
		return nil, errNoSchedule
	}
	return unique, nil
}

// nextOccurrenceAfter is the first day of the series after the given one, and
// false past its end.
func (r RepetitionSchedule) nextOccurrenceAfter(day string) (string, bool) {
	from := addDays(day, 1)
	if r.StartsOn > from {
		from = r.StartsOn
	}
	var next string
	switch r.Rhythm {
	case RepeatsDaily, RepeatsWeekly:
		next = r.nextWeekdayOnOrAfter(from)
	default:
		next = r.nextMonthlyOnOrAfter(from)
	}
	if r.EndsOn != "" && next > r.EndsOn {
		return "", false
	}
	return next, true
}

// occurrenceToShowNow is the occurrence "Show now" makes, and false past the
// end and while the series holds one after today.
func (r RepetitionSchedule) occurrenceToShowNow(lastOccursOn, resumedOn, today string) (string, bool) {
	if lastOccursOn != "" && today < lastOccursOn {
		return "", false
	}
	return r.upcomingOccurrence(lastOccursOn, resumedOn, today)
}

// upcomingOccurrence is the next day not yet made, and false past the end. A
// series that fell behind owes the latest of the days that have passed, not
// each of them.
func (r RepetitionSchedule) upcomingOccurrence(lastOccursOn, resumedOn, today string) (string, bool) {
	after := max(lastOccursOn, resumedOn)
	if after == "" {
		after = addDays(r.StartsOn, -1)
	}
	next, ok := r.nextOccurrenceAfter(after)
	for ok {
		later, more := r.nextOccurrenceAfter(next)
		if !more || later > today {
			break
		}
		next = later
	}
	return next, ok
}

// showsFrom is the first day the round makes the occurrence of the given day:
// its lead ahead, and not before the day of the occurrence made last.
func (r RepetitionSchedule) showsFrom(occursOn string, leadDays int, lastOccursOn string) string {
	return max(addDays(occursOn, -leadDays), lastOccursOn)
}

func (r RepetitionSchedule) nextWeekdayOnOrAfter(from string) string {
	firstWeek := mondayOf(r.firstMatchingWeekday())
	day, _ := midnightOf(from)
	for ahead := 0; ahead < 7*(r.Interval+1); ahead++ {
		date := day.AddDate(0, 0, ahead)
		// Counted in seconds, as Minka does: a duration would overflow for
		// days some three hundred years apart.
		weeks := (mondayOf(date).Unix() - firstWeek.Unix()) / (7 * 86400)
		if slices.Contains(r.Weekdays, isoWeekday(date)) && weeks%int64(r.Interval) == 0 {
			return date.Format(dayLayout)
		}
	}
	panic(fmt.Sprintf("a %s series found no day after %s", r.Rhythm, from))
}

func (r RepetitionSchedule) nextMonthlyOnOrAfter(from string) string {
	firstMonth := monthIndexOf(r.firstMonthlyDay())
	month := monthIndexOf(from)
	for ahead := 0; ahead <= 12*(r.Interval+1); ahead++ {
		candidate, ok := r.dayInMonth(month + ahead)
		if ok && candidate >= from && (month+ahead-firstMonth)%r.Interval == 0 {
			return candidate
		}
	}
	panic(fmt.Sprintf("a %s series found no day after %s", r.Rhythm, from))
}

func (r RepetitionSchedule) firstMatchingWeekday() time.Time {
	start, _ := midnightOf(r.StartsOn)
	for ahead := 0; ahead < 7; ahead++ {
		date := start.AddDate(0, 0, ahead)
		if slices.Contains(r.Weekdays, isoWeekday(date)) {
			return date
		}
	}
	panic("a series has no weekday")
}

func (r RepetitionSchedule) firstMonthlyDay() string {
	month := monthIndexOf(r.StartsOn)
	for ahead := 0; ahead <= 12; ahead++ {
		if candidate, ok := r.dayInMonth(month + ahead); ok && candidate >= r.StartsOn {
			return candidate
		}
	}
	panic(fmt.Sprintf("a %s series found no first day", r.Rhythm))
}

// dayInMonth is the day the series falls on in the month of the index, the
// year times twelve plus the month counted from 0, and false for a month it
// skips.
func (r RepetitionSchedule) dayInMonth(monthIndex int) (string, bool) {
	year, month := monthIndex/12, monthIndex%12+1
	days := daysIn(year, month)
	var day int
	switch r.Rhythm {
	case RepeatsMonthlyOnDay:
		day = min(r.DayOfMonth, days)
	case RepeatsYearly:
		if month != r.Month {
			return "", false
		}
		day = min(r.DayOfMonth, days)
	case RepeatsMonthlyOnWeekday:
		day = r.instanceOfWeekdayIn(year, month, days)
	default:
		panic("a weekday rhythm has no day of a month")
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), true
}

func (r RepetitionSchedule) instanceOfWeekdayIn(year, month, days int) int {
	weekday := r.Weekdays[0]
	if r.Instance == LastWeekdayInstance {
		weekdayOfLastDay := isoWeekday(time.Date(year, time.Month(month), days, 0, 0, 0, 0, time.UTC))
		return days - (weekdayOfLastDay-weekday+7)%7
	}
	weekdayOfFirstDay := isoWeekday(time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC))
	return 1 + (weekday-weekdayOfFirstDay+7)%7 + 7*(r.Instance-1)
}

func monthIndexOf(day string) int {
	date, _ := midnightOf(day)
	return date.Year()*12 + int(date.Month()) - 1
}

func daysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func mondayOf(date time.Time) time.Time {
	return date.AddDate(0, 0, -(isoWeekday(date) - 1))
}

func addDays(day string, days int) string {
	date, _ := midnightOf(day)
	return date.AddDate(0, 0, days).Format(dayLayout)
}

// midnightOf reads a calendar day written as YYYY-MM-DD, and nothing else.
func midnightOf(day string) (time.Time, bool) {
	date, err := time.Parse(dayLayout, day)
	return date, err == nil && date.Format(dayLayout) == day
}
