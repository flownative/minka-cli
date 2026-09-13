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
					if me.Value.Principal != nil {
						fmt.Fprintf(w, "Acting for: %s <%s>\n", me.Value.Principal.Name, me.Value.Principal.EmailAddress)
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
	return &cobra.Command{
		Use:   "list",
		Short: "List the projects you are a member of",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			projects, err := s.client.Projects(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    projects.Raw,
				summary: plural(len(projects.Value), "project", "projects"),
				breadcrumbs: []Breadcrumb{
					{Action: "boards", Cmd: "minka boards list <projectId>", Description: "List the boards of a project"},
				},
				text: func(w io.Writer) {
					if len(projects.Value) == 0 {
						fmt.Fprintln(w, "No projects.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tNAME")
					for _, project := range projects.Value {
						fmt.Fprintf(table, "%s\t%s\n", project.ID, project.Name)
					}
					_ = table.Flush()
				},
			})
		},
	}
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
lists only the 25 cards completed last; its cardCount counts all of them.`,
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
