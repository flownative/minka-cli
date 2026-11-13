package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const stepHelp = `A step is given by its ID, by its position in the list starting at 1, or by
its text. A number always means a position. A text has to match exactly one
step, ignoring case.`

const stepPeopleHint = "Only a member of the project who sees the board can be on a step. The members are listed by: minka people list <projectId>"

// apiCodeStepCompletedMissing is how a Minka that takes only a tick refuses a
// change of a step that does not tick it.
const apiCodeStepCompletedMissing = "stepCompletedMissing"

// apiCodeStepPlaceUnknown is how Minka refuses a move next to a step that is
// the step itself or no other step of the card.
const apiCodeStepPlaceUnknown = "stepPlaceUnknown"

const stepsOfAnOlderMinkaHint = "The Minka of this host is older than the people and the due day of a step and takes only a tick: minka cards steps complete or reopen."

const stepsWithoutRenamingHint = "The Minka of this host does not rename a step yet. Until it does, add the step anew with minka cards steps create and delete the old one."

const stepsWithoutMovingHint = "The step was on the card a moment ago: the Minka of this host may not move a step yet, which it answers like a step that is not there."

// The answers of Minka about a card as a step.
const (
	apiCodeStepCardUnknown    = "stepCardUnknown"
	apiCodeStepCardIsThisCard = "stepCardIsThisCard"
	apiCodeStepTitleAndCard   = "stepTitleAndCard"
	apiCodeStepIsACard        = "stepIsACard"
	apiCodeStepMissing        = "stepMissing"
)

const stepCardUnknownHint = "No card you can see goes by that name: it does not exist, is in the trash or in a project you are not a member of. Name the card by its ID, its short identifier such as MNK-1A or #MNK-1A, or its address on this host."

const cardStepsUnknownHint = "Its Minka is older than this minka expects and does not know a card as the step of another yet. Until it is updated, name the card in the text of a step or in a comment."

// phpBlanks are the characters Minka trims off both ends of a step.
const phpBlanks = " \t\n\r\x00\x0B"

// stepTextAsKept is the text of a step as Minka keeps what was sent: invalid
// UTF-8 replaced as encoding/json replaces it on the way out, then trimmed.
func stepTextAsKept(text string) string {
	sent, _ := json.Marshal(text)
	var received string
	_ = json.Unmarshal(sent, &received)
	return strings.Trim(received, phpBlanks)
}

func (a *app) newCardsStepsCmd() *cobra.Command {
	return group("steps", "List, add, change, move, tick off and remove the steps of a card",
		a.newCardsStepsListCmd(),
		a.newCardsStepsCreateCmd(),
		a.newCardsStepsEditCmd(),
		a.newCardsStepsMoveCmd(),
		a.newCardsStepsCompleteCmd(),
		a.newCardsStepsReopenCmd(),
		a.newCardsStepsDeleteCmd(),
		a.newCardsStepsMakeCardCmd(),
	)
}

func (a *app) newCardsStepsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <cardId>",
		Short: "List the steps of a card",
		Args:  requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			// The data is the list as the API sent it inside the card.
			var raw struct {
				Steps json.RawMessage `json:"steps"`
			}
			if err := json.Unmarshal(card.Raw, &raw); err != nil || raw.Steps == nil {
				raw.Steps = json.RawMessage("[]")
			}

			return a.ok(success{
				data:    raw.Steps,
				summary: fmt.Sprintf("%d of %s completed on card %s", completedSteps(card.Value.Steps), plural(len(card.Value.Steps), "step", "steps"), card.Value.Title),
				breadcrumbs: []Breadcrumb{
					{Action: "create", Cmd: "minka cards steps create " + card.Value.ID + " <text>", Description: "Add a step"},
					{Action: "complete", Cmd: "minka cards steps complete " + card.Value.ID + " <step>", Description: "Tick a step off"},
				},
				text: func(w io.Writer) {
					if len(card.Value.Steps) == 0 {
						fmt.Fprintln(w, "No steps.")
						return
					}
					for i, step := range card.Value.Steps {
						fmt.Fprintf(w, "%s  (%s)\n", stepLine(i, step), step.ID)
					}
				},
			})
		},
	}
}

func (a *app) newCardsStepsCreateCmd() *cobra.Command {
	var child, todo string
	cmd := &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Add a step at the end of the list of a card, in words, as another card or as a to-do",
		Long: `Adds a step at the end of the list of a card. A step is one line of at most
500 characters. Give - as the text to read it from stdin.

A step in words comes without people and without a due day: Minka takes
neither when it adds one. Give them afterwards with minka cards steps edit.

--card makes another card the step instead of a text: the step then reads as
that card stands, with its title, its people and its column, and it is done
when the card is in Done. Name the card by its ID, its short identifier
(MNK-1A, also #MNK-1A) or its address on this host. A card can be a step of
any card you can see, on any board, and of several cards; it is a step of the
same card at most once, and naming it again succeeds without a second step.
A card cannot be a step of itself: exit 9 (validation) with
stepCardIsThisCard. A card you cannot see is exit 9 with stepCardUnknown. A
Minka that knows no card steps yet is exit 2 (not_found).

--todo makes a to-do the step instead of a text: the step then reads as that
to-do stands, with its title, and it is done when the to-do is. Name the to-do
by its ID; a to-do has no short identifier. It may be a to-do of any project
you can see it in, in a list in the archive as well, and a step of several
cards; it is a step of the same card at most once, and naming it again
succeeds without a second step. A to-do you cannot see or one in the trash is
exit 9 with stepTodoUnknown. A to-do step is ticked on its to-do, with minka
todos complete, never on the step. A Minka that knows no to-do steps yet is
exit 2 (not_found).

A step is a text, a card or a to-do: two of them at once are a usage error,
and nothing is sent.

data is the step, as an entry of the steps of minka cards show, and the step
that stands already where the card or the to-do was a step of it before; data.id is the
ID of the step for the commands that follow, such as minka cards steps move.
A Minka that answers with the whole card is read the same way.`,
		Example: `  minka cards steps create 0199... "Call the caterer"
  minka cards steps create MNK-12 --card MNK-15
  minka cards steps create MNK-12 --todo 0199...`,
		Args: func(cmd *cobra.Command, args []string) error {
			asCard, asTodo := cmd.Flags().Changed("card"), cmd.Flags().Changed("todo")
			switch {
			case !asCard && !asTodo:
				return requireArgs("cardId", "text")(cmd, args)
			case asCard && asTodo:
				// A Minka that does not refuse the two together takes the card
				// and passes over the to-do without a word.
				return usageErrorf(cmd, "give either --card or --todo, not both")
			case len(args) > 1 && asCard:
				return usageErrorf(cmd, "give either a text or --card, not both")
			case len(args) > 1:
				return usageErrorf(cmd, "give either a text or --todo, not both")
			case asCard && strings.TrimSpace(child) == "":
				return usageErrorf(cmd, "--card names no card")
			}
			if asTodo {
				if err := requireID(cmd, "--todo", &todo); err != nil {
					return err
				}
			}
			return requireArgs("cardId")(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("card") {
				return a.addCardStep(args[0], child)
			}
			if cmd.Flags().Changed("todo") {
				return a.addTodoStep(args[0], todo)
			}
			text, err := a.readValue("the step", args[1])
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			given := args[0]
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			step, err := s.client.AddStep(a.ctx, args[0], text)
			if err != nil {
				e := s.fail(err)
				unreadStepHint(e, args[0])
				return e
			}

			summary := fmt.Sprintf("Added %s to card %s", stepName(nil, step.Value), cardAsNamed(given, args[0]))
			breadcrumbs := append(stepBreadcrumbs(args[0]),
				Breadcrumb{Action: "edit", Cmd: fmt.Sprintf("minka cards steps edit %s %s --assignee <person> --due <YYYY-MM-DD>", args[0], step.Value.ID), Description: "Give the step people and a due day"})
			return a.ok(success{
				data:        step.Raw,
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, stepEntry(step.Value))
				},
			})
		},
	}
	cmd.Flags().StringVar(&child, "card", "", "Card to add as the step instead of a text: its ID, short identifier or address")
	cmd.Flags().StringVar(&todo, "todo", "", "To-do to add as the step instead of a text: its ID")
	return cmd
}

func (a *app) newCardsStepsEditCmd() *cobra.Command {
	var title, due string
	var assignees []string
	var noAssignees bool

	cmd := &cobra.Command{
		Use:   "edit <cardId> <step>",
		Short: "Change the text of a step, who is on it and the day it is due",
		Long: `Changes the text of a step, the people on it, the day it is due or several of
them, in one request. What is not given stays as it is. --title is one line of
at most 500 characters; --title "" does not empty it: exit 9 (validation) with
stepMissing. --due is a day as YYYY-MM-DD; --due "" takes it off.

--assignee gives the WHOLE set of people: whoever is on the step and not named
again is taken off. --no-assignees takes everybody off. ` + personHelp + ` Only a
member of the project who sees the board can be put on a step: anybody else
refuses the whole write with 9 (validation) and meta.apiCode
personNotAProjectMember, and nothing is written, the text neither.

A card in the archive of its board takes no change: exit 9 with cardArchived.
A step that is a card or a to-do carries nothing of its own: exit 9 with
stepIsACard or stepIsATodo, and the hint names the command that changes the
card or the to-do.

` + stepHelp,
		Example: `  minka cards steps edit 0199... 2 --assignee me --due 2026-10-09
  minka cards steps edit 0199... 2 --title "Call the caterer about the tent"
  minka cards steps edit 0199... "Call the caterer" --assignee me --assignee anke@example.com
  minka cards steps edit 0199... 2 --no-assignees --due ""`,
		Args: requireArgs("cardId", "step"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noAssignees && len(assignees) > 0 {
				return usageErrorf(cmd, "--assignee and --no-assignees contradict each other")
			}
			var input api.UpdateStepInput
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("due") {
				input.DueOn = &due
			}
			if noAssignees {
				input.AssigneeIDs = &[]string{}
			}
			if input == (api.UpdateStepInput{}) && len(assignees) == 0 {
				return usageErrorf(cmd, "nothing to change, give --title, --assignee, --no-assignees or --due")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			step, err := resolveStep(card.Value.Steps, args[1])
			if err != nil {
				return err
			}
			if len(assignees) > 0 {
				people := newProjectPeople(a, s, func() (string, error) { return card.Value.ProjectID, nil })
				ids, err := people.resolve(assignees)
				if err != nil {
					return err
				}
				input.AssigneeIDs = &ids
			}

			changed, err := s.client.UpdateStep(a.ctx, card.Value.ID, step.ID, input)
			if err != nil {
				e := stepRefusal(s, err)
				switch {
				case e.APICode == apiCodeStepIsACard:
					e.Hint = stepIsACardHint(card.Value.ID, card.Value.Steps, step)
				case e.APICode == apiCodeStepIsATodo:
					e.Hint = stepIsATodoHint(card.Value.ID, card.Value.Steps, step, "edit")
				case input.Title != nil && e.APICode == apiCodeNothingToChange:
					// A Minka that does not rename a step yet reads a text
					// alone as no change at all.
					e.Hint = stepsWithoutRenamingHint
				}
				return e
			}

			summary := fmt.Sprintf("Changed step %q on card %s", step.Title, card.Value.Title)
			if input.Title != nil {
				at := slices.IndexFunc(changed.Value.Steps, func(now api.Step) bool { return now.ID == step.ID })
				if at >= 0 {
					now := changed.Value.Steps[at]
					// Such a Minka writes the rest beside the text and passes
					// over the text, with a 200 that would otherwise pass for
					// success.
					if now.Title != stepTextAsKept(*input.Title) {
						return textPassedOver(now, input, changed.Raw)
					}
					if now.Title != step.Title {
						summary = fmt.Sprintf("Changed step %q to %q on card %s", step.Title, now.Title, card.Value.Title)
					}
				}
			}
			return a.ok(success{
				data:        changed.Raw,
				summary:     summary,
				breadcrumbs: stepBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					for i, now := range changed.Value.Steps {
						if now.ID == step.ID {
							fmt.Fprintln(w, stepLine(i, now))
						}
					}
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New text of the step")
	flags.StringVar(&due, "due", "", "Day the step is due as YYYY-MM-DD, empty to take it off")
	flags.StringArrayVar(&assignees, "assignee", nil, "Person to be on the step, the whole set (repeatable)")
	flags.BoolVar(&noAssignees, "no-assignees", false, "Take everybody off the step")

	return cmd
}

func (a *app) newCardsStepsMoveCmd() *cobra.Command {
	var after, before string
	var top, bottom bool

	cmd := &cobra.Command{
		Use:   "move <cardId> <step>",
		Short: "Move a step to another place on its card",
		Long: `Puts a step elsewhere among the steps of its card: --top puts it first,
--bottom last, --after below the step named, --before above it. Exactly one of
the four is required. A step never leaves its card. Moving a step to where it
stands already succeeds.

The step named with --after or --before has to be another step of the same
card; the step itself is refused with exit 9 (validation) and meta.apiCode
stepPlaceUnknown, and nothing moves. A card in the archive of its board takes
no move: exit 9 with cardArchived.

` + stepHelp,
		Example: `  minka cards steps move 0199... 3 --top
  minka cards steps move 0199... "Call the caterer" --after 1
  minka cards steps move 0199... 1 --bottom`,
		Args: requireArgs("cardId", "step"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, bottom, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the step goes: give exactly one of --top, --bottom, --after and --before")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			step, err := resolveStep(card.Value.Steps, args[1])
			if err != nil {
				return err
			}
			var input api.MoveStepInput
			for _, neighbour := range []struct {
				value string
				id    *string
			}{{after, &input.PreviousStepID}, {before, &input.NextStepID}} {
				if neighbour.value == "" {
					continue
				}
				found, err := resolveStep(card.Value.Steps, neighbour.value)
				if err != nil {
					return err
				}
				*neighbour.id = found.ID
			}
			// Minka has no word for the end: a step goes there behind the
			// last of the others, or to the top where there is no other.
			if bottom {
				for _, other := range card.Value.Steps {
					if other.ID != step.ID {
						input.PreviousStepID = other.ID
					}
				}
			}

			moved, err := s.client.MoveStep(a.ctx, card.Value.ID, step.ID, input)
			if err != nil {
				e := stepRefusal(s, err)
				wasOnCard := slices.ContainsFunc(card.Value.Steps, func(read api.Step) bool { return read.ID == step.ID })
				switch {
				case e.APICode == apiCodeStepPlaceUnknown && bottom:
					// The last step as read can leave the card before the move.
					e.Hint = "The step that was last on the card when it was read is gone from it since. The steps as they stand now: minka cards steps list " + card.Value.ID
				case e.APICode == apiCodeStepPlaceUnknown:
					e.Hint = "The step named with --after or --before is the step itself or none of this card. The steps in their order: minka cards steps list " + card.Value.ID
				case e.Code == CodeNotFound && wasOnCard:
					// A Minka without the address answers like one without the
					// step, but this step was just read on the card.
					e.Hint = stepsWithoutMovingHint
				}
				return e
			}

			summary := fmt.Sprintf("Moved %s on card %s", stepName(card.Value.Steps, step), card.Value.Title)
			return a.ok(success{
				data:        moved.Raw,
				summary:     summary,
				breadcrumbs: stepBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					for i, now := range moved.Value.Steps {
						if now.ID == step.ID {
							fmt.Fprintln(w, stepLine(i, now))
						}
					}
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&top, "top", false, "Put the step first")
	flags.BoolVar(&bottom, "bottom", false, "Put the step last")
	flags.StringVar(&after, "after", "", "Put the step below this step")
	flags.StringVar(&before, "before", "", "Put the step above this step")

	return cmd
}

// textPassedOver is the failure of an edit whose answer does not carry the
// text sent: it says of the rest only what was sent along.
func textPassedOver(now api.Step, input api.UpdateStepInput, card json.RawMessage) *Error {
	message := fmt.Sprintf("this Minka does not rename a step yet: the step still reads %q", now.Title)
	if input.DueOn != nil || input.AssigneeIDs != nil {
		message += ", and the rest was written"
	}
	return &Error{Code: CodeAPI, Message: message, Hint: stepsWithoutRenamingHint, Data: card}
}

// stepRefusal classifies a failed change of a step: the people and the
// archive of the card name what can be done about them.
func stepRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodePersonNotAProjectMember:
		e.Hint = stepPeopleHint
	case apiCodeCardArchived:
		e.Hint = "The card lies in the archive of its board. Bring it back with: minka cards unarchive <cardId>"
	case apiCodeStepCompletedMissing:
		// An edit never sends the tick, so only a Minka that knows nothing
		// but the tick asks for it.
		e.Hint = stepsOfAnOlderMinkaHint
	}
	return e
}

func (a *app) newCardsStepsCompleteCmd() *cobra.Command {
	return a.newCardsStepsSetCompletedCmd("complete", "Tick a step off", true)
}

func (a *app) newCardsStepsReopenCmd() *cobra.Command {
	return a.newCardsStepsSetCompletedCmd("reopen", "Open a ticked off step again", false)
}

func (a *app) newCardsStepsSetCompletedCmd(name, short string, completed bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <cardId> <step>",
		Short: short,
		Long: short + `. A step that stands that way already stays as it is, and the
command succeeds.

A step that is a card or a to-do has no tick of its own: exit 9 (validation)
with stepIsACard or stepIsATodo. A card step is done when its card is in Done,
a to-do step when its to-do is ticked off with minka todos complete.

` + stepHelp,
		Example: fmt.Sprintf("  minka cards steps %s 0199... 2", name),
		Args:    requireArgs("cardId", "step"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			step, err := resolveStep(card.Value.Steps, args[1])
			if err != nil {
				return err
			}

			changed, err := s.client.SetStepCompleted(a.ctx, card.Value.ID, step.ID, completed)
			if err != nil {
				e := s.fail(err)
				switch e.APICode {
				case apiCodeStepIsACard:
					e.Hint = stepIsACardHint(card.Value.ID, card.Value.Steps, step)
				case apiCodeStepIsATodo:
					e.Hint = stepIsATodoHint(card.Value.ID, card.Value.Steps, step, name)
				}
				return e
			}

			verb, state := "Completed", "completed"
			if !completed {
				verb, state = "Reopened", "open"
			}
			summary := fmt.Sprintf("%s %s on card %s", verb, stepName(card.Value.Steps, step), card.Value.Title)
			if step.Title != "" && step.Completed == completed {
				summary = fmt.Sprintf("%s on card %s was %s already", capitalized(stepName(card.Value.Steps, step)), card.Value.Title, state)
			}
			return a.ok(success{
				data:        changed.Raw,
				summary:     summary,
				breadcrumbs: stepBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
				},
			})
		},
	}
}

func (a *app) newCardsStepsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <cardId> <step>",
		Short: "Remove a step from a card",
		Long: `Removes a step from a card, without asking. Removing a step that is not on the
card succeeds as well.

` + stepHelp,
		Example: `  minka cards steps delete 0199... 3`,
		Args:    requireArgs("cardId", "step"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			step, err := resolveStep(card.Value.Steps, args[1])
			if err != nil {
				return err
			}

			if err := s.client.DeleteStep(a.ctx, card.Value.ID, step.ID); err != nil {
				return s.fail(err)
			}

			// A card or a to-do step the reader cannot see has no title either,
			// so only the card as read says whether the step was on it.
			wasOnCard := slices.ContainsFunc(card.Value.Steps, func(read api.Step) bool { return read.ID == step.ID })
			summary := fmt.Sprintf("Removed %s from card %s", stepName(card.Value.Steps, step), card.Value.Title)
			switch {
			case !wasOnCard:
				summary = fmt.Sprintf("Step %s was not on card %s", step.ID, card.Value.Title)
			case step.Card != nil && step.Card.Seen():
				summary = fmt.Sprintf("Took card %s off the steps of card %s; the card itself stays", linkedCardName(*step.Card), card.Value.Title)
			case step.Card != nil:
				summary = fmt.Sprintf("Took a card you cannot open off the steps of card %s; the card itself stays", card.Value.Title)
			case step.Todo != nil && step.Todo.Seen():
				summary = fmt.Sprintf("Took to-do %q off the steps of card %s; the to-do itself stays", step.Todo.Title, card.Value.Title)
			case step.Todo != nil:
				summary = fmt.Sprintf("Took a to-do you cannot open off the steps of card %s; the to-do itself stays", card.Value.Title)
			}
			return a.ok(success{
				data:        map[string]any{"cardId": card.Value.ID, "stepId": step.ID, "wasOnCard": wasOnCard},
				summary:     summary,
				breadcrumbs: stepBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
				},
			})
		},
	}
}

// resolveStep finds a step of a card by ID, position or text. An ID that is on
// no step of the card is passed on, so the server gives its answer; the
// returned step then has no title.
func resolveStep(steps []api.Step, value string) (api.Step, error) {
	ids, titles := make([]string, len(steps)), make([]string, len(steps))
	for i, step := range steps {
		ids[i], titles[i] = step.ID, step.Title
	}
	at, id, err := findLine(namedLines{noun: "step", owner: "the card", ids: ids, titles: titles, described: describeSteps(steps)}, value)
	if err != nil || at < 0 {
		return api.Step{ID: id}, err
	}
	return steps[at], nil
}

// namedLines are the lines of a list somebody names by ID, by position or by
// text, such as the steps of a card: what a line is called, what the list
// hangs on, and the list as a hint describes it.
type namedLines struct {
	noun, owner string
	ids, titles []string
	described   string
	// caseMatters says that two titles may differ in case alone, as the names
	// of files do: the title as written is then taken before case is ignored.
	caseMatters bool
}

// findLine finds a line by ID, position or text and answers its index. A
// number always means a position, and a text has to match exactly one line,
// ignoring case. An ID that is on no line is answered with the index -1, so
// that the server gives its answer about it.
func findLine(lines namedLines, value string) (int, string, error) {
	wanted, _ := asID(strings.TrimSpace(value))
	if at := slices.Index(lines.ids, wanted); at >= 0 {
		return at, wanted, nil
	}

	if position, err := strconv.Atoi(wanted); err == nil {
		if position < 1 || position > len(lines.ids) {
			return -1, "", &Error{
				Code:    CodeUsage,
				Message: fmt.Sprintf("there is no %s %d, %s has %s", lines.noun, position, lines.owner, plural(len(lines.ids), lines.noun, lines.noun+"s")),
				Hint:    lines.described,
			}
		}
		return position - 1, lines.ids[position-1], nil
	}

	// A line without a title, such as a step whose card or to-do the reader
	// cannot see, is named by its position or its ID, never by a text.
	if lines.caseMatters && wanted != "" {
		if at := slices.Index(lines.titles, wanted); at >= 0 && slices.Index(lines.titles[at+1:], wanted) < 0 {
			return at, lines.ids[at], nil
		}
	}

	var matches []int
	var positions []string
	for i, title := range lines.titles {
		if title != "" && strings.EqualFold(title, wanted) {
			matches = append(matches, i)
			positions = append(positions, strconv.Itoa(i+1))
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], lines.ids[matches[0]], nil
	case len(matches) > 1:
		return -1, "", &Error{
			Code:    CodeAmbiguous,
			Message: fmt.Sprintf("%q matches %ss %s", value, lines.noun, strings.Join(positions, " and ")),
			Hint:    "Give the position or the ID. " + lines.described,
		}
	case uuidPattern.MatchString(wanted):
		return -1, wanted, nil
	default:
		return -1, "", &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("no %s of %s matches %q", lines.noun, lines.owner, value),
			Hint:    lines.described,
		}
	}
}

func describeSteps(steps []api.Step) string {
	if len(steps) == 0 {
		return "The card has no steps."
	}
	lines := make([]string, len(steps))
	for i, step := range steps {
		lines[i] = fmt.Sprintf("%s (%s)", stepLine(i, step), step.ID)
	}
	return "Steps: " + strings.Join(lines, "; ")
}

func stepLine(index int, step api.Step) string {
	return fmt.Sprintf("%d. %s", index+1, stepEntry(step))
}

// stepEntry shows a step without its position, for a step read by itself.
func stepEntry(step api.Step) string {
	if step.Card != nil {
		return cardStepEntry(*step.Card)
	}
	if step.Todo != nil {
		return todoStepEntry(*step.Todo)
	}
	mark := " "
	if step.Completed {
		mark = "x"
	}
	return fmt.Sprintf("[%s] %s%s", mark, step.Title, stepDetails(step))
}

// unreadStepHint says what to do where Minka's answer to a new step shows no
// step: after a 201 the step is written, and running the command again would
// add it twice.
func unreadStepHint(e *Error, cardID string) {
	if e.APICode != api.CodeUnexpectedResponse {
		return
	}
	e.Hint = "Do not repeat the command; read the card with: minka cards show " + cardID
	if e.HTTPStatus == http.StatusOK {
		e.Hint = "Read the steps with: minka cards steps list " + cardID
	}
}

// cardAsNamed names the card a command was given the way it was given: by
// its short identifier, or by its ID. It stands in for the title where the
// answer carries none.
func cardAsNamed(given, id string) string {
	if uuidPattern.MatchString(given) {
		return id
	}
	return "#" + strings.ToUpper(given)
}

// stepDetails says who is on a step and when it is due.
func stepDetails(step api.Step) string {
	var details []string
	if len(step.Assignees) > 0 {
		details = append(details, assigneeNames(step.Assignees))
	}
	if step.DueOn != nil {
		details = append(details, "due "+*step.DueOn)
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func completedSteps(steps []api.Step) int {
	completed := 0
	for _, step := range steps {
		if step.Completed {
			completed++
		}
	}
	return completed
}

func stepBreadcrumbs(cardID string) []Breadcrumb {
	return []Breadcrumb{
		{Action: "list", Cmd: "minka cards steps list " + cardID, Description: "List the steps of the card"},
		{Action: "show", Cmd: "minka cards show " + cardID, Description: "Show the card"},
	}
}
