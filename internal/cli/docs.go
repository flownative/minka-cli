package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a write on a Docs & Files tool that it will not carry
// out.
const (
	apiCodeDocumentChanged  = "documentChanged"
	apiCodeUploadChanged    = "uploadChanged"
	apiCodeDocumentArchived = "documentArchived"
	apiCodeUploadArchived   = "uploadArchived"
	apiCodeFolderUnknown    = "folderUnknown"
	apiCodeFolderArchived   = "folderArchived"
	apiCodeFileUnknown      = "fileUnknown"
	apiCodeFileInUse        = "fileInUse"
)

// theGuardIsKeptToTheSecond is what a guard on an entry cannot do. Minka keeps
// the moment of a document and of an upload to the whole second, so a change
// made in the same second as the read carries the moment that was read and
// passes for it.
const theGuardIsKeptToTheSecond = "the guard is kept to the whole second: a change somebody else made in the same second as your read is not noticed, so read it back after a write that matters"

const documentBodyHelp = `The body is Markdown by default and is sent as HTML, the way a description is.
Minka keeps headings, bold, italic, strikethrough, inline code, links, lists,
quotes, code blocks with their language, tables and horizontal rules, and the
shapes of a document: a callout as "> [!NOTE] Title" or "> [!WARNING]", a
Mermaid diagram as a code block of the language mermaid, link tiles as a list
under "<!-- minka:tiles -->" and a block of variables as a table under
"<!-- minka:variables -->". Images become links and links Minka does not keep
become plain text, each with a warning. Use --body - to read it from stdin.

--body-format html sends HTML as it is. Minka refuses no body: one that
carries no element it serves is kept as the characters it consists of, so
Markdown sent that way stands in the document afterwards as "# Heading". The
command says so in a notice, and nothing else ever will.`

const documentAudienceHelp = audienceHelp + `

Somebody named has to be able to see the document. A client of the project,
or an agent working for one, is told only about a document in a folder shown to
clients, in a tool shown to clients; the topmost folder decides for everything
below it. Naming them anywhere else is refused with personCannotSeeIt. Nothing
is written then, and sending it again changes nothing: name somebody else.`

const documentNotFoundHint = "There is no such document, it is a draft, it or a folder above it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."

const filesToolNotFoundHint = "There is no such Docs & Files tool, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."

const uploadNotFoundHint = "There is no such upload, it or a folder above it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."

func (a *app) newDocsCmd() *cobra.Command {
	cmd := group("docs", "Read, write and keep in order the documents, uploads and folders of a Docs & Files tool, and comment on them",
		a.newDocsToolsCmd(),
		a.newDocsToolCmd(),
		a.newDocsListCmd(),
		a.newDocsShowCmd(),
		a.newDocsCreateCmd(),
		a.newDocsEditCmd(),
		a.newDocsVersionsCmd(),
		a.newDocsUploadsCmd(),
		a.newDocsFoldersCmd(),
		a.newDocsBookCmd(),
		a.newDocsExportCmd(),
		a.newDocsOrderCmd(),
		a.newDocsCommentsCmd(),
	)
	cmd.AddCommand(orderCommands(a, documentKind)...)
	return cmd
}

func (a *app) newDocsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the Docs & Files tools of a project",
		Long: `Lists the Docs & Files tools of a project, in the order of the project page.
A project can have more than one. What stands in one is read with
"docs list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.FilesTools(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "Docs & Files tool", "Docs & Files tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka docs list <toolId>", Description: "List what stands at the top of a tool"},
				},
				text: func(w io.Writer) { renderFilesTools(w, tools.Value) },
			})
		},
	}
}

func (a *app) newDocsListCmd() *cobra.Command {
	var folderID string
	var archived bool

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List what stands at the top of a Docs & Files tool, or in one of its folders",
		Long: `Lists the folders, documents and uploads of a Docs & Files tool: those at the
top, or those in the folder --folder names. It reaches one folder deep, so
going down means calling it again with a folder of the answer.

Nothing here is paged and nothing is cut. The order is the one the web shows,
which is neither alphabetical nor by date: order says whether it is automatic,
folders before documents before uploads, or set by hand, where the three are
mixed and the text lists them in one list. entries gives the order shown, and
"docs order" chooses it. isBook and book say whether the place is a book or
belongs to one ("docs book" reads it). Drafts and entries in the archive
are not listed, and neither is a folder in the archive — but the inside of one
can be listed by naming it, and then the answer and every line of it says
archived: true.

Every document carries a lead: the first 240 characters of its body without
markup, cut where the count runs out and not at a word.

--archived lists the archive of the tool instead: the folders, documents and
uploads that went into it themselves, from any folder, the one archived last
first, with when and by whom. archivedWithAFolderAbove marks one whose folder
went into the archive too, and only that folder brings it back. What stands in
an archived folder without being archived itself is not listed there; --folder
lists it. It is not paged, and does not go with --folder.`,
		Example: `  minka docs list 0199... --json
  minka docs list 0199... --folder 0199... --json
  minka docs list 0199... --archived`,
		Args: requireArgs("toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if archived {
				if cmd.Flags().Changed("folder") {
					return usageErrorf(cmd, "--archived lists the whole archive of the tool and takes no --folder")
				}
				return a.docsArchive(args[0])
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
			entries, err := s.client.FilesEntriesOf(a.ctx, args[0], folderID)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "There is no such tool or folder, or it belongs to a project you are not a member of. List the tools of a project with: minka docs tools <projectId>"
				}
				return e
			}

			return a.ok(success{
				data:        entries.Raw,
				summary:     entriesSummary(entries.Value),
				breadcrumbs: entriesBreadcrumbs(args[0], entries.Value),
				text:        func(w io.Writer) { renderFilesEntries(w, entries.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&folderID, "folder", "", "ID of the folder to list; the top of the tool without it")
	cmd.Flags().BoolVar(&archived, "archived", false, "List the archive of the tool instead")

	return cmd
}

func (a *app) newDocsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <documentId>",
		Short: "Show a document with its body and comments",
		Long: `Shows a document with the folder it stands in, its body and its comments. In
text output the body is converted to Markdown, which docs edit --body turns
back into the same HTML; in JSON it is the HTML the server stores.

updatedAt is what docs edit --expect takes to write over the document. A
document in the archive, or one in a folder in the archive, is shown as well
and says so: reading works, writing on it is refused.

A draft is at no address, its author included: it exits 2 (not_found).`,
		Args: requireArgs("documentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			document, err := s.client.Document(a.ctx, args[0])
			if err != nil {
				return documentRefusal(s, err)
			}

			return a.ok(success{
				data:        document.Raw,
				summary:     documentSummary(document.Value),
				breadcrumbs: documentBreadcrumbs(document.Value),
				text:        func(w io.Writer) { renderDocument(w, document.Value) },
			})
		},
	}
}

func (a *app) newDocsCreateCmd() *cobra.Command {
	var toolID, title, body, bodyFormat, folderID, audience string
	var notify []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Write a document in a Docs & Files tool",
		Long: "Writes a published document at the top of a Docs & Files tool, or in the\n" +
			"folder --folder names. There are no drafts here: what this writes is published.\n\n" +
			documentAudienceHelp + "\n\n" +
			documentBodyHelp + "\n\n" +
			"A document may be only a title: without --body it is written with an empty body.",
		Example: `  minka docs create --tool 0199... --title "How the stage goes up" --audience nobody --json
  minka docs create --tool 0199... --title "Costs" --folder 0199... --audience everybody --body - < costs.md`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			if cmd.Flags().Changed("folder") {
				if err := requireID(cmd, "--folder", &folderID); err != nil {
					return err
				}
			}
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			if err := checkAudience(cmd, "document", audience); err != nil {
				return err
			}

			input := api.CreateDocumentInput{Title: title, Audience: audience, FolderID: folderID}
			var notices []string
			if cmd.Flags().Changed("body") {
				html, warnings, err := a.documentBody(body, bodyFormat)
				if err != nil {
					return err
				}
				input.Body, notices = html, warnings
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			input.NotifyIDs, err = a.peopleOfATool(s, a.projectOfFilesTool(s, toolID), notify)
			if err != nil {
				return err
			}

			document, err := s.client.CreateDocument(a.ctx, toolID, input)
			if err != nil {
				return documentRefusal(s, err)
			}

			summary := "Wrote document " + document.Value.Title
			return a.ok(success{
				data:        document.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: documentBreadcrumbs(document.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nDocument: %s\n%s\n", summary, document.Value.ID, document.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the Docs & Files tool the document goes into")
	flags.StringVar(&title, "title", "", "Title of the document")
	flags.StringVar(&body, "body", "", "Body of the document, - to read it from stdin")
	flags.StringVar(&bodyFormat, "body-format", "markdown", "Format of the body: markdown or html")
	flags.StringVar(&folderID, "folder", "", "ID of the folder to write in; the top of the tool without it")
	flags.StringVar(&audience, "audience", "", "Whom the document reaches: everybody, chosen or nobody")
	flags.StringArrayVar(&notify, "notify", nil, "Person to tell about the document, with --audience chosen: ID, me, e-mail address or name (repeatable)")

	return cmd
}

func (a *app) newDocsEditCmd() *cobra.Command {
	var title, body, bodyFormat, expect string
	var force, dropAttachments, inNavigation, notInNavigation bool

	cmd := &cobra.Command{
		Use:   "edit <documentId>",
		Short: "Change the title or the body of a document, or its place in the navigation of a book",
		Long: "Changes the title, the body or both. What is not given stays as it is, and\n" +
			"changing a document tells nobody.\n\n" +
			`--body "" empties the body. --title "" does not empty the title: a document
needs one, so an empty title exits 9 (validation) with meta.apiCode
documentTitleMissing. A title can only be replaced, never removed.` + "\n\n" +
			documentBodyHelp + "\n\n" +
			`Somebody else may have written the document since you read it. Give the
updatedAt you read with --expect: a document written over since is then left
as it stands, the command prints it and exits with 9 (meta.apiCode
documentChanged). Merge your text into it and send it again with its updatedAt.
--force writes over whatever stands, and one of the two flags is required.
--expect none is not a value here: it belongs to the notes of a card, where it
means that nobody has written any.

The guard reaches only as far as Minka's own clock: the moment of a document is
kept to the whole second, so a change made in the same second as your read is
not noticed. After a write that matters, read the document back.

Files and mentions stand in a body as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource from docs show --json,
change it and send it back with --body-format html. A new body that would drop
them is refused unless --drop-attachments is given.

--not-in-navigation leaves the document out of the navigation of the book it
stands in: it stays in the book and is reached by link, reference and search,
and previous and next pass it over. --in-navigation lists it again. It may be
set on a document in no book and counts once the document is in one. Alone,
it writes only the flag: not the title or the body, no version, updatedAt as
it was, and never a 409. So it needs neither --expect nor --force; --expect,
where given, is sent as it is. Only the team chooses it: a client of the
project exits 2 (not_found), whatever else the call changes. A refused title
or body leaves the navigation as it was. An answer that does not carry the
navigation, as from a Minka that does not know books, is exit 7 (api_error).`,
		Example: `  minka docs edit 0199... --title "How the stage goes up" --expect 2026-09-26T14:03:12.000000Z
  minka docs edit 0199... --body - --expect 2026-09-26T14:03:12.000000Z < body.md
  minka docs edit 0199... --body "" --force
  minka docs edit 0199... --not-in-navigation --json`,
		Args: requireArgs("documentId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			if err := refuseAnExplicitFalse(cmd, "in-navigation", "not-in-navigation"); err != nil {
				return err
			}
			if inNavigation && notInNavigation {
				return usageErrorf(cmd, "--in-navigation and --not-in-navigation exclude each other")
			}
			text := cmd.Flags().Changed("title") || cmd.Flags().Changed("body")
			if !text && !inNavigation && !notInNavigation {
				return usageErrorf(cmd, "nothing to change, give --title, --body, --in-navigation or --not-in-navigation")
			}
			// The guard stands over the title and the body. A change of the
			// navigation alone writes only the flag and is never answered
			// with 409, so it asks for neither flag, and sends --expect where
			// it is given.
			var guard *string
			if text || cmd.Flags().Changed("expect") || force {
				var err error
				if guard, err = entryGuard(cmd, "document", "minka docs show <documentId>", expect, force); err != nil {
					return err
				}
			}

			input := api.UpdateDocumentInput{ExpectedUpdatedAt: guard}
			var notices []string
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("body") {
				html, warnings, err := a.documentBody(body, bodyFormat)
				if err != nil {
					return err
				}
				input.Body, notices = &html, warnings
			}
			if inNavigation || notInNavigation {
				input.InNavigation = &inNavigation
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if input.Body != nil {
				// The document is read for the one thing the new body has to be
				// held against, not for the guard: an expectation stands for what
				// the writer read before it began to write, and one filled in here
				// would protect nothing.
				current, err := s.client.Document(a.ctx, args[0])
				if err != nil {
					return documentRefusal(s, err)
				}
				// A document of the archive, or one below an archived folder,
				// takes no body at all.
				dropped, err := a.guardOverFiles(s, current.Value.ProjectID, closedIf(current.Value.Archived), firstNonEmpty(current.Value.BodySource, current.Value.Body), *input.Body, dropAttachments,
					"the document has %s, and the new body would drop them",
					"Change the bodySource from docs show --json and send it back with --body-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			document, err := s.client.UpdateDocument(a.ctx, args[0], input)
			if err != nil {
				e := documentRefusal(s, err)
				if e.Code == CodeNotFound && input.InNavigation != nil {
					e.Hint = documentNotFoundHint + " Only the team chooses the navigation of a book: a client of the project meets the same, whatever else the call changes."
				}
				return entryConflict(a, e, err, apiCodeDocumentChanged, "document",
					func(d api.Document) string { return d.UpdatedAt }, renderDocument)
			}

			// A Minka without books answers 200 and writes no navigation; only
			// the missing field tells.
			if input.InNavigation != nil && document.Value.InNavigation == nil {
				return booksUnknown("the navigation", text, document.Raw)
			}
			if guard != nil && text {
				notices = append(notices, theGuardIsKeptToTheSecond)
			}
			if input.InNavigation != nil && document.Value.Book == nil && document.Value.InNavigation != nil {
				notices = append(notices, "the document stands in no book: whether it is in the navigation counts once it is in one")
			}
			summary := "Wrote over document " + document.Value.Title
			if !text {
				summary = "Document " + document.Value.Title + " is " + map[bool]string{true: "in", false: "out of"}[inNavigation] + " the navigation of its book"
			}
			return a.ok(success{
				data:        document.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: documentBreadcrumbs(document.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nTo write over it: --expect %s\n%s\n", summary, document.Value.UpdatedAt, document.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.StringVar(&body, "body", "", "New body, - to read it from stdin, empty to empty the body")
	flags.StringVar(&bodyFormat, "body-format", "markdown", "Format of the body: markdown or html")
	flags.StringVar(&expect, "expect", "", "The updatedAt of the document you read")
	flags.BoolVar(&force, "force", false, "Write over the document whatever stands")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a body that has attachments, mentions or references, which drops them")
	flags.BoolVar(&inNavigation, "in-navigation", false, "List the document in the navigation of its book")
	flags.BoolVar(&notInNavigation, "not-in-navigation", false, "Leave the document out of the navigation of its book")

	return cmd
}

func (a *app) newDocsUploadsCmd() *cobra.Command {
	cmd := group("uploads", "Read, put in, change and keep in order the uploads of a Docs & Files tool",
		a.newDocsUploadsShowCmd(),
		a.newDocsUploadsCreateCmd(),
		a.newDocsUploadsEditCmd(),
		a.newDocsUploadsVersionsCmd(),
	)
	cmd.AddCommand(orderCommands(a, uploadKind)...)
	return cmd
}

func (a *app) newDocsUploadsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <uploadId>",
		Short: "Show an upload of a Docs & Files tool with its description and comments",
		Long: `Shows an upload: a file standing on its own in a Docs & Files tool, with what
was said about it and everything said under it.

The bytes are fetched with "uploads download <blobId>", where <blobId> is
file.id of this upload — not its own ID, which names the entry.

updatedAt is what docs uploads edit --expect takes. Another file is given with
docs uploads versions create, and the files it showed are listed with docs
uploads versions list.`,
		Args: requireArgs("uploadId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			upload, err := s.client.FilesUploadOf(a.ctx, args[0])
			if err != nil {
				return uploadRefusal(s, err)
			}

			return a.ok(success{
				data:        upload.Raw,
				summary:     uploadSummary(upload.Value),
				breadcrumbs: uploadBreadcrumbs(upload.Value),
				text:        func(w io.Writer) { renderFilesUpload(w, upload.Value) },
			})
		},
	}
}

const uploadDescriptionHelp = `The description is Markdown by default and is sent as HTML, the way the body of
a document is; --description - reads it from stdin. --description-format html
sends HTML as it is, and one that carries no element Minka serves is kept as
the characters it consists of, which the command says in a notice.

Somebody mentioned in the description is told about the upload. Nobody else is.`

func (a *app) newDocsUploadsCreateCmd() *cobra.Command {
	var toolID, projectID, blobID, folderID, title, description, descriptionFormat, name, contentType string

	cmd := &cobra.Command{
		Use:   "create (<file> | --blob <blobId>) --tool <toolId>",
		Short: "Put a file into a Docs & Files tool",
		Long: `Puts a file into a Docs & Files tool, or into the folder --folder names, as an
upload of its own: first among the uploads where the order is automatic, last
where it is set by hand.

Minka takes it in two requests: the file is stored in the project of the tool
first, and then put into the tool. Given a file, the command does both, and
reads the tool for its project before the file travels, so a tool that is not
there costs no transfer. A step that fails ends the run, and nothing after it
is sent.

Where the file was stored and the second step was refused, the error hands
over the stored file under "data": give --blob <blobId> with its id in place
of the file to try again without sending it anew. --blob also takes a file
stored earlier in the project of the tool with "uploads create --project". A
file goes into one place only: one that stands in an upload or a text already
is refused with fileInUse.

` + unattachedFileHelp + `

Without --title the upload is named after the file. A title cannot be empty:
--title "" is refused here, before the file travels.

` + uploadDescriptionHelp + `

` + mediaTypeHelp,
		Example: `  minka docs uploads create site-plan.pdf --tool 0199...
  minka docs uploads create shot.png --tool 0199... --folder 0199... --title "The stage" --description - < about.md
  minka docs uploads create --blob 0199... --tool 0199...`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageErrorf(cmd, "unexpected argument %q", args[1])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			if err := requireID(cmd, "--tool", &toolID); err != nil {
				return err
			}
			storing := len(args) == 1
			switch {
			case storing == flags.Changed("blob"):
				return usageErrorf(cmd, "give a file or --blob, not both and not neither")
			case storing:
				if flags.Changed("project") {
					if err := requireID(cmd, "--project", &projectID); err != nil {
						return err
					}
				}
			default:
				if err := requireID(cmd, "--blob", &blobID); err != nil {
					return err
				}
				for _, flag := range []string{"project", "name", "content-type"} {
					if flags.Changed(flag) {
						return usageErrorf(cmd, "--%s belongs to storing a file; the file --blob names is stored already", flag)
					}
				}
			}
			if flags.Changed("folder") {
				if err := requireID(cmd, "--folder", &folderID); err != nil {
					return err
				}
			}
			input := api.AddFilesUploadInput{FolderID: folderID}
			if flags.Changed("title") {
				// Minka refuses an empty title only after the file has travelled,
				// so it is refused here while that costs nothing.
				if strings.Trim(title, phpWhitespace) == "" {
					return &Error{
						Code:    CodeUsage,
						Message: "an upload needs a title, and an empty one is refused",
						Hint:    "Leave --title out to name the upload after the file.",
					}
				}
				input.Title = &title
			}
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}
			var notices []string
			if flags.Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description, notices = html, warnings
			}

			var file *fileToStore
			if storing {
				var err error
				if file, err = openFileToStore(args[0], name, contentType); err != nil {
					return err
				}
				defer file.close()
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			var stored *api.Result[api.Upload]
			if storing {
				projectOfTool, err := a.projectOfFilesTool(s, toolID)()
				if err != nil {
					return err
				}
				if projectID != "" {
					if projectID != projectOfTool {
						return &Error{
							Code:    CodeNotFound,
							Message: "the Docs & Files tool " + toolID + " stands in project " + projectOfTool + ", not in " + projectID + ", nothing was sent",
							Hint:    "--project is no longer needed: leave it out, and the file is stored where the tool stands.",
						}
					}
					notices = append(notices, "--project is no longer needed: the file is stored in the project the tool stands in")
				}
				uploaded, err := s.client.UploadToProject(a.ctx, projectOfTool, file.filename, file.mediaType, file.content, file.size)
				if err != nil {
					return storeRefusal(s, err)
				}
				stored, blobID = &uploaded, uploaded.Value.ID
			}
			input.BlobID = blobID

			upload, err := s.client.AddFilesUpload(a.ctx, toolID, input)
			if err != nil {
				e := filesUploadRefusal(s, err, toolID, blobID, stored)
				e.Notices = notices
				return e
			}

			summary := fmt.Sprintf("Put %s into %s as upload %s", upload.Value.File.Filename, upload.Value.Tool.Title, upload.Value.Title)
			return a.ok(success{
				data:        upload.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: uploadBreadcrumbs(upload.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nUpload: %s\nFile: %s\n%s\n", summary, upload.Value.ID, upload.Value.File.ID, upload.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the Docs & Files tool the file goes into")
	// A project named beside the tool is held against the tool's own, so a
	// script that names one keeps working and never stores the file anywhere
	// else.
	flags.StringVar(&projectID, "project", "", "ID of the project of the tool; no longer needed, and checked where given")
	_ = flags.MarkHidden("project")
	flags.StringVar(&blobID, "blob", "", "ID of a file stored already, in place of a file of this machine")
	flags.StringVar(&folderID, "folder", "", "ID of the folder to put it in; the top of the tool without it")
	flags.StringVar(&title, "title", "", "Title of the upload; the name of the file without it")
	flags.StringVar(&description, "description", "", "Description of the upload, - to read it from stdin")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	addFileToStoreFlags(cmd, &name, &contentType)

	return cmd
}

// phpWhitespace is what Minka trims off a title before it asks whether one is
// left.
const phpWhitespace = " \t\n\r\x00\x0b"

// projectOfFilesTool reads a tool for the project it stands in.
func (a *app) projectOfFilesTool(s *session, toolID string) func() (string, error) {
	return func() (string, error) {
		tool, err := s.client.FilesTool(a.ctx, toolID)
		if err != nil {
			return "", toolRefusal(s, err, filesToolNotFoundHint)
		}
		return tool.Value.ProjectID, nil
	}
}

// filesUploadRefusal classifies a file Minka did not put into a tool. stored
// is the file this run stored a moment before, and nil where --blob named it:
// a refusal then costs no transfer, and a file stored in this run cannot have
// been swept.
func filesUploadRefusal(s *session, err error, toolID, blobID string, stored *api.Result[api.Upload]) *Error {
	e := documentRefusal(s, err)
	switch {
	case e.Code == CodeNotFound:
		e.Hint = filesToolNotFoundHint
	case e.APICode == apiCodeFileUnknown && stored != nil:
		e.Hint = "The file was stored in this same run, so it has not been swept, and storing it again is not the remedy. Minka says no more about why it did not take it: check that the tool is still there with minka docs list " + toolID + ", and try once more with --blob " + blobID + "."
	case e.APICode == apiCodeFileUnknown:
		e.Hint = "Minka tells no cause apart: the file may have been swept after standing in nothing (such a file is kept for at least seven days and may go any time after), never have been stored, belong to another project, or be somebody else's. The one remedy worth trying is to store it again: give the file itself in place of --blob."
	case e.APICode == apiCodeFileInUse && stored != nil:
		e.Hint = "The file stands in an upload or a text already, and a file goes into one place only, so naming it again cannot help. Running the command again stores the file anew."
	case e.APICode == apiCodeFileInUse:
		e.Hint = "The file stands in an upload or a text already, and a file goes into one place only. Give the file itself in place of --blob to store it anew."
	}
	return handOverTheStoredFile(e, blobID, stored)
}

// handOverTheStoredFile makes the file this run stored the data of a refused
// second step, and names --blob where naming the file again can help.
func handOverTheStoredFile(e *Error, blobID string, stored *api.Result[api.Upload]) *Error {
	if stored == nil {
		return e
	}
	// The transfer is not lost: the file stands stored, and a second try can
	// name it rather than send it again.
	e.Data = stored.Raw
	// A file Minka does not know or has put somewhere already is no better
	// for being named again.
	if e.APICode != apiCodeFileUnknown && e.APICode != apiCodeFileInUse {
		e.Hint = strings.TrimSpace(e.Hint + " The file is stored as " + blobID + " and kept for at least seven days while nothing carries it: give --blob " + blobID + " in place of the file to try again without sending it anew.")
	}
	return e
}

func (a *app) newDocsUploadsEditCmd() *cobra.Command {
	var title, description, descriptionFormat, expect string
	var force, dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <uploadId>",
		Short: "Change the title or the description of an upload",
		Long: `Changes the title, the description or both. What is not given stays as it is.
The file of an upload stays whatever is sent: Minka ignores a file sent with a
change, so this command has no flag for it. docs uploads versions create gives
an upload another file.

--description "" empties the description. --title "" does not empty the title:
an upload needs one, so an empty title exits 9 (validation) with meta.apiCode
uploadTitleMissing. A title can only be replaced, never removed.

` + uploadDescriptionHelp + ` Of the people mentioned, only those the old
description did not mention yet are told.

Somebody else may have written the upload since you read it. Give the
updatedAt you read with --expect: an upload written over since is then left as
it stands, the command prints it and exits with 9 (meta.apiCode
uploadChanged). --force writes over whatever stands, and one of the two flags
is required. --expect none is not a value here: it belongs to the notes of a
card. The guard reaches only as far as Minka's own clock, the whole second, so
a change made in the same second as your read is not noticed.

Files and mentions stand in a description as action-text-attachment elements,
which Markdown cannot hold. To keep them, take the descriptionSource from docs
uploads show --json, change it and send it back with --description-format
html. A new description that would drop them is refused unless
--drop-attachments is given.`,
		Example: `  minka docs uploads edit 0199... --title "Site plan, final" --expect 2026-09-26T14:03:12.000000Z
  minka docs uploads edit 0199... --description - --expect 2026-09-26T14:03:12.000000Z < about.md
  minka docs uploads edit 0199... --description "" --force`,
		Args: requireArgs("uploadId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDescriptionFormat(cmd, descriptionFormat); err != nil {
				return err
			}
			guard, err := entryGuard(cmd, "upload", "minka docs uploads show <uploadId>", expect, force)
			if err != nil {
				return err
			}

			input := api.UpdateFilesUploadInput{ExpectedUpdatedAt: guard}
			var notices []string
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("description") {
				html, warnings, err := a.descriptionHTML(cmd, description, descriptionFormat)
				if err != nil {
					return err
				}
				input.Description, notices = &html, warnings
			}
			if input.Title == nil && input.Description == nil {
				return usageErrorf(cmd, "nothing to change, give --title or --description")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if input.Description != nil {
				// Read for the files and mentions the new description has to keep,
				// not for the guard, which stands for what the writer read.
				current, err := s.client.FilesUploadOf(a.ctx, args[0])
				if err != nil {
					return uploadRefusal(s, err)
				}
				dropped, err := a.guardOverFiles(s, current.Value.ProjectID, closedIf(current.Value.Archived), firstNonEmpty(current.Value.DescriptionSource, current.Value.Description), *input.Description, dropAttachments,
					"the upload has %s in its description, and the new one would drop them",
					"Change the descriptionSource from docs uploads show --json and send it back with --description-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			upload, err := s.client.UpdateFilesUpload(a.ctx, args[0], input)
			if err != nil {
				return entryConflict(a, uploadRefusal(s, err), err, apiCodeUploadChanged, "upload",
					func(u api.FilesUpload) string { return u.UpdatedAt }, renderFilesUpload)
			}

			if guard != nil {
				notices = append(notices, theGuardIsKeptToTheSecond)
			}
			summary := "Wrote over upload " + upload.Value.Title
			return a.ok(success{
				data:        upload.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: uploadBreadcrumbs(upload.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nTo write over it: --expect %s\n%s\n", summary, upload.Value.UpdatedAt, upload.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.StringVar(&description, "description", "", "New description, - to read it from stdin, empty to empty it")
	flags.StringVar(&descriptionFormat, "description-format", "markdown", "Format of the description: markdown or html")
	flags.StringVar(&expect, "expect", "", "The updatedAt of the upload you read")
	flags.BoolVar(&force, "force", false, "Write over the upload whatever stands")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a description that has attachments, mentions or references, which drops them")

	return cmd
}

// uploadRefusal classifies a failed request about one upload.
func uploadRefusal(s *session, err error) *Error {
	e := documentRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = uploadNotFoundHint
	}
	return e
}

func (a *app) newDocsCommentsCmd() *cobra.Command {
	return group("comments", "Write, change, remove and react to comments on a document or an upload",
		a.newDocsCommentsCreateCmd(),
		a.newDocsCommentsEditCmd(),
		a.newDocsCommentsDeleteCmd(),
		a.newDocsCommentsReactCmd(),
		a.newDocsCommentsUnreactCmd(),
	)
}

// entryFlagHelp says why the entry a comment hangs on is named by a flag: a
// document and an upload have addresses of their own, and the identifier of
// one is nothing at the address of the other.
const entryFlagHelp = `The entry is named by --document <documentId> or --upload <uploadId>, exactly
one of the two. The two kinds have addresses of their own, and the identifier
of one is nothing at the address of the other.`

func (a *app) newDocsCommentsCreateCmd() *cobra.Command {
	var documentID, uploadID, format string

	cmd := &cobra.Command{
		Use:   "create <text|->",
		Short: "Comment on a document or an upload",
		Long:  "Writes a comment under a document or under an upload.\n\n" + entryFlagHelp + "\n\n" + commentFormatHelp,
		Example: `  minka docs comments create --document 0199... "Looks right to me"
  minka docs comments create --upload 0199... - < comment.md`,
		Args: requireArgs("text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := entryOfFlags(cmd, documentID, uploadID)
			if err != nil {
				return err
			}
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[0])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.WriteCommentOn(a.ctx, entry, body)
			if err != nil {
				return documentRefusal(s, err)
			}

			summary := "Commented on " + string(entry.Kind) + " " + entry.ID
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { writtenComment(w, summary, comment.Value) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")

	return cmd
}

func (a *app) newDocsCommentsEditCmd() *cobra.Command {
	var documentID, uploadID, format string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <commentId> <text|->",
		Short: "Change a comment you wrote on a document or an upload",
		Long: "Changes the text of a comment. Only its author can.\n\n" + entryFlagHelp + "\n\n" + commentFormatHelp + `

Files and mentions stand in a comment as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource of the comment from
docs show --json, change it and send it back with --format html. A new text
that would drop them is refused unless --drop-attachments is given.`,
		Example: `  minka docs comments edit --document 0199... 0199... "Looks right after all"`,
		Args:    requireArgs("commentId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := entryOfFlags(cmd, documentID, uploadID)
			if err != nil {
				return err
			}
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[1])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			under, err := a.entryUnder(s, entry)
			if err != nil {
				return err
			}
			dropped, err := a.keepsTheAttachmentsOfAComment(s, under.projectID, a.closedToComments(s, under), under.comments, args[0], body, dropAttachments,
				"Change the bodySource of the comment from docs show --json and send it back with --format html, or give --drop-attachments.")
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)

			comment, err := s.client.EditCommentOn(a.ctx, entry, args[0], body)
			if err != nil {
				return commentRefusal(s, err)
			}

			summary := fmt.Sprintf("Changed comment %s on %s %s", args[0], entry.Kind, entry.ID)
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { writtenComment(w, summary, comment.Value) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments, mentions or references, which drops them")

	return cmd
}

func (a *app) newDocsCommentsDeleteCmd() *cobra.Command {
	var documentID, uploadID string

	cmd := &cobra.Command{
		Use:   "delete <commentId>",
		Short: "Remove a comment you wrote on a document or an upload",
		Long: "Removes a comment. Only its author can. A removed comment is gone, not\n" +
			"marked: removing it again exits with 2 (not_found), like one that never\n" +
			"existed.\n\n" + entryFlagHelp,
		Args: requireArgs("commentId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := entryOfFlags(cmd, documentID, uploadID)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteCommentOn(a.ctx, entry, args[0]); err != nil {
				return commentRefusal(s, err)
			}

			summary := fmt.Sprintf("Removed comment %s from %s %s", args[0], entry.Kind, entry.ID)
			return a.ok(success{
				data:        map[string]any{string(entry.Kind) + "Id": entry.ID, "commentId": args[0], "deleted": true},
				summary:     summary,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)

	return cmd
}

func (a *app) newDocsCommentsReactCmd() *cobra.Command {
	var documentID, uploadID string

	cmd := &cobra.Command{
		Use:   "react <commentId> <content>",
		Short: "React to a comment on a document or an upload",
		Long: "Leaves a reaction on a comment: mostly an emoji, at most 20 characters.\n" +
			"The same reaction of yours a second time changes nothing.\n\n" + entryFlagHelp + `

Only the comments take reactions. A document and an upload themselves do not,
although the web offers it.`,
		Args: requireArgs("commentId", "content"),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := entryOfFlags(cmd, documentID, uploadID)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.ReactToCommentOn(a.ctx, entry, args[0], args[1])
			if err != nil {
				return commentRefusal(s, err)
			}

			summary := fmt.Sprintf("Reacted %s to comment %s", args[1], args[0])
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)

	return cmd
}

func (a *app) newDocsCommentsUnreactCmd() *cobra.Command {
	var documentID, uploadID string

	cmd := &cobra.Command{
		Use:   "unreact <commentId> <content>",
		Short: "Take a reaction off a comment on a document or an upload",
		Long: "Takes one reaction off a comment: your own, and where you left none the\n" +
			"last one somebody else left. Taking away what is not there succeeds as\n" +
			"well.\n\n" + entryFlagHelp,
		Args: requireArgs("commentId", "content"),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := entryOfFlags(cmd, documentID, uploadID)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.UnreactFromCommentOn(a.ctx, entry, args[0], args[1]); err != nil {
				return commentRefusal(s, err)
			}

			summary := fmt.Sprintf("Took %s off comment %s", args[1], args[0])
			return a.ok(success{
				data:        map[string]any{string(entry.Kind) + "Id": entry.ID, "commentId": args[0], "content": args[1]},
				summary:     summary,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)

	return cmd
}

func addEntryFlags(cmd *cobra.Command, documentID, uploadID *string) {
	cmd.Flags().StringVar(documentID, "document", "", "ID of the document the comment hangs on")
	cmd.Flags().StringVar(uploadID, "upload", "", "ID of the upload the comment hangs on")
}

// entryOfFlags is the document or the upload a command is aimed at. Where one
// position would carry two kinds of identifier, a flag names which of them it
// is instead.
func entryOfFlags(cmd *cobra.Command, documentID, uploadID string) (api.Commented, error) {
	switch {
	case documentID != "" && uploadID != "":
		return api.Commented{}, usageErrorf(cmd, "--document and --upload exclude each other")
	case documentID != "":
		if err := requireID(cmd, "--document", &documentID); err != nil {
			return api.Commented{}, err
		}
		return api.Commented{Kind: api.CommentedDocument, ID: documentID}, nil
	case uploadID != "":
		if err := requireID(cmd, "--upload", &uploadID); err != nil {
			return api.Commented{}, err
		}
		return api.Commented{Kind: api.CommentedUpload, ID: uploadID}, nil
	default:
		return api.Commented{}, usageErrorf(cmd, "say what the comment hangs on: --document <documentId> or --upload <uploadId>")
	}
}

// commentedEntry is what a change of a comment has to know beforehand of the
// entry it hangs on: whether the text it replaces carries files or mentions
// that Markdown cannot hold, and whether Minka would take the change at all.
type commentedEntry struct {
	projectID string
	toolID    string
	folder    *api.FolderReference
	archived  bool
	comments  []api.Comment
}

func (a *app) entryUnder(s *session, entry api.Commented) (commentedEntry, error) {
	if entry.Kind == api.CommentedUpload {
		upload, err := s.client.FilesUploadOf(a.ctx, entry.ID)
		if err != nil {
			return commentedEntry{}, documentRefusal(s, err)
		}
		u := upload.Value
		return commentedEntry{projectID: u.ProjectID, toolID: u.ToolID, folder: u.Folder, archived: u.Archived, comments: u.Comments}, nil
	}
	document, err := s.client.Document(a.ctx, entry.ID)
	if err != nil {
		return commentedEntry{}, documentRefusal(s, err)
	}
	d := document.Value
	return commentedEntry{projectID: d.ProjectID, toolID: d.ToolID, folder: d.Folder, archived: d.Archived, comments: d.Comments}, nil
}

// closedToComments says whether Minka refuses every change of a comment under
// an entry. An entry of the archive is refused by every Minka; one that lies
// there only through a folder above it is refused only by a Minka that asks
// the folders as well, and one that asks the entry alone changes the comment,
// which loses its files. The entry says archived for either, so where it
// stands in a folder the folder is read to tell the two apart, and below an
// archived folder the guard stays. Against a Minka that refuses there, that
// costs the read of the folder and of the archived projects and a second run
// with the flag to reach the refusal; a guard too few costs the files.
func (a *app) closedToComments(s *session, entry commentedEntry) func() (bool, error) {
	return func() (bool, error) {
		if !entry.archived || entry.folder == nil {
			return entry.archived, nil
		}
		folder, err := s.client.FilesEntriesOf(a.ctx, entry.toolID, entry.folder.ID)
		if err != nil {
			e := s.fail(err)
			if e.Code == CodeNotFound {
				e.Hint = "The folder the entry stands in is gone since the entry was read. Read it again with docs show."
			}
			return false, e
		}
		return !folder.Value.Archived, nil
	}
}

// keepsTheAttachments refuses a text that would drop the files, mentions or
// references of the one it replaces, unless that was asked for. Markdown
// cannot hold a file or a mention, so only a text that carries them keeps
// them; a reference it keeps as the address its view shows. The message names
// what would be lost where it says %s.
func keepsTheAttachments(current, replacement string, allowed bool, message, hint string) ([]string, error) {
	return refusedLoss(richtext.Dropped(current, replacement), allowed, message, hint)
}

// refusedLoss turns what a new text would drop into the refusal, or into the
// notice where dropping was asked for.
func refusedLoss(loss richtext.Loss, allowed bool, message, hint string) ([]string, error) {
	if !loss.Any() {
		return nil, nil
	}
	if !allowed {
		if loss.References {
			hint += " A reference is kept by writing its address as <https://…> or a card as #MNK-1; one you cannot open only by sending the source back."
		}
		return nil, &Error{Code: CodeUsage, Message: fmt.Sprintf(message, loss), Hint: hint}
	}
	return []string{"the " + loss.String() + " of the text were dropped"}, nil
}

// guardOverFiles holds a new text against the one it replaces, and stands
// aside where Minka refuses the write whatever is sent: nothing can be lost
// there, and Minka's refusal says what is really wrong where the guard's would
// point at a flag that cannot help. What the read of the text shows is asked
// through closed; whether the project is archived no such read says. Both are
// asked only where the guard is about to refuse.
func (a *app) guardOverFiles(s *session, projectID string, closed func() (bool, error), current, replacement string, allowed bool, message, hint string) ([]string, error) {
	dropped, refusal := keepsTheAttachments(current, replacement, allowed, message, hint)
	if refusal == nil {
		return dropped, nil
	}
	refusedAnyway, err := closed()
	if err != nil || refusedAnyway {
		return nil, err
	}
	archived, err := s.client.Projects(a.ctx, true)
	if err != nil {
		return nil, s.fail(err)
	}
	for _, project := range archived.Value {
		if project.ID == projectID && project.Status == "archived" {
			return nil, nil
		}
	}
	return nil, refusal
}

// closedIf is what a read has already said about whether the place a text
// stands in takes any write.
func closedIf(archived bool) func() (bool, error) {
	return func() (bool, error) { return archived, nil }
}

// keepsTheAttachmentsOfAComment holds a new text against the comment it
// replaces. Only its author changes a comment, and Minka refuses one of
// somebody else whatever is sent, so there its answer comes and not the guard.
// Whose the comment is gets asked only where the guard is about to refuse.
func (a *app) keepsTheAttachmentsOfAComment(s *session, projectID string, closed func() (bool, error), comments []api.Comment, commentID, body string, allowed bool, hint string) ([]string, error) {
	const message = "the comment has %s, and the new text would drop them"
	for _, current := range comments {
		if current.ID != commentID {
			continue
		}
		text := firstNonEmpty(current.BodySource, current.Body)
		dropped, refusal := keepsTheAttachments(text, body, allowed, message, hint)
		if refusal == nil {
			return dropped, nil
		}
		me, err := a.meID(s)()
		if err != nil {
			return nil, err
		}
		if current.Author.ID != me {
			return nil, nil
		}
		return a.guardOverFiles(s, projectID, closed, text, body, allowed, message, hint)
	}
	return nil, nil
}

func checkBodyFormat(cmd *cobra.Command, format string) error {
	if format != "markdown" && format != "html" {
		return usageErrorf(cmd, "--body-format is markdown or html, not %q", format)
	}
	return nil
}

// documentBody reads and converts the body of a document.
//
// The conversion happens here because Minka refuses nothing: a body without an
// element it serves is kept as the characters it consists of, and the answer is
// the same 201 or 200 as any other. Where somebody sends HTML themselves, the
// notice is the only word anybody gets about it.
func (a *app) documentBody(value, format string) (string, []string, error) {
	text, err := a.readValue("the body", value)
	if err != nil {
		return "", nil, err
	}
	if format == "html" {
		return text, markupNotice(text), nil
	}

	result := markdown.ToHTML(text)
	if result.HTML == "" && strings.TrimSpace(text) != "" {
		return "", nil, &Error{
			Code:    CodeUsage,
			Message: "the body has nothing Minka keeps and would empty the body",
			Hint:    `To empty the body on purpose, give --body "".`,
		}
	}
	return result.HTML, result.Warnings, nil
}

// entryGuard turns --expect and --force into what a write on a document or an
// upload sends: the moment the writer read, or nothing at all. kind names the
// entry and show the command that reads it.
//
// It shares no rule with the notes of a card. There "--expect none" sends
// expectedUpdatedAt null and means that nobody has written any notes yet; on an
// entry null says that nothing is expected. A write that expects nothing
// therefore leaves the field out.
func entryGuard(cmd *cobra.Command, kind, show, expect string, force bool) (*string, error) {
	expected := cmd.Flags().Changed("expect")
	switch {
	case expected && force:
		return nil, usageErrorf(cmd, "--expect and --force exclude each other")
	case force:
		return nil, nil
	case !expected:
		return nil, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("say which %s the change may write over: give --expect or --force", kind),
			Hint:    show + " gives the updatedAt for --expect.",
		}
	case expect == expectNone:
		hint := show + " gives the updatedAt to copy."
		// A command that writes without an expectation by leaving --expect out
		// has no --force to name.
		if cmd.Flags().Lookup("force") != nil {
			hint = show + " gives the updatedAt to copy; --force writes over whatever stands."
		}
		return nil, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("--expect none belongs to the notes of a card, where it means that nobody has written any; %s %s always has an updatedAt", articleOf(kind), kind),
			Hint:    hint,
		}
	case !isATimeMinkaReads(expect):
		return nil, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("--expect takes the updatedAt of the %s as Minka gives it, such as 2026-09-26T14:03:12.000000Z; %q is not", kind, expect),
			Hint:    show + " gives the updatedAt to copy.",
		}
	}
	return &expect, nil
}

// articleOf is the indefinite article that goes before a word.
func articleOf(word string) string {
	if strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}

// documentRefusal classifies a failed request about a Docs & Files tool. Each
// code names a state the entry, its folder or somebody named on it is in, and
// the hint says what can be done about it — including where that is nothing.
func documentRefusal(s *session, err error) *Error {
	e := s.fail(err)
	if hint, ok := audienceHint(e.APICode); ok {
		e.Hint = hint
		return e
	}
	switch {
	case e.APICode == apiCodePersonCannotSeeIt:
		e.Hint = "Nothing was written, and the same request will be refused again: somebody named with --notify is a client of the project, or an agent working for one, and is told only about a document in a folder shown to clients, in a tool shown to clients. Name somebody else. Which tools and folders the clients are shown is decided in Minka on the web."
	case e.APICode == apiCodeDocumentArchived:
		e.Hint = "The document, or a folder above it, lies in the archive. It can be read; nothing is written on it until it is brought back with minka docs unarchive <documentId>, or where a folder above it is archived, minka docs folders unarchive <folderId>."
	case e.APICode == apiCodeUploadArchived:
		e.Hint = "The upload, or a folder above it, lies in the archive. It can be read; nothing is written on it until it is brought back with minka docs uploads unarchive <uploadId>, or where a folder above it is archived, minka docs folders unarchive <folderId>."
	case e.APICode == apiCodeFolderUnknown:
		e.Hint = "This Docs & Files tool has no such folder, it is in the trash, or you are a client of the project and it is not shown to you. List the folders with: minka docs list <toolId>"
	case e.APICode == apiCodeFolderArchived:
		e.Hint = "The folder, or one above it, lies in the archive and takes nothing new. Write at the top of the tool or in another folder, or bring the folder back first: minka docs folders unarchive <folderId>"
	case e.Code == CodeNotFound:
		e.Hint = documentNotFoundHint
	}
	return e
}

// commentRefusal is a refusal of a write on one comment. An address that has
// nothing may be the entry, the comment or its author, and a document says
// which of them it is not. An archived entry closes its comments to every
// write, not only to new ones.
func commentRefusal(s *session, err error) *Error {
	e := documentRefusal(s, err)
	switch {
	case e.APICode == apiCodeDocumentArchived:
		e.Hint = "The document, or a folder above it, lies in the archive. Its comments can be read, but not changed, removed or reacted to until it is brought back with minka docs unarchive <documentId>, or where a folder above it is archived, minka docs folders unarchive <folderId>."
	case e.APICode == apiCodeUploadArchived:
		e.Hint = "The upload, or a folder above it, lies in the archive. Its comments can be read, but not changed, removed or reacted to until it is brought back with minka docs uploads unarchive <uploadId>, or where a folder above it is archived, minka docs folders unarchive <folderId>."
	case e.Code == CodeNotFound:
		e.Hint = "The document or upload does not exist or is not visible to you, the comment was removed, or somebody else wrote it: only the author can change or remove a comment."
	}
	return e
}

// entryConflict adds to a write refused as code what a writer needs to merge:
// the entry as it stands, which Minka sends under kind, as the data of the
// error and printed for a person.
func entryConflict[T any](a *app, e *Error, err error, code, kind string, updatedAt func(T) string, render func(io.Writer, T)) *Error {
	var apiErr *api.Error
	if e.APICode != code || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return e
	}
	var body map[string]json.RawMessage
	var current T
	if json.Unmarshal(apiErr.Body, &body) != nil || json.Unmarshal(body[kind], &current) != nil {
		return e
	}

	e.Message = "the " + kind + " was written over since it was read, nothing was written"
	e.Data = body[kind]
	e.Hint = "Merge your change into the " + kind + " as it stands and send it again with --expect " + updatedAt(current) + "."
	e.Text = func(w io.Writer) {
		fmt.Fprintf(a.stderr, "The %s as it stands:\n\n", kind)
		render(w, current)
	}
	return e
}

func renderFilesTools(w io.Writer, tools []api.FilesTool) {
	if len(tools) == 0 {
		fmt.Fprintln(w, "No Docs & Files tools.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE")
	for _, tool := range tools {
		fmt.Fprintf(table, "%s\t%s\n", tool.ID, tool.Title)
	}
	_ = table.Flush()
}

func renderFilesEntries(w io.Writer, entries api.FilesEntries) {
	fmt.Fprintf(w, "# %s\n", whereTheEntriesStand(entries))
	fmt.Fprintf(w, "Tool: %s\n", entries.Tool.ID)
	if entries.Folder != nil {
		fmt.Fprintf(w, "Folder: %s\n", entries.Folder.ID)
		fmt.Fprintf(w, "Path: %s\n", pathOfFolders(entries.Path))
	}
	// A folder in the archive takes no change, and neither does anything in it,
	// so the line stands here rather than on every entry below.
	if entries.Archived {
		fmt.Fprintln(w, "Archived: yes, this folder or one above it")
	}
	switch {
	case entries.IsBook:
		fmt.Fprintln(w, "Book: yes, this folder")
	case entries.Book != nil:
		fmt.Fprintf(w, "Book: %s (%s)\n", entries.Book.Title, entries.Book.ID)
	}
	if entries.Order != "" {
		fmt.Fprintf(w, "Order: %s\n", entries.Order)
	}
	fmt.Fprintf(w, "URL: %s\n", entries.AppURL)

	// Arranged by hand, the three kinds stand mixed, and only one list keeps
	// the order the web shows.
	if entries.Order == orderManual {
		renderEntriesArrangedByHand(w, entries)
		return
	}

	fmt.Fprintf(w, "\n## Folders (%d)\n\n", len(entries.Folders))
	if len(entries.Folders) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, folder := range entries.Folders {
		renderFolderLine(w, folder, "")
	}

	fmt.Fprintf(w, "\n## Documents (%d)\n\n", len(entries.Documents))
	if len(entries.Documents) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, document := range entries.Documents {
		renderDocumentLine(w, document, "")
	}

	fmt.Fprintf(w, "\n## Files (%d)\n\n", len(entries.Uploads))
	if len(entries.Uploads) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, upload := range entries.Uploads {
		renderUploadLine(w, upload, "")
	}
}

// renderEntriesArrangedByHand lists the three kinds in the one order the
// place keeps, each line naming its kind.
func renderEntriesArrangedByHand(w io.Writer, entries api.FilesEntries) {
	folders := map[string]api.FolderListItem{}
	for _, folder := range entries.Folders {
		folders[folder.ID] = folder
	}
	documents := map[string]api.DocumentListItem{}
	for _, document := range entries.Documents {
		documents[document.ID] = document
	}
	uploads := map[string]api.FilesUploadListItem{}
	for _, upload := range entries.Uploads {
		uploads[upload.ID] = upload
	}

	fmt.Fprintf(w, "\n## Entries (%d), in the order set by hand\n\n", len(entries.Entries))
	if len(entries.Entries) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, entry := range entries.Entries {
		switch entry.Kind {
		case "folder":
			renderFolderLine(w, folders[entry.ID], "folder ")
		case "document":
			renderDocumentLine(w, documents[entry.ID], "document ")
		case "upload":
			renderUploadLine(w, uploads[entry.ID], "file ")
		}
	}
}

func renderFolderLine(w io.Writer, folder api.FolderListItem, kind string) {
	book := ""
	if folder.IsBook {
		book = ", book"
	}
	fmt.Fprintf(w, "- %s  %s%s (%s%s)\n", folder.ID, kind, folder.Title, plural(folder.EntryCount, "entry", "entries"), book)
}

func renderDocumentLine(w io.Writer, document api.DocumentListItem, kind string) {
	details := entryDetails(document.CommentCount, document.Archived)
	if document.InNavigation != nil && !*document.InNavigation {
		details = entryDetails(document.CommentCount, document.Archived, "not in navigation")
	}
	fmt.Fprintf(w, "- %s  %s%s%s\n", document.ID, kind, document.Title, details)
	if document.Lead != "" {
		fmt.Fprintf(w, "  %s\n", oneLine(document.Lead))
	}
}

func renderUploadLine(w io.Writer, upload api.FilesUploadListItem, kind string) {
	fmt.Fprintf(w, "- %s  %s%s%s\n", upload.ID, kind, upload.Title, entryDetails(upload.CommentCount, upload.Archived))
	fmt.Fprintf(w, "  %s, %s, file %s\n", upload.File.ContentType, fileSize(upload.File.ByteSize), upload.File.ID)
}

// entryDetails says what a line of a folder has to add: how much was said
// under it, whether it takes any change at all, and what else holds of it.
func entryDetails(commentCount int, archived bool, more ...string) string {
	var details []string
	if commentCount > 0 {
		details = append(details, plural(commentCount, "comment", "comments"))
	}
	if archived {
		details = append(details, "archived")
	}
	details = append(details, more...)
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func renderDocument(w io.Writer, document api.Document) {
	fmt.Fprintf(w, "# %s\n", document.Title)
	fmt.Fprintf(w, "Document: %s\nTool: %s\nProject: %s\n", document.ID, document.ToolID, document.ProjectID)
	renderEntryPlace(w, document.Folder, document.Path, document.Archived)
	if document.Book != nil {
		fmt.Fprintf(w, "Book: %s (%s)\n", document.Book.Title, document.Book.ID)
	}
	if document.InNavigation != nil && !*document.InNavigation {
		fmt.Fprintln(w, "Navigation: left out of the navigation of its book")
	}
	fmt.Fprintf(w, "Created: %s by %s\n", formatTime(document.CreatedAt), document.Creator.Name)
	if document.EditedAt != nil {
		fmt.Fprintf(w, "Edited: %s\n", formatTime(*document.EditedAt))
	}
	fmt.Fprintf(w, "To write over it: --expect %s\n", document.UpdatedAt)
	fmt.Fprintf(w, "URL: %s\n", document.AppURL)

	if body := richtext.ToMarkdown(firstNonEmpty(document.BodySource, document.Body)); body != "" {
		fmt.Fprintf(w, "\n## Body\n\n%s\n", body)
	}
	renderEntryFiles(w, document.Attachments)
	renderEntryComments(w, document.Comments)
}

func renderFilesUpload(w io.Writer, upload api.FilesUpload) {
	fmt.Fprintf(w, "# %s\n", upload.Title)
	fmt.Fprintf(w, "Upload: %s\nTool: %s\nProject: %s\n", upload.ID, upload.ToolID, upload.ProjectID)
	renderEntryPlace(w, upload.Folder, upload.Path, upload.Archived)
	fmt.Fprintf(w, "File: %s (%s, %s, %s)\n", upload.File.Filename, upload.File.ContentType, fileSize(upload.File.ByteSize), upload.File.ID)
	fmt.Fprintf(w, "Created: %s by %s\n", formatTime(upload.CreatedAt), upload.Creator.Name)
	if upload.EditedAt != nil {
		fmt.Fprintf(w, "Edited: %s\n", formatTime(*upload.EditedAt))
	}
	fmt.Fprintf(w, "URL: %s\n", upload.AppURL)

	if description := richtext.ToMarkdown(firstNonEmpty(upload.DescriptionSource, upload.Description)); description != "" {
		fmt.Fprintf(w, "\n## Description\n\n%s\n", description)
	}
	renderEntryFiles(w, upload.Attachments)
	renderEntryComments(w, upload.Comments)
}

// renderEntryPlace says where an entry stands. An entry in the archive takes
// no change, and the line says so where it holds rather than everywhere.
func renderEntryPlace(w io.Writer, folder *api.FolderReference, path []api.FolderReference, archived bool) {
	if folder == nil {
		fmt.Fprintln(w, "Folder: at the top of the tool")
	} else {
		fmt.Fprintf(w, "Folder: %s (%s)\n", pathOfFolders(path), folder.ID)
	}
	if archived {
		fmt.Fprintln(w, "Archived: yes, it or a folder above it")
	}
}

func renderEntryFiles(w io.Writer, attachments []api.Attachment) {
	if len(attachments) == 0 {
		return
	}
	fmt.Fprintf(w, "\n## Files (%d)\n\n", len(attachments))
	for _, file := range attachments {
		fmt.Fprintf(w, "- %s  %s (%s, %s)\n", file.ID, file.Filename, file.ContentType, fileSize(file.ByteSize))
	}
}

func renderEntryComments(w io.Writer, comments []api.Comment) {
	if len(comments) == 0 {
		return
	}
	fmt.Fprintf(w, "\n## Comments (%d)\n", len(comments))
	for _, comment := range comments {
		edited := ""
		if comment.EditedAt != nil {
			edited = ", edited"
		}
		body := firstNonEmpty(comment.BodySource, comment.Body)
		if richtext.CarriesMarkup(body) {
			body = richtext.ToMarkdown(body)
		}
		fmt.Fprintf(w, "\n### %s  %s, %s%s\n\n%s\n", comment.ID, comment.Author.Name, formatTime(comment.CreatedAt), edited, body)
		if line := fileLine(comment.Attachments); line != "" {
			fmt.Fprintf(w, "%s\n", line)
		}
		if line := reactionLine(comment.Reactions); line != "" {
			fmt.Fprintf(w, "%s\n", line)
		}
		if line := commentAddressLine(comment); line != "" {
			fmt.Fprintf(w, "%s\n", line)
		}
	}
}

// pathOfFolders is the way down to a folder, written as the web writes it.
func pathOfFolders(path []api.FolderReference) string {
	titles := make([]string, len(path))
	for i, folder := range path {
		titles[i] = folder.Title
	}
	return strings.Join(titles, " / ")
}

// oneLine keeps a lead to one line: it is cut out of a body, and a line break
// in it would read as a line of the listing.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func whereTheEntriesStand(entries api.FilesEntries) string {
	if entries.Folder == nil {
		return entries.Tool.Title
	}
	return entries.Folder.Title
}

func entriesSummary(entries api.FilesEntries) string {
	summary := fmt.Sprintf("%s, %s and %s",
		plural(len(entries.Folders), "folder", "folders"),
		plural(len(entries.Documents), "document", "documents"),
		plural(len(entries.Uploads), "file", "files"))
	if entries.Archived {
		summary += ", in the archive"
	}
	return summary
}

func documentSummary(document api.Document) string {
	if document.Archived {
		return "Document " + document.Title + " in the archive of " + document.Tool.Title
	}
	return "Document " + document.Title + " in " + document.Tool.Title
}

func uploadSummary(upload api.FilesUpload) string {
	if upload.Archived {
		return "Upload " + upload.Title + " in the archive of " + upload.Tool.Title
	}
	return "Upload " + upload.Title + " in " + upload.Tool.Title
}

func entriesBreadcrumbs(toolID string, entries api.FilesEntries) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka docs show <documentId>", Description: "Read a document with its body and comments"},
		{Action: "upload", Cmd: "minka docs uploads show <uploadId>", Description: "Read an upload with its description and comments"},
	}
	if len(entries.Folders) > 0 {
		crumbs = append(crumbs, Breadcrumb{
			Action:      "folder",
			Cmd:         "minka docs list " + toolID + " --folder " + entries.Folders[0].ID,
			Description: "List what stands in a folder",
		})
	}
	return append(crumbs, Breadcrumb{
		Action:      "create",
		Cmd:         "minka docs create --tool " + toolID + " --title <title> --audience nobody",
		Description: "Write a document here",
	}, Breadcrumb{
		Action:      "put-file",
		Cmd:         "minka docs uploads create <file> --tool " + toolID,
		Description: "Put a file here",
	})
}

func documentBreadcrumbs(document api.Document) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "edit", Cmd: "minka docs edit " + document.ID + " --body - --expect " + document.UpdatedAt, Description: "Write over the document, unless somebody wrote since"},
		{Action: "comment", Cmd: "minka docs comments create --document " + document.ID + " <text|->", Description: "Comment on the document"},
		{Action: "versions", Cmd: "minka docs versions list " + document.ID, Description: "List the earlier versions of the document"},
		{Action: "folder", Cmd: "minka docs list " + document.ToolID + folderFlagOf(document.Folder), Description: "List what stands beside it"},
	}
	if document.Book != nil {
		crumbs = append(crumbs, Breadcrumb{Action: "book", Cmd: "minka docs book " + document.Book.ID, Description: "Read the book it stands in as one tree"})
	}
	return crumbs
}

func uploadBreadcrumbs(upload api.FilesUpload) []Breadcrumb {
	return []Breadcrumb{
		{Action: "edit", Cmd: "minka docs uploads edit " + upload.ID + " --description - --expect " + upload.UpdatedAt, Description: "Write over the upload, unless somebody wrote since"},
		{Action: "download", Cmd: "minka uploads download " + upload.File.ID, Description: "Fetch the bytes of the file"},
		{Action: "comment", Cmd: "minka docs comments create --upload " + upload.ID + " <text|->", Description: "Comment on the upload"},
		{Action: "replace", Cmd: "minka docs uploads versions create " + upload.ID + " <file>", Description: "Give the upload another file, keeping this one as an earlier version"},
		{Action: "folder", Cmd: "minka docs list " + upload.ToolID + folderFlagOf(upload.Folder), Description: "List what stands beside it"},
	}
}

// entryBreadcrumbs name what follows a write on a comment: reading the entry
// it hangs under, whichever kind it is.
func entryBreadcrumbs(entry api.Commented) []Breadcrumb {
	if entry.Kind == api.CommentedUpload {
		return []Breadcrumb{
			{Action: "show", Cmd: "minka docs uploads show " + entry.ID, Description: "Read the upload with its comments"},
		}
	}
	return []Breadcrumb{
		{Action: "show", Cmd: "minka docs show " + entry.ID, Description: "Read the document with its comments"},
	}
}

func folderFlagOf(folder *api.FolderReference) string {
	if folder == nil {
		return ""
	}
	return " --folder " + folder.ID
}
