package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// What Minka answers a reminder it will not set or take away.
const (
	apiCodeReminderTimeMissing    = "reminderTimeMissing"
	apiCodeReminderPresetUnknown  = "reminderPresetUnknown"
	apiCodeReminderTimeNotAMoment = "reminderTimeNotAMoment"
	apiCodeReminderTimeGone       = "reminderTimeGone"
	apiCodeReminderOutcomeUnknown = "reminderOutcomeUnknown"
)

// reminderKinds are the words the address of a reminder takes for what its ID
// names, spelt the way Minka compares them.
var reminderKinds = []string{
	"card", "post", "todo", "todoList", "document", "upload", "question", "answer", "event", "forward",
	"comment", "chatMessage",
}

const reminderKindsHelp = `<kind> says what <item> names:

  card, post, todo, todoList, document, upload, question, answer,
  event, forward   the item, by its ID; a card also by its short identifier
  comment          a comment, by its own ID, on whichever of these it stands
  chatMessage      a line of a project chat, by its own ID

The kind is taken in any case. A line of a private conversation takes no
reminder.`

const reminderPresetHelp = `--preset is one of the times of the web menu: in30Minutes, in1Hour, in3Hours,
tomorrow (9:00) and nextMonday (9:00; on a Monday the Monday a week on).
tomorrow and nextMonday are counted in the time zone of your Minka
preferences, not in that of this machine.

--at is a moment in RFC 3339: a capital T, the seconds, a fraction if you
like, and Z or an offset of ±HH:MM, such as 2026-10-06T09:00:00+02:00 or
2026-10-06T07:00:00Z. That is the one form Minka reads; anything else is
refused before it is sent. The moment has to lie ahead of Minka's clock.`

const reminderNotFoundHint = "Nothing you can see goes by this ID as this kind. The kind may not be the one of the ID: a comment is kind comment and a chat line chatMessage, by their own IDs. Or it does not exist, is in the trash or in a tool or project in the trash, belongs to a project you are not a member of, is hidden from you as a client of the project, or is a line of a private conversation, which takes no reminder."

const noReminderHint = "You have no reminder of this ID. Your reminders: minka reminders list. A reminder of something you no longer see is not listed there and is still taken away by its kind and ID."

func (a *app) newRemindersCmd() *cobra.Command {
	return group("reminders", "List, set and take away your own reminders of cards, posts, comments and more",
		a.newRemindersListCmd(),
		a.newRemindersSetCmd(),
		a.newRemindersDeleteCmd(),
	)
}

func (a *app) newRemindersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your reminders, the due ones first",
		Long: `Lists your own reminders ("Remind me" on the web) across all projects: the due
ones first, the most recently due first, then those ahead, the soonest first.
A reminder is private: nobody else sees it, and an agent has its own.

isDue is true once the moment has come, until the reminder is taken away or
set anew. A reminder of something you can no longer see is left out, and it
comes back once you see it again.

These are not the reminders of check-ins: those are listed with
minka checkins reminders.

Minka reads at most 500 due reminders and 500 ahead, and leaves out the ones
out of sight only after that, so a side can be cut and still list fewer than
500. Where a side lists 500, a notice says there may be more.`,
		Args: requireArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			reminders, err := s.client.MyReminders(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			due := 0
			for _, reminder := range reminders.Value {
				if reminder.IsDue {
					due++
				}
			}
			var notices []string
			if due >= api.RemindersMaxOnASide {
				notices = append(notices, fmt.Sprintf("Minka reads at most %d due reminders, so there may be more: the ones left out are those due earliest", api.RemindersMaxOnASide))
			}
			if len(reminders.Value)-due >= api.RemindersMaxOnASide {
				notices = append(notices, fmt.Sprintf("Minka reads at most %d reminders ahead, so there may be more: the ones left out are those set for the latest moments", api.RemindersMaxOnASide))
			}
			return a.ok(success{
				data:    reminders.Raw,
				summary: fmt.Sprintf("%s, %d due", plural(len(reminders.Value), "reminder", "reminders"), due),
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "set", Cmd: "minka reminders set <kind> <item> --preset tomorrow", Description: "Set a reminder, or move one"},
					{Action: "done", Cmd: "minka reminders delete <kind> <item> --outcome done", Description: "Tick a reminder off"},
					{Action: "dismiss", Cmd: "minka reminders delete <kind> <item>", Description: "Wave a reminder away"},
				},
				text: func(w io.Writer) { renderReminders(w, reminders.Value) },
			})
		},
	}
}

func (a *app) newRemindersSetCmd() *cobra.Command {
	var preset, at string
	cmd := &cobra.Command{
		Use:   "set <kind> <item>",
		Short: "Set a reminder of an item, a comment or a chat line, or move it",
		Long: `Sets a reminder of an item, a comment or a line of a project chat for you
alone. A reminder that is set already, due or not, is moved to the new
moment; a due one is no longer due then, and its line in the bell counts as
read, as on the web.

` + reminderKindsHelp + `

Give --preset or --at, one of the two.

` + reminderPresetHelp + `

What you cannot see is not found, exit 2, and so is a kind other than the one
of the ID. A reminder can be set in an archived project, and on a card or a
post in the archive.`,
		Example: `  minka reminders set card MNK-1A --preset tomorrow
  minka reminders set comment 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6 --at 2026-10-06T09:00:00+02:00`,
		Args: requireArgs("kind", "item"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, item, err := reminderSubject(cmd, args)
			if err != nil {
				return err
			}
			if (preset == "") == (at == "") {
				return usageErrorf(cmd, "give --preset or --at, one of the two")
			}
			if at != "" && !isRFC3339Moment(at) {
				return &Error{Code: CodeUsage, Message: fmt.Sprintf("--at %q is no moment in RFC 3339", at), Hint: rfc3339Hint}
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			if kind == "card" {
				if err := s.resolveCard(a.ctx, &item); err != nil {
					return err
				}
			}
			reminder, err := s.client.SetReminder(a.ctx, kind, item, api.SetReminderInput{Preset: preset, RemindAt: at})
			if err != nil {
				return reminderRefusal(s, err, reminderNotFoundHint)
			}

			r := reminder.Value
			return a.ok(success{
				data:    reminder.Raw,
				summary: fmt.Sprintf("Reminds you of %s at %s", r.Title, formatTime(r.RemindAt)),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka reminders list", Description: "List your reminders"},
					{Action: "done", Cmd: "minka reminders delete " + r.Kind + " " + r.ID + " --outcome done", Description: "Tick the reminder off"},
					{Action: "dismiss", Cmd: "minka reminders delete " + r.Kind + " " + r.ID, Description: "Wave the reminder away"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Reminds you of %s (%s %s) at %s.\n%s\n", r.Title, r.Kind, r.ID, formatTime(r.RemindAt), r.AppURL)
				},
			})
		},
	}
	cmd.Flags().StringVar(&preset, "preset", "", "One of in30Minutes, in1Hour, in3Hours, tomorrow, nextMonday")
	cmd.Flags().StringVar(&at, "at", "", "Moment in RFC 3339 with Z or ±HH:MM, such as 2026-10-06T09:00:00+02:00")
	return cmd
}

func (a *app) newRemindersDeleteCmd() *cobra.Command {
	var outcome string
	cmd := &cobra.Command{
		Use:   "delete <kind> <item>",
		Short: "Take a reminder away, ticked off or waved away",
		Long: `Takes your reminder of an item, a comment or a chat line away, due or not,
and whether or not you still see what it is of: a reminder of something gone
out of sight is taken away by its kind and ID as well.

` + reminderKindsHelp + `

The kind has to be one of these, but Minka does not compare it with the
reminder: the ID decides.

--outcome done is the web's tick, dismissed its X, which is what Minka takes
when none is given. Both take the reminder away alike; Minka only records which
it was. An outcome it does not know is refused before it looks for the
reminder, exit 9 (reminderOutcomeUnknown). No reminder of yours for the ID is
exit 2.`,
		Args: requireArgs("kind", "item"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, item, err := reminderSubject(cmd, args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			if kind == "card" {
				if err := s.resolveCard(a.ctx, &item); err != nil {
					return err
				}
			}
			if err := s.client.RemoveReminder(a.ctx, kind, item, outcome); err != nil {
				return reminderRefusal(s, err, noReminderHint)
			}

			recorded := outcome
			if recorded == "" {
				recorded = "dismissed"
			}
			return a.ok(success{
				data:    map[string]any{"kind": kind, "id": item, "outcome": recorded, "deleted": true},
				summary: fmt.Sprintf("Took the reminder of %s %s away (%s)", kind, item, recorded),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka reminders list", Description: "List your reminders"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Took the reminder of %s %s away (%s).\n", kind, item, recorded)
				},
			})
		},
	}
	cmd.Flags().StringVar(&outcome, "outcome", "", "done or dismissed (default: dismissed, as Minka takes it)")
	return cmd
}

// reminderSubject reads the kind and the item of a reminder before anything is
// asked: a kind Minka knows, in its own spelling, and an ID in lowercase. A
// card may be named by its short identifier, which is looked up afterwards.
func reminderSubject(cmd *cobra.Command, args []string) (kind, item string, err error) {
	for _, known := range reminderKinds {
		if strings.EqualFold(args[0], known) {
			kind = known
		}
	}
	if kind == "" {
		return "", "", usageErrorf(cmd, "%q is no kind of reminder; the kinds are %s", args[0], strings.Join(reminderKinds, ", "))
	}
	item = args[1]
	if kind == "card" {
		if id, ok := asID(item); ok {
			return kind, id, nil
		}
		if shortIdentifierPattern.MatchString(item) {
			return kind, item, nil
		}
		return "", "", usageErrorf(cmd, "<item> %q is not an ID; a card is named by a UUID such as 0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6 or by its short identifier such as MNK-1A", item)
	}
	err = requireID(cmd, "<item>", &item)
	return kind, item, err
}

// rfc3339Moment is the one form Minka reads the moment of a reminder in: a
// capital T, seconds, a fraction of any length and Z or an offset of ±HH:MM up
// to 14 hours.
var rfc3339Moment = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d+))?(Z|[+-](?:(?:0\d|1[0-3]):[0-5]\d|14:00))$`)

const rfc3339Hint = "--at is a moment in RFC 3339 on a day that exists: a capital T, the seconds, and Z or an offset of ±HH:MM, such as 2026-10-06T09:00:00+02:00 or 2026-10-06T07:00:00Z."

// isRFC3339Moment says whether Minka reads the text as a moment. It holds the
// same form and the same days, so it refuses nothing Minka would take.
func isRFC3339Moment(text string) bool {
	parts := rfc3339Moment.FindStringSubmatch(text)
	if parts == nil {
		return false
	}
	_, err := time.Parse(time.RFC3339, parts[1]+parts[3])
	return err == nil
}

// reminderRefusal classifies a failed request about a reminder. A 404 means
// another thing when setting than when taking away, so the caller names it.
func reminderRefusal(s *session, err error, notFoundHint string) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeReminderTimeMissing:
		e.Hint = "Give --preset or --at."
	case e.APICode == apiCodeReminderPresetUnknown:
		e.Hint = "--preset is one of in30Minutes, in1Hour, in3Hours, tomorrow and nextMonday."
	case e.APICode == apiCodeReminderTimeNotAMoment:
		e.Hint = rfc3339Hint
	case e.APICode == apiCodeReminderTimeGone:
		e.Hint = "--at has to lie ahead of Minka's clock. For a moment soon, --preset in30Minutes is counted from that clock."
	case e.APICode == apiCodeReminderOutcomeUnknown:
		e.Hint = "--outcome is done (ticked off) or dismissed (waved away), or left out for dismissed."
	case e.Code == CodeNotFound:
		e.Hint = notFoundHint
	}
	return e
}

func renderReminders(w io.Writer, reminders []api.Reminder) {
	if len(reminders) == 0 {
		fmt.Fprintln(w, "No reminders.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "WHEN\tDUE\tKIND\tID\tTITLE\tPROJECT")
	for _, reminder := range reminders {
		due := ""
		if reminder.IsDue {
			due = "due"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", formatTime(reminder.RemindAt), due, reminder.Kind, reminder.ID, reminder.Title, reminder.Project.Name)
	}
	_ = table.Flush()
}
