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
		Long: `Lists the cards, columns and tools that were taken out of a project, the newest
first, at most 200 of them.

Only a card comes back, with cards restore, and only while neither its column
nor its board is in the trash: restorable says whether it does, blockedBy which
of the two is in the way. Columns and tools come back in the web only.`,
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

// whereItHung is the board and the column a trashed thing belonged to, as far
// as it had one.
func whereItHung(entry api.TrashEntry) string {
	var parts []string
	if entry.Board != nil {
		parts = append(parts, entry.Board.Title)
	}
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
		return "no, its " + *entry.BlockedBy + " is in the trash"
	default:
		return "in the web only"
	}
}
