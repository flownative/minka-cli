package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// answerTo sends one request as the person of the token and says how it was
// answered: the status, and the code of a refusal behind it.
func answerTo(t *testing.T, s *Server, token, method, path, body string) (string, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	var answer map[string]any
	_ = json.Unmarshal(raw, &answer)
	got := fmt.Sprint(response.StatusCode)
	if refusal, ok := answer["error"].(map[string]any); ok && response.StatusCode != http.StatusNotFound {
		got += " " + fmt.Sprint(refusal["code"])
	}
	return got, answer
}

// A file is put into a tool in the order Minka refuses: the tool, the file,
// what the request says about the upload, and only then what the write meets.
// A file that is not the writer's own or stands somewhere already is learnt
// of last, after an archived project.
func TestAFileGoesIntoAToolInTheOrderMinkaRefuses(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepare func(s *Server, f *Fixture, file *Upload)
		body    string
		want    string
	}{
		{"no blob", nil, `{}`, "400 malformedBody"},
		{"a blob that is no identifier", nil, `{"blobId":"plan.pdf"}`, "400 malformedBody"},
		{"a swept blob in an archived project", func(s *Server, f *Fixture, file *Upload) {
			file.Swept = true
			f.Project.Archived = true
		}, `{"blobId":"BLOB","title":""}`, "422 projectArchived"},
		{"a swept blob", func(_ *Server, _ *Fixture, file *Upload) { file.Swept = true }, `{"blobId":"BLOB","title":""}`, "422 fileUnknown"},
		{"a blob of another project", func(_ *Server, f *Fixture, file *Upload) { file.Circle = f.OtherProject.ID }, `{"blobId":"BLOB","title":""}`, "422 fileUnknown"},
		{"an empty title", nil, `{"blobId":"BLOB","title":"  "}`, "422 uploadTitleMissing"},
		{"a folder in the archive", nil, `{"blobId":"BLOB","folderId":"ARCHIVED"}`, "422 folderArchived"},
		{"somebody else's blob in an archived project", func(_ *Server, f *Fixture, file *Upload) {
			file.UploaderID = f.Anke.ID
			f.Project.Archived = true
		}, `{"blobId":"BLOB","folderId":"ARCHIVED"}`, "422 folderArchived"},
		{"somebody else's blob", func(_ *Server, f *Fixture, file *Upload) { file.UploaderID = f.Anke.ID }, `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"an own blob in a text, in an archived project", func(_ *Server, f *Fixture, file *Upload) {
			file.StandsInAText = true
			f.Project.Archived = true
		}, `{"blobId":"BLOB"}`, "422 projectArchived"},
		{"an own blob in a text", func(_ *Server, _ *Fixture, file *Upload) { file.StandsInAText = true }, `{"blobId":"BLOB"}`, "422 fileInUse"},
		{"somebody else's blob in a text", func(_ *Server, f *Fixture, file *Upload) {
			file.StandsInAText = true
			file.UploaderID = f.Anke.ID
		}, `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"an own blob in an upload", func(s *Server, f *Fixture, file *Upload) {
			f.Files.Uploads = append(f.Files.Uploads, &FilesUpload{ID: newID(), Title: "twice", BlobID: file.ID})
		}, `{"blobId":"BLOB"}`, "422 fileInUse"},
		{"an own blob that stands nowhere", nil, `{"blobId":"BLOB","title":null,"description":null}`, "201"},
		{"an own blob named in capitals", nil, `{"blobId":"UP"}`, "201"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			file := &Upload{ID: newID(), Circle: f.Project.ID, Filename: "plan.pdf", ContentType: "application/pdf", Content: []byte("%PDF"), UploaderID: f.Bruno.ID}
			s.Uploads = append(s.Uploads, file)
			if c.prepare != nil {
				c.prepare(s, f, file)
			}
			body := strings.NewReplacer("BLOB", file.ID, "UP", strings.ToUpper(file.ID), "ARCHIVED", f.ArchivedFolder.ID).Replace(c.body)
			got, answer := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/uploads", body)
			if got != c.want {
				t.Fatalf("%s, want %s", got, c.want)
			}
			if got == "201" && (answer["title"] != "plan.pdf" || answer["description"] != "") {
				t.Fatalf("title %v, description %v: an upload without either is named after its file and described by nothing", answer["title"], answer["description"])
			}
		})
	}
}

// A change of an upload keeps its file whatever is sent, empties the
// description on null as on an empty string, and keeps what it is not given.
func TestAChangeOfAnUploadKeepsItsFile(t *testing.T) {
	s, f := New(t)
	upload := f.FilesUpload
	other := &Upload{ID: newID(), Circle: f.Project.ID, Filename: "other.pdf", ContentType: "application/pdf", Content: []byte("%PDF"), UploaderID: f.Bruno.ID}
	s.Uploads = append(s.Uploads, other)
	path := "/api/v1/files-uploads/" + upload.ID

	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"New","blobId":"`+other.ID+`"}`); got != "200" {
		t.Fatalf("a change with a blobId: %s", got)
	}
	if upload.BlobID == other.ID || upload.Title != "New" || upload.Description == "" {
		t.Fatalf("blob %s, title %q, description %q", upload.BlobID, upload.Title, upload.Description)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"description":null}`); got != "200" || upload.Description != "" {
		t.Fatalf("a description sent as null: %s, left %q", got, upload.Description)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":null}`); got != "200" || upload.Title != "New" {
		t.Fatalf("a title sent as null: %s, left %q", got, upload.Title)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":""}`); got != "422 uploadTitleMissing" {
		t.Fatalf("an empty title: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"New","expectedUpdatedAt":"2020-01-01T00:00:00.000000Z"}`); got != "200" {
		t.Fatalf("a change that changes nothing is no conflict, whatever it expects: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"Newer","expectedUpdatedAt":"2020-01-01T00:00:00.000000Z"}`); got != "409 uploadChanged" {
		t.Fatalf("a stale expectation: %s", got)
	}
	upload.Archived = true
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"","expectedUpdatedAt":"nonsense"}`); got != "422 uploadTitleMissing" {
		t.Fatalf("the title is read before the entry is asked whether it takes a change: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"Newer","expectedUpdatedAt":"nonsense"}`); got != "400 malformedBody" {
		t.Fatalf("the expectation is read before the entry is asked whether it takes a change: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"title":"Newer"}`); got != "422 uploadArchived" {
		t.Fatalf("an upload of the archive: %s", got)
	}
}

// Below an archived folder the comments of an entry are neither written,
// changed nor removed, and none of them is reacted to or has a reaction taken
// back: each answers with the code of the entry. An entry outside the folder
// keeps taking all of it.
func TestCommentsBelowAnArchivedFolderAreRefusedAsOnAnArchivedEntry(t *testing.T) {
	s, f := New(t)
	thumbsUp := func() []*Reaction { return []*Reaction{{Content: "👍", PersonIDs: []string{f.Bruno.ID}}} }
	upload := f.FilesUpload
	upload.FolderID = f.ArchivedFolder.ID
	upload.Comments[0].Reactions = thumbsUp()
	document := f.DocumentBelowTheArchivedFolder
	document.Comments = []*Comment{{ID: newID(), AuthorID: f.Bruno.ID, Body: "<p>Thanks</p>", CreatedAt: document.CreatedAt, Reactions: thumbsUp()}}

	for entry, c := range map[string]struct {
		comments *[]*Comment
		code     string
	}{
		"/api/v1/documents/" + document.ID:   {&document.Comments, "documentArchived"},
		"/api/v1/files-uploads/" + upload.ID: {&upload.Comments, "uploadArchived"},
	} {
		written := (*c.comments)[0]
		body := written.Body
		comment := entry + "/comments/" + written.ID
		for _, request := range []struct{ method, path, body string }{
			{http.MethodPost, entry + "/comments", `{"body":"More"}`},
			{http.MethodPatch, comment, `{"body":"Not after all"}`},
			{http.MethodDelete, comment, ``},
			{http.MethodPost, comment + "/reactions", `{"content":"🎉"}`},
			{http.MethodDelete, comment + "/reactions?content=%F0%9F%91%8D", ``},
		} {
			if got, _ := answerTo(t, s, f.Token, request.method, request.path, request.body); got != "422 "+c.code {
				t.Errorf("%s %s: %s", request.method, request.path, got)
			}
		}
		if len(*c.comments) != 1 || written.Body != body || written.EditedAt != nil ||
			len(written.Reactions) != 1 || len(written.Reactions[0].PersonIDs) != 1 {
			t.Errorf("%s: the comments changed: %d, %q, %d reactions", entry, len(*c.comments), written.Body, len(written.Reactions))
		}
	}

	outside := "/api/v1/documents/" + f.Document.ID + "/comments/"
	mine := &Comment{ID: newID(), AuthorID: f.Bruno.ID, Body: "<p>Good</p>", CreatedAt: f.Document.CreatedAt}
	f.Document.Comments = append(f.Document.Comments, mine)
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, outside+mine.ID, `{"body":"Very good"}`); got != "200" {
		t.Errorf("a change outside the folder: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, outside+mine.ID, ``); got != "204" {
		t.Errorf("a removal outside the folder: %s", got)
	}
}

// Under an archived entry a comment is asked for in the order of the
// controller: a body that is no JSON first, then the address with the comment
// under it, then what the request says, then the archive, which answers for
// somebody else's comment as well. A comment never written is not found
// before anything the request says is looked at.
func TestACommentUnderAnArchivedEntryIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	s, f := New(t)
	document := f.DocumentBelowTheArchivedFolder
	others := &Comment{ID: newID(), AuthorID: f.Anke.ID, Body: "<p>Mine</p>", CreatedAt: document.CreatedAt}
	document.Comments = []*Comment{others}
	entry := "/api/v1/documents/" + document.ID + "/comments/"

	for _, c := range []struct{ name, method, path, body, want string }{
		{"a change of an ID that is none", http.MethodPatch, entry + "nonsense", `{"body":""}`, "404"},
		{"a change that is no JSON of a comment never written", http.MethodPatch, entry + newID(), `{`, "400 malformedBody"},
		{"an empty change of a comment never written", http.MethodPatch, entry + newID(), `{"body":""}`, "404"},
		{"an empty change", http.MethodPatch, entry + others.ID, `{"body":""}`, "422 commentMissing"},
		{"a change of somebody else's comment", http.MethodPatch, entry + others.ID, `{"body":"Mine now"}`, "422 documentArchived"},
		{"a change of a comment never written", http.MethodPatch, entry + newID(), `{"body":"Mine now"}`, "404"},
		{"a removal of somebody else's comment", http.MethodDelete, entry + others.ID, ``, "422 documentArchived"},
		{"a removal of a comment never written", http.MethodDelete, entry + newID(), ``, "404"},
		{"a reaction that is no JSON on an entry never written", http.MethodPost, "/api/v1/documents/" + newID() + "/comments/" + newID() + "/reactions", `{`, "400 malformedBody"},
		{"a reaction on an ID that is none", http.MethodPost, entry + "nonsense/reactions", `{"content":""}`, "404"},
		{"an empty reaction on a comment never written", http.MethodPost, entry + newID() + "/reactions", `{"content":""}`, "404"},
		{"an empty reaction", http.MethodPost, entry + others.ID + "/reactions", `{"content":""}`, "422 reactionMissing"},
		{"a reaction on a comment never written", http.MethodPost, entry + newID() + "/reactions", `{"content":"🎉"}`, "404"},
		{"a reaction", http.MethodPost, entry + others.ID + "/reactions", `{"content":"🎉"}`, "422 documentArchived"},
		{"an empty reaction taken back from a comment never written", http.MethodDelete, entry + newID() + "/reactions?content=", ``, "404"},
		{"an empty reaction taken back", http.MethodDelete, entry + others.ID + "/reactions?content=", ``, "422 reactionMissing"},
		{"a reaction taken back from a comment never written", http.MethodDelete, entry + newID() + "/reactions?content=x", ``, "404"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	if others.Body != "<p>Mine</p>" || len(document.Comments) != 1 {
		t.Errorf("the comment changed: %q, %d", others.Body, len(document.Comments))
	}
}

// An upload without a title is named after its file, and a name too long for
// a title is cut before its extension, which stays.
func TestAnUploadIsNamedAfterItsFileWithinTheLengthOfATitle(t *testing.T) {
	for filename, want := range map[string]string{
		"plan.pdf":                           "plan.pdf",
		strings.Repeat("a", 250) + ".pdf":    strings.Repeat("a", 196) + ".pdf",
		strings.Repeat("a", 250) + ".tar-gz": strings.Repeat("a", 200),
		strings.Repeat("ä", 250) + ".pdf":    strings.Repeat("ä", 196) + ".pdf",
		// The cut leaves a space at the end, which the trim of a title takes.
		strings.Repeat("a", 199) + " " + strings.Repeat("b", 50): strings.Repeat("a", 199),
	} {
		s, f := New(t)
		file := &Upload{ID: newID(), Circle: f.Project.ID, Filename: filename, ContentType: "application/pdf", Content: []byte("%PDF"), UploaderID: f.Bruno.ID}
		s.Uploads = append(s.Uploads, file)
		got, answer := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/files-tools/"+f.Files.ID+"/uploads", `{"blobId":"`+file.ID+`"}`)
		if got != "201" || answer["title"] != want {
			t.Errorf("%d characters: %s, title %v", len(filename), got, answer["title"])
		}
	}
}

// A refused title names the entry with its article, as Minka's messages do.
func TestATitleRefusalNamesTheEntryWithItsArticle(t *testing.T) {
	s, f := New(t)
	upload, document := "/api/v1/files-uploads/"+f.FilesUpload.ID, "/api/v1/documents/"+f.Document.ID
	long := strings.Repeat("a", maxTitleLength+1)
	for _, c := range []struct{ path, body, want string }{
		{upload, `{"title":""}`, "An upload needs a title."},
		{upload, `{"title":"` + long + `"}`, "The title of an upload is too long."},
		{upload, `{"title":"a\u0007b"}`, "The title of an upload must not contain control characters."},
		{document, `{"title":""}`, "A document needs a title."},
		{document, `{"title":"` + long + `"}`, "The title of a document is too long."},
	} {
		_, answer := answerTo(t, s, f.Token, http.MethodPatch, c.path, c.body)
		if message := answer["error"].(map[string]any)["message"]; message != c.want {
			t.Errorf("%s %s: %v, want %q", c.path, c.body, message, c.want)
		}
	}
}
