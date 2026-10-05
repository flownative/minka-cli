package cli

import (
	"fmt"
	"io"
	"strings"
)

// calendarNotices are what a run that brought calendars has to say about them
// beside the counts. A run that was not asked for calendars says nothing.
func calendarNotices(report importReport) []string {
	if report.Calendars == nil || len(report.Calendars.Calendars) == 0 {
		return nil
	}
	notices := []string{plural(len(report.Calendars.Calendars),
		"calendar is in the bundle, which only a Minka whose import applies events takes: any other stops at it",
		"calendars are in the bundle, which only a Minka whose import applies events takes: any other stops at the first of them")}
	single, saidOtherwise, refused, tooLong := 0, 0, 0, 0
	for _, calendar := range report.Calendars.Calendars {
		single += calendar.ThatRepeat - calendar.WithASeries
		saidOtherwise += len(calendar.AllDayInInstants) + len(calendar.TimedInDays)
		refused += len(calendar.TimesMinkaRefuses)
		tooLong += calendar.LongerThanMinkaTakes
	}
	if single > 0 {
		notices = append(notices, plural(single,
			"event repeats at the source and comes once, because a part of its schedule has no place in a bundle or does not fit the event; data.calendars.calendars[].repetitionNotMapped names each part with up to ten events",
			"events repeat at the source and come once, because a part of their schedule has no place in a bundle or does not fit the event; data.calendars.calendars[].repetitionNotMapped names each part with up to ten events"))
	}
	if saidOtherwise > 0 {
		notices = append(notices, plural(saidOtherwise,
			"entry of a calendar is left out because the source says its days in moments or its moments in days, which no rule turns into the other without moving it for somebody; data.calendars.calendars[] names it under allDayInInstants or timedInDays",
			"entries of calendars are left out because the source says their days in moments or their moments in days, which no rule turns into the other without moving them for somebody; data.calendars.calendars[] names them under allDayInInstants and timedInDays"))
	}
	if refused > 0 {
		notices = append(notices, plural(refused,
			"entry of a calendar is left out because it ends before it starts or starts before the year 1, which Minka takes no event with; data.calendars.calendars[].timesMinkaRefuses names it, and a later run brings it once its times are put right at the source",
			"entries of calendars are left out because they end before they start or start before the year 1, which Minka takes no event with; data.calendars.calendars[].timesMinkaRefuses names them, and a later run brings them once their times are put right at the source"))
	}
	if tooLong > 0 {
		notices = append(notices, plural(tooLong,
			"event has a description longer than Minka holds and is passed over there on every run, with its comments and its boosts",
			"events have a description longer than Minka holds and are passed over there on every run, with their comments and their boosts"))
	}
	comments := 0
	for _, project := range report.Calendars.CommentsUnderSingleDays {
		comments += project.Comments
	}
	if comments > 0 {
		notices = append(notices, plural(comments,
			"comment was written under a single day of a series, which a bundle has no place for, and is not carried; data.calendars.commentsUnderSingleDays counts it with its project",
			"comments were written under single days of a series, which a bundle has no place for, and are not carried; data.calendars.commentsUnderSingleDays counts them per project"))
	}
	return notices
}

// renderImportedCalendars says what became of the calendars of a run that brought them.
func renderImportedCalendars(w io.Writer, report importReport) {
	if report.Calendars == nil {
		return
	}
	for _, calendar := range report.Calendars.Calendars {
		fmt.Fprintf(w, "Calendar %s of project %s: %s, %d trashed; %d repeat at the source, %d of them with their series\n",
			calendar.ToolSourceID, calendar.ProjectSourceID, plural(calendar.Written, "event", "events"), calendar.Trashed,
			calendar.ThatRepeat, calendar.WithASeries)
		for _, part := range calendar.RepetitionNotMapped {
			more := ""
			if part.Events > len(part.SourceIDs) {
				more = fmt.Sprintf(" and %d more", part.Events-len(part.SourceIDs))
			}
			fmt.Fprintf(w, "  %s of a schedule, not mapped, the event comes once: %d — %s%s\n", part.Part, part.Events, strings.Join(part.SourceIDs, ", "), more)
		}
		if len(calendar.AllDayInInstants) > 0 {
			fmt.Fprintf(w, "  all-day entries said in moments, left out: %d — %s\n", len(calendar.AllDayInInstants), strings.Join(calendar.AllDayInInstants, ", "))
		}
		if len(calendar.TimedInDays) > 0 {
			fmt.Fprintf(w, "  timed entries said in days, left out: %d — %s\n", len(calendar.TimedInDays), strings.Join(calendar.TimedInDays, ", "))
		}
		if len(calendar.TimesMinkaRefuses) > 0 {
			fmt.Fprintf(w, "  entries that end before they start or start before the year 1, left out: %d — %s\n", len(calendar.TimesMinkaRefuses), strings.Join(calendar.TimesMinkaRefuses, ", "))
		}
		if calendar.LongerThanMinkaTakes > 0 {
			fmt.Fprintf(w, "  descriptions longer than Minka holds: %d\n", calendar.LongerThanMinkaTakes)
		}
	}
	for _, project := range report.Calendars.CommentsUnderSingleDays {
		fmt.Fprintf(w, "Comments under single days of a series in project %s, not carried: %d\n", project.ProjectSourceID, project.Comments)
	}
}
