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

// What Minka answers a write on a to-do list or a group that it will not
// carry out.
const (
	apiCodeTodoListArchived    = "todoListArchived"
	apiCodeTodoListNotArchived = "todoListNotArchived"
	apiCodeTodoPlaceUnknown    = "todoPlaceUnknown"
)

const todoListNotFoundHint = "There is no such list, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. The lists of a tool are listed with: minka todolists list <toolId>, and those in its archive with --archived."

// todoGroupNotFoundHint names the one place a group goes quiet in that a list
// does not: the archive of its list, where the list itself is still read.
const todoGroupNotFoundHint = "There is no such group, it is in the trash, it belongs to a project you are not a member of, or its list lies in the archive: a group of an archived list is at no address until the list is back, with minka todolists unarchive <listId>."

const todoListDescriptionHelp = `The description is Markdown by default and is sent as HTML, the way the
description of a card is. Use --description - to read it from stdin.

--description-format html sends HTML as it is. Minka refuses no description:
one that carries no element it serves is kept as the characters it consists of,
so Markdown sent that way stands on the list afterwards as "# Heading". The
command says so in a notice, and nothing else ever will.`

func (a *app) newTodoListsCmd() *cobra.Command {
	return group("todolists", "Read, start, change, archive, trash and comment on the to-do lists of a To-dos tool",
		a.newTodoListsToolsCmd(),
		a.newTodoListsListCmd(),
		a.newTodoListsShowCmd(),
		a.newTodoListsCreateCmd(),
		a.newTodoListsEditCmd(),
		a.newTodoListsArchiveCmd(),
		a.newTodoListsUnarchiveCmd(),
		a.newTodoListsDeleteCmd(),
		a.newItemCommentsCmd(todoListComments),
	)
}

// todoListComments are the comments under a to-do list. A list in the archive
// takes no text at all.
var todoListComments = commentedKind{
	parent: "todolists", noun: "to-do list", aNoun: "a to-do list", arg: "listId", kind: api.CommentedTodoList,
	read: func(a *app, s *session, id string) (commentedItem, error) {
		list, err := s.client.TodoList(a.ctx, id)
		if err != nil {
			return commentedItem{}, err
		}
		return commentedItem{projectID: list.Value.ProjectID, closed: closedIf(list.Value.Archived), comments: list.Value.Comments}, nil
	},
	refusal: todoListRefusal,
}

func (a *app) newTodoListsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the To-dos tools of a project",
		Long: `Lists the To-dos tools of a project that are in use, in the order of the
project page. A project can have more than one. The lists of one are read with
"todolists list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.TodosTools(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "To-dos tool", "To-dos tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka todolists list <toolId>", Description: "List the lists of a tool"},
				},
				text: func(w io.Writer) { renderTodosTools(w, tools.Value) },
			})
		},
	}
}

func (a *app) newTodoListsListCmd() *cobra.Command {
	var archived bool

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List the to-do lists of a To-dos tool, or those in its archive",
		Long: `Lists the lists on the page of a To-dos tool, in the order of the page: a new
list goes on top. --archived lists the archive of the tool instead, the list
archived last first. A list in the archive is on neither page but its own, so
a list missing from the one is looked for in the other.

Nothing here is paged and nothing is cut. A line carries the whole description
of a list, and how many of its to-dos are open and done, but not the to-dos
themselves: "todolists show <listId>" reads them.`,
		Example: `  minka todolists list 0199... --json
  minka todolists list 0199... --archived --json`,
		Args: requireArgs("toolId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			lists, err := s.client.TodoLists(a.ctx, args[0], archived)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "There is no such To-dos tool, or it belongs to a project you are not a member of. List the tools of a project with: minka todolists tools <projectId>"
				}
				return e
			}

			summary := plural(len(lists.Value), "list", "lists")
			if archived {
				summary += " in the archive"
			}
			return a.ok(success{
				data:        lists.Raw,
				summary:     summary,
				breadcrumbs: todoListsBreadcrumbs(args[0], archived),
				text:        func(w io.Writer) { renderTodoLists(w, lists.Value, archived) },
			})
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "List the archive of the tool instead")

	return cmd
}

func (a *app) newTodoListsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <listId>",
		Short: "Show a to-do list with its groups, its open to-dos and its comments",
		Long: `Shows a list with its description, the open to-dos standing in it, its groups
in their order with the open to-dos of each, and the comments under it. In text
output the description is converted to Markdown; in JSON it is the HTML the
server stores.

The done to-dos are counted and not listed. A list in the archive is shown as
well and says so: reading works, writing on it is refused.`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			list, err := s.client.TodoList(a.ctx, args[0])
			if err != nil {
				return todoListRefusal(s, err, args[0])
			}

			return a.ok(success{
				data:        list.Raw,
				summary:     todoListSummary(list.Value),
				breadcrumbs: todoListBreadcrumbs(list.Value),
				text:        func(w io.Writer) { renderTodoList(w, list.Value) },
			})
		},
	}
}

func (a *app) newTodoListsCreateCmd() *cobra.Command {
	var toolID, title, description, descriptionFormat string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Start a to-do list in a To-dos tool",
		Long: "Starts a list on top of a To-dos tool. A list may be only a title: without\n" +
			"--description it has none.\n\n" + todoListDescriptionHelp,
		Example: `  minka todolists create --tool 0199... --title "Setting up" --json
  minka todolists create --tool 0199... --title "Setting up" --description - < plan.md`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}

			input := api.CreateTodoListInput{Title: title}
			var notices []string
			if cmd.Flags().Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description, notices = &html, warnings
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			list, err := s.client.CreateTodoList(a.ctx, toolID, input)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "There is no such To-dos tool, or it belongs to a project you are not a member of. List the tools of a project with: minka todolists tools <projectId>"
				}
				return e
			}

			summary := "Started list " + list.Value.Title + " in " + list.Value.Tool.Title
			return a.ok(success{
				data:        list.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: todoListBreadcrumbs(list.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nList: %s\n%s\n", summary, list.Value.ID, list.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the To-dos tool the list goes into")
	flags.StringVar(&title, "title", "", "Title of the list")
	flags.StringVar(&description, "description", "", "Description, or - to read it from stdin")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")

	return cmd
}

func (a *app) newTodoListsEditCmd() *cobra.Command {
	var title, description, descriptionFormat string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <listId>",
		Short: "Change the title or the description of a to-do list",
		Long: "Changes the title, the description or both. What is not given stays as it is.\n\n" +
			`--description "" empties the description. --title "" does not empty the
title: a list needs one, so an empty title exits 9 (validation) with
meta.apiCode todoListTitleMissing. A title can only be replaced.` + "\n\n" +
			todoListDescriptionHelp + `

Files and mentions stand in a description as action-text-attachment elements,
which Markdown cannot hold. To keep them, take descriptionSource from
todolists show --json, change it and send it back with --description-format
html. A new description that would drop them is refused unless
--drop-attachments is given.

A list in the archive takes no change: exit 9 with todoListArchived.`,
		Example: `  minka todolists edit 0199... --title "Setting up on Saturday"
  minka todolists edit 0199... --description - < plan.md
  minka todolists edit 0199... --description ""`,
		Args: requireArgs("listId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}

			var input api.UpdateTodoListInput
			var notices []string
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description, notices = &html, warnings
			}
			if input.Title == nil && input.Description == nil {
				return usageErrorf(cmd, "nothing to change, give --title or --description")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if input.Description != nil {
				current, err := s.client.TodoList(a.ctx, args[0])
				if err != nil {
					return todoListRefusal(s, err, args[0])
				}
				// A list of the archive takes no description at all.
				dropped, err := a.guardOverFiles(s, current.Value.ProjectID, closedIf(current.Value.Archived), firstNonEmpty(current.Value.DescriptionSource, current.Value.Description), *input.Description, dropAttachments,
					"the description has %s, and the new description would drop them",
					"Change descriptionSource from todolists show --json and send it back with --description-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			list, err := s.client.UpdateTodoList(a.ctx, args[0], input)
			if err != nil {
				return todoListRefusal(s, err, args[0])
			}

			summary := "Changed list " + list.Value.Title
			return a.ok(success{
				data:        list.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: todoListBreadcrumbs(list.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, list.Value.AppURL) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.StringVar(&description, "description", "", "New description, - to read it from stdin, empty to remove it")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a description that has attachments, mentions or references, which drops them")

	return cmd
}

func (a *app) newTodoListsArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <listId>",
		Short: "Put a to-do list away into the archive of its tool",
		Long: `Takes a list off the page of its tool into the archive of the tool, and answers
with the list, which now says archived: true.

A list in the archive is read at its own address and listed with
"todolists list <toolId> --archived". Nothing in it changes until it is back:
every write on it is refused with 9 (validation) and meta.apiCode
todoListArchived, and its groups are at no address at all (exit 2) until then.
"todolists unarchive" brings it back, and "todolists delete" puts it into the
trash from the archive as well.

A list that lies in the archive already exits 9 with todoListArchived.`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			list, err := s.client.ArchiveTodoList(a.ctx, args[0])
			if err != nil {
				return todoListRefusal(s, err, args[0])
			}

			summary := "Put list " + list.Value.Title + " into the archive of " + list.Value.Tool.Title
			return a.ok(success{
				data:        list.Raw,
				summary:     summary,
				breadcrumbs: todoListBreadcrumbs(list.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, list.Value.AppURL) },
			})
		},
	}
}

func (a *app) newTodoListsUnarchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unarchive <listId>",
		Short: "Bring a to-do list back out of the archive of its tool",
		Long: `Brings a list back onto the page of its tool, where it takes the place it had
before it was put away, and answers with the list.

A list that stands on the page of its tool exits 9 (validation) with
meta.apiCode todoListNotArchived.`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			list, err := s.client.UnarchiveTodoList(a.ctx, args[0])
			if err != nil {
				return todoListRefusal(s, err, args[0])
			}

			summary := "Brought list " + list.Value.Title + " back onto " + list.Value.Tool.Title
			return a.ok(success{
				data:        list.Raw,
				summary:     summary,
				breadcrumbs: todoListBreadcrumbs(list.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n%s\n", summary, list.Value.AppURL) },
			})
		},
	}
}

func (a *app) newTodoListsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <listId>",
		Aliases: []string{"trash"},
		Short:   "Move a to-do list to the trash",
		Long: `Moves a list into the trash of its project, its groups and its to-dos with it,
from the page of its tool or from its archive.

A list in the trash reads like one that never existed: showing, changing or
deleting it again exits with 2 (not_found). "trash list <projectId>" shows it,
and it comes back from there in Minka on the web.`,
		Args: requireArgs("listId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteTodoList(a.ctx, args[0]); err != nil {
				return todoListRefusal(s, err, args[0])
			}

			summary := "Moved list " + args[0] + " to the trash"
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

// todoListRefusal classifies a failed request about a list. Each code names a
// state the list is in, and the hint says what can be done about it.
func todoListRefusal(s *session, err error, listID string) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeTodoListArchived:
		e.Hint = "The list lies in the archive of its tool. It can be read, and nothing in it changes until it is back: minka todolists unarchive " + listID
	case e.APICode == apiCodeTodoListNotArchived:
		e.Hint = "The list stands on the page of its tool already. Put it away with: minka todolists archive " + listID
	case e.Code == CodeNotFound:
		e.Hint = todoListNotFoundHint
	}
	return e
}

func renderTodosTools(w io.Writer, tools []api.TodosTool) {
	if len(tools) == 0 {
		fmt.Fprintln(w, "No To-dos tools.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE")
	for _, tool := range tools {
		fmt.Fprintf(table, "%s\t%s\n", tool.ID, tool.Title)
	}
	_ = table.Flush()
}

func renderTodoLists(w io.Writer, lists []api.TodoListItem, archived bool) {
	if len(lists) == 0 {
		if archived {
			fmt.Fprintln(w, "No lists in the archive.")
		} else {
			fmt.Fprintln(w, "No lists.")
		}
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE\tOPEN\tDONE")
	for _, list := range lists {
		fmt.Fprintf(table, "%s\t%s\t%d\t%d\n", list.ID, list.Title, list.OpenCount, list.CompletedCount)
	}
	_ = table.Flush()
}

func renderTodoList(w io.Writer, list api.TodoList) {
	fmt.Fprintf(w, "# %s\n", list.Title)
	fmt.Fprintf(w, "List: %s\nTool: %s (%s)\nProject: %s (%s)\n", list.ID, list.Tool.Title, list.ToolID, list.Project.Name, list.ProjectID)
	if list.Archived {
		fmt.Fprintln(w, "Archived: yes, in the archive of its tool")
	}
	fmt.Fprintf(w, "To-dos: %d open, %d done\n", list.OpenCount, list.CompletedCount)
	fmt.Fprintf(w, "URL: %s\n", list.AppURL)

	if description := richtext.ToMarkdown(firstNonEmpty(list.DescriptionSource, list.Description)); description != "" {
		fmt.Fprintf(w, "\n## Description\n\n%s\n", description)
	}
	renderEntryFiles(w, list.Attachments)

	fmt.Fprintf(w, "\n## Open to-dos (%d)\n\n", len(list.Todos))
	renderTodoLines(w, list.Todos)
	for _, group := range list.Groups {
		colour := ""
		if group.Color != nil {
			colour = *group.Color + ", "
		}
		fmt.Fprintf(w, "\n## %s (group %s, %s%d open)\n\n", group.Title, group.ID, colour, len(group.Todos))
		renderTodoLines(w, group.Todos)
	}
	renderEntryComments(w, list.Comments)
}

func renderTodoLines(w io.Writer, todos []api.TodoLine) {
	if len(todos) == 0 {
		fmt.Fprintln(w, "(none)")
		return
	}
	for _, todo := range todos {
		fmt.Fprintf(w, "- %s  %s%s\n", todo.ID, todo.Title, todoLineDetails(todo))
	}
}

// todoLineDetails says what a line of a list has to add to its title: who has
// it, when it is due and how far along it is.
func todoLineDetails(todo api.TodoLine) string {
	var details []string
	for _, person := range todo.Assignees {
		details = append(details, person.Name)
	}
	if todo.DueOn != nil {
		due := "due " + *todo.DueOn
		if todo.IsOverdue {
			due += ", overdue"
		}
		details = append(details, due)
	}
	if todo.SeriesID != nil {
		details = append(details, "repeats")
	}
	if todo.SubtaskCount > 0 {
		details = append(details, fmt.Sprintf("%d/%d subtasks", todo.CompletedSubtaskCount, todo.SubtaskCount))
	}
	if todo.CommentCount > 0 {
		details = append(details, plural(todo.CommentCount, "comment", "comments"))
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func todoListSummary(list api.TodoList) string {
	if list.Archived {
		return "List " + list.Title + " in the archive of " + list.Tool.Title
	}
	return "List " + list.Title + " in " + list.Tool.Title
}

func todoListsBreadcrumbs(toolID string, archived bool) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka todolists show <listId>", Description: "Read a list with its groups and open to-dos"},
	}
	if archived {
		return append(crumbs, Breadcrumb{Action: "in use", Cmd: "minka todolists list " + toolID, Description: "List the lists on the page of the tool"})
	}
	return append(crumbs,
		Breadcrumb{Action: "archive", Cmd: "minka todolists list " + toolID + " --archived", Description: "List the archive of the tool"},
		Breadcrumb{Action: "create", Cmd: "minka todolists create --tool " + toolID + " --title <title>", Description: "Start a list on top of the tool"},
	)
}

func todoListBreadcrumbs(list api.TodoList) []Breadcrumb {
	if list.Archived {
		return []Breadcrumb{
			{Action: "unarchive", Cmd: "minka todolists unarchive " + list.ID, Description: "Bring the list back onto the page of its tool"},
			{Action: "lists", Cmd: "minka todolists list " + list.ToolID + " --archived", Description: "List the archive of the tool"},
		}
	}
	return []Breadcrumb{
		{Action: "show", Cmd: "minka todolists show " + list.ID, Description: "Read the list with its groups and open to-dos"},
		{Action: "group", Cmd: "minka todogroups create --list " + list.ID + " --title <title>", Description: "Add a group at the end of the list"},
		{Action: "edit", Cmd: "minka todolists edit " + list.ID + " --description -", Description: "Change the title or the description"},
		{Action: "comment", Cmd: "minka todolists comments create " + list.ID + " <text|->", Description: "Comment on the list"},
		{Action: "series", Cmd: "minka todos series list " + list.ID, Description: "List the series that repeat to-dos of the list"},
		{Action: "lists", Cmd: "minka todolists list " + list.ToolID, Description: "List the lists of the tool"},
	}
}
