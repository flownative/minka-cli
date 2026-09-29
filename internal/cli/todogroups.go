package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *app) newTodoGroupsCmd() *cobra.Command {
	return group("todogroups", "Add, rename, move and trash the groups of a to-do list",
		a.newTodoGroupsCreateCmd(),
		a.newTodoGroupsEditCmd(),
		a.newTodoGroupsMoveCmd(),
		a.newTodoGroupsDeleteCmd(),
	)
}

// todoGroupsReadHelp says where a group is read, since it has no address of
// its own to read it at.
const todoGroupsReadHelp = `A group is read with its list: "todolists show <listId>" names every group
in its order, with its ID and its open to-dos. A group of a list in the
archive is at no address until the list is back, so every command here exits
2 (not_found) on it.`

// todoGroupColors are the colours Minka takes for a group, those of a board
// column. The CLI only names them: refusing any other is left to Minka.
var todoGroupColors = []string{"white", "red", "orange", "yellow", "green", "blue", "aqua", "purple", "gray", "pink", "brown"}

// todoGroupColorHelp says which colours a group takes.
const todoGroupColorHelp = `--color takes one of white, red, orange, yellow, green, blue, aqua, purple,
gray, pink and brown, spelt exactly so. White is no colour: it takes the one a
group has away. Any other value, the empty one included, is refused with exit
9 (validation) and meta.apiCode todoGroupColorUnknown.`

// todoGroupColorUnknownHint names what Minka takes as the colour of a group.
var todoGroupColorUnknownHint = "A group takes one of " + strings.Join(todoGroupColors, ", ") + ", spelt exactly so; white takes the colour away."

const apiCodeTodoGroupColorUnknown = "todoGroupColorUnknown"

func (a *app) newTodoGroupsCreateCmd() *cobra.Command {
	var listID, title, color string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a group at the end of a to-do list",
		Long: `Adds a named group at the end of a list, without a colour unless --color
names one. A list in the archive takes no new group: exit 9 (validation) with
meta.apiCode todoListArchived.

` + todoGroupColorHelp + `

` + todoGroupsReadHelp,
		Example: `  minka todogroups create --list 0199... --title "Stage" --json
  minka todogroups create --list 0199... --title "Stage" --color blue`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--list", &listID); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			input := api.CreateTodoGroupInput{Title: title}
			if cmd.Flags().Changed("color") {
				input.Color = &color
			}
			group, err := s.client.CreateTodoGroup(a.ctx, listID, input)
			if err != nil {
				e := todoListRefusal(s, err, listID)
				if e.APICode == apiCodeTodoGroupColorUnknown {
					e.Hint = todoGroupColorUnknownHint
				}
				return e
			}

			summary := "Added group " + group.Value.Title + " to list " + group.Value.ListID
			return a.ok(success{
				data:        group.Raw,
				summary:     summary,
				breadcrumbs: todoGroupBreadcrumbs(group.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nGroup: %s\nColour: %s\n", summary, group.Value.ID, todoGroupColor(group.Value))
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&listID, "list", "", "ID of the list the group goes into")
	flags.StringVar(&title, "title", "", "Title of the group")
	flags.StringVar(&color, "color", "", "Colour of the group, one of Minka's eleven (see above)")

	return cmd
}

func (a *app) newTodoGroupsEditCmd() *cobra.Command {
	var title, color string

	cmd := &cobra.Command{
		Use:   "edit <groupId>",
		Short: "Rename or recolour a group of a to-do list",
		Long: `Gives a group a new title, a new colour or both; at least one of --title and
--color is required. What both name is written together or not at all: a
refusal of either leaves the group as it was. An empty title is refused with
exit 9 (validation) and meta.apiCode todoGroupTitleMissing: a title can only
be replaced.

` + todoGroupColorHelp + `

` + todoGroupsReadHelp,
		Example: `  minka todogroups edit 0199... --title "Stage and light"
  minka todogroups edit 0199... --color green
  minka todogroups edit 0199... --color white`,
		Args: requireArgs("groupId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			var input api.UpdateTodoGroupInput
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("color") {
				input.Color = &color
			}
			if input.Title == nil && input.Color == nil {
				return usageErrorf(cmd, "nothing to change, give --title, --color or both")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			group, err := s.client.UpdateTodoGroup(a.ctx, args[0], input)
			if err != nil {
				return todoGroupRefusal(s, err)
			}

			summary := "Changed group " + group.Value.ID + ": " + group.Value.Title + ", colour " + todoGroupColor(group.Value)
			return a.ok(success{
				data:        group.Raw,
				summary:     summary,
				breadcrumbs: todoGroupBreadcrumbs(group.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title of the group")
	flags.StringVar(&color, "color", "", "New colour of the group, one of Minka's eleven; white takes it away")

	return cmd
}

func (a *app) newTodoGroupsMoveCmd() *cobra.Command {
	var after, before string
	var top bool

	cmd := &cobra.Command{
		Use:   "move <groupId>",
		Short: "Move a group to another place in its to-do list",
		Long: `Puts a group elsewhere among the groups of its list: --top puts it first,
--after below the group named, --before above it. Exactly one of the three is
required. A group never leaves its list.

The group named with --after or --before has to be another group of the same
list, out of the trash; any other, the group itself included, is refused with
exit 9 (validation) and meta.apiCode todoPlaceUnknown, and nothing moves.

` + todoGroupsReadHelp,
		Example: `  minka todogroups move 0199... --top
  minka todogroups move 0199... --after 0199...`,
		Args: requireArgs("groupId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the group goes: give exactly one of --top, --after and --before")
			}
			neighbour, flag := &after, "--after"
			if before != "" {
				neighbour, flag = &before, "--before"
			}
			if *neighbour != "" {
				if err := requireID(cmd, flag, neighbour); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			group, err := s.client.MoveTodoGroup(a.ctx, args[0], api.MoveTodoGroupInput{PreviousGroupID: after, NextGroupID: before})
			if err != nil {
				return todoGroupRefusal(s, err)
			}

			summary := "Moved group " + group.Value.Title
			return a.ok(success{
				data:        group.Raw,
				summary:     summary,
				breadcrumbs: todoGroupBreadcrumbs(group.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&top, "top", false, "Put the group first in its list")
	flags.StringVar(&after, "after", "", "Put the group below this group of the same list")
	flags.StringVar(&before, "before", "", "Put the group above this group of the same list")

	return cmd
}

func (a *app) newTodoGroupsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <groupId>",
		Aliases: []string{"trash"},
		Short:   "Move a group of a to-do list to the trash",
		Long: `Moves a group into the trash of its project, and its to-dos with it: they leave
the list and its counts. Deleting it again exits with 2 (not_found).
"trash list <projectId>" shows it, and it comes back from there in Minka on
the web.

` + todoGroupsReadHelp,
		Args: requireArgs("groupId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteTodoGroup(a.ctx, args[0]); err != nil {
				return todoGroupRefusal(s, err)
			}

			summary := "Moved group " + args[0] + " to the trash"
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

// todoGroupRefusal classifies a failed write on a group. A group has no
// address to read it at, so where the hint cannot name its list it names the
// command that finds it.
func todoGroupRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeTodoPlaceUnknown:
		e.Hint = "The group named with --after or --before is the group itself, no group of the same list, or in the trash. The groups of a list, in their order: minka todolists show <listId>"
	case e.APICode == apiCodeTodoGroupColorUnknown:
		e.Hint = todoGroupColorUnknownHint
	case e.APICode == apiCodeTodoListArchived:
		e.Hint = "The list of the group lies in the archive of its tool, and nothing in it changes until it is back: minka todolists unarchive <listId>"
	case e.Code == CodeNotFound:
		e.Hint = todoGroupNotFoundHint
	}
	return e
}

// todoGroupColor names the colour of a group in words.
func todoGroupColor(group api.TodoGroup) string {
	if group.Color == nil {
		return "none"
	}
	return *group.Color
}

func todoGroupBreadcrumbs(group api.TodoGroup) []Breadcrumb {
	return []Breadcrumb{
		{Action: "show", Cmd: "minka todolists show " + group.ListID, Description: "Read the list with its groups in their order"},
		{Action: "move", Cmd: "minka todogroups move " + group.ID + " --after <groupId>", Description: "Put the group elsewhere in its list"},
	}
}
