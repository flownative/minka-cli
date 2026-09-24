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
		fmt.Fprintln(w)
		renderColumn(w, column)
	}
}

// renderColumn lists the cards of a column and, where it has the area, the
// cards on hold below them.
func renderColumn(w io.Writer, column api.Column) {
	count := plural(column.CardCount, "card", "cards")
	if len(column.Cards) < column.CardCount {
		count += fmt.Sprintf(", the %d completed last are listed", len(column.Cards))
	}
	if column.OnHoldEnabled {
		count += fmt.Sprintf(", %d on hold", column.OnHoldCardCount)
	}
	fmt.Fprintf(w, "## %s\n", column.Title)
	fmt.Fprintf(w, "Column: %s (%s, %s)\n", column.ID, column.Type, count)
	renderBoardCards(w, column.Cards)
	if column.OnHoldEnabled {
		fmt.Fprintf(w, "\n### On hold in %s\n", column.Title)
		renderBoardCards(w, column.OnHoldCards)
	}
}

func renderBoardCards(w io.Writer, cards []api.BoardCard) {
	if len(cards) == 0 {
		fmt.Fprintln(w, "(no cards)")
		return
	}
	for _, card := range cards {
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
		line := fmt.Sprintf("- %s  %s%s", card.ID, shortIDBefore(card.ShortID), card.Title)
		if len(details) > 0 {
			line += " (" + strings.Join(details, ", ") + ")"
		}
		fmt.Fprintln(w, line)
	}
}

// dayFormat is the way Minka writes a day.
const dayFormat = "2006-01-02"

// renderMyCards lists cards of several projects, in the order the server sent
// them, under a heading for every board they hang on.
func renderMyCards(w io.Writer, cards []api.Card, today string) {
	if len(cards) == 0 {
		fmt.Fprintln(w, "No cards.")
		return
	}

	heading := ""
	for _, card := range cards {
		if where := card.Project.Name + " / " + card.Board.Title; where != heading {
			if heading != "" {
				fmt.Fprintln(w)
			}
			heading = where
			fmt.Fprintf(w, "## %s\n", where)
		}
		details := []string{card.Column.Title}
		if card.OnHold {
			details = append(details, "on hold")
		}
		if card.DueOn != nil {
			due := "due " + *card.DueOn
			if *card.DueOn < today {
				due += ", overdue"
			}
			details = append(details, due)
		}
		if card.CommentCount > 0 {
			details = append(details, plural(card.CommentCount, "comment", "comments"))
		}
		if open := openSteps(card.Steps); open > 0 {
			details = append(details, fmt.Sprintf("steps %d/%d", len(card.Steps)-open, len(card.Steps)))
		}
		fmt.Fprintf(w, "- %s  %s%s (%s)\n", card.ID, shortIDBefore(card.ShortID), card.Title, strings.Join(details, ", "))
	}
}

func openSteps(steps []api.Step) int {
	open := 0
	for _, step := range steps {
		if !step.Completed {
			open++
		}
	}
	return open
}

func renderCard(w io.Writer, card api.Card) {
	// The heading stays the bare title, so that somebody who takes it as the
	// new title of the card does not write the short identifier into it.
	fmt.Fprintf(w, "# %s\n", card.Title)
	if card.ShortID != nil {
		fmt.Fprintf(w, "Short ID: %s\n", *card.ShortID)
	}
	fmt.Fprintf(w, "Card: %s\nBoard: %s\nProject: %s\n", card.ID, card.BoardID, card.ProjectID)
	fmt.Fprintf(w, "Column: %s (%s, %s)\n", card.Column.Title, card.Column.Type, card.Column.ID)
	if card.OnHold {
		fmt.Fprintln(w, "On hold: yes")
	}
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

	if len(card.Attachments) > 0 {
		fmt.Fprintf(w, "\n## Files (%d)\n\n", len(card.Attachments))
		for _, file := range card.Attachments {
			fmt.Fprintf(w, "- %s  %s (%s, %s)\n", file.ID, file.Filename, file.ContentType, fileSize(file.ByteSize))
		}
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
			if line := fileLine(comment.Attachments); line != "" {
				fmt.Fprintf(w, "%s\n", line)
			}
			if line := reactionLine(comment.Reactions); line != "" {
				fmt.Fprintf(w, "%s\n", line)
			}
		}
	}
}

// fileLine names the files hanging in a comment or a message, in one line,
// and nothing at all where none hang in it. The identifier is there so that
// somebody who reads the text can fetch the file with "uploads download".
func fileLine(attachments []api.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}
	parts := make([]string, len(attachments))
	for i, file := range attachments {
		parts[i] = fmt.Sprintf("%s (%s, %s, %s)", file.Filename, file.ContentType, fileSize(file.ByteSize), file.ID)
	}
	return "Files: " + strings.Join(parts, "; ")
}

// reactionLine says what people said about a comment or a message, in one
// line, and nothing at all where nobody said anything.
func reactionLine(reactions []api.Reaction) string {
	if len(reactions) == 0 {
		return ""
	}
	parts := make([]string, len(reactions))
	for i, reaction := range reactions {
		names := make([]string, len(reaction.People))
		for j, person := range reaction.People {
			names[j] = person.Name
		}
		parts[i] = fmt.Sprintf("%s %s", reaction.Content, strings.Join(names, ", "))
	}
	return "Reactions: " + strings.Join(parts, "; ")
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

// shortIDBefore puts the short identifier of a card in front of its title in
// a list. It is what people call a card by; commands still take the ID,
// because the API finds a card by nothing else.
func shortIDBefore(shortID *string) string {
	if shortID == nil {
		return ""
	}
	return *shortID + "  "
}
