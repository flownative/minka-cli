package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const subtaskHelp = `A subtask is given by its ID, by its position under the to-do starting at 1,
or by its text. A number always means a position. A text has to match exactly
one subtask, ignoring case.`

const subtaskNotFoundHint = "There is no such to-do or no such subtask on it: the to-do is in the trash, its list or its group is, or it belongs to a project you are not a member of. The subtasks of a to-do: minka todos subtasks list <todoId>"

func (a *app) newSubtasksCmd() *cobra.Command {
	return group("subtasks", "List, add, change, tick off, move and remove the subtasks of a to-do",
		a.newSubtasksListCmd(),
		a.newSubtasksCreateCmd(),
		a.newSubtasksEditCmd(),
		a.newSubtasksCompleteCmd(),
		a.newSubtasksReopenCmd(),
		a.newSubtasksMoveCmd(),
		a.newSubtasksDeleteCmd(),
	)
}

func (a *app) newSubtasksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <todoId>",
		Short: "List the subtasks of a to-do",
		Long:  "Lists the subtasks of a to-do in their order. data is the list as it stands in\nthe to-do.",
		Args:  requireArgs("todoId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			todo, err := s.client.Todo(a.ctx, args[0])
			if err != nil {
				return todoRefusal(s, err, todoHints{})
			}

			return a.ok(success{
				data:        rawField(todo.Raw, "subtasks"),
				summary:     fmt.Sprintf("%d of %s done on to-do %s", completedSubtasks(todo.Value.Subtasks), plural(len(todo.Value.Subtasks), "subtask", "subtasks"), todo.Value.Title),
				breadcrumbs: subtaskBreadcrumbs(todo.Value.ID),
				text: func(w io.Writer) {
					if len(todo.Value.Subtasks) == 0 {
						fmt.Fprintln(w, "No subtasks.")
						return
					}
					for i, subtask := range todo.Value.Subtasks {
						fmt.Fprintf(w, "%s  (%s)\n", subtaskLine(i, subtask), subtask.ID)
					}
				},
			})
		},
	}
}

func (a *app) newSubtasksCreateCmd() *cobra.Command {
	var assignee, due string

	cmd := &cobra.Command{
		Use:   "create <todoId> <text|->",
		Short: "Add a subtask at the end of a to-do",
		Long: `Adds a subtask at the end of a to-do. A subtask is one line of at most 500
characters; give - as the text to read it from stdin. --assignee names one
member of the project, who is told and follows the to-do. ` + personHelp + `
--due is a day, as YYYY-MM-DD.

A to-do of a list in the archive takes no subtask: exit 9 (validation) with
todoListArchived.`,
		Example: `  minka todos subtasks create 0199... "Find the cables" --assignee me --due 2026-10-01`,
		Args:    requireArgs("todoId", "text"),
		RunE: func(_ *cobra.Command, args []string) error {
			text, err := a.readValue("the subtask", args[1])
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			input := api.CreateSubtaskInput{Title: text, DueOn: due}
			if assignee != "" {
				people := newProjectPeople(a, s, func() (string, error) {
					todo, err := s.client.Todo(a.ctx, args[0])
					if err != nil {
						return "", todoRefusal(s, err, todoHints{})
					}
					return todo.Value.ProjectID, nil
				})
				ids, err := people.resolve([]string{assignee})
				if err != nil {
					return err
				}
				input.AssigneeID = ids[0]
			}

			todo, err := s.client.CreateSubtask(a.ctx, args[0], input)
			if err != nil {
				return todoRefusal(s, err, todoHints{})
			}

			summary := "Added a subtask to to-do " + todo.Value.Title
			return a.ok(success{
				data:        todo.Raw,
				summary:     summary,
				breadcrumbs: subtaskBreadcrumbs(todo.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					if n := len(todo.Value.Subtasks); n > 0 {
						fmt.Fprintf(w, "%s  (%s)\n", subtaskLine(n-1, todo.Value.Subtasks[n-1]), todo.Value.Subtasks[n-1].ID)
					}
				},
			})
		},
	}
	cmd.Flags().StringVar(&assignee, "assignee", "", "Person to assign: ID, me, e-mail address or name")
	cmd.Flags().StringVar(&due, "due", "", "Day the subtask is due, as YYYY-MM-DD")

	return cmd
}

func (a *app) newSubtasksEditCmd() *cobra.Command {
	var title, assignee, due string

	cmd := &cobra.Command{
		Use:   "edit <todoId> <subtask>",
		Short: "Change the text, the assignee or the day of a subtask",
		Long: `Changes the text, the assignee, the day or several of them. What is not given
stays as it is. --assignee "" and --due "" take the assignee or the day off.
--title "" does not empty the text: exit 9 (validation) with subtaskMissing.

` + subtaskHelp + " " + personHelp,
		Example: `  minka todos subtasks edit 0199... 2 --assignee anke@example.com
  minka todos subtasks edit 0199... "Find the cables" --due ""`,
		Args: requireArgs("todoId", "subtask"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			var input api.UpdateSubtaskInput
			if flags.Changed("title") {
				input.Title = &title
			}
			if flags.Changed("assignee") {
				input.AssigneeID = &assignee
			}
			if flags.Changed("due") {
				input.DueOn = &due
			}
			if input == (api.UpdateSubtaskInput{}) {
				return usageErrorf(cmd, "nothing to change, give --title, --assignee or --due")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			todo, subtask, err := a.findSubtask(s, args[0], args[1])
			if err != nil {
				return err
			}
			if input.AssigneeID != nil && *input.AssigneeID != "" {
				people := newProjectPeople(a, s, func() (string, error) { return todo.ProjectID, nil })
				ids, err := people.resolve([]string{*input.AssigneeID})
				if err != nil {
					return err
				}
				input.AssigneeID = &ids[0]
			}

			changed, err := s.client.UpdateSubtask(a.ctx, todo.ID, subtask.ID, input)
			if err != nil {
				return subtaskRefusal(s, err)
			}

			summary := "Changed a subtask of to-do " + changed.Value.Title
			return a.ok(success{
				data:        changed.Raw,
				summary:     summary,
				breadcrumbs: subtaskBreadcrumbs(todo.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New text of the subtask")
	flags.StringVar(&assignee, "assignee", "", "Person to assign: ID, me, e-mail address or name; empty for nobody")
	flags.StringVar(&due, "due", "", "Day the subtask is due as YYYY-MM-DD, empty to take it off")

	return cmd
}

func (a *app) newSubtasksCompleteCmd() *cobra.Command {
	return a.newSubtasksSetCompletedCmd("complete", "Tick a subtask off", true)
}

func (a *app) newSubtasksReopenCmd() *cobra.Command {
	return a.newSubtasksSetCompletedCmd("reopen", "Open a ticked off subtask again", false)
}

func (a *app) newSubtasksSetCompletedCmd(name, short string, completed bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <todoId> <subtask>",
		Short: short,
		Long: short + `. A subtask that stands that way already stays as it is, and
the command succeeds. The to-do itself is not ticked off or opened with it.

` + subtaskHelp,
		Example: fmt.Sprintf("  minka todos subtasks %s 0199... 2", name),
		Args:    requireArgs("todoId", "subtask"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			todo, subtask, err := a.findSubtask(s, args[0], args[1])
			if err != nil {
				return err
			}

			changed, err := s.client.UpdateSubtask(a.ctx, todo.ID, subtask.ID, api.UpdateSubtaskInput{Completed: &completed})
			if err != nil {
				return subtaskRefusal(s, err)
			}

			verb, state := "Completed", "done"
			if !completed {
				verb, state = "Reopened", "open"
			}
			summary := fmt.Sprintf("%s subtask %q of to-do %s", verb, subtask.Title, changed.Value.Title)
			if subtask.Title != "" && subtask.Completed == completed {
				summary = fmt.Sprintf("Subtask %q of to-do %s was %s already", subtask.Title, changed.Value.Title, state)
			}
			return a.ok(success{
				data:        changed.Raw,
				summary:     summary,
				breadcrumbs: subtaskBreadcrumbs(todo.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newSubtasksMoveCmd() *cobra.Command {
	var after, before string
	var top bool

	cmd := &cobra.Command{
		Use:   "move <todoId> <subtask>",
		Short: "Move a subtask to another place under its to-do",
		Long: `Puts a subtask elsewhere among the subtasks of its to-do: --top puts it first,
--after below the subtask named, --before above it. Exactly one of the three
is required. A subtask never leaves its to-do.

The subtask named with --after or --before has to be another subtask of the
same to-do; the subtask itself is refused with exit 9 (validation) and
meta.apiCode todoPlaceUnknown, and nothing moves.

` + subtaskHelp,
		Example: `  minka todos subtasks move 0199... 3 --top
  minka todos subtasks move 0199... 3 --after 1`,
		Args: requireArgs("todoId", "subtask"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the subtask goes: give exactly one of --top, --after and --before")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			todo, subtask, err := a.findSubtask(s, args[0], args[1])
			if err != nil {
				return err
			}
			var input api.MoveSubtaskInput
			for _, neighbour := range []struct {
				value string
				id    *string
			}{{after, &input.PreviousSubtaskID}, {before, &input.NextSubtaskID}} {
				if neighbour.value == "" {
					continue
				}
				found, err := resolveSubtask(todo.Subtasks, neighbour.value)
				if err != nil {
					return err
				}
				*neighbour.id = found.ID
			}

			moved, err := s.client.MoveSubtask(a.ctx, todo.ID, subtask.ID, input)
			if err != nil {
				e := subtaskRefusal(s, err)
				if e.APICode == apiCodeTodoPlaceUnknown {
					e.Hint = "The subtask named with --after or --before is the subtask itself or none of this to-do. The subtasks in their order: minka todos subtasks list " + todo.ID
				}
				return e
			}

			summary := "Moved a subtask of to-do " + moved.Value.Title
			return a.ok(success{
				data:        moved.Raw,
				summary:     summary,
				breadcrumbs: subtaskBreadcrumbs(todo.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&top, "top", false, "Put the subtask first")
	flags.StringVar(&after, "after", "", "Put the subtask below this subtask")
	flags.StringVar(&before, "before", "", "Put the subtask above this subtask")

	return cmd
}

func (a *app) newSubtasksDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <todoId> <subtask>",
		Short: "Remove a subtask from a to-do",
		Long: `Removes a subtask from a to-do, without asking and without a trash: a subtask
taken off is gone. Removing a subtask that is not on the to-do succeeds as
well.

` + subtaskHelp,
		Example: `  minka todos subtasks delete 0199... 3`,
		Args:    requireArgs("todoId", "subtask"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			todo, subtask, err := a.findSubtask(s, args[0], args[1])
			if err != nil {
				return err
			}
			if err := s.client.DeleteSubtask(a.ctx, todo.ID, subtask.ID); err != nil {
				return subtaskRefusal(s, err)
			}

			summary := fmt.Sprintf("Removed subtask %q from to-do %s", subtask.Title, todo.Title)
			if subtask.Title == "" {
				summary = fmt.Sprintf("Subtask %s was not on to-do %s", subtask.ID, todo.Title)
			}
			return a.ok(success{
				data:        map[string]any{"todoId": todo.ID, "subtaskId": subtask.ID, "wasOnTodo": subtask.Title != ""},
				summary:     summary,
				breadcrumbs: subtaskBreadcrumbs(todo.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

// findSubtask reads a to-do and finds one of its subtasks by what somebody
// named it by. An ID that is on no subtask is passed on without a text.
func (a *app) findSubtask(s *session, todoID, value string) (api.Todo, api.TodoSubtask, error) {
	todo, err := s.client.Todo(a.ctx, todoID)
	if err != nil {
		return api.Todo{}, api.TodoSubtask{}, todoRefusal(s, err, todoHints{})
	}
	subtask, err := resolveSubtask(todo.Value.Subtasks, value)
	return todo.Value, subtask, err
}

func resolveSubtask(subtasks []api.TodoSubtask, value string) (api.TodoSubtask, error) {
	ids, titles := make([]string, len(subtasks)), make([]string, len(subtasks))
	for i, subtask := range subtasks {
		ids[i], titles[i] = subtask.ID, subtask.Title
	}
	at, id, err := findLine(namedLines{noun: "subtask", owner: "the to-do", ids: ids, titles: titles, described: describeSubtasks(subtasks)}, value)
	if err != nil || at < 0 {
		return api.TodoSubtask{ID: id}, err
	}
	return subtasks[at], nil
}

// subtaskRefusal classifies a failed write on a subtask, whose 404 may be the
// subtask as well as the to-do.
func subtaskRefusal(s *session, err error) *Error {
	e := todoRefusal(s, err, todoHints{})
	if e.Code == CodeNotFound {
		e.Hint = subtaskNotFoundHint
	}
	return e
}

func describeSubtasks(subtasks []api.TodoSubtask) string {
	if len(subtasks) == 0 {
		return "The to-do has no subtasks."
	}
	lines := make([]string, len(subtasks))
	for i, subtask := range subtasks {
		lines[i] = fmt.Sprintf("%s (%s)", subtaskLine(i, subtask), subtask.ID)
	}
	return "Subtasks: " + strings.Join(lines, "; ")
}

func subtaskLine(index int, subtask api.TodoSubtask) string {
	mark := " "
	if subtask.Completed {
		mark = "x"
	}
	return fmt.Sprintf("%d. [%s] %s%s", index+1, mark, subtask.Title, subtaskDetails(subtask))
}

// subtaskDetails says who has a subtask and when it is due.
func subtaskDetails(subtask api.TodoSubtask) string {
	var details []string
	if subtask.Assignee != nil {
		details = append(details, subtask.Assignee.Name)
	}
	if subtask.DueOn != nil {
		due := "due " + *subtask.DueOn
		if subtask.IsOverdue {
			due += ", overdue"
		}
		details = append(details, due)
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func completedSubtasks(subtasks []api.TodoSubtask) int {
	completed := 0
	for _, subtask := range subtasks {
		if subtask.Completed {
			completed++
		}
	}
	return completed
}

// rawField is one field of an answer as the API sent it, or an empty list
// where the answer carries none.
func rawField(raw json.RawMessage, name string) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields[name] == nil {
		return json.RawMessage("[]")
	}
	return fields[name]
}

func subtaskBreadcrumbs(todoID string) []Breadcrumb {
	return []Breadcrumb{
		{Action: "list", Cmd: "minka todos subtasks list " + todoID, Description: "List the subtasks of the to-do"},
		{Action: "show", Cmd: "minka todos show " + todoID, Description: "Show the to-do"},
	}
}
