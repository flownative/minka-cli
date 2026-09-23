package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func (a *app) newColumnsCmd() *cobra.Command {
	return group("columns", "Switch the area for cards on hold of a column", a.newColumnsOnHoldCmd())
}

func (a *app) newColumnsOnHoldCmd() *cobra.Command {
	var boardID string
	var off bool

	cmd := &cobra.Command{
		Use:   "on-hold <column>",
		Short: "Switch the area for cards on hold of a column on or off",
		Long: `Gives a working column an area for the cards that are waiting, or with --off
takes it away. Triage, Not now and Done cannot have one.

A card on hold stays in its column, apart from the rest and in an order of its
own; "minka cards move --on-hold" puts it there. Switching the area off hands
its cards back to the end of the column, in the order they stood on hold.
Switching it on or off a second time changes nothing.

The column is given by its ID. With --board it may also be given by its title,
such as "Doing".`,
		Example: `  minka columns on-hold 0199...
  minka columns on-hold Doing --board 0199...
  minka columns on-hold Doing --board 0199... --off`,
		Args: requireArgs("column"),
		RunE: func(cmd *cobra.Command, args []string) error {
			column := args[0]
			if boardID != "" {
				if err := requireID(cmd, "--board", boardID); err != nil {
					return err
				}
			} else if !uuidPattern.MatchString(column) {
				return usageErrorf(cmd, "column %q is not an ID; give --board to name a column by its title", column)
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if boardID != "" {
				board, err := s.client.Board(a.ctx, boardID)
				if err != nil {
					return s.fail(err)
				}
				target, err := resolveColumn(board.Value, column)
				if err != nil {
					return err
				}
				column = target.ID
			}

			result, err := s.client.SetColumnOnHold(a.ctx, column, !off)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "It does not exist or belongs to a project you are not a member of."
				}
				return e
			}

			state := "on"
			if off {
				state = "off"
			}
			summary := fmt.Sprintf("Switched the area on hold of %s %s", result.Value.Title, state)
			var breadcrumbs []Breadcrumb
			if !off {
				breadcrumbs = append(breadcrumbs, Breadcrumb{Action: "hold", Cmd: "minka cards move <cardId> --column " + result.Value.ID + " --on-hold", Description: "Put a card on hold in this column"})
			}
			if boardID != "" {
				breadcrumbs = append(breadcrumbs, Breadcrumb{Action: "show", Cmd: "minka boards show " + boardID, Description: "Show the board with its columns and cards"})
			}
			return a.ok(success{
				data:        result.Raw,
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					renderColumn(w, result.Value)
				},
			})
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "ID of the board, to give the column by its title")
	cmd.Flags().BoolVar(&off, "off", false, "Switch the area off and hand its cards back to the column")

	return cmd
}
