package fakeapi

import (
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/richtext"
)

// What the server keeps of a document, and how much of its body a folder
// shows beside its title.
const (
	maxDocumentBodyLength = 100000
	leadLength            = 240
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

	CreatedAt time.Time
	CreatorID string
}

// Document is a page of a tool. Its moment is kept to the whole second, as the
// column behind it is, so the updatedAt a reader gets back is the one a guarded
// write sends in again.
type Document struct {
	ID        string
	FolderID  string // empty at the top of the tool
	Title     string
	Body      string // the HTML as stored
	Draft     bool   // at no address, its author included
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatorID string
	Comments  []*Comment
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
}

func (s *Server) registerFilesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/files", s.authenticated(s.filesTools))
	mux.HandleFunc("GET /api/v1/files-tools/{toolId}/entries", s.authenticated(s.filesEntries))
	mux.HandleFunc("POST /api/v1/files-tools/{toolId}/documents", s.authenticated(s.addDocument))
	mux.HandleFunc("GET /api/v1/documents/{documentId}", s.authenticated(s.document))
	mux.HandleFunc("PATCH /api/v1/documents/{documentId}", s.authenticated(s.updateDocument))
	mux.HandleFunc("GET /api/v1/files-uploads/{uploadId}", s.authenticated(s.filesUpload))

	for _, entry := range []string{"documents/{documentId}", "files-uploads/{uploadId}"} {
		under := "/api/v1/" + entry + "/comments"
		mux.HandleFunc("GET "+under, s.authenticated(s.commentsOnEntry))
		mux.HandleFunc("POST "+under, s.authenticated(s.writeCommentOnEntry))
		mux.HandleFunc("PATCH "+under+"/{commentId}", s.authenticated(s.editCommentOnEntry))
		mux.HandleFunc("DELETE "+under+"/{commentId}", s.authenticated(s.removeCommentOnEntry))
		mux.HandleFunc("POST "+under+"/{commentId}/reactions", s.authenticated(s.reactToCommentOnEntry))
		mux.HandleFunc("DELETE "+under+"/{commentId}/reactions", s.authenticated(s.unreactFromCommentOnEntry))
	}
}

func (s *Server) filesTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}

	tools := []any{}
	for _, tool := range s.FilesTools {
		if tool.ProjectID == project.ID {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.filesToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
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
	folderID := r.URL.Query().Get("folderId")
	var folder *Folder
	if folderID != "" {
		if folder = folderIn(tool, folderID); folder == nil {
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
	writeJSON(w, http.StatusOK, s.documentResource(tool, document))
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
	if !validEntryTitle(w, "document", deref(title)) {
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
	folder, ok := folderToWriteIn(w, tool, deref(folderID))
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
	tool.Documents = append([]*Document{document}, tool.Documents...)
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
	if title != nil && !validEntryTitle(w, "document", *title) {
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

	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.refusedAsAnArchivedEntry(w, tool, document.FolderID, document.Archived, "document") {
		return
	}
	// The text that stands already is no write and no conflict, whatever the
	// expectation: nothing was lost.
	if newTitle == document.Title && newBody == document.Body {
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
	document.Title, document.Body = newTitle, newBody
	document.UpdatedAt = later(s.aWholeSecond(), document.UpdatedAt.Add(time.Second))
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

// commentedEntry is what the comments under a document and under an upload
// have in common: where they stand, whether anything may be written on them,
// and how the entry answers for itself.
type commentedEntry struct {
	tool     *FilesTool
	folderID string
	archived bool
	kind     string // "document" or "upload", which is what a refusal names
	comments *[]*Comment
}

// entryOfTheRequest finds the document or the upload an address names. Which
// of the two it is the address says, and an identifier of one is nothing at
// the address of the other.
func (s *Server) entryOfTheRequest(r *http.Request, person *Person) *commentedEntry {
	if documentID := r.PathValue("documentId"); documentID != "" {
		tool, document := s.visibleDocument(documentID, person)
		if document == nil {
			return nil
		}
		return &commentedEntry{tool: tool, folderID: document.FolderID, archived: document.Archived, kind: "document", comments: &document.Comments}
	}
	tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
	if upload == nil {
		return nil
	}
	return &commentedEntry{tool: tool, folderID: upload.FolderID, archived: upload.Archived, kind: "upload", comments: &upload.Comments}
}

// refusedAsClosed says why nothing was written under an entry: the project
// archived, or the entry itself or a folder above it.
func (s *Server) refusedAsClosed(w http.ResponseWriter, entry *commentedEntry) bool {
	return s.refusedAsArchived(w, s.projectWithID(entry.tool.ProjectID)) ||
		s.refusedAsAnArchivedEntry(w, entry.tool, entry.folderID, entry.archived, entry.kind)
}

func (s *Server) commentsOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	entry := s.entryOfTheRequest(r, person)
	if entry == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.commentResources(entry))
}

func (s *Server) writeCommentOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	entry := s.entryOfTheRequest(r, person)
	if entry == nil {
		notFound(w)
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text, ok := commentText(w, deref(body))
	if !ok {
		return
	}
	if s.refusedAsClosed(w, entry) {
		return
	}

	comment := &Comment{ID: newID(), AuthorID: person.ID, Body: text, CreatedAt: s.now}
	*entry.comments = append(*entry.comments, comment)
	writeJSON(w, http.StatusCreated, s.commentResource(entry.tool.ProjectID, comment))
}

func (s *Server) editCommentOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	entry := s.entryOfTheRequest(r, person)
	if entry == nil {
		notFound(w)
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	text, ok := commentText(w, deref(body))
	if !ok {
		return
	}
	if s.refusedAsClosed(w, entry) {
		return
	}
	comment := ownCommentOf(*entry.comments, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}

	if comment.Body != text {
		now := s.now
		comment.Body, comment.EditedAt = text, &now
	}
	writeJSON(w, http.StatusOK, s.commentResource(entry.tool.ProjectID, comment))
}

func (s *Server) removeCommentOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	entry := s.entryOfTheRequest(r, person)
	if entry == nil {
		notFound(w)
		return
	}
	if s.refusedAsClosed(w, entry) {
		return
	}
	comment := ownCommentOf(*entry.comments, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}

	*entry.comments = slices.DeleteFunc(*entry.comments, func(c *Comment) bool { return c == comment })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reactToCommentOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	entry := s.entryOfTheRequest(r, person)
	comment := commentIn(entry, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	content, ok := reactionContent(w, r)
	if !ok {
		return
	}
	if s.refusedAsClosed(w, entry) {
		return
	}

	status := reactTo(&comment.Reactions, content, person.ID)
	writeJSON(w, status, s.commentResource(entry.tool.ProjectID, comment))
}

func (s *Server) unreactFromCommentOnEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	entry := s.entryOfTheRequest(r, person)
	comment := commentIn(entry, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	if s.refusedAsClosed(w, entry) {
		return
	}

	unreactFrom(&comment.Reactions, r.URL.Query().Get("content"), person.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func commentIn(entry *commentedEntry, commentID string) *Comment {
	if entry == nil {
		return nil
	}
	for _, comment := range *entry.comments {
		if comment.ID == commentID {
			return comment
		}
	}
	return nil
}

func ownCommentOf(comments []*Comment, commentID string, person *Person) *Comment {
	for _, comment := range comments {
		if comment.ID == commentID && comment.AuthorID == person.ID {
			return comment
		}
	}
	return nil
}

// commentText is a comment as the server keeps it, or the refusal it draws.
func commentText(w http.ResponseWriter, body string) (string, bool) {
	text := phpTrim(body)
	switch {
	case strings.IndexFunc(text, isCommentControl) >= 0:
		unprocessable(w, "commentHasControlCharacters", "A comment must not contain control characters other than line breaks and tabs.")
	case utf8.RuneCountInString(text) > maxCommentLength:
		unprocessable(w, "commentTooLong", "The comment is too long.")
	case text == "":
		unprocessable(w, "commentMissing", "A comment needs a body.")
	default:
		return text, true
	}
	return "", false
}

// validEntryTitle holds a title against what the server keeps, and names the
// kind of entry in the refusal, as the controller does.
func validEntryTitle(w http.ResponseWriter, kind, title string) bool {
	title = phpTrim(title)
	switch {
	case utf8.RuneCountInString(title) > maxTitleLength:
		unprocessable(w, kind+"TitleTooLong", "The title of a "+kind+" is too long.")
	case strings.IndexFunc(title, unicode.IsControl) >= 0:
		unprocessable(w, kind+"TitleHasControlCharacters", "The title of a "+kind+" must not contain control characters.")
	case title == "":
		unprocessable(w, kind+"TitleMissing", "A "+kind+" needs a title.")
	default:
		return true
	}
	return false
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
// takes nothing new.
func folderToWriteIn(w http.ResponseWriter, tool *FilesTool, folderID string) (*Folder, bool) {
	if folderID == "" {
		return nil, true
	}
	folder := folderIn(tool, folderID)
	if folder == nil {
		unprocessable(w, "folderUnknown", "This Docs & Files tool has no such folder.")
		return nil, false
	}
	if folderIsOutOfUse(tool, folder.ID) {
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
// The writer is on the list already and is passed over. Anybody else named a
// second time finds themselves on it too, and Minka reads that as somebody it
// could not enrol, so the second naming is answered as a stranger would be.
func (s *Server) refusedAsSomebodyWhoCannotBeTold(w http.ResponseWriter, writer *Person, tool *FilesTool, folder *Folder, notifyIDs []string) bool {
	project := s.projectWithID(tool.ProjectID)
	enrolled := map[string]bool{}
	for _, personID := range notifyIDs {
		if personID == writer.ID {
			continue
		}
		if project == nil || !slices.Contains(project.MemberIDs, personID) || enrolled[personID] {
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

func (s *Server) visibleFilesTool(toolID string, person *Person) *FilesTool {
	for _, tool := range s.FilesTools {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil {
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
			if document.ID != documentID || document.Draft {
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
			if upload.ID != uploadID {
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
// archive. The top of a tool never is.
func folderIsOutOfUse(tool *FilesTool, folderID string) bool {
	for seen := 0; folderID != "" && seen <= len(tool.Folders); seen++ {
		folder := folderIn(tool, folderID)
		if folder == nil {
			return false
		}
		if folder.Archived {
			return true
		}
		folderID = folder.ParentID
	}
	return false
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

	folders := []any{}
	for _, entry := range tool.Folders {
		if entry.ParentID != folderID || entry.Archived {
			continue
		}
		folders = append(folders, map[string]any{
			"id": entry.ID, "title": entry.Title, "entryCount": entriesIn(tool, entry.ID),
			"createdAt": timestamp(entry.CreatedAt), "creator": s.personReference(entry.CreatorID),
			"appUrl": s.filesToolURL(tool) + "/folders/" + entry.ID,
		})
	}

	documents := []any{}
	for _, document := range tool.Documents {
		if document.FolderID != folderID || document.Draft || document.Archived {
			continue
		}
		documents = append(documents, map[string]any{
			"id": document.ID, "title": document.Title, "lead": leadOf(document.Body),
			"commentCount": len(document.Comments), "archived": archived,
			"createdAt": timestamp(document.CreatedAt), "updatedAt": timestamp(document.UpdatedAt),
			"creator": s.personReference(document.CreatorID), "appUrl": s.documentURL(tool, document),
		})
	}

	uploads := []any{}
	for _, upload := range tool.Uploads {
		if upload.FolderID != folderID || upload.Archived {
			continue
		}
		uploads = append(uploads, map[string]any{
			"id": upload.ID, "title": upload.Title, "file": s.attachmentResource(upload.BlobID),
			"commentCount": len(upload.Comments), "archived": archived,
			"createdAt": timestamp(upload.CreatedAt), "updatedAt": timestamp(upload.UpdatedAt),
			"creator": s.personReference(upload.CreatorID), "appUrl": s.filesUploadURL(tool, upload),
		})
	}

	appURL := s.filesToolURL(tool)
	if folder != nil {
		appURL += "/folders/" + folder.ID
	}
	return map[string]any{
		"tool": map[string]any{"id": tool.ID, "title": tool.Title}, "folder": folderReference(folder),
		"archived": archived, "path": s.pathTo(tool, folderID),
		"folders": folders, "documents": documents, "uploads": uploads, "appUrl": appURL,
	}
}

// entriesIn counts what a reader finds on opening a folder: the entries
// directly in it, without the drafts and without what lies in the archive.
func entriesIn(tool *FilesTool, folderID string) int {
	count := 0
	for _, folder := range tool.Folders {
		if folder.ParentID == folderID && !folder.Archived {
			count++
		}
	}
	for _, document := range tool.Documents {
		if document.FolderID == folderID && !document.Draft && !document.Archived {
			count++
		}
	}
	for _, upload := range tool.Uploads {
		if upload.FolderID == folderID && !upload.Archived {
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
	return map[string]any{
		"id": document.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"folder": folderReference(folderIn(tool, document.FolderID)), "path": s.pathTo(tool, document.FolderID),
		"title": document.Title, "body": document.Body, "bodySource": document.Body,
		"attachments":  s.attachmentsIn(tool.ProjectID, document.Body),
		"archived":     document.Archived || folderIsOutOfUse(tool, document.FolderID),
		"commentCount": len(document.Comments),
		"createdAt":    timestamp(document.CreatedAt), "editedAt": editedAt(document.CreatedAt, document.UpdatedAt),
		"updatedAt": timestamp(document.UpdatedAt), "creator": s.personReference(document.CreatorID),
		"comments": s.commentResources(&commentedEntry{tool: tool, comments: &document.Comments}),
		"appUrl":   s.documentURL(tool, document),
	}
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
		"comments": s.commentResources(&commentedEntry{tool: tool, comments: &upload.Comments}),
		"appUrl":   s.filesUploadURL(tool, upload),
	}
}

func (s *Server) commentResources(entry *commentedEntry) []any {
	comments := []any{}
	for _, comment := range *entry.comments {
		comments = append(comments, s.commentResource(entry.tool.ProjectID, comment))
	}
	return comments
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

func (s *Server) filesToolURL(tool *FilesTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/files/" + tool.ID
}

func (s *Server) documentURL(tool *FilesTool, document *Document) string {
	return s.filesToolURL(tool) + "/" + document.ID
}

func (s *Server) filesUploadURL(tool *FilesTool, upload *FilesUpload) string {
	return s.filesToolURL(tool) + "/uploads/" + upload.ID
}
