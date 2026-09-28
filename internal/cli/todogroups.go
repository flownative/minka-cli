package cli

import (
	"fmt"
	"io"

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

func (a *app) newTodoGroupsCreateCmd() *cobra.Command {
	var listID, title string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a group at the end of a to-do list",
		Long: `Adds a named group at the end of a list. A list in the archive takes no new
group: exit 9 (validation) with meta.apiCode todoListArchived.

` + todoGroupsReadHelp,
		Example: `  minka todogroups create --list 0199... --title "Stage" --json`,
		Args:    requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--list", listID); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			group, err := s.client.CreateTodoGroup(a.ctx, listID, title)
			if err != nil {
				return todoListRefusal(s, err, listID)
			}

			summary := "Added group " + group.Value.Title + " to list " + group.Value.ListID
			return a.ok(success{
				data:        group.Raw,
				summary:     summary,
				breadcrumbs: todoGroupBreadcrumbs(group.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nGroup: %s\n", summary, group.Value.ID)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&listID, "list", "", "ID of the list the group goes into")
	flags.StringVar(&title, "title", "", "Title of the group")

	return cmd
}

func (a *app) newTodoGroupsEditCmd() *cobra.Command {
	var title string

	cmd := &cobra.Command{
		Use:   "edit <groupId>",
		Short: "Rename a group of a to-do list",
		Long: `Gives a group a new title, which is the one thing of a group that changes. An
empty title is refused with exit 9 (validation) and meta.apiCode
todoGroupTitleMissing: a title can only be replaced.

` + todoGroupsReadHelp,
		Example: `  minka todogroups edit 0199... --title "Stage and light"`,
		Args:    requireArgs("groupId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "nothing to change, give --title")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			group, err := s.client.RenameTodoGroup(a.ctx, args[0], title)
			if err != nil {
				return todoGroupRefusal(s, err)
			}

			summary := "Renamed group " + group.Value.ID + " to " + group.Value.Title
			return a.ok(success{
				data:        group.Raw,
				summary:     summary,
				breadcrumbs: todoGroupBreadcrumbs(group.Value),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "New title of the group")

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
			neighbour, flag := after, "--after"
			if before != "" {
				neighbour, flag = before, "--before"
			}
			if neighbour != "" {
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
	case e.APICode == apiCodeTodoListArchived:
		e.Hint = "The list of the group lies in the archive of its tool, and nothing in it changes until it is back: minka todolists unarchive <listId>"
	case e.Code == CodeNotFound:
		e.Hint = todoGroupNotFoundHint
	}
	return e
}

func todoGroupBreadcrumbs(group api.TodoGroup) []Breadcrumb {
	return []Breadcrumb{
		{Action: "show", Cmd: "minka todolists show " + group.ListID, Description: "Read the list with its groups in their order"},
		{Action: "move", Cmd: "minka todogroups move " + group.ID + " --after <groupId>", Description: "Put the group elsewhere in its list"},
	}
}
