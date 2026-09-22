package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func (a *app) newMeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the person the access token acts as",
		Long: `Shows the person the access token acts as. For an agent account, principal
is the person the agent acts for.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			me, err := s.client.Me(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    me.Raw,
				summary: fmt.Sprintf("%s <%s>", me.Value.Name, me.Value.EmailAddress),
				breadcrumbs: []Breadcrumb{
					{Action: "projects", Cmd: "minka projects list", Description: "List your projects"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s <%s>\nID: %s\n", me.Value.Name, me.Value.EmailAddress, me.Value.ID)
					if me.Value.OrganizationRole != nil {
						fmt.Fprintf(w, "Organization role: %s\n", *me.Value.OrganizationRole)
					}
					if me.Value.Kind != "" && me.Value.Kind != "human" {
						kind := me.Value.Kind
						if me.Value.Retired {
							kind += ", retired"
						}
						fmt.Fprintf(w, "Kind: %s\n", kind)
					}
					if me.Value.Principal != nil {
						fmt.Fprintf(w, "Acting for: %s\n", me.Value.Principal.Name)
					}
				},
			})
		},
	}
}

func (a *app) newProjectsCmd() *cobra.Command {
	return group("projects", "List projects", a.newProjectsListCmd())
}

func (a *app) newProjectsListCmd() *cobra.Command {
	var archived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the projects you are a member of",
		Long: `Lists the projects in use that you are a member of. Every project carries
its status, "active" or "archived".

--archived lists the archived ones instead. An archived project can still be
read, but nothing is written in it: every write exits 9 with meta.apiCode
projectArchived. Its cards are not among "cards mine".`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			projects, err := s.client.Projects(a.ctx, archived)
			if err != nil {
				return s.fail(err)
			}

			// A host that does not know the filter ignores it and lists the
			// projects in use, which must not be passed off as archived.
			if archived {
				for _, project := range projects.Value {
					if project.Status != "archived" {
						return &Error{
							Code:    CodeAPI,
							Message: s.host + " does not tell archived projects apart yet",
							Hint:    "Its Minka is older than this minka expects and may not know archived projects yet.",
						}
					}
				}
			}

			singular, several, none := "project", "projects", "No projects."
			if archived {
				singular, several, none = "archived project", "archived projects", "No archived projects."
			}
			return a.ok(success{
				data:    projects.Raw,
				summary: plural(len(projects.Value), singular, several),
				breadcrumbs: []Breadcrumb{
					{Action: "boards", Cmd: "minka boards list <projectId>", Description: "List the boards of a project"},
				},
				text: func(w io.Writer) {
					if len(projects.Value) == 0 {
						fmt.Fprintln(w, none)
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tNAME\tSTATUS")
					for _, project := range projects.Value {
						fmt.Fprintf(table, "%s\t%s\t%s\n", project.ID, project.Name, project.Status)
					}
					_ = table.Flush()
				},
			})
		},
	}
	cmd.Flags().BoolVar(&archived, "archived", false, "List the archived projects instead of the ones in use")
	return cmd
}

func (a *app) newBoardsCmd() *cobra.Command {
	return group("boards", "List and show boards", a.newBoardsListCmd(), a.newBoardsShowCmd())
}

func (a *app) newBoardsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <projectId>",
		Short: "List the boards of a project",
		Args:  requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			boards, err := s.client.Boards(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    boards.Raw,
				summary: plural(len(boards.Value), "board", "boards"),
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka boards show <boardId>", Description: "Show a board with its columns and cards"},
				},
				text: func(w io.Writer) {
					if len(boards.Value) == 0 {
						fmt.Fprintln(w, "No boards.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tTITLE")
					for _, board := range boards.Value {
						fmt.Fprintf(table, "%s\t%s\n", board.ID, board.Title)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newBoardsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <boardId>",
		Short: "Show a board with its columns and cards",
		Long: `Shows the columns of a board in board order, each with its cards.

Column types are triage, column (a working column), not_now and done. Done
lists only the 25 cards completed last; its cardCount counts all of them.

A working column with an area for cards on hold lists them apart, in
onHoldCards with their own count onHoldCardCount; cards and cardCount leave
them out.`,
		Args: requireArgs("boardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			board, err := s.client.Board(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    board.Raw,
				summary: fmt.Sprintf("Board %s with %s", board.Value.Title, plural(len(board.Value.Columns), "column", "columns")),
				breadcrumbs: []Breadcrumb{
					{Action: "create", Cmd: "minka cards create --board " + board.Value.ID + " --title <title>", Description: "Create a card in Triage"},
					{Action: "show", Cmd: "minka cards show <cardId>", Description: "Show a card with description and comments"},
				},
				text: func(w io.Writer) { renderBoard(w, board.Value) },
			})
		},
	}
}
