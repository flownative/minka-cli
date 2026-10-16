package fakeapi

import (
	"net/http"
	"testing"
	"time"
)

// entryIDs are the identifiers of a place's "entries", in the order shown.
func entryIDs(t *testing.T, answer map[string]any) []string {
	t.Helper()
	var ids []string
	for _, entry := range answer["entries"].([]any) {
		ids = append(ids, entry.(map[string]any)["id"].(string))
	}
	return ids
}

func sameIDs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Marking a folder makes it a book: the folder and every folder in it are
// arranged by hand in the order they showed, every place in it names the
// book, and the book is read as one tree from any of its folders.
func TestAMarkedFolderIsReadAsABook(t *testing.T) {
	s, f := New(t)
	folder := "/api/v1/files-folders/" + f.Folder.ID

	got, marked := answerTo(t, s, f.Token, http.MethodPatch, folder, `{"isBook":true}`)
	if got != "200" || marked["isBook"] != true || marked["order"] != "manual" || marked["book"].(map[string]any)["id"] != f.Folder.ID {
		t.Fatalf("%s %v", got, marked)
	}
	if !sameIDs(entryIDs(t, marked), f.DocumentInFolder.ID, f.FilesUpload.ID) {
		t.Fatalf("entries %v", marked["entries"])
	}
	if !arrangedByHand(f.Files, f.ArchivedFolder.ID) || arrangedByHand(f.Files, "") {
		t.Fatal("marking arranges the book and every folder in it by hand, and nothing above it")
	}

	_, top := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-tools/"+f.Files.ID+"/entries", "")
	if top["order"] != "automatic" || top["isBook"] != false || top["book"] != nil {
		t.Fatalf("top %v", top)
	}
	if listed := top["folders"].([]any)[0].(map[string]any); listed["id"] != f.Folder.ID || listed["isBook"] != true {
		t.Fatalf("folders %v", top["folders"])
	}
	_, document := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/documents/"+f.DocumentInFolder.ID, "")
	if document["book"].(map[string]any)["title"] != "Plans" || document["inNavigation"] != true {
		t.Fatalf("document %v", document)
	}
	_, outside := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/documents/"+f.Document.ID, "")
	if outside["book"] != nil {
		t.Fatalf("a document in no book names %v", outside["book"])
	}

	// A chapter in the archive is left out; asked through it, the book is
	// read all the same, and so is a book in the archive.
	f.ArchivedFolder.Archived = false
	chapter := &Folder{ID: newID(), Title: "Put away", ParentID: f.Folder.ID, Archived: true, CreatorID: f.Bruno.ID}
	f.Files.Folders = append(f.Files.Folders, chapter)
	f.DocumentInFolder.OutOfNavigation = true
	got, book := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+chapter.ID+"/book", "")
	if got != "200" || book["id"] != f.Folder.ID || book["order"] != "manual" || book["tool"].(map[string]any)["id"] != f.Files.ID {
		t.Fatalf("%s %v", got, book)
	}
	entries := book["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries %v", entries)
	}
	lastYear, page := entries[0].(map[string]any), entries[1].(map[string]any)
	if lastYear["kind"] != "chapter" || lastYear["id"] != f.ArchivedFolder.ID || lastYear["order"] != "manual" || len(lastYear["entries"].([]any)) != 1 {
		t.Fatalf("chapter %v", lastYear)
	}
	if page["kind"] != "page" || page["id"] != f.DocumentInFolder.ID || page["inNavigation"] != false {
		t.Fatalf("page %v", page)
	}
	f.Folder.Archived = true
	if got, _ := answerTo(t, s, f.Token, http.MethodGet, folder+"/book", ""); got != "200" {
		t.Fatalf("a book in the archive: %s", got)
	}
}

// A book is read by whoever sees its folder; outside a book there is none.
func TestABookIsNotFoundOutsideABook(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	f.Folder.IsBook = true
	f.FolderShownToClients.IsBook = true

	orderSteps(t, s, []orderStep{
		{"an ID that is none", f.Token, http.MethodGet, "/api/v1/files-folders/" + f.FolderShownToClients.ID + "x/book", "", "404"},
		{"a book for a client shown it", "minka_pat_client", http.MethodGet, "/api/v1/files-folders/" + f.FolderShownToClients.ID + "/book", "", "200"},
		{"a book for a client not shown it", "minka_pat_client", http.MethodGet, "/api/v1/files-folders/" + f.Folder.ID + "/book", "", "404"},
		{"a book for an outsider", "minka_pat_stranger", http.MethodGet, "/api/v1/files-folders/" + f.Folder.ID + "/book", "", "404"},
	})
	f.Folder.IsBook = false
	orderSteps(t, s, []orderStep{
		{"a folder that is no book", f.Token, http.MethodGet, "/api/v1/files-folders/" + f.Folder.ID + "/book", "", "404"},
		{"a folder below none", f.Token, http.MethodGet, "/api/v1/files-folders/" + f.ArchivedFolder.ID + "/book", "", "404"},
	})
	f.Folder.IsBook = true
	f.Folder.Trashed = true
	orderSteps(t, s, []orderStep{
		{"a book in the trash", f.Token, http.MethodGet, "/api/v1/files-folders/" + f.ArchivedFolder.ID + "/book", "", "404"},
	})
}

// No book stands inside another: not by a mark, not by a move, and a book in
// the trash counts.
func TestABookNeverStandsInsideAnother(t *testing.T) {
	s, f := New(t)
	plans := "/api/v1/files-folders/" + f.Folder.ID
	got, made := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/folders", `{"title":"Chapter","folderId":"`+f.Folder.ID+`"}`)
	if got != "201" || made["order"] != "automatic" {
		t.Fatalf("%s %v", got, made)
	}
	chapter := "/api/v1/files-folders/" + made["folder"].(map[string]any)["id"].(string)

	orderSteps(t, s, []orderStep{
		{"the chapter a book", f.Token, http.MethodPatch, chapter, `{"isBook":true}`, "200"},
		{"the folder around it", f.Token, http.MethodPatch, plans, `{"isBook":true}`, "422 bookWithinABook"},
		{"the chapter, a book already", f.Token, http.MethodPatch, chapter, `{"isBook":true}`, "200"},
		{"the chapter into the trash", f.Token, http.MethodDelete, chapter, "", "204"},
		{"the folder around a book in the trash", f.Token, http.MethodPatch, plans, `{"isBook":true}`, "422 bookWithinABook"},
		{"the chapter back", f.Token, http.MethodPost, chapter + "/restore", "", "200"},
		{"the chapter no book", f.Token, http.MethodPatch, chapter, `{"isBook":false}`, "200"},
		{"the chapter no book again", f.Token, http.MethodPatch, chapter, `{"isBook":false}`, "200"},
		{"the folder around it a book", f.Token, http.MethodPatch, plans, `{"isBook":true}`, "200"},
		{"the chapter inside it", f.Token, http.MethodPatch, chapter, `{"isBook":true}`, "422 bookWithinABook"},
		{"the chapter inside it no book", f.Token, http.MethodPatch, chapter, `{"isBook":false}`, "200"},
		{"another book", f.Token, http.MethodPatch, "/api/v1/files-folders/" + f.FolderShownToClients.ID, `{"isBook":true}`, "200"},
		{"a book into a book", f.Token, http.MethodPost, "/api/v1/files-folders/" + f.FolderShownToClients.ID + "/moves", `{"folderId":"` + f.Folder.ID + `"}`, "422 bookWithinABook"},
		{"another book no book", f.Token, http.MethodPatch, "/api/v1/files-folders/" + f.FolderShownToClients.ID, `{"isBook":false}`, "200"},
	})
	if !arrangedByHand(f.Files, made["folder"].(map[string]any)["id"].(string)) {
		t.Fatal("marking the book arranges the folders in it by hand")
	}
	// A folder that moves into a book is arranged by hand, with what is in it.
	got, moved := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-folders/"+f.FolderShownToClients.ID+"/moves", `{"folderId":"`+f.Folder.ID+`"}`)
	if got != "200" || moved["order"] != "manual" || moved["book"].(map[string]any)["id"] != f.Folder.ID {
		t.Fatalf("%s %v", got, moved)
	}
	// Taking the mark away leaves every order as it is.
	_, unmarked := answerTo(t, s, f.Token, http.MethodPatch, plans, `{"isBook":false}`)
	if unmarked["isBook"] != false || unmarked["order"] != "manual" || unmarked["book"] != nil {
		t.Fatalf("unmarked %v", unmarked)
	}
}

// The mark is refused before anything is written, in Minka's order: what the
// body says, a client, an archived project, an archived folder, a book within
// a book; the title only after.
func TestTheMarkIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	plans := "/api/v1/files-folders/" + f.Folder.ID
	shown := "/api/v1/files-folders/" + f.FolderShownToClients.ID
	f.ArchivedFolder.IsBook = true

	orderSteps(t, s, []orderStep{
		{"a mark that is no boolean", f.Token, http.MethodPatch, plans, `{"isBook":"yes"}`, "400 malformedBody"},
		{"a mark of null", f.Token, http.MethodPatch, plans, `{"isBook":null}`, "422 isBookNotABoolean"},
		{"a title that is no string", f.Token, http.MethodPatch, plans, `{"title":1,"isBook":"yes"}`, "400 malformedBody"},
		{"by a client", "minka_pat_client", http.MethodPatch, shown, `{"isBook":true}`, "404"},
		{"by a client with a title", "minka_pat_client", http.MethodPatch, shown, `{"title":"New","isBook":false}`, "404"},
		{"an archived folder", f.Token, http.MethodPatch, "/api/v1/files-folders/" + f.ArchivedFolder.ID, `{"isBook":false}`, "422 folderArchived"},
		{"around a book, with an empty title", f.Token, http.MethodPatch, plans, `{"title":"","isBook":true}`, "422 bookWithinABook"},
		{"an empty title, unmarked", f.Token, http.MethodPatch, plans, `{"title":"","isBook":false}`, "422 folderTitleMissing"},
		{"a client renames", "minka_pat_client", http.MethodPatch, shown, `{"title":"Renamed"}`, "200"},
	})
	if f.Folder.Title != "Plans" || f.FolderShownToClients.Title != "Renamed" {
		t.Fatalf("titles %q, %q", f.Folder.Title, f.FolderShownToClients.Title)
	}

	f.Project.Archived = true
	orderSteps(t, s, []orderStep{
		{"around a book in an archived project", f.Token, http.MethodPatch, plans, `{"isBook":true}`, "422 projectArchived"},
		{"an archived folder in an archived project", f.Token, http.MethodPatch, "/api/v1/files-folders/" + f.ArchivedFolder.ID, `{"isBook":false}`, "422 projectArchived"},
		{"no boolean in an archived project", f.Token, http.MethodPatch, plans, `{"isBook":1}`, "400 malformedBody"},
		{"null in an archived project", f.Token, http.MethodPatch, plans, `{"isBook":null}`, "422 isBookNotABoolean"},
	})
}

// A document is left out of the navigation of its book and put back. The
// navigation is refused before anything is written, is not written with a
// title or body refused, and moves no moment of the document.
func TestADocumentLeavesTheNavigationAndComesBack(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	document := "/api/v1/documents/" + f.DocumentInFolder.ID
	updatedAt := timestamp(f.DocumentInFolder.UpdatedAt)
	versions := len(f.DocumentInFolder.versions())
	s.now = s.now.Add(time.Hour)

	// The navigation alone is no write of the text: no 409 whatever the
	// expectation, no version, and the moment stays.
	got, answer := answerTo(t, s, f.Token, http.MethodPatch, document, `{"inNavigation":false,"expectedUpdatedAt":"2020-01-01T00:00:00.000000Z"}`)
	if got != "200" || answer["inNavigation"] != false || answer["updatedAt"] != updatedAt || !f.DocumentInFolder.OutOfNavigation {
		t.Fatalf("%s %v", got, answer)
	}
	if len(f.DocumentInFolder.Versions) != versions {
		t.Fatalf("%d versions, want %d", len(f.DocumentInFolder.Versions), versions)
	}
	orderSteps(t, s, []orderStep{
		{"no boolean", f.Token, http.MethodPatch, document, `{"inNavigation":"no"}`, "400 malformedBody"},
		{"null", f.Token, http.MethodPatch, document, `{"inNavigation":null}`, "422 inNavigationNotABoolean"},
		{"after a refused title", f.Token, http.MethodPatch, document, `{"title":"","inNavigation":"no"}`, "422 documentTitleMissing"},
		{"by a client", "minka_pat_client", http.MethodPatch, "/api/v1/documents/" + f.Document.ID, `{"inNavigation":true}`, "404"},
		{"an archived document", f.Token, http.MethodPatch, "/api/v1/documents/" + f.ArchivedDocument.ID, `{"inNavigation":true}`, "422 documentArchived"},
		{"below an archived folder", f.Token, http.MethodPatch, "/api/v1/documents/" + f.DocumentBelowTheArchivedFolder.ID, `{"inNavigation":true}`, "422 documentArchived"},
		{"a stale guard over a new title", f.Token, http.MethodPatch, document, `{"title":"New","inNavigation":true,"expectedUpdatedAt":"2020-01-01T00:00:00.000000Z"}`, "409 documentChanged"},
	})
	if !f.DocumentInFolder.OutOfNavigation || f.DocumentInFolder.Title != "What the tent costs" {
		t.Fatal("a refused write changed the navigation or the title")
	}
	got, answer = answerTo(t, s, f.Token, http.MethodPatch, document, `{"title":"New","inNavigation":true}`)
	if got != "200" || answer["inNavigation"] != true || answer["title"] != "New" {
		t.Fatalf("%s %v", got, answer)
	}

	f.Project.Archived = true
	orderSteps(t, s, []orderStep{
		{"an archived document in an archived project", f.Token, http.MethodPatch, "/api/v1/documents/" + f.ArchivedDocument.ID, `{"inNavigation":true}`, "422 projectArchived"},
		{"in an archived project", f.Token, http.MethodPatch, document, `{"inNavigation":false}`, "422 projectArchived"},
	})
}

// A place is arranged by hand in the order it showed, takes what comes into
// it at the end, and orders automatically again as before.
func TestAPlaceIsArrangedByHandAndBack(t *testing.T) {
	s, f := New(t)
	order := "/api/v1/files-tools/" + f.Files.ID + "/order"

	got, arranged := answerTo(t, s, f.Token, http.MethodPut, order, `{"order":"manual"}`)
	if got != "200" || arranged["order"] != "manual" || arranged["folder"] != nil {
		t.Fatalf("%s %v", got, arranged)
	}
	if !sameIDs(entryIDs(t, arranged), f.Folder.ID, f.FolderShownToClients.ID, f.Document.ID) {
		t.Fatalf("entries %v", arranged["entries"])
	}
	got, written := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/documents", `{"title":"New","audience":"nobody"}`)
	if got != "201" {
		t.Fatal(got)
	}
	_, top := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-tools/"+f.Files.ID+"/entries", "")
	if !sameIDs(entryIDs(t, top), f.Folder.ID, f.FolderShownToClients.ID, f.Document.ID, written["id"].(string)) {
		t.Fatalf("a new document does not go last: %v", top["entries"])
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPut, order, `{"order":"manual"}`); got != "200" {
		t.Fatalf("manual again: %s", got)
	}

	_, back := answerTo(t, s, f.Token, http.MethodPut, order, `{"order":"automatic"}`)
	if back["order"] != "automatic" || !sameIDs(entryIDs(t, back), f.Folder.ID, f.FolderShownToClients.ID, written["id"].(string), f.Document.ID) {
		t.Fatalf("automatic %v", back["entries"])
	}
	_, folder := answerTo(t, s, f.Token, http.MethodPut, order, `{"order":"manual","folderId":"`+f.Folder.ID+`"}`)
	if folder["order"] != "manual" || folder["folder"].(map[string]any)["id"] != f.Folder.ID || arrangedByHand(f.Files, f.ArchivedFolder.ID) {
		t.Fatalf("a folder arranged by hand %v; the folder in it keeps its own order", folder)
	}
}

// Choosing the order is refused in Minka's order: the body, the tool, the
// order, the folder, the project, and a client last.
func TestTheOrderIsRefusedInMinkasOrder(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	order := "/api/v1/files-tools/" + f.Files.ID + "/order"

	orderSteps(t, s, []orderStep{
		{"a tool not seen", f.Token, http.MethodPut, "/api/v1/files-tools/" + f.OtherFiles.ID + "/order", `{"order":"manual"}`, "404"},
		{"an order that is none", f.Token, http.MethodPut, order, `{"order":"sideways"}`, "422 orderUnknown"},
		{"no order", f.Token, http.MethodPut, order, `{}`, "422 orderUnknown"},
		{"an order that is no string", f.Token, http.MethodPut, order, `{"order":1}`, "400 malformedBody"},
		{"a folder that is none", f.Token, http.MethodPut, order, `{"order":"manual","folderId":"nothing"}`, "422 folderUnknown"},
		{"an archived folder", f.Token, http.MethodPut, order, `{"order":"manual","folderId":"` + f.ArchivedFolder.ID + `"}`, "422 folderArchived"},
		{"by a client", "minka_pat_client", http.MethodPut, order, `{"order":"manual"}`, "404"},
	})
	f.Project.Archived = true
	orderSteps(t, s, []orderStep{
		{"an order that is none in an archived project", f.Token, http.MethodPut, order, `{"order":"sideways"}`, "422 orderUnknown"},
		{"an archived folder in an archived project", f.Token, http.MethodPut, order, `{"order":"manual","folderId":"` + f.ArchivedFolder.ID + `"}`, "422 projectArchived"},
		{"in an archived project", f.Token, http.MethodPut, order, `{"order":"manual"}`, "422 projectArchived"},
		{"by a client in an archived project", "minka_pat_client", http.MethodPut, order, `{"order":"manual"}`, "422 projectArchived"},
	})
}

// A move names neighbours only into a place arranged by hand, and lands right
// behind the previous one, right before the next one, or first where neither
// stands there.
func TestAMovePlacesAnEntryBetweenItsNeighbours(t *testing.T) {
	s, f := New(t)
	moves := func(kind, id string) string { return "/api/v1/" + kind + "/" + id + "/moves" }
	into := func(neighbours string) string { return `{"folderId":"` + f.Folder.ID + `"` + neighbours + `}` }

	orderSteps(t, s, []orderStep{
		{"into a place ordered automatically", f.Token, http.MethodPost, moves("documents", f.Document.ID), into(`,"nextEntryId":null`), "422 folderOrderAutomatic"},
		{"a neighbour that is no ID", f.Token, http.MethodPost, moves("documents", f.Document.ID), into(`,"previousEntryId":"first"`), "422 entryPlaceUnknown"},
		{"a neighbour that is no string", f.Token, http.MethodPost, moves("documents", f.Document.ID), into(`,"previousEntryId":1`), "400 malformedBody"},
	})
	answerTo(t, s, f.Token, http.MethodPut, "/api/v1/files-tools/"+f.Files.ID+"/order", `{"order":"manual","folderId":"`+f.Folder.ID+`"}`)
	// Plans shows the archived folder, the document and the upload; the
	// archived folder keeps its place, out of sight.
	steps := []struct {
		name, kind, id, neighbours string
		want                       []string
	}{
		{"moved in without a neighbour", "documents", f.Document.ID, "", []string{f.DocumentInFolder.ID, f.FilesUpload.ID, f.Document.ID}},
		{"after the upload's neighbour", "files-uploads", f.FilesUpload.ID, into(`,"previousEntryId":"` + f.Document.ID + `"`), []string{f.DocumentInFolder.ID, f.Document.ID, f.FilesUpload.ID}},
		{"before the first", "documents", f.Document.ID, into(`,"nextEntryId":"` + f.DocumentInFolder.ID + `"`), []string{f.Document.ID, f.DocumentInFolder.ID, f.FilesUpload.ID}},
		{"the previous one decides", "documents", f.Document.ID, into(`,"previousEntryId":"` + f.FilesUpload.ID + `","nextEntryId":"` + f.DocumentInFolder.ID + `"`), []string{f.DocumentInFolder.ID, f.FilesUpload.ID, f.Document.ID}},
		{"a neighbour from elsewhere", "files-uploads", f.FilesUpload.ID, into(`,"previousEntryId":"` + f.FolderShownToClients.ID + `"`), []string{f.FilesUpload.ID, f.DocumentInFolder.ID, f.Document.ID}},
		{"empty neighbours", "documents", f.Document.ID, into(`,"previousEntryId":"","nextEntryId":null`), []string{f.Document.ID, f.FilesUpload.ID, f.DocumentInFolder.ID}},
	}
	for _, step := range steps {
		body := step.neighbours
		if body == "" {
			body = into("")
		}
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, moves(step.kind, step.id), body); got != "200" {
			t.Fatalf("%s: %s", step.name, got)
		}
		_, place := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-tools/"+f.Files.ID+"/entries?folderId="+f.Folder.ID, "")
		if !sameIDs(entryIDs(t, place), step.want...) {
			t.Fatalf("%s: entries %v", step.name, place["entries"])
		}
	}
	// Moving out takes the entry out of the order it left.
	answerTo(t, s, f.Token, http.MethodPost, moves("documents", f.Document.ID), `{"folderId":null}`)
	if order := f.Files.Arranged[f.Folder.ID]; len(order) != 3 || order[0] == f.Document.ID {
		t.Fatalf("order %v", order)
	}

	f.Project.Archived = true
	orderSteps(t, s, []orderStep{
		{"a neighbour that is no ID in an archived project", f.Token, http.MethodPost, moves("files-uploads", f.FilesUpload.ID), into(`,"previousEntryId":"first"`), "422 entryPlaceUnknown"},
		{"a neighbour in an archived project", f.Token, http.MethodPost, moves("files-uploads", f.FilesUpload.ID), `{"folderId":null,"previousEntryId":"` + f.Folder.ID + `"}`, "422 projectArchived"},
	})
}

// Whatever comes into a book is arranged by hand: a folder made in it, and a
// folder that was never marked moved into it, with every folder below it.
func TestWhatComesIntoABookIsArrangedByHand(t *testing.T) {
	s, f := New(t)
	f.Folder.IsBook = true
	f.ArchivedFolder.Archived = false
	arrangeByHand(f.Files, f.Folder.ID)

	got, made := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/folders", `{"title":"Chapter","folderId":"`+f.ArchivedFolder.ID+`"}`)
	if got != "201" || made["order"] != "manual" {
		t.Fatalf("a folder made in a book: %s %v", got, made["order"])
	}

	below := &Folder{ID: newID(), Title: "Below", ParentID: f.FolderShownToClients.ID, CreatorID: f.Bruno.ID}
	f.Files.Folders = append(f.Files.Folders, below)
	got, moved := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-folders/"+f.FolderShownToClients.ID+"/moves", `{"folderId":"`+f.Folder.ID+`"}`)
	if got != "200" || moved["order"] != "manual" || !arrangedByHand(f.Files, below.ID) {
		t.Fatalf("a folder moved into a book: %s %v; the one below it arranged by hand: %v", got, moved["order"], arrangedByHand(f.Files, below.ID))
	}
}
