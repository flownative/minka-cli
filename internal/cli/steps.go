package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const stepHelp = `A step is given by its ID, by its position in the list starting at 1, or by
its text. A number always means a position. A text has to match exactly one
step, ignoring case.`

func (a *app) newStepsCmd() *cobra.Command {
	return group("steps", "List, add, tick off and remove the steps of a card",
		a.newStepsListCmd(),
		a.newStepsCreateCmd(),
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
500 characters. Give - as the text to read it from stdin.`,
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
			return a.ok(success{
				data:        card.Raw,
				summary:     summary,
				breadcrumbs: stepBreadcrumbs(card.Value.ID),
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
	wanted := strings.TrimSpace(value)
	for _, step := range steps {
		if step.ID == wanted {
			return step, nil
		}
	}

	if position, err := strconv.Atoi(wanted); err == nil {
		if position < 1 || position > len(steps) {
			return api.Step{}, &Error{
				Code:    CodeUsage,
				Message: fmt.Sprintf("there is no step %d, the card has %s", position, plural(len(steps), "step", "steps")),
				Hint:    describeSteps(steps),
			}
		}
		return steps[position-1], nil
	}

	var matches []api.Step
	var positions []string
	for i, step := range steps {
		if strings.EqualFold(step.Title, wanted) {
			matches = append(matches, step)
			positions = append(positions, strconv.Itoa(i+1))
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return api.Step{}, &Error{
			Code:    CodeAmbiguous,
			Message: fmt.Sprintf("%q matches steps %s", value, strings.Join(positions, " and ")),
			Hint:    "Give the position or the ID. " + describeSteps(steps),
		}
	case uuidPattern.MatchString(wanted):
		return api.Step{ID: wanted}, nil
	default:
		return api.Step{}, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("no step of the card matches %q", value),
			Hint:    describeSteps(steps),
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
	return fmt.Sprintf("%d. [%s] %s", index+1, mark, step.Title)
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
