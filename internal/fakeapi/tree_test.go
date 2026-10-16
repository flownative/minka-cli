package fakeapi

import (
	"net/http"
	"testing"
)

// A folder is read by its identifier alone, in the shape of the entries of its
// tool with its project, the folder it stands in and when it changed; one in
// the archive as well, one in the trash or below one not.
func TestAFolderIsReadByItself(t *testing.T) {
	s, f := New(t)
	got, folder := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.ArchivedFolder.ID, "")
	if got != "200" || folder["folder"].(map[string]any)["id"] != f.ArchivedFolder.ID || folder["archived"] != true ||
		folder["parent"].(map[string]any)["id"] != f.Folder.ID || folder["projectId"] != f.Project.ID ||
		folder["project"].(map[string]any)["name"] != "Test-Projekt" || folder["updatedAt"] != timestamp(f.ArchivedFolder.CreatedAt) {
		t.Fatalf("%s %v", got, folder)
	}
	if !sameIDs(entryIDs(t, folder), f.DocumentBelowTheArchivedFolder.ID) {
		t.Fatalf("entries %v", folder["entries"])
	}

	_, top := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID, "")
	if top["parent"] != nil || top["updatedAt"] != timestamp(f.Folder.CreatedAt) {
		t.Fatalf("a folder at the top: %v", top)
	}
	_, tool := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-tools/"+f.Files.ID+"/entries", "")
	if tool["parent"] != nil || tool["updatedAt"] != nil || tool["projectId"] != f.Project.ID {
		t.Fatalf("the top of the tool: %v", tool)
	}

	// A rename is a change of the folder itself.
	answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/files-folders/"+f.Folder.ID, `{"title":"Plans 2027"}`)
	if _, renamed := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID, ""); renamed["updatedAt"] == timestamp(f.Folder.CreatedAt) {
		t.Fatalf("a rename left updatedAt: %v", renamed["updatedAt"])
	}

	f.Folder.Trashed = true
	for _, id := range []string{f.Folder.ID, f.ArchivedFolder.ID, newID(), "plans"} {
		if got, _ := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+id, ""); got != "404" {
			t.Errorf("%s: %s", id, got)
		}
	}
}

// Any folder is read as one tree in the shape of a book, starting at the
// folder, with its uploads in the order of their folder: drafts and what lies
// in the archive left out, and a page's version named to the team alone.
func TestAnyFolderIsReadAsATree(t *testing.T) {
	s, f := New(t)
	f.Files.Arranged = map[string][]string{f.Folder.ID: {f.FilesUpload.ID, f.DocumentInFolder.ID}}
	f.DocumentInFolder.Edit("<p>Five hundred.</p>")
	draft := &Document{ID: newID(), Title: "Not finished", FolderID: f.Folder.ID, Draft: true, CreatedAt: f.Folder.CreatedAt, UpdatedAt: f.Folder.CreatedAt}
	archived := &FilesUpload{ID: newID(), Title: "old.pdf", BlobID: f.FilesUpload.BlobID, FolderID: f.Folder.ID, Archived: true}
	f.Files.Documents = append(f.Files.Documents, draft)
	f.Files.Uploads = append(f.Files.Uploads, archived)

	got, tree := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID+"/tree", "")
	if got != "200" || tree["id"] != f.Folder.ID || tree["order"] != "manual" || tree["tool"].(map[string]any)["id"] != f.Files.ID {
		t.Fatalf("%s %v", got, tree)
	}
	if !sameIDs(entryIDs(t, tree), f.FilesUpload.ID, f.DocumentInFolder.ID) {
		t.Fatalf("entries %v", tree["entries"])
	}
	upload, page := tree["entries"].([]any)[0].(map[string]any), tree["entries"].([]any)[1].(map[string]any)
	if upload["kind"] != "upload" || upload["file"].(map[string]any)["id"] != f.FilesUpload.BlobID ||
		upload["downloadUrl"] != s.URL+"/api/v1/uploads/"+f.FilesUpload.BlobID+"/original" || upload["updatedAt"] != timestamp(f.FilesUpload.UpdatedAt) {
		t.Fatalf("upload %v", upload)
	}
	if page["kind"] != "page" || page["version"] != 2.0 || page["updatedAt"] != timestamp(f.DocumentInFolder.UpdatedAt) {
		t.Fatalf("page %v", page)
	}

	// The book of the folder carries no uploads, its pages the same version.
	f.Folder.IsBook = true
	_, book := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID+"/book", "")
	if !sameIDs(entryIDs(t, book), f.DocumentInFolder.ID) || book["entries"].([]any)[0].(map[string]any)["version"] != 2.0 {
		t.Fatalf("book %v", book["entries"])
	}

	// A folder in the archive is read with what is in use in it.
	_, lastYear := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.ArchivedFolder.ID+"/tree", "")
	if !sameIDs(entryIDs(t, lastYear), f.DocumentBelowTheArchivedFolder.ID) {
		t.Fatalf("archived %v", lastYear)
	}

	// A client sees everything in a folder shown to clients, and no version.
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.DocumentInFolder.FolderID = f.FolderShownToClients.ID
	got, shown := answerTo(t, s, "minka_pat_client", http.MethodGet, "/api/v1/files-folders/"+f.FolderShownToClients.ID+"/tree", "")
	if got != "200" || !sameIDs(entryIDs(t, shown), f.DocumentInFolder.ID) {
		t.Fatalf("%s %v", got, shown)
	}
	if page := shown["entries"].([]any)[0].(map[string]any); page["version"] != nil || page["updatedAt"] == nil {
		t.Fatalf("a client's page %v", page)
	}
	if got, _ := answerTo(t, s, "minka_pat_client", http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID+"/tree", ""); got != "404" {
		t.Fatalf("a folder not shown to a client: %s", got)
	}
}

// A Minka without the tree answers both addresses with a page of its web.
func TestAMinkaWithoutTheTreeAnswersWithAPage(t *testing.T) {
	s, f := New(t)
	s.KnowsNoTree = true
	for _, path := range []string{"/api/v1/files-folders/" + f.Folder.ID, "/api/v1/files-folders/" + f.Folder.ID + "/tree"} {
		got, answer := answerTo(t, s, f.Token, http.MethodGet, path, "")
		if got != "404" || answer != nil {
			t.Fatalf("%s: %s %v", path, got, answer)
		}
	}
	f.Folder.IsBook = true
	_, book := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-folders/"+f.Folder.ID+"/book", "")
	if _, named := book["entries"].([]any)[0].(map[string]any)["version"]; named {
		t.Fatalf("book %v", book)
	}
}
