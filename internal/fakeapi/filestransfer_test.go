package fakeapi

import (
	"net/http"
	"strings"
	"testing"
)

// A move into another tool is refused in the order Minka asks: the entry and
// its archive before the body, the body, a client, the tool, neighbours, the
// folder there, both projects, and then what the move itself meets.
func TestAMoveIntoAnotherToolIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	const clientToken = "minka_pat_client"
	for _, c := range []struct {
		name    string
		prepare func(s *Server, f *Fixture)
		token   string
		entry   string // "document", "archived", "shown", "folder" or "upload"
		body    string
		want    string
	}{
		{"an archived entry before the body", nil, "", "archived", `{`, "422 documentArchived"},
		{"an archived entry in an archived project", func(_ *Server, f *Fixture) { f.Project.Archived = true }, "", "archived", `{"toolId":"OTHER","folderId":null}`, "422 projectArchived"},
		{"a body that is none", nil, "", "document", `{`, "400 malformedBody"},
		{"a toolId that is no string", nil, "", "document", `{"toolId":5,"folderId":null}`, "400 malformedBody"},
		{"a toolId that is no string within the tool", nil, "", "document", `{"toolId":true,"folderId":"FOLDER"}`, "400 malformedBody"},
		{"a client shown the entry", nil, clientToken, "shown", `{"toolId":"OTHER","folderId":null}`, "404"},
		{"a client before an unknown tool", nil, clientToken, "shown", `{"toolId":"nothing","folderId":null}`, "404"},
		{"a toolId that is no ID", nil, "", "document", `{"toolId":"nothing","folderId":null}`, "422 toolUnknown"},
		{"an empty toolId", nil, "", "document", `{"toolId":"","folderId":null}`, "422 toolUnknown"},
		{"a tool of nothing", nil, "", "document", `{"toolId":"00000000-0000-4000-8000-000000000000","folderId":null}`, "422 toolUnknown"},
		{"a tool of a project one is not on", func(_ *Server, f *Fixture) {
			f.OtherProject.MemberIDs = []string{f.Stranger.ID}
		}, "", "document", `{"toolId":"OTHER","folderId":null}`, "422 toolUnknown"},
		{"a tool of the template library", func(s *Server, f *Fixture) {
			s.FilesTools = append(s.FilesTools, &FilesTool{ID: "0199bbbb-0000-7000-8000-000000000001", ProjectID: f.TemplateLibrary.ID, Title: "Library files"})
		}, "", "document", `{"toolId":"0199bbbb-0000-7000-8000-000000000001","folderId":null}`, "422 toolUnknown"},
		{"an unknown tool before neighbours", nil, "", "document", `{"toolId":"nothing","folderId":null,"nextEntryId":null}`, "422 toolUnknown"},
		{"a neighbour named as null", nil, "", "document", `{"toolId":"OTHER","folderId":null,"previousEntryId":null}`, "422 neighboursAcrossTools"},
		{"a neighbour before the folder", nil, "", "document", `{"toolId":"OTHER","nextEntryId":"FOLDER"}`, "422 neighboursAcrossTools"},
		{"no folderId", nil, "", "document", `{"toolId":"OTHER"}`, "422 folderIdMissing"},
		{"a folderId that is no string", nil, "", "document", `{"toolId":"OTHER","folderId":5}`, "400 malformedBody"},
		{"a folder of the entry's own tool", nil, "", "document", `{"toolId":"OTHER","folderId":"FOLDER"}`, "422 folderUnknown"},
		{"an unknown folder in an archived project", func(_ *Server, f *Fixture) { f.Project.Archived = true }, "", "document", `{"toolId":"OTHER","folderId":"FOLDER"}`, "422 folderUnknown"},
		{"an archived folder there in an archived project", func(_ *Server, f *Fixture) {
			f.OtherFiles.Folders = append(f.OtherFiles.Folders, &Folder{ID: "0199cccc-0000-7000-8000-000000000001", Title: "Old", Archived: true})
			f.OtherProject.Archived = true
		}, "", "document", `{"toolId":"OTHER","folderId":"0199cccc-0000-7000-8000-000000000001"}`, "422 projectArchived"},
		{"an archived folder there", func(_ *Server, f *Fixture) {
			f.OtherFiles.Folders = append(f.OtherFiles.Folders, &Folder{ID: "0199cccc-0000-7000-8000-000000000001", Title: "Old", Archived: true})
		}, "", "document", `{"toolId":"OTHER","folderId":"0199cccc-0000-7000-8000-000000000001"}`, "422 folderArchived"},
		{"the entry's project archived", func(_ *Server, f *Fixture) { f.Project.Archived = true }, "", "document", `{"toolId":"OTHER","folderId":null}`, "422 projectArchived"},
		{"the target project archived", func(_ *Server, f *Fixture) { f.OtherProject.Archived = true }, "", "document", `{"toolId":"OTHER","folderId":null}`, "422 projectArchived"},
		{"a book into a book", func(_ *Server, f *Fixture) {
			f.Folder.IsBook = true
			f.OtherFiles.Folders = append(f.OtherFiles.Folders, &Folder{ID: "0199cccc-0000-7000-8000-000000000002", Title: "Handbook", IsBook: true, CreatorID: f.Bruno.ID})
		}, "", "folder", `{"toolId":"OTHER","folderId":"0199cccc-0000-7000-8000-000000000002"}`, "422 bookWithinABook"},
		{"a file shared with what stays", func(_ *Server, f *Fixture) {
			f.Files.Uploads = append(f.Files.Uploads, &FilesUpload{ID: "0199dddd-0000-7000-8000-000000000001", Title: "copy", BlobID: f.FilesUpload.BlobID})
		}, "", "upload", `{"toolId":"OTHER","folderId":null}`, "422 entrySharesFiles"},
		{"a file shared within the project", func(s *Server, f *Fixture) {
			f.Files.Uploads = append(f.Files.Uploads, &FilesUpload{ID: "0199dddd-0000-7000-8000-000000000001", Title: "copy", BlobID: f.FilesUpload.BlobID})
			s.FilesTools = append(s.FilesTools, &FilesTool{ID: "0199bbbb-0000-7000-8000-000000000002", ProjectID: f.Project.ID, Title: "More files"})
		}, "", "upload", `{"toolId":"0199bbbb-0000-7000-8000-000000000002","folderId":null}`, "200"},
		{"the entry's own tool in capitals", nil, "", "document", `{"toolId":"OWN","folderId":"FOLDER"}`, "200"},
		{"another tool", nil, "", "document", `{"toolId":"OTHER","folderId":null}`, "200"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID, f.Client.ID)
			s.Tokens[clientToken] = f.Client.ID
			f.Files.ShownToClients = true
			shown := &Document{ID: "0199eeee-0000-7000-8000-000000000001", Title: "For the client", FolderID: f.FolderShownToClients.ID, CreatorID: f.Bruno.ID}
			f.Files.Documents = append(f.Files.Documents, shown)
			if c.prepare != nil {
				c.prepare(s, f)
			}
			path := map[string]string{
				"document": "/api/v1/documents/" + f.Document.ID, "archived": "/api/v1/documents/" + f.ArchivedDocument.ID,
				"shown": "/api/v1/documents/" + shown.ID, "folder": "/api/v1/files-folders/" + f.Folder.ID,
				"upload": "/api/v1/files-uploads/" + f.FilesUpload.ID,
			}[c.entry]
			token := f.Token
			if c.token != "" {
				token = c.token
			}
			body := strings.NewReplacer("OTHER", f.OtherFiles.ID, "OWN", strings.ToUpper(f.Files.ID), "FOLDER", f.Folder.ID).Replace(c.body)
			if got, _ := answerTo(t, s, token, http.MethodPost, path+"/moves", body); got != c.want {
				t.Fatalf("%s, want %s", got, c.want)
			}
		})
	}
}

// What moves into another tool keeps its ID and is read at its address in the
// new place, hidden from clients there; a label of neither the project nor
// the tool there comes off.
func TestAnEntryMovedIntoAnotherToolKeepsItsIDAndLosesForeignLabels(t *testing.T) {
	s, f := New(t)
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID)
	sameTool := &FilesTool{ID: "0199bbbb-0000-7000-8000-000000000003", ProjectID: f.Project.ID, Title: "More files"}
	s.FilesTools = append(s.FilesTools, sameTool)
	ofTheProject := s.AddLabel(f.Project.ID, "", "Urgent", "red")
	ofTheTool := s.AddLabel(f.Project.ID, f.Files.ID, "Draft", "grey")
	ofTheOtherTool := s.AddLabel(f.Project.ID, sameTool.ID, "Filed", "blue")
	for _, label := range []*Label{ofTheProject, ofTheTool, ofTheOtherTool} {
		s.PutLabelOn(f.FilesUpload.ID, label)
	}

	got, answer := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-uploads/"+f.FilesUpload.ID+"/moves", `{"toolId":"`+sameTool.ID+`","folderId":null}`)
	if got != "200" || answer["id"] != f.FilesUpload.ID || answer["toolId"] != sameTool.ID {
		t.Fatalf("%s %v", got, answer)
	}
	if kept := s.LabelsOn(f.FilesUpload.ID); len(kept) != 2 || kept[0] != ofTheOtherTool || kept[1] != ofTheProject {
		t.Fatalf("labels %v: within the project only the tool's own come off", kept)
	}
	if !f.FilesUpload.HiddenFromClients {
		t.Fatal("the upload arrived shown to clients")
	}

	got, _ = answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-uploads/"+f.FilesUpload.ID+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":null}`)
	if got != "200" || len(s.LabelsOn(f.FilesUpload.ID)) != 0 {
		t.Fatalf("%s, labels %v: across projects every label comes off", got, s.LabelsOn(f.FilesUpload.ID))
	}
	if got, answer := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/files-uploads/"+f.FilesUpload.ID, ""); got != "200" || answer["projectId"] != f.OtherProject.ID {
		t.Fatalf("read back: %s %v", got, answer)
	}
}

// A document at the top that a client made is shown to clients, and once it
// moved into another tool it is not, whoever made it.
func TestADocumentMovedToTheTopOfAnotherToolIsHiddenFromClients(t *testing.T) {
	const clientToken = "minka_pat_client"
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID, f.Client.ID)
	f.OtherFiles.ShownToClients = true
	f.Document.CreatorID = f.Client.ID
	path := "/api/v1/documents/" + f.Document.ID

	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "200" {
		t.Fatalf("before the move the client reads %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, path+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":null}`); got != "200" {
		t.Fatalf("the move: %s", got)
	}
	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "404" {
		t.Fatalf("after the move the client reads %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodGet, path, ""); got != "200" {
		t.Fatalf("after the move the team reads %s", got)
	}
}

// An upload in a folder shown to clients is read by a client, and once it
// moved to the top of another tool it is not.
func TestAnUploadMovedIntoAnotherToolIsHiddenFromClients(t *testing.T) {
	const clientToken = "minka_pat_client"
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID, f.Client.ID)
	f.OtherFiles.ShownToClients = true
	f.Folder.ShownToClients = true
	path := "/api/v1/files-uploads/" + f.FilesUpload.ID

	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "200" {
		t.Fatalf("before the move the client reads %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, path+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":null}`); got != "200" {
		t.Fatalf("the move: %s", got)
	}
	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "404" {
		t.Fatalf("after the move the client reads %s", got)
	}
}

// A client reads an entry only in a tool the clients are let into, whatever
// its folder says.
func TestAClientReadsNoEntryOfAToolNotShownToClients(t *testing.T) {
	const clientToken = "minka_pat_client"
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	f.Folder.ShownToClients = true
	path := "/api/v1/documents/" + f.DocumentInFolder.ID

	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "200" {
		t.Fatalf("in a tool shown to clients the client reads %s", got)
	}
	f.Files.ShownToClients = false
	if got, _ := answerTo(t, s, clientToken, http.MethodGet, path, ""); got != "404" {
		t.Fatalf("in a tool not shown to clients the client reads %s", got)
	}
}

// A folder that moves into a book of the other tool is arranged by hand, and
// goes to the end of the book's order.
func TestAFolderMovedIntoABookElsewhereIsArrangedByHand(t *testing.T) {
	s, f := New(t)
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID)
	handbook := &Folder{ID: "0199cccc-0000-7000-8000-000000000003", Title: "Handbook", IsBook: true, CreatorID: f.Bruno.ID}
	f.OtherFiles.Folders = append(f.OtherFiles.Folders, handbook)
	f.OtherFiles.Arranged = map[string][]string{handbook.ID: {}}

	if got, _ := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-folders/"+f.Folder.ID+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":"`+handbook.ID+`"}`); got != "200" {
		t.Fatalf("the move: %s", got)
	}
	if !arrangedByHand(f.OtherFiles, f.Folder.ID) {
		t.Fatal("the folder in the book is not arranged by hand")
	}
	if order := f.OtherFiles.Arranged[handbook.ID]; len(order) == 0 || order[len(order)-1] != f.Folder.ID {
		t.Fatalf("order of the book %v: the folder does not stand at its end", order)
	}
}

// The places below a moved folder that were arranged by hand stay so in the
// new tool, outside any book.
func TestTheOrderByHandBelowAMovedFolderGoesAlong(t *testing.T) {
	s, f := New(t)
	f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID)
	below := &Folder{ID: "0199cccc-0000-7000-8000-000000000004", Title: "Below", ParentID: f.Folder.ID, CreatorID: f.Bruno.ID}
	f.Files.Folders = append(f.Files.Folders, below)
	f.Files.Arranged = map[string][]string{below.ID: {}}

	if got, _ := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-folders/"+f.Folder.ID+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":null}`); got != "200" {
		t.Fatalf("the move: %s", got)
	}
	if !arrangedByHand(f.OtherFiles, below.ID) || arrangedByHand(f.Files, below.ID) {
		t.Fatal("the order by hand of the folder below did not go along")
	}
	if arrangedByHand(f.OtherFiles, f.Folder.ID) {
		t.Fatal("the folder itself is arranged by hand outside a book")
	}
}

// A file shared with what stays keeps the entry in its project, held by the
// back of a card or by an earlier version of another upload.
func TestAFileSharedWithWhatStaysKeepsTheEntryInItsProject(t *testing.T) {
	for name, share := range map[string]func(s *Server, f *Fixture){
		"the back of a card": func(_ *Server, f *Fixture) {
			f.CardInTriage.BackFiles = append(f.CardInTriage.BackFiles, &CardBackFile{ID: newID(), Filename: "site-plan.pdf", Versions: []CardBackFileVersion{{BlobID: f.FilesUpload.BlobID, UploadedByID: f.Bruno.ID}}})
		},
		"an earlier version of the upload itself": func(s *Server, f *Fixture) {
			earlier := &Upload{ID: newID(), Circle: f.Project.ID, Filename: "old-plan.pdf", ContentType: "application/pdf", Content: []byte("%PDF"), UploaderID: f.Bruno.ID}
			s.Uploads = append(s.Uploads, earlier)
			f.FilesUpload.Versions = []*UploadVersion{{ID: newID(), BlobID: earlier.ID}, {ID: newID(), BlobID: f.FilesUpload.BlobID}}
			f.Files.Uploads = append(f.Files.Uploads, &FilesUpload{ID: newID(), Title: "old-plan.pdf", BlobID: earlier.ID})
		},
		"an earlier version of another upload": func(_ *Server, f *Fixture) {
			f.Files.Uploads = append(f.Files.Uploads, &FilesUpload{ID: newID(), Title: "plan.pdf", BlobID: newID(), Versions: []*UploadVersion{{ID: newID(), BlobID: f.FilesUpload.BlobID}}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, f := New(t)
			f.OtherProject.MemberIDs = append(f.OtherProject.MemberIDs, f.Bruno.ID)
			share(s, f)
			if got, _ := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-uploads/"+f.FilesUpload.ID+"/moves", `{"toolId":"`+f.OtherFiles.ID+`","folderId":null}`); got != "422 entrySharesFiles" {
				t.Fatalf("answered %s", got)
			}
		})
	}
}

// The entry's own tool named in capitals is a move within it: a label of the
// tool stays, and so does what a move into another tool would change.
func TestTheOwnToolInCapitalsChangesNothingButThePlace(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, f.Files.ID, "Draft", "grey")
	s.PutLabelOn(f.FilesUpload.ID, label)

	got, answer := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-uploads/"+f.FilesUpload.ID+"/moves", `{"toolId":"`+strings.ToUpper(f.Files.ID)+`","folderId":null}`)
	if got != "200" || answer["toolId"] != f.Files.ID || f.FilesUpload.FolderID != "" {
		t.Fatalf("%s %v", got, answer)
	}
	if kept := s.LabelsOn(f.FilesUpload.ID); len(kept) != 1 || kept[0] != label || f.FilesUpload.HiddenFromClients {
		t.Fatalf("labels %v, hidden %v", kept, f.FilesUpload.HiddenFromClients)
	}
}

// A Minka from before moves into another tool reads no toolId at all, of
// whatever type, and moves the entry within its own tool.
func TestAMinkaThatMovesWithinToolsOnlyReadsNoToolID(t *testing.T) {
	s, f := New(t)
	s.MovesWithinToolsOnly = true
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/documents/"+f.Document.ID+"/moves", `{"toolId":5,"folderId":"`+f.Folder.ID+`"}`); got != "200" || f.Document.FolderID != f.Folder.ID {
		t.Fatalf("answered %s", got)
	}
}
