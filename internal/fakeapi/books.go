package fakeapi

import (
	"net/http"
	"slices"
	"time"
)

// Books and the order set by hand: a folder marked as a book is read as one
// tree, and a place of a tool, the top or a folder, orders its entries either
// automatically — folders before documents before uploads, each newest first
// — or by hand, all three mixed.

func (s *Server) registerBookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/files-folders/{folderId}", s.authenticated(s.filesFolder))
	mux.HandleFunc("GET /api/v1/files-folders/{folderId}/book", s.authenticated(s.book))
	mux.HandleFunc("GET /api/v1/files-folders/{folderId}/tree", s.authenticated(s.filesTree))
	mux.HandleFunc("PUT /api/v1/files-tools/{toolId}/order", s.authenticated(s.changeFilesOrder))
}

// filesFolder reads a folder by its identifier alone, in the shape of the
// entries of its tool. A folder in the archive is read as well, one in the
// trash or below one is not.
func (s *Server) filesFolder(w http.ResponseWriter, r *http.Request, person *Person) {
	if s.KnowsNoTree {
		unknownAddress(w)
		return
	}
	entry, found := s.visibleEntry("folder", r.PathValue("folderId"), person)
	if !found {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.entriesResource(entry.tool, entry.folder))
}

// book reads the book a folder belongs to, itself or one above it, with its
// chapters and pages in use. A book in the archive is read as well.
func (s *Server) book(w http.ResponseWriter, r *http.Request, person *Person) {
	entry, found := s.visibleEntry("folder", r.PathValue("folderId"), person)
	if !found {
		notFound(w)
		return
	}
	book := bookOf(entry.tool, entry.id)
	if book == nil {
		notFound(w)
		return
	}
	s.writeTree(w, entry.tool, book, person, false)
}

// filesTree reads any folder as a book is read, starting at the folder, with
// the uploads in use beside its chapters and pages. A folder in the archive is
// read as well. Whoever sees the folder sees everything in it: what a client
// is shown is decided by the topmost folder.
func (s *Server) filesTree(w http.ResponseWriter, r *http.Request, person *Person) {
	if s.KnowsNoTree {
		unknownAddress(w)
		return
	}
	entry, found := s.visibleEntry("folder", r.PathValue("folderId"), person)
	if !found {
		notFound(w)
		return
	}
	s.writeTree(w, entry.tool, entry.folder, person, true)
}

func (s *Server) writeTree(w http.ResponseWriter, tool *FilesTool, root *Folder, person *Person, withUploads bool) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": root.ID, "title": root.Title, "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"order": orderOf(tool, root.ID), "entries": append(s.bookEntriesOf(tool, root.ID, map[string]bool{root.ID: true}, person, withUploads), s.LaterBookEntries...),
		"appUrl": s.filesToolURL(tool) + "/folders/" + root.ID,
	})
}

// bookEntriesOf are the chapters, pages and, where asked for, uploads of one
// place of a tree in its order: folders in use, published documents in use
// and uploads in use, nothing else. A page names its newest version to the
// team alone.
func (s *Server) bookEntriesOf(tool *FilesTool, chapterID string, seen map[string]bool, person *Person, withUploads bool) []any {
	var shown []filesEntry
	for _, folder := range tool.Folders {
		if folder.ParentID == chapterID && !folder.Archived && !folder.Trashed && !seen[folder.ID] {
			shown = append(shown, filesEntry{kind: "folder", id: folder.ID, folder: folder})
		}
	}
	for _, document := range tool.Documents {
		if document.FolderID == chapterID && !document.Draft && !document.Archived && !document.Trashed {
			shown = append(shown, filesEntry{kind: "document", id: document.ID, document: document})
		}
	}
	for _, upload := range tool.Uploads {
		if withUploads && upload.FolderID == chapterID && !upload.Archived && !upload.Trashed {
			shown = append(shown, filesEntry{kind: "upload", id: upload.ID, upload: upload})
		}
	}

	entries := []any{}
	for _, entry := range inTheOrderShown(tool, chapterID, shown) {
		switch entry.kind {
		case "document":
			var version any
			if !s.isAClient(person.ID) {
				version = max(1, len(entry.document.Versions))
			}
			page := map[string]any{
				"id": entry.id, "title": entry.document.Title, "inNavigation": !entry.document.OutOfNavigation,
				"updatedAt": timestamp(entry.document.UpdatedAt), "version": version,
				"appUrl": s.documentURL(tool, entry.document), "kind": "page",
			}
			if s.KnowsNoTree {
				delete(page, "updatedAt")
				delete(page, "version")
			}
			entries = append(entries, page)
		case "upload":
			entries = append(entries, map[string]any{
				"id": entry.id, "title": entry.upload.Title, "file": s.attachmentResource(entry.upload.BlobID),
				"downloadUrl": s.URL + "/api/v1/uploads/" + entry.upload.BlobID + "/original",
				"updatedAt":   timestamp(entry.upload.UpdatedAt),
				"appUrl":      s.filesUploadURL(tool, entry.upload), "kind": "upload",
			})
		default:
			seen[entry.id] = true
			entries = append(entries, map[string]any{
				"id": entry.id, "title": entry.folder.Title, "order": orderOf(tool, entry.id),
				"entries": s.bookEntriesOf(tool, entry.id, seen, person, withUploads),
				"appUrl":  s.filesToolURL(tool) + "/folders/" + entry.id, "kind": "chapter",
			})
		}
	}
	return entries
}

// Edit writes over a document's body as a save does: a moment later, and a
// version more.
func (d *Document) Edit(body string) {
	d.save(d.Title, body)
}

// Rename gives a document another title as a save does.
func (d *Document) Rename(title string) {
	d.save(title, d.Body)
}

func (d *Document) save(title, body string) {
	d.versions()
	d.Title, d.Body = title, body
	d.UpdatedAt = d.UpdatedAt.Add(time.Second)
	d.keepAVersion("", d.UpdatedAt)
}

// changeFilesOrder sets how the top of a tool, or a folder of it, orders its
// entries. Turning to the order by hand keeps the order shown a moment
// before; a place that orders so already is answered the same. Only the team
// chooses, and the statement learns it last.
func (s *Server) changeFilesOrder(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visibleFilesTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	order, ok := stringField(w, fields, "order")
	if !ok {
		return
	}
	if deref(order) != "automatic" && deref(order) != "manual" {
		unprocessable(w, "orderUnknown", `The order is either "automatic" or "manual".`)
		return
	}
	folderID, ok := stringField(w, fields, "folderId")
	if !ok {
		return
	}
	folder, ok := s.folderToWriteIn(w, tool, deref(folderID))
	if !ok || s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.isAClient(person.ID) {
		notFound(w)
		return
	}

	place := folderIDOf(folder)
	if *order == "manual" {
		arrangePlace(tool, place)
	} else {
		delete(tool.Arranged, place)
	}
	writeJSON(w, http.StatusOK, s.entriesResource(tool, folder))
}

// namedNeighbours reads the neighbours a move names, nil where it names
// neither. Each has to be an identifier or empty, and the place the entry
// goes to has to be arranged by hand.
func (s *Server) namedNeighbours(w http.ResponseWriter, fields map[string]any, tool *FilesTool, project *Project, place string) ([]string, bool) {
	_, previousGiven := fields["previousEntryId"]
	_, nextGiven := fields["nextEntryId"]
	if !previousGiven && !nextGiven {
		return nil, true
	}
	neighbours := make([]string, 2)
	for i, name := range []string{"previousEntryId", "nextEntryId"} {
		value, ok := stringField(w, fields, name)
		if !ok {
			return nil, false
		}
		if deref(value) != "" && !uuidPattern.MatchString(*value) {
			unprocessable(w, "entryPlaceUnknown", "A neighbour of the move is no identifier of an entry.")
			return nil, false
		}
		neighbours[i] = deref(value)
	}
	if s.refusedAsArchived(w, project) {
		return nil, false
	}
	if !arrangedByHand(tool, place) {
		unprocessable(w, "folderOrderAutomatic", "This place orders its entries automatically; arrange it by hand first to place an entry in it.")
		return nil, false
	}
	return neighbours, true
}

// bookField reads isBook or inNavigation, which a Minka without books does
// not read at all.
func (s *Server) bookField(w http.ResponseWriter, fields map[string]any, name string) (*bool, bool) {
	if s.KnowsNoBooks {
		return nil, true
	}
	return strictBooleanField(w, fields, name)
}

// withoutBooks takes the fields of books and of the order set by hand out of
// an answer of a Minka that has none.
func (s *Server) withoutBooks(resource map[string]any, names ...string) map[string]any {
	if s.KnowsNoBooks {
		for _, name := range names {
			delete(resource, name)
		}
	}
	return resource
}

// strictBooleanField reads a field that is true or false where it is given.
// A value of another type is a body Minka cannot read, as for every field;
// an explicit null reaches the controller and is refused with the field's
// own code.
func strictBooleanField(w http.ResponseWriter, fields map[string]any, name string) (*bool, bool) {
	value, present := fields[name]
	if !present {
		return nil, true
	}
	if value == nil {
		unprocessable(w, name+"NotABoolean", `"`+name+`" is true or false.`)
		return nil, false
	}
	return booleanField(w, fields, name)
}

func refuseABookWithinABook(w http.ResponseWriter) {
	unprocessable(w, "bookWithinABook", "A book never stands inside another book: a folder inside a book, or one holding a book, cannot become one, and a book cannot move into a book.")
}

// wouldNestABook says whether marking the folder would put a book inside
// another: it stands below a book, or holds one at any depth.
func wouldNestABook(tool *FilesTool, folder *Folder) bool {
	book := bookOf(tool, folder.ID)
	return book != nil && book != folder || holdsABook(tool, folder.ID)
}

// bookOf is the book a place belongs to: the folder itself or the nearest one
// above it marked as a book, nil outside a book and at the top of the tool.
func bookOf(tool *FilesTool, folderID string) *Folder {
	for seen := 0; folderID != "" && seen <= len(tool.Folders); seen++ {
		folder := folderIn(tool, folderID)
		if folder == nil {
			return nil
		}
		if folder.IsBook {
			return folder
		}
		folderID = folder.ParentID
	}
	return nil
}

// holdsABook says whether a folder below this one, at any depth and in any
// state, the trash included, is marked as a book.
func holdsABook(tool *FilesTool, folderID string) bool {
	for _, below := range subtreeOf(tool, folderID)[1:] {
		if folderIn(tool, below).IsBook {
			return true
		}
	}
	return false
}

// subtreeOf is the folder and every folder below it, in any state.
func subtreeOf(tool *FilesTool, folderID string) []string {
	subtree := []string{folderID}
	for i := 0; i < len(subtree); i++ {
		for _, folder := range tool.Folders {
			if folder.ParentID == subtree[i] && !slices.Contains(subtree, folder.ID) {
				subtree = append(subtree, folder.ID)
			}
		}
	}
	return subtree
}

func orderOf(tool *FilesTool, place string) string {
	if arrangedByHand(tool, place) {
		return "manual"
	}
	return "automatic"
}

func arrangedByHand(tool *FilesTool, place string) bool {
	_, arranged := tool.Arranged[place]
	return arranged
}

// arrangeByHand orders the folder and every folder below it by hand, each
// place that is not so yet in the order it shows.
func arrangeByHand(tool *FilesTool, folderID string) {
	for _, place := range subtreeOf(tool, folderID) {
		arrangePlace(tool, place)
	}
}

// arrangePlace orders one place by hand in the order it shows automatically,
// unless it is ordered by hand already.
func arrangePlace(tool *FilesTool, place string) {
	if arrangedByHand(tool, place) {
		return
	}
	if tool.Arranged == nil {
		tool.Arranged = map[string][]string{}
	}
	ids := []string{}
	for _, entry := range automaticOrderOf(tool, place) {
		ids = append(ids, entry.id)
	}
	tool.Arranged[place] = ids
}

// automaticOrderOf is every entry of a place, in whatever state, as the
// automatic order puts them: folders, documents, uploads, each newest first.
func automaticOrderOf(tool *FilesTool, place string) []filesEntry {
	var entries []filesEntry
	for _, entry := range entriesOf(tool) {
		if *entry.parent == place {
			entries = append(entries, entry)
		}
	}
	return entries
}

// inTheOrderShown puts the entries of a place, given in the automatic order,
// into the order set by hand where there is one; what it does not name stays
// behind the rest in the order given.
func inTheOrderShown(tool *FilesTool, place string, entries []filesEntry) []filesEntry {
	if !arrangedByHand(tool, place) {
		return entries
	}
	order := tool.Arranged[place]
	rank := func(entry filesEntry) int {
		if i := slices.Index(order, entry.id); i >= 0 {
			return i
		}
		return len(order)
	}
	sorted := slices.Clone(entries)
	slices.SortStableFunc(sorted, func(a, b filesEntry) int { return rank(a) - rank(b) })
	return sorted
}

// completePlace gives every entry of a place arranged by hand a place in its
// order, those without one behind the rest in the automatic order, as Minka
// does before it places anything there.
func completePlace(tool *FilesTool, place string) {
	for _, entry := range automaticOrderOf(tool, place) {
		if !slices.Contains(tool.Arranged[place], entry.id) {
			tool.Arranged[place] = append(tool.Arranged[place], entry.id)
		}
	}
}

// placeAtTheEnd puts an entry that came into a place last in its order, where
// the place is arranged by hand.
func placeAtTheEnd(tool *FilesTool, place, id string) {
	if !arrangedByHand(tool, place) {
		return
	}
	takeOutOfPlace(tool, place, id)
	completePlace(tool, place)
	tool.Arranged[place] = append(slices.DeleteFunc(tool.Arranged[place], func(other string) bool { return other == id }), id)
}

// takeOutOfPlace drops an entry that left a place from its order.
func takeOutOfPlace(tool *FilesTool, place, id string) {
	if !arrangedByHand(tool, place) {
		return
	}
	tool.Arranged[place] = slices.DeleteFunc(tool.Arranged[place], func(other string) bool { return other == id })
}

// placeBetween puts an entry right behind the previous neighbour, or where
// that does not stand in the place, right before the next one, and where
// neither does, first.
func placeBetween(tool *FilesTool, place, id, previous, next string) {
	completePlace(tool, place)
	order := slices.DeleteFunc(tool.Arranged[place], func(other string) bool { return other == id })
	at := 0
	if i := slices.Index(order, previous); previous != "" && i >= 0 {
		at = i + 1
	} else if i := slices.Index(order, next); next != "" && i >= 0 {
		at = i
	}
	tool.Arranged[place] = slices.Insert(order, at, id)
}
