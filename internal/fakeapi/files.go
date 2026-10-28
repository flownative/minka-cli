package fakeapi

import (
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/richtext"
)

// What the server keeps of a document, and how much of its body a folder
// shows beside its title.
const (
	maxDocumentBodyLength      = 100000
	maxUploadDescriptionLength = 100000
	leadLength                 = 240
)

// Audiences a new document can reach, as the API names them.
var documentAudiences = []string{"everybody", "chosen", "nobody"}

// FilesTool is a Docs & Files tool of a project: folders to any depth, and
// documents and uploads standing in them or at the top.
type FilesTool struct {
	ID        string
	ProjectID string
	Title     string
	Folders   []*Folder
	Documents []*Document
	Uploads   []*FilesUpload

	// ShownToClients lets the clients of the project into the tool at all.
	// Without it no entry of the tool is shown to them, whatever its folder
	// says.
	ShownToClients bool

	// Arranged are the places ordered by hand, keyed by folder ID and "" for
	// the top, each with its entries in the order set. An entry the list does
	// not name stands behind the rest in the automatic order.
	Arranged map[string][]string
}

// Folder holds entries of a tool. A folder in the archive is in no list, and
// everything below it goes quiet with it: readable, and closed to every write.
type Folder struct {
	ID       string
	Title    string
	ParentID string // empty at the top of the tool
	Archived bool

	// ShownToClients decides for everything below this folder, and only the
	// topmost folder of a chain is asked. An entry at the top of a tool decides
	// for itself, and a new document there is shown to no client, so a folder
	// is the only place a new one reaches a client.
	ShownToClients bool

	// IsBook marks the folder to be read as a book. No book stands inside
	// another.
	IsBook bool

	CreatedAt time.Time
	// UpdatedAt is when the folder itself last changed, a rename; zero for
	// one never changed since it was made.
	UpdatedAt time.Time
	CreatorID string
	EntryState
}

// updatedAt is when the folder itself last changed.
func (f *Folder) updatedAt() time.Time {
	if f.UpdatedAt.IsZero() {
		return f.CreatedAt
	}
	return f.UpdatedAt
}

// Document is a page of a tool. Its moment is kept to the whole second, as the
// column behind it is, so the updatedAt a reader gets back is the one a guarded
// write sends in again.
type Document struct {
	ID       string
	FolderID string // empty at the top of the tool
	Title    string
	Body     string // the HTML as stored
	Draft    bool   // at no address, its author included
	Archived bool
	// OutOfNavigation leaves the document out of the navigation of the book
	// it stands in; it may be set on one in no book, and counts once it is in
	// one.
	OutOfNavigation bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CreatorID       string
	Comments        []*Comment

	// Versions are the texts the document had, oldest first, the last one what
	// it says now. A document set up without any is taken to have been written
	// before versions were kept: its first version names no author.
	Versions []*DocumentVersion
	EntryState
}

// FilesUpload is a file standing on its own in a tool. The bytes are an
// Upload of the fake, which is what a download asks for.
type FilesUpload struct {
	ID          string
	FolderID    string
	Title       string
	BlobID      string
	Description string // the HTML as stored
	Archived    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatorID   string
	Comments    []*Comment

	// Versions are the files the upload was given, oldest first. An upload set
	// up without any starts with the file it shows, given by whoever stored
	// it, at the moment the upload was made.
	Versions []*UploadVersion
	EntryState
}

func (s *Server) registerFilesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/files", s.authenticated(s.filesTools))
	mux.HandleFunc("GET /api/v1/files-tools/{toolId}", s.authenticated(s.filesTool))
	mux.HandleFunc("GET /api/v1/files-tools/{toolId}/entries", s.authenticated(s.filesEntries))
	mux.HandleFunc("POST /api/v1/files-tools/{toolId}/documents", s.authenticated(s.addDocument))
	mux.HandleFunc("GET /api/v1/documents/{documentId}", s.authenticated(s.document))
	mux.HandleFunc("PATCH /api/v1/documents/{documentId}", s.authenticated(s.updateDocument))
	mux.HandleFunc("POST /api/v1/files-tools/{toolId}/uploads", s.authenticated(s.addFilesUpload))
	mux.HandleFunc("GET /api/v1/files-uploads/{uploadId}", s.authenticated(s.filesUpload))
	mux.HandleFunc("PATCH /api/v1/files-uploads/{uploadId}", s.authenticated(s.updateFilesUpload))
	s.registerVersionRoutes(mux)
	s.registerFilesOrderRoutes(mux)

	s.registerCommentRoutes(mux, "documents/{documentId}")
	s.registerCommentRoutes(mux, "files-uploads/{uploadId}")
}

func (s *Server) filesTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}

	tools := []any{}
	for _, tool := range s.FilesTools {
		if tool.ProjectID == project.ID && (tool.ShownToClients || !s.isAClient(person.ID)) {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.filesToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
}

// filesTool reads a tool by its identifier alone, with the project it stands
// in.
func (s *Server) filesTool(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.filesToolURL(tool)))
}

// filesEntries lists what stands at the top of a tool, or in one of its
// folders. The inside of a folder in the archive is listed as well, and every
// line of it says that it is archived.
func (s *Server) filesEntries(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	folderID := strings.ToLower(r.URL.Query().Get("folderId"))
	var folder *Folder
	if folderID != "" {
		if folder = folderInUse(tool, folderID); folder == nil {
			notFound(w)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.entriesResource(tool, folder))
}

func (s *Server) document(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil {
		notFound(w)
		return
	}
	resource := s.documentResource(tool, document)
	resource["body"] = s.servedReferences(document.Body, person)
	writeJSON(w, http.StatusOK, resource)
}

// addDocument writes a published document, refusing in the order the
// controller refuses: what the request says before what the write meets.
func (s *Server) addDocument(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if !validTitle(w, "document", "a document", deref(title)) {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	stored, ok := storedBody(w, "document", deref(body))
	if !ok {
		return
	}
	folderID, ok := stringField(w, fields, "folderId")
	if !ok {
		return
	}
	folder, ok := s.folderToWriteIn(w, tool, deref(folderID))
	if !ok {
		return
	}
	audience, ok := stringField(w, fields, "audience")
	if !ok {
		return
	}
	if !slices.Contains(documentAudiences, deref(audience)) {
		unprocessable(w, "documentAudienceMissing", `A document says whom it reaches: "everybody", "chosen" with "notifyIds", or "nobody".`)
		return
	}
	notifyIDs, ok := stringListField(w, fields, "notifyIds")
	if !ok {
		return
	}
	// Every identifier is read before the audience is held against the list,
	// and one that is none, such as one in capitals, is answered like
	// somebody outside the project.
	for _, personID := range notifyIDs {
		if !uuidPattern.MatchString(personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a document.")
			return
		}
	}
	// A list the audience would not use is refused rather than dropped, and
	// both refusals read the request alone, so they come before the write
	// meets the project.
	if deref(audience) == "chosen" && len(notifyIDs) == 0 {
		unprocessable(w, "notifyIdsMissing", `An audience of "chosen" names the people in "notifyIds".`)
		return
	}
	if deref(audience) != "chosen" && len(notifyIDs) > 0 {
		unprocessable(w, "notifyIdsWithoutChosenAudience", `Only an audience of "chosen" takes "notifyIds".`)
		return
	}
	// The people a document tells are enrolled after it is written, so an
	// archived project refuses the write before anybody is looked up.
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.refusedAsSomebodyWhoCannotBeTold(w, person, tool, folder, notifyIDs) {
		return
	}

	document := &Document{
		ID: newID(), FolderID: folderIDOf(folder), Title: phpTrim(deref(title)), Body: stored,
		CreatedAt: s.aWholeSecond(), UpdatedAt: s.aWholeSecond(), CreatorID: person.ID,
	}
	document.keepAVersion(person.ID, document.UpdatedAt)
	tool.Documents = append([]*Document{document}, tool.Documents...)
	placeAtTheEnd(tool, document.FolderID, document.ID)
	writeJSON(w, http.StatusCreated, s.documentResource(tool, document))
}

// updateDocument writes over the title and the body of a document. What the
// request leaves out stays; "expectedUpdatedAt" writes only over what was
// read, and leaving it out or sending null expects nothing.
func (s *Server) updateDocument(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if title != nil && !validTitle(w, "document", "a document", *title) {
		return
	}
	newTitle := document.Title
	if title != nil {
		newTitle = phpTrim(*title)
	}
	newBody := document.Body
	if _, given := fields["body"]; given {
		body, ok := stringField(w, fields, "body")
		if !ok {
			return
		}
		if newBody, ok = storedBody(w, "document", deref(body)); !ok {
			return
		}
	}
	expected, guarded, ok := expectedUpdatedAt(w, fields)
	if !ok {
		return
	}
	inNavigation, ok := s.bookField(w, fields, "inNavigation")
	if !ok {
		return
	}
	// Every refusal of the navigation stands before anything is written: a
	// client is told nothing is there, whatever else the request changes.
	if inNavigation != nil {
		if s.isAClient(person.ID) {
			notFound(w)
			return
		}
		if isArchivedOrBelowTheArchive(filesEntry{tool: tool, parent: &document.FolderID, archived: &document.Archived}) {
			if !s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
				s.refusedAsAnArchivedEntry(w, tool, document.FolderID, document.Archived, "document")
			}
			return
		}
	}

	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.refusedAsAnArchivedEntry(w, tool, document.FolderID, document.Archived, "document") {
		return
	}
	// The text that stands already is no write and no conflict, whatever the
	// expectation: nothing was lost. The navigation is written all the same,
	// and touches no moment of the document.
	if newTitle == document.Title && newBody == document.Body {
		if inNavigation != nil {
			document.OutOfNavigation = !*inNavigation
		}
		writeJSON(w, http.StatusOK, s.documentResource(tool, document))
		return
	}
	if guarded && !expected.Equal(document.UpdatedAt) {
		refusal := errorBody("documentChanged", `This document was written over since it was read. Nothing was written; "document" holds it as it stands.`)
		refusal["document"] = s.documentResource(tool, document)
		writeJSON(w, http.StatusConflict, refusal)
		return
	}
	// Every write stands a second after the one before, so that each has a
	// moment of its own for the next guard to tell apart.
	document.versions()
	document.Title, document.Body = newTitle, newBody
	document.UpdatedAt = later(s.aWholeSecond(), document.UpdatedAt.Add(time.Second))
	document.keepAVersion(person.ID, document.UpdatedAt)
	if inNavigation != nil {
		document.OutOfNavigation = !*inNavigation
	}
	writeJSON(w, http.StatusOK, s.documentResource(tool, document))
}

func (s *Server) filesUpload(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
	if upload == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.filesUploadResource(tool, upload))
}

// addFilesUpload puts a stored file that stands nowhere yet into a tool, on
// top of the tool or of a folder. The file is found first, and what the
// request says about the upload only then; whether the file may go in is
// learnt from the write itself, after the archived project.
func (s *Server) addFilesUpload(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	project := s.projectWithID(tool.ProjectID)
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
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	newTitle := titleOfFile(file.Filename)
	if title != nil {
		if !validTitle(w, "upload", "an upload", *title) {
			return
		}
		newTitle = phpTrim(*title)
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	stored, ok := storedDescription(w, deref(description))
	if !ok {
		return
	}
	folderID, ok := stringField(w, fields, "folderId")
	if !ok {
		return
	}
	folder, ok := s.folderToWriteIn(w, tool, deref(folderID))
	if !ok {
		return
	}

	// The write takes only a file of the writer's own that stands nowhere
	// yet. Why it took nothing is asked afterwards, and only a file of one's
	// own is ever said to be in use.
	if file.UploaderID != person.ID || s.fileIsInUse(file) {
		if s.refusedAsArchived(w, project) {
			return
		}
		if file.UploaderID == person.ID {
			unprocessable(w, "fileInUse", "This file already stands in an upload or a text. Store it again to put it in another upload.")
			return
		}
		fileUnknown(w)
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	upload := &FilesUpload{
		ID: newID(), FolderID: folderIDOf(folder), Title: newTitle, BlobID: file.ID, Description: stored,
		CreatedAt: s.aWholeSecond(), UpdatedAt: s.aWholeSecond(), CreatorID: person.ID,
	}
	upload.keepAVersion(file.ID, person.ID, upload.CreatedAt)
	tool.Uploads = append([]*FilesUpload{upload}, tool.Uploads...)
	placeAtTheEnd(tool, upload.FolderID, upload.ID)
	writeJSON(w, http.StatusCreated, s.filesUploadResource(tool, upload))
}

// updateFilesUpload writes over the title and the description of an upload.
// The file stays whatever the request says: a "blobId" is not read at all.
// A description sent as null is emptied, as one sent empty is.
func (s *Server) updateFilesUpload(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
	if upload == nil {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if title != nil && !validTitle(w, "upload", "an upload", *title) {
		return
	}
	newTitle := upload.Title
	if title != nil {
		newTitle = phpTrim(*title)
	}
	newDescription := upload.Description
	if _, given := fields["description"]; given {
		description, ok := stringField(w, fields, "description")
		if !ok {
			return
		}
		if newDescription, ok = storedDescription(w, deref(description)); !ok {
			return
		}
	}
	expected, guarded, ok := expectedUpdatedAt(w, fields)
	if !ok {
		return
	}

	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.refusedAsAnArchivedEntry(w, tool, upload.FolderID, upload.Archived, "upload") {
		return
	}
	if newTitle == upload.Title && newDescription == upload.Description {
		writeJSON(w, http.StatusOK, s.filesUploadResource(tool, upload))
		return
	}
	if guarded && !expected.Equal(upload.UpdatedAt) {
		refusal := errorBody("uploadChanged", `This upload was written over since it was read. Nothing was written; "upload" holds it as it stands.`)
		refusal["upload"] = s.filesUploadResource(tool, upload)
		writeJSON(w, http.StatusConflict, refusal)
		return
	}
	upload.Title, upload.Description = newTitle, newDescription
	upload.UpdatedAt = later(s.aWholeSecond(), upload.UpdatedAt.Add(time.Second))
	writeJSON(w, http.StatusOK, s.filesUploadResource(tool, upload))
}

// titleOfFile is the title an upload without one is given: its file name,
// and where that is too long for a title, cut before the extension so that
// the extension stays. It is trimmed as every title is, after the cut.
func titleOfFile(filename string) string {
	name := []rune(filename)
	if len(name) <= maxTitleLength {
		return phpTrim(filename)
	}
	extension := []rune(fileExtension.FindString(filename))
	room := maxTitleLength - len(extension)
	if room <= 0 {
		return phpTrim(string(name[:maxTitleLength]))
	}
	return phpTrim(string(name[:room]) + string(extension))
}

// fileExtension is what Minka counts as the extension of a file name.
var fileExtension = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)

// storedFileOf is a file stored in this project that has not been swept,
// whoever stored it and wherever it stands.
func (s *Server) storedFileOf(projectID, blobID string) *Upload {
	for _, file := range s.Uploads {
		if file.ID == blobID && file.Circle == projectID && !file.Swept {
			return file
		}
	}
	return nil
}

// fileIsInUse says whether an upload of any tool stands for the file, an
// earlier version of one holds it, a saved text carries it, or it is on the
// back of a card.
func (s *Server) fileIsInUse(file *Upload) bool {
	if file.StandsInAText || s.heldByACardBack(file.ID) {
		return true
	}
	for _, tool := range s.FilesTools {
		for _, upload := range tool.Uploads {
			if upload.BlobID == file.ID || upload.showedEarlier(file.ID) {
				return true
			}
		}
	}
	return false
}

// fileUnknown is one answer for every file the writer cannot put into an
// upload: swept, never stored, of another project or of somebody else.
func fileUnknown(w http.ResponseWriter) {
	unprocessable(w, "fileUnknown", `The field "blobId" has to name a file you stored in this project that stands in no upload or text yet.`)
}

// storedDescription is the description of an upload as the server keeps it.
func storedDescription(w http.ResponseWriter, value string) (string, bool) {
	return storedRichText(w, "uploadDescriptionTooLong", "The description of an upload is too long.", maxUploadDescriptionLength, value)
}

// commentInvisibleMessage is what a comment with invisible characters draws.
const commentInvisibleMessage = "A comment must not contain invisible format characters such as direction overrides, isolates or tag characters; the zero-width joiner and non-joiner are fine."

// commentText is a comment as the server keeps it, the harmless characters
// stripped, or the refusal it draws, in Minka's order: the length of what was
// sent, control characters, the length of what is kept, nothing at all, then
// invisible characters. Minka reduces the markup before it judges the rest;
// the fake keeps what was sent, so the second length never refuses here, and
// nothing at all is a body that is empty once trimmed. A body of harmless
// characters alone is therefore kept empty, as Minka answers it today.
func commentText(w http.ResponseWriter, body string) (string, bool) {
	sent := phpTrim(body)
	kept := phpTrim(harmlessStripped(body))
	switch {
	case utf8.RuneCountInString(sent) > maxCommentLength:
		unprocessable(w, "commentTooLong", "The comment is too long.")
	case strings.IndexFunc(sent, isCommentControl) >= 0:
		unprocessable(w, "commentHasControlCharacters", "A comment must not contain control characters other than line breaks and tabs.")
	case utf8.RuneCountInString(kept) > maxCommentLength:
		unprocessable(w, "commentTooLong", "The comment is too long.")
	case sent == "":
		unprocessable(w, "commentMissing", "A comment needs a body.")
	case hasInvisibleCharactersInMarkup(kept):
		unprocessable(w, "commentHasInvisibleCharacters", commentInvisibleMessage)
	default:
		return kept, true
	}
	return "", false
}

// storedBody is a long text as the server keeps it. A value without a single
// element the server serves is no refusal: it is kept as the characters it
// consists of, escaped into one div, so that Markdown sent as HTML stands
// there afterwards as the characters somebody typed.
func storedBody(w http.ResponseWriter, kind, value string) (string, bool) {
	return storedRichText(w, kind+"BodyTooLong", "The body of a "+kind+" is too long.", maxDocumentBodyLength, value)
}

// storedRichText is a text written with the editor as the server keeps it:
// trimmed, refused where it is longer than the place takes, and escaped into
// one div where it carries no element the server serves.
func storedRichText(w http.ResponseWriter, tooLong, message string, maxLength int, value string) (string, bool) {
	value = phpTrim(value)
	if utf8.RuneCountInString(value) > maxLength {
		unprocessable(w, tooLong, message)
		return "", false
	}
	if value == "" {
		return "", true
	}
	if !richtext.CarriesMarkup(value) {
		return "<div>" + plainTextLineBreaks.ReplaceAllString(plainTextEscaper.Replace(value), "<br>$0") + "</div>", true
	}
	return value, true
}

// plainTextEscaper escapes what the server escapes in a text it keeps as
// characters, and nothing more: quotes stay as they are.
var plainTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// plainTextLineBreaks are the line breaks the server puts a <br> in front of,
// keeping the break itself. A pair of the two characters is one break.
var plainTextLineBreaks = regexp.MustCompile("\r\n|\n\r|\r|\n")

// expectedUpdatedAt reads the moment a guarded write names. A moment that is
// no moment refuses the request; a field left out and one set to null both
// expect nothing, which is the opposite of what null means for the notes of
// a card.
func expectedUpdatedAt(w http.ResponseWriter, fields map[string]any) (time.Time, bool, bool) {
	value, ok := stringField(w, fields, "expectedUpdatedAt")
	if !ok {
		return time.Time{}, false, false
	}
	if value == nil {
		return time.Time{}, false, true
	}
	at, read := aPointInTime(*value)
	if !read {
		malformed(w, `The field "expectedUpdatedAt" has to be a point in time as "updatedAt" gives it.`)
		return time.Time{}, false, false
	}
	return at, true, true
}

// folderToWriteIn is the folder a new entry goes into, and nil for the top of
// the tool. A folder this tool has not got is unknown; one in the archive
// takes nothing new, and in an archived project the project says so first.
func (s *Server) folderToWriteIn(w http.ResponseWriter, tool *FilesTool, folderID string) (*Folder, bool) {
	if folderID == "" {
		return nil, true
	}
	folder := folderInUse(tool, folderID)
	if folder == nil {
		unprocessable(w, "folderUnknown", "This Docs & Files tool has no such folder.")
		return nil, false
	}
	if folderIsOutOfUse(tool, folder.ID) {
		if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
			return nil, false
		}
		unprocessable(w, "folderArchived", "This folder is archived. Nothing new is written in it until it is back among the folders.")
		return nil, false
	}
	return folder, true
}

// refusedAsAnArchivedEntry says that an entry takes no change: it lies in the
// archive itself, or a folder above it does. A folder above it is named after
// the entry all the same, because that is what the write was aimed at.
func (s *Server) refusedAsAnArchivedEntry(w http.ResponseWriter, tool *FilesTool, folderID string, archived bool, kind string) bool {
	if !archived && !folderIsOutOfUse(tool, folderID) {
		return false
	}
	unprocessable(w, kind+"Archived", "This "+kind+" is archived. Nothing is written on it until it is back among the entries.")
	return true
}

// refusedAsSomebodyWhoCannotBeTold answers for the first person a new document
// cannot be told to, in the order the enrolment meets them: membership first,
// so that somebody outside the project hears that and not that the document is
// hidden from them, and then whether the document is shown to them at all,
// which only a client of the project is ever refused over.
//
// Whoever is on the list already, the writer from the start, is passed over,
// so a person named twice is told once.
func (s *Server) refusedAsSomebodyWhoCannotBeTold(w http.ResponseWriter, writer *Person, tool *FilesTool, folder *Folder, notifyIDs []string) bool {
	project := s.projectWithID(tool.ProjectID)
	enrolled := map[string]bool{writer.ID: true}
	for _, personID := range notifyIDs {
		if enrolled[personID] {
			continue
		}
		if project == nil || !slices.Contains(project.MemberIDs, personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a document.")
			return true
		}
		enrolled[personID] = true
		if s.isAClient(personID) && !shownToClients(tool, folderIDOf(folder)) {
			unprocessable(w, "personCannotSeeIt", "Somebody chosen is a client of the project, and this document is not shown to clients.")
			return true
		}
	}
	return false
}

// isAClient says whether a person is told only what the clients are shown: a
// client, or an agent working for one.
func (s *Server) isAClient(personID string) bool {
	person := s.People[personID]
	if person == nil {
		return false
	}
	if principal := s.People[person.PrincipalID]; principal != nil && principal.Client {
		return true
	}
	return person.Client
}

// shownToClients says whether the clients of the project are shown a new
// document written in this folder of the tool: the tool has to let them in,
// and the topmost folder of the chain decides for everything below it. A new
// document at the top of a tool is shown to no client.
func shownToClients(tool *FilesTool, folderID string) bool {
	return tool.ShownToClients && folderShownToClients(tool, folderID)
}

// folderShownToClients says whether the clients of the project are shown what
// stands in this folder. The topmost folder of the chain decides for
// everything below it; at the top of the tool there is no folder to ask, and
// what a new document there would say is no.
func folderShownToClients(tool *FilesTool, folderID string) bool {
	for seen := 0; folderID != "" && seen <= len(tool.Folders); seen++ {
		folder := folderIn(tool, folderID)
		if folder == nil {
			return false
		}
		if folder.ParentID == "" {
			return folder.ShownToClients
		}
		folderID = folder.ParentID
	}
	return false
}

func (s *Server) projectWithID(projectID string) *Project {
	for _, project := range s.Projects {
		if project.ID == projectID {
			return project
		}
	}
	return nil
}

// visibleFilesTool finds a tool in a project the person is a member of, and
// for a client only a tool the clients are let into.
func (s *Server) visibleFilesTool(toolID string, person *Person) *FilesTool {
	for _, tool := range s.FilesTools {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil && (tool.ShownToClients || !s.isAClient(person.ID)) {
			return tool
		}
	}
	return nil
}

// visibleDocument finds a published document by its identifier alone, as the
// API addresses it. A draft is at no address, its author included.
func (s *Server) visibleDocument(documentID string, person *Person) (*FilesTool, *Document) {
	if !uuidPattern.MatchString(documentID) {
		return nil, nil
	}
	for _, tool := range s.FilesTools {
		for _, document := range tool.Documents {
			if document.ID != documentID || document.Draft || document.Trashed || folderIsInTheTrash(tool, document.FolderID) {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil {
				return nil, nil
			}
			return tool, document
		}
	}
	return nil, nil
}

func (s *Server) visibleFilesUpload(uploadID string, person *Person) (*FilesTool, *FilesUpload) {
	if !uuidPattern.MatchString(uploadID) {
		return nil, nil
	}
	for _, tool := range s.FilesTools {
		for _, upload := range tool.Uploads {
			if upload.ID != uploadID || upload.Trashed || folderIsInTheTrash(tool, upload.FolderID) {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil {
				return nil, nil
			}
			return tool, upload
		}
	}
	return nil, nil
}

func folderIn(tool *FilesTool, folderID string) *Folder {
	for _, folder := range tool.Folders {
		if folder.ID == folderID {
			return folder
		}
	}
	return nil
}

// folderIsOutOfUse says whether this folder or one above it lies in the
// archive or in the trash. The top of a tool never is.
func folderIsOutOfUse(tool *FilesTool, folderID string) bool {
	return folderChainHolds(tool, folderID, func(folder *Folder) bool { return folder.Archived || folder.Trashed })
}

// folderIsInTheTrash says whether this folder or one above it lies in the
// trash, which takes everything below it out of sight.
func folderIsInTheTrash(tool *FilesTool, folderID string) bool {
	return folderChainHolds(tool, folderID, func(folder *Folder) bool { return folder.Trashed })
}

// folderChainHolds says whether this folder or one above it is as asked.
func folderChainHolds(tool *FilesTool, folderID string, holds func(*Folder) bool) bool {
	for seen := 0; folderID != "" && seen <= len(tool.Folders); seen++ {
		folder := folderIn(tool, folderID)
		if folder == nil {
			return false
		}
		if holds(folder) {
			return true
		}
		folderID = folder.ParentID
	}
	return false
}

// folderInUse is a folder of the tool that is out of the trash, and no
// folder above it is in the trash: one in the archive is still found.
func folderInUse(tool *FilesTool, folderID string) *Folder {
	folder := folderIn(tool, folderID)
	if folder == nil || folderIsInTheTrash(tool, folder.ID) {
		return nil
	}
	return folder
}

func folderIDOf(folder *Folder) string {
	if folder == nil {
		return ""
	}
	return folder.ID
}

// aWholeSecond is the moment the server notes, kept to the second as the
// column behind it is.
func (s *Server) aWholeSecond() time.Time {
	return s.now.Truncate(time.Second)
}

func later(one, other time.Time) time.Time {
	if other.After(one) {
		return other
	}
	return one
}

func (s *Server) entriesResource(tool *FilesTool, folder *Folder) map[string]any {
	archived := folder != nil && folderIsOutOfUse(tool, folder.ID)
	folderID := folderIDOf(folder)

	var shown []filesEntry
	folders := []any{}
	for _, entry := range tool.Folders {
		if entry.ParentID != folderID || entry.Archived || entry.Trashed {
			continue
		}
		shown = append(shown, filesEntry{kind: "folder", id: entry.ID})
		folders = append(folders, map[string]any{
			"id": entry.ID, "projectId": tool.ProjectID, "title": entry.Title, "entryCount": entriesIn(tool, entry.ID),
			"isBook":    entry.IsBook,
			"createdAt": timestamp(entry.CreatedAt), "creator": s.personReference(entry.CreatorID),
			"appUrl": s.filesToolURL(tool) + "/folders/" + entry.ID,
		})
	}

	documents := []any{}
	for _, document := range tool.Documents {
		if document.FolderID != folderID || document.Draft || document.Archived || document.Trashed {
			continue
		}
		shown = append(shown, filesEntry{kind: "document", id: document.ID})
		documents = append(documents, map[string]any{
			"id": document.ID, "projectId": tool.ProjectID, "title": document.Title, "lead": leadOf(document.Body),
			"inNavigation": !document.OutOfNavigation,
			"commentCount": len(document.Comments), "archived": archived,
			"createdAt": timestamp(document.CreatedAt), "updatedAt": timestamp(document.UpdatedAt),
			"creator": s.personReference(document.CreatorID), "labels": s.labelResources(s.LabelsOn(document.ID)),
			"appUrl": s.documentURL(tool, document),
		})
	}

	uploads := []any{}
	for _, upload := range tool.Uploads {
		if upload.FolderID != folderID || upload.Archived || upload.Trashed {
			continue
		}
		shown = append(shown, filesEntry{kind: "upload", id: upload.ID})
		uploads = append(uploads, map[string]any{
			"id": upload.ID, "projectId": tool.ProjectID, "title": upload.Title, "file": s.attachmentResource(upload.BlobID),
			"commentCount": len(upload.Comments), "archived": archived,
			"createdAt": timestamp(upload.CreatedAt), "updatedAt": timestamp(upload.UpdatedAt),
			"creator": s.personReference(upload.CreatorID), "labels": s.labelResources(s.LabelsOn(upload.ID)),
			"appUrl": s.filesUploadURL(tool, upload),
		})
	}

	appURL := s.filesToolURL(tool)
	if folder != nil {
		appURL += "/folders/" + folder.ID
	}
	entries := []any{}
	for _, entry := range inTheOrderShown(tool, folderID, shown) {
		entries = append(entries, map[string]any{"kind": entry.kind, "id": entry.id})
	}
	for _, line := range folders {
		s.withoutBooks(line.(map[string]any), "isBook")
	}
	for _, line := range documents {
		s.withoutBooks(line.(map[string]any), "inNavigation")
	}
	var parent *Folder
	var updatedAt any
	if folder != nil {
		parent = folderIn(tool, folder.ParentID)
		updatedAt = timestamp(folder.updatedAt())
	}
	resource := map[string]any{
		"projectId": tool.ProjectID, "project": s.projectReference(tool.ProjectID),
		"tool": map[string]any{"id": tool.ID, "title": tool.Title}, "folder": folderReference(folder),
		"parent": folderReference(parent), "updatedAt": updatedAt,
		"archived": archived, "path": s.pathTo(tool, folderID),
		"folders": folders, "documents": documents, "uploads": uploads,
		"order": orderOf(tool, folderID), "isBook": folder != nil && folder.IsBook,
		"book": folderReference(bookOf(tool, folderID)), "entries": entries,
		"appUrl": appURL,
	}
	if s.KnowsNoTree {
		for _, name := range []string{"projectId", "project", "parent", "updatedAt"} {
			delete(resource, name)
		}
	}
	return s.withoutBooks(resource, "order", "isBook", "book", "entries")
}

// entriesIn counts what a reader finds on opening a folder: the entries
// directly in it, without the drafts and without what lies in the archive.
func entriesIn(tool *FilesTool, folderID string) int {
	count := 0
	for _, folder := range tool.Folders {
		if folder.ParentID == folderID && !folder.Archived && !folder.Trashed {
			count++
		}
	}
	for _, document := range tool.Documents {
		if document.FolderID == folderID && !document.Draft && !document.Archived && !document.Trashed {
			count++
		}
	}
	for _, upload := range tool.Uploads {
		if upload.FolderID == folderID && !upload.Archived && !upload.Trashed {
			count++
		}
	}
	return count
}

// leadOf is the first characters of a body without its markup, cut where the
// count runs out and not at a word.
func leadOf(body string) string {
	text := phpTrim(html.UnescapeString(tagForm.ReplaceAllString(body, "")))
	if utf8.RuneCountInString(text) <= leadLength {
		return text
	}
	return string([]rune(text)[:leadLength])
}

func (s *Server) documentResource(tool *FilesTool, document *Document) map[string]any {
	return s.withoutBooks(map[string]any{
		"id": document.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"folder": folderReference(folderIn(tool, document.FolderID)), "path": s.pathTo(tool, document.FolderID),
		"title": document.Title, "body": document.Body, "bodySource": document.Body,
		"attachments":  s.attachmentsIn(tool.ProjectID, document.Body),
		"archived":     document.Archived || folderIsOutOfUse(tool, document.FolderID),
		"inNavigation": !document.OutOfNavigation, "book": folderReference(bookOf(tool, document.FolderID)),
		"commentCount": len(document.Comments),
		"createdAt":    timestamp(document.CreatedAt), "editedAt": editedAt(document.CreatedAt, document.UpdatedAt),
		"updatedAt": timestamp(document.UpdatedAt), "creator": s.personReference(document.CreatorID),
		"comments": s.commentResources(tool.ProjectID, s.documentURL(tool, document), document.Comments),
		"labels":   s.labelResources(s.LabelsOn(document.ID)),
		"appUrl":   s.documentURL(tool, document),
	}, "inNavigation", "book")
}

func (s *Server) filesUploadResource(tool *FilesTool, upload *FilesUpload) map[string]any {
	return map[string]any{
		"id": upload.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"folder": folderReference(folderIn(tool, upload.FolderID)), "path": s.pathTo(tool, upload.FolderID),
		"title": upload.Title, "file": s.attachmentResource(upload.BlobID),
		"description": upload.Description, "descriptionSource": upload.Description,
		"attachments":  s.attachmentsIn(tool.ProjectID, upload.Description),
		"archived":     upload.Archived || folderIsOutOfUse(tool, upload.FolderID),
		"commentCount": len(upload.Comments),
		"createdAt":    timestamp(upload.CreatedAt), "editedAt": editedAt(upload.CreatedAt, upload.UpdatedAt),
		"updatedAt": timestamp(upload.UpdatedAt), "creator": s.personReference(upload.CreatorID),
		"comments": s.commentResources(tool.ProjectID, s.filesUploadURL(tool, upload), upload.Comments),
		"labels":   s.labelResources(s.LabelsOn(upload.ID)),
		"appUrl":   s.filesUploadURL(tool, upload),
	}
}

// editedAt is the moment somebody wrote over an entry, and nothing while
// nobody has: the entry then stands at the moment it was made.
func editedAt(createdAt, updatedAt time.Time) any {
	if !updatedAt.After(createdAt) {
		return nil
	}
	return timestamp(updatedAt)
}

// attachmentResource describes the file an upload stands for, as a file
// hanging in a text is described.
func (s *Server) attachmentResource(blobID string) any {
	for _, file := range s.Uploads {
		if file.ID != blobID {
			continue
		}
		return map[string]any{
			"id": file.ID, "filename": file.Filename, "contentType": file.ContentType,
			"byteSize": len(file.Content), "isImage": isPicture(file.ContentType),
			"url": s.URL + "/api/v1/uploads/" + file.ID,
		}
	}
	return nil
}

func folderReference(folder *Folder) any {
	if folder == nil {
		return nil
	}
	return map[string]any{"id": folder.ID, "title": folder.Title}
}

// pathTo names the folders from the top of the tool down to this one, this
// one last.
func (s *Server) pathTo(tool *FilesTool, folderID string) []any {
	var path []any
	for seen := 0; folderID != "" && seen <= len(tool.Folders); seen++ {
		folder := folderIn(tool, folderID)
		if folder == nil {
			break
		}
		path = append([]any{folderReference(folder)}, path...)
		folderID = folder.ParentID
	}
	if path == nil {
		return []any{}
	}
	return path
}

func (s *Server) projectReference(projectID string) any {
	project := s.projectWithID(projectID)
	if project == nil {
		return nil
	}
	return map[string]any{"id": project.ID, "name": project.Name}
}

// toolResource is a tool as its own address answers it: with the project it
// stands in, so that whoever holds only the tool learns its project.
func (s *Server) toolResource(id, projectID, title, appURL string) map[string]any {
	return map[string]any{"id": id, "projectId": projectID, "project": s.projectReference(projectID), "title": title, "appUrl": appURL}
}

func (s *Server) filesToolURL(tool *FilesTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/files/" + tool.ID
}

func (s *Server) documentURL(tool *FilesTool, document *Document) string {
	return s.filesToolURL(tool) + "/" + document.ID
}

func (s *Server) filesUploadURL(tool *FilesTool, upload *FilesUpload) string {
	return s.filesToolURL(tool) + "/uploads/" + upload.ID
}
