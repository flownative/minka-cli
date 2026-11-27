package fakeapi

import "net/http"

func (s *Server) registerFilesLockRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/documents/{documentId}/lock", s.authenticated(s.lockDocument))
	mux.HandleFunc("POST /api/v1/documents/{documentId}/unlock", s.authenticated(s.unlockDocument))
}

// lockDocument holds the newest version of a document until somebody of the
// team unlocks it; locking it again is refused.
func (s *Server) lockDocument(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, document, refused := s.documentToLockOrUnlock(w, r, person)
	if refused {
		return
	}
	if document.Lock != nil {
		unprocessable(w, "documentLocked", "This document is locked. Its title and text stay as they are until somebody of the team unlocks it.")
		return
	}
	s.lock(tool, document, person.ID)
	writeJSON(w, http.StatusOK, s.documentResource(tool, document, person))
}

// unlockDocument opens the title and the text to every write again, and the
// blocks list the decisions afresh.
func (s *Server) unlockDocument(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, document, refused := s.documentToLockOrUnlock(w, r, person)
	if refused {
		return
	}
	if document.Lock == nil {
		unprocessable(w, "documentNotLocked", "This document is not locked.")
		return
	}
	document.Lock = nil
	writeJSON(w, http.StatusOK, s.documentResource(tool, document, person))
}

// documentToLockOrUnlock finds the document and refuses in Minka's order: a
// document that is not there, and a client, who meets the same, then an
// archived project, then the document or a folder above it in the archive.
// Whether it is locked is asked only after all of them.
func (s *Server) documentToLockOrUnlock(w http.ResponseWriter, r *http.Request, person *Person) (*FilesTool, *Document, bool) {
	tool, document := s.visibleDocument(r.PathValue("documentId"), person)
	if document == nil || s.isAClient(person.ID) {
		notFound(w)
		return nil, nil, true
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) || s.refusedAsAnArchivedEntry(w, tool, document.FolderID, document.Archived, "document") {
		return nil, nil, true
	}
	return tool, document, false
}

func lockedAt(lock *DocumentLock) any {
	if lock == nil {
		return nil
	}
	return timestamp(lock.At)
}

// lockedBy is null while the document is not locked, and where nothing kept
// who locked it.
func (s *Server) lockedBy(lock *DocumentLock) any {
	if lock == nil {
		return nil
	}
	return s.personReferenceOrNil(lock.ByID)
}

func lockedVersion(lock *DocumentLock) any {
	if lock == nil || lock.VersionID == "" {
		return nil
	}
	return map[string]any{"id": lock.VersionID, "number": lock.VersionNumber}
}
