package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// What Minka answers a request about a meeting it will not take.
const (
	apiCodeMeetingDayMissing              = "meetingDayMissing"
	apiCodeMeetingClosed                  = "meetingClosed"
	apiCodeDecisionAlreadyOnMeetingAgenda = "decisionAlreadyOnMeetingAgenda"
	apiCodeDecisionNotForAMeetingAgenda   = "decisionNotForAMeetingAgenda"
	apiCodeDecisionUnknown                = "decisionUnknown"
	apiCodeAgendaPlaceUnknown             = "agendaPlaceUnknown"
	apiCodeMeetingOutcomeUnknown          = "meetingOutcomeUnknown"
	apiCodeSentenceWithoutChanges         = "sentenceWithoutChanges"
	apiCodeMeetingOutcomeHasNoStatus      = "meetingOutcomeHasNoStatus"
	apiCodeMeetingAudienceUnknown         = "meetingAudienceUnknown"
	apiCodeDecisionVoteCountMalformed     = "decisionVoteCountMalformed"
	apiCodeDecisionTitleTooLong           = "decisionTitleTooLong"
	apiCodeDecisionTitleHasControlChars   = "decisionTitleHasControlCharacters"
	apiCodeDecisionVotesTooLong           = "decisionVotesTooLong"
	apiCodeDecisionVotesHasControlChars   = "decisionVotesHasControlCharacters"
)

// maxVoteCount is the highest count of votes Minka keeps.
const maxVoteCount = 99999

// meetingOutcomeWords are the outcomes as the command line spells them, each
// with Minka's word for it.
var meetingOutcomeWords = map[string]string{
	"accepted":              api.OutcomeAccepted,
	"accepted-with-changes": api.OutcomeAcceptedWithChanges,
	"rejected":              api.OutcomeRejected,
	"deferred":              api.OutcomeDeferred,
}

const meetingOutcomeList = "accepted, accepted-with-changes, rejected or deferred"

const meetingDayHelp = `--day names the day of a series as YYYY-MM-DD: a series meets on each of its
days, and each day has an agenda of its own. The day is the one the schedule
gives, occurrences[].day of "minka events list <toolId> --from ... --to ...":
never the first day of the series, and for an occurrence that was moved not
the day it takes place on. A series without --day is exit 9 (validation) with
meetingDayMissing, a day the schedule does not give exit 2 (not_found). An
event that does not repeat needs no day, and Minka reads none for it.`

const agendaItemHelp = `An item is given by its ID, by its number on the agenda as "events meeting
show" prints it, or by the decision it carries: the ID of the decision or its
short identifier such as ENT-14. A number always means a place on the agenda
as you are shown it.`

// meetingVotesHelp is the one place that says what an outcome does to the
// votes.
const meetingVotesHelp = `The votes are a note of one line (--votes, at most 100 characters) and three
counts, each a whole number from 0 to 99999 (--votes-for, --votes-against,
--abstentions). Only what is given is sent. A vote flag left out keeps what
stands, on the agenda item and on the decision; --no-votes takes the note
off.`

const meetingDayMissingHint = "The event repeats, and a series meets on each of its days: give --day YYYY-MM-DD. The day is occurrences[].day of minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD, the day the schedule gives: not the first day of the series, and for a moved occurrence not the day it takes place on."

const meetingNotFoundHint = "There is no such event, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you; or the event repeats and --day is no day its schedule gives. The days of a series: minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD"

const meetingWriteNotFoundHint = "The event is not there for you, --day is no day the schedule of the series gives or a cancelled one, which takes no change, or the item is none of this agenda that you may change: it is gone, hidden from you, or its decision lies in the archive. The agenda as it stands: minka events meeting show <eventId>"

const meetingAddNotFoundHint = "There is no such event, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you; or the event repeats and --day is no day its schedule gives, or a cancelled one: a cancelled day takes no agenda. The days of a series: minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD"

const meetingCloseNotFoundHint = "There is nothing to close here, or you may not close it: the agenda is empty, or it holds an item you may not change, one hidden from you included. Minka gives the same answer for an event that is not there for you and for a day the series does not meet on or that is cancelled. The agenda as it stands: minka events meeting show <eventId>"

const meetingsUnknownHint = "Its Minka is older than this minka expects and knows no meetings yet: the agenda of an event is there from Minka v0.122.0 on."

// checkMeetingDay holds --day to a day that exists, before any request: a
// series answers a day it cannot read like one that was left out.
func checkMeetingDay(cmd *cobra.Command, day string) error {
	if !cmd.Flags().Changed("day") {
		return nil
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return usageErrorf(cmd, "--day %q is no day; a day is written as YYYY-MM-DD", day)
	}
	return nil
}

func registerMeetingDay(cmd *cobra.Command, day *string) {
	cmd.Flags().StringVar(day, "day", "", "Day of a series the meeting is on, as YYYY-MM-DD: occurrences[].day of events list")
}

// daySuffix is the flag that names the same meeting again in a command line.
func daySuffix(day string) string {
	if day == "" {
		return ""
	}
	return " --day " + day
}

func (a *app) newEventsMeetingCmd() *cobra.Command {
	return group("meeting", "Read the meeting of an event, keep its agenda, record outcomes and close it",
		a.newMeetingShowCmd(),
		group("agenda", "Put decisions on the agenda of a meeting, move them and take them off",
			a.newAgendaAddCmd(),
			a.newAgendaMoveCmd(),
			a.newAgendaRemoveCmd(),
		),
		a.newMeetingOutcomeCmd(),
		a.newMeetingCloseCmd(),
	)
}

func (a *app) newMeetingShowCmd() *cobra.Command {
	var day string
	cmd := &cobra.Command{
		Use:   "show <eventId>",
		Short: "Show the meeting of an event: its agenda, the outcomes and whether it is closed",
		Long: `Shows the meeting of an event. Every event you can see has one: without an
agenda it is open and empty. data is the meeting as Minka answers it:
{eventId, projectId, toolId, title, day, cancelled, closed, closedAt,
closedBy, mayClose, items, appUrl}.

items are the decisions on the agenda that you are shown, in their order,
each with id (the item's own, not the decision's), number (from 1, without a
gap), current (the first item without an outcome), decision ({id, title,
status, archived, appUrl, shortId}), outcome (null until recorded, else
{outcome, status, withChanges, votes, votesFor, votesAgainst, abstentions,
recordedBy, recordedAt}) and what you may do with it: mayMove, mayTakeOff and
mayRecordOutcome. mayClose says whether you may close the meeting; it is
false for an empty agenda. A client of the project sees only the decisions
shown to them. The cancelled day of a series reads with cancelled true and
takes no change.

` + meetingDayHelp,
		Example: `  minka events meeting show 0199... --json
  minka events meeting show 0199... --day 2026-10-26`,
		Args: requireArgs("eventId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			meeting, err := s.client.Meeting(a.ctx, args[0], day)
			if err != nil {
				return meetingRefusal(s, err, meetingNotFoundHint)
			}
			return a.ok(meetingSuccess(meeting, describeMeeting(meeting.Value), nil))
		},
	}
	registerMeetingDay(cmd, &day)
	return cmd
}

func (a *app) newAgendaAddCmd() *cobra.Command {
	var day string
	cmd := &cobra.Command{
		Use:   "add <eventId> <decision> [<decision>...]",
		Short: "Put decisions at the end of the agenda of a meeting",
		Long: `Puts one or more decisions at the end of the agenda, in the order given and
in one request: all of them or none. A decision is named by its ID or its
short identifier such as ENT-14, which is looked up first. It may be of any
Decisions tool of the event's project, and it has to be still to be taken: a
status of the role proposed or deferred, or one the team added. Anything else
is exit 9 (validation) with decisionNotForAMeetingAgenda; a decision that is
unknown, in the archive, in another project or hidden from you is
decisionUnknown.

A decision stands on one agenda once: putting it on again is exit 9 with
decisionAlreadyOnMeetingAgenda and puts none on. It may stand on the agendas
of several meetings. Putting a decision on changes nothing on the decision.
A closed meeting takes none: meetingClosed.

data is the meeting.

` + meetingDayHelp,
		Example: `  minka events meeting agenda add 0199... ENT-14 ENT-15
  minka events meeting agenda add 0199... ENT-14 --day 2026-10-26`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return requireArgs("eventId", "decision")(cmd, args)
			}
			return requireID(cmd, "eventId", &args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}
			decisions := slices.Clone(args[1:])
			for i := range decisions {
				if err := requireID(cmd, "decisionId", &decisions[i]); err != nil {
					return err
				}
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			for i := range decisions {
				if err := s.resolveDecision(a.ctx, &decisions[i]); err != nil {
					return err
				}
			}
			meeting, err := s.client.PutOnAgenda(a.ctx, args[0], day, decisions)
			if err != nil {
				e := meetingRefusal(s, err, meetingAddNotFoundHint)
				if e.APICode == apiCodeMeetingClosed {
					e.Hint = "The meeting is closed, and a closed meeting stays closed: its agenda takes no further decision. Put the decision on the agenda of another meeting."
				}
				return e
			}
			summary := fmt.Sprintf("Put %s on the agenda of %s", plural(len(decisions), "decision", "decisions"), meetingName(meeting.Value))
			return a.ok(meetingSuccess(meeting, summary, nil))
		},
	}
	registerMeetingDay(cmd, &day)
	return cmd
}

func (a *app) newAgendaMoveCmd() *cobra.Command {
	var day, after, before string
	var top, bottom bool
	cmd := &cobra.Command{
		Use:   "move <eventId> <item>",
		Short: "Move an item to another place on the agenda",
		Long: `Puts an item elsewhere on its agenda: --top puts it first, --bottom last,
--after below the item named, --before above it. Exactly one of the four is
required. Moving an item to where it stands already succeeds. The item named
with --after or --before is another item of the agenda that you are shown;
the item itself is exit 9 (validation) with agendaPlaceUnknown. A closed
meeting keeps its order: meetingClosed.

--bottom means behind the last item as read: the command reads the meeting
first, as it does to find the item. It is the end of the agenda as you are
shown it: where items are hidden from you, as from a client of the project,
the item goes behind the last one you see, and hidden ones may follow it.

data is the meeting.

` + agendaItemHelp + `

` + meetingDayHelp,
		Example: `  minka events meeting agenda move 0199... ENT-15 --top
  minka events meeting agenda move 0199... 3 --after ENT-14 --day 2026-10-26`,
		Args: requireArgs("eventId", "item"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, bottom, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the item goes: give exactly one of --top, --bottom, --after and --before")
			}
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			read, err := s.client.Meeting(a.ctx, args[0], day)
			if err != nil {
				return meetingRefusal(s, err, meetingNotFoundHint)
			}
			item, err := resolveAgendaItem(read.Value.Items, args[1])
			if err != nil {
				return err
			}
			var input api.MoveAgendaItemInput
			for _, neighbour := range []struct {
				value string
				id    *string
			}{{after, &input.PreviousItemID}, {before, &input.NextItemID}} {
				if neighbour.value == "" {
					continue
				}
				found, err := resolveAgendaItem(read.Value.Items, neighbour.value)
				if err != nil {
					return err
				}
				*neighbour.id = found.ID
			}
			// Minka has no word for the end: an item goes there behind the
			// last of the others, or to the top where there is no other.
			if bottom {
				for _, other := range read.Value.Items {
					if other.ID != item.ID {
						input.PreviousItemID = other.ID
					}
				}
			}

			meeting, err := s.client.MoveAgendaItem(a.ctx, args[0], item.ID, day, input)
			if err != nil {
				e := meetingRefusal(s, err, meetingWriteNotFoundHint)
				if e.APICode == apiCodeAgendaPlaceUnknown && bottom {
					e.Hint = "The item that was last on the agenda when it was read is gone from it since. The agenda as it stands: minka events meeting show " + args[0] + daySuffix(day)
				}
				return e
			}
			summary := fmt.Sprintf("Moved %s on the agenda of %s", agendaItemName(item), meetingName(meeting.Value))
			return a.ok(meetingSuccess(meeting, summary, nil))
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&top, "top", false, "Put the item first")
	flags.BoolVar(&bottom, "bottom", false, "Put the item last")
	flags.StringVar(&after, "after", "", "Put the item below this item")
	flags.StringVar(&before, "before", "", "Put the item above this item")
	registerMeetingDay(cmd, &day)
	return cmd
}

func (a *app) newAgendaRemoveCmd() *cobra.Command {
	var day string
	cmd := &cobra.Command{
		Use:   "remove <eventId> <item>",
		Short: "Take an item off the agenda of a meeting",
		Long: `Takes an item off the agenda, without asking. The decision stays as it is,
with the status an outcome recorded here gave it. An item that is not on the
agenda is exit 2 (not_found), a second removal too: Minka does not answer a
removal twice. A closed meeting keeps its items: exit 9 (validation) with
meetingClosed.

Minka answers a removal without a body, so data is {eventId, day, itemId,
decisionId} as the command knows them, day null for an event that does not
repeat.

` + agendaItemHelp + `

` + meetingDayHelp,
		Example: `  minka events meeting agenda remove 0199... ENT-14`,
		Args:    requireArgs("eventId", "item"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			read, err := s.client.Meeting(a.ctx, args[0], day)
			if err != nil {
				return meetingRefusal(s, err, meetingNotFoundHint)
			}
			item, err := resolveAgendaItem(read.Value.Items, args[1])
			if err != nil {
				return err
			}
			if err := s.client.TakeOffAgenda(a.ctx, args[0], item.ID, day); err != nil {
				return meetingRefusal(s, err, meetingWriteNotFoundHint)
			}
			summary := fmt.Sprintf("Took %s off the agenda of %s; the decision itself stays", agendaItemName(item), meetingName(read.Value))
			return a.ok(success{
				data:    map[string]any{"eventId": args[0], "day": read.Value.Day, "itemId": item.ID, "decisionId": item.Decision.ID},
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka events meeting show " + args[0] + daySuffix(day), Description: "Show the meeting with its agenda"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	registerMeetingDay(cmd, &day)
	return cmd
}

func (a *app) newMeetingOutcomeCmd() *cobra.Command {
	var day, sentence, note string
	var votesFor, votesAgainst, abstentions int
	var noVotes bool
	cmd := &cobra.Command{
		Use:   "outcome <eventId> <item> <outcome>",
		Short: "Record what the meeting made of an item, or correct it",
		Long: `Records what the meeting made of an item on its agenda. The outcome is ` + meetingOutcomeList + `,
in any case.

An outcome changes the decision: it puts it into the status of its tool that
has the role holds (accepted, accepted-with-changes), rejected or deferred.
The first three also make this meeting the day and the event the decision was
taken on. accepted-with-changes needs --sentence, the decision's new sentence,
one line of at most 200 characters; no other outcome takes one. Recording
tells nobody.

The outcome is the last one written: recording again corrects it, and nothing
guards against somebody else's outcome in between. A correction does not
bring an earlier sentence back. On a closed meeting an outcome can still be
corrected, and an item without one gets none: exit 9 (validation) with
meetingClosed. A tool without a status of the role needed is
meetingOutcomeHasNoStatus.

` + meetingVotesHelp + `

data is the meeting.

` + agendaItemHelp + `

` + meetingDayHelp,
		Example: `  minka events meeting outcome 0199... ENT-14 accepted --votes-for 7 --votes-against 2
  minka events meeting outcome 0199... 2 accepted-with-changes --sentence "We buy two tents"
  minka events meeting outcome 0199... ENT-15 deferred --day 2026-10-26`,
		Args: requireArgs("eventId", "item", "outcome"),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := cmd.Flags().Changed
			spelled := strings.ToLower(args[2])
			word, known := meetingOutcomeWords[spelled]
			if !known {
				return usageErrorf(cmd, "the outcome is %s, not %q", meetingOutcomeList, args[2])
			}
			withChanges := word == api.OutcomeAcceptedWithChanges
			switch {
			case changed("sentence") && !withChanges:
				return usageErrorf(cmd, "--sentence goes with accepted-with-changes alone")
			case withChanges && strings.TrimSpace(sentence) == "":
				return usageErrorf(cmd, "accepted-with-changes needs the changed sentence: give --sentence")
			case noVotes && changed("votes"):
				return usageErrorf(cmd, "--votes and --no-votes contradict each other")
			}
			input := api.MeetingInput{"outcome": word}
			if withChanges {
				input["sentence"] = sentence
			}
			if changed("votes") {
				input["votes"] = note
			}
			if noVotes {
				input["votes"] = nil
			}
			for _, count := range []struct {
				flag, field string
				value       int
			}{{"votes-for", "votesFor", votesFor}, {"votes-against", "votesAgainst", votesAgainst}, {"abstentions", "abstentions", abstentions}} {
				if !changed(count.flag) {
					continue
				}
				if count.value < 0 || count.value > maxVoteCount {
					return usageErrorf(cmd, "--%s is a whole number from 0 to %d, not %d", count.flag, maxVoteCount, count.value)
				}
				input[count.field] = count.value
			}
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			read, err := s.client.Meeting(a.ctx, args[0], day)
			if err != nil {
				return meetingRefusal(s, err, meetingNotFoundHint)
			}
			item, err := resolveAgendaItem(read.Value.Items, args[1])
			if err != nil {
				return err
			}
			meeting, err := s.client.RecordOutcome(a.ctx, args[0], item.ID, day, input)
			if err != nil {
				e := meetingRefusal(s, err, meetingWriteNotFoundHint)
				if e.APICode == apiCodeMeetingClosed {
					e.Hint = "The meeting is closed, and this item has no outcome: a closed meeting takes only the correction of an outcome it recorded. The decision itself can still be given a status: minka decisions status " + item.Decision.ID + " <status>"
				}
				return e
			}
			summary := fmt.Sprintf("Recorded %s as %s in %s", agendaItemName(item), spelled, meetingName(meeting.Value))
			if item.Outcome != nil {
				summary = fmt.Sprintf("Corrected the outcome of %s to %s in %s", agendaItemName(item), spelled, meetingName(meeting.Value))
			}
			return a.ok(meetingSuccess(meeting, summary, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n", summary)
				for _, now := range meeting.Value.Items {
					if now.ID == item.ID {
						renderAgendaItem(w, now)
					}
				}
			}))
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&sentence, "sentence", "", "The decision's changed sentence, with accepted-with-changes")
	flags.StringVar(&note, "votes", "", "Note on the votes, one line")
	flags.BoolVar(&noVotes, "no-votes", false, "Take the note on the votes off")
	flags.IntVar(&votesFor, "votes-for", 0, "Votes for, 0 to 99999")
	flags.IntVar(&votesAgainst, "votes-against", 0, "Votes against, 0 to 99999")
	flags.IntVar(&abstentions, "abstentions", 0, "Abstentions, 0 to 99999")
	registerMeetingDay(cmd, &day)
	return cmd
}

func (a *app) newMeetingCloseCmd() *cobra.Command {
	var day, audience string
	var notify []string
	cmd := &cobra.Command{
		Use:   "close <eventId>",
		Short: "Close a meeting for good and tell people about it",
		Long: `Closes a meeting. This cannot be undone: there is no reopening. A closed
meeting takes no decision on its agenda, no move and no removal; an item
without an outcome stays without one, and only an outcome already recorded
can still be corrected. Closing a closed meeting is exit 9 (validation) with
meetingClosed.

Closing tells people. --audience says whom and is required, so that nobody is
told by a flag left out: "everybody", the humans of the project, "chosen" with one
--notify per person, or "nobody". --notify belongs to "chosen" alone: Minka
refuses names under any other audience with notifyIdsWithoutChosenAudience
and "chosen" without a name with notifyIdsMissing. ` + personHelp + ` A name
or an e-mail address is looked up among the members of the event's project.
Somebody who is no member is personNotAProjectMember, a client where the
event is not shown to clients personCannotSeeIt; either leaves the meeting
open.

You may close a meeting whose every item you may change, and none with an
empty agenda: mayClose of "events meeting show" says so beforehand. Minka
answers both cases with exit 2 (not_found), as it answers an event that is
not there, and does not tell them apart.

data is the meeting.

` + meetingDayHelp,
		Example: `  minka events meeting close 0199... --audience everybody
  minka events meeting close 0199... --audience chosen --notify anke@example.com --day 2026-10-26
  minka events meeting close 0199... --audience nobody`,
		Args: requireArgs("eventId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("audience") {
				return usageErrorf(cmd, "say whom the closed meeting is told to: --audience everybody, chosen or nobody")
			}
			if err := checkDecisionAudience(cmd, audience); err != nil {
				return err
			}
			if err := checkMeetingDay(cmd, day); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			input := api.DecisionInput{}
			project := func() (string, error) {
				read, err := s.client.Meeting(a.ctx, args[0], day)
				if err != nil {
					return "", meetingRefusal(s, err, meetingNotFoundHint)
				}
				return read.Value.ProjectID, nil
			}
			if err := a.audienceInput(cmd, s, input, audience, notify, project); err != nil {
				return err
			}
			meeting, err := s.client.CloseMeeting(a.ctx, args[0], day, api.MeetingInput(input))
			if err != nil {
				e := meetingRefusal(s, err, meetingCloseNotFoundHint)
				switch e.APICode {
				case apiCodeMeetingClosed:
					e.Hint = "The meeting is closed already, and stays closed. Read it with: minka events meeting show " + args[0] + daySuffix(day)
				case apiCodePersonCannotSeeIt:
					e.Hint = "The meeting stays open, and the same request will be refused again: somebody named with --notify is a client of the project, or an agent working for one, and the event is not shown to clients. Name somebody else."
				}
				return e
			}
			summary := fmt.Sprintf("Closed %s, with the audience %s", meetingName(meeting.Value), audience)
			return a.ok(meetingSuccess(meeting, summary, nil))
		},
	}
	registerDecisionAudience(cmd, &audience, &notify)
	cmd.Flags().Lookup("audience").Usage = "Whom the closed meeting is told to: everybody, chosen or nobody (required)"
	registerMeetingDay(cmd, &day)
	return cmd
}

// resolveAgendaItem finds an item of an agenda by its ID, its number or the
// decision it carries. An ID on no item is passed on, so that the server
// gives its answer; the returned item then carries nothing else.
func resolveAgendaItem(items []api.AgendaItem, value string) (api.AgendaItem, error) {
	ids, shortIDs := make([]string, len(items)), make([]string, len(items))
	for i, item := range items {
		ids[i], shortIDs[i] = item.ID, deref(item.Decision.ShortID)
	}
	if id, isID := asID(strings.TrimSpace(value)); isID && !slices.Contains(ids, id) {
		if at := slices.IndexFunc(items, func(item api.AgendaItem) bool { return item.Decision.ID == id }); at >= 0 {
			return items[at], nil
		}
	}
	at, id, err := findLine(namedLines{noun: "item", owner: "the agenda", ids: ids, titles: shortIDs, described: describeAgenda(items)}, value)
	if err != nil || at < 0 {
		return api.AgendaItem{ID: id}, err
	}
	return items[at], nil
}

func describeAgenda(items []api.AgendaItem) string {
	if len(items) == 0 {
		return "The agenda is empty."
	}
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = fmt.Sprintf("%d. %s (%s)", item.Number, decisionTitle(item.Decision.ShortID, item.Decision.Title), item.ID)
	}
	return "Agenda: " + strings.Join(lines, "; ")
}

// agendaItemName names an item in a summary by its decision, or by its ID
// where the agenda as read did not show it.
func agendaItemName(item api.AgendaItem) string {
	if item.Decision.ID == "" {
		return "item " + item.ID
	}
	return decisionTitle(item.Decision.ShortID, item.Decision.Title)
}

// meetingName names a meeting by its event and, for a series, its day.
func meetingName(meeting api.Meeting) string {
	if meeting.Day != nil {
		return "the meeting " + meeting.Title + " on " + *meeting.Day
	}
	return "the meeting " + meeting.Title
}

func describeMeeting(meeting api.Meeting) string {
	recorded := 0
	for _, item := range meeting.Items {
		if item.Outcome != nil {
			recorded++
		}
	}
	state := "open"
	switch {
	case meeting.Cancelled:
		state = "cancelled"
	case meeting.Closed:
		state = "closed"
	}
	return fmt.Sprintf("%s, %s: %s on the agenda, %d with an outcome", capitalized(meetingName(meeting)), state, plural(len(meeting.Items), "item", "items"), recorded)
}

// meetingSuccess hands a meeting out with the calls its reader may make
// next, taken from what the meeting says they may do.
func meetingSuccess(meeting api.Result[api.Meeting], summary string, text func(io.Writer)) success {
	m := meeting.Value
	named := m.EventID + daySuffix(deref(m.Day))
	crumbs := []Breadcrumb{{Action: "show", Cmd: "minka events meeting show " + named, Description: "Show the meeting with its agenda"}}
	if !m.Closed && !m.Cancelled {
		crumbs = append(crumbs, Breadcrumb{Action: "add", Cmd: "minka events meeting agenda add " + m.EventID + " <decision>" + daySuffix(deref(m.Day)), Description: "Put a decision on the agenda"})
	}
	if slices.ContainsFunc(m.Items, func(item api.AgendaItem) bool { return item.MayRecordOutcome }) {
		crumbs = append(crumbs, Breadcrumb{Action: "outcome", Cmd: "minka events meeting outcome " + m.EventID + " <item> <outcome>" + daySuffix(deref(m.Day)), Description: "Record what the meeting made of an item"})
	}
	if m.MayClose {
		crumbs = append(crumbs, Breadcrumb{Action: "close", Cmd: "minka events meeting close " + named + " --audience everybody|chosen|nobody", Description: "Close the meeting for good and tell people"})
	}
	crumbs = append(crumbs, Breadcrumb{Action: "event", Cmd: "minka events show " + m.EventID, Description: "Show the event"})
	if text == nil {
		text = func(w io.Writer) { renderMeeting(w, m) }
	}
	return success{data: meeting.Raw, summary: summary, breadcrumbs: crumbs, text: text}
}

func renderMeeting(w io.Writer, meeting api.Meeting) {
	fmt.Fprintf(w, "# Meeting: %s\nEvent: %s\n", meeting.Title, meeting.EventID)
	if meeting.Day != nil {
		fmt.Fprintf(w, "Day: %s\n", *meeting.Day)
	}
	switch {
	case meeting.Cancelled:
		fmt.Fprintln(w, "State: cancelled, this day of the series does not take place")
	case meeting.Closed:
		closed := "State: closed"
		if meeting.ClosedAt != nil {
			closed += " " + formatTime(*meeting.ClosedAt)
		}
		if meeting.ClosedBy != nil {
			closed += " by " + meeting.ClosedBy.Name
		}
		fmt.Fprintln(w, closed)
	default:
		fmt.Fprintln(w, "State: open")
	}
	if meeting.MayClose {
		fmt.Fprintln(w, "You may close it.")
	}
	fmt.Fprintf(w, "URL: %s\n", meeting.AppURL)

	if len(meeting.Items) == 0 {
		fmt.Fprintln(w, "\nNo items on the agenda.")
		return
	}
	fmt.Fprintln(w, "\n## Agenda")
	for _, item := range meeting.Items {
		fmt.Fprintln(w)
		renderAgendaItem(w, item)
	}
}

// renderAgendaItem shows an item with its decision, what the meeting made of
// it and what the reader may do with it; the current item carries an arrow.
func renderAgendaItem(w io.Writer, item api.AgendaItem) {
	mark := " "
	if item.Current {
		mark = ">"
	}
	status := item.Decision.Status.Name
	if item.Decision.Archived {
		status += ", archived"
	}
	fmt.Fprintf(w, "%s %d. %s  [%s]\n", mark, item.Number, decisionTitle(item.Decision.ShortID, item.Decision.Title), status)
	fmt.Fprintf(w, "     %s\n     Item: %s\n", item.Decision.AppURL, item.ID)
	if outcome := item.Outcome; outcome != nil {
		line := "     Outcome: " + describeOutcome(*outcome)
		if votes := describeVotes(outcome.VotesFor, outcome.VotesAgainst, outcome.Abstentions, outcome.Votes); votes != "" {
			line += "; votes: " + votes
		}
		line += "; recorded " + formatTime(outcome.RecordedAt)
		if outcome.RecordedBy != nil {
			line += " by " + outcome.RecordedBy.Name
		}
		fmt.Fprintln(w, line)
	}
	var may []string
	for _, allowed := range []struct {
		flag bool
		what string
	}{{item.MayRecordOutcome, "record its outcome"}, {item.MayMove, "move it"}, {item.MayTakeOff, "take it off"}} {
		if allowed.flag {
			may = append(may, allowed.what)
		}
	}
	if len(may) > 0 {
		fmt.Fprintf(w, "     You may %s.\n", strings.Join(may, ", "))
	}
}

// describeOutcome is the outcome in the words of the command line, with the
// status it put the decision in.
func describeOutcome(outcome api.MeetingOutcome) string {
	word := "an outcome this minka does not know"
	for spelled, minkas := range meetingOutcomeWords {
		if outcome.Outcome != nil && *outcome.Outcome == minkas {
			word = spelled
		}
	}
	return word + " (" + outcome.Status.Name + ")"
}

// describeVotes says how a vote went: the counts that were counted, then the
// note; empty where there is neither.
func describeVotes(votesFor, votesAgainst, abstentions *int, note *string) string {
	var parts []string
	for _, count := range []struct {
		value *int
		what  string
	}{{votesFor, "for"}, {votesAgainst, "against"}, {abstentions, "abstained"}} {
		if count.value != nil {
			parts = append(parts, fmt.Sprintf("%d %s", *count.value, count.what))
		}
	}
	if note != nil {
		parts = append(parts, *note)
	}
	return strings.Join(parts, ", ")
}

// meetingRefusal classifies a failed request about a meeting. A host whose
// Minka has no meetings answers with a page of its web instead of the API's
// own 404, and is told from an event that is not there by that.
func meetingRefusal(s *session, err error, notFoundHint string) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodeMeetingDayMissing:
		e.Hint = meetingDayMissingHint
	case apiCodeMeetingClosed:
		e.Hint = "The meeting is closed, and a closed meeting stays closed: its agenda keeps its items and their order. Only an outcome it recorded can still be corrected."
	case apiCodeDecisionAlreadyOnMeetingAgenda:
		e.Hint = "One of the decisions stands on this agenda already, or was named twice, and none was put on. The agenda as it stands: minka events meeting show <eventId>"
	case apiCodeDecisionNotForAMeetingAgenda:
		e.Hint = "Only a decision still to be taken goes on an agenda: one whose status has the role proposed or deferred, or a status the team added. None was put on. Propose it first: minka decisions status <decisionId> proposed --audience nobody"
	case apiCodeDecisionUnknown:
		e.Hint = "One of the decisions is none of the event's project that you may change: it does not exist, is in the trash or the archive, belongs to another project or is hidden from you. None was put on. The decisions of a tool: minka decisions list <toolId>"
	case apiCodeDecisionsMissing:
		e.Hint = "An agenda takes at least one decision: name it by its ID or its short identifier."
	case apiCodeAgendaPlaceUnknown:
		e.Hint = "The item named with --after or --before is the item itself or none of this agenda as you are shown it. The agenda in its order: minka events meeting show <eventId>"
	case apiCodeMeetingOutcomeUnknown:
		e.Hint = "The outcome is " + meetingOutcomeList + "."
	case apiCodeSentenceWithoutChanges:
		e.Hint = "--sentence goes with accepted-with-changes alone."
	case apiCodeDecisionTitleMissing:
		e.Hint = "accepted-with-changes needs the decision's changed sentence: give --sentence."
	case apiCodeDecisionTitleTooLong, apiCodeDecisionTitleHasControlChars:
		e.Hint = "--sentence is one line of at most 200 characters, without a line break or a tab."
	case apiCodeDecisionVotesTooLong, apiCodeDecisionVotesHasControlChars:
		e.Hint = "--votes is a note of one line of at most 100 characters, without a line break or a tab."
	case apiCodeDecisionVoteCountMalformed:
		e.Hint = "--votes-for, --votes-against and --abstentions are whole numbers from 0 to 99999."
	case apiCodeMeetingOutcomeHasNoStatus:
		e.Hint = "The Decisions tool of this decision has no status of the role this outcome puts it in: holds for an accepted one, rejected or deferred. Nothing was recorded. The statuses of the tool: minka decisions statuses <toolId>; the team sets them up in Minka on the web."
	case apiCodeMeetingAudienceUnknown:
		e.Hint = "A closed meeting is told to everybody, to chosen people with one --notify each, or to nobody."
	case apiCodeNotifyIDsMissing, apiCodeNotifyIDsWithoutChosenAudience, apiCodePersonNotAProjectMember:
		e.Hint, _ = audienceHint(e.APICode)
	default:
		if e.Code != CodeNotFound {
			break
		}
		e.Hint = notFoundHint
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Body == nil {
			e.Message = s.host + " does not know meetings"
			e.Hint = meetingsUnknownHint
		}
	}
	return e
}
