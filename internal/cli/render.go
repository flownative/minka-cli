package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
)

// The text output reads like Markdown, which suits a terminal as well as an
// agent that asked for text.

func renderBoard(w io.Writer, board api.Board) {
	fmt.Fprintf(w, "# %s\n", board.Title)
	fmt.Fprintf(w, "Board: %s\nProject: %s\nURL: %s\n", board.ID, board.ProjectID, board.AppURL)

	for _, column := range board.Columns {
		count := plural(column.CardCount, "card", "cards")
		if len(column.Cards) < column.CardCount {
			count += fmt.Sprintf(", the %d completed last are listed", len(column.Cards))
		}
		fmt.Fprintf(w, "\n## %s\n", column.Title)
		fmt.Fprintf(w, "Column: %s (%s, %s)\n", column.ID, column.Type, count)
		if len(column.Cards) == 0 {
			fmt.Fprintln(w, "(no cards)")
			continue
		}
		for _, card := range column.Cards {
			var details []string
			if card.HasDescription {
				details = append(details, "description")
			}
			if card.CommentCount > 0 {
				details = append(details, plural(card.CommentCount, "comment", "comments"))
			}
			if card.StepCount > 0 {
				details = append(details, fmt.Sprintf("steps %d/%d", card.CompletedStepCount, card.StepCount))
			}
			if len(card.Assignees) > 0 {
				details = append(details, "assigned to "+assigneeNames(card.Assignees))
			}
			if card.DueOn != nil {
				details = append(details, "due "+*card.DueOn)
			}
			if card.CompletedAt != nil {
				details = append(details, "completed "+formatTime(*card.CompletedAt))
			}
			line := fmt.Sprintf("- %s  %s", card.ID, card.Title)
			if len(details) > 0 {
				line += " (" + strings.Join(details, ", ") + ")"
			}
			fmt.Fprintln(w, line)
		}
	}
}

func renderCard(w io.Writer, card api.Card) {
	fmt.Fprintf(w, "# %s\n", card.Title)
	fmt.Fprintf(w, "Card: %s\nBoard: %s\nProject: %s\n", card.ID, card.BoardID, card.ProjectID)
	fmt.Fprintf(w, "Column: %s (%s, %s)\n", card.Column.Title, card.Column.Type, card.Column.ID)
	fmt.Fprintf(w, "Created: %s by %s\n", formatTime(card.CreatedAt), card.Creator.Name)
	if len(card.Assignees) > 0 {
		fmt.Fprintf(w, "Assigned to: %s\n", assigneeNames(card.Assignees))
	}
	if len(card.Subscribers) > 0 {
		fmt.Fprintf(w, "Subscribers: %s\n", assigneeNames(card.Subscribers))
	}
	if card.DueOn != nil {
		fmt.Fprintf(w, "Due: %s\n", *card.DueOn)
	}
	if card.CompletedAt != nil {
		by := ""
		if card.CompletedBy != nil {
			by = " by " + card.CompletedBy.Name
		}
		fmt.Fprintf(w, "Completed: %s%s\n", formatTime(*card.CompletedAt), by)
	}
	fmt.Fprintf(w, "URL: %s\n", card.AppURL)

	if len(card.Steps) > 0 {
		fmt.Fprintf(w, "\n## Steps (%d/%d)\n\n", completedSteps(card.Steps), len(card.Steps))
		for i, step := range card.Steps {
			fmt.Fprintln(w, stepLine(i, step))
		}
	}

	if description := richtext.ToMarkdown(firstNonEmpty(card.DescriptionSource, card.Description)); description != "" {
		fmt.Fprintf(w, "\n## Description\n\n%s\n", description)
	}

	if len(card.Comments) > 0 {
		fmt.Fprintf(w, "\n## Comments (%d)\n", len(card.Comments))
		for _, comment := range card.Comments {
			edited := ""
			if comment.EditedAt != nil {
				edited = ", edited"
			}
			body := firstNonEmpty(comment.BodySource, comment.Body)
			if richtext.CarriesMarkup(body) {
				body = richtext.ToMarkdown(body)
			}
			fmt.Fprintf(w, "\n### %s, %s%s\n\n%s\n", comment.Author.Name, formatTime(comment.CreatedAt), edited, body)
		}
	}
}

func cardBreadcrumbs(cardID string) []Breadcrumb {
	return []Breadcrumb{
		{Action: "show", Cmd: "minka cards show " + cardID, Description: "Show the card with description and comments"},
		{Action: "move", Cmd: "minka cards move " + cardID + " --column <column>", Description: "Move the card to another column"},
		{Action: "done", Cmd: "minka cards done " + cardID, Description: "Complete the card"},
		{Action: "comment", Cmd: "minka comments create " + cardID + " <text|->", Description: "Comment on the card"},
	}
}

// formatTime shows an API timestamp in local time, to the minute.
func formatTime(value string) string {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return t.Local().Format("2006-01-02 15:04")
}

// firstNonEmpty prefers the source of a text, which carries the addresses of
// its files, and falls back to the stored text for a server that sends none.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
