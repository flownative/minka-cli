package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a question or an answer it will not take.
const (
	apiCodeQuestionWeekdaysMissing = "questionWeekdaysMissing"
	apiCodeQuestionTimeInvalid     = "questionTimeInvalid"
	apiCodeQuestionScheduleInvalid = "questionScheduleInvalid"
	apiCodeQuestionNobodyAsked     = "questionNobodyAsked"
	apiCodeAnswerDayNotADate       = "answerDayNotADate"
	apiCodeAnswerDayNotAsked       = "answerDayNotAsked"
	apiCodeAnswerMissing           = "answerMissing"
	apiCodeAnswersBeforeUnknown    = "answersBeforeUnknown"
)

const checkinsToolNotFoundHint = "There is no such Check-ins tool, it is in the trash, or it belongs to a project you are not a member of. The tools of a project: minka checkins tools <projectId>"

const questionNotFoundHint = "There is no such question, it is in the trash, its tool or project is in the trash, or it belongs to a project you are not a member of. The questions of a tool: minka checkins list <toolId>"

const answerNotFoundHint = "There is no such answer, it or its question is in the trash, or it belongs to a project you are not a member of. The answers to a question: minka checkins answers list <questionId>"

const answerWriterHint = "There is no such answer, it or its question is in the trash, or somebody else wrote it: only its writer can change an answer, and an answer of somebody else is answered as if it were not there."

const askedPeopleHint = "Only members of the project can be asked. The members are listed by: minka people list <projectId>"

const scheduleHelp = `--rhythm is one of Minka's four words: daily (on each weekday given),
weekly, everyOtherWeek and monthly (on the first of the weekday in a month),
and every rhythm but daily takes exactly one --weekday. --weekday is a number
from 1 (Monday) to 7 (Sunday) or a name such as mon or monday; it can be given
more than once or as a list such as mon,wed,fri. --time is HH:MM on the 24-hour
clock and holds in the time zone of each person asked. The rhythm and the
numbers go out as given, and Minka says what it does not take.`

const answerFormatHelp = `The text is Markdown by default and is sent as HTML, keeping every line
break, the way a comment is. --format html sends HTML as it is. Give - as the
text to read it from stdin.`

func (a *app) newCheckinsCmd() *cobra.Command {
	return group("checkins", "Read and write check-in questions, their answers and your reminders",
		a.newCheckinsToolsCmd(),
		a.newCheckinsListCmd(),
		a.newCheckinsShowCmd(),
		a.newCheckinsCreateCmd(),
		a.newCheckinsEditCmd(),
		a.newCheckinsPauseCmd(),
		a.newCheckinsResumeCmd(),
		a.newCheckinsRemindersCmd(),
		a.newCheckinAnswersCmd(),
	)
}

func (a *app) newCheckinsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the Check-ins tools of a project",
		Long: `Lists the Check-ins tools of a project that are in use, in the order of the
project page. The questions of one are read with "checkins list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.CheckinsTools(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "Check-ins tool", "Check-ins tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka checkins list <toolId>", Description: "List the questions of a tool"},
				},
				text: func(w io.Writer) {
					if len(tools.Value) == 0 {
						fmt.Fprintln(w, "No Check-ins tools.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tTITLE")
					for _, tool := range tools.Value {
						fmt.Fprintf(table, "%s\t%s\n", tool.ID, tool.Title)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newCheckinsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <toolId>",
		Short: "List the questions of a Check-ins tool",
		Long: `Lists the questions of a Check-ins tool, the oldest first, each with its
schedule, whether it is paused and the people it asks. Nothing is paged.`,
		Args: requireArgs("toolId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			questions, err := s.client.CheckinQuestions(a.ctx, args[0])
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = checkinsToolNotFoundHint
				}
				return e
			}

			return a.ok(success{
				data:    questions.Raw,
				summary: plural(len(questions.Value), "question", "questions"),
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka checkins show <questionId>", Description: "Read a question"},
					{Action: "answers", Cmd: "minka checkins answers list <questionId>", Description: "Read the answers to a question"},
					{Action: "create", Cmd: "minka checkins create --tool " + args[0] + " --title <question> --rhythm <rhythm> --weekday <day> --time <HH:MM> --ask <personId>", Description: "Set up a question"},
				},
				text: func(w io.Writer) {
					if len(questions.Value) == 0 {
						fmt.Fprintln(w, "No questions.")
						return
					}
					for _, question := range questions.Value {
						paused := ""
						if question.Paused {
							paused = ", paused"
						}
						fmt.Fprintf(w, "- %s  %s (%s%s)\n", question.ID, question.Title, describeSchedule(question.Schedule), paused)
					}
				},
			})
		},
	}
}

func (a *app) newCheckinsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <questionId>",
		Short: "Show a question with its schedule and the people it asks",
		Long: `Shows a question: its schedule, the day it started, whether it is paused, and
the people it asks who are still members and may see it. The answers are read
with "checkins answers list <questionId>".`,
		Args: requireArgs("questionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			question, err := s.client.CheckinQuestion(a.ctx, args[0])
			if err != nil {
				return questionRefusal(s, err, false)
			}
			return a.ok(questionSuccess(question, questionSummary(question.Value), nil, true))
		},
	}
}

func (a *app) newCheckinsCreateCmd() *cobra.Command {
	var toolID, title, rhythm, timeOfDay string
	var weekdays, asked []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Set up a question that asks chosen people on a schedule",
		Long: `Sets up a question in a Check-ins tool. It starts on your today, in your time
zone, and asks nobody at once: the first reminders follow the schedule.

` + scheduleHelp + `

--ask names a person to be asked, by ID or as me, and can be given more than
once. A name or an e-mail address cannot be looked up here, because the ID of
a tool names no project; minka people list <projectId> gives the IDs. Only
members of the project can be asked. Minka lists as asked only the members who
may see the question, so a client of the project it is not shown to is asked
and named nowhere; the command says so in a notice.`,
		Example: `  minka checkins create --tool 0199... --title "What did you do today?" --rhythm daily --weekday mon,tue,wed,thu,fri --time 17:00 --ask me --ask 0199...
  minka checkins create --tool 0199... --title "What are you working on?" --rhythm weekly --weekday mon --time 09:00 --ask me`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			days, err := weekdayNumbers(cmd, weekdays)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			askedIDs, err := a.peopleByIDOrMe(cmd, s, asked)
			if err != nil {
				return err
			}
			input := api.CreateCheckinQuestionInput{Title: title, Rhythm: rhythm, Weekdays: days, TimeOfDay: timeOfDay, AskedIDs: askedIDs}
			question, err := s.client.CreateCheckinQuestion(a.ctx, toolID, input)
			if err != nil {
				e := questionRefusal(s, err, false)
				if e.Code == CodeNotFound {
					e.Hint = checkinsToolNotFoundHint
				}
				return e
			}

			summary := "Set up question " + question.Value.Title + ", " + describeSchedule(question.Value.Schedule)
			return a.ok(questionSuccess(question, summary, askedNotices(askedIDs, question.Value), false))
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the Check-ins tool the question goes into")
	flags.StringVar(&title, "title", "", "The question as it is asked")
	flags.StringVar(&rhythm, "rhythm", "", "How often: daily, weekly, everyOtherWeek or monthly")
	flags.StringSliceVar(&weekdays, "weekday", nil, "Weekday it is asked on: 1 (Monday) to 7, or a name such as mon (repeatable)")
	flags.StringVar(&timeOfDay, "time", "", "Time of day it is asked at, HH:MM on the 24-hour clock")
	flags.StringArrayVar(&asked, "ask", nil, "Person to be asked: ID or me (repeatable)")

	return cmd
}

func (a *app) newCheckinsEditCmd() *cobra.Command {
	var title, rhythm, timeOfDay string
	var weekdays, asked []string

	cmd := &cobra.Command{
		Use:   "edit <questionId>",
		Short: "Change the wording, the schedule or the people of a question",
		Long: `Changes any of wording, rhythm, weekdays, time and people in one write. What
is not given stays as it is, and so does the day the question started. A
question asked every other week is asked on the first of its weekday on or
after that day and every second week from there, so a new weekday can move it
into the other set of weeks; the command names the first of them in a notice.

` + scheduleHelp + ` The weekdays not given are the question's own, so a
daily question asked on several days needs one --weekday to become weekly.

--weekday and --ask give the WHOLE set: the weekdays or the people not named
again are taken off. ` + personHelp + ` Nobody can be taken off without
somebody else being named: a question asks at least one person.

An edit that names nobody writes back the people Minka shows as asked, so
whoever it does not show is asked no longer: a client the question is not
shown to, and a member who left the project, who is not asked again on coming
back. A question that shows nobody at all takes no change until --ask names
somebody: exit 9 with questionNobodyAsked.`,
		Example: `  minka checkins edit 0199... --title "What did you get done today?"
  minka checkins edit 0199... --rhythm weekly --weekday fri --time 16:00
  minka checkins edit 0199... --ask me --ask anke@example.com`,
		Args: requireArgs("questionId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			var input api.UpdateCheckinQuestionInput
			if flags.Changed("title") {
				input.Title = &title
			}
			if flags.Changed("rhythm") {
				input.Rhythm = &rhythm
			}
			if flags.Changed("time") {
				input.TimeOfDay = &timeOfDay
			}
			if flags.Changed("weekday") {
				days, err := weekdayNumbers(cmd, weekdays)
				if err != nil {
					return err
				}
				input.Weekdays = &days
			}
			if input == (api.UpdateCheckinQuestionInput{}) && len(asked) == 0 {
				return usageErrorf(cmd, "nothing to change, give --title, --rhythm, --weekday, --time or --ask")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			var askedIDs []string
			if len(asked) > 0 {
				people := newProjectPeople(a, s, func() (string, error) {
					question, err := s.client.CheckinQuestion(a.ctx, args[0])
					if err != nil {
						return "", questionRefusal(s, err, false)
					}
					return question.Value.ProjectID, nil
				})
				if askedIDs, err = people.resolve(asked); err != nil {
					return err
				}
				input.AskedIDs = &askedIDs
			}

			question, err := s.client.UpdateCheckinQuestion(a.ctx, args[0], input)
			if err != nil {
				return questionRefusal(s, err, input.AskedIDs == nil)
			}

			notices := askedNotices(askedIDs, question.Value)
			if (input.Rhythm != nil || input.Weekdays != nil) && question.Value.Schedule.Rhythm == "everyOtherWeek" {
				notices = append(notices, fortnightNotice(question.Value.Schedule))
			}
			summary := "Changed question " + question.Value.Title + ", " + describeSchedule(question.Value.Schedule)
			return a.ok(questionSuccess(question, summary, notices, false))
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New wording; an empty one is refused")
	flags.StringVar(&rhythm, "rhythm", "", "How often: daily, weekly, everyOtherWeek or monthly")
	flags.StringSliceVar(&weekdays, "weekday", nil, "Weekday it is asked on, the whole set: 1 (Monday) to 7, or a name (repeatable)")
	flags.StringVar(&timeOfDay, "time", "", "Time of day it is asked at, HH:MM on the 24-hour clock")
	flags.StringArrayVar(&asked, "ask", nil, "Person to be asked, the whole set: ID, me, e-mail address or name (repeatable)")

	return cmd
}

func (a *app) newCheckinsPauseCmd() *cobra.Command {
	return a.newCheckinsSetPausedCmd("pause", "Stop asking a question for now", true)
}

func (a *app) newCheckinsResumeCmd() *cobra.Command {
	return a.newCheckinsSetPausedCmd("resume", "Ask a paused question again", false)
}

func (a *app) newCheckinsSetPausedCmd(name, short string, paused bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <questionId>",
		Short: short,
		Long: short + `. A question that stands that way already stays as it is,
and the command succeeds. The days a question was paused are not asked for
afterwards.`,
		Args: requireArgs("questionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			var question api.Result[api.CheckinQuestion]
			if paused {
				question, err = s.client.PauseCheckinQuestion(a.ctx, args[0])
			} else {
				question, err = s.client.ResumeCheckinQuestion(a.ctx, args[0])
			}
			if err != nil {
				return questionRefusal(s, err, false)
			}

			summary := "Question " + question.Value.Title + " is asked again"
			if question.Value.Paused {
				summary = "Question " + question.Value.Title + " is paused"
			}
			return a.ok(questionSuccess(question, summary, nil, false))
		},
	}
}

func (a *app) newCheckinsRemindersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reminders",
		Short: "List the check-ins you were asked and have not answered yet",
		Long: `Lists the questions you were asked across all projects and have not answered
yet, the one asked last first. Per question only the latest day it asked you
for counts, and it stays open until an answer of yours belongs to that day or
a later one. A paused question, one in the trash, one that no longer asks you
and one of an archived project are not among them.

Answer one for the day it asked for with:
  minka checkins answers create <questionId> - --asked-for <askedFor>

Minka hands over at most 100 and has no page beyond them: where the list is
that long, a notice says there may be more.`,
		Args: requireArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			reminders, err := s.client.MyCheckinReminders(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			var notices []string
			if len(reminders.Value) >= api.CheckinRemindersMax {
				notices = append(notices, fmt.Sprintf("Minka hands over at most %d open reminders and none beyond them, so there may be more: the ones left out are those asked earlier, and answering some of these brings them into view", api.CheckinRemindersMax))
			}
			return a.ok(success{
				data:    reminders.Raw,
				summary: plural(len(reminders.Value), "open reminder", "open reminders"),
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "answer", Cmd: "minka checkins answers create <questionId> - --asked-for <askedFor>", Description: "Answer a reminder for the day it asked for"},
				},
				text: func(w io.Writer) {
					if len(reminders.Value) == 0 {
						fmt.Fprintln(w, "No open reminders.")
						return
					}
					for _, reminder := range reminders.Value {
						fmt.Fprintf(w, "- %s  %s (%s, asked for %s)\n", reminder.Question.ID, reminder.Question.Title, reminder.Project.Name, reminder.AskedFor)
					}
				},
			})
		},
	}
}

func (a *app) newCheckinAnswersCmd() *cobra.Command {
	return group("answers", "Read, write and change answers to check-in questions",
		a.newCheckinAnswersListCmd(),
		a.newCheckinAnswersShowCmd(),
		a.newCheckinAnswersCreateCmd(),
		a.newCheckinAnswersEditCmd(),
	)
}

func (a *app) newCheckinAnswersListCmd() *cobra.Command {
	var person, on, before string
	var limit int

	cmd := &cobra.Command{
		Use:   "list <questionId>",
		Short: "List the answers to a question, a page at a time",
		Long: `Lists the answers to a question, the newest day first and within a day the one
written last first, as a bare array. --person narrows them to the answers of
one person, --on to those for one day, YYYY-MM-DD.

` + personHelp + ` A person given by name, address or as me is
looked up among the members of the project, for which the command reads the
question first.

Minka marks the end of the list by nothing but a page shorter than asked for.
--limit is 1 to 100, 50 unless given; a full page says in a notice where the
next one starts: --before <the last answerId>.`,
		Example: `  minka checkins answers list 0199... --json
  minka checkins answers list 0199... --person me --on 2026-09-11 --json`,
		Args: requireArgs("questionId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > api.CheckinAnswersPageMax {
				return usageErrorf(cmd, "--limit is 1 to %d, not %d: Minka serves no larger page and would cut one down without a word", api.CheckinAnswersPageMax, limit)
			}
			if before != "" {
				if err := requireID(cmd, "--before", &before); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			query := api.CheckinAnswersQuery{On: on, Before: before, Limit: limit}
			if person != "" {
				people := newProjectPeople(a, s, func() (string, error) {
					question, err := s.client.CheckinQuestion(a.ctx, args[0])
					if err != nil {
						return "", questionRefusal(s, err, false)
					}
					return question.Value.ProjectID, nil
				})
				ids, err := people.resolve([]string{person})
				if err != nil {
					return err
				}
				query.PersonID = ids[0]
			}

			answers, err := s.client.CheckinAnswers(a.ctx, args[0], query)
			if err != nil {
				e := questionRefusal(s, err, false)
				switch e.APICode {
				case apiCodeAnswerDayNotADate:
					e.Hint = "--on is a day written as YYYY-MM-DD."
				case apiCodeAnswersBeforeUnknown:
					e.Hint = "--before takes the ID of an answer to this question: the last one of the page before. The first page: minka checkins answers list " + args[0]
				}
				return e
			}

			var notices []string
			crumbs := []Breadcrumb{
				{Action: "show", Cmd: "minka checkins answers show <answerId>", Description: "Read an answer with its comments"},
				{Action: "answer", Cmd: "minka checkins answers create " + args[0] + " -", Description: "Answer the question"},
			}
			if n := len(answers.Value); n == limit {
				next := "minka checkins answers list " + args[0]
				if query.PersonID != "" {
					next += " --person " + query.PersonID
				}
				if on != "" {
					next += " --on " + on
				}
				next += " --before " + answers.Value[n-1].ID
				notices = append(notices, "the page is full, so there may be more: "+next)
				crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: next, Description: "Read the next page"})
			}
			return a.ok(success{
				data:        answers.Raw,
				summary:     plural(len(answers.Value), "answer", "answers"),
				notices:     notices,
				breadcrumbs: crumbs,
				text:        func(w io.Writer) { renderAnswerLines(w, answers.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&person, "person", "", "Only the answers of this person: ID, me, e-mail address or name")
	cmd.Flags().StringVar(&on, "on", "", "Only the answers for this day, YYYY-MM-DD")
	cmd.Flags().StringVar(&before, "before", "", "Start after this answer: the last one of the page before")
	cmd.Flags().IntVar(&limit, "limit", 50, "Number of answers on the page, 1 to 100")

	return cmd
}

func (a *app) newCheckinAnswersShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <answerId>",
		Short: "Show an answer with its files and its comments",
		Long: `Shows an answer: the question, the day it belongs to, who wrote it, its text,
the files in it and the comments under it. In text output the text is
converted to Markdown; in JSON it is the HTML the server stores.`,
		Args: requireArgs("answerId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			answer, err := s.client.CheckinAnswer(a.ctx, args[0])
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = answerNotFoundHint
				}
				return e
			}
			summary := fmt.Sprintf("Answer of %s to %s for %s", answer.Value.Author.Name, answer.Value.Question.Title, answer.Value.GroupOn)
			return a.ok(answerSuccess(answer, summary, nil, func(w io.Writer) { renderAnswer(w, answer.Value) }))
		},
	}
}

func (a *app) newCheckinAnswersCreateCmd() *cobra.Command {
	var format, askedFor string

	cmd := &cobra.Command{
		Use:   "create <questionId> <text|->",
		Short: "Answer a question",
		Long: `Answers a question as the account of the token. Anybody in the project may
answer, as often as they like, the same day as well.

--asked-for names the day of a reminder, YYYY-MM-DD, as askedFor of "checkins
reminders" gives it, and the answer belongs to that day. Minka takes only a
day the question was put to you for: any other is exit 9 with
answerDayNotAsked. Without it the answer belongs to the day of the latest
asking by the question's schedule, in your time zone, whether the question
was paused then or asks you at all, or to your today before its first asking;
the day comes back as groupOn and stays with the answer for good.

` + answerFormatHelp,
		Example: `  minka checkins answers create 0199... "Counted the tents."
  minka checkins answers create 0199... - --asked-for 2026-09-07 < answer.md`,
		Args: requireArgs("questionId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := answerBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the answer", args[1])
			if err != nil {
				return err
			}
			body, notices, err := answerBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			answer, err := s.client.CreateCheckinAnswer(a.ctx, args[0], api.CreateCheckinAnswerInput{Body: body, AskedFor: askedFor})
			if err != nil {
				return questionRefusal(s, err, false)
			}

			summary := fmt.Sprintf("Answered %s for %s", answer.Value.Question.Title, answer.Value.GroupOn)
			return a.ok(answerSuccess(answer, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nAnswer: %s\n%s\n", summary, answer.Value.ID, answer.Value.AppURL)
			}))
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().StringVar(&askedFor, "asked-for", "", "Day of the reminder the answer is for, YYYY-MM-DD")

	return cmd
}

func (a *app) newCheckinAnswersEditCmd() *cobra.Command {
	var format string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <answerId> <text|->",
		Short: "Change the text of an answer you wrote",
		Long: `Changes the text of an answer. Only its writer can; an answer of somebody else
is exit 2 (not_found), as if it were not there. The same text again succeeds
and changes nothing. The day an answer belongs to never changes.

` + answerFormatHelp + `

Files and mentions stand in an answer as action-text-attachment elements,
which Markdown cannot hold. To keep them, take bodySource from checkins
answers show --json, change it and send it back with --format html. A new text
that would drop them is refused unless --drop-attachments is given; the
command reads the answer first to know.`,
		Example: `  minka checkins answers edit 0199... "Counted the tents and the benches."`,
		Args:    requireArgs("answerId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := answerBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the answer", args[1])
			if err != nil {
				return err
			}
			body, notices, err := answerBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			current, err := s.client.CheckinAnswer(a.ctx, args[0])
			if err != nil {
				return answerRefusal(s, err)
			}
			// Minka answers somebody else's answer as if it were not there, and
			// that answer comes before any guard of this side.
			if richtext.HasAttachments(firstNonEmpty(current.Value.BodySource, current.Value.Body)) {
				me, err := a.meID(s)()
				if err != nil {
					return err
				}
				if current.Value.Author.ID != me {
					return &Error{Code: CodeNotFound, Message: "answer " + args[0] + " was written by " + current.Value.Author.Name + ", not by you", Hint: answerWriterHint}
				}
			}
			// An answer lies in no archive of its own; only its project closes it.
			dropped, err := a.guardOverFiles(s, current.Value.ProjectID, closedIf(false), firstNonEmpty(current.Value.BodySource, current.Value.Body), body, dropAttachments,
				"the answer has attachments or mentions, and the new text would drop them",
				"Change bodySource from checkins answers show --json and send it back with --format html, or give --drop-attachments.")
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)

			answer, err := s.client.UpdateCheckinAnswer(a.ctx, args[0], body)
			if err != nil {
				return answerRefusal(s, err)
			}

			summary := fmt.Sprintf("Changed answer %s to %s", answer.Value.ID, answer.Value.Question.Title)
			return a.ok(answerSuccess(answer, summary, notices, func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) }))
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace an answer that has attachments or mentions, which drops them")

	return cmd
}

// answerBody turns the text of an answer into what is sent: Markdown with every
// line break kept, as for a comment, or HTML as it is.
func answerBody(cmd *cobra.Command, text, format string) (string, []string, error) {
	switch format {
	case "html":
		return text, markupNotice(text), nil
	case "markdown":
		result := markdown.ToHTML(text, markdown.KeepLineBreaks())
		if result.HTML == "" && strings.TrimSpace(text) != "" {
			return "", nil, &Error{Code: CodeUsage, Message: "the answer has nothing Minka keeps"}
		}
		return result.HTML, result.Warnings, nil
	default:
		return "", nil, usageErrorf(cmd, "--format is markdown or html, not %q", format)
	}
}

// weekdayNames are the words a weekday may be given by, beside its number.
var weekdayNames = map[string]int{
	"mon": 1, "monday": 1, "tue": 2, "tuesday": 2, "wed": 3, "wednesday": 3, "thu": 4, "thursday": 4,
	"fri": 5, "friday": 5, "sat": 6, "saturday": 6, "sun": 7, "sunday": 7,
}

// weekdayNumbers turns the weekdays given into the numbers Minka reads. A
// number goes out as it is, so that Minka says what it makes of one outside
// the week; only a word that names no weekday is refused here, because it
// cannot be sent as a number at all.
func weekdayNumbers(cmd *cobra.Command, values []string) ([]int, error) {
	var days []int
	for _, value := range values {
		value = strings.TrimSpace(value)
		if number, err := strconv.Atoi(value); err == nil {
			days = append(days, number)
			continue
		}
		number, known := weekdayNames[strings.ToLower(value)]
		if !known {
			return nil, usageErrorf(cmd, "--weekday is a number from 1 (Monday) to 7 or a name such as mon, not %q", value)
		}
		days = append(days, number)
	}
	return days, nil
}

// peopleByIDOrMe is the people a new question asks. Only the identifier and
// "me" are read here: the ID of a tool names no project, and a name can only
// be looked up among the members of one.
func (a *app) peopleByIDOrMe(cmd *cobra.Command, s *session, values []string) ([]string, error) {
	var people []string
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "me") {
			id, err := a.meID(s)()
			if err != nil {
				return nil, err
			}
			people = append(people, id)
			continue
		}
		if err := requireID(cmd, "--ask", &value); err != nil {
			return nil, &Error{
				Code:    CodeUsage,
				Message: fmt.Sprintf("--ask takes the ID of a person, or me; %q is neither", value),
				Hint:    "minka people list <projectId> gives the ID of every member of the project.",
			}
		}
		people = append(people, value)
	}
	return people, nil
}

// askedNotices say who was named to be asked and is not among the people the
// question shows as asked. Minka shows only the members who may see a
// question and answers the write with the same success, so this is the only
// word about it.
func askedNotices(named []string, question api.CheckinQuestion) []string {
	var missing []string
	for _, id := range named {
		shown := slices.ContainsFunc(question.Asked, func(person api.PersonReference) bool { return person.ID == id })
		if !shown && !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s named but not shown among the people asked: Minka lists as asked only the members who may see the question, so a client of the project it is not shown to is asked and named nowhere", strings.Join(missing, ", "))}
}

// questionRefusal classifies a failed request about a question or its answers.
// namedNobody says that an edit named no people, which makes an empty set of
// people a matter of whom Minka shows rather than of what was sent.
func questionRefusal(s *session, err error, namedNobody bool) *Error {
	e := s.fail(err)
	switch {
	case e.Code == CodeNotFound:
		e.Hint = questionNotFoundHint
	case e.APICode == apiCodePersonNotAProjectMember:
		e.Hint = askedPeopleHint
	case e.APICode == apiCodeQuestionNobodyAsked && namedNobody:
		e.Hint = "The edit names nobody, so Minka writes back the people it shows as asked, and it shows nobody: they have left the project or may not see the question. Name who is to be asked with --ask."
	case e.APICode == apiCodeQuestionNobodyAsked:
		e.Hint = "A question asks at least one member of the project: name one with --ask."
	case e.APICode == apiCodeQuestionWeekdaysMissing:
		e.Hint = "Give --weekday: a number from 1 (Monday) to 7 or a name such as mon."
	case e.APICode == apiCodeQuestionTimeInvalid:
		e.Hint = "--time is HH:MM on the 24-hour clock, such as 09:00."
	case e.APICode == apiCodeQuestionScheduleInvalid:
		e.Hint = "--rhythm is daily, weekly, everyOtherWeek or monthly, --weekday is 1 (Monday) to 7, and every rhythm but daily takes exactly one weekday. On an edit the weekdays not given are the question's own."
	case e.APICode == apiCodeAnswerDayNotADate:
		e.Hint = "--asked-for is a day written as YYYY-MM-DD."
	case e.APICode == apiCodeAnswerDayNotAsked:
		e.Hint = "Minka takes only a day the question was put to you for: askedFor of minka checkins reminders. Without --asked-for the answer belongs to the day of the latest asking by the schedule."
	case e.APICode == apiCodeAnswerMissing:
		e.Hint = "An answer needs text or a file."
	}
	return e
}

// answerRefusal classifies a failed request about one answer.
func answerRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch {
	case e.Code == CodeNotFound:
		e.Hint = answerWriterHint
	case e.APICode == apiCodeAnswerMissing:
		e.Hint = "An answer needs text or a file."
	}
	return e
}

func questionSummary(question api.CheckinQuestion) string {
	summary := "Question " + question.Title + ", " + describeSchedule(question.Schedule)
	if question.Paused {
		summary += ", paused"
	}
	return summary
}

func questionSuccess(question api.Result[api.CheckinQuestion], summary string, notices []string, full bool) success {
	q := question.Value
	crumbs := []Breadcrumb{
		{Action: "answers", Cmd: "minka checkins answers list " + q.ID, Description: "Read the answers to the question"},
		{Action: "answer", Cmd: "minka checkins answers create " + q.ID + " -", Description: "Answer the question"},
	}
	if q.Paused {
		crumbs = append(crumbs, Breadcrumb{Action: "resume", Cmd: "minka checkins resume " + q.ID, Description: "Ask the question again"})
	} else {
		crumbs = append(crumbs, Breadcrumb{Action: "pause", Cmd: "minka checkins pause " + q.ID, Description: "Stop asking the question for now"})
	}
	return success{
		data:        question.Raw,
		summary:     summary,
		notices:     notices,
		breadcrumbs: crumbs,
		text: func(w io.Writer) {
			if full {
				renderQuestion(w, q)
				return
			}
			fmt.Fprintf(w, "%s.\nQuestion: %s\n%s\n", summary, q.ID, q.AppURL)
		},
	}
}

func answerSuccess(answer api.Result[api.CheckinAnswer], summary string, notices []string, text func(io.Writer)) success {
	return success{
		data:    answer.Raw,
		summary: summary,
		notices: notices,
		breadcrumbs: []Breadcrumb{
			{Action: "show", Cmd: "minka checkins answers show " + answer.Value.ID, Description: "Read the answer with its comments"},
			{Action: "answers", Cmd: "minka checkins answers list " + answer.Value.QuestionID, Description: "Read the answers to the question"},
		},
		text: text,
	}
}

// weekdayAbbreviations name the ISO weekdays, Monday first.
var weekdayAbbreviations = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// describeSchedule says a schedule in words. A rhythm or a weekday this CLI has
// no words for is named as Minka gives it.
func describeSchedule(schedule api.CheckinSchedule) string {
	days := make([]string, len(schedule.Weekdays))
	for i, day := range schedule.Weekdays {
		days[i] = strconv.Itoa(day)
		if day >= 1 && day <= 7 {
			days[i] = weekdayAbbreviations[day-1]
		}
	}
	on := strings.Join(days, ", ")
	switch schedule.Rhythm {
	case "daily":
		return fmt.Sprintf("daily on %s at %s", on, schedule.TimeOfDay)
	case "weekly":
		return fmt.Sprintf("weekly on %s at %s", on, schedule.TimeOfDay)
	case "everyOtherWeek":
		if first := firstFortnightlyDay(schedule); first != "" {
			return fmt.Sprintf("every other week on %s at %s, first on %s", on, schedule.TimeOfDay, first)
		}
		return fmt.Sprintf("every other week on %s at %s, counted from %s", on, schedule.TimeOfDay, schedule.StartsOn)
	case "monthly":
		return fmt.Sprintf("monthly on the first %s at %s", on, schedule.TimeOfDay)
	}
	return fmt.Sprintf("%s on %s at %s", schedule.Rhythm, on, schedule.TimeOfDay)
}

// firstFortnightlyDay is the day the weeks of a question asked every other
// week are counted from: the first of its weekday on or after its start. It
// is empty where the schedule names no day the CLI can read.
func firstFortnightlyDay(schedule api.CheckinSchedule) string {
	start, err := time.Parse("2006-01-02", schedule.StartsOn)
	if err != nil || len(schedule.Weekdays) == 0 || schedule.Weekdays[0] < 1 || schedule.Weekdays[0] > 7 {
		return ""
	}
	weekday := (int(start.Weekday())+6)%7 + 1
	return start.AddDate(0, 0, (schedule.Weekdays[0]-weekday+7)%7).Format("2006-01-02")
}

// fortnightNotice says which weeks a question asked every other week asks in.
// The start of a question stays where it was, and its weekday decides which of
// the two sets of weeks follows from it.
func fortnightNotice(schedule api.CheckinSchedule) string {
	first := firstFortnightlyDay(schedule)
	if first == "" {
		return "every other week is counted from the first of its weekday on or after " + schedule.StartsOn + ", the day the question started, and no edit moves that day"
	}
	day, _ := time.Parse("2006-01-02", first)
	return fmt.Sprintf("every other week is counted from %s, the first %s on or after %s, the day the question started: the question asks in the weeks of %s, %s, %s and so on, and another weekday can move it into the other weeks",
		first, weekdayAbbreviations[schedule.Weekdays[0]-1], schedule.StartsOn, first, day.AddDate(0, 0, 14).Format("2006-01-02"), day.AddDate(0, 0, 28).Format("2006-01-02"))
}

func renderQuestion(w io.Writer, question api.CheckinQuestion) {
	fmt.Fprintf(w, "# %s\n", question.Title)
	fmt.Fprintf(w, "Question: %s\nTool: %s (%s)\nProject: %s (%s)\n", question.ID, question.Tool.Title, question.ToolID, question.Project.Name, question.ProjectID)
	fmt.Fprintf(w, "Asked: %s\nStarted: %s\n", describeSchedule(question.Schedule), question.Schedule.StartsOn)
	if question.Paused {
		fmt.Fprintln(w, "Paused: yes")
	} else {
		fmt.Fprintln(w, "Paused: no")
	}
	if len(question.Asked) > 0 {
		fmt.Fprintf(w, "Asks: %s\n", assigneeNames(question.Asked))
	}
	fmt.Fprintf(w, "URL: %s\n", question.AppURL)
}

func renderAnswer(w io.Writer, answer api.CheckinAnswer) {
	fmt.Fprintf(w, "# %s\n", answer.Question.Title)
	fmt.Fprintf(w, "Answer: %s\nQuestion: %s\nFor: %s\n", answer.ID, answer.QuestionID, answer.GroupOn)
	edited := ""
	if answer.EditedAt != nil {
		edited = ", edited " + formatTime(*answer.EditedAt)
	}
	fmt.Fprintf(w, "By: %s, %s%s\nURL: %s\n", answer.Author.Name, formatTime(answer.CreatedAt), edited, answer.AppURL)
	if text := richtext.ToMarkdown(firstNonEmpty(answer.BodySource, answer.Body)); text != "" {
		fmt.Fprintf(w, "\n%s\n", text)
	}
	renderEntryFiles(w, answer.Attachments)
	renderEntryComments(w, answer.Comments)
}

func renderAnswerLines(w io.Writer, answers []api.CheckinAnswerLine) {
	if len(answers) == 0 {
		fmt.Fprintln(w, "No answers.")
		return
	}
	day := ""
	for _, answer := range answers {
		if answer.GroupOn != day {
			if day != "" {
				fmt.Fprintln(w)
			}
			day = answer.GroupOn
			fmt.Fprintf(w, "## %s\n\n", day)
		}
		text := strings.Join(strings.Fields(richtext.ToMarkdown(answer.Body)), " ")
		fmt.Fprintf(w, "- %s  %s: %s\n", answer.ID, answer.Author.Name, text)
	}
}
