package fakeapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
)

// EntryState is what a folder, a document and an upload keep about the trash
// and about when and by whom they went into the archive.
type EntryState struct {
	// Trashed takes the entry out of sight, and everything below a folder with
	// it: what stands in the folder keeps its own state and comes back with
	// the folder.
	Trashed   bool
	TrashedAt time.Time
	TrashedBy string // person ID
	trashSeq  int

	ArchivedAt time.Time
	ArchivedBy string // person ID, empty where nobody is known
}

// filesEntry is a folder, a document or an upload seen through what the
// writes that put a tool in order share.
type filesEntry struct {
	kind     string // "folder", "document" or "upload"
	tool     *FilesTool
	id       string
	title    string
	parent   *string // the folder it stands in, empty at the top of the tool
	archived *bool   // the entry itself, not a folder above it
	creator  string
	draft    bool
	state    *EntryState
	folder   *Folder
	document *Document
	upload   *FilesUpload
}

func (s *Server) registerFilesOrderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/files-tools/{toolId}/archive", s.authenticated(s.filesArchive))
	mux.HandleFunc("POST /api/v1/files-tools/{toolId}/folders", s.authenticated(s.addFolder))
	mux.HandleFunc("PATCH /api/v1/files-folders/{folderId}", s.authenticated(s.updateFolder))
	mux.HandleFunc("POST /api/v1/documents/{documentId}/versions/{versionId}/restore", s.authenticated(s.restoreDocumentVersion))

	for _, address := range []struct{ kind, base, argument string }{
		{"folder", "files-folders", "folderId"},
		{"document", "documents", "documentId"},
		{"upload", "files-uploads", "uploadId"},
	} {
		path := "/api/v1/" + address.base + "/{" + address.argument + "}"
		mux.HandleFunc("DELETE "+path, s.authenticated(s.removeFilesEntry(address.kind, address.argument)))
		mux.HandleFunc("POST "+path+"/restore", s.authenticated(s.restoreFilesEntry(address.kind, address.argument)))
		mux.HandleFunc("POST "+path+"/archive", s.authenticated(s.archiveFilesEntry(address.kind, address.argument)))
		mux.HandleFunc("POST "+path+"/unarchive", s.authenticated(s.unarchiveFilesEntry(address.kind, address.argument)))
		mux.HandleFunc("POST "+path+"/moves", s.authenticated(s.moveFilesEntry(address.kind, address.argument)))
	}
}

// filesArchive lists the entries of a tool that went into the archive
// themselves, at any depth, the one archived last first. What stands below a
// folder in the trash is gone with it.
func (s *Server) filesArchive(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	var archived []filesEntry
	for _, entry := range entriesOf(tool) {
		if *entry.archived && !entry.state.Trashed && !entry.draft && !folderIsInTheTrash(tool, *entry.parent) {
			archived = append(archived, entry)
		}
	}
	slices.SortStableFunc(archived, func(a, b filesEntry) int {
		if !a.state.ArchivedAt.Equal(b.state.ArchivedAt) {
			return b.state.ArchivedAt.Compare(a.state.ArchivedAt)
		}
		return strings.Compare(a.id, b.id)
	})

	list := []any{}
	for _, entry := range archived {
		list = append(list, map[string]any{
			"id": entry.id, "projectId": tool.ProjectID, "kind": entry.kind, "title": entry.title,
			"archivedAt": timestamp(entry.state.ArchivedAt), "archivedBy": s.personReferenceOrNil(entry.state.ArchivedBy),
			"archivedWithAFolderAbove": folderIsOutOfUse(tool, *entry.parent),
			"appUrl":                   s.entryURL(entry),
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// addFolder makes a folder on top of the tool or of another folder. Only the
// team shows one to clients, and only at the top of the tool; a client's own
// folder there is shown to clients anyway.
func (s *Server) addFolder(w http.ResponseWriter, r *http.Request, person *Person) {
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
	if !ok || !validTitle(w, "folder", "a folder", deref(title)) {
		return
	}
	parentID, ok := stringField(w, fields, "folderId")
	if !ok {
		return
	}
	parent, ok := s.folderToWriteIn(w, tool, deref(parentID))
	if !ok {
		return
	}
	shown := false
	if parent == nil {
		visible, ok := booleanField(w, fields, "visibleToClients")
		if !ok {
			return
		}
		shown = visible != nil && *visible || s.isAClient(person.ID)
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}

	folder := &Folder{
		ID: newID(), Title: phpTrim(deref(title)), ParentID: folderIDOf(parent), ShownToClients: shown,
		CreatedAt: s.aWholeSecond(), CreatorID: person.ID,
	}
	tool.Folders = append([]*Folder{folder}, tool.Folders...)
	writeJSON(w, http.StatusCreated, s.entriesResource(tool, folder))
}

// updateFolder renames a folder; without a title it writes nothing.
func (s *Server) updateFolder(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	entry, found := s.visibleEntry("folder", r.PathValue("folderId"), person)
	if !found {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	if title != nil {
		if !validTitle(w, "folder", "a folder", *title) {
			return
		}
		if s.refusedAsArchived(w, s.projectWithID(entry.tool.ProjectID)) || refusedAsAnArchivedFolder(w, entry) {
			return
		}
		entry.folder.Title = phpTrim(*title)
	}
	writeJSON(w, http.StatusOK, s.entriesResource(entry.tool, entry.folder))
}

// removeFilesEntry puts an entry into the trash of its project, from the tool
// or from its archive, a folder with everything in it. A client puts away only
// what they made. Why nothing was written is asked afterwards, as the
// controller asks it.
func (s *Server) removeFilesEntry(kind, argument string) handler {
	return func(w http.ResponseWriter, r *http.Request, person *Person) {
		entry, found := s.visibleEntry(kind, r.PathValue(argument), person)
		if !found {
			notFound(w)
			return
		}
		project := s.projectWithID(entry.tool.ProjectID)
		if !s.mayPutAway(entry, person) || project.Archived || folderIsOutOfUse(entry.tool, *entry.parent) {
			if !s.refusedAsArchived(w, project) && !refusedBelowAnArchivedFolder(w, entry) {
				notFound(w)
			}
			return
		}

		s.trashSeq++
		entry.state.Trashed, entry.state.TrashedAt, entry.state.TrashedBy, entry.state.trashSeq = true, s.now, person.ID, s.trashSeq
		*entry.archived, entry.state.ArchivedAt, entry.state.ArchivedBy = false, time.Time{}, ""
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNoContent)
	}
}

// restoreFilesEntry brings an entry back out of the trash, the only address
// at which one in the trash answers. Only the team brings anything back, and
// only what lies in the trash by itself, below folders in use.
func (s *Server) restoreFilesEntry(kind, argument string) handler {
	return func(w http.ResponseWriter, r *http.Request, person *Person) {
		entry, found := s.trashedEntry(kind, r.PathValue(argument), person)
		if !found {
			notFound(w)
			return
		}
		if s.refusedAsArchived(w, s.projectWithID(entry.tool.ProjectID)) {
			return
		}
		if folderIsOutOfUse(entry.tool, *entry.parent) {
			notFound(w)
			return
		}

		entry.state.Trashed, entry.state.TrashedAt, entry.state.TrashedBy = false, time.Time{}, ""
		writeJSON(w, http.StatusOK, s.entryResource(entry))
	}
}

// archiveFilesEntry takes an entry into the archive of its tool. A client puts
// away only what they made.
func (s *Server) archiveFilesEntry(kind, argument string) handler {
	return func(w http.ResponseWriter, r *http.Request, person *Person) {
		entry, found := s.visibleEntry(kind, r.PathValue(argument), person)
		if !found {
			notFound(w)
			return
		}
		project := s.projectWithID(entry.tool.ProjectID)
		if !s.mayPutAway(entry, person) || project.Archived || isArchivedOrBelowTheArchive(entry) {
			if !s.refusedAsArchived(w, project) && !refusedAsAnArchivedEntryOfAnyKind(w, entry) {
				notFound(w)
			}
			return
		}

		*entry.archived = true
		entry.state.ArchivedAt, entry.state.ArchivedBy = s.momentAfterEveryFilesArchiving(entry.tool), person.ID
		writeJSON(w, http.StatusOK, s.entryResource(entry))
	}
}

// unarchiveFilesEntry brings an entry back out of the archive. Only the team
// does; one archived with a folder above it comes back only with that folder.
func (s *Server) unarchiveFilesEntry(kind, argument string) handler {
	return func(w http.ResponseWriter, r *http.Request, person *Person) {
		entry, found := s.visibleEntry(kind, r.PathValue(argument), person)
		if !found {
			notFound(w)
			return
		}
		project := s.projectWithID(entry.tool.ProjectID)
		folderAboveOutOfUse := folderIsOutOfUse(entry.tool, *entry.parent)
		if !*entry.archived || folderAboveOutOfUse || s.isAClient(person.ID) || project.Archived {
			switch {
			case s.refusedAsArchived(w, project):
			case s.isAClient(person.ID):
				notFound(w)
			case !isArchivedOrBelowTheArchive(entry):
				unprocessable(w, kind+"NotArchived", "This "+kind+" is not archived.")
			case folderAboveOutOfUse:
				unprocessable(w, "folderArchived", "A folder above it is archived. What stands in it comes back with that folder.")
			default:
				notFound(w)
			}
			return
		}

		*entry.archived, entry.state.ArchivedAt, entry.state.ArchivedBy = false, time.Time{}, ""
		writeJSON(w, http.StatusOK, s.entryResource(entry))
	}
}

// moveFilesEntry moves an entry to the top of a folder of the same tool, or
// of the tool for a folderId of null. The body and the target are read before
// an archived project refuses the move, one to where the entry stands already
// included; there nothing is written.
func (s *Server) moveFilesEntry(kind, argument string) handler {
	return func(w http.ResponseWriter, r *http.Request, person *Person) {
		entry, found := s.visibleEntry(kind, r.PathValue(argument), person)
		if !found {
			notFound(w)
			return
		}
		project := s.projectWithID(entry.tool.ProjectID)
		if isArchivedOrBelowTheArchive(entry) {
			if !s.refusedAsArchived(w, project) {
				refusedAsAnArchivedEntryOfAnyKind(w, entry)
			}
			return
		}
		fields, ok := readBody(w, r)
		if !ok {
			return
		}
		if _, given := fields["folderId"]; !given {
			unprocessable(w, "folderIdMissing", `A move names the folder to move to in "folderId", null for the top of the tool.`)
			return
		}
		targetID, ok := stringField(w, fields, "folderId")
		if !ok {
			return
		}
		target, ok := s.folderToWriteIn(w, entry.tool, deref(targetID))
		if !ok || s.refusedAsArchived(w, project) {
			return
		}
		if kind == "folder" && target != nil && folderChainHolds(entry.tool, target.ID, func(folder *Folder) bool { return folder.ID == entry.id }) {
			unprocessable(w, "folderIntoItself", "A folder cannot move into itself or a folder below it.")
			return
		}

		if folderIDOf(target) != *entry.parent {
			*entry.parent = folderIDOf(target)
			entry.putOnTop()
		}
		writeJSON(w, http.StatusOK, s.entryResource(entry))
	}
}

// restoreDocumentVersion writes what a version said as the newest version of
// its document, and tells nobody. A version that says what the document says
// is no write and no mistake. The team's alone.
func (s *Server) restoreDocumentVersion(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil || s.isAClient(person.ID) || !uuidPattern.MatchString(r.PathValue("versionId")) {
		notFound(w)
		return
	}
	var version *DocumentVersion
	for _, one := range document.versions() {
		if one.ID == r.PathValue("versionId") {
			version = one
		}
	}
	if version == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) || s.refusedAsAnArchivedEntry(w, tool, document.FolderID, document.Archived, "document") {
		return
	}

	if version.Title != document.Title || version.Body != document.Body {
		document.Title, document.Body = version.Title, version.Body
		document.UpdatedAt = later(s.aWholeSecond(), document.UpdatedAt.Add(time.Second))
		document.keepAVersion(person.ID, document.UpdatedAt)
	}
	writeJSON(w, http.StatusOK, s.documentResource(tool, document))
}

// mayPutAway says whether this person may take the entry into the trash or
// the archive: the team may take anything, a client only what they made.
func (s *Server) mayPutAway(entry filesEntry, person *Person) bool {
	return entry.creator == person.ID || !s.isAClient(person.ID)
}

// visibleEntry finds an entry in use or in the archive by its identifier
// alone, in a tool this person is let into. A draft is at no address.
func (s *Server) visibleEntry(kind, id string, person *Person) (filesEntry, bool) {
	if !uuidPattern.MatchString(id) {
		return filesEntry{}, false
	}
	for _, tool := range s.FilesTools {
		for _, entry := range entriesOf(tool) {
			if entry.kind != kind || entry.id != id {
				continue
			}
			if entry.draft || entry.state.Trashed || folderIsInTheTrash(tool, *entry.parent) || s.visibleFilesTool(tool.ID, person) == nil {
				return filesEntry{}, false
			}
			if s.isAClient(person.ID) && !s.shownToClientsAsAnEntry(entry) {
				return filesEntry{}, false
			}
			return entry, true
		}
	}
	return filesEntry{}, false
}

// shownToClientsAsAnEntry says whether the clients of the project see the
// entry: the topmost folder above it decides, and at the top of the tool a
// folder decides for itself. A document or upload there is shown to clients
// where a client made it, which is the only way one at the top is shown
// through the API.
func (s *Server) shownToClientsAsAnEntry(entry filesEntry) bool {
	if entry.kind == "folder" {
		return folderShownToClients(entry.tool, entry.id)
	}
	if *entry.parent == "" {
		return s.isAClient(entry.creator)
	}
	return folderShownToClients(entry.tool, *entry.parent)
}

// trashedEntry finds an entry that lies in the trash by itself, for the team
// of its project alone. A draft is at no address, its author's included.
func (s *Server) trashedEntry(kind, id string, person *Person) (filesEntry, bool) {
	if !uuidPattern.MatchString(id) {
		return filesEntry{}, false
	}
	for _, tool := range s.FilesTools {
		for _, entry := range entriesOf(tool) {
			if entry.kind != kind || entry.id != id {
				continue
			}
			if !entry.state.Trashed || entry.draft || s.visibleProject(tool.ProjectID, person) == nil || s.isAClient(person.ID) {
				return filesEntry{}, false
			}
			return entry, true
		}
	}
	return filesEntry{}, false
}

// entriesOf are the folders, documents and uploads of a tool, in every state.
func entriesOf(tool *FilesTool) []filesEntry {
	var entries []filesEntry
	for _, folder := range tool.Folders {
		entries = append(entries, filesEntry{
			kind: "folder", tool: tool, id: folder.ID, title: folder.Title, parent: &folder.ParentID,
			archived: &folder.Archived, creator: folder.CreatorID, state: &folder.EntryState, folder: folder,
		})
	}
	for _, document := range tool.Documents {
		entries = append(entries, filesEntry{
			kind: "document", tool: tool, id: document.ID, title: document.Title, parent: &document.FolderID,
			archived: &document.Archived, creator: document.CreatorID, draft: document.Draft, state: &document.EntryState, document: document,
		})
	}
	for _, upload := range tool.Uploads {
		entries = append(entries, filesEntry{
			kind: "upload", tool: tool, id: upload.ID, title: upload.Title, parent: &upload.FolderID,
			archived: &upload.Archived, creator: upload.CreatorID, state: &upload.EntryState, upload: upload,
		})
	}
	return entries
}

// putOnTop puts the entry first among its kind, which is where a move puts it
// in the folder it moves to.
func (entry filesEntry) putOnTop() {
	switch entry.kind {
	case "folder":
		entry.tool.Folders = onTop(entry.tool.Folders, entry.folder)
	case "document":
		entry.tool.Documents = onTop(entry.tool.Documents, entry.document)
	default:
		entry.tool.Uploads = onTop(entry.tool.Uploads, entry.upload)
	}
}

func onTop[T comparable](list []T, one T) []T {
	return append([]T{one}, slices.DeleteFunc(list, func(other T) bool { return other == one })...)
}

// isArchivedOrBelowTheArchive says whether the entry or a folder above it
// lies in the archive, which is what an entry's "archived" says.
func isArchivedOrBelowTheArchive(entry filesEntry) bool {
	return *entry.archived || folderIsOutOfUse(entry.tool, *entry.parent)
}

// refusedAsAnArchivedEntryOfAnyKind names the entry a write was aimed at as
// archived, itself or through a folder above it.
func refusedAsAnArchivedEntryOfAnyKind(w http.ResponseWriter, entry filesEntry) bool {
	if entry.kind == "folder" {
		return refusedAsAnArchivedFolder(w, entry)
	}
	if !isArchivedOrBelowTheArchive(entry) {
		return false
	}
	unprocessable(w, entry.kind+"Archived", "This "+entry.kind+" is archived. Nothing is written on it until it is back among the entries.")
	return true
}

func refusedAsAnArchivedFolder(w http.ResponseWriter, entry filesEntry) bool {
	if !isArchivedOrBelowTheArchive(entry) {
		return false
	}
	unprocessable(w, "folderArchived", "This folder is archived. Nothing in it changes until it is back among the folders.")
	return true
}

// refusedBelowAnArchivedFolder says that what stands in an archived folder is
// not taken out on its own.
func refusedBelowAnArchivedFolder(w http.ResponseWriter, entry filesEntry) bool {
	if !folderIsOutOfUse(entry.tool, *entry.parent) {
		return false
	}
	unprocessable(w, "folderArchived", "A folder above it is archived. What stands in it comes back with that folder.")
	return true
}

// entryResource is the entry as its own address answers it: a folder with
// what stands in it, a document or an upload as it is read.
func (s *Server) entryResource(entry filesEntry) map[string]any {
	switch entry.kind {
	case "folder":
		return s.entriesResource(entry.tool, entry.folder)
	case "document":
		return s.documentResource(entry.tool, entry.document)
	default:
		return s.filesUploadResource(entry.tool, entry.upload)
	}
}

func (s *Server) entryURL(entry filesEntry) string {
	switch entry.kind {
	case "folder":
		return s.filesToolURL(entry.tool) + "/folders/" + entry.id
	case "document":
		return s.documentURL(entry.tool, entry.document)
	default:
		return s.filesUploadURL(entry.tool, entry.upload)
	}
}

// momentAfterEveryFilesArchiving is when an entry goes into the archive: after
// every entry of the tool that went before it, so that the archive has an
// order to read even while the clock of the fake stands still.
func (s *Server) momentAfterEveryFilesArchiving(tool *FilesTool) time.Time {
	moment := s.now
	for _, entry := range entriesOf(tool) {
		if *entry.archived && !entry.state.ArchivedAt.Before(moment) {
			moment = entry.state.ArchivedAt.Add(time.Second)
		}
	}
	return moment
}

// trashedFilesEntries are the rows the trash of a project lists for Docs &
// Files: what lies in it by itself, and a draft for its author alone, who
// cannot bring it back here.
func (s *Server) trashedFilesEntries(project *Project, person *Person) []trashRow {
	var rows []trashRow
	for _, tool := range s.FilesTools {
		if tool.ProjectID != project.ID {
			continue
		}
		for _, entry := range entriesOf(tool) {
			if !entry.state.Trashed || entry.draft && entry.creator != person.ID {
				continue
			}
			var blockedBy any
			switch {
			case folderIsInTheTrash(tool, *entry.parent):
				blockedBy = "folder"
			case folderIsOutOfUse(tool, *entry.parent):
				blockedBy = "archivedFolder"
			}
			rows = append(rows, trashRow{entry.state.TrashedAt, entry.state.trashSeq, map[string]any{
				"kind": entry.kind, "id": entry.id, "title": entry.title, "toolType": nil,
				"board": map[string]any{"id": tool.ID, "title": tool.Title}, "column": nil,
				"trashedAt": timestampOrNil(nilIfZero(entry.state.TrashedAt)), "trashedBy": s.personReferenceOrNil(entry.state.TrashedBy),
				"restorable": blockedBy == nil && !entry.draft, "blockedBy": blockedBy,
				"appUrl": s.URL + "/projects/" + project.ID + "/trash",
			}})
		}
	}
	return rows
}
