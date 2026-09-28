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
	apiCodeDocumentChanged                = "documentChanged"
	apiCodeDocumentArchived               = "documentArchived"
	apiCodeUploadArchived                 = "uploadArchived"
	apiCodeFolderUnknown                  = "folderUnknown"
	apiCodeFolderArchived                 = "folderArchived"
	apiCodeNotifyIDsMissing               = "notifyIdsMissing"
	apiCodeNotifyIDsWithoutChosenAudience = "notifyIdsWithoutChosenAudience"
	apiCodePersonCannotSeeIt              = "personCannotSeeIt"
)

// theGuardIsKeptToTheSecond is what a guard on an entry cannot do. Minka keeps
// the moment of a document and of an upload to the whole second, so a change
// made in the same second as the read carries the moment that was read and
// passes for it.
const theGuardIsKeptToTheSecond = "the guard is kept to the whole second: a change somebody else made in the same second as your read is not noticed, so read the document back after a write that matters"

const documentBodyHelp = `The body is Markdown by default and is sent as HTML, the way a description is.
Minka keeps headings, bold, italic, strikethrough, inline code, links, lists,
quotes, code blocks with their language, tables and horizontal rules. Images
become links and links Minka does not keep become plain text, each with a
warning. Use --body - to read it from stdin.

--body-format html sends HTML as it is. Minka refuses no body: one that
carries no element it serves is kept as the characters it consists of, so
Markdown sent that way stands in the document afterwards as "# Heading". The
command says so in a notice, and nothing else ever will.`

const documentAudienceHelp = `--audience says whom a new document reaches: "everybody" in the project,
"chosen" with one --notify per person, or "nobody". It is required. --notify
belongs to "chosen" alone: Minka refuses names under any other audience with
notifyIdsWithoutChosenAudience and "chosen" without a name with
notifyIdsMissing; this command sends what it is given and leaves the rule
to Minka.
A person is given by ID, or as "me"; minka people list <projectId> gives the
IDs.

Somebody named has to be able to see the document. A client of the project,
or an agent working for one, is told only about a document in a folder shown to
clients, in a tool shown to clients; the topmost folder decides for everything
below it. Naming them anywhere else is refused with personCannotSeeIt. Nothing
is written then, and sending it again changes nothing: name somebody else.`

const documentNotFoundHint = "There is no such document, it is a draft, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."

const uploadNotFoundHint = "There is no such upload, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."

func (a *app) newDocsCmd() *cobra.Command {
	return group("docs", "Read and write the documents of a Docs & Files tool, and comment on its entries",
		a.newDocsToolsCmd(),
		a.newDocsListCmd(),
		a.newDocsShowCmd(),
		a.newDocsCreateCmd(),
		a.newDocsEditCmd(),
		a.newDocsUploadsCmd(),
		a.newDocsCommentsCmd(),
	)
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

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List what stands at the top of a Docs & Files tool, or in one of its folders",
		Long: `Lists the folders, documents and uploads of a Docs & Files tool: those at the
top, or those in the folder --folder names. It reaches one folder deep, so
going down means calling it again with a folder of the answer.

Nothing here is paged and nothing is cut. The order is the one the web shows,
which is neither alphabetical nor by date. Drafts and entries in the archive
are not listed, and neither is a folder in the archive — but the inside of one
can be listed by naming it, and then the answer and every line of it says
archived: true.

Every document carries a lead: the first 240 characters of its body without
markup, cut where the count runs out and not at a word.`,
		Example: `  minka docs list 0199... --json
  minka docs list 0199... --folder 0199... --json`,
		Args: requireArgs("toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("folder") {
				if err := requireID(cmd, "--folder", folderID); err != nil {
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
			if err := requireID(cmd, "--tool", toolID); err != nil {
				return err
			}
			if cmd.Flags().Changed("folder") {
				if err := requireID(cmd, "--folder", folderID); err != nil {
					return err
				}
			}
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			if err := checkAudience(cmd, audience); err != nil {
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
			if input.NotifyIDs, err = a.notifiedPeople(cmd, s, notify); err != nil {
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
	flags.StringArrayVar(&notify, "notify", nil, "Person to tell about the document, with --audience chosen; can be given more than once")

	return cmd
}

func (a *app) newDocsEditCmd() *cobra.Command {
	var title, body, bodyFormat, expect string
	var force, dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <documentId>",
		Short: "Change the title or the body of a document",
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
them is refused unless --drop-attachments is given.`,
		Example: `  minka docs edit 0199... --title "How the stage goes up" --expect 2026-09-26T14:03:12.000000Z
  minka docs edit 0199... --body - --expect 2026-09-26T14:03:12.000000Z < body.md
  minka docs edit 0199... --body "" --force`,
		Args: requireArgs("documentId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			guard, err := documentGuard(cmd, expect, force)
			if err != nil {
				return err
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
			if input.Title == nil && input.Body == nil {
				return usageErrorf(cmd, "nothing to change, give --title or --body")
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
				dropped, err := keepsTheAttachments(firstNonEmpty(current.Value.BodySource, current.Value.Body), *input.Body, dropAttachments,
					"the document has attachments or mentions, and the new body would drop them",
					"Change the bodySource from docs show --json and send it back with --body-format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			document, err := s.client.UpdateDocument(a.ctx, args[0], input)
			if err != nil {
				return a.documentConflict(documentRefusal(s, err), err)
			}

			if guard != nil {
				notices = append(notices, theGuardIsKeptToTheSecond)
			}
			summary := "Wrote over document " + document.Value.Title
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
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a body that has attachments or mentions, which drops them")

	return cmd
}

func (a *app) newDocsUploadsCmd() *cobra.Command {
	return group("uploads", "Read the uploads of a Docs & Files tool", a.newDocsUploadsShowCmd())
}

func (a *app) newDocsUploadsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <uploadId>",
		Short: "Show an upload of a Docs & Files tool with its description and comments",
		Long: `Shows an upload: a file standing on its own in a Docs & Files tool, with what
was said about it and everything said under it.

The bytes are fetched with "uploads download <blobId>", where <blobId> is
file.id of this upload — not its own ID, which names the entry.

Putting a file into a Docs & Files tool, and changing the title or the
description of one, are not here yet. Both are done in Minka on the web.`,
		Args: requireArgs("uploadId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			upload, err := s.client.FilesUploadOf(a.ctx, args[0])
			if err != nil {
				e := documentRefusal(s, err)
				if e.Code == CodeNotFound {
					e.Hint = uploadNotFoundHint
				}
				return e
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
			comment, err := s.client.WriteEntryComment(a.ctx, entry, body)
			if err != nil {
				return documentRefusal(s, err)
			}

			summary := "Commented on " + string(entry.Kind) + " " + entry.ID
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
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
			comments, err := a.commentsUnder(s, entry)
			if err != nil {
				return err
			}
			for _, current := range comments {
				if current.ID != args[0] {
					continue
				}
				dropped, err := keepsTheAttachments(firstNonEmpty(current.BodySource, current.Body), body, dropAttachments,
					"the comment has attachments or mentions, and the new text would drop them",
					"Change the bodySource of the comment from docs show --json and send it back with --format html, or give --drop-attachments.")
				if err != nil {
					return err
				}
				notices = append(notices, dropped...)
			}

			comment, err := s.client.EditEntryComment(a.ctx, entry, args[0], body)
			if err != nil {
				return commentRefusal(s, err)
			}

			summary := fmt.Sprintf("Changed comment %s on %s %s", args[0], entry.Kind, entry.ID)
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: entryBreadcrumbs(entry),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	addEntryFlags(cmd, &documentID, &uploadID)
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments or mentions, which drops them")

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
			if err := s.client.DeleteEntryComment(a.ctx, entry, args[0]); err != nil {
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
			comment, err := s.client.ReactToEntryComment(a.ctx, entry, args[0], args[1])
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
			if err := s.client.UnreactFromEntryComment(a.ctx, entry, args[0], args[1]); err != nil {
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
func entryOfFlags(cmd *cobra.Command, documentID, uploadID string) (api.Entry, error) {
	switch {
	case documentID != "" && uploadID != "":
		return api.Entry{}, usageErrorf(cmd, "--document and --upload exclude each other")
	case documentID != "":
		if err := requireID(cmd, "--document", documentID); err != nil {
			return api.Entry{}, err
		}
		return api.Entry{Kind: api.EntryDocument, ID: documentID}, nil
	case uploadID != "":
		if err := requireID(cmd, "--upload", uploadID); err != nil {
			return api.Entry{}, err
		}
		return api.Entry{Kind: api.EntryUpload, ID: uploadID}, nil
	default:
		return api.Entry{}, usageErrorf(cmd, "say what the comment hangs on: --document <documentId> or --upload <uploadId>")
	}
}

// commentsUnder reads the entry a comment hangs on, for the one thing a write
// has to know beforehand: whether the text it replaces carries files or
// mentions that Markdown cannot hold.
func (a *app) commentsUnder(s *session, entry api.Entry) ([]api.Comment, error) {
	if entry.Kind == api.EntryUpload {
		upload, err := s.client.FilesUploadOf(a.ctx, entry.ID)
		if err != nil {
			return nil, documentRefusal(s, err)
		}
		return upload.Value.Comments, nil
	}
	document, err := s.client.Document(a.ctx, entry.ID)
	if err != nil {
		return nil, documentRefusal(s, err)
	}
	return document.Value.Comments, nil
}

// keepsTheAttachments refuses a text that would drop the files and mentions
// of the one it replaces, unless that was asked for. Markdown cannot hold
// them, so only a text that carries them keeps them.
func keepsTheAttachments(current, replacement string, allowed bool, message, hint string) ([]string, error) {
	if !richtext.HasAttachments(current) || richtext.HasAttachments(replacement) {
		return nil, nil
	}
	if !allowed {
		return nil, &Error{Code: CodeUsage, Message: message, Hint: hint}
	}
	return []string{"the attachments and mentions of the text were dropped"}, nil
}

func checkBodyFormat(cmd *cobra.Command, format string) error {
	if format != "markdown" && format != "html" {
		return usageErrorf(cmd, "--body-format is markdown or html, not %q", format)
	}
	return nil
}

// checkAudience holds the one word a document cannot be written without. Which
// people go with which audience is the server's rule and is left to it, so that
// there is one place where it stands.
func checkAudience(cmd *cobra.Command, audience string) error {
	switch {
	case audience == "":
		return usageErrorf(cmd, "say whom the document reaches: --audience everybody, chosen or nobody")
	case audience != "everybody" && audience != "chosen" && audience != "nobody":
		return usageErrorf(cmd, "--audience is everybody, chosen or nobody, not %q", audience)
	}
	return nil
}

// notifiedPeople is the people a new document tells about itself. Only the
// identifier and "me" are read here: which project the tool hangs in is not
// known before the document exists, and a name can only be looked up among
// the members of a project.
func (a *app) notifiedPeople(cmd *cobra.Command, s *session, notify []string) ([]string, error) {
	var people []string
	for _, value := range notify {
		if strings.EqualFold(strings.TrimSpace(value), "me") {
			id, err := a.meID(s)()
			if err != nil {
				return nil, err
			}
			people = append(people, id)
			continue
		}
		if err := requireID(cmd, "--notify", value); err != nil {
			return nil, &Error{
				Code:    CodeUsage,
				Message: fmt.Sprintf("--notify takes the ID of a person, or me; %q is neither", value),
				Hint:    "minka people list <projectId> gives the ID of every member of the project.",
			}
		}
		people = append(people, value)
	}
	return people, nil
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

// documentGuard turns --expect and --force into what a write sends: the moment
// the writer read, or nothing at all.
//
// It shares no rule with the notes of a card. There "--expect none" sends
// expectedUpdatedAt null and means that nobody has written any notes yet; on a
// document null says that nothing is expected. A write that expects nothing
// therefore leaves the field out.
func documentGuard(cmd *cobra.Command, expect string, force bool) (*string, error) {
	expected := cmd.Flags().Changed("expect")
	switch {
	case expected && force:
		return nil, usageErrorf(cmd, "--expect and --force exclude each other")
	case force:
		return nil, nil
	case !expected:
		return nil, &Error{
			Code:    CodeUsage,
			Message: "say which document the change may write over: give --expect or --force",
			Hint:    "minka docs show <documentId> gives the updatedAt for --expect.",
		}
	case expect == expectNone:
		return nil, &Error{
			Code:    CodeUsage,
			Message: "--expect none belongs to the notes of a card, where it means that nobody has written any; a document always has an updatedAt",
			Hint:    "minka docs show <documentId> gives the updatedAt to copy; --force writes over whatever stands.",
		}
	case !isATimeMinkaReads(expect):
		return nil, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("--expect takes the updatedAt of the document as Minka gives it, such as 2026-09-26T14:03:12.000000Z; %q is not", expect),
			Hint:    "minka docs show <documentId> gives the updatedAt to copy.",
		}
	}
	return &expect, nil
}

// documentRefusal classifies a failed request about a Docs & Files tool. Each
// code names a state the entry, its folder or somebody named on it is in, and
// the hint says what can be done about it — including where that is nothing.
func documentRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch {
	case e.APICode == apiCodeNotifyIDsMissing:
		e.Hint = "An audience of \"chosen\" tells the people it names, and this request named none. Give one --notify per person, or reach everybody or nobody with --audience."
	case e.APICode == apiCodeNotifyIDsWithoutChosenAudience:
		e.Hint = "Only --audience chosen tells the people a request names. Leave the named people out, or say --audience chosen to tell them."
	case e.APICode == apiCodePersonCannotSeeIt:
		e.Hint = "Nothing was written, and the same request will be refused again: somebody named with --notify is a client of the project, or an agent working for one, and is told only about a document in a folder shown to clients, in a tool shown to clients. Name somebody else. Which tools and folders the clients are shown is decided in Minka on the web."
	case e.APICode == apiCodeDocumentArchived:
		e.Hint = "The document, or a folder above it, lies in the archive. It can be read; nothing is written on it until it is brought back, which is done in Minka on the web."
	case e.APICode == apiCodeUploadArchived:
		e.Hint = "The upload, or a folder above it, lies in the archive. It can be read; nothing is written on it until it is brought back, which is done in Minka on the web."
	case e.APICode == apiCodeFolderUnknown:
		e.Hint = "This Docs & Files tool has no such folder, or you are a client of the project and it is not shown to you. List the folders with: minka docs list <toolId>"
	case e.APICode == apiCodeFolderArchived:
		e.Hint = "The folder lies in the archive and takes nothing new. Write at the top of the tool or in another folder; a folder comes back in Minka on the web."
	case e.Code == CodeNotFound:
		e.Hint = documentNotFoundHint
	}
	return e
}

// commentRefusal is a refusal of a write on one comment. An address that has
// nothing may be the entry, the comment or its author, and a document says
// which of them it is not.
func commentRefusal(s *session, err error) *Error {
	e := documentRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = "The document or upload does not exist or is not visible to you, the comment was removed, or somebody else wrote it: only the author can change or remove a comment."
	}
	return e
}

// documentConflict adds to a refused write what a writer needs to merge: the
// document as it stands, as the data of the error and printed for a person.
func (a *app) documentConflict(e *Error, err error) *Error {
	var apiErr *api.Error
	if e.APICode != apiCodeDocumentChanged || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return e
	}
	var body struct {
		Document json.RawMessage `json:"document"`
	}
	var current api.Document
	if json.Unmarshal(apiErr.Body, &body) != nil || json.Unmarshal(body.Document, &current) != nil {
		return e
	}

	e.Message = "the document was written over since it was read, nothing was written"
	e.Data = body.Document
	e.Hint = "Merge your text into the document as it stands and send it again with --expect " + current.UpdatedAt + "."
	e.Text = func(w io.Writer) {
		fmt.Fprintln(a.stderr, "The document as it stands:")
		fmt.Fprintln(a.stderr)
		renderDocument(w, current)
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
	fmt.Fprintf(w, "URL: %s\n", entries.AppURL)

	fmt.Fprintf(w, "\n## Folders (%d)\n\n", len(entries.Folders))
	if len(entries.Folders) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, folder := range entries.Folders {
		fmt.Fprintf(w, "- %s  %s (%s)\n", folder.ID, folder.Title, plural(folder.EntryCount, "entry", "entries"))
	}

	fmt.Fprintf(w, "\n## Documents (%d)\n\n", len(entries.Documents))
	if len(entries.Documents) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, document := range entries.Documents {
		fmt.Fprintf(w, "- %s  %s%s\n", document.ID, document.Title, entryDetails(document.CommentCount, document.Archived))
		if document.Lead != "" {
			fmt.Fprintf(w, "  %s\n", oneLine(document.Lead))
		}
	}

	fmt.Fprintf(w, "\n## Files (%d)\n\n", len(entries.Uploads))
	if len(entries.Uploads) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, upload := range entries.Uploads {
		fmt.Fprintf(w, "- %s  %s%s\n", upload.ID, upload.Title, entryDetails(upload.CommentCount, upload.Archived))
		fmt.Fprintf(w, "  %s, %s, file %s\n", upload.File.ContentType, fileSize(upload.File.ByteSize), upload.File.ID)
	}
}

// entryDetails says what a line of a folder has to add: how much was said
// under it, and whether it takes any change at all.
func entryDetails(commentCount int, archived bool) string {
	var details []string
	if commentCount > 0 {
		details = append(details, plural(commentCount, "comment", "comments"))
	}
	if archived {
		details = append(details, "archived")
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func renderDocument(w io.Writer, document api.Document) {
	fmt.Fprintf(w, "# %s\n", document.Title)
	fmt.Fprintf(w, "Document: %s\nTool: %s\nProject: %s\n", document.ID, document.ToolID, document.ProjectID)
	renderEntryPlace(w, document.Folder, document.Path, document.Archived)
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
	})
}

func documentBreadcrumbs(document api.Document) []Breadcrumb {
	return []Breadcrumb{
		{Action: "edit", Cmd: "minka docs edit " + document.ID + " --body - --expect " + document.UpdatedAt, Description: "Write over the document, unless somebody wrote since"},
		{Action: "comment", Cmd: "minka docs comments create --document " + document.ID + " <text|->", Description: "Comment on the document"},
		{Action: "folder", Cmd: "minka docs list " + document.ToolID + folderFlagOf(document.Folder), Description: "List what stands beside it"},
	}
}

func uploadBreadcrumbs(upload api.FilesUpload) []Breadcrumb {
	return []Breadcrumb{
		{Action: "download", Cmd: "minka uploads download " + upload.File.ID, Description: "Fetch the bytes of the file"},
		{Action: "comment", Cmd: "minka docs comments create --upload " + upload.ID + " <text|->", Description: "Comment on the upload"},
		{Action: "folder", Cmd: "minka docs list " + upload.ToolID + folderFlagOf(upload.Folder), Description: "List what stands beside it"},
	}
}

// entryBreadcrumbs name what follows a write on a comment: reading the entry
// it hangs under, whichever kind it is.
func entryBreadcrumbs(entry api.Entry) []Breadcrumb {
	if entry.Kind == api.EntryUpload {
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
