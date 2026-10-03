package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a span or an event it will not take.
const (
	apiCodeEventsSpanNotADay               = "eventsSpanNotADay"
	apiCodeEventsSpanOutOfOrder            = "eventsSpanOutOfOrder"
	apiCodeEventsSpanTooLong               = "eventsSpanTooLong"
	apiCodeEventTitleMissing               = "eventTitleMissing"
	apiCodeEventTitleTooLong               = "eventTitleTooLong"
	apiCodeEventTitleHasControlCharacters  = "eventTitleHasControlCharacters"
	apiCodeEventTimeMissing                = "eventTimeMissing"
	apiCodeEventTimeAmbiguous              = "eventTimeAmbiguous"
	apiCodeEventTimeNotReadable            = "eventTimeNotReadable"
	apiCodeEventEndsBeforeItStarts         = "eventEndsBeforeItStarts"
	apiCodeEventJoinURLNotAnAddress        = "eventJoinUrlNotAnAddress"
	apiCodeEventDescriptionTooLong         = "eventDescriptionTooLong"
	apiCodeEventDescriptionNotReadable     = "eventDescriptionNotReadable"
	apiCodeEventRepetitionScheduleInvalid  = "eventRepetitionScheduleInvalid"
	apiCodeEventRepetitionDatesNotADate    = "eventRepetitionDatesNotADate"
	apiCodeEventRepetitionDatesOutOfOrder  = "eventRepetitionDatesOutOfOrder"
	apiCodeEventRepetitionTimeZoneUnknown  = "eventRepetitionTimeZoneUnknown"
	apiCodeEventRepetitionDoesNotFit       = "eventRepetitionDoesNotFit"
	apiCodeEventRepetitionKeepsItsFirstDay = "eventRepetitionKeepsItsFirstDay"
)

const calendarNotFoundHint = "There is no such calendar, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. The calendars of a project: minka events tools <projectId>"

const eventNotFoundHint = "There is no such event, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. The events of a calendar: minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD"

const eventTimeHelp = `The time is given in one of two ways. A timed event takes --starts-at and
--ends-at, both, each a moment in ISO 8601 with seconds and an offset, such as
2026-10-05T09:00:00+02:00 or 2026-10-05T07:00:00Z; Minka keeps it in UTC and
answers it that way. A moment without an offset is refused, since there is no
zone to assume for it. An all-day event takes --first-day and, for more than
one day, --last-day, both YYYY-MM-DD and both included; a day has no zone and
is the same day for everybody.`

const eventScheduleHelp = `An event repeats by a schedule. On create, --repeat alone repeats it every
week on the weekday of its first day; on edit, it keeps the stored schedule
from the first day. The details below set another rhythm. The series is one
event: its time is that of the first occurrence, and its first day is the
first day of the series, whatever the details say. They have to fall on that
day, or Minka refuses the schedule with eventRepetitionDoesNotFit.

--rhythm is one of Minka's five words, and each takes its own details:

  daily             --weekdays (every week, on each weekday given)
  weekly            --weekdays, --interval (every n-th week)
  monthlyOnDay      --day, --interval (every n-th month)
  monthlyOnWeekday  --weekday, --instance (1 to 4, or last), --interval
  yearly            --month, --day

--weekdays is repeatable and takes a comma list; a weekday is 1 (Monday) to 7
or a name such as mon. A daily series falls on Monday to Friday unless
--weekdays says otherwise, so one that starts on a weekend needs them. A
detail the rhythm does not use is ignored by Minka: with --rhythm given, the
CLI refuses it instead, and without, it says so in a notice. --ends-on is the
last day an occurrence may fall on.

A timed series keeps its clock time in a time zone: 10:00 in Europe/Berlin
stays 10:00 there across a change of summer time, so its moments in UTC move
by an hour. --time-zone names that zone as an IANA name such as Europe/Berlin;
left out, it is the zone the series has already, else your own from your
preferences in Minka. Minka reads it only together with a schedule, and an
all-day series keeps none.

The CLI works out no day of a series: the occurrences are what Minka answers.`

const eventParticipantsHelp = `--participant names a member of the project who takes part; it can be given
more than once. ` + personHelp + ` Only somebody the event is shown to can take
part: an event added by the team is shown to no client of the project, so
naming a client, or an agent working for one, is refused with
personCannotSeeIt. Whoever is put on is told so by Minka, and the writer
follows the event, taking part or not.`

func (a *app) newEventsCmd() *cobra.Command {
	return group("events", "Read and write the events of a calendar, one-off, all-day and repeating",
		a.newEventsToolsCmd(),
		a.newEventsListCmd(),
		a.newEventsShowCmd(),
		a.newEventsCreateCmd(),
		a.newEventsEditCmd(),
		a.newEventsDeleteCmd(),
		a.newItemCommentsCmd(eventComments),
	)
}

// eventComments are the comments under an event, a series included: they are
// on the event as a whole. Only the archive of its project closes an event to
// them, and events show carries only their number.
var eventComments = commentedKind{
	parent: "events", noun: "event", aNoun: "an event", arg: "eventId", kind: api.CommentedEvent, showOmitsComments: true,
	read: func(a *app, s *session, id string) (commentedItem, error) {
		event, err := s.client.Event(a.ctx, id)
		if err != nil {
			return commentedItem{}, err
		}
		comments, err := s.client.CommentsOn(a.ctx, api.Commented{Kind: api.CommentedEvent, ID: id})
		if err != nil {
			return commentedItem{}, err
		}
		return commentedItem{projectID: event.Value.ProjectID, closed: closedIf(false), comments: comments.Value}, nil
	},
	refusal: func(s *session, err error, _ string) *Error { return eventRefusal(s, err, eventNotFoundHint) },
}

func (a *app) newEventsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the calendars of a project",
		Long: `Lists the calendars of a project that are in use, in the order of the project
page. A project can have more than one. A calendar no longer in use is not
listed and is still read and written at its own ID. Its events are read with
"events list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			calendars, err := s.client.Calendars(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			return a.ok(success{
				data:    calendars.Raw,
				summary: plural(len(calendars.Value), "calendar", "calendars"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD", Description: "List the events of a calendar between two days"},
				},
				text: func(w io.Writer) { renderCalendars(w, calendars.Value) },
			})
		},
	}
}

func (a *app) newEventsListCmd() *cobra.Command {
	var from, to string

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List the events of a calendar between two days",
		Long: `Lists every event with an occurrence between --from and --to, both included and
both required: at most 366 days, and nothing is paged. data is the span as
Minka answers it: {from, to, timeZone, events}.

The days are yours: those of the time zone of your preferences in Minka, else
the installation's, which timeZone names. A timed occurrence counts when a
part of it lies in the span, so one that started the evening before reaches
in; an all-day one counts when one of its days does.

Every event stands in the span once, a series too: occurrences holds the times
it takes place at in the span, in the order they start, and is never empty.
The event's own startsAt, endsAt, firstDay and lastDay are its first
occurrence, which for a series may lie long before the span. An agenda is
read from the occurrences, not from the lines. The events come in the order
of their first occurrence in the span. A line carries no description; events
show reads one.

The text view shows times in the local time of this machine.`,
		Example: `  minka events list 0199... --from 2026-10-19 --to 2026-10-25 --json
  minka events list 0199... --from 2026-10-01 --to 2026-12-31`,
		Args: requireArgs("toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("from") || !cmd.Flags().Changed("to") {
				return usageErrorf(cmd, "a span needs both --from and --to, each a day written as YYYY-MM-DD")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			span, err := s.client.Events(a.ctx, args[0], from, to)
			if err != nil {
				return eventRefusal(s, err, calendarNotFoundHint)
			}
			return a.ok(success{
				data:    span.Raw,
				summary: fmt.Sprintf("%s from %s to %s, days of %s", plural(len(span.Value.Events), "event", "events"), span.Value.From, span.Value.To, span.Value.TimeZone),
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka events show <eventId>", Description: "Read an event with its description"},
					{Action: "create", Cmd: "minka events create --tool " + args[0] + " --title <title> --first-day YYYY-MM-DD", Description: "Add an event to the calendar"},
				},
				text: func(w io.Writer) { renderEventSpan(w, span.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "First day of the span as YYYY-MM-DD")
	cmd.Flags().StringVar(&to, "to", "", "Last day of the span as YYYY-MM-DD, at most 366 days in all")
	return cmd
}

func (a *app) newEventsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <eventId>",
		Short: "Show an event with its time, its repetition and its description",
		Long: `Shows an event: its calendar, its time, its schedule where it repeats, its
link to join, who takes part, and its description. In text output the
description is converted to Markdown and the times to the local time of this
machine; in JSON the description is the HTML the server stores and the
moments are in UTC.

commentCount counts the comments, which "events comments list" reads; they
are on the event as a whole, a series included.`,
		Args: requireArgs("eventId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			event, err := s.client.Event(a.ctx, args[0])
			if err != nil {
				return eventRefusal(s, err, eventNotFoundHint)
			}
			summary := "Event " + event.Value.Title + " in " + event.Value.Tool.Title + ", " + describeEventTime(event.Value.EventTime)
			return a.ok(eventSuccess(event, summary, nil, func(w io.Writer) { renderEvent(w, event.Value) }))
		},
	}
}

// eventFlags are the flags that say something about an event. Only those
// given go out.
type eventFlags struct {
	title, startsAt, endsAt, firstDay, lastDay string
	description, descriptionFormat, joinURL    string
	timeZone                                   string
	highlighted, repeat, noRepeat              bool
	noParticipants, dropAttachments            bool
	participants                               []string
	schedule                                   scheduleFlags
}

func (f *eventFlags) register(cmd *cobra.Command, editing bool) {
	flags := cmd.Flags()
	if editing {
		flags.StringVar(&f.title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	} else {
		flags.StringVar(&f.title, "title", "", "Title of the event")
	}
	flags.StringVar(&f.startsAt, "starts-at", "", "Start of a timed event in ISO 8601 with an offset, such as 2026-10-05T09:00:00+02:00")
	flags.StringVar(&f.endsAt, "ends-at", "", "End of a timed event in ISO 8601 with an offset")
	flags.StringVar(&f.firstDay, "first-day", "", "First day of an all-day event as YYYY-MM-DD")
	flags.StringVar(&f.lastDay, "last-day", "", "Last day of an all-day event as YYYY-MM-DD, the first day unless given")
	flags.StringVar(&f.description, "description", "", "Description, - to read it from stdin")
	flags.StringVar(&f.descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	flags.StringVar(&f.joinURL, "join-url", "", "Address to join the event at, http:// or https://")
	flags.BoolVar(&f.highlighted, "highlighted", false, "Highlight the event in the calendar")
	flags.StringArrayVar(&f.participants, "participant", nil, "Person taking part: ID, me, e-mail address or name (repeatable)")
	flags.BoolVar(&f.repeat, "repeat", false, "Let the event repeat, weekly on its first day unless the details say otherwise")
	flags.StringVar(&f.timeZone, "time-zone", "", "IANA zone a timed series keeps its clock time in, such as Europe/Berlin")
	f.schedule.register(cmd, false, false)
	if editing {
		flags.BoolVar(&f.noRepeat, "no-repeat", false, "Stop the event repeating; it stays where its first occurrence is")
		flags.BoolVar(&f.noParticipants, "no-participants", false, "Take everybody off the event")
		flags.BoolVar(&f.dropAttachments, "drop-attachments", false, "Replace a description that has attachments, mentions or references, which drops them")
	}
}

// input turns the flags given into what goes out. A contradiction between
// flags, and a flag Minka would ignore without a word, are refused here; what
// a value is worth is Minka's to say.
func (f *eventFlags) input(cmd *cobra.Command, a *app, editing bool) (api.EventInput, []string, error) {
	flags := cmd.Flags()
	var in api.EventInput
	var notices []string
	if err := checkDescriptionFormat(cmd, f.descriptionFormat); err != nil {
		return in, nil, err
	}
	if !editing || flags.Changed("title") {
		in.Title = &f.title
	}

	moments := flags.Changed("starts-at") || flags.Changed("ends-at")
	days := flags.Changed("first-day") || flags.Changed("last-day")
	switch {
	case moments && days:
		return in, nil, usageErrorf(cmd, "--starts-at and --ends-at make a timed event, --first-day and --last-day an all-day one; give one of the two")
	case moments && !(flags.Changed("starts-at") && flags.Changed("ends-at")):
		return in, nil, usageErrorf(cmd, "--starts-at and --ends-at go together: a change of the time names the whole time anew")
	case days && !flags.Changed("first-day"):
		return in, nil, usageErrorf(cmd, "--last-day goes with --first-day: a change of the time names the whole time anew")
	case !editing && !moments && !days:
		return in, nil, usageErrorf(cmd, "an event needs a time: --starts-at and --ends-at, or --first-day")
	}
	if moments {
		in.StartsAt, in.EndsAt = &f.startsAt, &f.endsAt
	}
	if days {
		in.FirstDay = &f.firstDay
		if flags.Changed("last-day") {
			in.LastDay = &f.lastDay
		}
	}

	if flags.Changed("description") {
		html, warnings, err := a.richTextHTML("the description", "--description", f.description, f.descriptionFormat)
		if err != nil {
			return in, nil, err
		}
		in.Description, notices = &html, warnings
	}
	if flags.Changed("join-url") {
		in.JoinURL = &f.joinURL
	}
	if flags.Changed("highlighted") {
		in.Highlighted = &f.highlighted
	}
	if f.noParticipants && len(f.participants) > 0 {
		return in, nil, usageErrorf(cmd, "--participant and --no-participants contradict each other")
	}
	if f.noParticipants {
		in.ParticipantIDs = &[]string{}
	}

	schedule, _, err := f.schedule.input(cmd)
	if err != nil {
		return in, nil, err
	}
	repeats := f.repeat || !schedule.IsEmpty()
	switch {
	case f.noRepeat && repeats:
		return in, nil, usageErrorf(cmd, "--no-repeat contradicts --repeat and the details of a schedule")
	case flags.Changed("time-zone") && !repeats:
		return in, nil, usageErrorf(cmd, "Minka reads --time-zone only together with a schedule: give --repeat or a detail of it as well")
	case flags.Changed("time-zone") && days:
		return in, nil, usageErrorf(cmd, "an all-day series keeps no time zone; --time-zone goes with --starts-at and --ends-at")
	}
	if repeats {
		in.Schedule = &schedule
	}
	in.StopsRepeating = f.noRepeat
	if flags.Changed("time-zone") {
		in.TimeZone = &f.timeZone
	}
	return in, notices, nil
}

// noticesOfTheAnswer say what Minka ignored of what was sent, which only its
// answer tells: details of a schedule the rhythm it kept does not use, and a
// zone for an event that turned out to be all-day.
func noticesOfTheAnswer(in api.EventInput, event api.Event) []string {
	var notices []string
	if in.Schedule != nil && event.Schedule != nil {
		notices = append(notices, ignoredDetailsNotice(*in.Schedule, *event.Schedule)...)
	}
	if in.TimeZone != nil && event.AllDay {
		notices = append(notices, "Minka ignored --time-zone: the event is all-day, and an all-day series keeps no time zone")
	}
	return notices
}

func (a *app) newEventsCreateCmd() *cobra.Command {
	var toolID string
	var f eventFlags

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add an event to a calendar, once, all-day or repeating",
		Long: "Adds an event to a calendar. What the team adds is shown to no client of the\n" +
			"project; what a client adds is shown to clients.\n\n" +
			eventTimeHelp + "\n\n" +
			eventScheduleHelp + "\n\n" +
			eventParticipantsHelp + `

--description is Markdown by default and is sent as HTML, the way the
description of a card is; - reads it from stdin. --join-url is the address to
join the event at, starting with http:// or https://. --highlighted marks the
event in the calendar.`,
		Example: `  minka events create --tool 0199... --title "Acceptance" --starts-at 2026-10-21T14:00:00+02:00 --ends-at 2026-10-21T15:00:00+02:00 --json
  minka events create --tool 0199... --title "Trade fair" --first-day 2026-10-20 --last-day 2026-10-22 --highlighted
  minka events create --tool 0199... --title "Weekly planning" --starts-at 2026-10-19T10:00:00+02:00 --ends-at 2026-10-19T10:30:00+02:00 --repeat --time-zone Europe/Berlin --participant me`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			input, notices, err := f.input(cmd, a, false)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if len(f.participants) > 0 {
				ids, err := a.peopleOfATool(s, func() (string, error) {
					calendar, err := s.client.Calendar(a.ctx, toolID)
					if err != nil {
						return "", toolRefusal(s, err, calendarNotFoundHint)
					}
					return calendar.Value.ProjectID, nil
				}, f.participants)
				if err != nil {
					return err
				}
				input.ParticipantIDs = &ids
			}

			event, err := s.client.CreateEvent(a.ctx, toolID, input)
			if err != nil {
				return eventRefusal(s, err, calendarNotFoundHint)
			}
			summary := "Added event " + event.Value.Title + " to " + event.Value.Tool.Title + ", " + describeEventTime(event.Value.EventTime)
			return a.ok(eventSuccess(event, summary, append(notices, noticesOfTheAnswer(input, event.Value)...), func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nEvent: %s\n%s\n", summary, event.Value.ID, event.Value.AppURL)
			}))
		},
	}
	cmd.Flags().StringVar(&toolID, "tool", "", "ID of the calendar the event goes into")
	f.register(cmd, false)
	return cmd
}

func (a *app) newEventsEditCmd() *cobra.Command {
	var f eventFlags

	cmd := &cobra.Command{
		Use:   "edit <eventId>",
		Short: "Change the title, the time, the repetition, the people or the description of an event",
		Long: `Changes what is given in one write; what is not given stays as it is. There is
no guard: the last write wins, so a change somebody else made meanwhile is
written over where this one names the same thing.

--title "" does not empty the title: an event needs one, so an empty title
exits 9 (validation) with meta.apiCode eventTitleMissing. --description ""
empties the description, --join-url "" takes the link off, and
--highlighted=false takes the highlight off.

` + eventTimeHelp + ` A change that names any of the four flags names the
whole time anew, and may turn a timed event into an all-day one and back.

` + eventScheduleHelp + `

What becomes of the repetition follows what is given. Without --repeat and
without a detail of a schedule, a series repeats as before and an event that
does not repeat stays so; such a change cannot move the first day of a series
or turn it between timed and all-day (eventRepetitionKeepsItsFirstDay), while
moving the clock time on the same day is fine. To move a series, give the new
time with --repeat, and with the details that hold the new first day where
the stored ones do not. --repeat and the details set the schedule anew: what
is not given is taken from the stored schedule while rhythm and first day
stay, else from the first day. --no-repeat stops the repetition: the event
stays where its first occurrence is. A single occurrence cannot be changed.

--participant gives the WHOLE set of people: whoever takes part and is not
named again is taken off, the others keep their place and newcomers follow,
among themselves in no promised order. --no-participants takes everybody off. ` + personHelp + `

Files and mentions stand in a description as action-text-attachment elements,
which Markdown cannot hold. To keep them, take descriptionSource from events
show --json, change it and send it back with --description-format html. A new
description that would drop them is refused unless --drop-attachments is
given; the command reads the event first to know.

A change that asks for what stands already succeeds and writes nothing.`,
		Example: `  minka events edit 0199... --title "Acceptance with the client"
  minka events edit 0199... --starts-at 2026-10-27T10:00:00+01:00 --ends-at 2026-10-27T10:30:00+01:00 --repeat --rhythm weekly --weekdays tue
  minka events edit 0199... --no-repeat
  minka events edit 0199... --participant me --participant anke@example.com`,
		Args: requireArgs("eventId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, notices, err := f.input(cmd, a, true)
			if err != nil {
				return err
			}
			if input.Title == nil && input.StartsAt == nil && input.FirstDay == nil && input.Description == nil && input.JoinURL == nil &&
				input.Highlighted == nil && input.ParticipantIDs == nil && len(f.participants) == 0 && input.Schedule == nil && !input.StopsRepeating {
				return usageErrorf(cmd, "nothing to change, give --title, the time, --description, --join-url, --highlighted, --participant, --repeat, a detail of a schedule or --no-repeat")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			var current *api.Event
			read := func() (*api.Event, error) {
				if current == nil {
					event, err := s.client.Event(a.ctx, args[0])
					if err != nil {
						return nil, eventRefusal(s, err, eventNotFoundHint)
					}
					current = &event.Value
				}
				return current, nil
			}
			if len(f.participants) > 0 {
				people := newProjectPeople(a, s, func() (string, error) {
					event, err := read()
					if err != nil {
						return "", err
					}
					return event.ProjectID, nil
				})
				ids, err := people.resolve(f.participants)
				if err != nil {
					return err
				}
				input.ParticipantIDs = &ids
			}
			if input.Description != nil {
				event, err := read()
				if err != nil {
					return err
				}
				dropped, err := a.guardOverFiles(s, event.ProjectID, closedIf(false), firstNonEmpty(event.DescriptionSource, event.Description), *input.Description, f.dropAttachments,
					"the description has %s, and the new description would drop them",
					"Change descriptionSource from events show --json and send it back with --description-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			event, err := s.client.UpdateEvent(a.ctx, args[0], input)
			if err != nil {
				return eventRefusal(s, err, eventNotFoundHint)
			}
			summary := "Changed event " + event.Value.Title + ", " + describeEventTime(event.Value.EventTime)
			return a.ok(eventSuccess(event, summary, append(notices, noticesOfTheAnswer(input, event.Value)...), func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n%s\n", summary, event.Value.AppURL)
			}))
		},
	}
	f.register(cmd, true)
	return cmd
}

func (a *app) newEventsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <eventId>",
		Aliases: []string{"trash"},
		Short:   "Move an event to the trash, a series with all its days",
		Long: `Moves an event to the trash, as removing it on the web does; a series goes with
all its days, since a single occurrence cannot be left out. A member of the
team may remove any event, a client only one of their own: anybody else's
exits 2 (not_found) for a client.

An event in the trash reads like one that never existed: showing, changing or
deleting it again exits 2. It comes back in Minka on the web only.`,
		Args: requireArgs("eventId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteEvent(a.ctx, args[0]); err != nil {
				return eventRefusal(s, err, eventNotFoundHint)
			}
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: "Moved event " + args[0] + " to the trash",
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Moved event %s to the trash.\n", args[0])
				},
			})
		},
	}
}

// eventRefusal classifies a failed request about a calendar, a span or an
// event; the hint says what can be done, including where that is nothing.
func eventRefusal(s *session, err error, notFoundHint string) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodeEventsSpanNotADay:
		e.Hint = "--from and --to are days written as YYYY-MM-DD, and both are needed."
	case apiCodeEventsSpanOutOfOrder:
		e.Hint = "--to lies before --from; a span of one day names the same day twice."
	case apiCodeEventsSpanTooLong:
		e.Hint = "A span holds at most 366 days, both ends counted. Read a longer time in several spans."
	case apiCodeEventTitleMissing:
		e.Hint = `An event needs a title: --title "" can only replace it, never remove it.`
	case apiCodeEventTitleTooLong:
		e.Hint = "The title is longer than Minka keeps for an event; give a shorter --title."
	case apiCodeEventTitleHasControlCharacters:
		e.Hint = "The title carries a control character, such as a line break or a tab; give --title on one line without them."
	case apiCodeEventTimeMissing, apiCodeEventTimeAmbiguous:
		e.Hint = "An event is timed, with --starts-at and --ends-at together, or all-day, with --first-day and optionally --last-day, never both. A change that names any of them names the whole time anew."
	case apiCodeEventTimeNotReadable:
		e.Hint = "A moment is ISO 8601 with seconds and an offset of at most 14 hours, such as 2026-10-05T09:00:00+02:00 or 2026-10-05T07:00:00Z, and starts in the year 1 or later once told in UTC; a day is YYYY-MM-DD and has to exist."
	case apiCodeEventEndsBeforeItStarts:
		e.Hint = "--ends-at lies before --starts-at, or --last-day before --first-day. An event may end the moment it starts."
	case apiCodeEventJoinURLNotAnAddress:
		e.Hint = `--join-url is a web address starting with http:// or https://, at most 2000 characters and without spaces; --join-url "" takes the link off.`
	case apiCodeEventDescriptionTooLong:
		e.Hint = "The description is longer than Minka keeps for an event; shorten it."
	case apiCodeEventDescriptionNotReadable:
		e.Hint = "Minka cannot read the description, for example because its markup is nested too deeply; simplify it."
	case apiCodeEventRepetitionScheduleInvalid:
		e.Hint = todoSeriesScheduleHint
	case apiCodeEventRepetitionDatesNotADate:
		e.Hint = "--ends-on is a day written as YYYY-MM-DD."
	case apiCodeEventRepetitionDatesOutOfOrder:
		e.Hint = "--ends-on lies before the first day of the event, which is the first day of the series."
	case apiCodeEventRepetitionTimeZoneUnknown:
		e.Hint = "--time-zone is a name of the IANA zone database such as Europe/Berlin, written with its capitals; an offset such as +02:00 is none. Without --time-zone, a timed series keeps the zone it has, else takes yours from your preferences in Minka, which may be none Minka can keep a series in: give --time-zone then."
	case apiCodeEventRepetitionDoesNotFit:
		e.Hint = "The schedule does not fall on the first day of the event, such as a weekly one on other weekdays. Give the details that hold that day, or leave them out to take them from it."
	case apiCodeEventRepetitionKeepsItsFirstDay:
		e.Hint = "The event repeats, and a change without a schedule keeps the first day of the series and whether it is all-day. To move it, give the new time with --repeat, and with the details that hold the new first day where the stored ones do not; --no-repeat stops the repetition instead."
	case apiCodePersonNotAProjectMember:
		e.Hint = "Only members of the project can take part in an event of it. The members: minka people list <projectId>"
	case apiCodePersonCannotSeeIt:
		e.Hint = "Nothing was written, and the same request will be refused again: somebody named with --participant is a client of the project, or an agent working for one, and the event is not shown to clients. An event added by the team is shown to no client; name somebody else."
	default:
		if e.Code == CodeNotFound {
			e.Hint = notFoundHint
		}
	}
	return e
}

func eventSuccess(event api.Result[api.Event], summary string, notices []string, text func(io.Writer)) success {
	e := event.Value
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka events show " + e.ID, Description: "Read the event with its description"},
		{Action: "comments", Cmd: "minka events comments list " + e.ID, Description: "Read the comments on the event"},
		{Action: "edit", Cmd: "minka events edit " + e.ID + " --title <title>", Description: "Change the event"},
		{Action: "delete", Cmd: "minka events delete " + e.ID, Description: "Move the event to the trash"},
		{Action: "list", Cmd: "minka events list " + e.ToolID + " --from YYYY-MM-DD --to YYYY-MM-DD", Description: "List the events of its calendar between two days"},
	}
	return success{data: event.Raw, summary: summary, notices: notices, breadcrumbs: crumbs, text: text}
}

// describeEventTime says when an event or one occurrence of it takes place,
// a moment in the local time of this machine.
func describeEventTime(t api.EventTime) string {
	if t.FirstDay != nil {
		if t.LastDay == nil || *t.LastDay == *t.FirstDay {
			return *t.FirstDay + ", all day"
		}
		return *t.FirstDay + " to " + *t.LastDay + ", all day"
	}
	start, end := deref(t.StartsAt), deref(t.EndsAt)
	startsAt, errStart := time.Parse(time.RFC3339Nano, start)
	endsAt, errEnd := time.Parse(time.RFC3339Nano, end)
	if errStart != nil || errEnd != nil {
		return start + " to " + end
	}
	startsAt, endsAt = startsAt.Local(), endsAt.Local()
	if startsAt.Format("2006-01-02") == endsAt.Format("2006-01-02") {
		return startsAt.Format("Mon 2006-01-02 15:04") + " to " + endsAt.Format("15:04")
	}
	return startsAt.Format("Mon 2006-01-02 15:04") + " to " + endsAt.Format("Mon 2006-01-02 15:04")
}

func renderCalendars(w io.Writer, calendars []api.Calendar) {
	if len(calendars) == 0 {
		fmt.Fprintln(w, "No calendars.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE")
	for _, calendar := range calendars {
		fmt.Fprintf(table, "%s\t%s\n", calendar.ID, calendar.Title)
	}
	_ = table.Flush()
}

func renderEventSpan(w io.Writer, span api.EventSpan) {
	fmt.Fprintf(w, "From %s to %s, days of %s.\n", span.From, span.To, span.TimeZone)
	if len(span.Events) == 0 {
		fmt.Fprintln(w, "No events.")
		return
	}
	for _, event := range span.Events {
		var details []string
		if event.Schedule != nil {
			details = append(details, "repeats "+describeRepetition(*event.Schedule))
		}
		if event.Highlighted {
			details = append(details, "highlighted")
		}
		if len(event.Participants) > 0 {
			details = append(details, plural(len(event.Participants), "participant", "participants"))
		}
		if event.CommentCount > 0 {
			details = append(details, plural(event.CommentCount, "comment", "comments"))
		}
		line := fmt.Sprintf("\n- %s  %s", event.ID, event.Title)
		if len(details) > 0 {
			line += " (" + strings.Join(details, ", ") + ")"
		}
		fmt.Fprintln(w, line)
		for _, occurrence := range event.Occurrences {
			fmt.Fprintf(w, "  %s\n", describeEventTime(occurrence))
		}
	}
}

func renderEvent(w io.Writer, event api.Event) {
	fmt.Fprintf(w, "# %s\n", event.Title)
	fmt.Fprintf(w, "Event: %s\nCalendar: %s (%s)\nProject: %s (%s)\n", event.ID, event.Tool.Title, event.ToolID, event.Project.Name, event.ProjectID)
	fmt.Fprintf(w, "When: %s\n", describeEventTime(event.EventTime))
	if event.Schedule != nil {
		repeats := describeRepetition(*event.Schedule)
		if event.TimeZone != nil {
			repeats += ", at the clock time of " + *event.TimeZone
		}
		fmt.Fprintf(w, "Repeats: %s\n", repeats)
	}
	if event.Highlighted {
		fmt.Fprintln(w, "Highlighted: yes")
	}
	if event.JoinURL != nil {
		fmt.Fprintf(w, "Join: %s\n", *event.JoinURL)
	}
	if len(event.Participants) > 0 {
		fmt.Fprintf(w, "Taking part: %s\n", assigneeNames(event.Participants))
	}
	if event.CommentCount > 0 {
		fmt.Fprintf(w, "Comments: %d, read with minka events comments list %s\n", event.CommentCount, event.ID)
	}
	fmt.Fprintf(w, "Created: %s by %s\nURL: %s\n", formatTime(event.CreatedAt), event.Creator.Name, event.AppURL)
	if description := richtext.ToMarkdown(firstNonEmpty(event.DescriptionSource, event.Description)); description != "" {
		fmt.Fprintf(w, "\n## Description\n\n%s\n", description)
	}
	renderEntryFiles(w, event.Attachments)
}
