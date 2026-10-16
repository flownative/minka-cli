package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// What Minka answers a write that would put a Docs & Files tool in order and
// that it will not carry out.
const (
	apiCodeFolderIntoItself           = "folderIntoItself"
	apiCodeFolderIDMissing            = "folderIdMissing"
	apiCodeFolderTitleMissing         = "folderTitleMissing"
	apiCodeFolderTitleTooLong         = "folderTitleTooLong"
	apiCodeFolderTitleHasControlChars = "folderTitleHasControlCharacters"
	apiCodeBookWithinABook            = "bookWithinABook"
	apiCodeFolderOrderAutomatic       = "folderOrderAutomatic"
	apiCodeEntryPlaceUnknown          = "entryPlaceUnknown"
	apiCodeOrderUnknown               = "orderUnknown"
)

// The two ways a place of a tool orders its entries.
const (
	orderAutomatic = "automatic"
	orderManual    = "manual"
)

// What only the team may do answers a client as though nothing were there, so
// every 404 of these writes names it beside what else it can mean.
const (
	folderNotFoundHint          = "There is no such folder, it or a folder above it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."
	onlyTheTeamBringsBack       = "Only the team brings anything back: a client of the project meets the same."
	aClientPutsAwayOnlyTheirOwn = "A client of the project puts away only what they made."
)

// entryFacts is what the commands that put a tool in order say about an
// entry, whichever kind it is.
type entryFacts struct {
	id, title, toolID, toolTitle, appURL string
	path                                 []api.FolderReference // down to the folder it stands in, empty at the top
}

// filesEntryKind is one kind of entry of a Docs & Files tool with the writes
// that put it in order. The three kinds take the same commands, each in its
// own group, so that an ID is never read as the wrong kind.
type filesEntryKind[T any] struct {
	noun     string // "document", "upload" or "folder"
	command  string // the group, "minka docs uploads"
	argument string // "uploadId"
	show     string // a call that names the folder an entry stands in, with ID for the entry

	remove                      func(*api.Client, context.Context, string) error
	restore, archive, unarchive func(*api.Client, context.Context, string) (api.Result[T], error)
	move                        func(*api.Client, context.Context, string, api.MoveFilesEntryInput) (api.Result[T], error)

	facts       func(T) entryFacts
	breadcrumbs func(T) []Breadcrumb
	notFound    string
}

var documentKind = filesEntryKind[api.Document]{
	noun: "document", command: "minka docs", argument: "documentId", show: "minka docs show ID",
	remove: (*api.Client).DeleteDocument, restore: (*api.Client).RestoreDocument,
	archive: (*api.Client).ArchiveDocument, unarchive: (*api.Client).UnarchiveDocument, move: (*api.Client).MoveDocument,
	facts: func(d api.Document) entryFacts {
		return entryFacts{d.ID, d.Title, d.ToolID, d.Tool.Title, d.AppURL, d.Path}
	},
	breadcrumbs: documentBreadcrumbs,
	notFound:    documentNotFoundHint,
}

var uploadKind = filesEntryKind[api.FilesUpload]{
	noun: "upload", command: "minka docs uploads", argument: "uploadId", show: "minka docs uploads show ID",
	remove: (*api.Client).DeleteFilesUpload, restore: (*api.Client).RestoreFilesUpload,
	archive: (*api.Client).ArchiveFilesUpload, unarchive: (*api.Client).UnarchiveFilesUpload, move: (*api.Client).MoveFilesUpload,
	facts: func(u api.FilesUpload) entryFacts {
		return entryFacts{u.ID, u.Title, u.ToolID, u.Tool.Title, u.AppURL, u.Path}
	},
	breadcrumbs: uploadBreadcrumbs,
	notFound:    uploadNotFoundHint,
}

var folderKind = filesEntryKind[api.FilesEntries]{
	noun: "folder", command: "minka docs folders", argument: "folderId", show: "minka docs list <toolId> --folder ID",
	remove: (*api.Client).DeleteFolder, restore: (*api.Client).RestoreFolder,
	archive: (*api.Client).ArchiveFolder, unarchive: (*api.Client).UnarchiveFolder, move: (*api.Client).MoveFolder,
	facts:       folderFacts,
	breadcrumbs: folderBreadcrumbs,
	notFound:    folderNotFoundHint,
}

// folderFacts reads a folder from the answer that lists what stands in it:
// the folder stands where the step of the path before it is.
func folderFacts(entries api.FilesEntries) entryFacts {
	facts := entryFacts{toolID: entries.Tool.ID, toolTitle: entries.Tool.Title, appURL: entries.AppURL}
	if entries.Folder != nil {
		facts.id, facts.title = entries.Folder.ID, entries.Folder.Title
	}
	if len(entries.Path) > 1 {
		facts.path = entries.Path[:len(entries.Path)-1]
	}
	return facts
}

// orderCommands are the writes every kind of entry takes.
func orderCommands[T any](a *app, k filesEntryKind[T]) []*cobra.Command {
	return []*cobra.Command{
		newEntryDeleteCmd(a, k),
		newEntryRestoreCmd(a, k),
		newEntryArchiveCmd(a, k),
		newEntryUnarchiveCmd(a, k),
		newEntryMoveCmd(a, k),
	}
}

func (a *app) newDocsFoldersCmd() *cobra.Command {
	cmd := group("folders", "Show, make, rename, move, archive and trash the folders of a Docs & Files tool",
		a.newDocsFoldersShowCmd(),
		a.newDocsFoldersCreateCmd(),
		a.newDocsFoldersEditCmd(),
	)
	cmd.AddCommand(orderCommands(a, folderKind)...)
	return cmd
}

func newEntryDeleteCmd[T any](a *app, k filesEntryKind[T]) *cobra.Command {
	long := fmt.Sprintf(`Moves a %[1]s into the trash of its project, from the tool or from its
archive, as removing it on the web does. The team may remove any %[1]s; a client
of the project only one they made.

A %[1]s in the trash reads like one that never existed: showing, changing or
deleting it again exits with 2 (not_found). "trash list <projectId>" shows it,
and "%[2]s restore <%[3]s>" brings it back.

One that stands below an archived folder exits 9 with folderArchived: it is not
taken out on its own, and comes back or goes with that folder.`, k.noun, k.command, k.argument)
	if k.noun == "folder" {
		long += `

What stands in the folder goes into the trash with it and comes back with it;
the trash lists the folder alone.`
	}

	return &cobra.Command{
		Use:     "delete <" + k.argument + ">",
		Aliases: []string{"trash"},
		Short:   "Move " + articleOf(k.noun) + " " + k.noun + " to the trash",
		Long:    long,
		Args:    requireArgs(k.argument),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := k.remove(s.client, a.ctx, args[0]); err != nil {
				return entryOrderRefusal(s, err, k, args[0], "delete")
			}

			summary := "Moved " + k.noun + " " + args[0] + " to the trash"
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "restore", Cmd: k.command + " restore " + args[0], Description: "Bring it back out of the trash"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func newEntryRestoreCmd[T any](a *app, k filesEntryKind[T]) *cobra.Command {
	return &cobra.Command{
		Use:   "restore <" + k.argument + ">",
		Short: "Bring " + articleOf(k.noun) + " " + k.noun + " back out of the trash",
		Long: fmt.Sprintf(`Brings a %[1]s back out of the trash to where it stood, and answers with it.
Only the team brings anything back: a client of the project exits 2
(not_found), as for a %[1]s that is not in the trash.

Only what went into the trash by itself comes back here. A %[1]s that went in
with a folder above it comes back with that folder, and one whose folder lies in
the trash or in the archive comes back once the folder does: "trash list
<projectId>" says which under blockedBy, and restorable says whether this
command takes it. A draft comes back on the web only.`, k.noun),
		Args: requireArgs(k.argument),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			entry, err := k.restore(s.client, a.ctx, args[0])
			if err != nil {
				return entryOrderRefusal(s, err, k, args[0], "restore")
			}
			facts := k.facts(entry.Value)
			return a.ok(entrySuccess(entry.Raw, facts, "Brought "+k.noun+" "+facts.title+" back out of the trash into "+placeOf(facts), k.breadcrumbs(entry.Value)))
		},
	}
}

func newEntryArchiveCmd[T any](a *app, k filesEntryKind[T]) *cobra.Command {
	long := fmt.Sprintf(`Takes a %[1]s into the archive of its tool, and answers with it, which now
says archived: true. The team may archive any %[1]s; a client of the project
only one they made.

A %[1]s in the archive is read at its own address and listed with "docs list
<toolId> --archived"; nothing is written on it until "%[2]s unarchive" brings it
back. One that lies in the archive already, itself or through a folder above it,
exits 9 with %[3]s.`, k.noun, k.command, k.noun+"Archived")
	if k.noun == "folder" {
		long += `

What stands in the folder goes quiet with it: readable, and closed to every
write until the folder is back.`
	}

	return &cobra.Command{
		Use:   "archive <" + k.argument + ">",
		Short: "Put " + articleOf(k.noun) + " " + k.noun + " away into the archive of its tool",
		Long:  long,
		Args:  requireArgs(k.argument),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			entry, err := k.archive(s.client, a.ctx, args[0])
			if err != nil {
				return entryOrderRefusal(s, err, k, args[0], "archive")
			}
			facts := k.facts(entry.Value)
			return a.ok(entrySuccess(entry.Raw, facts, "Put "+k.noun+" "+facts.title+" into the archive of "+facts.toolTitle, []Breadcrumb{
				{Action: "unarchive", Cmd: k.command + " unarchive " + facts.id, Description: "Bring it back out of the archive"},
				{Action: "archive", Cmd: "minka docs list " + facts.toolID + " --archived", Description: "List the archive of the tool"},
			}))
		},
	}
}

func newEntryUnarchiveCmd[T any](a *app, k filesEntryKind[T]) *cobra.Command {
	return &cobra.Command{
		Use:   "unarchive <" + k.argument + ">",
		Short: "Bring " + articleOf(k.noun) + " " + k.noun + " back out of the archive of its tool",
		Long: fmt.Sprintf(`Brings a %[1]s back among the entries of its tool, and answers with it. Only
the team brings anything back: a client of the project exits 2 (not_found).

A %[1]s that is not in the archive exits 9 with %[2]s. One that went into the
archive with a folder above it comes back only with that folder: it exits 9 with
folderArchived, and "docs list <toolId> --archived" marks it with
archivedWithAFolderAbove.`, k.noun, k.noun+"NotArchived"),
		Args: requireArgs(k.argument),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			entry, err := k.unarchive(s.client, a.ctx, args[0])
			if err != nil {
				return entryOrderRefusal(s, err, k, args[0], "unarchive")
			}
			facts := k.facts(entry.Value)
			return a.ok(entrySuccess(entry.Raw, facts, "Brought "+k.noun+" "+facts.title+" back out of the archive into "+placeOf(facts), k.breadcrumbs(entry.Value)))
		},
	}
}

func newEntryMoveCmd[T any](a *app, k filesEntryKind[T]) *cobra.Command {
	var folderID, after, before string
	var topLevel bool

	long := fmt.Sprintf(`Moves a %[1]s to another folder of the same tool, --folder, or to the top
level of the tool, --top-level; one of the two is required. In a place ordered
automatically it lands first among the %[1]ss there; in one arranged by hand it
goes to the end. A %[1]s moves within its tool only: a folder of another tool
or project exits 9 with folderUnknown.

--after <entryId> or --before <entryId> places it next to a folder, document
or upload of a place arranged by hand, one of the two at most; naming the place
where it stands rearranges it there. A place ordered automatically refuses it
with folderOrderAutomatic: "docs order <toolId> manual" arranges it by hand
first. A neighbour that does not stand in the place counts as none, and the
%[1]s then goes first.

A %[1]s that already stands there without a neighbour named is answered as
moved, and nothing is written. One in the archive, itself or through a folder
above it, does not move, and neither does anything into an archived folder
(folderArchived).`, k.noun)
	if k.noun == "folder" {
		long += ` A folder takes
everything in it along, and cannot move into itself or a folder below it
(folderIntoItself). A book, or a folder holding one, does not move into a book
(bookWithinABook); a folder that moves into a book is arranged by hand, with
every folder in it.`
	}

	cmd := &cobra.Command{
		Use:   "move <" + k.argument + "> (--folder <folderId> | --top-level) [--after <entryId> | --before <entryId>]",
		Short: "Move " + articleOf(k.noun) + " " + k.noun + " to another folder of its tool, or to another place in its order",
		Long:  long,
		Example: fmt.Sprintf(`  %[1]s move 0199... --folder 0199... --json
  %[1]s move 0199... --top-level --json
  %[1]s move 0199... --folder 0199... --after 0199... --json`, k.command),
		Args: requireArgs(k.argument),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("folder") == topLevel {
				return usageErrorf(cmd, "name the place: --folder <folderId> or --top-level, one of the two")
			}
			if cmd.Flags().Changed("after") && cmd.Flags().Changed("before") {
				return usageErrorf(cmd, "--after and --before exclude each other: name one neighbour")
			}
			input := api.MoveFilesEntryInput{}
			if !topLevel {
				if err := requireID(cmd, "--folder", &folderID); err != nil {
					return err
				}
				input.FolderID = &folderID
			}
			for _, neighbour := range []struct {
				flag  string
				value *string
				field **string
			}{{"--after", &after, &input.PreviousEntryID}, {"--before", &before, &input.NextEntryID}} {
				if !cmd.Flags().Changed(strings.TrimPrefix(neighbour.flag, "--")) {
					continue
				}
				if err := requireID(cmd, neighbour.flag, neighbour.value); err != nil {
					return err
				}
				// Minka takes the entry out of the order before it looks for
				// the neighbour, so the entry itself would count as none and
				// put it first without a word.
				if *neighbour.value == args[0] {
					return usageErrorf(cmd, "%s names the %s that moves; name another entry of the place", neighbour.flag, k.noun)
				}
				*neighbour.field = neighbour.value
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			entry, err := k.move(s.client, a.ctx, args[0], input)
			if err != nil {
				return entryOrderRefusal(s, err, k, args[0], "move")
			}
			facts := k.facts(entry.Value)
			summary := "Moved " + k.noun + " " + facts.title + " to " + placeOf(facts)
			switch {
			case input.PreviousEntryID != nil:
				summary += ", asked after " + after
			case input.NextEntryID != nil:
				summary += ", asked before " + before
			}
			return a.ok(entrySuccess(entry.Raw, facts, summary, k.breadcrumbs(entry.Value)))
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&folderID, "folder", "", "ID of the folder to move it into")
	flags.BoolVar(&topLevel, "top-level", false, "Move it to the top level of the tool, into no folder")
	flags.StringVar(&after, "after", "", "ID of the entry to place it right behind, in a place arranged by hand")
	flags.StringVar(&before, "before", "", "ID of the entry to place it right before, in a place arranged by hand")

	return cmd
}

// folderUnknownToTheHostHint answers a Minka that has no address for a folder
// by itself.
const folderUnknownToTheHostHint = "Its Minka is older than this minka expects and reads a folder only through its tool: minka docs list <toolId> --folder <folderId>"

func (a *app) newDocsFoldersShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <folderId>",
		Short: "Show a folder by its ID alone: its project, tool, place and what stands in it",
		Long: `Reads a folder by its ID, without its tool, and answers as "docs list
<toolId> --folder" does, with projectId and project, parent (the folder it
stands in, null for a folder at the top of the tool) and updatedAt (when the
folder itself last changed: a rename) besides. A folder in the archive is
shown with archived: true; one in the trash is not found.`,
		Example: `  minka docs folders show 0199... --json`,
		Args:    requireArgs("folderId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			folder, err := s.client.FilesFolder(a.ctx, args[0])
			if err != nil {
				e := folderRefusal(s, err, args[0], "show")
				var apiErr *api.Error
				if e.Code == CodeNotFound && errors.As(err, &apiErr) && apiErr.Body == nil {
					e.Message = s.host + " has no address for a folder by itself"
					e.Hint = folderUnknownToTheHostHint
				}
				return e
			}
			summary := "Folder " + folderFacts(folder.Value).title
			if folder.Value.Project != nil {
				summary += " in " + folder.Value.Tool.Title + " of " + folder.Value.Project.Name
			}
			return a.ok(success{
				data:        folder.Raw,
				summary:     summary + ": " + entriesSummary(folder.Value),
				breadcrumbs: folderBreadcrumbs(folder.Value),
				text:        func(w io.Writer) { renderFilesEntries(w, folder.Value) },
			})
		},
	}
}

func (a *app) newDocsFoldersCreateCmd() *cobra.Command {
	var toolID, title, parentID string
	var visibleToClients bool

	cmd := &cobra.Command{
		Use:   "create --tool <toolId> --title <title>",
		Short: "Make a folder in a Docs & Files tool",
		Long: `Makes a folder at the top of a tool, or inside the folder --folder names: first
among the folders where the order is automatic, last where it is set by hand.
Made inside a book, it is arranged by hand itself. data is the new folder as
"docs list <toolId> --folder" reads it, its ID under folder.id.

--visible-to-clients shows a folder at the top of the tool to the clients of
the project at once, as the web's checkbox does. It counts only at the top, in
a project open to clients and from the team: a folder further down is shown to
clients as the topmost folder above it is, and a client's own folder at the
top is shown to clients anyway. Minka says in no answer whether it counted.

A folder in the archive takes nothing new (folderArchived), and one the tool
has not got is folderUnknown. An empty title exits 9 with folderTitleMissing.`,
		Example: `  minka docs folders create --tool 0199... --title "Offers" --json
  minka docs folders create --tool 0199... --title "2027" --folder 0199... --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			if !cmd.Flags().Changed("title") {
				return usageErrorf(cmd, "a folder needs --title")
			}
			var notices []string
			if parentID != "" {
				if err := requireID(cmd, "--folder", &parentID); err != nil {
					return err
				}
				if visibleToClients {
					notices = append(notices, "--visible-to-clients counts only at the top of the tool: a folder inside another is shown to clients as the topmost folder above it is")
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			folder, err := s.client.CreateFolder(a.ctx, toolID, api.CreateFolderInput{Title: title, FolderID: parentID, VisibleToClients: visibleToClients})
			if err != nil {
				return folderRefusal(s, err, "", "create")
			}
			facts := folderFacts(folder.Value)
			where := "at " + placeOf(facts)
			if len(facts.path) > 0 {
				where = "in " + placeOf(facts)
			}
			result := entrySuccess(folder.Raw, facts, "Made folder "+facts.title+" "+where, folderBreadcrumbs(folder.Value))
			result.notices = notices
			return a.ok(result)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the Docs & Files tool")
	flags.StringVar(&title, "title", "", "Title of the folder")
	flags.StringVar(&parentID, "folder", "", "ID of the folder to make it in; the top of the tool without it")
	flags.BoolVar(&visibleToClients, "visible-to-clients", false, "Show a folder at the top of the tool to the clients of the project")

	return cmd
}

func (a *app) newDocsFoldersEditCmd() *cobra.Command {
	var title string
	var book, noBook bool

	cmd := &cobra.Command{
		Use:   "edit <folderId> [--title <title>] [--book | --no-book]",
		Short: "Rename a folder, or mark it as a book",
		Long: `Gives a folder another title, marks it as a book or takes the mark away, and
answers with it as "docs list <toolId> --folder" reads it; what is not given
stays. A title can only be replaced: an empty one exits 9 with
folderTitleMissing. A folder in the archive, itself or through a folder above
it, keeps its title and its mark until it is back (folderArchived).

--book makes the folder a book: "docs book <folderId>" reads it as one tree of
chapters and pages. Marking arranges the book and every folder in it by hand,
in the order they showed, and a folder made in it or moved into it later is
arranged by hand too. --no-book takes the mark away and leaves every order as
it is. Marking a book that is one, or unmarking a folder that is none, changes
nothing and is answered the same.

A book never stands inside another: a folder below a book, or one holding a
book at any depth (the trash counts), exits 9 with bookWithinABook. Only the
team marks a book: a client of the project exits 2 (not_found), whatever else
the call changes. An answer that does not carry the mark asked for, as from
a Minka that does not know books, is exit 7 (api_error); the title, where
given, went through.`,
		Example: `  minka docs folders edit 0199... --title "Handbook" --json
  minka docs folders edit 0199... --book --json
  minka docs folders edit 0199... --title "Handbook" --book --json
  minka docs folders edit 0199... --no-book --json`,
		Args: requireArgs("folderId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseAnExplicitFalse(cmd, "book", "no-book"); err != nil {
				return err
			}
			if book && noBook {
				return usageErrorf(cmd, "--book and --no-book exclude each other")
			}
			input := api.UpdateFolderInput{}
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if book || noBook {
				input.IsBook = &book
			}
			if input.Title == nil && input.IsBook == nil {
				return usageErrorf(cmd, "nothing to change: give --title, --book or --no-book")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			folder, err := s.client.UpdateFolder(a.ctx, args[0], input)
			if err != nil {
				e := folderRefusal(s, err, args[0], "edit")
				if e.Code == CodeNotFound && input.IsBook != nil {
					e.Hint = folderNotFoundHint + " Only the team marks a book: a client of the project meets the same."
				}
				return e
			}
			// A Minka without books answers 200 and writes no mark; only the
			// answer that does not carry it tells.
			if input.IsBook != nil && folder.Value.IsBook != *input.IsBook {
				return booksUnknown("the mark of a book", input.Title != nil, folder.Raw)
			}
			facts := folderFacts(folder.Value)
			return a.ok(entrySuccess(folder.Raw, facts, folderEditSummary(facts, input), folderBreadcrumbs(folder.Value)))
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.BoolVar(&book, "book", false, "Mark the folder as a book")
	flags.BoolVar(&noBook, "no-book", false, "Take the mark of a book away")

	return cmd
}

// folderEditSummary says what an edit of a folder asked for. Minka answers a
// mark that was there already the same as a new one, so the summary names the
// state, not a change.
func folderEditSummary(facts entryFacts, input api.UpdateFolderInput) string {
	mark := ""
	if input.IsBook != nil {
		mark = map[bool]string{true: "a book", false: "no book"}[*input.IsBook]
	}
	switch {
	case input.Title != nil && mark != "":
		return "Renamed folder " + facts.id + " to " + facts.title + ", which is " + mark
	case input.Title != nil:
		return "Renamed folder " + facts.id + " to " + facts.title
	default:
		return "Folder " + facts.title + " is " + mark
	}
}

func (a *app) newDocsVersionsRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <documentId> <versionId>",
		Short: "Make an earlier version of a document its newest one",
		Long: `Writes what an earlier version said, its title and its body, as the newest
version of the document, as the web's history does, and answers with the
document. Nothing is removed: the versions in between stay, and nobody is told.
A version that says what the document says already is answered the same and
adds no version.

It is the team's alone: a client of the project exits 2 (not_found). A document
in the archive, itself or through a folder above it, exits 9 with
documentArchived. The IDs come from "docs versions list".`,
		Args: requireArgs("documentId", "versionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			document, err := s.client.RestoreDocumentVersion(a.ctx, args[0], args[1])
			if err != nil {
				e := entryOrderRefusal(s, err, documentKind, args[0], "version")
				if e.Code == CodeNotFound {
					e.Hint = "There is no such document or version, the document is a draft or in the trash, it belongs to a project you are not a member of, or you are a client of the project: only the team restores a version. The versions are listed with: minka docs versions list " + args[0]
				}
				return e
			}
			facts := documentKind.facts(document.Value)
			crumbs := append([]Breadcrumb{
				{Action: "versions", Cmd: "minka docs versions list " + args[0], Description: "List the versions of the document"},
			}, documentBreadcrumbs(document.Value)...)
			return a.ok(entrySuccess(document.Raw, facts, "Wrote version "+args[1]+" as the newest of document "+facts.title, crumbs))
		},
	}
}

// docsArchive lists the archive of a tool, for docs list --archived.
func (a *app) docsArchive(toolID string) error {
	s, err := a.session()
	if err != nil {
		return err
	}
	archive, err := s.client.FilesArchive(a.ctx, toolID)
	if err != nil {
		e := s.fail(err)
		if e.Code == CodeNotFound {
			e.Hint = filesToolNotFoundHint
		}
		return e
	}

	return a.ok(success{
		data:    archive.Raw,
		summary: plural(len(archive.Value), "entry", "entries") + " in the archive",
		breadcrumbs: []Breadcrumb{
			{Action: "unarchive", Cmd: "minka docs unarchive <documentId>", Description: "Bring a document back out of the archive"},
			{Action: "unarchive-upload", Cmd: "minka docs uploads unarchive <uploadId>", Description: "Bring an upload back out of the archive"},
			{Action: "unarchive-folder", Cmd: "minka docs folders unarchive <folderId>", Description: "Bring a folder back out of the archive, with what stands in it"},
		},
		text: func(w io.Writer) { renderFilesArchive(w, archive.Value) },
	})
}

// entrySuccess is the answer of a write on one entry: the entry as Minka
// hands it back, and in text what happened and where it is.
func entrySuccess(raw any, facts entryFacts, summary string, breadcrumbs []Breadcrumb) success {
	return success{
		data:        raw,
		summary:     summary,
		breadcrumbs: breadcrumbs,
		text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\nID: %s\n%s\n", summary, facts.id, facts.appURL) },
	}
}

// placeOf names where an entry stands: in a folder by its path, or at the top
// of the tool.
func placeOf(facts entryFacts) string {
	if len(facts.path) == 0 {
		return "the top of " + facts.toolTitle
	}
	return pathOfFolders(facts.path) + " in " + facts.toolTitle
}

// entryOrderRefusal classifies a refused write that puts an entry in order.
// The same code means different things to different writes — folderArchived
// is the folder above an entry for one and the folder it moves into for
// another — so the hint is chosen by the code and the write together.
func entryOrderRefusal[T any](s *session, err error, k filesEntryKind[T], id, action string) *Error {
	e := documentRefusal(s, err)
	show := strings.ReplaceAll(k.show, "ID", id)
	// For a folder its own code and the code of a folder above it are one.
	archived := e.APICode == k.noun+"Archived"
	switch {
	case e.APICode == apiCodeFolderArchived && action == "delete":
		e.Hint = fmt.Sprintf("A folder above the %s lies in the archive, and what stands in it is not taken out on its own: it comes back with that folder, minka docs folders unarchive <folderId>, or goes into the trash with it. %s names the folder.", k.noun, show)
	case e.APICode == apiCodeFolderArchived && action == "unarchive":
		e.Hint = fmt.Sprintf("The %s went into the archive with a folder above it, and only that folder brings it back: minka docs folders unarchive <folderId>. minka docs list <toolId> --archived marks such an entry with archivedWithAFolderAbove.", k.noun)
	case archived && action == "archive":
		e.Hint = fmt.Sprintf("The %s lies in the archive already, itself or through a folder above it; %s says which, and minka docs list <toolId> --archived lists the archive.", k.noun, show)
	case archived && action == "move" && k.noun == "folder":
		e.Hint = "The folder, or one above it, lies in the archive, or the folder --folder names does: nothing in the archive moves, and nothing new goes into it. Bring it back with minka docs folders unarchive <folderId>, or name another place."
	case archived:
		e.Hint = fmt.Sprintf("The %s, or a folder above it, lies in the archive; %s says which. It can be read, and nothing is written on it until it is back: %s unarchive %s, or where a folder above it is archived, minka docs folders unarchive <folderId>.", k.noun, show, k.command, id)
	case e.APICode == apiCodeFolderArchived && action == "move":
		e.Hint = "The folder --folder names, or one above it, lies in the archive and takes nothing new. Name another place, --top-level, or bring that folder back first: minka docs folders unarchive <folderId>"
	case e.APICode == k.noun+"NotArchived":
		e.Hint = fmt.Sprintf("The %s stands among the entries of its tool, not in the archive. Put it away with: %s archive %s", k.noun, k.command, id)
	case e.APICode == apiCodeFolderUnknown && action == "move":
		e.Hint = fmt.Sprintf("--folder names no folder of the %s's tool: an entry moves within its tool only, and a folder in the trash, or one you are not shown, is none. List the folders with: minka docs list <toolId>", k.noun)
	case e.APICode == apiCodeFolderIntoItself:
		e.Hint = "Name a folder outside the one that moves with --folder, or move it with --top-level."
	case e.APICode == apiCodeFolderOrderAutomatic:
		e.Hint = "The place the move names orders its entries automatically, and --after and --before place an entry only where the order is set by hand. Arrange it by hand first, minka docs order <toolId> manual [--folder <folderId>], or move without a neighbour."
	case e.APICode == apiCodeBookWithinABook:
		e.Hint = "A book never stands inside another: the folder is a book or holds one, and the place it would move to belongs to a book. Name a place outside every book, or take the mark away first with minka docs folders edit <folderId> --no-book."
	case e.APICode == apiCodeEntryPlaceUnknown:
		e.Hint = "A neighbour names an entry by its ID. This CLI sends only IDs, so the Minka it talks to reads the request otherwise."
	case e.APICode == apiCodeFolderIDMissing:
		e.Hint = "A move names its place. This CLI always sends one, so the Minka it talks to reads the request otherwise; read the entry back before trying again."
	case e.Code == CodeNotFound:
		switch action {
		case "restore":
			e.Hint = fmt.Sprintf("The %[1]s is not in the trash by itself: it was never there or is back already, it went in with a folder above it, which brings it back (minka docs folders restore <folderId>), or a folder above it lies in the trash or the archive — minka trash list <projectId> names that under blockedBy. A draft comes back on the web only. %[2]s", k.noun, onlyTheTeamBringsBack)
		case "delete", "archive":
			e.Hint = k.notFound + " " + aClientPutsAwayOnlyTheirOwn
		case "unarchive":
			e.Hint = k.notFound + " " + onlyTheTeamBringsBack
		default:
			e.Hint = k.notFound
		}
	}
	return e
}

// folderRefusal classifies a refused write of a folder's own: making one and
// renaming one.
func folderRefusal(s *session, err error, id, action string) *Error {
	e := documentRefusal(s, err)
	switch {
	case e.APICode == apiCodeFolderTitleMissing:
		e.Hint = "A folder needs a title with words in it. A title can be replaced, never emptied."
	case e.APICode == apiCodeFolderTitleTooLong:
		e.Hint = "The title is longer than Minka keeps; shorten --title."
	case e.APICode == apiCodeFolderTitleHasControlChars:
		e.Hint = "The title holds a line break or another control character; give it on one line."
	case e.APICode == apiCodeFolderArchived && action == "edit":
		e.Hint = "The folder, or one above it, lies in the archive, and keeps its title and its mark until it is back: minka docs folders unarchive " + id + ", or the folder above it."
	case e.APICode == apiCodeBookWithinABook:
		e.Hint = "A book never stands inside another: the folder stands below a book, which minka docs book " + id + " names, or holds one below it. Take that mark away first with minka docs folders edit <folderId> --no-book, or mark another folder."
	case e.Code == CodeNotFound && action == "create":
		e.Hint = filesToolNotFoundHint
	case e.Code == CodeNotFound:
		e.Hint = folderNotFoundHint
	}
	return e
}

func folderBreadcrumbs(entries api.FilesEntries) []Breadcrumb {
	facts := folderFacts(entries)
	crumbs := []Breadcrumb{
		{Action: "list", Cmd: "minka docs list " + facts.toolID + " --folder " + facts.id, Description: "List what stands in the folder"},
		{Action: "create", Cmd: "minka docs create --tool " + facts.toolID + " --folder " + facts.id + " --title <title> --audience nobody", Description: "Write a document in it"},
		{Action: "folder", Cmd: "minka docs folders create --tool " + facts.toolID + " --folder " + facts.id + " --title <title>", Description: "Make a folder in it"},
		{Action: "rename", Cmd: "minka docs folders edit " + facts.id + " --title <title>", Description: "Rename the folder"},
	}
	if entries.IsBook || entries.Book != nil {
		crumbs = append(crumbs, Breadcrumb{Action: "book", Cmd: "minka docs book " + facts.id, Description: "Read the book it belongs to as one tree"})
	}
	return crumbs
}

func renderFilesArchive(w io.Writer, entries []api.ArchivedFilesEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "The archive is empty.")
		return
	}
	for _, entry := range entries {
		fmt.Fprintf(w, "- %s  %s %s\n", entry.ID, entry.Kind, entry.Title)
		when := "  archived " + formatTime(entry.ArchivedAt)
		if entry.ArchivedBy != nil {
			when += " by " + entry.ArchivedBy.Name
		}
		if entry.ArchivedWithAFolderAbove {
			when += ", with a folder above it, which alone brings it back"
		}
		fmt.Fprintln(w, when)
	}
}
