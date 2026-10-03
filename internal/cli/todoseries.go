package cli

import (
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a write on a to-do series that it will not carry out.
const (
	apiCodeTodoToRepeatUnknown           = "todoToRepeatUnknown"
	apiCodeTodoSeriesNeedsDueDay         = "todoSeriesNeedsDueDay"
	apiCodeTodoRepeatsAlready            = "todoRepeatsAlready"
	apiCodeTodoSeriesScheduleInvalid     = "todoSeriesScheduleInvalid"
	apiCodeTodoSeriesDatesNotADate       = "todoSeriesDatesNotADate"
	apiCodeTodoSeriesDatesOutOfOrder     = "todoSeriesDatesOutOfOrder"
	apiCodeTodoSeriesLeadDaysInvalid     = "todoSeriesLeadDaysInvalid"
	apiCodeTodoSeriesStopped             = "todoSeriesStopped"
	apiCodeTodoSeriesNotStopped          = "todoSeriesNotStopped"
	apiCodeTodoSeriesOccurrenceAhead     = "todoSeriesOccurrenceAhead"
	apiCodeTodoSeriesFinished            = "todoSeriesFinished"
	apiCodeTodoSeriesChanged             = "todoSeriesChanged"
	apiCodeTodoTitleMissing              = "todoTitleMissing"
	apiCodeTodoTitleTooLong              = "todoTitleTooLong"
	apiCodeTodoTitleHasControlCharacters = "todoTitleHasControlCharacters"
	apiCodeTodoNotesTooLong              = "todoNotesTooLong"
	apiCodeTodoNotesNotReadable          = "todoNotesNotReadable"
)

// The rhythms of a series, as Minka names them.
const (
	rhythmDaily            = "daily"
	rhythmWeekly           = "weekly"
	rhythmMonthlyOnDay     = "monthlyOnDay"
	rhythmMonthlyOnWeekday = "monthlyOnWeekday"
	rhythmYearly           = "yearly"
)

const todoSeriesNotFoundHint = "There is no such series, its list is in the trash, it belongs to a project you are not a member of, or you are a client of the project and its list is not shown to you. The series of a list are listed with: minka todos series list <listId>"

const todoSeriesScheduleHint = "--rhythm is daily, weekly, monthlyOnDay, monthlyOnWeekday or yearly. --weekdays and --weekday take 1 (Monday) to 7 or a name such as mon, and a daily or weekly series needs at least one weekday. --interval is 1 to 99, --day 1 to 31 (a yearly day has to exist in its month in a leap year, so 31 April is none), --instance 1 to 4 or last, --month 1 to 12."

const todoSeriesPeopleHint = "Only members of the project who see the list can be named on a series: a client the list is hidden from cannot. The members are listed by: minka people list <projectId>"

const todoSeriesScheduleHelp = `--rhythm is one of Minka's five words, and each takes its own details:

  daily             --weekdays (every week, on each weekday given)
  weekly            --weekdays, --interval (every n-th week)
  monthlyOnDay      --day, --interval (every n-th month)
  monthlyOnWeekday  --weekday, --instance (1 to 4, or last), --interval
  yearly            --month, --day

--weekdays is repeatable and takes a comma list; a weekday is 1 (Monday) to 7
or a name such as mon. A day of the month a month lacks falls on its last day,
and 29 February on the 28th outside a leap year. A detail the rhythm does not
use is ignored by Minka: with --rhythm given, the CLI refuses it instead, and
without, it says so in a notice.

What is not given Minka takes from the first day of the series: its weekday,
its day of the month, which of its weekday it is in the month (a day after the
28th is the last), its month. A new daily series falls on Monday to Friday.
--ends-on is the last day an occurrence may fall on.

--lead-days is how many days before its day an occurrence appears in the list,
0 to 366; Minka's own is 7.

The CLI never works out a day of the series itself: nextOccursOn and
nextShowsOn are what Minka answers, counted on the installation's today.`

func (a *app) newTodoSeriesCmd() *cobra.Command {
	return group("series", "Read, change, stop, resume and show now the series that repeat to-dos",
		a.newTodoSeriesListCmd(),
		a.newTodoSeriesShowCmd(),
		a.newTodoSeriesEditCmd(),
		a.newTodoSeriesStopCmd(),
		a.newTodoSeriesResumeCmd(),
		a.newTodoSeriesShowNowCmd(),
	)
}

// scheduleFlags are the flags that say something about a schedule. Only those
// given go out.
type scheduleFlags struct {
	rhythm, weekday, instance, startsOn, endsOn string
	weekdays                                    []string
	interval, day, month, leadDays              int
}

func (f *scheduleFlags) register(cmd *cobra.Command, withStart, withLead bool) {
	flags := cmd.Flags()
	flags.StringVar(&f.rhythm, "rhythm", "", "daily, weekly, monthlyOnDay, monthlyOnWeekday or yearly")
	flags.StringSliceVar(&f.weekdays, "weekdays", nil, "Weekdays of a daily or weekly series: 1 (Monday) to 7 or names such as mon (repeatable)")
	flags.IntVar(&f.interval, "interval", 0, "Every n-th week or month, 1 to 99")
	flags.IntVar(&f.day, "day", 0, "Day of the month of a monthlyOnDay or yearly series, 1 to 31")
	flags.StringVar(&f.weekday, "weekday", "", "Weekday of a monthlyOnWeekday series: 1 (Monday) to 7 or a name such as fri")
	flags.StringVar(&f.instance, "instance", "", "Which of its weekday in the month: 1 to 4, or last")
	flags.IntVar(&f.month, "month", 0, "Month of a yearly series, 1 to 12")
	if withStart {
		flags.StringVar(&f.startsOn, "starts-on", "", "First day of the series as YYYY-MM-DD")
	}
	flags.StringVar(&f.endsOn, "ends-on", "", "Last day an occurrence may fall on as YYYY-MM-DD, empty to take the end off")
	if withLead {
		flags.IntVar(&f.leadDays, "lead-days", 0, "Days before its day an occurrence appears in the list, 0 to 366")
	}
}

// input turns the flags given into the schedule and the lead that go out,
// each nil where nothing was given. A weekday that is no number and no name
// and an instance that is neither are refused here, because they cannot be
// sent at all; so is a detail the rhythm given does not use, which Minka would
// ignore without a word.
func (f *scheduleFlags) input(cmd *cobra.Command) (api.ScheduleInput, *int, error) {
	flags := cmd.Flags()
	var in api.ScheduleInput
	if flags.Changed("rhythm") {
		in.Rhythm = &f.rhythm
	}
	if flags.Changed("weekdays") {
		days, err := weekdayNumbers(cmd, "--weekdays", f.weekdays)
		if err != nil {
			return in, nil, err
		}
		in.Weekdays = &days
	}
	if flags.Changed("weekday") {
		days, err := weekdayNumbers(cmd, "--weekday", []string{f.weekday})
		if err != nil {
			return in, nil, err
		}
		in.Weekday = &days[0]
	}
	if flags.Changed("instance") {
		instance, err := weekdayInstance(cmd, f.instance)
		if err != nil {
			return in, nil, err
		}
		in.Instance = &instance
	}
	for name, target := range map[string]**int{"interval": &in.Interval, "day": &in.Day, "month": &in.Month} {
		if flags.Changed(name) {
			value, _ := flags.GetInt(name)
			*target = &value
		}
	}
	if flags.Lookup("starts-on") != nil && flags.Changed("starts-on") {
		in.StartsOn = &f.startsOn
	}
	if flags.Changed("ends-on") {
		in.EndsOn = &f.endsOn
	}
	if in.Rhythm != nil {
		if unused := detailsUnusedBy(*in.Rhythm, in); len(unused) > 0 {
			return in, nil, usageErrorf(cmd, "a %s series does not use %s; Minka would ignore it", *in.Rhythm, strings.Join(unused, " and "))
		}
	}
	var leadDays *int
	if flags.Changed("lead-days") {
		leadDays = &f.leadDays
	}
	return in, leadDays, nil
}

// weekdayInstance reads which of its weekday in the month a series falls on.
// A number goes out as it is, so that Minka says what it makes of one out of
// range; "last" is Minka's -1.
func weekdayInstance(cmd *cobra.Command, value string) (int, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "last") {
		return -1, nil
	}
	instance, err := strconv.Atoi(value)
	if err != nil {
		return 0, usageErrorf(cmd, "--instance is 1 to 4 or last, not %q", value)
	}
	return instance, nil
}

// detailsUnusedBy names the flags given for details a rhythm does not use.
// A rhythm the CLI does not know uses everything: what to make of it is
// Minka's to say.
func detailsUnusedBy(rhythm string, in api.ScheduleInput) []string {
	uses := map[string][]string{
		rhythmDaily:            {"--weekdays"},
		rhythmWeekly:           {"--weekdays", "--interval"},
		rhythmMonthlyOnDay:     {"--day", "--interval"},
		rhythmMonthlyOnWeekday: {"--weekday", "--instance", "--interval"},
		rhythmYearly:           {"--month", "--day"},
	}[rhythm]
	if uses == nil {
		return nil
	}
	var unused []string
	for _, detail := range []struct {
		flag  string
		given bool
	}{
		{"--weekdays", in.Weekdays != nil}, {"--interval", in.Interval != nil}, {"--day", in.Day != nil},
		{"--weekday", in.Weekday != nil}, {"--instance", in.Instance != nil}, {"--month", in.Month != nil},
	} {
		if detail.given && !slices.Contains(uses, detail.flag) {
			unused = append(unused, detail.flag)
		}
	}
	return unused
}

// ignoredDetailsNotice says which details the rhythm Minka answered with does
// not use, for a write that named no rhythm: Minka ignores them without a word.
func ignoredDetailsNotice(in api.ScheduleInput, schedule api.RepetitionSchedule) []string {
	if in.Rhythm != nil {
		return nil
	}
	if unused := detailsUnusedBy(schedule.Rhythm, in); len(unused) > 0 {
		return []string{fmt.Sprintf("Minka ignored %s: a %s series does not use it", strings.Join(unused, " and "), schedule.Rhythm)}
	}
	return nil
}

func (a *app) newTodosRepeatCmd() *cobra.Command {
	var schedule scheduleFlags

	cmd := &cobra.Command{
		Use:   "repeat <todoId>",
		Short: "Make a to-do repeat: it becomes the first occurrence of a new series",
		Long: `Makes a series of a to-do with a due day. The to-do becomes the first
occurrence, and the series starts on its due day, even where that day is off
the rhythm. The series takes the to-do's title, notes, group, assignees and the
people told when it is done; each occurrence is a to-do of its own at the end
of the list. A done to-do repeats as well.

Without --rhythm the series repeats weekly on the weekday of the due day.

` + todoSeriesScheduleHelp + `

The to-do is read first for its list, which the address of a new series
names. A to-do without a due day is exit 9 todoSeriesNeedsDueDay, one that is
an occurrence already todoRepeatsAlready.`,
		Example: `  minka todos repeat 0199...
  minka todos repeat 0199... --rhythm monthlyOnWeekday --weekday fri --instance 1 --lead-days 7
  minka todos repeat 0199... --rhythm weekly --weekdays tue,thu --interval 2 --ends-on 2026-12-31`,
		Args: requireArgs("todoId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			in, leadDays, err := schedule.input(cmd)
			if err != nil {
				return err
			}
			input := api.RepeatTodoInput{TodoID: args[0], LeadDays: leadDays}
			if !in.IsEmpty() {
				input.Schedule = &in
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			todo, err := s.client.Todo(a.ctx, args[0])
			if err != nil {
				return todoRefusal(s, err, todoHints{})
			}
			series, err := s.client.RepeatTodo(a.ctx, todo.Value.ListID, input)
			if err != nil {
				e := todoSeriesRefusal(s, err, args[0], "")
				if e.APICode == apiCodeTodoListArchived {
					e.Hint = "The list of the to-do lies in the archive of its tool, and nothing in it repeats until it is back: minka todolists unarchive " + todo.Value.ListID
				}
				return e
			}

			summary := "To-do " + todo.Value.Title + " repeats " + describeRepetition(series.Value.Schedule)
			return a.ok(success{
				data:        series.Raw,
				summary:     summary,
				notices:     ignoredDetailsNotice(in, series.Value.Schedule),
				breadcrumbs: todoSeriesBreadcrumbs(series.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nSeries: %s\n%s\n", summary, series.Value.ID, nextOccurrenceLine(series.Value))
				},
			})
		},
	}
	schedule.register(cmd, false, true)
	return cmd
}

func (a *app) newTodoSeriesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <listId>",
		Short: "List the series of a to-do list, stopped ones too",
		Long: `Lists every series of a list, stopped and finished ones too, the one made
first first, as a bare array of what todos series show gives for each.`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			series, err := s.client.TodoSeriesOfList(a.ctx, args[0])
			if err != nil {
				return todoListRefusal(s, err, args[0])
			}
			return a.ok(success{
				data:    series.Raw,
				summary: plural(len(series.Value), "series", "series") + " of list " + args[0],
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka todos series show <seriesId>", Description: "Read a series with its schedule and its next day"},
					{Action: "repeat", Cmd: "minka todos repeat <todoId>", Description: "Make a to-do with a due day repeat"},
				},
				text: func(w io.Writer) { renderTodoSeriesList(w, series.Value) },
			})
		},
	}
}

func (a *app) newTodoSeriesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <seriesId>",
		Short: "Show a series with its schedule, its state and its next day",
		Long: `Shows a series: its list and group, what its occurrences start from (the
title, and the notes with the files that hang in them), its schedule, its
state (running, stopped, or finished: past its end), the day of the next
occurrence (nextOccursOn) and the day it appears in the list (nextShowsOn),
whether show-now would make one (canShowNow), and updatedAt, which todos
series edit --expect takes.

The notes are shown as notesSource, the text todos series edit --notes takes
back with --notes-format html, keeping their files and mentions.

nextShowsOn is null where the next occurrence comes with the next round or is
in the list already, and while the list is archived; a series of an archived
list makes nothing until the list is back.`,
		Args: requireArgs("seriesId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			series, err := s.client.TodoSeries(a.ctx, args[0])
			if err != nil {
				return todoSeriesRefusal(s, err, "", args[0])
			}
			return a.ok(success{
				data:        series.Raw,
				summary:     todoSeriesSummary(series.Value),
				breadcrumbs: todoSeriesBreadcrumbs(series.Value),
				text:        func(w io.Writer) { renderTodoSeries(w, series.Value) },
			})
		},
	}
}

// todoSeriesNotesHelp says how the notes of a series are written.
const todoSeriesNotesHelp = `--notes is Markdown by default and is sent as HTML, the way the notes of a
to-do are; --notes - reads them from stdin, and --notes "" empties them. With
--notes-format html the HTML goes as it is, and notes that carry no element
Minka serves are kept as the characters they consist of, which the command
says in a notice.

Files and mentions stand in the notes as action-text-attachment elements,
which Markdown cannot hold. To keep them, take notesSource from todos series
show --json, change it and send it back with --notes-format html. New notes
that would drop them are refused unless --drop-attachments is given. Minka
drops a file or a person from outside the project without a word, and tells
nobody the notes mention: read the series back to see what was kept.`

func (a *app) newTodoSeriesEditCmd() *cobra.Command {
	var schedule scheduleFlags
	var title, notes, notesFormat, expect string
	var assignees, notified []string
	var noAssignees, nobodyNotified, dropAttachments, force bool

	cmd := &cobra.Command{
		Use:   "edit <seriesId>",
		Short: "Change the title, the notes, the schedule, the lead or the people of a series",
		Long: `Changes what the occurrences made from now on start from. The occurrence made
already stays as it is, and what is not given stays as it is: only the flags
given go out, never the details of the schedule as they stand.

While rhythm and first day stay, the details not given are the series' own,
so --interval 2 alone keeps the day. On a new --rhythm or --starts-on, the
details not given follow from the first day, as on the web. A stopped or
finished series takes changes too; a finished one gets further days again by
a later --ends-on.

` + todoSeriesScheduleHelp + `

` + todoSeriesNotesHelp + `

--assignee and --notify-when-done give the WHOLE set of people: whoever is on
the series and not named again is taken off. --no-assignees and
--no-notify-when-done take everybody off. ` + personHelp + `

Somebody else may have changed the series since you read it. Give the
updatedAt you read with --expect: a series written over since, by an edit, a
stop, a resume or an occurrence made, is then left as it stands, the command
prints it and exits with 9 (meta.apiCode todoSeriesChanged). --force writes
over whatever stands, and one of the two flags is required.

A change that asks for what stands already writes nothing: updatedAt stays,
and it is no conflict even over a series written since. People are compared
as the series shows them, in their order; somebody on it who no longer sees
the list is taken off by a write that names the people and changes anything,
and kept otherwise.`,
		Example: `  minka todos series edit 0199... --interval 2 --expect 2026-09-29T20:08:41.123456Z
  minka todos series edit 0199... --rhythm monthlyOnDay --day 31 --expect 2026-09-29T20:08:41.123456Z
  minka todos series edit 0199... --ends-on "" --assignee me --force
  minka todos series edit 0199... --notes - --expect 2026-09-29T20:08:41.123456Z < notes.md`,
		Args: requireArgs("seriesId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			guard, err := entryGuard(cmd, "series", "minka todos series show <seriesId>", expect, force)
			if err != nil {
				return err
			}
			if err := checkNotesFormat(cmd, notesFormat); err != nil {
				return err
			}
			if noAssignees && len(assignees) > 0 {
				return usageErrorf(cmd, "--assignee and --no-assignees contradict each other")
			}
			if nobodyNotified && len(notified) > 0 {
				return usageErrorf(cmd, "--notify-when-done and --no-notify-when-done contradict each other")
			}
			in, leadDays, err := schedule.input(cmd)
			if err != nil {
				return err
			}

			input := api.UpdateTodoSeriesInput{LeadDays: leadDays, ExpectedUpdatedAt: guard}
			var notices []string
			if flags.Changed("title") {
				input.Title = &title
			}
			if flags.Changed("notes") {
				html, warnings, err := a.richTextHTML("the notes", "--notes", notes, notesFormat)
				if err != nil {
					return err
				}
				input.Notes, notices = &html, warnings
			}
			if !in.IsEmpty() {
				input.Schedule = &in
			}
			if noAssignees {
				input.AssigneeIDs = &[]string{}
			}
			if nobodyNotified {
				input.NotifiedIDs = &[]string{}
			}
			if input.Title == nil && input.Notes == nil && input.Schedule == nil && input.LeadDays == nil && input.AssigneeIDs == nil && input.NotifiedIDs == nil &&
				len(assignees) == 0 && len(notified) == 0 {
				return usageErrorf(cmd, "nothing to change, give --title, --notes, a detail of the schedule, --lead-days, --assignee or --notify-when-done")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			// The series is read only to look up people by name and to hold new
			// notes against its own; its schedule never goes back out, and the
			// read fills no --expect.
			var current *api.TodoSeries
			read := func() (*api.TodoSeries, error) {
				if current == nil {
					series, err := s.client.TodoSeries(a.ctx, args[0])
					if err != nil {
						return nil, todoSeriesRefusal(s, err, "", args[0])
					}
					current = &series.Value
				}
				return current, nil
			}
			people := newProjectPeople(a, s, func() (string, error) {
				series, err := read()
				if err != nil {
					return "", err
				}
				return series.ProjectID, nil
			})
			if len(assignees) > 0 {
				ids, err := people.resolve(assignees)
				if err != nil {
					return err
				}
				input.AssigneeIDs = &ids
			}
			if len(notified) > 0 {
				ids, err := people.resolve(notified)
				if err != nil {
					return err
				}
				input.NotifiedIDs = &ids
			}
			if input.Notes != nil {
				series, err := read()
				if err != nil {
					return err
				}
				// A series of a list in the archive takes no notes at all. The
				// series does not say so, its list does.
				listArchived := func() (bool, error) {
					list, err := s.client.TodoList(a.ctx, series.ListID)
					if err != nil {
						return false, todoListRefusal(s, err, series.ListID)
					}
					return list.Value.Archived, nil
				}
				dropped, err := a.guardOverFiles(s, series.ProjectID, listArchived, firstNonEmpty(series.NotesSource, series.Notes), *input.Notes, dropAttachments,
					"the notes have %s, and the new notes would drop them",
					"Change notesSource from todos series show --json and send it back with --notes-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			series, err := s.client.UpdateTodoSeries(a.ctx, args[0], input)
			if err != nil {
				return entryConflict(a, todoSeriesRefusal(s, err, "", args[0]), err, apiCodeTodoSeriesChanged, "series",
					func(series api.TodoSeries) string { return series.UpdatedAt }, renderTodoSeries)
			}

			summary := "Changed series " + series.Value.Title + ": repeats " + describeRepetition(series.Value.Schedule)
			notices = append(notices, ignoredDetailsNotice(in, series.Value.Schedule)...)
			// A write that asks for what stands already writes nothing, so the
			// moment read is still the series' own.
			if guard != nil && sameMoment(*guard, series.Value.UpdatedAt) {
				summary = "Series " + series.Value.Title + " holds all of this already; nothing was written"
				notices = append(notices, "nothing changed: the series holds what was sent, and its updatedAt stays "+series.Value.UpdatedAt)
			}
			return a.ok(success{
				data:        series.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: todoSeriesBreadcrumbs(series.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\nTo write over it: --expect %s\n", summary, nextOccurrenceLine(series.Value), series.Value.UpdatedAt)
				},
			})
		},
	}

	schedule.register(cmd, true, true)
	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title of the occurrences; an empty one is refused")
	flags.StringVar(&notes, "notes", "", "New notes of the occurrences, - to read them from stdin, empty to remove them")
	flags.StringVar(&notesFormat, "notes-format", "markdown", "Format of the notes: markdown or html")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace notes that have attachments, mentions or references, which drops them")
	flags.StringArrayVar(&assignees, "assignee", nil, "Person to be assigned, the whole set (repeatable)")
	flags.BoolVar(&noAssignees, "no-assignees", false, "Assign nobody")
	flags.StringArrayVar(&notified, "notify-when-done", nil, "Person to tell when an occurrence is done, the whole set (repeatable)")
	flags.BoolVar(&nobodyNotified, "no-notify-when-done", false, "Tell nobody when an occurrence is done")
	flags.StringVar(&expect, "expect", "", "The updatedAt of the series you read")
	flags.BoolVar(&force, "force", false, "Write over the series whatever stands")
	return cmd
}

func (a *app) newTodoSeriesStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <seriesId>",
		Short: "Stop a series from repeating; the to-dos it made stay",
		Long: `Stops a series, as "Stop repeating" on the web does. It is not deleted: the
to-dos it made stay ordinary to-dos, and todos series resume lets it repeat
again. A finished series can be stopped as well. One stopped already is exit 9
todoSeriesStopped.`,
		Args: requireArgs("seriesId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			series, err := s.client.StopTodoSeries(a.ctx, args[0])
			if err != nil {
				e := todoSeriesRefusal(s, err, "", args[0])
				if e.APICode == apiCodeTodoSeriesStopped {
					e.Hint = "The series is stopped already, and nothing changed. It repeats again after: minka todos series resume " + args[0]
				}
				return e
			}
			summary := "Series " + series.Value.Title + " is stopped"
			return a.ok(success{
				data:        series.Raw,
				summary:     summary,
				breadcrumbs: todoSeriesBreadcrumbs(series.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newTodoSeriesResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <seriesId>",
		Short: "Let a stopped series repeat again, from its first day after today",
		Long: `Lets a stopped series repeat again. It continues with the first day of its
schedule after today: the days that passed while it was stopped are not made
up. A series that is not stopped, a finished one included, is exit 9
todoSeriesNotStopped.`,
		Args: requireArgs("seriesId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			series, err := s.client.ResumeTodoSeries(a.ctx, args[0])
			if err != nil {
				return todoSeriesRefusal(s, err, "", args[0])
			}
			summary := "Series " + series.Value.Title + " repeats again"
			return a.ok(success{
				data:        series.Raw,
				summary:     summary,
				breadcrumbs: todoSeriesBreadcrumbs(series.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, nextOccurrenceLine(series.Value)) },
			})
		},
	}
}

func (a *app) newTodoSeriesShowNowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-now <seriesId>",
		Short: "Make the upcoming occurrence of a series at once",
		Long: `Makes the upcoming occurrence at once instead of waiting for its lead: a to-do
at the end of the list, due on its day. data is {series, todoId}: series is
the series as Minka answers it, todoId the new to-do, which Minka names in the
Location header alone; it is null where Minka named none.

A series holds at most one occurrence not yet due: while it does, the next one
is exit 9 todoSeriesOccurrenceAhead. A stopped series is todoSeriesStopped, one
past its end todoSeriesFinished. canShowNow in todos series show says
beforehand whether it would make one.`,
		Args: requireArgs("seriesId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			series, err := s.client.ShowTodoSeriesNow(a.ctx, args[0])
			if err != nil {
				return todoSeriesRefusal(s, err, "", args[0])
			}

			todoID, found := todoOfLocation(series.Header.Get("Location"))
			data := map[string]any{"series": series.Raw, "todoId": nil}
			summary := "Made the occurrence of series " + series.Value.Title
			var notices []string
			crumbs := todoSeriesBreadcrumbs(series.Value)
			if found {
				data["todoId"] = todoID
				summary += ": to-do " + todoID
				crumbs = append([]Breadcrumb{{Action: "todo", Cmd: "minka todos show " + todoID, Description: "Read the new to-do"}}, crumbs...)
			} else {
				notices = append(notices, "Minka named no new to-do in its answer. The open to-dos of the list: minka todolists show "+series.Value.ListID)
			}
			return a.ok(success{
				data:        data,
				summary:     summary,
				notices:     notices,
				breadcrumbs: crumbs,
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, nextOccurrenceLine(series.Value)) },
			})
		},
	}
}

// sameMoment says whether two moments as Minka writes them are one, whatever
// form of the offset each was written in.
func sameMoment(one, other string) bool {
	a, errA := time.Parse(time.RFC3339Nano, one)
	b, errB := time.Parse(time.RFC3339Nano, other)
	return errA == nil && errB == nil && a.Equal(b)
}

// todoOfLocation reads the ID of a to-do out of the address Minka names it by.
// Anything else names no to-do.
func todoOfLocation(location string) (string, bool) {
	address, err := url.Parse(location)
	if err != nil {
		return "", false
	}
	_, id, found := strings.Cut(address.Path, "/api/v1/todos/")
	if !found {
		return "", false
	}
	return asID(id)
}

// todoSeriesRefusal classifies a failed request about a series. Each code
// names a state the series, its to-do or its list is in, and the hint says
// what can be done. todoID is the to-do a series is made of, seriesID the
// series written; the other is empty.
func todoSeriesRefusal(s *session, err error, todoID, seriesID string) *Error {
	e := s.fail(err)
	series := firstNonEmpty(seriesID, "<seriesId>")
	switch {
	case e.APICode == apiCodeTodoListArchived:
		e.Hint = "The list lies in the archive of its tool: its series are read, and nothing about them changes and nothing is made until the list is back. minka todos series show " + series + " names the list; bring it back with: minka todolists unarchive <listId>"
	case e.APICode == apiCodeTodoSeriesNeedsDueDay:
		e.Hint = "A to-do repeats from its due day, and this one has none. Give it one first: minka todos edit " + todoID + " --due YYYY-MM-DD"
	case e.APICode == apiCodeTodoRepeatsAlready:
		e.Hint = "The to-do is an occurrence of a series already: minka todos show " + todoID + " names it under series. Change that series with: minka todos series edit <seriesId>"
	case e.APICode == apiCodeTodoToRepeatUnknown:
		e.Hint = "The to-do is no longer an active to-do of the list it was read in: it was moved or put into the trash since. Read it again: minka todos show " + todoID
	case e.APICode == apiCodeTodoSeriesScheduleInvalid:
		e.Hint = todoSeriesScheduleHint
	case e.APICode == apiCodeTodoSeriesDatesNotADate:
		e.Hint = "--starts-on and --ends-on are days written as YYYY-MM-DD."
	case e.APICode == apiCodeTodoSeriesDatesOutOfOrder:
		e.Hint = "--ends-on lies before the first day of the series: its startsOn, the --starts-on given, or for a new series the due day of the to-do."
	case e.APICode == apiCodeTodoTitleMissing:
		e.Hint = `--title "" is no title: a series needs one, and a title can only be replaced, never removed.`
	case e.APICode == apiCodeTodoTitleTooLong:
		e.Hint = "The title is longer than Minka keeps for a to-do; give a shorter --title."
	case e.APICode == apiCodeTodoNotesTooLong:
		e.Hint = "The notes are longer than Minka keeps for a to-do, counted as they would be stored, where escaping adds characters; give shorter --notes."
	case e.APICode == apiCodeTodoNotesNotReadable:
		e.Hint = "Minka cannot read the notes, for example because their markup is nested too deeply; give --notes with flatter markup."
	case e.APICode == apiCodeTodoTitleHasControlCharacters:
		e.Hint = "The title carries a control character, such as a line break or a tab; give --title on one line without them."
	case e.APICode == apiCodeTodoSeriesLeadDaysInvalid:
		e.Hint = "--lead-days is 0 to 366."
	case e.APICode == apiCodePersonNotAProjectMember:
		e.Hint = todoSeriesPeopleHint
	case e.APICode == apiCodeTodoSeriesStopped:
		e.Hint = "The series is stopped and makes nothing until it is resumed: minka todos series resume " + series
	case e.APICode == apiCodeTodoSeriesNotStopped:
		e.Hint = "Only a stopped series is resumed. This one is running, or finished: past its end it has no further day, and a later end gives it more: minka todos series edit " + series + " --ends-on YYYY-MM-DD --expect <updatedAt>"
	case e.APICode == apiCodeTodoSeriesFinished:
		e.Hint = "The series has no further day before its end (schedule.endsOn), so there is nothing to make. A later end gives it more: minka todos series edit " + series + " --ends-on YYYY-MM-DD --expect <updatedAt>"
	case e.APICode == apiCodeTodoSeriesOccurrenceAhead:
		e.Hint = "The series holds an occurrence not yet due, and makes the next one only after that day: it stands among the open to-dos of the list. minka todos series show " + series + " names the list and the day after it."
	case e.Code == CodeNotFound && seriesID != "":
		e.Hint = todoSeriesNotFoundHint
	case e.Code == CodeNotFound:
		e.Hint = todoListNotFoundHint
	}
	return e
}

func todoSeriesSummary(series api.TodoSeries) string {
	return "Series " + series.Title + " (" + series.State + ") in list " + series.List.Title + ", repeats " + describeRepetition(series.Schedule)
}

func todoSeriesBreadcrumbs(series api.TodoSeries) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka todos series show " + series.ID, Description: "Read the series with its schedule and its next day"},
		{Action: "edit", Cmd: "minka todos series edit " + series.ID + " --expect " + series.UpdatedAt, Description: "Change the series as it was read"},
	}
	switch {
	case series.State == "stopped":
		crumbs = append(crumbs, Breadcrumb{Action: "resume", Cmd: "minka todos series resume " + series.ID, Description: "Let the series repeat again"})
	default:
		if series.CanShowNow {
			crumbs = append(crumbs, Breadcrumb{Action: "show-now", Cmd: "minka todos series show-now " + series.ID, Description: "Make the upcoming occurrence at once"})
		}
		crumbs = append(crumbs, Breadcrumb{Action: "stop", Cmd: "minka todos series stop " + series.ID, Description: "Stop the series from repeating"})
	}
	return append(crumbs, Breadcrumb{Action: "list", Cmd: "minka todolists show " + series.ListID, Description: "Read the list of the series"})
}

// nextOccurrenceLine says when the next occurrence comes, as Minka counted it.
func nextOccurrenceLine(series api.TodoSeries) string {
	switch {
	case series.State == "stopped":
		return "Next: none while it is stopped"
	case series.NextOccursOn == nil:
		return "Next: none, the series is past its end"
	case series.NextShowsOn != nil:
		return "Next: " + *series.NextOccursOn + ", in the list from " + *series.NextShowsOn
	}
	return "Next: " + *series.NextOccursOn
}

var monthNames = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

var instanceNames = map[int]string{1: "first", 2: "second", 3: "third", 4: "fourth", -1: "last"}

// describeRepetition says a schedule in words. A rhythm, a weekday or an
// instance this CLI has no words for is named as Minka gives it.
func describeRepetition(schedule api.RepetitionSchedule) string {
	weekdayName := func(day int) string {
		if day >= 1 && day <= 7 {
			return weekdayAbbreviations[day-1]
		}
		return strconv.Itoa(day)
	}
	days := make([]string, len(schedule.Weekdays))
	for i, day := range schedule.Weekdays {
		days[i] = weekdayName(day)
	}
	every := func(one, many string) string {
		if schedule.Interval <= 1 {
			return "every " + one
		}
		return fmt.Sprintf("every %d %s", schedule.Interval, many)
	}
	number := func(value *int) string {
		if value == nil {
			return "?"
		}
		return strconv.Itoa(*value)
	}

	var words string
	switch schedule.Rhythm {
	case rhythmDaily:
		words = "daily on " + strings.Join(days, ", ")
	case rhythmWeekly:
		words = every("week", "weeks") + " on " + strings.Join(days, ", ")
	case rhythmMonthlyOnDay:
		words = every("month", "months") + " on day " + number(schedule.Day)
	case rhythmMonthlyOnWeekday:
		instance := number(schedule.Instance)
		if schedule.Instance != nil && instanceNames[*schedule.Instance] != "" {
			instance = instanceNames[*schedule.Instance]
		}
		weekday := "?"
		if schedule.Weekday != nil {
			weekday = weekdayName(*schedule.Weekday)
		}
		words = every("month", "months") + " on the " + instance + " " + weekday
	case rhythmYearly:
		month := number(schedule.Month)
		if schedule.Month != nil && *schedule.Month >= 1 && *schedule.Month <= 12 {
			month = monthNames[*schedule.Month-1]
		}
		words = "every year on " + number(schedule.Day) + " " + month
	default:
		words = schedule.Rhythm
	}
	words += ", from " + schedule.StartsOn
	if schedule.EndsOn != nil {
		words += " until " + *schedule.EndsOn
	}
	return words
}

func renderTodoSeriesList(w io.Writer, series []api.TodoSeries) {
	if len(series) == 0 {
		fmt.Fprintln(w, "No series.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTATE\tNEXT\tTITLE\tREPEATS")
	for _, one := range series {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", one.ID, one.State, firstNonEmpty(deref(one.NextOccursOn), "-"), one.Title, describeRepetition(one.Schedule))
	}
	_ = table.Flush()
}

func renderTodoSeries(w io.Writer, series api.TodoSeries) {
	fmt.Fprintf(w, "# %s\n", series.Title)
	fmt.Fprintf(w, "Series: %s\nList: %s (%s)\n", series.ID, series.List.Title, series.ListID)
	if series.Group != nil {
		fmt.Fprintf(w, "Group: %s (%s)\n", series.Group.Title, series.Group.ID)
	}
	fmt.Fprintf(w, "Tool: %s (%s)\nProject: %s (%s)\n", series.Tool.Title, series.ToolID, series.Project.Name, series.ProjectID)
	fmt.Fprintf(w, "Repeats: %s\n", describeRepetition(series.Schedule))
	fmt.Fprintf(w, "Lead: in the list %s ahead\n", plural(series.LeadDays, "day", "days"))
	switch {
	case series.State == "stopped" && series.EndedAt != nil:
		fmt.Fprintf(w, "State: stopped on %s\n", formatTime(*series.EndedAt))
	default:
		fmt.Fprintf(w, "State: %s\n", series.State)
	}
	fmt.Fprintln(w, nextOccurrenceLine(series))
	if series.CanShowNow {
		fmt.Fprintln(w, "Show now: possible")
	} else {
		fmt.Fprintln(w, "Show now: not possible")
	}
	if len(series.Assignees) > 0 {
		fmt.Fprintf(w, "Assigned: %s\n", assigneeNames(series.Assignees))
	}
	if len(series.Notified) > 0 {
		fmt.Fprintf(w, "Told when done: %s\n", assigneeNames(series.Notified))
	}
	fmt.Fprintf(w, "Created: %s by %s\nURL: %s\n", formatTime(series.CreatedAt), series.Creator.Name, series.AppURL)
	fmt.Fprintf(w, "To write over it: --expect %s\n", series.UpdatedAt)
	if notes := richtext.ToMarkdown(firstNonEmpty(series.NotesSource, series.Notes)); notes != "" {
		fmt.Fprintf(w, "\n## Notes\n\n%s\n", notes)
	}
	renderEntryFiles(w, series.Attachments)
}
