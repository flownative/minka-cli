package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a request for a page of to-dos it cannot read.
const (
	apiCodeTodosLimitNotANumber        = "todosLimitNotANumber"
	apiCodeCompletedTodosBeforeUnknown = "completedTodosBeforeUnknown"
	apiCodeTodosAfterUnknown           = "todosAfterUnknown"
)

const todoNotFoundHint = "There is no such to-do, it is in the trash, its list or its group is in the trash, or it belongs to a project you are not a member of. The open to-dos of a list are named by: minka todolists show <listId>, your own by: minka todos mine"

const todoListOfTheTodoArchivedHint = "The list of the to-do lies in the archive of its tool: the to-do can be read, and nothing about it changes until the list is back. minka todos show <todoId> names the list; bring it back with: minka todolists unarchive <listId>"

const todoPeopleHint = "Only members of the project can be named on a to-do or a subtask. The members are listed by: minka people list <projectId>"

const todoNotesHelp = `The notes are Markdown by default and are sent as HTML, the way the
description of a card is. Use --notes - to read them from stdin.

--notes-format html sends HTML as it is. Minka refuses no notes: notes that
carry no element it serves are kept as the characters they consist of, so
Markdown sent that way stands on the to-do afterwards as "# Heading". The
command says so in a notice, and nothing else ever will.`

const todoDaysHelp = `A day is written as YYYY-MM-DD. Minka keeps a start only on a day before the
due day: a start alone, or one on the due day, becomes the due day, and the
command says so in a notice.`

const todoPeopleFlagsHelp = `--assignee and --notify-when-done name a member of the project each and can
be given more than once. ` + personHelp

func (a *app) newTodosCmd() *cobra.Command {
	return group("todos", "Show, add, change, tick off, move, trash and repeat to-dos, and their subtasks",
		a.newTodosMineCmd(),
		a.newTodosShowCmd(),
		a.newTodosCompletedCmd(),
		a.newTodosCreateCmd(),
		a.newTodosEditCmd(),
		a.newTodosCompleteCmd(),
		a.newTodosReopenCmd(),
		a.newTodosMoveCmd(),
		a.newTodosDeleteCmd(),
		a.newTodosRepeatCmd(),
		a.newSubtasksCmd(),
		a.newTodoSeriesCmd(),
	)
}

func (a *app) newTodosShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <todoId>",
		Short: "Show a to-do with its notes, its people, its subtasks and its comments",
		Long: `Shows a to-do: where it stands, whether it is done, its days, who is on it and
who is told when it is done, its notes, its subtasks in their order and the
comments under it. In text output the notes are converted to Markdown; in
JSON they are the HTML the server stores.

A to-do of a list in the archive is shown as well and says listArchived:
true. Reading works; nothing about it changes until the list is back.`,
		Args: requireArgs("todoId"),
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
				data:        todo.Raw,
				summary:     todoSummary(todo.Value),
				breadcrumbs: todoBreadcrumbs(todo.Value),
				text:        func(w io.Writer) { renderTodo(w, todo.Value) },
			})
		},
	}
}

func (a *app) newTodosCreateCmd() *cobra.Command {
	var listID, groupID, title, notes, notesFormat, startsOn, due string
	var assignees, notified []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a to-do at the end of a to-do list",
		Long: `Adds a to-do at the end of a list, or with --group at the end of a group of
it. A to-do may be only a title.

` + todoPeopleFlagsHelp + ` Assignees are told and follow the to-do;
the people named with --notify-when-done are told when it is done. The to-do
is added together with its people, or not at all.

` + todoDaysHelp + `

` + todoNotesHelp + `

A list in the archive takes no new to-do: exit 9 (validation) with
meta.apiCode todoListArchived.`,
		Example: `  minka todos create --list 0199... --title "Fetch the speakers" --assignee me --due 2026-10-02
  minka todos create --list 0199... --group 0199... --title "Book the van" --notes - < notes.md`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--list", &listID); err != nil {
				return err
			}
			if groupID != "" {
				if err := requireID(cmd, "--group", &groupID); err != nil {
					return err
				}
			}
			if err := checkNotesFormat(cmd, notesFormat); err != nil {
				return err
			}

			input := api.CreateTodoInput{Title: title, GroupID: groupID}
			var notices []string
			if cmd.Flags().Changed("notes") {
				html, warnings, err := a.richTextHTML("the notes", "--notes", notes, notesFormat)
				if err != nil {
					return err
				}
				input.Notes, notices = &html, warnings
			}
			if cmd.Flags().Changed("starts-on") {
				input.StartsOn = &startsOn
			}
			if cmd.Flags().Changed("due") {
				input.DueOn = &due
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			projectOfTheList := func() (string, error) {
				list, err := s.client.TodoList(a.ctx, listID)
				if err != nil {
					return "", todoListRefusal(s, err, listID)
				}
				return list.Value.ProjectID, nil
			}
			people := newProjectPeople(a, s, projectOfTheList)
			if input.AssigneeIDs, err = people.resolve(assignees); err != nil {
				return err
			}
			if input.NotifiedIDs, err = people.resolve(notified); err != nil {
				return err
			}

			todo, err := s.client.CreateTodo(a.ctx, listID, input)
			if err != nil {
				e := todoListRefusal(s, err, listID)
				switch e.APICode {
				case apiCodeTodoPlaceUnknown:
					e.Hint = "--group names no group of the list, or one in the trash. The groups of the list: minka todolists show " + listID
				case apiCodePersonNotAProjectMember:
					e.Hint = todoPeopleHint
				}
				return e
			}

			summary := "Added to-do " + todo.Value.Title + " to list " + todo.Value.List.Title
			return a.ok(success{
				data:        todo.Raw,
				summary:     summary,
				notices:     append(notices, dayNotices(false, nil, input.StartsOn, input.DueOn, todo.Value)...),
				breadcrumbs: todoBreadcrumbs(todo.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nTo-do: %s\n%s\n", summary, todo.Value.ID, todo.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&listID, "list", "", "ID of the list the to-do goes into")
	flags.StringVar(&groupID, "group", "", "ID of a group of the list the to-do goes into")
	flags.StringVar(&title, "title", "", "Title of the to-do")
	flags.StringVar(&notes, "notes", "", "Notes, or - to read them from stdin")
	flags.StringVar(&notesFormat, "notes-format", "markdown", "Format of the notes: markdown or html")
	flags.StringVar(&startsOn, "starts-on", "", "Day the to-do starts, as YYYY-MM-DD, before the due day")
	flags.StringVar(&due, "due", "", "Day the to-do is due, as YYYY-MM-DD")
	flags.StringArrayVar(&assignees, "assignee", nil, "Person to assign: ID, me, e-mail address or name (repeatable)")
	flags.StringArrayVar(&notified, "notify-when-done", nil, "Person to tell when the to-do is done: ID, me, e-mail address or name (repeatable)")

	return cmd
}

func (a *app) newTodosEditCmd() *cobra.Command {
	var title, notes, notesFormat, startsOn, due string
	var assignees, notified []string
	var noAssignees, nobodyNotified, dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <todoId>",
		Short: "Change the title, the notes, the days or the people of a to-do",
		Long: `Changes any of title, notes, days and people in one write. What is not given
stays as it is.

--title "" does not empty the title: a to-do needs one, so an empty title exits
9 (validation) with meta.apiCode todoTitleMissing. --notes "" empties the
notes. --starts-on "" and --due "" take a day off.

` + todoDaysHelp + ` Taking the due day off while a start is left
makes the start the due day: give --starts-on "" as well to take both off.

--assignee and --notify-when-done give the WHOLE set of people: whoever is on
the to-do and not named again is taken off. --no-assignees and
--no-notify-when-done take everybody off. ` + personHelp + `

` + todoNotesHelp + `

Files and mentions stand in the notes as action-text-attachment elements,
which Markdown cannot hold. To keep them, take notesSource from todos show
--json, change it and send it back with --notes-format html. New notes that
would drop them are refused unless --drop-attachments is given.

A to-do of a list in the archive takes no change: exit 9 with
todoListArchived.`,
		Example: `  minka todos edit 0199... --due 2026-10-04
  minka todos edit 0199... --assignee me --assignee anke@example.com
  minka todos edit 0199... --notes - < notes.md`,
		Args: requireArgs("todoId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			if err := checkNotesFormat(cmd, notesFormat); err != nil {
				return err
			}
			if noAssignees && len(assignees) > 0 {
				return usageErrorf(cmd, "--assignee and --no-assignees contradict each other")
			}
			if nobodyNotified && len(notified) > 0 {
				return usageErrorf(cmd, "--notify-when-done and --no-notify-when-done contradict each other")
			}

			var input api.UpdateTodoInput
			var notices []string
			if flags.Changed("title") {
				input.Title = &title
			}
			if flags.Changed("notes") {
				html, warnings, err := a.richTextHTML("the notes", "--notes", notes, notesFormat)
				if err != nil {
					return err
				}
				input.Notes, notices = &html, warnings
			}
			if flags.Changed("starts-on") {
				input.StartsOn = &startsOn
			}
			if flags.Changed("due") {
				input.DueOn = &due
			}
			if noAssignees {
				input.AssigneeIDs = &[]string{}
			}
			if nobodyNotified {
				input.NotifiedIDs = &[]string{}
			}
			if input == (api.UpdateTodoInput{}) && len(assignees) == 0 && len(notified) == 0 {
				return usageErrorf(cmd, "nothing to change, give --title, --notes, --starts-on, --due, --assignee or --notify-when-done")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			var current *api.Todo
			read := func() (*api.Todo, error) {
				if current == nil {
					todo, err := s.client.Todo(a.ctx, args[0])
					if err != nil {
						return nil, todoRefusal(s, err, todoHints{})
					}
					current = &todo.Value
				}
				return current, nil
			}
			people := newProjectPeople(a, s, func() (string, error) {
				todo, err := read()
				if err != nil {
					return "", err
				}
				return todo.ProjectID, nil
			})
			if len(assignees) > 0 {
				ids, err := people.resolve(assignees)
				if err != nil {
					return err
				}
				input.AssigneeIDs = &ids
			}
			if len(notified) > 0 {
				ids, err := people.resolve(notified)
				if err != nil {
					return err
				}
				input.NotifiedIDs = &ids
			}
			if input.Notes != nil {
				todo, err := read()
				if err != nil {
					return err
				}
				// A to-do of a list in the archive takes no notes at all.
				dropped, err := a.guardOverFiles(s, todo.ProjectID, closedIf(todo.ListArchived), firstNonEmpty(todo.NotesSource, todo.Notes), *input.Notes, dropAttachments,
					"the notes have attachments or mentions, and the new notes would drop them",
					"Change notesSource from todos show --json and send it back with --notes-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}
			// A due day alone is held against the start the to-do carries, so
			// what Minka does to that start can only be told from a read.
			if input.DueOn != nil && strings.TrimSpace(*input.DueOn) != "" && input.StartsOn == nil {
				if _, err := read(); err != nil {
					return err
				}
			}

			todo, err := s.client.UpdateTodo(a.ctx, args[0], input)
			if err != nil {
				return todoRefusal(s, err, todoHints{})
			}

			summary := "Changed to-do " + todo.Value.Title
			return a.ok(success{
				data:        todo.Raw,
				summary:     summary,
				notices:     append(notices, dayNotices(true, current, input.StartsOn, input.DueOn, todo.Value)...),
				breadcrumbs: todoBreadcrumbs(todo.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, todo.Value.AppURL) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.StringVar(&notes, "notes", "", "New notes, - to read them from stdin, empty to remove them")
	flags.StringVar(&notesFormat, "notes-format", "markdown", "Format of the notes: markdown or html")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace notes that have attachments or mentions, which drops them")
	flags.StringVar(&startsOn, "starts-on", "", "Day the to-do starts as YYYY-MM-DD, empty to take it off")
	flags.StringVar(&due, "due", "", "Day the to-do is due as YYYY-MM-DD, empty to take it off")
	flags.StringArrayVar(&assignees, "assignee", nil, "Person to be assigned, the whole set (repeatable)")
	flags.BoolVar(&noAssignees, "no-assignees", false, "Take everybody off the to-do")
	flags.StringArrayVar(&notified, "notify-when-done", nil, "Person to tell when the to-do is done, the whole set (repeatable)")
	flags.BoolVar(&nobodyNotified, "no-notify-when-done", false, "Tell nobody when the to-do is done")

	return cmd
}

func (a *app) newTodosCompleteCmd() *cobra.Command {
	return a.newTodosSetCompletedCmd("complete", "Tick a to-do off", true)
}

func (a *app) newTodosReopenCmd() *cobra.Command {
	return a.newTodosSetCompletedCmd("reopen", "Open a done to-do again", false)
}

func (a *app) newTodosSetCompletedCmd(name, short string, completed bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <todoId>",
		Short: short,
		Long: short + `. A to-do that stands that way already stays as it is, and the
command succeeds. Its subtasks stay as they are either way.

A to-do of a list in the archive takes neither: exit 9 (validation) with
todoListArchived.`,
		Args: requireArgs("todoId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			var todo api.Result[api.Todo]
			if completed {
				todo, err = s.client.CompleteTodo(a.ctx, args[0])
			} else {
				todo, err = s.client.ReopenTodo(a.ctx, args[0])
			}
			if err != nil {
				return todoRefusal(s, err, todoHints{})
			}

			summary := "To-do " + todo.Value.Title + " is done"
			if !completed {
				summary = "To-do " + todo.Value.Title + " is open"
			}
			return a.ok(success{
				data:        todo.Raw,
				summary:     summary,
				breadcrumbs: todoBreadcrumbs(todo.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newTodosMoveCmd() *cobra.Command {
	var listID, groupID, after, before string
	var top, noGroup bool

	cmd := &cobra.Command{
		Use:   "move <todoId>",
		Short: "Move a to-do within its list, into a group, or into another list of its tool",
		Long: `Puts a to-do elsewhere: --top puts it first in its place, --after below the
to-do named, --before above it. Exactly one of the three is required.

The place is a list and a group. --list names another list of the same
To-dos tool; without it the to-do stays in its list. --group names a group of
that list and --no-group puts the to-do into none. Without either, the to-do
keeps its group within its own list and stands in no group in another list.

The to-do named with --after or --before has to stand in the list and the
group the to-do goes to. One of another group, the to-do itself, a list or
group that is not there, and a list of another tool are refused with exit 9
(validation) and meta.apiCode todoPlaceUnknown, and nothing moves. Taking a
to-do into another tool or project is done in Minka on the web.

A to-do of a list in the archive does not move, and none moves into such a
list: exit 9 with todoListArchived.`,
		Example: `  minka todos move 0199... --top
  minka todos move 0199... --group 0199... --after 0199...
  minka todos move 0199... --list 0199... --no-group --top`,
		Args: requireArgs("todoId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the to-do goes: give exactly one of --top, --after and --before")
			}
			if groupID != "" && noGroup {
				return usageErrorf(cmd, "--group and --no-group contradict each other")
			}
			for flag, id := range map[string]*string{"--list": &listID, "--group": &groupID, "--after": &after, "--before": &before} {
				if *id != "" {
					if err := requireID(cmd, flag, id); err != nil {
						return err
					}
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			input := api.MoveTodoInput{ListID: listID, GroupID: groupID, NoGroup: noGroup, PreviousTodoID: after, NextTodoID: before}
			todo, err := s.client.MoveTodo(a.ctx, args[0], input)
			if err != nil {
				return todoRefusal(s, err, todoHints{
					place:    "The list, the group or the to-do named is not there. A to-do named with --after or --before has to stand in the list and the group the to-do goes to: without --group the to-do keeps its group within its own list, so name the neighbour's group with --group, or give --no-group for one standing in no group. --list takes a list of the same To-dos tool; another tool or project is done in Minka on the web. The groups and open to-dos of a list: minka todolists show <listId>",
					archived: "The list the to-do stands in, or the one named with --list, lies in the archive of its tool, and nothing in it changes until it is back: minka todolists unarchive <listId>",
				})
			}

			summary := "Moved to-do " + todo.Value.Title + " in list " + todo.Value.List.Title
			if todo.Value.Group != nil {
				summary += ", group " + todo.Value.Group.Title
			}
			return a.ok(success{
				data:        todo.Raw,
				summary:     summary,
				breadcrumbs: todoBreadcrumbs(todo.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&top, "top", false, "Put the to-do first in its place")
	flags.StringVar(&after, "after", "", "Put the to-do below this to-do")
	flags.StringVar(&before, "before", "", "Put the to-do above this to-do")
	flags.StringVar(&listID, "list", "", "Take the to-do into this list of the same tool")
	flags.StringVar(&groupID, "group", "", "Put the to-do into this group of the list")
	flags.BoolVar(&noGroup, "no-group", false, "Put the to-do into no group")

	return cmd
}

func (a *app) newTodosDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <todoId>",
		Aliases: []string{"trash"},
		Short:   "Move a to-do to the trash",
		Long: `Moves a to-do into the trash of its project, with its subtasks. It leaves its
list and the counts of it. Showing, changing or deleting it again exits with 2
(not_found). "trash list <projectId>" shows it, and it comes back from there
in Minka on the web.`,
		Args: requireArgs("todoId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteTodo(a.ctx, args[0]); err != nil {
				return todoRefusal(s, err, todoHints{})
			}

			summary := "Moved to-do " + args[0] + " to the trash"
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newTodosCompletedCmd() *cobra.Command {
	var limit int
	var before string

	cmd := &cobra.Command{
		Use:   "completed <listId>",
		Short: "List the done to-dos of a to-do list, a page at a time",
		Long: `Lists the done to-dos of a list, in use or in the archive, the one done last
first. data is the page as Minka answers it: {todos, next}.

next is where the page after this one starts, and null on the last page. Hand
it to --before exactly as it came; it is no ID. Opening a to-do again between
two pages moves nothing: the next page starts behind the moment the last one
was ticked off, not behind the to-do.

--limit is the size of the page, 50 unless given. Minka serves at most 100 and
cuts a larger page down to that, which next then says.`,
		Example: `  minka todos completed 0199... --json
  minka todos completed 0199... --before MjAyNi0wOS0xM1Qx... --json`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			page, err := s.client.CompletedTodos(a.ctx, args[0], limit, before)
			if err != nil {
				e := todoListRefusal(s, err, args[0])
				switch e.APICode {
				case apiCodeTodosLimitNotANumber:
					e.Hint = "--limit is a whole number of at least 1."
				case apiCodeCompletedTodosBeforeUnknown:
					e.Hint = "--before takes the next of the page before exactly as it came, and nothing else. The first page: minka todos completed " + args[0]
				}
				return e
			}

			summary := plural(len(page.Value.Todos), "done to-do", "done to-dos")
			var crumbs []Breadcrumb
			if page.Value.Next != nil {
				summary += ", and more on the next page"
				crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: "minka todos completed " + args[0] + " --before " + *page.Value.Next, Description: "Read the next page"})
			}
			crumbs = append(crumbs, Breadcrumb{Action: "reopen", Cmd: "minka todos reopen <todoId>", Description: "Open a done to-do again"})
			return a.ok(success{
				data:        page.Raw,
				summary:     summary,
				breadcrumbs: crumbs,
				text: func(w io.Writer) {
					renderCompletedTodos(w, page.Value.Todos)
					if page.Value.Next != nil {
						fmt.Fprintf(w, "\nNext page: minka todos completed %s --before %s\n", args[0], *page.Value.Next)
					}
				},
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Number of to-dos on the page")
	cmd.Flags().StringVar(&before, "before", "", "Where the page starts: next of the page before")

	return cmd
}

func (a *app) newTodosMineCmd() *cobra.Command {
	var limit int
	var after string

	cmd := &cobra.Command{
		Use:   "mine",
		Short: "List the open to-dos you are responsible for, across all projects",
		Long: `Lists the open to-dos the account of the token is assigned to or holds an open
subtask of, in every project it is a member of, as a bare array. Each names
its project, its tool and its list; isAssigned is false where only a subtask
of it is yours, and subtasks holds only your own open ones.

Done to-dos, to-dos in the trash, to-dos of a list in the archive and to-dos of
an archived project are not among them, so a to-do missing here is not
necessarily done.

The order is the one of the page of your tasks: projects by name, then by due
day with those without one last, then by title.

The list comes a page at a time, and Minka marks its end by nothing but a page
shorter than asked for. --limit is 1 to 100, 50 unless given; a full page says
in a notice where the next one starts: --after <the last todoId>.`,
		Example: `  minka todos mine --json
  minka todos mine --after 0199... --json`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 || limit > api.MyTodosPageMax {
				return usageErrorf(cmd, "--limit is 1 to %d, not %d: Minka serves no larger page and would cut one down without a word", api.MyTodosPageMax, limit)
			}
			if after != "" {
				if err := requireID(cmd, "--after", &after); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			todos, err := s.client.MyTodos(a.ctx, limit, after)
			if err != nil {
				e := s.fail(err)
				if e.APICode == apiCodeTodosAfterUnknown {
					e.Hint = "--after names no to-do of a project you are a member of. The first page: minka todos mine"
				}
				return e
			}

			var notices []string
			crumbs := []Breadcrumb{
				{Action: "show", Cmd: "minka todos show <todoId>", Description: "Read a to-do with its notes, subtasks and comments"},
				{Action: "complete", Cmd: "minka todos complete <todoId>", Description: "Tick a to-do off"},
			}
			if n := len(todos.Value); n == limit {
				next := "minka todos mine --after " + todos.Value[n-1].ID
				notices = append(notices, "the page is full, so there may be more: "+next)
				crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: next, Description: "Read the next page"})
			}
			return a.ok(success{
				data:        todos.Raw,
				summary:     plural(len(todos.Value), "to-do", "to-dos"),
				notices:     notices,
				breadcrumbs: crumbs,
				text:        func(w io.Writer) { renderMyTodos(w, todos.Value) },
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Number of to-dos on the page, 1 to 100")
	cmd.Flags().StringVar(&after, "after", "", "Start after this to-do: the last one of the page before")

	return cmd
}

// todoHints are the hints of a command for the refusals whose cause depends
// on what the command named.
type todoHints struct {
	place    string
	archived string
}

// todoRefusal classifies a failed request about a to-do. Each code names a
// state the to-do or its place is in, and the hint says what can be done.
func todoRefusal(s *session, err error, hints todoHints) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeTodoListArchived:
		e.Hint = firstNonEmpty(hints.archived, todoListOfTheTodoArchivedHint)
	case e.APICode == apiCodeTodoPlaceUnknown && hints.place != "":
		e.Hint = hints.place
	case e.APICode == apiCodePersonNotAProjectMember:
		e.Hint = todoPeopleHint
	case e.Code == CodeNotFound:
		e.Hint = todoNotFoundHint
	}
	return e
}

// dayNotices say where Minka kept the days of a to-do otherwise than they were
// meant: a start alone or on the due day is the due day, and a due day taken off
// under a start makes the start the due day. The server answers all of it with
// the same success as any write, so this is the only word about it. The start
// meant is the one sent, or on an edit that sends none the one the to-do
// carried before.
func dayNotices(edit bool, before *api.Todo, sentStart, sentDue *string, after api.Todo) []string {
	if edit && sentStart == nil && sentDue != nil && strings.TrimSpace(*sentDue) == "" && after.DueOn != nil {
		return []string{fmt.Sprintf(`the due day was not taken off: Minka keeps no start without a due day, so the start became the due day, %s. Give --starts-on "" as well to take both off`, *after.DueOn)}
	}
	start := ""
	switch {
	case sentStart != nil:
		start = strings.TrimSpace(*sentStart)
	case before != nil:
		start = deref(before.StartsOn)
	}
	if start != "" && after.StartsOn == nil {
		return []string{fmt.Sprintf("the start %s was not kept as a start: Minka keeps a start only on a day before the due day, and a day alone is the due day, which is now %s", start, deref(after.DueOn))}
	}
	return nil
}

func checkNotesFormat(cmd *cobra.Command, format string) error {
	if format != "markdown" && format != "html" {
		return usageErrorf(cmd, "--notes-format is markdown or html, not %q", format)
	}
	return nil
}

// projectPeople turns the people given for a to-do or a subtask into IDs. The
// members of the project are read only where a name, an e-mail address or
// "me" asks for them, and at most once; an ID goes out as it is, so that Minka
// says what it thinks of it. A person named twice goes out twice, and Minka
// takes them once.
type projectPeople struct {
	a         *app
	s         *session
	projectID func() (string, error)
	members   []api.ProjectMember
	read      bool
}

func newProjectPeople(a *app, s *session, projectID func() (string, error)) *projectPeople {
	return &projectPeople{a: a, s: s, projectID: projectID}
}

func (p *projectPeople) resolve(values []string) ([]string, error) {
	var ids []string
	for _, value := range values {
		id, isID := asID(strings.TrimSpace(value))
		if !isID {
			if err := p.readMembers(); err != nil {
				return nil, err
			}
			person, err := resolvePerson(value, p.members, p.a.meID(p.s))
			if err != nil {
				return nil, err
			}
			id = person.ID
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (p *projectPeople) readMembers() error {
	if p.read {
		return nil
	}
	projectID, err := p.projectID()
	if err != nil {
		return err
	}
	members, err := p.s.client.People(p.a.ctx, projectID)
	if err != nil {
		return p.s.fail(err)
	}
	p.members, p.read = members.Value, true
	return nil
}

func todoSummary(todo api.Todo) string {
	state := "open"
	if todo.Completed {
		state = "done"
	}
	summary := "To-do " + todo.Title + " (" + state + ") in list " + todo.List.Title
	if todo.ListArchived {
		summary += ", which lies in the archive"
	}
	return summary
}

func todoBreadcrumbs(todo api.Todo) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka todos show " + todo.ID, Description: "Read the to-do with its notes, subtasks and comments"},
	}
	if todo.ListArchived {
		return append(crumbs, Breadcrumb{Action: "unarchive", Cmd: "minka todolists unarchive " + todo.ListID, Description: "Bring the list of the to-do back, so that it changes again"})
	}
	if todo.Completed {
		crumbs = append(crumbs, Breadcrumb{Action: "reopen", Cmd: "minka todos reopen " + todo.ID, Description: "Open the to-do again"})
	} else {
		crumbs = append(crumbs, Breadcrumb{Action: "complete", Cmd: "minka todos complete " + todo.ID, Description: "Tick the to-do off"})
	}
	switch {
	case todo.Series != nil:
		crumbs = append(crumbs, Breadcrumb{Action: "series", Cmd: "minka todos series show " + todo.Series.ID, Description: "Read the series the to-do is an occurrence of"})
	case todo.DueOn != nil:
		crumbs = append(crumbs, Breadcrumb{Action: "repeat", Cmd: "minka todos repeat " + todo.ID, Description: "Make the to-do repeat from its due day"})
	}
	return append(crumbs,
		Breadcrumb{Action: "subtask", Cmd: "minka todos subtasks create " + todo.ID + " <text>", Description: "Add a subtask"},
		Breadcrumb{Action: "list", Cmd: "minka todolists show " + todo.ListID, Description: "Read the list of the to-do"},
	)
}

func renderTodo(w io.Writer, todo api.Todo) {
	fmt.Fprintf(w, "# %s\n", todo.Title)
	fmt.Fprintf(w, "To-do: %s\nList: %s (%s)\n", todo.ID, todo.List.Title, todo.ListID)
	if todo.Group != nil {
		fmt.Fprintf(w, "Group: %s (%s)\n", todo.Group.Title, todo.Group.ID)
	}
	fmt.Fprintf(w, "Tool: %s (%s)\nProject: %s (%s)\n", todo.Tool.Title, todo.ToolID, todo.Project.Name, todo.ProjectID)
	if todo.ListArchived {
		fmt.Fprintln(w, "Archived: its list lies in the archive of its tool")
	}
	switch {
	case todo.Completed && todo.CompletedByName != nil:
		fmt.Fprintf(w, "Done: %s by %s\n", formatTime(deref(todo.CompletedAt)), *todo.CompletedByName)
	case todo.Completed:
		fmt.Fprintf(w, "Done: %s\n", formatTime(deref(todo.CompletedAt)))
	default:
		fmt.Fprintln(w, "Done: no")
	}
	if todo.StartsOn != nil {
		fmt.Fprintf(w, "Starts: %s\n", *todo.StartsOn)
	}
	if todo.DueOn != nil {
		overdue := ""
		if todo.IsOverdue {
			overdue = " (overdue)"
		}
		fmt.Fprintf(w, "Due: %s%s\n", *todo.DueOn, overdue)
	}
	if todo.Series != nil {
		if todo.Series.OccursOn != nil {
			fmt.Fprintf(w, "Repeats: the occurrence of %s of series %s\n", *todo.Series.OccursOn, todo.Series.ID)
		} else {
			fmt.Fprintf(w, "Repeats: an occurrence of series %s\n", todo.Series.ID)
		}
	}
	if len(todo.Assignees) > 0 {
		fmt.Fprintf(w, "Assigned: %s\n", assigneeNames(todo.Assignees))
	}
	if len(todo.Notified) > 0 {
		fmt.Fprintf(w, "Told when done: %s\n", assigneeNames(todo.Notified))
	}
	fmt.Fprintf(w, "Created: %s by %s\nURL: %s\n", formatTime(todo.CreatedAt), todo.Creator.Name, todo.AppURL)

	if notes := richtext.ToMarkdown(firstNonEmpty(todo.NotesSource, todo.Notes)); notes != "" {
		fmt.Fprintf(w, "\n## Notes\n\n%s\n", notes)
	}
	renderEntryFiles(w, todo.Attachments)
	if len(todo.Subtasks) > 0 {
		fmt.Fprintf(w, "\n## Subtasks (%d of %d done)\n\n", completedSubtasks(todo.Subtasks), len(todo.Subtasks))
		for i, subtask := range todo.Subtasks {
			fmt.Fprintf(w, "%s  (%s)\n", subtaskLine(i, subtask), subtask.ID)
		}
	}
	renderEntryComments(w, todo.Comments)
}

func renderCompletedTodos(w io.Writer, todos []api.TodoLine) {
	if len(todos) == 0 {
		fmt.Fprintln(w, "No done to-dos.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tDONE\tTITLE")
	for _, todo := range todos {
		fmt.Fprintf(table, "%s\t%s\t%s\n", todo.ID, formatTime(deref(todo.CompletedAt)), todo.Title)
	}
	_ = table.Flush()
}

func renderMyTodos(w io.Writer, todos []api.AssignedTodo) {
	if len(todos) == 0 {
		fmt.Fprintln(w, "No to-dos.")
		return
	}
	for _, todo := range todos {
		details := []string{todo.Project.Name + " / " + todo.List.Title}
		if todo.DueOn != nil {
			due := "due " + *todo.DueOn
			if todo.IsOverdue {
				due += ", overdue"
			}
			details = append(details, due)
		}
		if !todo.IsAssigned {
			details = append(details, "a subtask of yours")
		}
		fmt.Fprintf(w, "- %s  %s (%s)\n", todo.ID, todo.Title, strings.Join(details, "; "))
		for _, subtask := range todo.Subtasks {
			fmt.Fprintf(w, "    [ ] %s%s  (%s)\n", subtask.Title, subtaskDetails(subtask), subtask.ID)
		}
	}
}
