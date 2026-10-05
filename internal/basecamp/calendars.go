package basecamp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// scheduleEntry is an entry of a calendar as the list of its calendar hands it
// out, which is everything a run reads of it: an entry that repeats is listed
// once, with its schedule and the times of its first day.
type scheduleEntry struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// Summary is the name of the entry; Title says the same.
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	Files       []bodyFile `json:"description_attachments"`
	AllDay      bool       `json:"all_day"`
	// StartsAt and EndsAt are two moments of a timed entry and two days of an
	// all-day one, the last day included.
	StartsAt     string   `json:"starts_at"`
	EndsAt       string   `json:"ends_at"`
	Participants []person `json:"participants"`
	JoinURL      *string  `json:"join_url"`
	Highlighted  bool     `json:"highlighted"`
	// Recurrence is the schedule of an entry that repeats; the key is missing on
	// one that does not. It is read on its own, so that one of a shape nobody
	// expected costs the series and not the run.
	Recurrence    json.RawMessage `json:"recurrence_schedule"`
	CommentsCount int             `json:"comments_count"`
	BoostsCount   int             `json:"boosts_count"`
}

// CalendarCounts says what became of the calendars a run brought: per calendar,
// and the comments written under single days of a series, per project.
type CalendarCounts struct {
	Calendars               []CalendarCount     `json:"calendars"`
	CommentsUnderSingleDays []SingleDayComments `json:"commentsUnderSingleDays"`
}

// CalendarCount is what became of the entries of one calendar. What the bundle
// leaves out for its status is in RecordsLeftOut of the run, as an "event" in
// this calendar.
type CalendarCount struct {
	ProjectSourceID string `json:"projectSourceId"`
	ToolSourceID    string `json:"toolSourceId"`
	// Written counts the events of the bundle, Trashed those in the trash
	// among them.
	Written int `json:"written"`
	Trashed int `json:"trashed"`
	// ThatRepeat counts the events written that repeat at the source and
	// WithASeries those among them the bundle carries with their schedule.
	ThatRepeat  int `json:"thatRepeat"`
	WithASeries int `json:"withASeries"`
	// AllDayInInstants names the all-day entries the source says in moments and
	// TimedInDays the timed ones it says in days. Neither has a rule that does
	// not move it by a day for somebody, so both are left out.
	AllDayInInstants []string `json:"allDayInInstants"`
	TimedInDays      []string `json:"timedInDays"`
	// TimesMinkaRefuses names the entries that end before they start or start
	// before the year 1, which Minka takes no event with; they are left out.
	TimesMinkaRefuses []string `json:"timesMinkaRefuses"`
	// RepetitionNotMapped names each part of a schedule the bundle does not
	// carry, with the events whose schedule held it. Each of them comes once.
	RepetitionNotMapped []EventRepetitionNotMapped `json:"repetitionNotMapped"`
	// LongerThanMinkaTakes counts the events whose description is longer than
	// Minka holds, which Minka passes over on every run.
	LongerThanMinkaTakes int `json:"longerThanMinkaTakes"`
}

// EventRepetitionNotMapped is a part of the schedules of one calendar the bundle
// does not carry, by Basecamp's field, how many events held it and the first
// ten of them.
type EventRepetitionNotMapped struct {
	Part      string   `json:"part"`
	Events    int      `json:"events"`
	SourceIDs []string `json:"sourceIds"`
}

// namedPerPart is how many events a part of a schedule names; the count says
// how many there are.
const namedPerPart = 10

// SingleDayComments counts the comments of one project written under a single
// day of a series, which a bundle has no place for, and names the days by the
// source's identifier of them.
type SingleDayComments struct {
	ProjectSourceID string   `json:"projectSourceId"`
	Comments        int      `json:"comments"`
	DaySourceIDs    []string `json:"daySourceIds"`
}

// aSingleDay is what a comment hangs on that was written under one day of a
// series: Basecamp makes the day a recording of its own.
const aSingleDay = "Schedule::Entry::Occurrence"

// extractCalendars adds the calendars of every project with their events, and
// counts what was written under the single days of a series.
func (e *Extractor) extractCalendars(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		repeating := false
		for _, calendar := range t.calendars {
			series, err := e.extractCalendar(ctx, w, t.bucket, calendar)
			if err != nil {
				return err
			}
			repeating = repeating || series
		}
		if repeating {
			if err := e.countCommentsUnderSingleDays(ctx, t.bucket); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractCalendar adds the entries of one calendar and says whether one of
// them repeats.
//
// Everything is read from the lists of the calendar: the list in use, the one
// of the trash, and the one of the shelf, which a bundle has no word for and
// which is read only to count what it leaves out. An entry of a series is never
// asked for on its own, and nor is a single day of it.
//
// A walk is no snapshot: an entry met twice is written once.
func (e *Extractor) extractCalendar(ctx context.Context, w *bundle.Writer, bucket, calendar int64) (bool, error) {
	count := CalendarCount{ProjectSourceID: sourceID(bucket), ToolSourceID: sourceID(calendar),
		AllDayInInstants: []string{}, TimedInDays: []string{}, TimesMinkaRefuses: []string{}, RepetitionNotMapped: []EventRepetitionNotMapped{}}
	met := map[int64]bool{}
	var entries []scheduleEntry
	for _, status := range []string{"", bundle.StatusTrashed, bundle.StatusArchived} {
		path := fmt.Sprintf("buckets/%d/schedules/%d/entries.json", bucket, calendar)
		if status != "" {
			path += "?status=" + status
		}
		listed, err := getList[scheduleEntry](ctx, e.source, path)
		if err != nil {
			return false, err
		}
		for _, found := range listed {
			if aSecondTime(met, found.ID) {
				continue
			}
			found.Status = statusOrInUse(found.Status)
			carried, err := e.metAmong(todoStatuses, "event", found.ID, found.Status, found.recording, "calendar", sourceID(calendar))
			if err != nil {
				return false, err
			}
			if carried {
				entries = append(entries, found)
			}
		}
	}
	if err := oldestFirst(entries, "event", func(entry scheduleEntry) (int64, string) { return entry.ID, entry.CreatedAt }); err != nil {
		return false, err
	}

	repeating := false
	for _, found := range entries {
		written, err := e.writeEvent(ctx, w, bucket, calendar, found, &count)
		if err != nil {
			return false, err
		}
		repeating = repeating || written && repeats(found.Recurrence)
	}
	e.calendars.Calendars = append(e.calendars.Calendars, count)
	return repeating, nil
}

// writeEvent adds an event with its description, its people and its schedule,
// and what hangs on it: its files, its comments and its boosts. It says whether
// the event was written.
//
// An entry whose days the source says in moments, or whose moments it says in
// days, is left out and named: a day made of a moment is the day of one zone,
// and somebody in another would find the event on the day before or after.
func (e *Extractor) writeEvent(ctx context.Context, w *bundle.Writer, bucket, calendar int64, found scheduleEntry, count *CalendarCount) (bool, error) {
	times, shape, err := timesOfAnEntry(found)
	if err != nil {
		return false, err
	}
	switch shape {
	case daysSaidInMoments:
		count.AllDayInInstants = append(count.AllDayInInstants, sourceID(found.ID))
		return false, nil
	case momentsSaidInDays:
		count.TimedInDays = append(count.TimedInDays, sourceID(found.ID))
		return false, nil
	case timesMinkaRefuses:
		count.TimesMinkaRefuses = append(count.TimesMinkaRefuses, sourceID(found.ID))
		return false, nil
	}
	title := e.titleOf("event", found.ID, found.Summary)
	origin, err := e.record("event", found.ID, found.recording)
	if err != nil {
		return false, err
	}
	participants, err := e.peopleOn(found.Participants)
	if err != nil {
		return false, err
	}
	mine := owns(bundle.OwnerEvent, found.ID)
	description, files, err := e.rewriteBody(mine, found.Description, "", found.Files)
	if err != nil {
		return false, err
	}

	record := bundle.Event{
		Record:           origin,
		ProjectSourceID:  sourceID(bucket),
		ToolSourceID:     sourceID(calendar),
		Title:            title,
		AllDay:           found.AllDay,
		Participants:     participants,
		Highlighted:      found.Highlighted,
		Status:           found.Status,
		VisibleToClients: visibleToClientsOf(found.recording),
	}
	if found.AllDay {
		record.StartsOn, record.EndsOn = &found.StartsAt, &found.EndsAt
	} else {
		record.StartsAt, record.EndsAt = &found.StartsAt, &found.EndsAt
	}
	if description != "" {
		record.Description = &description
	}
	if found.JoinURL != nil && strings.TrimSpace(*found.JoinURL) != "" {
		record.JoinURL = found.JoinURL
	}
	if record.Subscribers, err = e.subscribersOf(ctx, "event", bucket, found.ID, found.Status); err != nil {
		return false, err
	}
	if repeats(found.Recurrence) {
		var zone string
		if found.Creator != nil {
			zone = found.Creator.TimeZone
		}
		record.Repetition, record.RepetitionNotMapped = eventRepetitionOf(found.Recurrence, times, zone)
	}
	if err := w.Add(bundle.Events, record); err != nil {
		return false, err
	}

	e.measure(bundle.Events, description, LongestBodyRecord{Record: "event", SourceID: origin.SourceID, In: "calendar", InSourceID: sourceID(calendar)})
	count.Written++
	if found.Status == bundle.StatusTrashed {
		count.Trashed++
	}
	if longerThanMinkaTakes(description) {
		count.LongerThanMinkaTakes++
	}
	if repeats(found.Recurrence) {
		count.ThatRepeat++
		if record.Repetition != nil {
			count.WithASeries++
		}
		count.noteNotMapped(found.ID, record.RepetitionNotMapped)
	}
	if err := e.attachFiles(ctx, w, mine, origin, files); err != nil {
		return false, err
	}
	return true, e.writeDiscussion(ctx, w, bucket, bundle.ParentEvent, bundle.TargetEvent, found.ID, found.CommentsCount, found.BoostsCount)
}

// noteNotMapped counts an event under each part of its schedule the bundle does
// not carry.
func (c *CalendarCount) noteNotMapped(id int64, parts []string) {
	for _, part := range parts {
		i := 0
		for i < len(c.RepetitionNotMapped) && c.RepetitionNotMapped[i].Part != part {
			i++
		}
		if i == len(c.RepetitionNotMapped) {
			c.RepetitionNotMapped = append(c.RepetitionNotMapped, EventRepetitionNotMapped{Part: part, SourceIDs: []string{}})
		}
		one := &c.RepetitionNotMapped[i]
		one.Events++
		if len(one.SourceIDs) < namedPerPart {
			one.SourceIDs = append(one.SourceIDs, sourceID(id))
		}
	}
}

// entryShape is how the times of an entry are said: as the entry is, or in the
// other kind of time.
type entryShape int

const (
	asItIs entryShape = iota
	daysSaidInMoments
	momentsSaidInDays
	// timesMinkaRefuses is an entry that ends before it starts, or lies in
	// the year 0: somebody typed it so, and Minka takes no such event.
	timesMinkaRefuses
)

// timesOfAnEntry reads the times of an entry. A time that is neither a day nor
// a moment ends the run: the source wrote it, and a time nobody can read is one
// nobody can put right afterwards. An end before the start, and a start in
// the year 0, are what somebody typed; Minka takes neither, so the entry is
// left out and named rather than ending a walk of hours.
func timesOfAnEntry(found scheduleEntry) (eventTimes, entryShape, error) {
	what := fmt.Sprintf("event %d", found.ID)
	startsOn, startIsADay := aDayOfAnyYear(found.StartsAt)
	_, endIsADay := aDayOfAnyYear(found.EndsAt)
	startsAt, startIsAMoment := aMomentOfAnyYear(found.StartsAt)
	endsAt, endIsAMoment := aMomentOfAnyYear(found.EndsAt)
	switch {
	case startIsADay && endIsADay:
		if !found.AllDay {
			return eventTimes{}, momentsSaidInDays, nil
		}
		if found.EndsAt < found.StartsAt || startsOn.Year() < firstYear {
			return eventTimes{}, timesMinkaRefuses, nil
		}
		return eventTimes{allDay: true, startsOn: found.StartsAt}, asItIs, nil
	case startIsAMoment && endIsAMoment:
		if found.AllDay {
			return eventTimes{}, daysSaidInMoments, nil
		}
		if endsAt.Before(startsAt) {
			return eventTimes{}, timesMinkaRefuses, nil
		}
		// Minka holds the start of an event in UTC, where a moment early in the
		// year 1 east of Greenwich still falls in the year 0, as one of the year 0
		// itself does.
		if startsAt.UTC().Year() < firstYear {
			return eventTimes{}, timesMinkaRefuses, nil
		}
		return eventTimes{startsAt: startsAt, endsAt: endsAt}, asItIs, nil
	}
	for _, field := range []struct{ name, value string }{{"starts", found.StartsAt}, {"ends", found.EndsAt}} {
		if _, isADay := aDay(field.value); isADay {
			continue
		}
		if _, err := momentOf(what, field.name, field.value); err != nil {
			return eventTimes{}, asItIs, err
		}
	}
	return eventTimes{}, asItIs, &Error{Kind: API, Message: fmt.Sprintf("%s starts in one kind of time and ends in another", what)}
}

// commentOfAProject is a comment as the list of every comment of a project
// names it: with what it hangs on.
type commentOfAProject struct {
	ID     int64 `json:"id"`
	Parent struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"parent"`
}

// countCommentsUnderSingleDays counts the comments of a project that hang on a
// single day of a series. A bundle has no place for them, and asking a day for
// its comments would make the day at the source, so the list of every comment
// of the project is the one place they can be counted.
func (e *Extractor) countCommentsUnderSingleDays(ctx context.Context, bucket int64) error {
	listed, err := getList[commentOfAProject](ctx, e.source, fmt.Sprintf("projects/recordings.json?type=Comment&bucket=%d", bucket))
	if err != nil {
		return err
	}
	counted := SingleDayComments{ProjectSourceID: sourceID(bucket), DaySourceIDs: []string{}}
	days := map[int64]bool{}
	for _, comment := range listed {
		if comment.Parent.Type != aSingleDay {
			continue
		}
		counted.Comments++
		if !days[comment.Parent.ID] {
			days[comment.Parent.ID] = true
			counted.DaySourceIDs = append(counted.DaySourceIDs, sourceID(comment.Parent.ID))
		}
	}
	if counted.Comments > 0 {
		e.calendars.CommentsUnderSingleDays = append(e.calendars.CommentsUnderSingleDays, counted)
	}
	return nil
}
