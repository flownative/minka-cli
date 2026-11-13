package cli

import (
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/flownative/minka-cli/internal/api"
)

const unseenTodo = "(a to-do you cannot open)"

// The answers of Minka about a to-do as a step.
const (
	apiCodeStepTodoUnknown  = "stepTodoUnknown"
	apiCodeStepTitleAndTodo = "stepTitleAndTodo"
	apiCodeStepIsATodo      = "stepIsATodo"
	apiCodeStepCardAndTodo  = "stepCardAndTodo"
)

const stepTodoUnknownHint = "No to-do you can see has that ID: it does not exist, is in the trash or in a project you are not a member of. A to-do is named by its ID alone, which minka todos list <listId> and minka todos mine hand out."

const todoStepsUnknownHint = "Its Minka is older than this minka expects and does not know a to-do as the step of a card yet. Until it is updated, name the to-do in the text of a step or in a comment."

// todoStepEntry shows a to-do step as its to-do stands: the tick is the
// to-do's, and the address is where it is ticked.
func todoStepEntry(todo api.LinkedTodo) string {
	if !todo.Seen() {
		return "↳ " + unseenTodo
	}
	mark := " "
	if todo.Completed {
		mark = "x"
	}
	return fmt.Sprintf("↳ to-do [%s] %s  %s", mark, todo.Title, todo.AppURL)
}

// stepIsATodoHint says where a to-do step is ticked and changed: on its
// to-do, with the command that does what the refused one was after. A to-do
// the reader cannot see cannot be changed, so only the link is left to remove.
func stepIsATodoHint(cardID string, steps []api.Step, step api.Step, command string) string {
	position := slices.IndexFunc(steps, func(read api.Step) bool { return read.ID == step.ID }) + 1
	if step.Todo == nil || !step.Todo.Seen() {
		return fmt.Sprintf("This step is a to-do you cannot open, so neither it nor the step can be changed. Only the link can be removed: minka cards steps delete %s %d", cardID, position)
	}
	return fmt.Sprintf("This step is a to-do: its title, people, days and tick are the to-do's own. Do it on the to-do itself: minka todos %s %s", command, step.Todo.ID)
}

// addTodoStep makes a to-do a step of a card.
func (a *app) addTodoStep(cardID, todoID string) error {
	s, err := a.session()
	if err != nil {
		return err
	}
	given := cardID
	if err := s.resolveCard(a.ctx, &cardID); err != nil {
		return err
	}
	step, err := s.client.AddTodoStep(a.ctx, cardID, todoID)
	if err != nil {
		e := s.fail(err)
		switch e.APICode {
		case apiCodeStepMissing:
			// A Minka without to-do steps passes over the to-do and misses the
			// text every step of its own needs.
			return &Error{Code: CodeNotFound, Message: s.host + " does not know a to-do as the step of a card", Hint: todoStepsUnknownHint, APICode: e.APICode, HTTPStatus: e.HTTPStatus}
		case apiCodeStepTodoUnknown:
			e.Hint = stepTodoUnknownHint
		case apiCodeStepTitleAndTodo, apiCodeStepCardAndTodo:
			e.Hint = "minka sends a text, a card or a to-do, never two of them, so this answer points at a fault in minka."
		case apiCodeCardArchived:
			e.Hint = "The card lies in the archive of its board. Bring it back with: minka cards unarchive " + cardID
		}
		unreadStepHint(e, cardID)
		return e
	}

	shown, breadcrumbs := todoID, stepBreadcrumbs(cardID)
	if linked := step.Value.Todo; linked != nil && linked.Seen() {
		shown = fmt.Sprintf("%q", linked.Title)
		breadcrumbs = append(breadcrumbs,
			Breadcrumb{Action: "show", Cmd: "minka todos show " + linked.ID, Description: "Show the to-do that is the step"},
			Breadcrumb{Action: "complete", Cmd: "minka todos complete " + linked.ID, Description: "Tick the to-do off, and the step with it"})
	}
	parent := cardAsNamed(given, cardID)
	summary := fmt.Sprintf("Added to-do %s as a step of card %s", shown, parent)
	if step.StatusCode == http.StatusOK {
		summary = fmt.Sprintf("To-do %s was a step of card %s already", shown, parent)
	}
	return a.ok(success{
		data:        step.Raw,
		summary:     summary,
		breadcrumbs: breadcrumbs,
		text: func(w io.Writer) {
			fmt.Fprintf(w, "%s.\n%s\n", summary, stepEntry(step.Value))
		},
	})
}
