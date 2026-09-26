package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

func (a *app) newTrashCmd() *cobra.Command {
	return group("trash", "See what was taken out of a project", a.newTrashListCmd())
}

func (a *app) newTrashListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <projectId>",
		Short: "List what is in the trash of a project, the newest first",
		Long: `Lists what was taken out of a project, the newest first, at most 200 of them.
Anything a project holds can be there: a card, a column, a tool, a post, a
to-do with its list and its group, a question or an answer of a check-in, a
forward, and a folder, a document or a file of Docs & Files.

WHERE says what the thing hung in. Only a card and a column lie on a board;
everything else belongs to a tool of another sort, which is marked as a tool.

Only a card comes back through this CLI, with cards restore, and only while
nothing it hung in is in the trash: restorable says whether it does, blockedBy
what is in the way. Everything else comes back in the web only.`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			trash, err := s.client.Trash(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			var notices []string
			if trash.Header.Get("X-Minka-Truncated") == "true" {
				notices = append(notices, "the trash holds more than these; only the newest 200 are listed")
			}

			return a.ok(success{
				data:    trash.Raw,
				summary: plural(len(trash.Value), "entry", "entries"),
				notices: notices,
				breadcrumbs: []Breadcrumb{
					{Action: "restore", Cmd: "minka cards restore <cardId>", Description: "Put a card back where it was"},
				},
				text: func(w io.Writer) {
					if len(trash.Value) == 0 {
						fmt.Fprintln(w, "The trash is empty.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "KIND\tID\tTITLE\tWHERE\tTRASHED\tRESTORE")
					for _, entry := range trash.Value {
						kind := entry.Kind
						if entry.ToolType != nil {
							kind = *entry.ToolType
						}
						fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
							kind, entry.ID, entry.Title, whereItHung(entry), whenItWent(entry), howItComesBack(entry))
					}
					_ = table.Flush()
				},
			})
		},
	}
}

// whereItHung is what a trashed thing belonged to, as far as it belonged to
// anything. Only a card and a column lie on a board; for every other kind the
// same field carries the tool it hung in, and it is named as a tool so that no
// line reads as though a board had been emptied.
func whereItHung(entry api.TrashEntry) string {
	if entry.Board == nil {
		return ""
	}
	if entry.Kind != api.TrashCard && entry.Kind != api.TrashColumn {
		return entry.Board.Title + " (tool)"
	}
	parts := []string{entry.Board.Title}
	if entry.Column != nil {
		parts = append(parts, entry.Column.Title)
	}
	return strings.Join(parts, " / ")
}

// whenItWent is when somebody took it out, and who did. Rows from before
// Minka noted either say nothing.
func whenItWent(entry api.TrashEntry) string {
	when := ""
	if entry.TrashedAt != nil {
		when = formatTime(*entry.TrashedAt)
	}
	if entry.TrashedBy != nil {
		when = strings.TrimSpace(when + " by " + entry.TrashedBy.Name)
	}
	return when
}

func howItComesBack(entry api.TrashEntry) string {
	switch {
	case entry.Restorable:
		return "yes"
	case entry.BlockedBy != nil:
		return "no, " + whatIsInTheWay(*entry.BlockedBy)
	default:
		return "in the web only"
	}
}

// whatIsInTheWay says what has to come back before this thing can. A word this
// CLI does not know is passed on as it came and says no more than that it is in
// the way, because where a kind Minka gains lies is not something to guess.
func whatIsInTheWay(blocker string) string {
	switch blocker {
	case "board", "column", "tool", "folder":
		return "its " + blocker + " is in the trash"
	case "todoList":
		return "its to-do list is in the trash"
	case "todoGroup":
		return "its group is in the trash"
	case "question":
		return "its check-in question is in the trash"
	case "archivedFolder":
		return "its folder is in the archive"
	default:
		return blocker + " is in the way"
	}
}
