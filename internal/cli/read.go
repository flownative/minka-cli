package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
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
	return group("boards", "List and show boards, and read the archive of one",
		a.newBoardsListCmd(),
		a.newBoardsShowCmd(),
		a.newBoardsArchiveCmd(),
	)
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

func (a *app) newBoardsArchiveCmd() *cobra.Command {
	var before string
	var limit int
	var all bool

	cmd := &cobra.Command{
		Use:   "archive <boardId>",
		Short: "List the cards in the archive of a board, the one put away last first",
		Long: fmt.Sprintf(`Lists the cards a board has put away, the one put away last first. Each says
its id, its short identifier, its title, when and by whom it was put away, and
its address. Read the card itself with "cards show <cardId>", which works for a
card in an archive like any other.

A page holds %d cards by default and at most %d, which is the largest page the
server serves. A larger --limit is refused here: the server would cut it down
without a word, and the only sign its answer has for the end of the archive is
a page shorter than asked for, so a page cut down would read as the end and a
hundred cards would pass for the whole archive.

--before <cardId> gives the page behind that card. The breadcrumbs of a page
name the next one, so nothing has to be worked out.

--all reads every page to the end of the archive in one run. Then every card
that lay in the archive for the whole run is listed exactly once. What is not
promised is the archive at one moment: a card put away during the run lands on
the first page, which the run has already passed, and a card taken out of the
archive vanishes from the run, and the API says nothing about either. A run of
more than one page says so in a notice.

An unknown board, or one of a project you are not a member of, exits 2
(not_found). A --before that names no card in the archive of this board exits 9
(validation) with meta.apiCode archivedCardsBeforeUnknown: somebody took that
card back onto the board or into the trash. A run that ends that way hands over
what it read before it, with its notices.`, api.ArchivePageDefault, api.ArchivePageMax),
		Example: `  minka boards archive 0199...
  minka boards archive 0199... --limit 100 --all
  minka boards archive 0199... --before 0199... --limit 25`,
		Args: requireArgs("boardId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The bound holds before a request goes out, so that the page the
			// server cuts down in silence is one this CLI cannot ask for.
			if limit < 1 || limit > api.ArchivePageMax {
				return usageErrorf(cmd, "--limit is a number of cards between 1 and %d, not %d; a larger page is cut down by the server without a word, and the end of the archive would then be read where there is none", api.ArchivePageMax, limit)
			}
			if cmd.Flags().Changed("before") {
				if err := requireID(cmd, "--before", before); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if all {
				return a.wholeArchive(s, args[0], before, limit)
			}

			page, err := s.client.ArchivedCards(a.ctx, args[0], before, limit)
			if err != nil {
				return archiveRefusal(s, err, args[0])
			}

			cards := page.Value
			return a.ok(success{
				data:        page.Raw,
				summary:     plural(len(cards), "card", "cards") + " in the archive",
				breadcrumbs: archiveBreadcrumbs(args[0], cards, limit),
				text:        func(w io.Writer) { renderArchivedCards(w, cards) },
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&before, "before", "", "ID of the card the page starts behind")
	flags.IntVar(&limit, "limit", api.ArchivePageDefault, fmt.Sprintf("Cards per page, at most %d", api.ArchivePageMax))
	flags.BoolVar(&all, "all", false, "Read every page to the end of the archive")

	return cmd
}

// wholeArchive walks the archive of a board to its end. A run that breaks off
// hands over the cards it read all the same, with the exit code saying that
// something is missing from them.
func (a *app) wholeArchive(s *session, boardID, before string, pageSize int) error {
	run, err := s.client.WholeArchiveOf(a.ctx, boardID, before, pageSize)
	notices := noticesOfTheRun(run)
	raw := run.Raw
	if raw == nil {
		raw = []json.RawMessage{}
	}
	text := func(w io.Writer) { renderArchivedCards(w, run.Cards) }

	if err != nil {
		e := archiveRefusal(s, err, boardID)
		e.Data = raw
		e.Notices = append(notices, fmt.Sprintf("the run broke off after %s, and these are listed", plural(len(run.Cards), "card", "cards")))
		e.Text = text
		return e
	}

	summary := plural(len(run.Cards), "card", "cards") + " in the archive"
	if run.Pages > 1 {
		summary += fmt.Sprintf(", read in %d pages", run.Pages)
	}
	return a.ok(success{
		data:        raw,
		summary:     summary,
		notices:     notices,
		breadcrumbs: archiveBreadcrumbs(boardID, nil, pageSize),
		text:        text,
	})
}

// noticesOfTheRun says what a walk of several pages promises and what it does
// not. The address has no word for the end of the list but a page shorter than
// asked for, and none at all for what changed while the walk was going on, so a
// reader is told instead of left to assume.
func noticesOfTheRun(run api.ArchiveRun) []string {
	var notices []string
	if run.Pages > 1 {
		notices = append(notices, fmt.Sprintf("read in %d pages: every card that lay in the archive for the whole run is listed once, but the list is no picture of one moment — a card put away during the run lands on the first page, which was already read, and a card taken out of the archive left the run without a trace", run.Pages))
	}
	if run.Duplicates > 0 {
		notices = append(notices, fmt.Sprintf("%s came a second time, because a page had to start behind an earlier card of the page before it; each card is listed once", plural(run.Duplicates, "card", "cards")))
	}
	if run.BeganAgain {
		notices = append(notices, "no card of a page was left in the archive for the next one to start behind, so the run began again where it started")
	}
	return notices
}

// archiveBreadcrumbs name what commonly follows a page of an archive. The next
// page is named only where one was read that could have more behind it, so that
// nobody is sent after a page that is not there.
func archiveBreadcrumbs(boardID string, cards []api.ArchivedCard, limit int) []Breadcrumb {
	var crumbs []Breadcrumb
	if len(cards) == limit {
		crumbs = append(crumbs, Breadcrumb{
			Action:      "more",
			Cmd:         fmt.Sprintf("minka boards archive %s --before %s --limit %d", boardID, cards[len(cards)-1].ID, limit),
			Description: "Read the page behind these",
		})
		crumbs = append(crumbs, Breadcrumb{
			Action:      "all",
			Cmd:         fmt.Sprintf("minka boards archive %s --all --limit %d", boardID, limit),
			Description: "Read the whole archive in one run",
		})
	}
	return append(crumbs,
		Breadcrumb{Action: "show", Cmd: "minka cards show <cardId>", Description: "Read a card of the archive"},
		Breadcrumb{Action: "unarchive", Cmd: "minka cards unarchive <cardId>", Description: "Bring a card back onto its board"},
	)
}

// archiveRefusal classifies a failed request for an archive. The card a page
// was to start behind is the one refusal a caller can act on: it says nothing
// about the board and everything about the cursor.
func archiveRefusal(s *session, err error, boardID string) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == api.CodeArchivedCardsBeforeUnknown:
		e.Hint = "The card --before names does not lie in the archive of this board: somebody took it back onto the board or into the trash while the archive was being read. Read the archive again from the beginning with: minka boards archive " + boardID
	case e.Code == CodeNotFound:
		e.Hint = "There is no such board, or it belongs to a project you are not a member of. List the boards of a project with: minka boards list <projectId>"
	}
	return e
}
