package fakeapi

import (
	"net/http"
	"slices"
)

// transferFilesEntry moves an entry into another Docs & Files tool, also of
// another project, for the team of both projects. It lands on top of its kind
// there, at the end of a place arranged by hand, and a folder takes everything
// below it along, whatever its state. What arrives is hidden from clients, and
// labels that belong to neither the project nor the tool there come off. The
// checks follow the order Minka asks them in.
func (s *Server) transferFilesEntry(w http.ResponseWriter, entry filesEntry, fields map[string]any, toolID string, person *Person) {
	if s.isAClient(person.ID) {
		notFound(w)
		return
	}
	target := s.toolToTransferInto(toolID, person)
	if target == nil {
		unprocessable(w, "toolUnknown", "There is no such Docs & Files tool in a project you are on the team of.")
		return
	}
	_, previousGiven := fields["previousEntryId"]
	_, nextGiven := fields["nextEntryId"]
	if previousGiven || nextGiven {
		unprocessable(w, "neighboursAcrossTools", `An entry moving into another tool goes on top of its kind there; "previousEntryId" and "nextEntryId" only place it within its own tool.`)
		return
	}
	if _, given := fields["folderId"]; !given {
		unprocessable(w, "folderIdMissing", `A move names the folder to move to in "folderId", null for the top of the tool.`)
		return
	}
	folderID, ok := stringField(w, fields, "folderId")
	if !ok {
		return
	}
	folder, ok := s.folderToWriteIn(w, target, deref(folderID))
	if !ok {
		return
	}
	source := entry.tool
	if s.refusedAsArchived(w, s.projectWithID(source.ProjectID)) || s.refusedAsArchived(w, s.projectWithID(target.ProjectID)) {
		return
	}
	to := folderIDOf(folder)
	if entry.kind == "folder" && (entry.folder.IsBook || holdsABook(source, entry.id)) && bookOf(target, to) != nil {
		refuseABookWithinABook(w)
		return
	}
	moved := movedWith(entry)
	if s.sharesFilesWithWhatStays(moved, target.ProjectID) {
		unprocessable(w, "entrySharesFiles", "A file in this entry is shared with something that stays behind, so it moves only within its project.")
		return
	}

	takeOutOfPlace(source, *entry.parent, entry.id)
	*entry.parent = to
	s.carryInto(target, moved, entry)
	placeAtTheEnd(target, to, entry.id)
	if entry.kind == "folder" && bookOf(target, to) != nil {
		arrangeByHand(target, entry.id)
	}
	writeJSON(w, http.StatusOK, s.entryResource(s.entryIn(target, entry.kind, entry.id)))
}

// toolToTransferInto is the Docs & Files tool an entry may move into: one of a
// project this person is on the team of, outside the template library.
func (s *Server) toolToTransferInto(toolID string, person *Person) *FilesTool {
	if !uuidPattern.MatchString(toolID) {
		return nil
	}
	tool := s.visibleFilesTool(toolID, person)
	if tool == nil || s.projectWithID(tool.ProjectID).TemplateLibrary {
		return nil
	}
	return tool
}

// transferredSet is what goes along with an entry that moves into another
// tool.
type transferredSet struct {
	folders   []*Folder
	documents []*Document
	uploads   []*FilesUpload
}

func (set transferredSet) ids() []string {
	var ids []string
	for _, folder := range set.folders {
		ids = append(ids, folder.ID)
	}
	for _, document := range set.documents {
		ids = append(ids, document.ID)
	}
	for _, upload := range set.uploads {
		ids = append(ids, upload.ID)
	}
	return ids
}

// movedWith is the entry and, for a folder, everything below it, the trash
// and the archive included.
func movedWith(entry filesEntry) transferredSet {
	switch entry.kind {
	case "document":
		return transferredSet{documents: []*Document{entry.document}}
	case "upload":
		return transferredSet{uploads: []*FilesUpload{entry.upload}}
	}
	subtree := subtreeOf(entry.tool, entry.id)
	var set transferredSet
	for _, folder := range entry.tool.Folders {
		if slices.Contains(subtree, folder.ID) {
			set.folders = append(set.folders, folder)
		}
	}
	for _, document := range entry.tool.Documents {
		if slices.Contains(subtree, document.FolderID) {
			set.documents = append(set.documents, document)
		}
	}
	for _, upload := range entry.tool.Uploads {
		if slices.Contains(subtree, upload.FolderID) {
			set.uploads = append(set.uploads, upload)
		}
	}
	return set
}

// sharesFilesWithWhatStays says whether a file of the uploads that move, or of
// an earlier version of one, is also held by an upload or the back of a card
// that stays, where the file does not belong to the target project already.
func (s *Server) sharesFilesWithWhatStays(set transferredSet, targetProjectID string) bool {
	for _, moving := range set.uploads {
		for _, blobID := range append([]string{moving.BlobID}, versionFiles(moving)...) {
			if file := s.storedFile(blobID); file == nil || file.Circle == targetProjectID {
				continue
			}
			if s.heldByACardBack(blobID) {
				return true
			}
			for _, tool := range s.FilesTools {
				for _, other := range tool.Uploads {
					if !slices.Contains(set.uploads, other) && (other.BlobID == blobID || other.showedEarlier(blobID)) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (s *Server) storedFile(blobID string) *Upload {
	for _, file := range s.Uploads {
		if file.ID == blobID {
			return file
		}
	}
	return nil
}

// carryInto takes the set out of its tool and puts it into the target: the
// entry on top of its kind, what stands below a folder in the order it had.
// The places a folder of the set arranged by hand stay arranged so.
func (s *Server) carryInto(target *FilesTool, set transferredSet, entry filesEntry) {
	source := entry.tool
	source.Folders = slices.DeleteFunc(source.Folders, func(folder *Folder) bool { return slices.Contains(set.folders, folder) })
	source.Documents = slices.DeleteFunc(source.Documents, func(document *Document) bool { return slices.Contains(set.documents, document) })
	source.Uploads = slices.DeleteFunc(source.Uploads, func(upload *FilesUpload) bool { return slices.Contains(set.uploads, upload) })
	target.Folders = append(target.Folders, set.folders...)
	target.Documents = append(target.Documents, set.documents...)
	target.Uploads = append(target.Uploads, set.uploads...)
	s.entryIn(target, entry.kind, entry.id).putOnTop()

	for _, folder := range set.folders {
		folder.ShownToClients = false
		if order, arranged := source.Arranged[folder.ID]; arranged {
			if target.Arranged == nil {
				target.Arranged = map[string][]string{}
			}
			target.Arranged[folder.ID] = order
			delete(source.Arranged, folder.ID)
		}
	}
	for _, document := range set.documents {
		document.HiddenFromClients = true
	}
	for _, upload := range set.uploads {
		upload.HiddenFromClients = true
		if target.ProjectID != source.ProjectID {
			for _, blobID := range append([]string{upload.BlobID}, versionFiles(upload)...) {
				if file := s.storedFile(blobID); file != nil {
					file.Circle = target.ProjectID
				}
			}
		}
	}
	for _, id := range set.ids() {
		s.ItemLabels[id] = slices.DeleteFunc(s.ItemLabels[id], func(labelID string) bool {
			label := s.labelWithID(labelID)
			return label == nil || label.ProjectID != target.ProjectID || label.ToolID != "" && label.ToolID != target.ID
		})
	}
}

func versionFiles(upload *FilesUpload) []string {
	var files []string
	for _, version := range upload.Versions {
		files = append(files, version.BlobID)
	}
	return files
}

// entryIn finds an entry of the tool by its kind and ID, in every state.
func (s *Server) entryIn(tool *FilesTool, kind, id string) filesEntry {
	for _, entry := range entriesOf(tool) {
		if entry.kind == kind && entry.id == id {
			return entry
		}
	}
	return filesEntry{}
}

func (s *Server) labelWithID(id string) *Label {
	for _, label := range s.Labels {
		if label.ID == id {
			return label
		}
	}
	return nil
}
