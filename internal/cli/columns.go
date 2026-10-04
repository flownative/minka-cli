package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *app) newColumnsCmd() *cobra.Command {
	return group("columns", "Add, rename, colour, move and delete the columns of a board",
		a.newColumnsCreateCmd(),
		a.newColumnsEditCmd(),
		a.newColumnsMoveCmd(),
		a.newColumnsDeleteCmd(),
		a.newColumnsOnHoldCmd(),
	)
}

// columnArgHelp says how the column a command writes to is named.
const columnArgHelp = `The column is given by its ID. With --board it may also be given by its type
(triage, not_now, done) or by its title, such as "Doing".`

// columnColorHelp says which colours a column takes.
const columnColorHelp = `--color takes one of white, red, orange, yellow, green, blue, aqua, purple,
gray, pink and brown, spelt exactly so. Any other value, the empty one
included, is refused with exit 9 (validation) and meta.apiCode
columnColorUnknown. --no-color takes the colour away: the column is white then,
which is how a column without a colour looks, the same as with --color white.
A Minka that does not take a colour away yet refuses --no-color alone with
nothingToChange and keeps the colour beside --title: exit 7 (api_error), the
title written.
Triage and Not now carry no colour, Done is always green, and none of the three
takes one or gives one up: exit 9 with systemColumnHasNoColor.`

// columnWritersHelp says who may change the columns of a board.
const columnWritersHelp = `Only the team of a project changes its columns: a client of the project gets
exit 2 (not_found) like somebody outside it.`

const (
	apiCodeColumnTitleMissing            = "columnTitleMissing"
	apiCodeColumnTitleTooLong            = "columnTitleTooLong"
	apiCodeColumnTitleHasControlChars    = "columnTitleHasControlCharacters"
	apiCodeColumnColorUnknown            = "columnColorUnknown"
	apiCodeSystemColumnHasNoColor        = "systemColumnHasNoColor"
	apiCodeSystemColumnCannotMove        = "systemColumnCannotMove"
	apiCodeSystemColumnCannotBeRemoved   = "systemColumnCannotBeRemoved"
	apiCodeNeighbourColumnOnAnotherBoard = "neighbourColumnOnAnotherBoard"
	apiCodeNothingToChange               = "nothingToChange"
	columnWhiteInstead                   = "--color white gives the column the same look on any Minka."
	columnNotFoundHint                   = "It does not exist, lies in the trash, or belongs to a project you are not on the team of."
	columnBoardNotFoundHint              = "The board does not exist, lies in the trash, or belongs to a project you are not on the team of."
	columnBoardNotReadHint               = "The board does not exist, lies in the trash, or belongs to a project you are not in."
	columnSystemColumnsKeepTheirPlaces   = "Triage, Not now and Done keep their places; only a working column moves."
	columnSystemColumnsBelongToTheBoard  = "Triage, Not now and Done belong to every board and cannot be deleted."
)

// columnRefusal classifies a failed write on a column and gives the hint that
// names the way on.
func columnRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodeColumnTitleMissing:
		e.Hint = "A working column needs a title. Only Triage, Not now and Done take an empty one, which gives them back the name of their type."
	case apiCodeColumnTitleTooLong:
		e.Hint = "The title of a column takes at most 200 characters."
	case apiCodeColumnTitleHasControlChars:
		e.Hint = "The title of a column is one line: no line breaks, tabs or other control characters."
	case apiCodeColumnColorUnknown:
		e.Hint = "A column takes one of " + strings.Join(minkaColors, ", ") + ", spelt exactly so."
	case apiCodeSystemColumnHasNoColor:
		e.Hint = "Triage and Not now carry no colour and Done is always green. Nothing was changed, the title included: rename it without --color or --no-color."
	case apiCodeSystemColumnCannotMove:
		e.Hint = columnSystemColumnsKeepTheirPlaces
	case apiCodeSystemColumnCannotBeRemoved:
		e.Hint = columnSystemColumnsBelongToTheBoard
	case apiCodeNeighbourColumnOnAnotherBoard:
		e.Hint = "A column moves only among the working columns of its own board, which \"minka boards show <boardId>\" lists; or give --first."
	default:
		if e.Code == CodeNotFound {
			e.Hint = columnNotFoundHint
		}
	}
	return e
}

// checkColumnArg checks the column and the board a command is given before
// anything is asked: without a board the column has to be its ID.
func checkColumnArg(cmd *cobra.Command, boardID *string, column string) error {
	if *boardID != "" {
		return requireID(cmd, "--board", boardID)
	}
	if _, isID := asID(column); !isID {
		return usageErrorf(cmd, "column %q is not an ID; give --board to name a column by its type or title", column)
	}
	return nil
}

// lookUpColumn reads the board where one is given and finds the column on it;
// without a board the column is its ID and nothing is read.
func (a *app) lookUpColumn(s *session, boardID, column string) (string, *api.Board, *api.Column, error) {
	if boardID == "" {
		id, _ := asID(column)
		return id, nil, nil, nil
	}
	board, err := s.client.Board(a.ctx, boardID)
	if err != nil {
		e := s.fail(err)
		if e.Code == CodeNotFound {
			e.Hint = columnBoardNotReadHint
		}
		return "", nil, nil, e
	}
	target, err := resolveColumn(board.Value, column)
	if err != nil {
		return "", nil, nil, err
	}
	return target.ID, &board.Value, &target, nil
}

func columnBreadcrumbs(columnID, boardID string) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "edit", Cmd: "minka columns edit " + columnID + " --title <title>", Description: "Rename the column"},
	}
	if boardID != "" {
		crumbs = append(crumbs, Breadcrumb{Action: "show", Cmd: "minka boards show " + boardID, Description: "Show the board with its columns and cards"})
	}
	return crumbs
}

// columnColor names the colour of a column in words. White is how a column
// without a colour looks, and the words say so.
func columnColor(column api.Column) string {
	switch {
	case column.Color == nil:
		return "none"
	case *column.Color == "white":
		return "white, which is no colour"
	}
	return *column.Color
}

func (a *app) newColumnsCreateCmd() *cobra.Command {
	var boardID, title string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a working column behind the others of a board",
		Long: `Adds a working column behind the working columns of a board, before Not now and
Done. Minka gives it the colour the board holds least of; "minka columns edit
<columnId> --color <colour>" chooses another. A column cannot be created in
another place or colour in one step: Minka takes the title alone.

An empty title exits 9 (validation) with meta.apiCode columnTitleMissing,
columnTitleTooLong stands for more than 200 characters and
columnTitleHasControlCharacters for a line break or a tab.

` + columnWritersHelp,
		Example: `  minka columns create --board 0199... --title "Review" --json`,
		Args:    requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--board", &boardID); err != nil {
				return err
			}
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "a column needs --title")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			column, err := s.client.CreateColumn(a.ctx, boardID, title)
			if err != nil {
				e := columnRefusal(s, err)
				if e.Code == CodeNotFound {
					e.Hint = columnBoardNotFoundHint
				}
				return e
			}

			summary := "Added column " + column.Value.Title + " in " + columnColor(column.Value)
			return a.ok(success{
				data:        column.Raw,
				summary:     summary,
				breadcrumbs: columnBreadcrumbs(column.Value.ID, boardID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nColumn: %s\n", summary, column.Value.ID)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&boardID, "board", "", "ID of the board the column goes on")
	flags.StringVar(&title, "title", "", "Title of the column")

	return cmd
}

func (a *app) newColumnsEditCmd() *cobra.Command {
	var boardID, title, color string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "edit <column>",
		Short: "Rename or recolour a column",
		Long: `Gives a column a new title, a new colour or both; at least one of --title,
--color and --no-color is required, and --color and --no-color exclude each
other. What is not given stays as it is.

Triage, Not now and Done take a title of their own as well, and --title ""
gives them back the name of their type. A working column cannot go without a
title: --title "" exits 9 (validation) with meta.apiCode columnTitleMissing.
columnTitleTooLong stands for more than 200 characters and
columnTitleHasControlCharacters for a line break or a tab.

` + columnColorHelp + ` Where both are given, the colour is
written first, and a refused colour leaves the title as it was.

` + columnArgHelp + `

` + columnWritersHelp,
		Example: `  minka columns edit 0199... --title "In review"
  minka columns edit Doing --board 0199... --color purple
  minka columns edit Doing --board 0199... --no-color
  minka columns edit triage --board 0199... --title "Inbox"
  minka columns edit triage --board 0199... --title ""`,
		Args: requireArgs("column"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if noColor && cmd.Flags().Changed("color") {
				return usageErrorf(cmd, "--color and --no-color exclude each other")
			}
			input := api.UpdateColumnInput{NoColor: noColor}
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("color") {
				input.Color = &color
			}
			if input.Title == nil && input.Color == nil && !input.NoColor {
				return usageErrorf(cmd, "nothing to change, give --title, --color or --no-color")
			}
			if err := checkColumnArg(cmd, &boardID, args[0]); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			columnID, _, _, err := a.lookUpColumn(s, boardID, args[0])
			if err != nil {
				return err
			}
			column, err := s.client.UpdateColumn(a.ctx, columnID, input)
			if err != nil {
				e := columnRefusal(s, err)
				// A Minka that does not take a colour away yet reads null as
				// no colour named at all.
				if input.NoColor && e.APICode == apiCodeNothingToChange {
					e.Hint = "This Minka does not take a colour away yet and read --no-color as no change. " + columnWhiteInstead
				}
				return e
			}
			// Such a Minka writes the title sent along and keeps the colour,
			// with a 200 that would otherwise pass for success.
			if input.NoColor && deref(column.Value.Color) != "white" {
				message := "this Minka does not take a colour away yet: the column kept its colour " + columnColor(column.Value)
				if input.Title != nil {
					message += ", and the title was written"
				}
				return &Error{Code: CodeAPI, Message: message, Hint: columnWhiteInstead, Data: column.Raw}
			}

			summary := "Changed column " + column.Value.ID + ": " + column.Value.Title + ", colour " + columnColor(column.Value)
			return a.ok(success{
				data:        column.Raw,
				summary:     summary,
				breadcrumbs: columnBreadcrumbs(column.Value.ID, boardID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&boardID, "board", "", "ID of the board, to give the column by its type or title")
	flags.StringVar(&title, "title", "", `New title of the column; "" gives a system column the name of its type back`)
	flags.StringVar(&color, "color", "", "New colour of a working column, one of Minka's eleven (see above)")
	flags.BoolVar(&noColor, "no-color", false, "Take the colour off a working column; it is white then")

	return cmd
}

func (a *app) newColumnsMoveCmd() *cobra.Command {
	var boardID, after, before string
	var first bool

	cmd := &cobra.Command{
		Use:   "move <column>",
		Short: "Move a working column to another place on its board",
		Long: `Puts a working column elsewhere among the working columns of its board:
--first puts it right behind Triage, --after behind the column named, --before
in front of it. Exactly one of the three is required, and so is --board.

The column named with --after or --before has to be another working column of
the same board. Minka would place a column next to Triage, Not now, Done or
itself as though --first had been given, without a word, so the board is read
first and any other neighbour exits 1 (usage) before anything moves. Triage,
Not now and Done keep their places: moving one exits 9 (validation) with
meta.apiCode systemColumnCannotMove. A column that stands where it is asked to
go already stays there, and the answer is the same.

The column and its neighbour are given by their ID, their type or their title,
such as "Doing".

` + columnWritersHelp,
		Example: `  minka columns move Review --board 0199... --after Doing
  minka columns move 0199... --board 0199... --first`,
		Args: requireArgs("column"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{first, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the column goes: give exactly one of --first, --after and --before")
			}
			if err := requireID(cmd, "--board", &boardID); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			columnID, board, _, err := a.lookUpColumn(s, boardID, args[0])
			if err != nil {
				return err
			}
			var input api.MoveColumnInput
			for _, place := range []struct {
				flag, value string
				id          *string
			}{{"--after", after, &input.PreviousColumnID}, {"--before", before, &input.NextColumnID}} {
				if place.value == "" {
					continue
				}
				neighbour, err := resolveColumn(*board, place.value)
				if err != nil {
					return err
				}
				// Minka passes over a system column or the column itself as
				// its neighbour and puts the column first instead.
				if neighbour.Type != api.ColumnWork || neighbour.ID == columnID {
					e := usageErrorf(cmd, "%s %s is no other working column of board %s", place.flag, neighbour.Title, board.Title)
					e.Hint = placesAmongWorkingColumns(*board, columnID)
					return e
				}
				*place.id = neighbour.ID
			}

			column, err := s.client.MoveColumn(a.ctx, columnID, input)
			if err != nil {
				return columnRefusal(s, err)
			}

			summary := "Moved column " + column.Value.Title
			return a.ok(success{
				data:        column.Raw,
				summary:     summary,
				breadcrumbs: columnBreadcrumbs(column.Value.ID, boardID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&boardID, "board", "", "ID of the board the column stands on")
	flags.BoolVar(&first, "first", false, "Put the column first among the working columns")
	flags.StringVar(&after, "after", "", "Put the column behind this working column of the same board")
	flags.StringVar(&before, "before", "", "Put the column in front of this working column of the same board")

	return cmd
}

// placesAmongWorkingColumns names the places a column can be moved to: the
// front, and the end behind the last other working column of the board.
func placesAmongWorkingColumns(board api.Board, columnID string) string {
	hint := "Give --first to put the column at the front of the working columns"
	var last *api.Column
	for i, column := range board.Columns {
		if column.Type == api.ColumnWork && column.ID != columnID {
			last = &board.Columns[i]
		}
	}
	if last == nil {
		return hint + "; it is the only one."
	}
	return hint + `, or --after "` + last.Title + `" (` + last.ID + `) to put it at the end.`
}

func (a *app) newColumnsDeleteCmd() *cobra.Command {
	var boardID string

	cmd := &cobra.Command{
		Use:     "delete <column>",
		Aliases: []string{"trash"},
		Short:   "Move a working column to the trash, its cards with it",
		Long: `Moves a working column into the trash of its project, and its cards go with it:
none of the cards standing in it can be read, moved or restored on its own
until the column is back. A card of the column in the archive of the board
stays readable and comes back into the first working column. Deleting it again exits 2 (not_found). "trash list <projectId>" shows
the column, and it comes back from there in Minka on the web; the API brings
no column back. Triage, Not now and Done cannot be deleted: exit 9
(validation) with meta.apiCode systemColumnCannotBeRemoved.

` + columnArgHelp + `

` + columnWritersHelp,
		Example: `  minka columns delete 0199...
  minka columns delete Review --board 0199... --json`,
		Args: requireArgs("column"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkColumnArg(cmd, &boardID, args[0]); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			columnID, _, target, err := a.lookUpColumn(s, boardID, args[0])
			if err != nil {
				return err
			}
			if err := s.client.DeleteColumn(a.ctx, columnID); err != nil {
				return columnRefusal(s, err)
			}

			summary := "Moved column " + columnID + " to the trash, its cards with it"
			if target != nil {
				cards := plural(target.CardCount+target.OnHoldCardCount, "card", "cards")
				summary = "Moved column " + target.Title + " to the trash, with its " + cards
			}
			var breadcrumbs []Breadcrumb
			if boardID != "" {
				breadcrumbs = append(breadcrumbs, Breadcrumb{Action: "show", Cmd: "minka boards show " + boardID, Description: "Show the board with its columns and cards"})
			}
			return a.ok(success{
				data:        map[string]any{"id": columnID, "deleted": true},
				summary:     summary,
				breadcrumbs: breadcrumbs,
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "ID of the board, to give the column by its type or title")

	return cmd
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

` + columnArgHelp,
		Example: `  minka columns on-hold 0199...
  minka columns on-hold Doing --board 0199...
  minka columns on-hold Doing --board 0199... --off`,
		Args: requireArgs("column"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkColumnArg(cmd, &boardID, args[0]); err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			column, _, _, err := a.lookUpColumn(s, boardID, args[0])
			if err != nil {
				return err
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

	cmd.Flags().StringVar(&boardID, "board", "", "ID of the board, to give the column by its type or title")
	cmd.Flags().BoolVar(&off, "off", false, "Switch the area off and hand its cards back to the column")

	return cmd
}
