package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

const columnHelp = `A column is given by its ID, by its type (triage, not_now, done) or by its
title, such as "Doing".`

const descriptionHelp = `The description is Markdown by default and is sent as HTML. Minka keeps
headings, bold, italic, strikethrough, inline code, links, lists, quotes, code
blocks with their language, tables and horizontal rules. Images become links
and links Minka does not keep become plain text, each with a warning.
Use --description - to read it from stdin.`

func (a *app) newCardsCmd() *cobra.Command {
	return group("cards", "Show, create, edit, move, assign, complete and delete cards",
		a.newCardsMineCmd(),
		a.newCardsShowCmd(),
		a.newCardsCreateCmd(),
		a.newCardsEditCmd(),
		a.newCardsMoveCmd(),
		a.newCardsAssignCmd(),
		a.newCardsUnassignCmd(),
		a.newCardsSubscribeCmd(),
		a.newCardsUnsubscribeCmd(),
		a.newCardsDoneCmd(),
		a.newCardsDeleteCmd(),
		a.newCardsRestoreCmd(),
	)
}

func (a *app) newCardsMineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mine",
		Short: "List the cards you are responsible for, across all projects",
		Long: `Lists the cards the account of the token is assigned to, in every project it is
a member of. Cards in Done, cards in the trash and cards of a project that was
left are not among them.

The order is the one the web page has: projects by name, and inside a project
the cards by due date, the overdue ones first and those without a date last.
Every card names the project and the board it hangs on.`,
		Args: requireArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			cards, err := s.client.MyCards(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    cards.Raw,
				summary: plural(len(cards.Value), "card", "cards"),
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: "minka cards show <cardId>", Description: "Read a card with its description and comments"},
					{Action: "done", Cmd: "minka cards done <cardId>", Description: "Move a card to Done"},
				},
				text: func(w io.Writer) {
					renderMyCards(w, cards.Value, time.Now().Format(dayFormat))
				},
			})
		},
	}
}

func (a *app) newCardsRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <cardId>",
		Short: "Take a card out of the trash",
		Long: `Takes a card out of the trash and puts it back on top of the column it hung in.

Only a card that is in the trash can be restored. A card that was never there,
a card restored already and a card whose board or project is itself in the
trash all answer with 2 (not_found), and so does a second restore.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.RestoreCard(a.ctx, args[0])
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "The card is not in the trash, or its board or project is. A card can only be restored once."
				}
				return e
			}

			summary := fmt.Sprintf("Restored card %s to %s", card.Value.Title, card.Value.Column.Title)
			return a.ok(success{
				data:        card.Raw,
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n", summary, card.Value.AppURL)
				},
			})
		},
	}
}

func (a *app) newCardsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <cardId>",
		Short: "Show a card with its description and comments",
		Long: `Shows a card with its column, description and comments. In text output the
description is converted to Markdown, which cards edit --description turns back
into the same HTML; in JSON it is the HTML the server stores.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        card.Raw,
				summary:     fmt.Sprintf("Card %s in %s", card.Value.Title, card.Value.Column.Title),
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text:        func(w io.Writer) { renderCard(w, card.Value) },
			})
		},
	}
}

func (a *app) newCardsCreateCmd() *cobra.Command {
	var boardID, column, title, description, descriptionFormat, due string
	var assignees []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a card on a board",
		Long: "Creates a card in Triage, or in the column given with --column.\n" +
			"Not now and Done take no new cards. A new card goes to the top of its column.\n\n" +
			columnHelp + "\n\n" + descriptionHelp + "\n\n" +
			"--assignee assigns a member of the project and can be given more than once.\n" +
			"The card is created together with its people, or not at all. " + personHelp + "\n\n" +
			"--due sets the day the card is due, as YYYY-MM-DD.",
		Example: `  minka cards create --board 0199... --title "Fix the login"
  minka cards create --board 0199... --column Doing --title "Review" --description - < notes.md`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--board", boardID); err != nil {
				return err
			}
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}

			input := api.CreateCardInput{Title: title}
			var notices []string
			if cmd.Flags().Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description = &html
				notices = warnings
			}

			s, err := a.session()
			if err != nil {
				return err
			}

			var board *api.Board
			if cmd.Flags().Changed("column") || len(assignees) > 0 {
				result, err := s.client.Board(a.ctx, boardID)
				if err != nil {
					return s.fail(err)
				}
				board = &result.Value
			}

			// A name that matches nobody is refused before a card exists.
			people, err := a.resolveMembers(s, board, assignees)
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("column") {
				target, err := resolveColumn(*board, column)
				if err != nil {
					return err
				}
				if target.Type == api.ColumnNotNow || target.Type == api.ColumnDone {
					return &Error{
						Code:    CodeUsage,
						Message: fmt.Sprintf("cards cannot be created in %s", target.Title),
						Hint:    "Create the card in Triage or a working column, then move it with: minka cards move <cardId> --column " + target.ID,
					}
				}
				input.ColumnID = &target.ID
			}

			for _, person := range people {
				input.AssigneeIDs = append(input.AssigneeIDs, person.ID)
			}
			if cmd.Flags().Changed("due") {
				input.DueOn = &due
			}

			card, err := s.client.CreateCard(a.ctx, boardID, input)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        card.Raw,
				summary:     fmt.Sprintf("Created card %s in %s", card.Value.Title, card.Value.Column.Title),
				notices:     notices,
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Created card %s in %s: %s%s\n%s\n", card.Value.ID, card.Value.Column.Title, shortIDBefore(card.Value.ShortID), card.Value.Title, card.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&boardID, "board", "", "ID of the board")
	flags.StringVar(&title, "title", "", "Title of the card")
	flags.StringVar(&column, "column", "", "Column for the card (default: Triage)")
	flags.StringArrayVar(&assignees, "assignee", nil, "Person to assign: ID, me, e-mail address or name (repeatable)")
	flags.StringVar(&due, "due", "", "Day the card is due, as YYYY-MM-DD")
	flags.StringVar(&description, "description", "", "Description, or - to read it from stdin")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("title")

	return cmd
}

func (a *app) newCardsEditCmd() *cobra.Command {
	var title, description, descriptionFormat, due string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <cardId>",
		Short: "Rename a card, change its description or its due date",
		Long: "Changes the title, the description, the due date or several of them. An\n" +
			"empty description or due date removes it. A due date is written as YYYY-MM-DD.\n\n" +
			descriptionHelp + "\n\n" +
			"Files and mentions stand in a description as action-text-attachment elements,\n" +
			"which Markdown cannot hold. To keep them, take descriptionSource from\n" +
			"cards show --json, change it and send it back with --description-format html.\n" +
			"A new description that would drop them is refused unless --drop-attachments\n" +
			"is given.",
		Example: `  minka cards edit 0199... --title "Fix the login form"
  minka cards edit 0199... --description - < notes.md
  minka cards edit 0199... --description ""`,
		Args: requireArgs("cardId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}

			var input api.UpdateCardInput
			var notices []string
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description = &html
				notices = warnings
			}
			if cmd.Flags().Changed("due") {
				input.DueOn = &due
			}
			if input.Title == nil && input.Description == nil && input.DueOn == nil {
				return usageErrorf(cmd, "nothing to change, give --title, --description or --due")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			if input.Description != nil {
				current, err := s.client.Card(a.ctx, args[0])
				if err != nil {
					return s.fail(err)
				}
				// Files and mentions stand in the text as elements Markdown cannot
				// hold, so only a description that carries them keeps them.
				existing := firstNonEmpty(current.Value.DescriptionSource, current.Value.Description)
				if richtext.HasAttachments(existing) && !richtext.HasAttachments(*input.Description) {
					if !dropAttachments {
						return &Error{
							Code:    CodeUsage,
							Message: "the description has attachments or mentions, and the new description would drop them",
							Hint:    "Change descriptionSource from cards show --json and send it back with --description-format html, or give --drop-attachments.",
						}
					}
					notices = append(notices, "the attachments and mentions of the description were dropped")
				}
			}
			card, err := s.client.UpdateCard(a.ctx, args[0], input)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        card.Raw,
				summary:     fmt.Sprintf("Updated card %s", card.Value.Title),
				notices:     notices,
				breadcrumbs: cardBreadcrumbs(card.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Updated card %s: %s\n%s\n", card.Value.ID, card.Value.Title, card.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title")
	flags.StringVar(&description, "description", "", "New description, - to read it from stdin, empty to remove it")
	flags.StringVar(&due, "due", "", "Day the card is due as YYYY-MM-DD, empty to remove it")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a description that has attachments, which drops them")

	return cmd
}

func (a *app) newCardsMoveCmd() *cobra.Command {
	var column, after, before string
	var top, bottom bool

	cmd := &cobra.Command{
		Use:   "move <cardId>",
		Short: "Move a card to a column of its board",
		Long: `Moves a card within its board. Without a placement it goes to the top of the
column. Moving into Done completes the card, moving out of Done reopens it.
Done is ordered by completion, so it takes no placement.

` + columnHelp,
		Example: `  minka cards move 0199... --column Doing --bottom
  minka cards move 0199... --column Doing --after 0199...
  minka cards move 0199... --column triage`,
		Args: requireArgs("cardId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{top, bottom, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements > 1 {
				return usageErrorf(cmd, "give only one of --top, --bottom, --after and --before")
			}
			neighbour := after
			if neighbour == "" {
				neighbour = before
			}
			if neighbour != "" {
				if err := requireID(cmd, neighbourCard, neighbour); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			if neighbour != "" {
				if err := s.resolveCard(a.ctx, &neighbour); err != nil {
					return err
				}
				if neighbour == args[0] {
					return usageErrorf(cmd, "a card cannot be placed next to itself")
				}
				if after != "" {
					after = neighbour
				} else {
					before = neighbour
				}
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			board, err := s.client.Board(a.ctx, card.Value.BoardID)
			if err != nil {
				return s.fail(err)
			}
			target, err := resolveColumn(board.Value, column)
			if err != nil {
				return err
			}

			if target.Type == api.ColumnDone && (bottom || neighbour != "") {
				return &Error{
					Code:    CodeUsage,
					Message: "Done is ordered by completion and takes no placement",
					Hint:    "Leave out --bottom, --after and --before, or run: minka cards done " + card.Value.ID,
				}
			}
			// The server puts a card whose neighbour is not in the column at the
			// top without saying so.
			if neighbour != "" && !columnHasCard(target, neighbour) {
				return usageErrorf(cmd, "card %s is not in column %s", neighbour, target.Title)
			}

			input := api.MoveCardInput{ColumnID: target.ID}
			switch {
			case top:
				input.Position = "top"
			case bottom:
				input.Position = "bottom"
			case after != "":
				input.PreviousCardID = after
			case before != "":
				input.NextCardID = before
			}

			moved, err := s.client.MoveCard(a.ctx, card.Value.ID, input)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        moved.Raw,
				summary:     fmt.Sprintf("Moved card %s to %s", moved.Value.Title, moved.Value.Column.Title),
				breadcrumbs: cardBreadcrumbs(moved.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Moved card %s to %s: %s\n%s\n", moved.Value.ID, moved.Value.Column.Title, moved.Value.Title, moved.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&column, "column", "", "Target column")
	flags.BoolVar(&top, "top", false, "Put the card at the top of the column")
	flags.BoolVar(&bottom, "bottom", false, "Put the card at the bottom of the column")
	flags.StringVar(&after, "after", "", "Put the card below this card")
	flags.StringVar(&before, "before", "", "Put the card above this card")
	_ = cmd.MarkFlagRequired("column")

	return cmd
}

func (a *app) newCardsDoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <cardId>",
		Short: "Complete a card by moving it to Done",
		Long: `Moves a card into the Done column of its board, which completes it. A card that
is already done stays as it is. To reopen a card, move it out of Done.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			if card.Value.Column.Type == api.ColumnDone {
				return a.ok(success{
					data:        card.Raw,
					summary:     fmt.Sprintf("Card %s is already done", card.Value.Title),
					breadcrumbs: cardBreadcrumbs(card.Value.ID),
					text: func(w io.Writer) {
						fmt.Fprintf(w, "Card %s is already done: %s\n%s\n", card.Value.ID, card.Value.Title, card.Value.AppURL)
					},
				})
			}

			board, err := s.client.Board(a.ctx, card.Value.BoardID)
			if err != nil {
				return s.fail(err)
			}
			var done *api.Column
			for i := range board.Value.Columns {
				if board.Value.Columns[i].Type == api.ColumnDone {
					done = &board.Value.Columns[i]
				}
			}
			if done == nil {
				return &Error{Code: CodeAPI, Message: fmt.Sprintf("board %s has no Done column", board.Value.Title)}
			}

			moved, err := s.client.MoveCard(a.ctx, card.Value.ID, api.MoveCardInput{ColumnID: done.ID, Position: "top"})
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        moved.Raw,
				summary:     fmt.Sprintf("Completed card %s", moved.Value.Title),
				breadcrumbs: cardBreadcrumbs(moved.Value.ID),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Completed card %s: %s\n%s\n", moved.Value.ID, moved.Value.Title, moved.Value.AppURL)
				},
			})
		},
	}
}

func (a *app) newCardsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <cardId>",
		Aliases: []string{"trash"},
		Short:   "Move a card to the trash",
		Long: `Moves a card to the trash, as removing it on the web does.

A card in the trash reads like one that never existed. Showing, moving,
commenting on or deleting it again exits with 2 (not_found). Deleting twice
leaves the card in the trash all the same; only the second exit code differs.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			if err := s.client.DeleteCard(a.ctx, args[0]); err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "It does not exist, is already in the trash, or belongs to a project you are not a member of."
				}
				return e
			}

			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: "Moved card " + args[0] + " to the trash",
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Moved card %s to the trash.\n", args[0])
				},
			})
		},
	}
}

func checkDescriptionFormat(cmd *cobra.Command, format string) error {
	if format != "markdown" && format != "html" {
		return usageErrorf(cmd, "--description-format is markdown or html, not %q", format)
	}
	return nil
}

// descriptionHTML reads and converts a description. Markdown that holds text
// but nothing Minka keeps is refused, because sending the empty result would
// remove the description.
func (a *app) descriptionHTML(cmd *cobra.Command, value, format string) (string, []string, error) {
	text, err := a.readValue("the description", value)
	if err != nil {
		return "", nil, err
	}
	if format == "html" {
		return text, nil, nil
	}

	result := markdown.ToHTML(text)
	if result.HTML == "" && strings.TrimSpace(text) != "" {
		return "", nil, &Error{
			Code:    CodeUsage,
			Message: "the description has nothing Minka keeps and would remove the current description",
			Hint:    `To remove the description on purpose, give --description "".`,
		}
	}
	return result.HTML, result.Warnings, nil
}

// resolveColumn finds a column of a board by ID, type or title. A value that
// names a type and also the title of another column is ambiguous.
func resolveColumn(board api.Board, value string) (api.Column, error) {
	for _, column := range board.Columns {
		if column.ID == value {
			return column, nil
		}
	}

	wanted := strings.TrimSpace(value)
	kind := strings.ReplaceAll(strings.ToLower(wanted), "-", "_")
	var matches []api.Column
	for _, column := range board.Columns {
		namesType := kind != api.ColumnWork && column.Type == kind
		if namesType || strings.EqualFold(column.Title, wanted) {
			matches = append(matches, column)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	describe := func(columns []api.Column) string {
		var names []string
		for _, column := range columns {
			names = append(names, fmt.Sprintf("%s (%s, %s)", column.Title, column.Type, column.ID))
		}
		return strings.Join(names, "; ")
	}
	if len(matches) > 1 {
		return api.Column{}, &Error{
			Code:    CodeAmbiguous,
			Message: fmt.Sprintf("%q matches %d columns of board %s", value, len(matches), board.Title),
			Hint:    "Give the column ID. Candidates: " + describe(matches),
		}
	}

	return api.Column{}, &Error{
		Code:    CodeUsage,
		Message: fmt.Sprintf("board %s has no column %q", board.Title, value),
		Hint:    "Columns: " + describe(board.Columns),
	}
}

func columnHasCard(column api.Column, cardID string) bool {
	for _, card := range column.Cards {
		if card.ID == cardID {
			return true
		}
	}
	return false
}
