package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// What Minka answers an unlock of a document that is not locked.
const apiCodeDocumentNotLocked = "documentNotLocked"

// Only the team locks and unlocks, and a client is told nothing is there.
const onlyTheTeamLocks = "Only the team locks and unlocks a document: a client of the project meets the same."

// documentLockUnknownHint names a Minka that answers a lock with a page of
// its web, which says nothing about the document.
const documentLockUnknownHint = "Its Minka is older than this minka expects and cannot lock or unlock a document yet: that is done on the document's page in Minka on the web."

func (a *app) newDocsLockCmd() *cobra.Command {
	return a.newDocumentLockCmd(lockWrite{
		verb: "lock", done: "Locked",
		short: "Lock a document at its newest version, so that its title and body stay as they are",
		long: `Locks the document at its newest version, as its page in Minka does, and
answers with the document, which now says locked: true. While it is locked,
docs edit takes no new title and no new body, docs versions restore no earlier
version, and its decisions blocks show the decisions as they stood at the lock.
Comments, labels, the navigation, moves, the archive and the trash stay open.
Nobody is told.

It is the team's alone: a client of the project exits 2 (not_found), as for a
document that is not there or a draft. A document in the archive, itself or
through a folder above it, exits 9 with documentArchived, and one that is
locked already exits 9 with documentLocked.`,
		write: (*api.Client).LockDocument,
	})
}

func (a *app) newDocsUnlockCmd() *cobra.Command {
	return a.newDocumentLockCmd(lockWrite{
		verb: "unlock", done: "Unlocked",
		short: "Unlock a document, so that its title and body take writes again",
		long: `Unlocks the document and answers with it, which now says locked: false. Its
title and body take writes again, and its decisions blocks list the decisions
afresh on every reading. Nobody is told.

It is the team's alone: a client of the project exits 2 (not_found), as for a
document that is not there or a draft. A document in the archive, itself or
through a folder above it, exits 9 with documentArchived, and one that is not
locked exits 9 with documentNotLocked.`,
		write: (*api.Client).UnlockDocument,
	})
}

// lockWrite is one of the two writes on the lock of a document.
type lockWrite struct {
	verb, done  string // "lock" and "Locked"
	short, long string
	write       func(*api.Client, context.Context, string) (api.Result[api.Document], error)
}

func (a *app) newDocumentLockCmd(w lockWrite) *cobra.Command {
	return &cobra.Command{
		Use:     w.verb + " <documentId>",
		Short:   w.short,
		Long:    w.long,
		Example: "  minka docs " + w.verb + " 0199... --json",
		Args:    requireArgs("documentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			document, err := w.write(s.client, a.ctx, args[0])
			if err != nil {
				return documentLockRefusal(s, err, w.verb, args[0])
			}
			summary := w.done + " document " + document.Value.Title + " in " + document.Value.Tool.Title
			return a.ok(success{
				data:        document.Raw,
				summary:     summary,
				breadcrumbs: documentLockBreadcrumbs(document.Value),
				text:        func(out io.Writer) { renderDocumentLock(out, summary, document.Value) },
			})
		},
	}
}

// documentLockRefusal classifies a refused lock or unlock. A 404 that is not
// Minka's own JSON is the page of a Minka without the address, and says
// nothing about the document.
func documentLockRefusal(s *session, err error, verb, documentID string) *Error {
	e := documentRefusal(s, err)
	var apiErr *api.Error
	switch {
	case e.Code == CodeNotFound && errors.As(err, &apiErr) && apiErr.Body == nil:
		e.Message = s.host + " cannot lock or unlock a document"
		e.Hint = documentLockUnknownHint
	case e.Code == CodeNotFound:
		e.Hint = documentNotFoundHint + " " + onlyTheTeamLocks
	case e.APICode == apiCodeDocumentLocked && verb == "lock":
		e.Hint = "The document is locked already, so nothing was written; minka docs show " + documentID + " says since when and by whom. minka docs unlock " + documentID + " unlocks it."
	case e.APICode == apiCodeDocumentNotLocked:
		e.Hint = "The document is not locked, so there is nothing to unlock and nothing was written. Its title and body take writes as they are."
	case e.APICode == apiCodeDocumentArchived:
		e.Hint = "The document, or a folder above it, lies in the archive, and its lock stays as it is until it is back: minka docs unarchive " + documentID + ", or where a folder above it is archived, minka docs folders unarchive <folderId>."
	}
	return e
}

func renderDocumentLock(w io.Writer, summary string, document api.Document) {
	fmt.Fprintf(w, "%s.\nDocument: %s\n", summary, document.ID)
	if document.Locked {
		fmt.Fprintf(w, "Locked: yes%s\n", lockDetails(document))
	} else {
		fmt.Fprintln(w, "Locked: no")
	}
	fmt.Fprintf(w, "URL: %s\n", document.AppURL)
}

// lockDetails names who locked the document, when and which version stands
// locked, as far as Minka kept it.
func lockDetails(document api.Document) string {
	details := ""
	if document.LockedVersion != nil {
		details += fmt.Sprintf(", version %d", document.LockedVersion.Number)
	}
	if document.LockedBy != nil {
		details += ", by " + document.LockedBy.Name
	}
	if document.LockedAt != nil {
		details += ", " + formatTime(*document.LockedAt)
	}
	return details
}

func documentLockBreadcrumbs(document api.Document) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "show", Cmd: "minka docs show " + document.ID, Description: "Read the document with its body and comments"},
	}
	if document.Locked {
		return append(crumbs, Breadcrumb{Action: "unlock", Cmd: "minka docs unlock " + document.ID, Description: "Unlock it, so that its title and body take writes again"})
	}
	return append(crumbs,
		Breadcrumb{Action: "edit", Cmd: "minka docs edit " + document.ID + " --body - --expect " + document.UpdatedAt, Description: "Write over the document, unless somebody wrote since"},
		Breadcrumb{Action: "lock", Cmd: "minka docs lock " + document.ID, Description: "Lock it again at its newest version"},
	)
}
