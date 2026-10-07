package cli

import (
	"encoding/json"
	"fmt"
	"io"
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

const stepsOfAnOlderMinkaHint = "The Minka of this host is older than the people and the due day of a step and takes only a tick: minka steps complete or reopen."

const stepsWithoutRenamingHint = "The Minka of this host does not rename a step yet. Until it does, add the step anew with minka steps create and delete the old one."

const stepsWithoutMovingHint = "The step was on the card a moment ago: the Minka of this host may not move a step yet, which it answers like a step that is not there."

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

func (a *app) newStepsCmd() *cobra.Command {
	return group("steps", "List, add, change, move, tick off and remove the steps of a card",
		a.newStepsListCmd(),
		a.newStepsCreateCmd(),
		a.newStepsEditCmd(),
		a.newStepsMoveCmd(),
		a.newStepsCompleteCmd(),
		a.newStepsReopenCmd(),
		a.newStepsDeleteCmd(),
	)
}

func (a *app) newStepsListCmd() *cobra.Command {
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
					{Action: "create", Cmd: "minka steps create " + card.Value.ID + " <text>", Description: "Add a step"},
					{Action: "complete", Cmd: "minka steps complete " + card.Value.ID + " <step>", Description: "Tick a step off"},
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

func (a *app) newStepsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Add a step at the end of the list of a card",
		Long: `Adds a step at the end of the list of a card. A step is one line of at most
500 characters. Give - as the text to read it from stdin.

A step comes without people and without a due day: Minka takes neither when it
adds one. Give them afterwards with minka steps edit.`,
		Example: `  minka steps create 0199... "Call the caterer"`,
		Args:    requireArgs("cardId", "text"),
		RunE: func(_ *cobra.Command, args []string) error {
			text, err := a.readValue("the step", args[1])
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.AddStep(a.ctx, args[0], text)
			if err != nil {
				return s.fail(err)
			}

			summary := fmt.Sprintf("Added a step to card %s", card.Value.Title)
			breadcrumbs := stepBreadcrumbs(card.Value.ID)
			if n := len(card.Value.Steps); n > 0 {
				breadcrumbs = append(breadcrumbs, Breadcrumb{Action: "edit", Cmd: fmt.Sprintf("minka steps edit %s %d --assignee <person> --due <YYYY-MM-DD>", card.Value.ID, n), Description: "Give the step people and a due day"})
			}
			return a.ok(success{
				data:        card.Raw,
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					if n := len(card.Value.Steps); n > 0 {
						fmt.Fprintln(w, stepLine(n-1, card.Value.Steps[n-1]))
					}
				},
			})
		},
	}
}

func (a *app) newStepsEditCmd() *cobra.Command {
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

` + stepHelp,
		Example: `  minka steps edit 0199... 2 --assignee me --due 2026-10-09
  minka steps edit 0199... 2 --title "Call the caterer about the tent"
  minka steps edit 0199... "Call the caterer" --assignee me --assignee anke@example.com
  minka steps edit 0199... 2 --no-assignees --due ""`,
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
				// A Minka that does not rename a step yet reads a text alone
				// as no change at all.
				if input.Title != nil && e.APICode == apiCodeNothingToChange {
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

func (a *app) newStepsMoveCmd() *cobra.Command {
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
		Example: `  minka steps move 0199... 3 --top
  minka steps move 0199... "Call the caterer" --after 1
  minka steps move 0199... 1 --bottom`,
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
					e.Hint = "The step that was last on the card when it was read is gone from it since. The steps as they stand now: minka steps list " + card.Value.ID
				case e.APICode == apiCodeStepPlaceUnknown:
					e.Hint = "The step named with --after or --before is the step itself or none of this card. The steps in their order: minka steps list " + card.Value.ID
				case e.Code == CodeNotFound && wasOnCard:
					// A Minka without the address answers like one without the
					// step, but this step was just read on the card.
					e.Hint = stepsWithoutMovingHint
				}
				return e
			}

			summary := fmt.Sprintf("Moved step %q on card %s", step.Title, card.Value.Title)
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

func (a *app) newStepsCompleteCmd() *cobra.Command {
	return a.newStepsSetCompletedCmd("complete", "Tick a step off", true)
}

func (a *app) newStepsReopenCmd() *cobra.Command {
	return a.newStepsSetCompletedCmd("reopen", "Open a ticked off step again", false)
}

func (a *app) newStepsSetCompletedCmd(name, short string, completed bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <cardId> <step>",
		Short: short,
		Long: short + `. A step that stands that way already stays as it is, and the
command succeeds.

` + stepHelp,
		Example: fmt.Sprintf("  minka steps %s 0199... 2", name),
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
				return s.fail(err)
			}

			verb, state := "Completed", "completed"
			if !completed {
				verb, state = "Reopened", "open"
			}
			summary := fmt.Sprintf("%s step %q on card %s", verb, step.Title, card.Value.Title)
			if step.Title != "" && step.Completed == completed {
				summary = fmt.Sprintf("Step %q on card %s was %s already", step.Title, card.Value.Title, state)
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

func (a *app) newStepsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <cardId> <step>",
		Short: "Remove a step from a card",
		Long: `Removes a step from a card, without asking. Removing a step that is not on the
card succeeds as well.

` + stepHelp,
		Example: `  minka steps delete 0199... 3`,
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

			summary := fmt.Sprintf("Removed step %q from card %s", step.Title, card.Value.Title)
			if step.Title == "" {
				summary = fmt.Sprintf("Step %s was not on card %s", step.ID, card.Value.Title)
			}
			return a.ok(success{
				data:        map[string]any{"cardId": card.Value.ID, "stepId": step.ID, "wasOnCard": step.Title != ""},
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

	var matches []int
	var positions []string
	for i, title := range lines.titles {
		if strings.EqualFold(title, wanted) {
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
	mark := " "
	if step.Completed {
		mark = "x"
	}
	return fmt.Sprintf("%d. [%s] %s%s", index+1, mark, step.Title, stepDetails(step))
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
		{Action: "list", Cmd: "minka steps list " + cardID, Description: "List the steps of the card"},
		{Action: "show", Cmd: "minka cards show " + cardID, Description: "Show the card"},
	}
}
