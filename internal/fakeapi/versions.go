package fakeapi

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

// DocumentVersion is one text a document had: what a write left it saying.
type DocumentVersion struct {
	ID        string
	Title     string
	Body      string // the HTML as stored
	AuthorID  string // empty where nothing kept who wrote it
	WrittenAt time.Time
}

// UploadVersion is one file an upload showed or shows, and who gave it that
// file.
type UploadVersion struct {
	ID        string
	BlobID    string
	AuthorID  string
	WrittenAt time.Time
}

// The versions are the team's alone: a client of the project meets 404 at
// every one of these addresses, also where the document or upload itself is
// shown to them.
func (s *Server) registerVersionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/documents/{documentId}/versions", s.authenticated(s.documentVersions))
	mux.HandleFunc("GET /api/v1/documents/{documentId}/versions/{versionId}", s.authenticated(s.documentVersion))
	mux.HandleFunc("GET /api/v1/files-uploads/{uploadId}/versions", s.authenticated(s.uploadVersions))
	mux.HandleFunc("POST /api/v1/files-uploads/{uploadId}/versions", s.authenticated(s.addUploadVersion))
}

func (s *Server) documentVersions(w http.ResponseWriter, r *http.Request, person *Person) {
	_, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	lines := []any{}
	for number, version := range document.versions() {
		lines = append(lines, map[string]any{
			"id": version.ID, "documentId": document.ID, "number": number + 1, "current": number == len(document.Versions)-1,
			"author": s.personReferenceOrNil(version.AuthorID), "writtenAt": timestamp(version.WrittenAt),
		})
	}
	writeJSON(w, http.StatusOK, lines)
}

// documentVersion hands out one version of a document. A version of another
// document is at no address here, as is an identifier that is none.
func (s *Server) documentVersion(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	versionID := r.PathValue("versionId")
	for number, version := range document.versions() {
		if version.ID != versionID {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": version.ID, "documentId": document.ID, "number": number + 1, "current": number == len(document.Versions)-1,
			"title": version.Title, "body": storedForm(version.Body), "attachments": s.attachmentsIn(tool.ProjectID, version.Body),
			"author": s.personReferenceOrNil(version.AuthorID), "writtenAt": timestamp(version.WrittenAt),
			"appUrl": s.documentURL(tool, document) + "/history/" + version.ID,
		})
		return
	}
	notFound(w)
}

// uploadVersions lists the files an upload was given, oldest first. The
// current one is the file the upload shows now.
func (s *Server) uploadVersions(w http.ResponseWriter, r *http.Request, person *Person) {
	_, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
	if upload == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	lines := []any{}
	for number, version := range s.versionsOfUpload(upload) {
		lines = append(lines, map[string]any{
			"id": version.ID, "uploadId": upload.ID, "number": number + 1, "current": version.BlobID == upload.BlobID,
			"file": s.attachmentResource(version.BlobID), "author": s.personReference(version.AuthorID),
			"writtenAt": timestamp(version.WrittenAt),
		})
	}
	writeJSON(w, http.StatusOK, lines)
}

// addUploadVersion gives an upload another stored file, keeping its title and
// description and the file it showed as an earlier version. It refuses in the
// order the controller and its write statement refuse: the body, the upload,
// the client, the expectation, the file as the project knows it, and then
// what the write meets — the archived project and entry before the answer
// that nothing changed, the expectation before whether the file may be taken.
func (s *Server) addUploadVersion(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
	if upload == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	project := s.projectWithID(tool.ProjectID)
	expected, guarded, ok := expectedUpdatedAt(w, fields)
	if !ok {
		return
	}
	blobID, ok := stringField(w, fields, "blobId")
	if !ok {
		return
	}
	if blobID == nil || !uuidPattern.MatchString(*blobID) {
		malformed(w, `The field "blobId" has to name a stored file by its identifier.`)
		return
	}
	file := s.storedFileOf(tool.ProjectID, *blobID)
	if file == nil {
		if !s.refusedAsArchived(w, project) {
			fileUnknown(w)
		}
		return
	}

	if s.refusedAsArchived(w, project) || s.refusedAsAnArchivedEntry(w, tool, upload.FolderID, upload.Archived, "upload") {
		return
	}
	// The file it shows already is no write and no conflict, whatever the
	// expectation: nothing was lost.
	if file.ID == upload.BlobID {
		writeJSON(w, http.StatusOK, s.filesUploadResource(tool, upload))
		return
	}
	if guarded && !expected.Equal(upload.UpdatedAt) {
		refusal := errorBody("uploadChanged", `This upload was written over since it was read. Nothing was written; "upload" holds it as it stands.`)
		refusal["upload"] = s.filesUploadResource(tool, upload)
		writeJSON(w, http.StatusConflict, refusal)
		return
	}
	// Only a file of one's own is ever said to be in use; an earlier file of
	// this same upload is in use as well.
	if file.UploaderID != person.ID {
		fileUnknown(w)
		return
	}
	if s.fileIsInUse(file) {
		unprocessable(w, "fileInUse", "This file already stands in an upload, a text or an earlier version. Store it again to give it to this upload.")
		return
	}

	s.versionsOfUpload(upload)
	upload.BlobID = file.ID
	upload.UpdatedAt = later(s.aWholeSecond(), upload.UpdatedAt.Add(time.Second))
	upload.keepAVersion(file.ID, person.ID, upload.UpdatedAt)
	writeJSON(w, http.StatusCreated, s.filesUploadResource(tool, upload))
}

// versions are the texts of the document, oldest first. One that has none
// yet was written before versions were kept, and gets the one it says now.
func (d *Document) versions() []*DocumentVersion {
	if len(d.Versions) == 0 {
		d.Versions = []*DocumentVersion{{ID: newID(), Title: d.Title, Body: d.Body, WrittenAt: d.UpdatedAt}}
	}
	return d.Versions
}

// keepAVersion notes what the document says after a write.
func (d *Document) keepAVersion(authorID string, at time.Time) {
	d.Versions = append(d.Versions, &DocumentVersion{ID: newID(), Title: d.Title, Body: d.Body, AuthorID: authorID, WrittenAt: at})
}

// versionsOfUpload are the files of the upload, oldest first. One that has
// none yet starts with the file it shows, given by whoever stored it.
func (s *Server) versionsOfUpload(upload *FilesUpload) []*UploadVersion {
	if len(upload.Versions) == 0 {
		authorID := upload.CreatorID
		if file := s.storedFileOf(s.projectOfFilesUpload(upload), upload.BlobID); file != nil && file.UploaderID != "" {
			authorID = file.UploaderID
		}
		upload.keepAVersion(upload.BlobID, authorID, upload.CreatedAt)
	}
	return upload.Versions
}

func (u *FilesUpload) keepAVersion(blobID, authorID string, at time.Time) {
	u.Versions = append(u.Versions, &UploadVersion{ID: newID(), BlobID: blobID, AuthorID: authorID, WrittenAt: at})
}

// showedEarlier says whether the file is one the upload showed before and
// shows no longer.
func (u *FilesUpload) showedEarlier(blobID string) bool {
	if u.BlobID == blobID {
		return false
	}
	for _, version := range u.Versions {
		if version.BlobID == blobID {
			return true
		}
	}
	return false
}

func (s *Server) projectOfFilesUpload(upload *FilesUpload) string {
	for _, tool := range s.FilesTools {
		for _, one := range tool.Uploads {
			if one == upload {
				return tool.ProjectID
			}
		}
	}
	return ""
}

// isAnEarlierFile says whether the file is one an upload showed before and
// shows no longer, which only the team of its project reaches.
func (s *Server) isAnEarlierFile(blobID string) bool {
	for _, tool := range s.FilesTools {
		for _, upload := range tool.Uploads {
			if upload.showedEarlier(blobID) {
				return true
			}
		}
	}
	return false
}

// storedForm is a text as Minka stores it and hands it out for a version: an
// attachment keeps only what names it, so a file in it carries no name and no
// address. What a reader sees of the file is the list of attachments.
func storedForm(text string) string {
	return attachmentTag.ReplaceAllStringFunc(text, func(tag string) string {
		kept := []string{"<action-text-attachment"}
		for _, attribute := range tagAttribute.FindAllStringSubmatch(tag, -1) {
			if slices.Contains(storedAttachmentAttributes, attribute[1]) {
				kept = append(kept, attribute[0])
			}
		}
		return strings.Join(kept, " ") + ">"
	})
}

// storedAttachmentAttributes are the attributes Minka keeps of an attachment
// in a stored text.
var storedAttachmentAttributes = []string{"sgid", "caption", "presentation", "content-type", "content"}

var (
	attachmentTag = regexp.MustCompile(`<action-text-attachment\b[^>]*>`)
	tagAttribute  = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
)
