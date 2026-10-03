package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// The versions of a document and the files of an upload are the team's alone,
// so a client of the project meets the same 404 as somebody outside it, even
// where the document or upload is shown to them.
const (
	documentVersionsNotFoundHint = "There is no such document or version, the document is a draft, it belongs to a project you are not a member of, or you are a client of the project: the versions are the team's alone, also of a document shown to clients."
	uploadVersionsNotFoundHint   = "There is no such upload, it belongs to a project you are not a member of, or you are a client of the project: the versions are the team's alone, also of an upload shown to clients."
)

func (a *app) newDocsVersionsCmd() *cobra.Command {
	return group("versions", "Read the earlier versions of a document",
		a.newDocsVersionsListCmd(),
		a.newDocsVersionsShowCmd(),
	)
}

func (a *app) newDocsVersionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <documentId>",
		Short: "List the versions of a document, oldest first",
		Long: `Lists every version a document had, oldest first: its number, who wrote it and
when. The newest is current, which is what docs show gives. A version is read
with "docs versions show <documentId> <versionId>".

Nothing here is paged and nothing is cut. author is null for a version written
before Minka kept who wrote it.

The versions are the team's alone: a client of the project gets exit 2
(not_found), even for a document shown to clients. A document in the archive,
below an archived folder or in an archived project still has its versions read.
Making an earlier version the current one is done in Minka on the web.`,
		Args: requireArgs("documentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			versions, err := s.client.DocumentVersions(a.ctx, args[0])
			if err != nil {
				return versionsRefusal(s, err, documentVersionsNotFoundHint)
			}

			return a.ok(success{
				data:        versions.Raw,
				summary:     plural(len(versions.Value), "version", "versions") + " of the document",
				breadcrumbs: documentVersionsBreadcrumbs(args[0], versions.Value),
				text:        func(w io.Writer) { renderDocumentVersions(w, versions.Value) },
			})
		},
	}
}

func (a *app) newDocsVersionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <documentId> <versionId>",
		Short: "Show one version of a document with its title and body",
		Long: `Shows one version of a document as it was written: its title, its body and
the files the body names. In text output the body is converted to Markdown, as
docs show does; in JSON it is the HTML the server stored.

Minka hands out a version's body only in the form it stores, where a file
carries no name and no address: it reads as "[attachment]" in the body, and
the file itself shows only in the list of files below it.

The IDs come from docs versions list. A version of another document is not
found, and neither is any version for a client of the project: the versions
are the team's alone.`,
		Args: requireArgs("documentId", "versionId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			version, err := s.client.DocumentVersionOf(a.ctx, args[0], args[1])
			if err != nil {
				return versionsRefusal(s, err, documentVersionsNotFoundHint)
			}

			return a.ok(success{
				data:    version.Raw,
				summary: fmt.Sprintf("Version %d of %s", version.Value.Number, version.Value.Title),
				breadcrumbs: []Breadcrumb{
					{Action: "versions", Cmd: "minka docs versions list " + args[0], Description: "List the versions of the document"},
					{Action: "show", Cmd: "minka docs show " + args[0], Description: "Read the document as it stands"},
				},
				text: func(w io.Writer) { renderDocumentVersion(w, version.Value) },
			})
		},
	}
}

func (a *app) newDocsUploadsVersionsCmd() *cobra.Command {
	return group("versions", "List the files an upload was given, and give it another",
		a.newDocsUploadsVersionsListCmd(),
		a.newDocsUploadsVersionsCreateCmd(),
	)
}

func (a *app) newDocsUploadsVersionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <uploadId>",
		Short: "List the files an upload was given, oldest first",
		Long: `Lists every file an upload showed, oldest first: who gave it that file and
when. The current one is the file it shows now. An earlier file is fetched
with "uploads download <blobId>", where <blobId> is file.id of its line.

Nothing here is paged and nothing is cut. The versions and the earlier files
are the team's alone: a client of the project gets exit 2 (not_found), even
for an upload shown to clients.`,
		Args: requireArgs("uploadId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			versions, err := s.client.UploadVersions(a.ctx, args[0])
			if err != nil {
				return versionsRefusal(s, err, uploadVersionsNotFoundHint)
			}

			return a.ok(success{
				data:        versions.Raw,
				summary:     plural(len(versions.Value), "file", "files") + " of the upload",
				breadcrumbs: uploadVersionsBreadcrumbs(args[0], versions.Value),
				text:        func(w io.Writer) { renderUploadVersions(w, versions.Value) },
			})
		},
	}
}

func (a *app) newDocsUploadsVersionsCreateCmd() *cobra.Command {
	var blobID, expect, name, contentType string

	cmd := &cobra.Command{
		Use:   "create <uploadId> (<file> | --blob <blobId>)",
		Short: "Replace the file of an upload, keeping the one it showed as an earlier version",
		Long: `Gives an upload another file, as the web's "Replace with a new version" does.
The title and the description stay, and the file the upload showed stays with
it as an earlier version, listed by docs uploads versions list.

Minka takes the file in two requests, as docs uploads create does: the file is
stored in the project of the upload first, and then given to the upload. Given
a file, the command does both. It reads the upload for its project and its
versions before the file travels, so an upload that is not there, or whose
versions you may not see as a client of the project, costs no transfer, and
neither does one in the archive or below an archived folder: that exits 9
before the file travels. A step that fails ends the run, and nothing after it
is sent.

Where the file was stored and the second step was refused, the error hands
over the stored file under "data": give --blob <blobId> with its id in place
of the file to try again without sending it anew. --blob also takes a file
stored earlier in the project with "uploads create --project".

A file goes into one place only. One that stands in an upload, a text or an
earlier version is refused with fileInUse — an earlier file of this same
upload as well. To give an upload an earlier file back, fetch it with
"uploads download" and give it here as a file, which stores its bytes anew.
--blob naming the file the upload shows already writes nothing and says so.

--expect <updatedAt> refuses the change where somebody wrote over the upload
since you read it: exit 9, meta.apiCode uploadChanged, the upload as it stands
under "data". Without it Minka still never writes over a title or description
changed in the meantime, and nothing is lost: the file the upload showed stays
as an earlier version. The guard reaches only as far as Minka's own clock, the
whole second.

` + unattachedFileHelp + `

` + mediaTypeHelp,
		Example: `  minka docs uploads versions create 0199... site-plan-v2.pdf
  minka docs uploads versions create 0199... site-plan-v2.pdf --expect 2026-09-26T14:03:12.000000Z
  minka docs uploads versions create 0199... --blob 0199...`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 2 {
				return usageErrorf(cmd, "unexpected argument %q", args[2])
			}
			return requireArgs("uploadId")(cmd, args[:min(len(args), 1)])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			uploadID := args[0]
			storing := len(args) == 2
			switch {
			case storing == flags.Changed("blob"):
				return usageErrorf(cmd, "give a file or --blob, not both and not neither")
			case !storing:
				if err := requireID(cmd, "--blob", &blobID); err != nil {
					return err
				}
				for _, flag := range []string{"name", "content-type"} {
					if flags.Changed(flag) {
						return usageErrorf(cmd, "--%s belongs to storing a file; the file --blob names is stored already", flag)
					}
				}
			}
			input := api.AddUploadVersionInput{}
			if flags.Changed("expect") {
				guard, err := entryGuard(cmd, "upload", "minka docs uploads show <uploadId>", expect, false)
				if err != nil {
					return err
				}
				input.ExpectedUpdatedAt = guard
			}

			var file *fileToStore
			if storing {
				var err error
				if file, err = openFileToStore(args[1], name, contentType); err != nil {
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
				upload, err := s.client.FilesUploadOf(a.ctx, uploadID)
				if err != nil {
					return uploadRefusal(s, err)
				}
				// Minka would refuse it only after the transfer, and leave the
				// stored file standing in nothing for days.
				if upload.Value.Archived {
					return &Error{
						Code:    CodeValidation,
						Message: "the upload " + upload.Value.Title + " lies in the archive, or a folder above it does, nothing was sent",
						Hint:    "It can be read; it takes no file until it is brought back, which is done in Minka on the web.",
					}
				}
				// A client of the project reads the upload and may not give it
				// a file; the versions tell, before the file travels.
				if _, err := s.client.UploadVersions(a.ctx, uploadID); err != nil {
					return versionsRefusal(s, err, uploadVersionsNotFoundHint)
				}
				uploaded, err := s.client.UploadToProject(a.ctx, upload.Value.ProjectID, file.filename, file.mediaType, file.content, file.size)
				if err != nil {
					return storeRefusal(s, err)
				}
				stored, blobID = &uploaded, uploaded.Value.ID
			}
			input.BlobID = blobID

			upload, err := s.client.AddUploadVersion(a.ctx, uploadID, input)
			if err != nil {
				return uploadVersionRefusal(a, s, err, blobID, stored)
			}

			var notices []string
			if input.ExpectedUpdatedAt != nil {
				notices = append(notices, theGuardIsKeptToTheSecond)
			}
			summary := fmt.Sprintf("Gave upload %s the file %s", upload.Value.Title, upload.Value.File.Filename)
			if upload.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("Upload %s shows the file %s already, nothing was written", upload.Value.Title, upload.Value.File.Filename)
				notices = append(notices, "the upload shows this file already, so no version was added")
			}
			return a.ok(success{
				data:    upload.Raw,
				summary: summary,
				notices: notices,
				breadcrumbs: append([]Breadcrumb{
					{Action: "versions", Cmd: "minka docs uploads versions list " + upload.Value.ID, Description: "List the files the upload was given"},
				}, uploadBreadcrumbs(upload.Value)...),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nUpload: %s\nFile: %s\n%s\n", summary, upload.Value.ID, upload.Value.File.ID, upload.Value.AppURL)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&blobID, "blob", "", "ID of a file stored already, in place of a file of this machine")
	flags.StringVar(&expect, "expect", "", "The updatedAt of the upload you read; refused where somebody wrote over it since")
	addFileToStoreFlags(cmd, &name, &contentType)

	return cmd
}

// versionsRefusal classifies a failed read of versions, whose 404 also
// answers a client of the project.
func versionsRefusal(s *session, err error, notFoundHint string) *Error {
	e := s.fail(err)
	if e.Code == CodeNotFound {
		e.Hint = notFoundHint
	}
	return e
}

// uploadVersionRefusal classifies a file Minka did not give to an upload.
// stored is the file this run stored a moment before, and nil where --blob
// named it.
func uploadVersionRefusal(a *app, s *session, err error, blobID string, stored *api.Result[api.Upload]) *Error {
	e := documentRefusal(s, err)
	switch {
	case e.Code == CodeNotFound:
		e.Hint = uploadVersionsNotFoundHint
	case e.APICode == apiCodeFileUnknown && stored != nil:
		e.Hint = "The file was stored in this same run, so it has not been swept, and storing it again is not the remedy. Minka says no more about why it did not take it: try once more with --blob " + blobID + "."
	case e.APICode == apiCodeFileUnknown:
		e.Hint = "Minka tells no cause apart: the file may have been swept after standing in nothing (such a file is kept for at least seven days and may go any time after), never have been stored, belong to another project, or be somebody else's. The one remedy worth trying is to store it again: give the file itself in place of --blob."
	case e.APICode == apiCodeFileInUse && stored != nil:
		e.Hint = "The file stands in an upload, a text or an earlier version already, and a file goes into one place only, so naming it again cannot help. Running the command again stores the file anew."
	case e.APICode == apiCodeFileInUse:
		e.Hint = "The file stands in an upload, a text or an earlier version already — an earlier file of this same upload included — and a file goes into one place only. Give the file itself in place of --blob to store its bytes anew."
	}
	e = handOverTheStoredFile(e, blobID, stored)

	conflict := entryConflict(a, e, err, apiCodeUploadChanged, "upload",
		func(u api.FilesUpload) string { return u.UpdatedAt }, renderFilesUpload)
	raw, handedOver := conflict.Data.(json.RawMessage)
	var current api.FilesUpload
	if conflict.APICode == apiCodeUploadChanged && handedOver && json.Unmarshal(raw, &current) == nil && current.UpdatedAt != "" {
		conflict.Hint = "Somebody wrote over the upload since you read it, perhaps giving it a file of their own: look at it as it stands, and send again with --expect " + current.UpdatedAt + " if it still needs your file."
		if stored != nil {
			conflict.Hint += " The file is stored as " + blobID + " and kept for at least seven days while nothing carries it: give --blob " + blobID + " in place of the file to send it no second time."
		}
	}
	return conflict
}

func renderDocumentVersions(w io.Writer, versions []api.DocumentVersionLine) {
	if len(versions) == 0 {
		fmt.Fprintln(w, "No versions.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NUMBER\tID\tWRITTEN\tAUTHOR\t")
	for _, version := range versions {
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%s\n", version.Number, version.ID, formatTime(version.WrittenAt), authorOf(version.Author), currentMark(version.Current))
	}
	_ = table.Flush()
}

func renderDocumentVersion(w io.Writer, version api.DocumentVersion) {
	fmt.Fprintf(w, "# %s\n", version.Title)
	fmt.Fprintf(w, "Version: %d%s\n", version.Number, map[bool]string{true: ", the current one", false: ""}[version.Current])
	fmt.Fprintf(w, "Document: %s\n", version.DocumentID)
	fmt.Fprintf(w, "Written: %s by %s\n", formatTime(version.WrittenAt), authorOf(version.Author))
	fmt.Fprintf(w, "URL: %s\n", version.AppURL)

	if body := richtext.ToMarkdown(version.Body); body != "" {
		fmt.Fprintf(w, "\n## Body\n\n%s\n", body)
	}
	renderEntryFiles(w, version.Attachments)
}

func renderUploadVersions(w io.Writer, versions []api.UploadVersion) {
	if len(versions) == 0 {
		fmt.Fprintln(w, "No files.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NUMBER\tFILE\tSIZE\tBLOB\tWRITTEN\tAUTHOR\t")
	for _, version := range versions {
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", version.Number, version.File.Filename, fileSize(version.File.ByteSize),
			version.File.ID, formatTime(version.WrittenAt), version.Author.Name, currentMark(version.Current))
	}
	_ = table.Flush()
}

// authorOf names who wrote a version, and says so where nothing kept it.
func authorOf(author *api.PersonReference) string {
	if author == nil {
		return "(not kept)"
	}
	return author.Name
}

func currentMark(current bool) string {
	if current {
		return "current"
	}
	return ""
}

func documentVersionsBreadcrumbs(documentID string, versions []api.DocumentVersionLine) []Breadcrumb {
	versionID := "<versionId>"
	if len(versions) > 0 {
		versionID = versions[0].ID
	}
	return []Breadcrumb{
		{Action: "show", Cmd: "minka docs versions show " + documentID + " " + versionID, Description: "Read one version with its title and body"},
		{Action: "document", Cmd: "minka docs show " + documentID, Description: "Read the document as it stands"},
	}
}

func uploadVersionsBreadcrumbs(uploadID string, versions []api.UploadVersion) []Breadcrumb {
	blobID := "<blobId>"
	if len(versions) > 0 {
		blobID = versions[0].File.ID
	}
	return []Breadcrumb{
		{Action: "download", Cmd: "minka uploads download " + blobID, Description: "Fetch the bytes of a file the upload showed"},
		{Action: "replace", Cmd: "minka docs uploads versions create " + uploadID + " <file>", Description: "Give the upload another file"},
		{Action: "upload", Cmd: "minka docs uploads show " + uploadID, Description: "Read the upload as it stands"},
	}
}
