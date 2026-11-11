package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// What Minka answers a decision it will not take.
const (
	apiCodeDecisionStatusUnknown    = "decisionStatusUnknown"
	apiCodeDecisionEarlierTwice     = "decisionEarlierTwice"
	apiCodeDecisionEarlierUnknown   = "decisionEarlierUnknown"
	apiCodeDecisionEventUnknown     = "decisionEventUnknown"
	apiCodeDecisionAppliesToUnknown = "decisionAppliesToUnknown"
	apiCodeDecisionLabelUnknown     = "decisionLabelUnknown"
	apiCodeDecisionArchived         = "decisionArchived"
	apiCodeDecisionTitleMissing     = "decisionTitleMissing"
	apiCodeEntryTargetUnknown       = "entryTargetUnknown"
	apiCodeEntryAddressMalformed    = "entryAddressMalformed"
	apiCodeLinkDescriptionMissing   = "linkDescriptionMissing"
	apiCodeDecisionAudienceUnknown  = "decisionAudienceUnknown"
)

// decisionNames are the arguments and flags that name a decision, by its ID
// or by its short identifier.
var decisionNames = []string{"decisionId", "--supersedes", "--revokes"}

// decisionRoles are the roles a status goes by, as Minka spells them.
var decisionRoles = []string{"draft", "proposed", "holds", "done", "superseded", "revoked", "rejected"}

const decisionNotFoundHint = "There is no such decision, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you: a client never meets a draft."

const decisionsToolNotFoundHint = "There is no such Decisions tool, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. The tools of a project: minka decisions tools <projectId>"

const decisionStatusHelp = `A status is named by its ID or by its role, in any case: draft, proposed,
holds, done, superseded, revoked or rejected. A status the team added has no
role and is named by its ID; "decisions statuses <toolId>" lists them all. A
client of the project is never offered draft.`

const decisionBodyHelp = `The body is what more is written down beside the sentence and the reason:
Markdown by default, sent as HTML the way a description is, - to read it from
stdin. --body-format html sends HTML as it is.`

const decisionFieldsHelp = `--title is the sentence that holds, one line of at most 200 characters.
--reason is why, one sentence of at most 500. --decided-on is the day it was
decided, --valid-until the day Minka sets a decision in force to done, and
--review-on the day Minka reminds those who follow it to look at it again,
each YYYY-MM-DD.

--decided-by names one of who decided and can be given more than once, in the
order they are to stand: an ID or "me" names a member of the project, and
anything else is a name given freely, such as "Board", which is linked to
nobody. --event is the ID of an event of the project's calendars the decision
was taken in; without --decided-on the decision takes its day, unless it
repeats. --votes is one line such as "5 : 1".

--applies-to-card or --applies-to-event names the one card or event the
decision applies to: once the event is over or the card done, Minka sets a
decision in force to done. A card is its ID or its short identifier.

` + decisionAudienceHelp

const decisionNameHelp = `A decision is named by its ID or by its short identifier, such as ENT-14, in
any case and also under a former prefix of its tool; a short identifier is
looked up first, and the command goes on with the ID.`

const decisionAudienceHelp = `--audience says whom the decision reaches once it is proposed or in force:
"everybody" in the project, which Minka takes where --audience is not given,
"chosen" with one --notify per person, who then follow it, or "nobody". It
takes effect only where the decision comes into a status of role proposed or
holds: a new decision proposed or in force, or a change that moves it there
from another status. A draft, a status the team added and the other roles tell
nobody, and a change that keeps the status ignores --audience and --notify,
which Minka checks all the same. --notify belongs to "chosen" alone: Minka
refuses names under any other audience with notifyIdsWithoutChosenAudience
and "chosen" without a name with notifyIdsMissing; this command sends what it
is given and leaves the rule to Minka.
` + personHelp + ` A name or an e-mail address
is looked up among the members of the project. Somebody chosen who is a client
of the project, or an agent working for one, has to be shown the decision, or
Minka refuses with personCannotSeeIt.`

const decisionEarlierHelp = `The earlier decision moves into the status of role superseded or revoked
only while the new one holds, when it is named or when the new one comes into
force. Nothing moves it back: taking the new one out of force, or naming
another, leaves the earlier one where it is.`

func (a *app) newDecisionsCmd() *cobra.Command {
	return group("decisions", "Read and write the decisions of a Decisions tool: what holds, why, and what follows",
		a.newDecisionsToolsCmd(),
		a.newDecisionsToolCmd(),
		a.newDecisionsStatusesCmd(),
		a.newDecisionsListCmd(),
		a.newDecisionsShowCmd(),
		a.newDecisionsCreateCmd(),
		a.newDecisionsEditCmd(),
		a.newDecisionsStatusCmd(),
		a.newDecisionsLaterCmd("supersede"),
		a.newDecisionsLaterCmd("revoke"),
		a.newDecisionsDeleteCmd(),
		a.newItemLabelCmd(a.decisionLabelsTarget(), "decisionId", true),
		a.newItemLabelCmd(a.decisionLabelsTarget(), "decisionId", false),
		a.newDecisionsVersionsCmd(),
		a.newDecisionsEntryAddCmd("history"),
		a.newDecisionsEntryAddCmd("consequences"),
		a.newDecisionsEntriesCmd(),
		a.newItemCommentsCmd(decisionComments),
	)
}

func (a *app) newDecisionsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the Decisions tools of a project",
		Long: `Lists the Decisions tools of a project that are in use, in the order of the
project page. A tool no longer in use is not listed and is still read and
written at its own ID. Its decisions are read with "decisions list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.DecisionsTools(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "Decisions tool", "Decisions tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "holds", Cmd: "minka decisions list <toolId> --holds", Description: "List the decisions in force"},
				},
				text: func(w io.Writer) {
					if len(tools.Value) == 0 {
						fmt.Fprintln(w, "No Decisions tools.")
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

func (a *app) newDecisionsToolCmd() *cobra.Command {
	return a.newOneToolCmd(oneTool{
		group: "decisions", kind: "Decisions tool", holds: "decisions list <toolId>",
		notFoundHint: decisionsToolNotFoundHint,
		read:         asTool((*api.Client).DecisionsTool),
	})
}

func (a *app) newDecisionsStatusesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "statuses <toolId>",
		Short: "List the statuses a decision of a tool can stand in",
		Long: `Lists the statuses of a Decisions tool in their order: ID, name and role. A
status that goes by its role is named in English; one the team added has no
role. The list itself is written on the web.

` + decisionStatusHelp,
		Args: requireArgs("toolId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			statuses, err := s.client.DecisionStatuses(a.ctx, args[0])
			if err != nil {
				return toolRefusal(s, err, decisionsToolNotFoundHint)
			}
			return a.ok(success{
				data:    statuses.Raw,
				summary: plural(len(statuses.Value), "status", "statuses"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka decisions list " + args[0] + " --status <status>", Description: "List the decisions in one status"},
				},
				text: func(w io.Writer) {
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tNAME\tROLE")
					for _, status := range statuses.Value {
						fmt.Fprintf(table, "%s\t%s\t%s\n", deref(status.ID), status.Name, deref(status.Role))
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newDecisionsListCmd() *cobra.Command {
	var status, label, card, eventID string
	var holds, setByAgent bool

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List the decisions of a tool, in force or in any status",
		Long: `Lists the decisions of a tool the way its page does: the day decided on last
first, one without a day by the day it was written. Nothing is paged. The text
names each decision by its ID, its sentence, its status and its reason, so
that what holds is looked up at a glance; --json hands over every field.

Every status is listed unless --status names one; --holds is short for
--status holds, the decisions in force. ` + decisionStatusHelp + `

--label narrows to the decisions that carry a label, by its ID or its name; a
name is looked up among the labels of the project. --applies-to-card and
--applies-to-event narrow to the decisions that apply to that card or event.
--set-by-agent narrows to the decisions whose status an agent set by hand; a
status Minka moved on an agent's behalf is not among them.

Each line leads with the short identifier where the decision has one, and
says where an agent or Minka set its status.`,
		Example: `  minka decisions list 0199... --holds
  minka decisions list 0199... --status proposed --json
  minka decisions list 0199... --label Board --applies-to-card MNK-1A`,
		Args: requireArgs("toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if holds && cmd.Flags().Changed("status") {
				return usageErrorf(cmd, "--holds and --status exclude each other")
			}
			if card != "" && eventID != "" {
				return usageErrorf(cmd, "a decision applies to one card or one event: give --applies-to-card or --applies-to-event")
			}
			if err := checkCardFlag(cmd, "--applies-to-card", &card); err != nil {
				return err
			}
			if eventID != "" {
				if err := requireID(cmd, "--applies-to-event", &eventID); err != nil {
					return err
				}
			}
			if label != "" {
				if err := checkLabelValues(cmd, []string{label}); err != nil {
					return err
				}
			}
			query := api.DecisionsQuery{Status: statusValue(status), AppliesTo: eventID, SetByAnAgent: setByAgent}
			if holds {
				query.Status = "holds"
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if card != "" {
				if err := s.resolveCard(a.ctx, &card); err != nil {
					return err
				}
				query.AppliesTo = card
			}
			if label != "" {
				if query.LabelID, err = a.labelOfADecisionsTool(s, args[0], label); err != nil {
					return err
				}
			}
			decisions, err := s.client.Decisions(a.ctx, args[0], query)
			if err != nil {
				return decisionsToolRefusal(s, err)
			}
			summary := plural(len(decisions.Value), "decision", "decisions")
			if query.Status == "holds" {
				summary += " in force"
			}
			if setByAgent {
				summary += " whose status an agent set"
			}
			return a.ok(success{
				data:    decisions.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka decisions show <decisionId>", Description: "Read a decision with its body, history and comments"},
					{Action: "create", Cmd: "minka decisions create --tool " + args[0] + " --title <sentence> --reason <why>", Description: "Write a decision down here"},
				},
				text: func(w io.Writer) { renderDecisionLines(w, decisions.Value) },
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&status, "status", "", "Status to list: its ID, its role, or all")
	flags.BoolVar(&holds, "holds", false, "List only the decisions in force")
	flags.StringVar(&label, "label", "", "Label the decisions carry: its ID or its name")
	flags.StringVar(&card, "applies-to-card", "", "Card the decisions apply to: its ID or short identifier")
	flags.StringVar(&eventID, "applies-to-event", "", "ID of the event the decisions apply to")
	flags.BoolVar(&setByAgent, "set-by-agent", false, "List only the decisions whose status an agent set by hand")
	return cmd
}

func (a *app) newDecisionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <decisionId>",
		Short: "Show a decision with its body, history, consequences and comments",
		Long: `Shows a decision: its sentence, reason and status, who decided, when and in
which event, the votes, what it applies to, the days Minka acts on, the earlier
decision it supersedes or revokes and the later ones that took its place, its
history and consequences, its labels, its body and its comments. In text
output the body is converted to Markdown, which decisions edit --body turns
back into the same HTML; in JSON it is the HTML the server stores.

The text leads with the short identifier where the decision has one, says who
set its status and when, an agent or Minka marked as such, and gives the
decision's short address, or its page where it has none.

A decision in the archive is shown as well and says so: it is read, and not
changed. A client of the project meets only a decision shown to clients, and
never a draft.

` + decisionNameHelp,
		Example: `  minka decisions show ENT-14
  minka decisions show 0199... --json`,
		Args: requireArgs("decisionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			decision, err := s.readDecision(a.ctx, args[0])
			if err != nil {
				return err
			}
			return a.ok(decisionSuccess(decision, decisionSummary(decision.Value), nil, func(w io.Writer) { renderDecision(w, decision.Value) }))
		},
	}
}

// decisionFlags are the fields of a decision as the commands that write one
// take them. A field is sent only where its flag is given.
type decisionFlags struct {
	title, reason, body, bodyFormat         string
	decidedOn, validUntil, reviewOn, status string
	decidedBy                               []string
	eventID, votes, card, appliesToEventID  string
	supersedes, revokes                     string
	visibleToClients                        bool
	audience                                string
	notify                                  []string

	// The flags of a change that clear a field.
	noReason, noDecidedOn, noValidUntil, noReviewOn, noDecidedBy bool
	noEvent, noVotes, noAppliesTo, noEarlier                     bool
}

// register adds the flags of the fields; editing adds those that clear one,
// and earlier those that name the decision a new one takes the place of.
func (f *decisionFlags) register(cmd *cobra.Command, editing, earlier bool) {
	flags := cmd.Flags()
	flags.StringVar(&f.title, "title", "", "The sentence that holds")
	flags.StringVar(&f.reason, "reason", "", "Why, in one sentence")
	flags.StringVar(&f.body, "body", "", "What more is written down, - to read it from stdin")
	flags.StringVar(&f.bodyFormat, "body-format", "markdown", "Format of the body: markdown or html")
	flags.StringVar(&f.decidedOn, "decided-on", "", "Day it was decided, YYYY-MM-DD")
	flags.StringVar(&f.validUntil, "valid-until", "", "Day it is done, YYYY-MM-DD")
	flags.StringVar(&f.reviewOn, "review-on", "", "Day to look at it again, YYYY-MM-DD")
	flags.StringVar(&f.status, "status", "", "Status: its ID or its role")
	flags.StringArrayVar(&f.decidedBy, "decided-by", nil, "One of who decided: a member's ID, me, or a name given freely (repeatable)")
	flags.StringVar(&f.eventID, "event", "", "ID of the event it was decided in")
	flags.StringVar(&f.votes, "votes", "", "The votes, one line")
	flags.StringVar(&f.card, "applies-to-card", "", "Card it applies to: its ID or short identifier")
	flags.StringVar(&f.appliesToEventID, "applies-to-event", "", "ID of the event it applies to")
	registerDecisionAudience(cmd, &f.audience, &f.notify)
	if earlier {
		flags.StringVar(&f.supersedes, "supersedes", "", "Earlier decision this one takes the place of: its ID or short identifier")
		flags.StringVar(&f.revokes, "revokes", "", "Earlier decision this one lifts: its ID or short identifier")
	}
	if !editing {
		flags.BoolVar(&f.visibleToClients, "visible-to-clients", false, "Show the decision to the clients of the project")
		return
	}
	flags.BoolVar(&f.noReason, "no-reason", false, "Take the reason off")
	flags.BoolVar(&f.noDecidedOn, "no-decided-on", false, "Take the day it was decided off")
	flags.BoolVar(&f.noValidUntil, "no-valid-until", false, "Take the day it is done off")
	flags.BoolVar(&f.noReviewOn, "no-review-on", false, "Take the day of the review off")
	flags.BoolVar(&f.noDecidedBy, "no-decided-by", false, "Name nobody as having decided")
	flags.BoolVar(&f.noEvent, "no-event", false, "Name no event it was decided in")
	flags.BoolVar(&f.noVotes, "no-votes", false, "Take the votes off")
	flags.BoolVar(&f.noAppliesTo, "no-applies-to", false, "Let it apply to no card or event")
	flags.BoolVar(&f.noEarlier, "no-earlier", false, "Name no earlier decision it supersedes or revokes")
}

// check refuses what is wrong in the flags before anything is sent.
func (f *decisionFlags) check(cmd *cobra.Command) error {
	if err := checkBodyFormat(cmd, f.bodyFormat); err != nil {
		return err
	}
	changed := cmd.Flags().Changed
	for _, pair := range [][2]string{
		{"reason", "no-reason"}, {"decided-on", "no-decided-on"}, {"valid-until", "no-valid-until"},
		{"review-on", "no-review-on"}, {"decided-by", "no-decided-by"}, {"event", "no-event"}, {"votes", "no-votes"},
		{"applies-to-card", "no-applies-to"}, {"applies-to-event", "no-applies-to"},
		{"supersedes", "no-earlier"}, {"revokes", "no-earlier"},
		{"applies-to-card", "applies-to-event"}, {"supersedes", "revokes"},
	} {
		if changed(pair[0]) && changed(pair[1]) {
			return usageErrorf(cmd, "--%s and --%s exclude each other", pair[0], pair[1])
		}
	}
	for flag, value := range map[string]*string{"--event": &f.eventID, "--applies-to-event": &f.appliesToEventID, "--supersedes": &f.supersedes, "--revokes": &f.revokes} {
		if changed(strings.TrimPrefix(flag, "--")) {
			if err := requireID(cmd, flag, value); err != nil {
				return err
			}
		}
	}
	if changed("applies-to-card") {
		if err := checkCardFlag(cmd, "--applies-to-card", &f.card); err != nil {
			return err
		}
	}
	return checkDecisionAudience(cmd, f.audience)
}

// registerDecisionAudience adds the flags that say whom a decision proposed
// or in force reaches.
func registerDecisionAudience(cmd *cobra.Command, audience *string, notify *[]string) {
	cmd.Flags().StringVar(audience, "audience", "", "Whom it reaches once proposed or in force: everybody, chosen or nobody")
	cmd.Flags().StringArrayVar(notify, "notify", nil, "Person to tell, with --audience chosen: ID, me, e-mail address or name (repeatable)")
}

// checkDecisionAudience holds the word of --audience where it is given.
// Which people go with which audience is the server's rule and is left to it.
func checkDecisionAudience(cmd *cobra.Command, audience string) error {
	if cmd.Flags().Changed("audience") && audience != "everybody" && audience != "chosen" && audience != "nobody" {
		return usageErrorf(cmd, "--audience is everybody, chosen or nobody, not %q", audience)
	}
	return nil
}

// audienceInput puts the audience and the people to tell into a write where
// their flags are given; a name is looked up among the members of the
// project, which is read only then.
func (a *app) audienceInput(cmd *cobra.Command, s *session, input api.DecisionInput, audience string, notify []string, project func() (string, error)) error {
	if cmd.Flags().Changed("audience") {
		input["audience"] = audience
	}
	if cmd.Flags().Changed("notify") {
		ids, err := a.peopleOfATool(s, project, notify)
		if err != nil {
			return err
		}
		input["notifyIds"] = ids
	}
	return nil
}

// input is the request the flags make: a field for every flag given, null
// for every one cleared. The body is converted here, "me", the people to tell
// and the short identifier of a card or an earlier decision are looked up;
// project names the project whose members are told.
func (f *decisionFlags) input(cmd *cobra.Command, a *app, s *session, project func() (string, error)) (api.DecisionInput, []string, error) {
	input := api.DecisionInput{}
	var notices []string
	changed := cmd.Flags().Changed
	for _, earlier := range []*string{&f.supersedes, &f.revokes} {
		if err := s.resolveDecision(a.ctx, earlier); err != nil {
			return nil, nil, err
		}
	}
	for name, value := range map[string]string{
		"title": f.title, "reason": f.reason, "decided-on": f.decidedOn, "valid-until": f.validUntil,
		"review-on": f.reviewOn, "event": f.eventID, "votes": f.votes, "supersedes": f.supersedes, "revokes": f.revokes,
	} {
		if changed(name) {
			input[decisionField(name)] = value
		}
	}
	for name, cleared := range map[string]bool{
		"reason": f.noReason, "decided-on": f.noDecidedOn, "valid-until": f.noValidUntil, "review-on": f.noReviewOn,
		"decided-by": f.noDecidedBy, "event": f.noEvent, "votes": f.noVotes, "applies-to": f.noAppliesTo,
	} {
		if cleared {
			input[decisionField(name)] = nil
		}
	}
	if f.noEarlier {
		// Minka reads an empty one beside the other as both named.
		input["supersedes"], input["revokes"] = nil, nil
	}
	if changed("status") {
		input["status"] = statusValue(f.status)
	}
	if changed("body") {
		html, warnings, err := a.decisionBody(f.body, f.bodyFormat)
		if err != nil {
			return nil, nil, err
		}
		input["body"], notices = html, warnings
	}
	if changed("decided-by") {
		deciders := []api.DeciderInput{}
		for _, value := range f.decidedBy {
			decider, err := decidedBy(a, s, value)
			if err != nil {
				return nil, nil, err
			}
			deciders = append(deciders, decider)
		}
		input["decidedBy"] = deciders
	}
	if changed("applies-to-card") {
		if err := s.resolveCard(a.ctx, &f.card); err != nil {
			return nil, nil, err
		}
		input["appliesTo"] = map[string]string{"kind": "card", "id": f.card}
	}
	if changed("applies-to-event") {
		input["appliesTo"] = map[string]string{"kind": "event", "id": f.appliesToEventID}
	}
	if f.visibleToClients {
		input["visibleToClients"] = true
	}
	if err := a.audienceInput(cmd, s, input, f.audience, f.notify, project); err != nil {
		return nil, nil, err
	}
	return input, notices, nil
}

// decisionField is the name of the field of the API a flag writes.
func decisionField(flag string) string {
	switch flag {
	case "event":
		return "decidedInEventId"
	case "applies-to":
		return "appliesTo"
	}
	parts := strings.Split(flag, "-")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// decidedBy reads one of who decided: an ID or "me" is a member, anything
// else a name given freely.
func decidedBy(a *app, s *session, value string) (api.DeciderInput, error) {
	if id, isID := asID(value); isID {
		return api.DeciderInput{PersonID: id}, nil
	}
	if strings.EqualFold(strings.TrimSpace(value), "me") {
		id, err := a.meID(s)()
		if err != nil {
			return api.DeciderInput{}, err
		}
		return api.DeciderInput{PersonID: id}, nil
	}
	return api.DeciderInput{Name: value}, nil
}

// statusValue is a status as it goes out: in lowercase, the form Minka
// keeps an ID and a role in.
func statusValue(value string) string {
	return strings.ToLower(value)
}

// checkCardFlag takes a card by its ID or its short identifier.
func checkCardFlag(cmd *cobra.Command, flag string, value *string) error {
	if *value == "" {
		return nil
	}
	if id, isID := asID(*value); isID {
		*value = id
		return nil
	}
	if shortIdentifierPattern.MatchString(*value) {
		return nil
	}
	return usageErrorf(cmd, "%s %q is no card; a card is named by its ID or its short identifier such as MNK-1A", flag, *value)
}

func (a *app) newDecisionsCreateCmd() *cobra.Command {
	var f decisionFlags
	var toolID, from string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Write a decision down in a Decisions tool, or several from a file",
		Long: "Writes a decision down in a Decisions tool. Without --status it holds.\n\n" +
			decisionFieldsHelp + "\n\n" + decisionStatusHelp + "\n\n" + decisionBodyHelp + `

--supersedes or --revokes names an earlier decision of the same tool by its ID
or its short identifier. ` + decisionEarlierHelp + ` decisions supersede and
decisions revoke do the same with the earlier decision named first.

--visible-to-clients shows the decision to the clients of the project, where
the project works with them; what a client writes down is shown to clients
anyway.

--from writes several decisions, read from a file or, with -, from stdin: a
JSON list of objects, each with the fields Minka takes for a new decision —
title, reason, body, decidedOn, status, decidedBy (a list of {"personId"} and
{"name"}), decidedInEventId, votes, supersedes or revokes (an ID or a short
identifier, looked up before anything is written), appliesTo ({"kind": "card"
or "event", "id"}), validUntil, reviewOn, audience, notifyIds (a list of IDs of
members) and visibleToClients. A body is Markdown unless --body-format html is
given. A field Minka does not take is refused before anything is sent. The decisions are written one after
another in the order given; the first refusal stops the run, and the error
says which were written before it ("created" in data, as Minka answered, and
"notCreated", the titles of the rest). On success data is the list of the
decisions written. --from takes --tool and --body-format beside it, no other
flag.`,
		Example: `  minka decisions create --tool 0199... --title "We meet on Mondays" --reason "Then everybody has time." --json
  minka decisions create --tool 0199... --title "We buy a tent" --status proposed --decided-by me --decided-by Board
  minka decisions create --tool 0199... --title "We buy a tent" --status proposed --audience chosen --notify me --notify Anke
  minka decisions create --tool 0199... --from decisions.json`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			if err := f.check(cmd); err != nil {
				return err
			}
			if cmd.Flags().Changed("from") {
				var others []string
				cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
					if flag.Changed && !slices.Contains([]string{"from", "tool", "body-format"}, flag.Name) {
						others = append(others, "--"+flag.Name)
					}
				})
				if len(others) > 0 {
					return usageErrorf(cmd, "--from takes the fields from the file and no %s beside it", strings.Join(others, ", "))
				}
				return a.createDecisionsFrom(cmd, toolID, from, f.bodyFormat)
			}
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "a decision needs --title, the sentence that holds")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			input, notices, err := f.input(cmd, a, s, func() (string, error) {
				tool, err := s.client.DecisionsTool(a.ctx, toolID)
				if err != nil {
					return "", toolRefusal(s, err, decisionsToolNotFoundHint)
				}
				return tool.Value.ProjectID, nil
			})
			if err != nil {
				return err
			}
			decision, err := s.client.CreateDecision(a.ctx, toolID, input)
			if err != nil {
				return createDecisionRefusal(s, err)
			}
			notices = append(notices, earlierNotice(decision.Value)...)
			summary := "Wrote decision " + decisionTitle(decision.Value.ShortID, decision.Value.Title) + " down in " + decision.Value.Tool.Title
			return a.ok(decisionSuccess(decision, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nDecision: %s\nStatus: %s\n%s\n", summary, decisionName(decision.Value), decision.Value.Status.Name, decisionAddress(decision.Value))
			}))
		},
	}
	f.register(cmd, false, true)
	cmd.Flags().StringVar(&toolID, "tool", "", "ID of the Decisions tool the decision goes into")
	cmd.Flags().StringVar(&from, "from", "", "File with a JSON list of decisions to write, - for stdin")
	return cmd
}

// decisionFromFields are the fields an entry of --from may have: those Minka
// takes for a new decision.
var decisionFromFields = []string{
	"title", "reason", "body", "decidedOn", "status", "decidedBy", "decidedInEventId", "votes",
	"supersedes", "revokes", "appliesTo", "validUntil", "reviewOn", "audience", "notifyIds", "visibleToClients",
}

// createDecisionsFrom writes the decisions of a list one after another and
// stops at the first refusal, handing over what was written before it.
func (a *app) createDecisionsFrom(cmd *cobra.Command, toolID, from, bodyFormat string) error {
	content, err := a.readFileOrStdin("the list of decisions", from)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(content)))
	var entries []map[string]any
	if err := decoder.Decode(&entries); err != nil {
		return usageErrorf(cmd, "--from takes a JSON list of objects, one per decision: %s", err)
	}
	if len(entries) == 0 {
		return usageErrorf(cmd, "--from names no decision; the list is empty")
	}
	var inputs []api.DecisionInput
	var notices []string
	for i, entry := range entries {
		for name := range entry {
			if !slices.Contains(decisionFromFields, name) {
				return usageErrorf(cmd, "decision %d: Minka takes no field %q for a new decision; it takes %s", i+1, name, strings.Join(decisionFromFields, ", "))
			}
		}
		for _, name := range []string{"supersedes", "revokes"} {
			if value, isText := entry[name].(string); isText && value != "" && !uuidPattern.MatchString(value) && !shortIdentifierPattern.MatchString(value) {
				return usageErrorf(cmd, "decision %d: %s %q is no decision; a decision is named by its ID or its short identifier such as ENT-14", i+1, name, value)
			}
		}
		if body, isText := entry["body"].(string); isText {
			html, warnings, err := decisionBodyOf(body, bodyFormat)
			if err != nil {
				return err
			}
			entry["body"] = html
			for _, warning := range warnings {
				notices = append(notices, fmt.Sprintf("decision %d: %s", i+1, warning))
			}
		}
		idsInLowercase(entry)
		inputs = append(inputs, api.DecisionInput(entry))
	}

	s, err := a.session()
	if err != nil {
		return err
	}
	// A short identifier of an earlier decision is looked up before the first
	// write, so that one nobody can find writes nothing.
	for _, entry := range entries {
		for _, name := range []string{"supersedes", "revokes"} {
			value, isText := entry[name].(string)
			if !isText || value == "" {
				continue
			}
			if err := s.resolveDecision(a.ctx, &value); err != nil {
				return err
			}
			entry[name] = value
		}
	}
	created := []json.RawMessage{}
	var titles []string
	for i, input := range inputs {
		decision, err := s.client.CreateDecision(a.ctx, toolID, input)
		if err != nil {
			e := createDecisionRefusal(s, err)
			notCreated := []string{}
			for _, rest := range entries[i:] {
				title, _ := rest["title"].(string)
				notCreated = append(notCreated, title)
			}
			title, _ := entries[i]["title"].(string)
			e.Message = fmt.Sprintf("decision %d (%q): %s", i+1, title, e.Message)
			e.Data = map[string]any{"created": created, "notCreated": notCreated}
			e.Notices = notices
			if len(titles) > 0 {
				e.Notices = append(e.Notices, "written before the refusal: "+strings.Join(titles, "; "))
			}
			return e
		}
		created = append(created, decision.Raw)
		titles = append(titles, decision.Value.Title)
		notices = append(notices, earlierNotice(decision.Value)...)
	}
	summary := fmt.Sprintf("Wrote %s down", plural(len(created), "decision", "decisions"))
	return a.ok(success{
		data:    created,
		summary: summary,
		notices: notices,
		breadcrumbs: []Breadcrumb{
			{Action: "list", Cmd: "minka decisions list " + toolID, Description: "List the decisions of the tool"},
		},
		text: func(w io.Writer) {
			fmt.Fprintf(w, "%s:\n", summary)
			for _, title := range titles {
				fmt.Fprintf(w, "- %s\n", title)
			}
		},
	})
}

// idsInLowercase writes the IDs an entry of --from names, and its status, in
// the form Minka keeps them in.
func idsInLowercase(entry map[string]any) {
	lower := func(value any) any {
		if text, isText := value.(string); isText {
			if id, isID := asID(text); isID {
				return id
			}
		}
		return value
	}
	for _, name := range []string{"status", "decidedInEventId", "supersedes", "revokes"} {
		if value, given := entry[name]; given {
			entry[name] = lower(value)
		}
	}
	if subject, isObject := entry["appliesTo"].(map[string]any); isObject {
		subject["id"] = lower(subject["id"])
	}
	if people, isList := entry["notifyIds"].([]any); isList {
		for i := range people {
			people[i] = lower(people[i])
		}
	}
	if deciders, isList := entry["decidedBy"].([]any); isList {
		for _, decider := range deciders {
			if object, isObject := decider.(map[string]any); isObject {
				if _, given := object["personId"]; given {
					object["personId"] = lower(object["personId"])
				}
			}
		}
	}
}

// readFileOrStdin reads a file, or stdin for -.
func (a *app) readFileOrStdin(what, path string) (string, error) {
	if path == "-" {
		return a.readValue(what, "-")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("reading %s: %s", what, err)}
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("reading %s: %s", what, err)}
	}
	if len(content) > maxInputBytes {
		return "", &Error{Code: CodeUsage, Message: fmt.Sprintf("%s in %s is longer than %d bytes", what, path, maxInputBytes)}
	}
	return strings.TrimPrefix(string(content), byteOrderMark), nil
}

func (a *app) newDecisionsLaterCmd(verb string) *cobra.Command {
	var f decisionFlags
	relation, done, role := "supersedes", "Superseded", "superseded"
	short := "Write a decision down that takes the place of an earlier one"
	if verb == "revoke" {
		relation, done, role = "revokes", "Revoked", "revoked"
		short = "Write a decision down that lifts an earlier one"
	}

	cmd := &cobra.Command{
		Use:   verb + " <decisionId>",
		Short: short,
		Long: fmt.Sprintf(`Writes a new decision down in the tool of the earlier one, which it %s:
the same as decisions create --%s <decisionId>, with the same flags. Without
--status the new one holds.

%s The answer names the earlier decision in %s with the status
it stands in now, and a notice says so where it did not move.

`, relation, relation, decisionEarlierHelp, relation) + decisionNameHelp + "\n\n" + decisionFieldsHelp + "\n\n" + decisionStatusHelp + "\n\n" + decisionBodyHelp,
		Example: fmt.Sprintf(`  minka decisions %s ENT-14 --title "We meet on Tuesdays" --reason "The hall is free then."`, verb),
		Args:    requireArgs("decisionId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.check(cmd); err != nil {
				return err
			}
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "a decision needs --title, the sentence that holds")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			earlier, err := s.readDecision(a.ctx, args[0])
			if err != nil {
				return err
			}
			input, notices, err := f.input(cmd, a, s, func() (string, error) { return earlier.Value.ProjectID, nil })
			if err != nil {
				return err
			}
			input[relation] = earlier.Value.ID
			decision, err := s.client.CreateDecision(a.ctx, earlier.Value.ToolID, input)
			if err != nil {
				return createDecisionRefusal(s, err)
			}
			notices = append(notices, earlierNotice(decision.Value)...)
			summary := fmt.Sprintf("Wrote decision %s down; it %s %s", decisionTitle(decision.Value.ShortID, decision.Value.Title), relation, decisionTitle(earlier.Value.ShortID, earlier.Value.Title))
			return a.ok(decisionSuccess(decision, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nDecision: %s\nStatus: %s\n", summary, decisionName(decision.Value), decision.Value.Status.Name)
				if link := earlierOf(decision.Value); link != nil {
					state := "moved into " + link.Status.Name
					if deref(link.Status.Role) != role {
						state = "still " + link.Status.Name + ": it moves only while the new one holds"
					}
					fmt.Fprintf(w, "%s: %s, %s\n", done, decisionTitle(link.ShortID, link.Title), state)
				}
				fmt.Fprintln(w, decisionAddress(decision.Value))
			}))
		},
	}
	f.register(cmd, false, false)
	return cmd
}

// earlierOf is the earlier decision a decision supersedes or revokes.
func earlierOf(decision api.Decision) *api.DecisionLink {
	if decision.Supersedes != nil {
		return decision.Supersedes
	}
	return decision.Revokes
}

// earlierNotice says where the earlier decision a write named stays in its
// status: it moves only while the new one holds.
func earlierNotice(decision api.Decision) []string {
	link, role := decision.Supersedes, "superseded"
	if link == nil {
		link, role = decision.Revokes, "revoked"
	}
	if link == nil || deref(link.Status.Role) == role {
		return nil
	}
	return []string{fmt.Sprintf("the earlier decision %q stays %s: it moves into %s only while this one holds", link.Title, link.Status.Name, role)}
}

func (a *app) newDecisionsEditCmd() *cobra.Command {
	var f decisionFlags
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <decisionId>",
		Short: "Change what a decision says, its status or the decision it follows",
		Long: `Changes the fields given; what is not given stays as it is. Each field is
cleared by its --no-... flag: --no-reason, --no-decided-on, --no-valid-until,
--no-review-on, --no-decided-by, --no-event, --no-votes, --no-applies-to, and
--no-earlier for the decision it supersedes or revokes. --body "" empties the
body. --title "" does not empty the sentence: a decision needs one, and exits
9 (validation) with meta.apiCode decisionTitleMissing. --decided-by names the
whole list of who decided anew. The same values again succeed and change
nothing.

--supersedes or --revokes names another earlier decision. ` + decisionEarlierHelp + `

--audience and --notify go with --status alone, the only change that tells
anybody: without --status they exit 1 (usage) before anything is sent.

A decision in the archive is read and not changed: Minka answers a change of
one with exit 9 (validation) and meta.apiCode decisionArchived.

` + decisionNameHelp + `

` + decisionFieldsHelp + "\n\n" + decisionStatusHelp + "\n\n" + decisionBodyHelp + `

Files and mentions stand in a body as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource from decisions show
--json, change it and send it back with --body-format html. A new body that
would drop them is refused unless --drop-attachments is given; the command
reads the decision first to know.`,
		Example: `  minka decisions edit 0199... --reason "The hall is free on Mondays."
  minka decisions edit ENT-14 --status holds --audience nobody
  minka decisions edit 0199... --no-earlier --no-votes`,
		Args: requireArgs("decisionId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.check(cmd); err != nil {
				return err
			}
			given := false
			cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
				if flag.Changed && flag.Name != "body-format" && flag.Name != "drop-attachments" {
					given = true
				}
			})
			if !given {
				return usageErrorf(cmd, "nothing to change; give a field to change or to clear")
			}
			if err := audienceWithStatus(cmd); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			current, err := s.decisionOnce(a.ctx, &args[0])
			if err != nil {
				return err
			}
			input, notices, err := f.input(cmd, a, s, projectOf(current))
			if err != nil {
				return err
			}
			if body, given := input["body"].(string); given {
				current, err := current()
				if err != nil {
					return err
				}
				dropped, err := a.guardOverFiles(s, current.Value.ProjectID, closedIf(current.Value.Archived), firstNonEmpty(current.Value.BodySource, current.Value.Body), body, dropAttachments,
					"the decision has %s, and the new body would drop them",
					"Change the bodySource from decisions show --json and send it back with --body-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}
			decision, err := s.client.UpdateDecision(a.ctx, args[0], input)
			if err != nil {
				return decisionRefusal(s, err)
			}
			notices = append(notices, earlierNotice(decision.Value)...)
			summary := "Changed decision " + decisionTitle(decision.Value.ShortID, decision.Value.Title)
			return a.ok(decisionSuccess(decision, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nStatus: %s\n%s\n", summary, decision.Value.Status.Name, decisionAddress(decision.Value))
			}))
		},
	}
	f.register(cmd, true, true)
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a body that has attachments, mentions or references, which drops them")
	return cmd
}

func (a *app) newDecisionsStatusCmd() *cobra.Command {
	var audience string
	var notify []string
	cmd := &cobra.Command{
		Use:   "status <decisionId> <status>",
		Short: "Put a decision into another status",
		Long: `Puts a decision into a status of its tool and changes nothing else. ` + decisionStatusHelp + `

A status set already succeeds and changes nothing. A decision that comes into
force moves the earlier decision it supersedes or revokes into the status of
that role; nothing moves it back. A decision in the archive takes no status:
Minka answers with exit 9 (validation) and meta.apiCode decisionArchived.

` + decisionAudienceHelp + `

` + decisionNameHelp,
		Example: `  minka decisions status ENT-14 holds
  minka decisions status 0199... proposed --audience chosen --notify Anke
  minka decisions status 0199... rejected --json`,
		Args: requireArgs("decisionId", "status"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDecisionAudience(cmd, audience); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			current, err := s.decisionOnce(a.ctx, &args[0])
			if err != nil {
				return err
			}
			input := api.DecisionInput{"status": statusValue(args[1])}
			if err := a.audienceInput(cmd, s, input, audience, notify, projectOf(current)); err != nil {
				return err
			}
			decision, err := s.client.UpdateDecision(a.ctx, args[0], input)
			if err != nil {
				return decisionRefusal(s, err)
			}
			summary := "Decision " + decisionTitle(decision.Value.ShortID, decision.Value.Title) + " is " + decision.Value.Status.Name
			return a.ok(decisionSuccess(decision, summary, earlierNotice(decision.Value), func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n%s\n", summary, decisionAddress(decision.Value))
			}))
		},
	}
	registerDecisionAudience(cmd, &audience, &notify)
	return cmd
}

// audienceWithStatus refuses an audience on a change that names no status:
// only a status newly set tells anybody, and Minka ignores it otherwise.
func audienceWithStatus(cmd *cobra.Command) error {
	changed := cmd.Flags().Changed
	if (changed("audience") || changed("notify")) && !changed("status") {
		return usageErrorf(cmd, "--audience and --notify go with --status: only a new status tells anybody, and a change without one tells nobody")
	}
	return nil
}

// decisionOnce replaces a short identifier by the ID and returns a read of
// the decision that asks the server once at most: the lookup of a short
// identifier is that read already, an ID is read only when it is asked.
func (s *session) decisionOnce(ctx context.Context, value *string) (func() (api.Result[api.Decision], error), error) {
	var read *api.Result[api.Decision]
	if !uuidPattern.MatchString(*value) {
		decision, err := s.readDecision(ctx, *value)
		if err != nil {
			return nil, err
		}
		*value, read = decision.Value.ID, &decision
	}
	id := *value
	return func() (api.Result[api.Decision], error) {
		if read == nil {
			decision, err := s.readDecision(ctx, id)
			if err != nil {
				return decision, err
			}
			read = &decision
		}
		return *read, nil
	}, nil
}

// projectOf is the project of the decision a read gives.
func projectOf(decision func() (api.Result[api.Decision], error)) func() (string, error) {
	return func() (string, error) {
		read, err := decision()
		if err != nil {
			return "", err
		}
		return read.Value.ProjectID, nil
	}
}

// readDecision reads a decision named by its ID or by its short identifier,
// which is looked up in capitals.
func (s *session) readDecision(ctx context.Context, value string) (api.Result[api.Decision], error) {
	if uuidPattern.MatchString(value) {
		decision, err := s.client.Decision(ctx, value)
		if err != nil {
			return decision, decisionRefusal(s, err)
		}
		return decision, nil
	}
	identifier := strings.ToUpper(value)
	decision, err := s.client.DecisionByShortIdentifier(ctx, identifier)
	if err != nil {
		e := decisionRefusal(s, err)
		if e.Code == CodeNotFound {
			e.Message = "no decision you can see goes by " + identifier
			e.Hint = "It does not exist, its tool has not numbered it yet, it is in the trash or in a project you are not a member of, or you are a client of the project and it is not shown to you: a client never meets a draft. The ID of a decision always works; minka decisions list <toolId> gives both."
		}
		return decision, e
	}
	// What comes back goes into the path of the requests that follow.
	if !uuidPattern.MatchString(decision.Value.ID) {
		return decision, &Error{Code: CodeAPI, Message: s.host + " answered for " + identifier + " with a decision that has no ID"}
	}
	return decision, nil
}

// resolveDecision replaces the short identifier of a decision by its ID, so
// that a renamed prefix does not change what the requests that follow name.
func (s *session) resolveDecision(ctx context.Context, value *string) error {
	if *value == "" || uuidPattern.MatchString(*value) {
		return nil
	}
	decision, err := s.readDecision(ctx, *value)
	if err != nil {
		return err
	}
	*value = decision.Value.ID
	return nil
}

func (a *app) newDecisionsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <decisionId>",
		Aliases: []string{"trash"},
		Short:   "Move a decision to the trash",
		Long: `Moves a decision to the trash, from the list or from the archive. A client of
the project removes only a decision of their own; another exits 2
(not_found).

A decision in the trash reads like one that never existed: showing, changing
or deleting it again exits 2. It comes back in Minka on the web only.

` + decisionNameHelp,
		Args: requireArgs("decisionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveDecision(a.ctx, &args[0]); err != nil {
				return err
			}
			if err := s.client.DeleteDecision(a.ctx, args[0]); err != nil {
				return decisionRefusal(s, err)
			}
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: "Moved decision " + args[0] + " to the trash",
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Moved decision %s to the trash.\n", args[0])
				},
			})
		},
	}
}

func (a *app) decisionLabelsTarget() itemLabelsTarget {
	return itemLabelsTarget{kind: "decisions", noun: "decision", show: "minka decisions show", example: "minka decisions",
		resolve: func(s *session, id *string) error { return s.resolveDecision(a.ctx, id) },
		read: func(s *session, id string) (json.RawMessage, []api.Label, error) {
			decision, err := s.client.Decision(a.ctx, id)
			if err != nil {
				return nil, nil, err
			}
			return rawField(decision.Raw, "labels"), decision.Value.Labels, nil
		}}
}

// labelOfADecisionsTool is the ID of a label named for a list: an ID as it
// is, a name looked up among the labels of the tool's project, which the
// filter of the list does not read.
func (a *app) labelOfADecisionsTool(s *session, toolID, value string) (string, error) {
	if id, isID := asID(value); isID {
		return id, nil
	}
	tool, err := s.client.DecisionsTool(a.ctx, toolID)
	if err != nil {
		return "", toolRefusal(s, err, decisionsToolNotFoundHint)
	}
	labels, err := s.client.Labels(a.ctx, api.LabelPlace{Kind: "projects", ID: tool.Value.ProjectID})
	if err != nil {
		return "", labelRefusal(s, err)
	}
	for _, label := range labels.Value {
		if labelNameKey(label.Name) == labelNameKey(value) {
			return label.ID, nil
		}
	}
	return "", &Error{Code: CodeNotFound,
		Message: fmt.Sprintf("project %s has no label %q", tool.Value.ProjectID, value),
		Hint:    "A decision takes the labels of its project. List them with: minka labels list --project " + tool.Value.ProjectID}
}

func (a *app) newDecisionsVersionsCmd() *cobra.Command {
	const notFound = "There is no such decision or version, the decision belongs to a project you are not a member of, or you are a client of the project: the versions are the team's alone, also of a decision shown to clients."
	list := &cobra.Command{
		Use:   "list <decisionId>",
		Short: "List the versions of a decision, oldest first",
		Long: `Lists every version a decision had, oldest first: its number, who wrote it and
when. The newest is current, which is what decisions show gives. A version
keeps the sentence, the reason, the body, the day, who decided, the event and
the votes; a change of the status or of the earlier decision makes none.

The versions are the team's alone: a client of the project gets exit 2
(not_found), even for a decision shown to clients.`,
		Args: requireArgs("decisionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveDecision(a.ctx, &args[0]); err != nil {
				return err
			}
			versions, err := s.client.DecisionVersions(a.ctx, args[0])
			if err != nil {
				return versionsRefusal(s, err, notFound)
			}
			versionID := "<versionId>"
			if len(versions.Value) > 0 {
				versionID = versions.Value[0].ID
			}
			return a.ok(success{
				data:    versions.Raw,
				summary: plural(len(versions.Value), "version", "versions") + " of the decision",
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka decisions versions show " + args[0] + " " + versionID, Description: "Read one version"},
					{Action: "decision", Cmd: "minka decisions show " + args[0], Description: "Read the decision as it stands"},
				},
				text: func(w io.Writer) {
					if len(versions.Value) == 0 {
						fmt.Fprintln(w, "No versions.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "NUMBER\tID\tWRITTEN\tAUTHOR\t")
					for _, version := range versions.Value {
						fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%s\n", version.Number, version.ID, formatTime(version.WrittenAt), authorOf(version.Author), currentMark(version.Current))
					}
					_ = table.Flush()
				},
			})
		},
	}
	show := &cobra.Command{
		Use:   "show <decisionId> <versionId>",
		Short: "Show one version of a decision",
		Long: `Shows one version of a decision as it was written: the sentence, the reason,
the day, who decided, the event, the votes and the body. In text output the
body is converted to Markdown; in JSON it is the HTML the server stored. The
IDs come from decisions versions list.`,
		Args: requireArgs("decisionId", "versionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveDecision(a.ctx, &args[0]); err != nil {
				return err
			}
			version, err := s.client.DecisionVersionOf(a.ctx, args[0], args[1])
			if err != nil {
				return versionsRefusal(s, err, notFound)
			}
			v := version.Value
			return a.ok(success{
				data:    version.Raw,
				summary: fmt.Sprintf("Version %d of %s", v.Number, v.Title),
				breadcrumbs: []Breadcrumb{
					{Action: "versions", Cmd: "minka decisions versions list " + args[0], Description: "List the versions of the decision"},
					{Action: "show", Cmd: "minka decisions show " + args[0], Description: "Read the decision as it stands"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "# %s\n", v.Title)
					fmt.Fprintf(w, "Version: %d%s\n", v.Number, map[bool]string{true: ", the current one", false: ""}[v.Current])
					fmt.Fprintf(w, "Written: %s by %s\n", formatTime(v.WrittenAt), authorOf(v.Author))
					if v.Reason != nil {
						fmt.Fprintf(w, "Reason: %s\n", *v.Reason)
					}
					renderDecisionCircumstances(w, v.DecidedOn, v.DecidedBy, v.DecidedIn, v.Votes)
					fmt.Fprintf(w, "URL: %s\n", v.AppURL)
					if body := richtext.ToMarkdown(v.Body); body != "" {
						fmt.Fprintf(w, "\n## Body\n\n%s\n", body)
					}
					renderEntryFiles(w, v.Attachments)
				},
			})
		},
	}
	return group("versions", "Read the earlier versions of a decision", list, show)
}

// newDecisionsEntryAddCmd is "history add" or "consequences add".
func (a *app) newDecisionsEntryAddCmd(part string) *cobra.Command {
	var description string
	consequence := part == "consequences"
	short, long := "Add what led to a decision to its history", `Appends to the history of a decision what led to it: the address of a record
of this Minka, such as a card, a to-do, a post or a document, a comment by its
#comment-… place, a line of a chat by its #chat-message-… place, a card by its
"#" name such as #MNK-1A, a decision by its "#" name such as #ENT-14 or by its
short address (shortUrl, .../go/decision/ENT-14), or any other web address
together with --description, a few words that say what it is.`
	if consequence {
		short, long = "Add what follows from a decision to its consequences", `Appends to the consequences of a decision what follows from it: a to-do or a
card, by the address of its page or a card by its "#" name such as #MNK-1A.
A decision is no consequence. decisions show counts the consequences done.`
	}
	add := &cobra.Command{
		Use:   "add <decisionId> <address>",
		Short: short,
		Long: long + `

Naming what stands there already succeeds and answers with the entry there.
An address that is none exits 9 (validation) with meta.apiCode
entryAddressMalformed, one that names nothing you see, or nothing this part
takes, with entryTargetUnknown. A decision in the archive takes no entry:
decisionArchived.`,
		Example: fmt.Sprintf("  minka decisions %s add 0199... '#MNK-1A'", part),
		Args:    requireArgs("decisionId", "address"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveDecision(a.ctx, &args[0]); err != nil {
				return err
			}
			entry, err := s.client.AddDecisionEntry(a.ctx, args[0], consequence, args[1], description)
			if err != nil {
				return decisionRefusal(s, err)
			}
			summary := fmt.Sprintf("Added %s to the %s of decision %s", entry.Value.Title, part, args[0])
			return a.ok(success{
				data:    entry.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka decisions show " + args[0], Description: "Read the decision with its entries"},
					{Action: "move", Cmd: "minka decisions entries move " + args[0] + " " + entry.Value.ID + " --top", Description: "Move the entry"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\nEntry: %s\n%s\n", summary, entry.Value.ID, entry.Value.AppURL) },
			})
		},
	}
	if !consequence {
		add.Flags().StringVar(&description, "description", "", "A few words that describe a web address outside Minka")
	}
	return group(part, map[bool]string{false: "Add to the history of a decision", true: "Add to the consequences of a decision"}[consequence], add)
}

func (a *app) newDecisionsEntriesCmd() *cobra.Command {
	var after, before string
	var top, bottom bool
	move := &cobra.Command{
		Use:   "move <decisionId> <entryId>",
		Short: "Move an entry of the history or the consequences of a decision",
		Long: `Puts an entry elsewhere within its part, the history or the consequences:
--top puts it first, --bottom last, --after below the entry named, --before
above it. Exactly one of the four is required. The entry named with --after or
--before has to be another entry of the same decision; otherwise exit 9
(validation) with meta.apiCode entryPlaceUnknown. The IDs of the entries are
in decisions show --json.`,
		Example: `  minka decisions entries move 0199... 0199... --top`,
		Args:    requireArgs("decisionId", "entryId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, bottom, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the entry goes: give exactly one of --top, --bottom, --after and --before")
			}
			var input api.MoveDecisionEntryInput
			if after != "" {
				if err := requireID(cmd, "--after", &after); err != nil {
					return err
				}
				input.PreviousEntryID = after
			}
			if before != "" {
				if err := requireID(cmd, "--before", &before); err != nil {
					return err
				}
				input.NextEntryID = before
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			current, err := s.decisionOnce(a.ctx, &args[0])
			if err != nil {
				return err
			}
			if bottom {
				// Minka has no word for the end: an entry goes there behind
				// the last of the others of its part.
				decision, err := current()
				if err != nil {
					return err
				}
				for _, part := range [][]api.DecisionEntry{decision.Value.History, decision.Value.Consequences} {
					if !slices.ContainsFunc(part, func(entry api.DecisionEntry) bool { return entry.ID == args[1] }) {
						continue
					}
					for _, other := range part {
						if other.ID != args[1] {
							input.PreviousEntryID = other.ID
						}
					}
				}
			}
			decision, err := s.client.MoveDecisionEntry(a.ctx, args[0], args[1], input)
			if err != nil {
				return decisionRefusal(s, err)
			}
			summary := "Moved an entry of decision " + decision.Value.Title
			return a.ok(decisionSuccess(decision, summary, nil, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n", summary)
				renderDecisionEntries(w, decision.Value)
			}))
		},
	}
	move.Flags().StringVar(&after, "after", "", "ID of the entry to put it below")
	move.Flags().StringVar(&before, "before", "", "ID of the entry to put it above")
	move.Flags().BoolVar(&top, "top", false, "Put it first")
	move.Flags().BoolVar(&bottom, "bottom", false, "Put it last")

	remove := &cobra.Command{
		Use:   "delete <decisionId> <entryId>",
		Short: "Take an entry off the history or the consequences of a decision",
		Long: `Takes an entry off a decision; what it names stays as it is. An entry that is
not there, or that you are not shown, exits 2 (not_found).`,
		Args: requireArgs("decisionId", "entryId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveDecision(a.ctx, &args[0]); err != nil {
				return err
			}
			if err := s.client.DeleteDecisionEntry(a.ctx, args[0], args[1]); err != nil {
				return decisionRefusal(s, err)
			}
			return a.ok(success{
				data:    map[string]any{"decisionId": args[0], "entryId": args[1], "deleted": true},
				summary: "Took entry " + args[1] + " off decision " + args[0],
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka decisions show " + args[0], Description: "Read the decision with its entries"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "Took entry %s off decision %s.\n", args[1], args[0]) },
			})
		},
	}
	return group("entries", "Move or take off the entries of the history and the consequences of a decision", move, remove)
}

// decisionComments are the comments under a decision. Only the archive of
// its project closes a decision to them.
var decisionComments = commentedKind{
	parent: "decisions", noun: "decision", aNoun: "a decision", arg: "decisionId", kind: api.CommentedDecision,
	read: func(a *app, s *session, id string) (commentedItem, error) {
		decision, err := s.client.Decision(a.ctx, id)
		if err != nil {
			return commentedItem{}, err
		}
		return commentedItem{projectID: decision.Value.ProjectID, closed: closedIf(false), comments: decision.Value.Comments}, nil
	},
	refusal: func(s *session, err error, _ string) *Error { return decisionRefusal(s, err) },
	resolve: func(a *app, s *session, id *string) error { return s.resolveDecision(a.ctx, id) },
	reactHelp: `Only the comments take reactions through the API, not the decision
itself.`,
}

// decisionBody reads the body of a flag, - from stdin, and converts it.
func (a *app) decisionBody(value, format string) (string, []string, error) {
	text, err := a.readValue("the body", value)
	if err != nil {
		return "", nil, err
	}
	return decisionBodyOf(text, format)
}

// decisionBodyOf converts a body as it was written. An empty body says
// nothing more than the sentence and goes out empty.
func decisionBodyOf(text, format string) (string, []string, error) {
	if format == "html" {
		return text, markupNotice(text), nil
	}
	result, err := markdownToHTML(text, "the body", "--body-format")
	if err != nil {
		return "", nil, err
	}
	if result.HTML == "" && strings.TrimSpace(text) != "" {
		return "", nil, &Error{Code: CodeUsage, Message: "the body has nothing Minka keeps",
			Hint: "Write the body in words, or leave --body out."}
	}
	return result.HTML, result.Warnings, nil
}

// decisionRefusal classifies a failed request about a decision and names the
// way on.
func decisionRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodeDecisionStatusUnknown:
		e.Hint = "The tool has no such status for you. Name one by its ID or by its role: " + strings.Join(decisionRoles, ", ") + "; a client is never offered draft. The statuses: minka decisions statuses <toolId>"
	case apiCodeDecisionEarlierTwice:
		e.Hint = "A decision supersedes or revokes one earlier decision, not both: give one of --supersedes and --revokes."
	case apiCodeDecisionEarlierUnknown:
		e.Hint = "The earlier decision is one of the same tool you see, out of the archive and the trash, and none that comes after this one. The decisions of the tool: minka decisions list <toolId>"
	case apiCodeDecisionEventUnknown:
		e.Hint = "The event a decision was taken in is one of its project's calendars that you see. The events: minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD"
	case apiCodeDecisionAppliesToUnknown:
		e.Hint = "A decision applies to an event of its project or a card you see, out of the trash."
	case apiCodeDecisionLabelUnknown:
		e.Hint = "A label is named by its ID or by the name of a label of the project. The labels: minka labels list --project <projectId>"
	case apiCodeDecisionArchived:
		e.Hint = "The decision lies in the archive of its tool. It can be read; it comes back out of the archive in Minka on the web only."
	case apiCodeDecisionTitleMissing:
		e.Hint = "A decision needs the sentence that holds: it can be replaced, never removed."
	case apiCodePersonNotAProjectMember:
		e.Hint = "Somebody named with --decided-by or --notify is not a member of the project. The members: minka people list <projectId>. Anybody else who decided is named by a name given freely, which is no ID, and is told nothing."
	case apiCodeNotifyIDsMissing, apiCodeNotifyIDsWithoutChosenAudience:
		e.Hint, _ = audienceHint(e.APICode)
	case apiCodeDecisionAudienceUnknown:
		e.Hint = "A decision reaches everybody, chosen with one --notify per person, or nobody."
	case apiCodePersonCannotSeeIt:
		e.Hint = "Nothing was written, and the same request will be refused again: somebody named with --notify is a client of the project, or an agent working for one, and the decision is not shown to clients. Name somebody else, or write a new decision down with --visible-to-clients."
	case apiCodeEntryAddressMalformed:
		e.Hint = `An entry is the address of a page of this Minka, a "#" card name such as #MNK-1A, or, in the history, a web address with --description.`
	case apiCodeEntryTargetUnknown:
		e.Hint = "The address names nothing you see, or nothing this part takes: a consequence is a to-do or a card."
	case apiCodeLinkDescriptionMissing:
		e.Hint = "A web address outside Minka goes into the history with --description, a few words that say what it is."
	case apiCodeEntryPlaceUnknown:
		e.Hint = "The entry named with --after or --before is the entry itself or none of this decision. The entries: minka decisions show <decisionId> --json"
	default:
		if e.Code == CodeNotFound {
			e.Hint = decisionNotFoundHint
		}
	}
	return e
}

// createDecisionRefusal is the refusal of a new decision, whose 404 is the
// tool's.
func createDecisionRefusal(s *session, err error) *Error {
	e := decisionRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = decisionsToolNotFoundHint
	}
	return e
}

func decisionsToolRefusal(s *session, err error) *Error {
	e := decisionRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = decisionsToolNotFoundHint
	}
	return e
}

func decisionSummary(decision api.Decision) string {
	summary := "Decision " + decisionTitle(decision.ShortID, decision.Title) + ", " + decision.Status.Name
	if decision.Archived {
		summary += ", in the archive of " + decision.Tool.Title
	}
	return summary
}

func decisionSuccess(decision api.Result[api.Decision], summary string, notices []string, text func(io.Writer)) success {
	d := decision.Value
	crumbs := []Breadcrumb{
		{Action: "comment", Cmd: "minka decisions comments create " + d.ID + " <text|->", Description: "Comment on the decision"},
		{Action: "edit", Cmd: "minka decisions edit " + d.ID + " --reason <why>", Description: "Change the decision"},
		{Action: "supersede", Cmd: "minka decisions supersede " + d.ID + " --title <sentence>", Description: "Write down a decision that takes its place"},
		{Action: "list", Cmd: "minka decisions list " + d.ToolID + " --holds", Description: "List the decisions in force beside it"},
	}
	if d.Archived {
		crumbs = crumbs[3:]
	}
	return success{data: decision.Raw, summary: summary, notices: notices, breadcrumbs: crumbs, text: text}
}

func renderDecisionLines(w io.Writer, decisions []api.DecisionLine) {
	if len(decisions) == 0 {
		fmt.Fprintln(w, "No decisions.")
		return
	}
	for _, decision := range decisions {
		details := []string{decision.Status.Name}
		if decision.DecidedOn != nil {
			details = append(details, "decided "+*decision.DecidedOn)
		}
		if decision.ValidUntil != nil {
			details = append(details, "until "+*decision.ValidUntil)
		}
		if setBy := statusSetByAnAgentOrMinka(decision.StatusSetBy); setBy != "" {
			details = append(details, setBy)
		}
		if tags := labelTags(decision.Labels); tags != "" {
			details = append(details, tags)
		}
		fmt.Fprintf(w, "- %s%s  %s (%s)\n", shortIDBefore(decision.ShortID), decision.ID, decision.Title, strings.Join(details, ", "))
		if decision.Reason != nil && *decision.Reason != "" {
			fmt.Fprintf(w, "  %s\n", oneLine(*decision.Reason))
		}
	}
}

func renderDecision(w io.Writer, d api.Decision) {
	// The heading stays the bare sentence, so that somebody who takes it as
	// the new title does not write the short identifier into it.
	fmt.Fprintf(w, "# %s\n", d.Title)
	if d.ShortID != nil {
		fmt.Fprintf(w, "Short ID: %s\n", *d.ShortID)
	}
	fmt.Fprintf(w, "Decision: %s\nTool: %s (%s)\nProject: %s\n", d.ID, d.Tool.Title, d.ToolID, d.ProjectID)
	status := d.Status.Name
	if d.Status.Role != nil && *d.Status.Role != "" {
		status += " (" + *d.Status.Role + ")"
	}
	fmt.Fprintf(w, "Status: %s\n", status)
	if line := statusSetByLine(d.StatusSetBy); line != "" {
		fmt.Fprintln(w, line)
	}
	if d.Archived {
		fmt.Fprintln(w, "Archived: yes, it is read and not changed")
	}
	if d.Reason != nil {
		fmt.Fprintf(w, "Reason: %s\n", *d.Reason)
	}
	renderDecisionCircumstances(w, d.DecidedOn, d.DecidedBy, d.DecidedIn, d.Votes)
	if d.AppliesTo != nil {
		trash := ""
		if d.AppliesTo.InTrash {
			trash = ", in the trash"
		}
		fmt.Fprintf(w, "Applies to: %s %s (%s%s)\n", d.AppliesTo.Kind, d.AppliesTo.Title, d.AppliesTo.ID, trash)
	}
	if d.ValidUntil != nil {
		fmt.Fprintf(w, "Valid until: %s\n", *d.ValidUntil)
	}
	if d.ReviewOn != nil {
		fmt.Fprintf(w, "Review on: %s\n", *d.ReviewOn)
	}
	for _, link := range []struct {
		label string
		link  *api.DecisionLink
	}{{"Supersedes", d.Supersedes}, {"Revokes", d.Revokes}} {
		if link.link != nil {
			fmt.Fprintf(w, "%s: %s (%s, %s)\n", link.label, decisionTitle(link.link.ShortID, link.link.Title), link.link.ID, link.link.Status.Name)
		}
	}
	for _, later := range []struct {
		label string
		links []api.DecisionLink
	}{{"Superseded by", d.SupersededBy}, {"Revoked by", d.RevokedBy}} {
		for _, link := range later.links {
			fmt.Fprintf(w, "%s: %s (%s, %s)\n", later.label, decisionTitle(link.ShortID, link.Title), link.ID, link.Status.Name)
		}
	}
	labelLine(w, d.Labels)
	fmt.Fprintf(w, "Created: %s by %s\n", formatTime(d.CreatedAt), d.Creator.Name)
	if d.EditedAt != nil {
		fmt.Fprintf(w, "Edited: %s\n", formatTime(*d.EditedAt))
	}
	fmt.Fprintf(w, "URL: %s\n", decisionAddress(d))

	renderDecisionEntries(w, d)
	if body := richtext.ToMarkdown(firstNonEmpty(d.BodySource, d.Body)); body != "" {
		fmt.Fprintf(w, "\n## Body\n\n%s\n", body)
	}
	renderEntryFiles(w, d.Attachments)
	renderEntryComments(w, d.Comments)
}

// renderDecisionCircumstances names when, by whom, where and by how many
// votes a decision was taken, each where it is known.
func renderDecisionCircumstances(w io.Writer, decidedOn *string, decidedBy []api.Decider, decidedIn *api.DecisionEvent, votes *string) {
	if decidedOn != nil {
		fmt.Fprintf(w, "Decided on: %s\n", *decidedOn)
	}
	if len(decidedBy) > 0 {
		var names []string
		for _, decider := range decidedBy {
			switch {
			case decider.Person != nil:
				names = append(names, decider.Person.Name+" ("+decider.Person.ID+")")
			case decider.Name != nil:
				names = append(names, *decider.Name)
			}
		}
		fmt.Fprintf(w, "Decided by: %s\n", strings.Join(names, ", "))
	}
	if decidedIn != nil {
		fmt.Fprintf(w, "Decided in: %s on %s (%s)\n", decidedIn.Title, decidedIn.StartsOn, decidedIn.ID)
	}
	if votes != nil {
		fmt.Fprintf(w, "Votes: %s\n", *votes)
	}
}

func renderDecisionEntries(w io.Writer, d api.Decision) {
	for _, part := range []struct {
		heading string
		entries []api.DecisionEntry
	}{{"History", d.History}, {"Consequences", d.Consequences}} {
		if len(part.entries) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## %s (%d)\n\n", part.heading, len(part.entries))
		for _, entry := range part.entries {
			mark := ""
			if entry.Done != nil {
				mark = map[bool]string{true: "[x] ", false: "[ ] "}[*entry.Done]
			}
			fmt.Fprintf(w, "- %s%s  %s %s\n  %s\n", mark, entry.ID, entry.Kind, decisionTitle(entry.ShortID, entry.Title), entry.AppURL)
		}
	}
}

// decisionTitle puts the short identifier of a decision, or of a card an
// entry names, in front of its title where it has one.
func decisionTitle(shortID *string, title string) string {
	if shortID == nil || *shortID == "" {
		return title
	}
	return *shortID + " " + title
}

// decisionName is how a confirmation names a decision: its short identifier
// with the ID beside it, or the ID alone.
func decisionName(d api.Decision) string {
	if d.ShortID == nil || *d.ShortID == "" {
		return d.ID
	}
	return *d.ShortID + " (" + d.ID + ")"
}

// decisionAddress is the address a person copies to name a decision
// elsewhere: the short one, which a renamed prefix keeps leading to, and the
// page for a decision its tool has not numbered.
func decisionAddress(d api.Decision) string {
	if d.ShortURL != nil && *d.ShortURL != "" {
		return *d.ShortURL
	}
	return d.AppURL
}

// statusSetByAnAgentOrMinka is what a line of the list says about who set the
// status: only an agent or Minka is worth the words there.
func statusSetByAnAgentOrMinka(setBy *api.DecisionStatusSetBy) string {
	switch {
	case setBy == nil:
		return ""
	case setBy.Minka:
		return "set by Minka"
	case setBy.Agent:
		return "set by an agent"
	}
	return ""
}

// statusSetByLine names who set the status and when: a person, an agent
// marked as such, or Minka with the person its line names.
func statusSetByLine(setBy *api.DecisionStatusSetBy) string {
	if setBy == nil {
		return ""
	}
	who := "somebody no longer known"
	if setBy.Person != nil {
		who = setBy.Person.Name + " (" + setBy.Person.ID + ")"
	}
	switch {
	case setBy.Minka:
		who = "Minka, on behalf of " + who
	case setBy.Agent:
		who += ", an agent"
	}
	return fmt.Sprintf("Status set: %s by %s", formatTime(setBy.At), who)
}
