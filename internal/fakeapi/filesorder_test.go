package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// orderSteps sends one request after the other and holds each answer against
// what Minka's controllers answer in the same state.
func orderSteps(t *testing.T, s *Server, steps []orderStep) {
	t.Helper()
	for _, step := range steps {
		got, _ := answerTo(t, s, step.token, step.method, step.path, step.body)
		if got != step.want {
			t.Fatalf("%s: %s %s answered %s, want %s", step.name, step.method, step.path, got, step.want)
		}
	}
}

type orderStep struct {
	name, token, method, path, body, want string
}

func withTokens(s *Server, f *Fixture) {
	s.Tokens["minka_pat_client"] = f.Client.ID
	s.Tokens["minka_pat_anke"] = f.Anke.ID
}

func TestADocumentGoesIntoTheTrashAndComesBack(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	document := "/api/v1/documents/" + f.Document.ID

	orderSteps(t, s, []orderStep{
		{"into the trash", f.Token, http.MethodDelete, document, "", "204"},
		{"read", f.Token, http.MethodGet, document, "", "404"},
		{"a second time", f.Token, http.MethodDelete, document, "", "404"},
		{"by an outsider", "minka_pat_stranger", http.MethodPost, document + "/restore", "", "404"},
		{"by a client", "minka_pat_client", http.MethodPost, document + "/restore", "", "404"},
		{"back", f.Token, http.MethodPost, document + "/restore", "", "200"},
		{"read again", f.Token, http.MethodGet, document, "", "200"},
		{"back a second time", f.Token, http.MethodPost, document + "/restore", "", "404"},
	})
}

// A folder takes what stands in it into the trash, and only the folder brings
// it back: the upload is in the trash through it, not by itself.
func TestAFolderGoesIntoTheTrashWithItsUploadAndComesBackWithIt(t *testing.T) {
	s, f := New(t)
	folder := "/api/v1/files-folders/" + f.Folder.ID
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID

	orderSteps(t, s, []orderStep{
		{"the folder", f.Token, http.MethodDelete, folder, "", "204"},
		{"its upload", f.Token, http.MethodGet, upload, "", "404"},
		{"its upload alone", f.Token, http.MethodPost, upload + "/restore", "", "404"},
		{"its upload into the archive", f.Token, http.MethodPost, upload + "/archive", "", "404"},
		{"its upload moved out", f.Token, http.MethodPost, upload + "/moves", `{"folderId":null}`, "404"},
		{"its upload into the trash", f.Token, http.MethodDelete, upload, "", "404"},
		{"a folder in it renamed", f.Token, http.MethodPatch, "/api/v1/files-folders/" + f.ArchivedFolder.ID, `{"title":"Older"}`, "404"},
		{"listing it", f.Token, http.MethodGet, "/api/v1/files-tools/" + f.Files.ID + "/entries?folderId=" + f.Folder.ID, "", "404"},
		{"writing into it", f.Token, http.MethodPost, "/api/v1/files-tools/" + f.Files.ID + "/folders", `{"title":"Inside","folderId":"` + f.Folder.ID + `"}`, "422 folderUnknown"},
	})
	rows := trashOf(t, s, f, f.Token)
	if len(rows) != 1 || rows[0]["id"] != f.Folder.ID || rows[0]["kind"] != "folder" || rows[0]["restorable"] != true {
		t.Fatalf("trash %v", rows)
	}

	_, back := answerTo(t, s, f.Token, http.MethodPost, folder+"/restore", "")
	if back["folder"].(map[string]any)["id"] != f.Folder.ID || len(back["uploads"].([]any)) != 1 {
		t.Fatalf("restored %v", back)
	}
}

// An entry that went into the trash by itself before its folder did comes
// back only after the folder, and the trash says what is in its way.
func TestAnEntryWhoseFolderIsInTheTrashIsBlockedByIt(t *testing.T) {
	s, f := New(t)
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID
	orderSteps(t, s, []orderStep{
		{"the upload", f.Token, http.MethodDelete, upload, "", "204"},
		{"its folder", f.Token, http.MethodDelete, "/api/v1/files-folders/" + f.Folder.ID, "", "204"},
		{"the upload back", f.Token, http.MethodPost, upload + "/restore", "", "404"},
	})
	for _, row := range trashOf(t, s, f, f.Token) {
		if row["id"] == f.FilesUpload.ID && (row["restorable"] != false || row["blockedBy"] != "folder") {
			t.Fatalf("row %v", row)
		}
	}

	f.Folder.Trashed, f.Folder.Archived = false, true
	for _, row := range trashOf(t, s, f, f.Token) {
		if row["id"] == f.FilesUpload.ID && (row["restorable"] != false || row["blockedBy"] != "archivedFolder") {
			t.Fatalf("row %v", row)
		}
	}
	orderSteps(t, s, []orderStep{{"below an archived folder", f.Token, http.MethodPost, upload + "/restore", "", "404"}})
}

func TestEntriesGoIntoTheArchiveAndComeBack(t *testing.T) {
	s, f := New(t)
	document := "/api/v1/documents/" + f.DocumentInFolder.ID
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID
	folder := "/api/v1/files-folders/" + f.Folder.ID

	orderSteps(t, s, []orderStep{
		{"a document", f.Token, http.MethodPost, document + "/archive", "", "200"},
		{"it again", f.Token, http.MethodPost, document + "/archive", "", "422 documentArchived"},
		{"its folder", f.Token, http.MethodPost, folder + "/archive", "", "200"},
		{"the folder again", f.Token, http.MethodPost, folder + "/archive", "", "422 folderArchived"},
		{"an upload in it", f.Token, http.MethodPost, upload + "/archive", "", "422 uploadArchived"},
		{"the upload back", f.Token, http.MethodPost, upload + "/unarchive", "", "422 folderArchived"},
		{"the upload into the trash", f.Token, http.MethodDelete, upload, "", "422 folderArchived"},
		{"the folder renamed", f.Token, http.MethodPatch, folder, `{"title":"Old"}`, "422 folderArchived"},
		{"the folder moved", f.Token, http.MethodPost, folder + "/moves", `{"folderId":null}`, "422 folderArchived"},
		{"the folder back", f.Token, http.MethodPost, folder + "/unarchive", "", "200"},
		{"the folder back again", f.Token, http.MethodPost, folder + "/unarchive", "", "422 folderNotArchived"},
		{"the document back", f.Token, http.MethodPost, document + "/unarchive", "", "200"},
		{"the document back again", f.Token, http.MethodPost, document + "/unarchive", "", "422 documentNotArchived"},
		{"the upload away", f.Token, http.MethodPost, upload + "/archive", "", "200"},
		{"the upload back for good", f.Token, http.MethodPost, upload + "/unarchive", "", "200"},
		{"an archived document into the trash", f.Token, http.MethodDelete, "/api/v1/documents/" + f.ArchivedDocument.ID, "", "204"},
	})
	if f.ArchivedDocument.Archived {
		t.Fatal("a document in the trash still says it is archived")
	}
}

// The archive of a tool lists what went into it itself, at any depth, the one
// archived last first, and says which of them a folder above holds back.
func TestTheArchiveOfAToolListsItsEntriesLastArchivedFirst(t *testing.T) {
	s, f := New(t)
	f.Document.Archived = true
	archivedBelow := &Document{ID: newID(), Title: "Below", FolderID: f.ArchivedFolder.ID, Archived: true, CreatorID: f.Bruno.ID}
	f.Files.Documents = append(f.Files.Documents, archivedBelow)
	orderSteps(t, s, []orderStep{{"an upload", f.Token, http.MethodPost, "/api/v1/files-uploads/" + f.FilesUpload.ID + "/archive", "", "200"}})

	_, got := answerList(t, s, f.Token, "/api/v1/files-tools/"+f.Files.ID+"/archive")
	var order []string
	for _, entry := range got {
		order = append(order, entry["kind"].(string)+" "+entry["title"].(string))
		if entry["projectId"] != f.Project.ID || entry["appUrl"] == "" {
			t.Fatalf("entry %v", entry)
		}
		if (entry["id"] == archivedBelow.ID) != (entry["archivedWithAFolderAbove"] == true) {
			t.Fatalf("entry %v", entry)
		}
	}
	want := []string{"upload site-plan.pdf", "document Put away", "folder Last year", "document Below", "document How the stage goes up"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i := range want[:3] {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v first", order, want[:3])
		}
	}
	if got[0]["archivedBy"].(map[string]any)["id"] != f.Bruno.ID || got[2]["archivedBy"].(map[string]any)["id"] != f.Anke.ID {
		t.Fatalf("archived by %v and %v", got[0]["archivedBy"], got[2]["archivedBy"])
	}

	f.Folder.Trashed = true
	_, got = answerList(t, s, f.Token, "/api/v1/files-tools/"+f.Files.ID+"/archive")
	if len(got) != 2 {
		t.Fatalf("what stands below a folder in the trash is listed: %v", got)
	}
	if code, _ := answerList(t, s, "minka_pat_stranger", "/api/v1/files-tools/"+f.Files.ID+"/archive"); code != "404" {
		t.Fatalf("an outsider: %s", code)
	}
}

func TestFoldersAreMadeRenamedAndMovedAndEntriesMoveBetweenThem(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/files-tools/" + f.Files.ID + "/folders"

	got, made := answerTo(t, s, f.Token, http.MethodPost, tool, `{"title":" Offers ","folderId":"`+f.Folder.ID+`"}`)
	if got != "201" || made["folder"].(map[string]any)["title"] != "Offers" || len(made["path"].([]any)) != 2 {
		t.Fatalf("%s %v", got, made)
	}
	offers := made["folder"].(map[string]any)["id"].(string)

	orderSteps(t, s, []orderStep{
		{"no title", f.Token, http.MethodPost, tool, `{"title":""}`, "422 folderTitleMissing"},
		{"a title that is no string", f.Token, http.MethodPost, tool, `{"title":1}`, "400 malformedBody"},
		{"into an archived folder", f.Token, http.MethodPost, tool, `{"title":"New","folderId":"` + f.ArchivedFolder.ID + `"}`, "422 folderArchived"},
		{"into no folder", f.Token, http.MethodPost, tool, `{"title":"New","folderId":"no-folder"}`, "422 folderUnknown"},
		{"into a tool not seen", f.Token, http.MethodPost, "/api/v1/files-tools/" + f.OtherFiles.ID + "/folders", `{"title":"New"}`, "404"},
		{"shown to clients, said wrongly", f.Token, http.MethodPost, tool, `{"title":"New","visibleToClients":"yes"}`, "400 malformedBody"},
		{"renamed", f.Token, http.MethodPatch, "/api/v1/files-folders/" + offers, `{"title":"Offers 2027"}`, "200"},
		{"renamed to nothing", f.Token, http.MethodPatch, "/api/v1/files-folders/" + offers, `{"title":""}`, "422 folderTitleMissing"},
		{"not renamed at all", f.Token, http.MethodPatch, "/api/v1/files-folders/" + offers, `{}`, "200"},
		{"a document moved without a place", f.Token, http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/moves", `{}`, "422 folderIdMissing"},
		{"a document moved into it", f.Token, http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/moves", `{"folderId":"` + offers + `"}`, "200"},
		{"into a folder of another tool", f.Token, http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/moves", `{"folderId":"` + f.OtherFiles.ID + `"}`, "422 folderUnknown"},
		{"into an archived folder", f.Token, http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/moves", `{"folderId":"` + f.ArchivedFolder.ID + `"}`, "422 folderArchived"},
		{"an upload to the top", f.Token, http.MethodPost, "/api/v1/files-uploads/" + f.FilesUpload.ID + "/moves", `{"folderId":null}`, "200"},
		{"a folder into one below it", f.Token, http.MethodPost, "/api/v1/files-folders/" + f.Folder.ID + "/moves", `{"folderId":"` + offers + `"}`, "422 folderIntoItself"},
		{"a folder into itself", f.Token, http.MethodPost, "/api/v1/files-folders/" + f.Folder.ID + "/moves", `{"folderId":"` + f.Folder.ID + `"}`, "422 folderIntoItself"},
		{"a folder to the top", f.Token, http.MethodPost, "/api/v1/files-folders/" + offers + "/moves", `{"folderId":null}`, "200"},
	})
	if f.Document.FolderID != offers || f.FilesUpload.FolderID != "" || f.Files.Folders[0].ID != offers || f.Files.Folders[0].ParentID != "" {
		t.Fatalf("document in %q, upload in %q", f.Document.FolderID, f.FilesUpload.FolderID)
	}
	if f.Files.Folders[0].Title != "Offers 2027" {
		t.Fatalf("title %q", f.Files.Folders[0].Title)
	}
}

// In an archived project every write is refused as such, a move to where
// the entry stands writes nothing and is answered as done.
func TestAnArchivedProjectTakesNoneOfIt(t *testing.T) {
	s, f := New(t)
	versionID := f.Document.versions()[0].ID
	trashed := &Document{ID: newID(), Title: "Old", CreatorID: f.Bruno.ID, EntryState: EntryState{Trashed: true}}
	f.Files.Documents = append(f.Files.Documents, trashed)
	f.Project.Archived = true
	document := "/api/v1/documents/" + f.Document.ID
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID
	folder := "/api/v1/files-folders/" + f.Folder.ID

	var steps []orderStep
	for _, one := range [][3]string{
		{http.MethodDelete, document, ""},
		{http.MethodPost, document + "/archive", ""},
		{http.MethodPost, "/api/v1/documents/" + f.ArchivedDocument.ID + "/unarchive", ""},
		{http.MethodPost, document + "/moves", `{"folderId":"` + f.Folder.ID + `"}`},
		{http.MethodPost, document + "/versions/" + versionID + "/restore", ""},
		{http.MethodPost, "/api/v1/documents/" + trashed.ID + "/restore", ""},
		{http.MethodDelete, upload, ""},
		{http.MethodPost, upload + "/archive", ""},
		{http.MethodPost, upload + "/moves", `{"folderId":null}`},
		{http.MethodPost, "/api/v1/files-tools/" + f.Files.ID + "/folders", `{"title":"New"}`},
		{http.MethodPatch, folder, `{"title":"New"}`},
		{http.MethodDelete, folder, ""},
		{http.MethodPost, folder + "/archive", ""},
		{http.MethodPost, "/api/v1/files-folders/" + f.ArchivedFolder.ID + "/moves", `{"folderId":null}`},
	} {
		steps = append(steps, orderStep{one[1], f.Token, one[0], one[1], one[2], "422 projectArchived"})
	}
	steps = append(steps,
		orderStep{"a move to where it stands", f.Token, http.MethodPost, upload + "/moves", `{"folderId":"` + f.Folder.ID + `"}`, "422 projectArchived"},
		orderStep{"a move into an archived folder", f.Token, http.MethodPost, upload + "/moves", `{"folderId":"` + f.ArchivedFolder.ID + `"}`, "422 projectArchived"},
		orderStep{"a folder into itself", f.Token, http.MethodPost, folder + "/moves", `{"folderId":"` + f.Folder.ID + `"}`, "422 projectArchived"},
		orderStep{"a folder made in an archived folder", f.Token, http.MethodPost, "/api/v1/files-tools/" + f.Files.ID + "/folders", `{"title":"New","folderId":"` + f.ArchivedFolder.ID + `"}`, "422 projectArchived"},
		orderStep{"a document written in an archived folder", f.Token, http.MethodPost, "/api/v1/files-tools/" + f.Files.ID + "/documents", `{"title":"New","audience":"nobody","folderId":"` + f.ArchivedFolder.ID + `"}`, "422 projectArchived"},
		orderStep{"a move without its field", f.Token, http.MethodPost, upload + "/moves", `{}`, "422 folderIdMissing"},
		orderStep{"a move into no folder", f.Token, http.MethodPost, upload + "/moves", `{"folderId":"` + f.OtherFiles.ID + `"}`, "422 folderUnknown"},
		orderStep{"a title the web refuses", f.Token, http.MethodPatch, folder, `{"title":""}`, "422 folderTitleMissing"},
		orderStep{"a version that is none", f.Token, http.MethodPost, document + "/versions/" + newID() + "/restore", "", "404"},
	)
	orderSteps(t, s, steps)
}

// A client puts away only what they made, brings nothing back and restores no
// version; making a folder and putting it away again is theirs to do.
func TestAClientPutsAwayOnlyTheirOwnAndBringsNothingBack(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	versionID := f.Document.versions()[0].ID
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID

	orderSteps(t, s, []orderStep{
		{"somebody else's into the trash", "minka_pat_client", http.MethodDelete, upload, "", "404"},
		{"somebody else's into the archive", "minka_pat_client", http.MethodPost, upload + "/archive", "", "404"},
		{"a version", "minka_pat_client", http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/versions/" + versionID + "/restore", "", "404"},
		{"out of the archive", "minka_pat_client", http.MethodPost, "/api/v1/documents/" + f.ArchivedDocument.ID + "/unarchive", "", "404"},
		{"moving what they are not shown", "minka_pat_client", http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/moves", `{"folderId":"` + f.FolderShownToClients.ID + `"}`, "404"},
		{"by the team", f.Token, http.MethodDelete, upload, "", "204"},
		{"out of the trash", "minka_pat_client", http.MethodPost, upload + "/restore", "", "404"},
	})

	got, made := answerTo(t, s, "minka_pat_client", http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/folders", `{"title":"From the client"}`)
	if got != "201" {
		t.Fatalf("a client's folder: %s", got)
	}
	own := "/api/v1/files-folders/" + made["folder"].(map[string]any)["id"].(string)
	if !f.Files.Folders[0].ShownToClients {
		t.Fatal("a client's own folder at the top is not shown to clients")
	}
	orderSteps(t, s, []orderStep{
		{"their own into the archive", "minka_pat_client", http.MethodPost, own + "/archive", "", "200"},
		{"their own out of it", "minka_pat_client", http.MethodPost, own + "/unarchive", "", "404"},
		{"their own into the trash", "minka_pat_client", http.MethodDelete, own, "", "204"},
		{"their own out of it", "minka_pat_client", http.MethodPost, own + "/restore", "", "404"},
	})

	// A document a client writes at the top of the tool is shown to clients,
	// so it is theirs to put away.
	got, written := answerTo(t, s, "minka_pat_client", http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/documents", `{"title":"From the client","audience":"nobody"}`)
	if got != "201" {
		t.Fatalf("a client's document: %s", got)
	}
	document := "/api/v1/documents/" + written["id"].(string)
	orderSteps(t, s, []orderStep{
		{"their own document into the archive", "minka_pat_client", http.MethodPost, document + "/archive", "", "200"},
		{"their own document into the trash", "minka_pat_client", http.MethodDelete, document, "", "204"},
	})
	if rows := trashOf(t, s, f, "minka_pat_client"); len(rows) != 0 {
		t.Fatalf("a client reads the trash: %v", rows)
	}
}

func TestOnlyTheTeamShowsANewFolderToClientsAndOnlyAtTheTop(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/files-tools/" + f.Files.ID + "/folders"
	for _, c := range []struct {
		body  string
		shown bool
	}{
		{`{"title":"Shown","visibleToClients":true}`, true},
		{`{"title":"Not asked"}`, false},
		{`{"title":"Deeper","visibleToClients":true,"folderId":"` + f.Folder.ID + `"}`, false},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, tool, c.body); got != "201" || f.Files.Folders[0].ShownToClients != c.shown {
			t.Fatalf("%s: %s, shown %v", c.body, got, f.Files.Folders[0].ShownToClients)
		}
	}
}

func TestAnEarlierVersionOfADocumentBecomesTheCurrentOne(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	document := "/api/v1/documents/" + f.Document.ID
	first := f.Document.versions()[0].ID
	orderSteps(t, s, []orderStep{{"a change", f.Token, http.MethodPatch, document, `{"body":"<p>Nine people.</p>"}`, "200"}})

	got, restored := answerTo(t, s, "minka_pat_anke", http.MethodPost, document+"/versions/"+first+"/restore", "")
	if got != "200" || restored["body"] != "<p>Eight people on <strong>Friday</strong>.</p>" || len(f.Document.Versions) != 3 {
		t.Fatalf("%s, body %v, %d versions", got, restored["body"], len(f.Document.Versions))
	}
	if f.Document.Versions[2].AuthorID != f.Anke.ID {
		t.Fatal("the restored text is not written by whoever restored it")
	}
	orderSteps(t, s, []orderStep{
		{"what it says already", f.Token, http.MethodPost, document + "/versions/" + first + "/restore", "", "200"},
		{"no such version", f.Token, http.MethodPost, document + "/versions/" + newID() + "/restore", "", "404"},
		{"a version that is no identifier", f.Token, http.MethodPost, document + "/versions/first/restore", "", "404"},
		{"by an outsider", "minka_pat_stranger", http.MethodPost, document + "/versions/" + first + "/restore", "", "404"},
	})
	if len(f.Document.Versions) != 3 {
		t.Fatalf("a restore that changes nothing kept a version: %d", len(f.Document.Versions))
	}
	f.Document.Archived = true
	orderSteps(t, s, []orderStep{
		{"of an archived document", f.Token, http.MethodPost, document + "/versions/" + first + "/restore", "", "422 documentArchived"},
		{"no such version of an archived document", f.Token, http.MethodPost, document + "/versions/" + newID() + "/restore", "", "404"},
	})
}

// A draft is at none of these addresses, and its author alone finds it in
// the trash, where it cannot be brought back.
func TestADraftIsAtNoneOfTheseAddresses(t *testing.T) {
	s, f := New(t)
	withTokens(s, f)
	draft := "/api/v1/documents/" + f.DraftDocument.ID
	orderSteps(t, s, []orderStep{
		{"into the trash", f.Token, http.MethodDelete, draft, "", "404"},
		{"into the archive", f.Token, http.MethodPost, draft + "/archive", "", "404"},
		{"moved", f.Token, http.MethodPost, draft + "/moves", `{"folderId":null}`, "404"},
	})
	f.DraftDocument.Trashed = true
	orderSteps(t, s, []orderStep{{"out of the trash", f.Token, http.MethodPost, draft + "/restore", "", "404"}})

	rows := trashOf(t, s, f, f.Token)
	if len(rows) != 1 || rows[0]["id"] != f.DraftDocument.ID || rows[0]["restorable"] != false || rows[0]["blockedBy"] != nil {
		t.Fatalf("the author's trash %v", rows)
	}
	if rows := trashOf(t, s, f, "minka_pat_anke"); len(rows) != 0 {
		t.Fatalf("somebody else's trash %v", rows)
	}
}

func TestAnOutsiderIsToldNothing(t *testing.T) {
	s, f := New(t)
	var steps []orderStep
	for _, one := range [][3]string{
		{http.MethodGet, "/api/v1/files-tools/" + f.Files.ID + "/archive", ""},
		{http.MethodPost, "/api/v1/files-tools/" + f.Files.ID + "/folders", `{"title":"Mine"}`},
		{http.MethodPatch, "/api/v1/files-folders/" + f.Folder.ID, `{"title":"Mine"}`},
		{http.MethodDelete, "/api/v1/files-folders/" + f.Folder.ID, ""},
		{http.MethodPost, "/api/v1/files-folders/" + f.Folder.ID + "/archive", ""},
		{http.MethodPost, "/api/v1/files-folders/" + f.ArchivedFolder.ID + "/unarchive", ""},
		{http.MethodPost, "/api/v1/files-folders/" + f.Folder.ID + "/moves", `{"folderId":null}`},
		{http.MethodDelete, "/api/v1/documents/" + f.Document.ID, ""},
		{http.MethodPost, "/api/v1/documents/" + f.Document.ID + "/archive", ""},
		{http.MethodPost, "/api/v1/files-uploads/" + f.FilesUpload.ID + "/moves", `{"folderId":null}`},
	} {
		steps = append(steps, orderStep{one[1], "minka_pat_stranger", one[0], one[1], one[2], "404"})
	}
	orderSteps(t, s, steps)
}

func trashOf(t *testing.T, s *Server, f *Fixture, token string) []map[string]any {
	t.Helper()
	code, rows := answerList(t, s, token, "/api/v1/projects/"+f.Project.ID+"/trash")
	if code != "200" {
		t.Fatalf("the trash: %s", code)
	}
	var entries []map[string]any
	for _, row := range rows {
		if row["kind"] == "folder" || row["kind"] == "document" || row["kind"] == "upload" {
			entries = append(entries, row)
		}
	}
	return entries
}

// answerList reads a list, or says how it was refused.
func answerList(t *testing.T, s *Server, token, path string) (string, []map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var list []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&list)
	return fmt.Sprint(response.StatusCode), list
}
