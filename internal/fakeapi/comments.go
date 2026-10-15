package fakeapi

import (
	"net/http"
	"slices"
)

// commentedItem is what the comments under a post, a document, an upload, a
// to-do, a to-do list and an event have in common: the project they are
// written in, the page they stand on, why a write under the item wrote
// nothing, and the comments themselves.
type commentedItem struct {
	projectID string
	appURL    string
	// refusedAsClosed answers a write the item takes no more, with the code of
	// the reason, and says whether it did.
	refusedAsClosed func(w http.ResponseWriter) bool
	comments        *[]*Comment
}

// registerCommentRoutes serves the comments under the item an address names,
// such as "posts/{postId}".
func (s *Server) registerCommentRoutes(mux *http.ServeMux, item string) {
	under := "/api/v1/" + item + "/comments"
	mux.HandleFunc("GET "+under, s.authenticated(s.commentsOnItem))
	mux.HandleFunc("POST "+under, s.authenticated(s.writeCommentOnItem))
	mux.HandleFunc("PATCH "+under+"/{commentId}", s.authenticated(s.editCommentOnItem))
	mux.HandleFunc("DELETE "+under+"/{commentId}", s.authenticated(s.removeCommentOnItem))
	mux.HandleFunc("POST "+under+"/{commentId}/reactions", s.authenticated(s.reactToCommentOnItem))
	mux.HandleFunc("DELETE "+under+"/{commentId}/reactions", s.authenticated(s.unreactFromCommentOnItem))
}

// itemOfTheRequest finds the item an address names, under the membership of
// the person asking. Which kind it is the address says, and an identifier of
// one kind is nothing at the address of another.
func (s *Server) itemOfTheRequest(r *http.Request, person *Person) *commentedItem {
	switch {
	case r.PathValue("postId") != "":
		tool, post := s.visiblePost(r.PathValue("postId"), person)
		if post == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.postURL(tool, post), comments: &post.Comments,
			refusedAsClosed: func(w http.ResponseWriter) bool { return s.refusedAsAClosedPost(w, tool, post) }}
	case r.PathValue("documentId") != "":
		tool, document := s.visibleDocument(r.PathValue("documentId"), person)
		if document == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.documentURL(tool, document), comments: &document.Comments,
			refusedAsClosed: s.refusedAsAClosedEntry(tool, document.FolderID, document.Archived, "document")}
	case r.PathValue("uploadId") != "":
		tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person)
		if upload == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.filesUploadURL(tool, upload), comments: &upload.Comments,
			refusedAsClosed: s.refusedAsAClosedEntry(tool, upload.FolderID, upload.Archived, "upload")}
	case r.PathValue("todoId") != "":
		tool, list, todo := s.visibleTodo(r.PathValue("todoId"), person)
		if todo == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.todoURL(tool, todo), comments: &todo.Comments,
			refusedAsClosed: func(w http.ResponseWriter) bool { return s.refusedAsNothingWritten(w, tool, list) }}
	case r.PathValue("listId") != "":
		tool, list := s.visibleTodoList(r.PathValue("listId"), person)
		if list == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.todoListURL(tool, list), comments: &list.Comments,
			refusedAsClosed: func(w http.ResponseWriter) bool { return s.refusedAsNothingWritten(w, tool, list) }}
	case r.PathValue("eventId") != "":
		// An event takes comments in a calendar out of use as well; only the
		// archive of its project closes it.
		tool, event := s.visibleEvent(r.PathValue("eventId"), person)
		if event == nil {
			return nil
		}
		return &commentedItem{projectID: tool.ProjectID, appURL: s.eventURL(tool, event), comments: &event.Comments,
			refusedAsClosed: func(w http.ResponseWriter) bool { return s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) }}
	}
	return nil
}

// refusedAsAClosedEntry says why nothing was written under a document or an
// upload: the project archived, or the entry itself or a folder above it.
func (s *Server) refusedAsAClosedEntry(tool *FilesTool, folderID string, archived bool, kind string) func(http.ResponseWriter) bool {
	return func(w http.ResponseWriter) bool {
		return s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) ||
			s.refusedAsAnArchivedEntry(w, tool, folderID, archived, kind)
	}
}

func (s *Server) commentsOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	item := s.itemOfTheRequest(r, person)
	if item == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.commentResources(item.projectID, item.appURL, *item.comments))
}

func (s *Server) writeCommentOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	item := s.itemOfTheRequest(r, person)
	if item == nil {
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
	if item.refusedAsClosed(w) {
		return
	}

	comment := &Comment{ID: newID(), AuthorID: person.ID, Body: text, CreatedAt: s.now}
	*item.comments = append(*item.comments, comment)
	writeJSON(w, http.StatusCreated, s.commentResource(item.projectID, item.appURL, comment))
}

// editCommentOnItem changes a comment of the person asking. A comment that is
// not under the item is 404 before anything the body says. Minka's statement
// changes a comment of the writer under an item that takes changes, and asks
// why only where it changed nothing, so somebody else's comment under an
// archived item is answered as the archive.
func (s *Server) editCommentOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	item := s.itemOfTheRequest(r, person)
	if commentIn(item, r.PathValue("commentId")) == nil {
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
	if item.refusedAsClosed(w) {
		return
	}
	comment := ownCommentOf(*item.comments, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}

	// The same text again is a write as well: the moment moves on.
	now := s.now
	comment.Body, comment.EditedAt = text, &now
	writeJSON(w, http.StatusOK, s.commentResource(item.projectID, item.appURL, comment))
}

func (s *Server) removeCommentOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	item := s.itemOfTheRequest(r, person)
	if commentIn(item, r.PathValue("commentId")) == nil {
		notFound(w)
		return
	}
	if item.refusedAsClosed(w) {
		return
	}
	comment := ownCommentOf(*item.comments, r.PathValue("commentId"), person)
	if comment == nil {
		notFound(w)
		return
	}

	*item.comments = slices.DeleteFunc(*item.comments, func(c *Comment) bool { return c == comment })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// reactToCommentOnItem reads the body before it looks for the item, and looks
// for the comment before it reads the reaction, as the controller does.
func (s *Server) reactToCommentOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	item := s.itemOfTheRequest(r, person)
	comment := commentIn(item, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	value, ok := stringField(w, fields, "content")
	if !ok {
		return
	}
	content, ok := validReaction(w, deref(value))
	if !ok {
		return
	}
	if item.refusedAsClosed(w) {
		return
	}

	status := reactTo(&comment.Reactions, content, person.ID)
	writeJSON(w, status, s.commentResource(item.projectID, item.appURL, comment))
}

func (s *Server) unreactFromCommentOnItem(w http.ResponseWriter, r *http.Request, person *Person) {
	item := s.itemOfTheRequest(r, person)
	comment := commentIn(item, r.PathValue("commentId"))
	if comment == nil {
		notFound(w)
		return
	}
	content, ok := validReaction(w, r.URL.Query().Get("content"))
	if !ok {
		return
	}
	if item.refusedAsClosed(w) {
		return
	}

	unreactFrom(&comment.Reactions, content, person.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func commentIn(item *commentedItem, commentID string) *Comment {
	if item == nil {
		return nil
	}
	for _, comment := range *item.comments {
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

func (s *Server) commentResources(projectID, itemURL string, comments []*Comment) []any {
	resources := []any{}
	for _, comment := range comments {
		resources = append(resources, s.commentResource(projectID, itemURL, comment))
	}
	return resources
}
