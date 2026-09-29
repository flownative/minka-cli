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
