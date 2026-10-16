package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const bookNotFoundHint = "The folder stands in no book, there is no such folder, it or a folder above it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. A folder becomes a book with: minka docs folders edit <folderId> --book"

func (a *app) newDocsBookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "book <folderId>",
		Short: "Read a book as one tree of chapters and pages",
		Long: `Reads the book a folder belongs to — the folder itself, or the one above it
that is marked as a book — as one tree: its chapters, which are its folders,
and its pages, which are its documents, at any depth and in reading order. Any
folder of the book can be named. A folder in no book exits 2 (not_found).

Every chapter and the book say their order, automatic or manual, and a page
left out of the navigation of the book says so: it is still reached by link,
reference and search, and only previous and next pass it over. There are no
bodies here, no uploads, and nothing that lies in the archive; a book that lies
in the archive itself is read all the same. Nothing is paged.

"docs show <documentId>" reads a page with its body. "docs edit <documentId>
--not-in-navigation" leaves a page out of the navigation, and "docs move ...
--after <entryId>" changes the order.`,
		Example: `  minka docs book 0199...
  minka docs book 0199... --json`,
		Args: requireArgs("folderId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			book, err := s.client.BookOf(a.ctx, args[0])
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = bookNotFoundHint
				}
				return e
			}

			chapters, pages := countBook(book.Value.Entries)
			return a.ok(success{
				data:        book.Raw,
				summary:     fmt.Sprintf("Book %s: %s and %s", book.Value.Title, plural(chapters, "chapter", "chapters"), plural(pages, "page", "pages")),
				breadcrumbs: bookBreadcrumbs(book.Value),
				text:        func(w io.Writer) { renderBook(w, book.Value) },
			})
		},
	}
}

func (a *app) newDocsOrderCmd() *cobra.Command {
	var folderID string

	cmd := &cobra.Command{
		Use:   "order <toolId> <automatic|manual> [--folder <folderId>]",
		Short: "Order the top of a Docs & Files tool, or one of its folders, automatically or by hand",
		Long: `Sets how the top of a tool, or the folder --folder names, orders what stands in
it, and answers with it as "docs list" reads it.

automatic shows folders first, then documents, then uploads, each newest
first. manual keeps the order shown a moment before and from then on mixes the
three kinds: an entry made in the place or moved into it goes to the end, and
"docs move <entryId> --folder <folderId> --after <entryId>" or --before places
one. Turning back to automatic brings the automatic order back unchanged. A
place that orders so already is answered the same.

Each place has an order of its own: a folder does not take the order of the
one above it. Marking a folder as a book arranges it and every folder in it by
hand.

Only the team chooses the order: a client of the project exits 2 (not_found).
An archived folder keeps its order until it is back (folderArchived).`,
		Example: `  minka docs order 0199... manual --json
  minka docs order 0199... automatic --folder 0199... --json`,
		Args: requireArgs("toolId", "order"),
		RunE: func(cmd *cobra.Command, args []string) error {
			order := args[1]
			if order != orderAutomatic && order != orderManual {
				return usageErrorf(cmd, "the order is automatic or manual, not %q", order)
			}
			if cmd.Flags().Changed("folder") {
				if err := requireID(cmd, "--folder", &folderID); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			entries, err := s.client.ChangeFilesOrder(a.ctx, args[0], api.ChangeFilesOrderInput{Order: order, FolderID: folderID})
			if err != nil {
				e := documentRefusal(s, err)
				switch {
				case e.APICode == apiCodeFolderArchived:
					e.Hint = "The folder, or one above it, lies in the archive and keeps its order until it is back: minka docs folders unarchive <folderId>"
				case e.APICode == apiCodeOrderUnknown:
					e.Hint = "This CLI sends only automatic or manual, so the Minka it talks to reads the request otherwise."
				case e.Code == CodeNotFound:
					e.Hint = filesToolNotFoundHint + " Only the team chooses the order: a client of the project meets the same."
				}
				return e
			}

			summary := whereTheEntriesStand(entries.Value) + " orders its entries automatically"
			if entries.Value.Order == orderManual {
				summary = whereTheEntriesStand(entries.Value) + " is arranged by hand"
			}
			return a.ok(success{
				data:        entries.Raw,
				summary:     summary,
				breadcrumbs: entriesBreadcrumbs(args[0], entries.Value),
				text:        func(w io.Writer) { renderFilesEntries(w, entries.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&folderID, "folder", "", "ID of the folder to order; the top of the tool without it")

	return cmd
}

func renderBook(w io.Writer, book api.Book) {
	fmt.Fprintf(w, "# %s\n", book.Title)
	fmt.Fprintf(w, "Book: %s\nTool: %s (%s)\nOrder: %s\nURL: %s\n\n", book.ID, book.Tool.Title, book.Tool.ID, book.Order, book.AppURL)
	if len(book.Entries) == 0 {
		fmt.Fprintln(w, "(no chapters and no pages)")
		return
	}
	renderBookEntries(w, book.Entries, 0)
}

// renderBookEntries writes one line per chapter and page in the shape of a
// line of "docs list", indented by depth. A chapter ends in a slash, as a
// folder does in a listing of files.
func renderBookEntries(w io.Writer, entries []api.BookEntry, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, entry := range entries {
		if entry.Kind != "chapter" && entry.Kind != "page" {
			continue // a kind a later Minka knows; --json carries it
		}
		if entry.Kind == "chapter" {
			details := ""
			if entry.Order == orderAutomatic {
				details = " (ordered automatically)"
			}
			fmt.Fprintf(w, "%s- %s  %s/%s\n", indent, entry.ID, entry.Title, details)
			renderBookEntries(w, entry.Entries, depth+1)
			continue
		}
		details := ""
		if !entry.InNavigation {
			details = " (not in navigation)"
		}
		fmt.Fprintf(w, "%s- %s  %s%s\n", indent, entry.ID, entry.Title, details)
	}
}

// refuseAnExplicitFalse refuses a switch given as --name=false, which would
// otherwise read as not given at all and change nothing without a word.
func refuseAnExplicitFalse(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if on, _ := cmd.Flags().GetBool(name); cmd.Flags().Changed(name) && !on {
			return usageErrorf(cmd, "--%s takes no value: leave it out, or give the flag that says the opposite", name)
		}
	}
	return nil
}

// booksUnknown answers a 200 that does not carry what was asked about a book.
// A Minka without books answers so: it takes the request, writes what it
// knows and leaves the rest out. The answer it gave goes along as data.
func booksUnknown(what string, restAsked bool, raw any) *Error {
	message := "the answer does not carry " + what + " asked for: this Minka may not know books"
	if restAsked {
		message += ", and the rest of the change went through"
	}
	return &Error{
		Code:    CodeAPI,
		Message: message,
		Hint:    "Books need a Minka that serves them; the answer is in data. Read the entry back, and ask whoever runs the host which Minka it runs.",
		Data:    raw,
	}
}

func countBook(entries []api.BookEntry) (chapters, pages int) {
	for _, entry := range entries {
		switch entry.Kind {
		case "chapter":
			c, p := countBook(entry.Entries)
			chapters, pages = chapters+1+c, pages+p
		case "page":
			pages++
		}
	}
	return chapters, pages
}

func bookBreadcrumbs(book api.Book) []Breadcrumb {
	return []Breadcrumb{
		{Action: "show", Cmd: "minka docs show <documentId>", Description: "Read a page with its body"},
		{Action: "list", Cmd: "minka docs list " + book.Tool.ID + " --folder " + book.ID, Description: "List what stands in the book, uploads included"},
		{Action: "navigation", Cmd: "minka docs edit <documentId> --not-in-navigation", Description: "Leave a page out of the navigation of the book"},
		{Action: "place", Cmd: "minka docs move <documentId> --folder <chapterId> --after <entryId>", Description: "Put a page at another place in the book"},
	}
}
