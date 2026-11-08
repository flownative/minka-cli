package fakeapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// aStoredFile is a file of Bruno's in the project of the fixture that stands
// nowhere yet.
func aStoredFile(s *Server, f *Fixture) *Upload {
	file := &Upload{ID: newID(), Circle: f.Project.ID, Filename: "plan-v2.pdf", ContentType: "application/pdf",
		Content: []byte("%PDF-1.4 the second plan"), UploaderID: f.Bruno.ID}
	s.Uploads = append(s.Uploads, file)
	return file
}

func listOf(t *testing.T, s *Server, token, path string) []map[string]any {
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
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s answered %d", path, response.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	return list
}

// A document written before versions were kept has one version naming no
// author; every write after it adds one, oldest first, the newest current.
func TestTheVersionsOfADocumentAreListedOldestFirst(t *testing.T) {
	s, f := New(t)
	versions := "/api/v1/documents/" + f.Document.ID + "/versions"

	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/documents/"+f.Document.ID, `{"title":"Up it goes","body":"<p>Nine people.</p>"}`); got != "200" {
		t.Fatalf("the edit answered %s", got)
	}
	list := listOf(t, s, f.Token, versions)
	if len(list) != 2 {
		t.Fatalf("%d versions", len(list))
	}
	first, second := list[0], list[1]
	if first["number"] != 1.0 || first["current"] != false || first["author"] != nil || first["documentId"] != f.Document.ID {
		t.Fatalf("first %v", first)
	}
	author, _ := second["author"].(map[string]any)
	if second["number"] != 2.0 || second["current"] != true || author["id"] != f.Bruno.ID {
		t.Fatalf("second %v", second)
	}

	got, earlier := answerTo(t, s, f.Token, http.MethodGet, versions+"/"+first["id"].(string), "")
	if got != "200" || earlier["title"] != "How the stage goes up" || earlier["body"] != "<p>Eight people on <strong>Friday</strong>.</p>" || earlier["current"] != false {
		t.Fatalf("%s %v", got, earlier)
	}
	if !strings.HasSuffix(earlier["appUrl"].(string), "/history/"+first["id"].(string)) {
		t.Fatalf("appUrl %v", earlier["appUrl"])
	}

	// An unchanged save keeps no version.
	answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/documents/"+f.Document.ID, `{"title":"Up it goes"}`)
	if n := len(listOf(t, s, f.Token, versions)); n != 2 {
		t.Fatalf("an unchanged save left %d versions", n)
	}
}

// The versions are the team's alone, and are where the document and the
// upload are: in the archive and in an archived project as well.
func TestTheVersionsAreTheTeamsAlone(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.Folder.ShownToClients = true
	f.Project.Archived = true
	document := "/api/v1/documents/" + f.DocumentInFolder.ID
	version := listOf(t, s, f.Token, document+"/versions")[0]["id"].(string)
	upload := "/api/v1/files-uploads/" + f.FilesUpload.ID

	for _, c := range []struct {
		name, token, path, want string
	}{
		{"the team, a document", f.Token, document + "/versions", "200"},
		{"the team, a version", f.Token, document + "/versions/" + version, "200"},
		{"the team, an upload", f.Token, upload + "/versions", "200"},
		{"the team, a document in the archive", f.Token, "/api/v1/documents/" + f.ArchivedDocument.ID + "/versions", "200"},
		{"the team, below an archived folder", f.Token, "/api/v1/documents/" + f.DocumentBelowTheArchivedFolder.ID + "/versions", "200"},
		{"a client, the document itself", "minka_pat_client", document, "200"},
		{"a client, a document", "minka_pat_client", document + "/versions", "404"},
		{"a client, a version", "minka_pat_client", document + "/versions/" + version, "404"},
		{"a client, an upload", "minka_pat_client", upload + "/versions", "404"},
		{"an outsider", f.StrangerToken, document + "/versions", "404"},
		{"a draft", f.Token, "/api/v1/documents/" + f.DraftDocument.ID + "/versions", "404"},
		{"a version of another document", f.Token, "/api/v1/documents/" + f.Document.ID + "/versions/" + version, "404"},
		{"a version that is no identifier", f.Token, document + "/versions/first", "404"},
		{"a version in capitals", f.Token, document + "/versions/" + strings.ToUpper(version), "200"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := answerTo(t, s, c.token, http.MethodGet, c.path, ""); got != c.want {
				t.Fatalf("answered %s, want %s", got, c.want)
			}
		})
	}
}

// Another file for an upload is taken in the order Minka refuses: the body,
// the upload, the client, the expectation, the file as the project knows it,
// and then what the write meets.
func TestAnUploadIsGivenAnotherFileInTheOrderMinkaRefuses(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepare func(s *Server, f *Fixture, file *Upload)
		token   string
		body    string
		want    string
	}{
		{"a body that is no JSON", nil, "", `{`, "400 malformedBody"},
		{"a client", nil, "minka_pat_client", `{}`, "404"},
		{"a client in an archived project", func(_ *Server, f *Fixture, _ *Upload) { f.Project.Archived = true }, "minka_pat_client", `{"blobId":"BLOB"}`, "404"},
		{"an expectation that is no moment", nil, "", `{"blobId":"BLOB","expectedUpdatedAt":"yesterday"}`, "400 malformedBody"},
		{"no blob", nil, "", `{}`, "400 malformedBody"},
		{"a blob that is no identifier", nil, "", `{"blobId":"plan.pdf"}`, "400 malformedBody"},
		{"a swept blob in an archived project", func(_ *Server, f *Fixture, file *Upload) {
			file.Swept = true
			f.Project.Archived = true
		}, "", `{"blobId":"BLOB"}`, "422 projectArchived"},
		{"a swept blob", func(_ *Server, _ *Fixture, file *Upload) { file.Swept = true }, "", `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"a blob of another project", func(_ *Server, f *Fixture, file *Upload) { file.Circle = f.OtherProject.ID }, "", `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"an archived project", func(_ *Server, f *Fixture, _ *Upload) { f.Project.Archived = true }, "", `{"blobId":"BLOB","expectedUpdatedAt":"2000-01-01T00:00:00.000000Z"}`, "422 projectArchived"},
		{"an archived upload in an archived project", func(_ *Server, f *Fixture, _ *Upload) {
			f.FilesUpload.Archived = true
			f.Project.Archived = true
		}, "", `{"blobId":"BLOB"}`, "422 projectArchived"},
		{"an archived upload", func(_ *Server, f *Fixture, _ *Upload) { f.FilesUpload.Archived = true }, "", `{"blobId":"BLOB","expectedUpdatedAt":"2000-01-01T00:00:00.000000Z"}`, "422 uploadArchived"},
		{"below an archived folder", func(_ *Server, f *Fixture, _ *Upload) { f.Folder.Archived = true }, "", `{"blobId":"BLOB"}`, "422 uploadArchived"},
		{"the file it shows, against an old expectation", nil, "", `{"blobId":"CURRENT","expectedUpdatedAt":"2000-01-01T00:00:00.000000Z"}`, "200"},
		{"an old expectation, somebody else's file", func(_ *Server, f *Fixture, file *Upload) { file.UploaderID = f.Anke.ID }, "", `{"blobId":"BLOB","expectedUpdatedAt":"2000-01-01T00:00:00.000000Z"}`, "409 uploadChanged"},
		{"somebody else's file", func(_ *Server, f *Fixture, file *Upload) { file.UploaderID = f.Anke.ID }, "", `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"somebody else's file in use", func(_ *Server, f *Fixture, file *Upload) {
			file.UploaderID = f.Anke.ID
			file.StandsInAText = true
		}, "", `{"blobId":"BLOB"}`, "422 fileUnknown"},
		{"a file of one's own in a text", func(_ *Server, _ *Fixture, file *Upload) { file.StandsInAText = true }, "", `{"blobId":"BLOB"}`, "422 fileInUse"},
		{"a file of one's own", nil, "", `{"blobId":"BLOB"}`, "201"},
		{"in capitals", nil, "", `{"blobId":"UPPER"}`, "201"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := New(t)
			s.Tokens["minka_pat_client"] = f.Client.ID
			file := aStoredFile(s, f)
			if c.prepare != nil {
				c.prepare(s, f, file)
			}
			token := c.token
			if token == "" {
				token = f.Token
			}
			body := strings.NewReplacer("UPPER", strings.ToUpper(file.ID), "BLOB", file.ID, "CURRENT", f.FilesUpload.BlobID).Replace(c.body)
			got, answer := answerTo(t, s, token, http.MethodPost, "/api/v1/files-uploads/"+f.FilesUpload.ID+"/versions", body)
			if got != c.want {
				t.Fatalf("answered %s, want %s", got, c.want)
			}
			if c.want == "409 uploadChanged" && answer["upload"] == nil {
				t.Fatal("the conflict does not carry the upload")
			}
			written := f.FilesUpload.BlobID == file.ID
			if written != (c.want == "201") {
				t.Fatalf("the upload shows %s after %s", f.FilesUpload.BlobID, got)
			}
		})
	}
}

// The file an upload showed stays as an earlier version: listed, still
// fetched by the team, hidden from a client, and in use, so naming it again
// is refused until its bytes are stored anew.
func TestAnEarlierFileStaysWithTheUpload(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	earlier := f.FilesUpload.BlobID
	title, description := f.FilesUpload.Title, f.FilesUpload.Description
	file := aStoredFile(s, f)
	path := "/api/v1/files-uploads/" + f.FilesUpload.ID + "/versions"

	got, upload := answerTo(t, s, f.Token, http.MethodPost, path, `{"blobId":"`+file.ID+`"}`)
	if got != "201" || upload["title"] != title || upload["description"] != description {
		t.Fatalf("%s %v", got, upload)
	}
	if shown, _ := upload["file"].(map[string]any); shown["id"] != file.ID {
		t.Fatalf("file %v", upload["file"])
	}

	list := listOf(t, s, f.Token, path)
	if len(list) != 2 {
		t.Fatalf("%d versions", len(list))
	}
	firstFile, _ := list[0]["file"].(map[string]any)
	firstAuthor, _ := list[0]["author"].(map[string]any)
	if list[0]["number"] != 1.0 || list[0]["current"] != false || firstFile["id"] != earlier || firstAuthor["id"] != f.Anke.ID {
		t.Fatalf("first %v", list[0])
	}
	secondFile, _ := list[1]["file"].(map[string]any)
	secondAuthor, _ := list[1]["author"].(map[string]any)
	if list[1]["number"] != 2.0 || list[1]["current"] != true || secondFile["id"] != file.ID || secondAuthor["id"] != f.Bruno.ID {
		t.Fatalf("second %v", list[1])
	}

	if got, _ := answerTo(t, s, f.Token, http.MethodPost, path, `{"blobId":"`+file.ID+`"}`); got != "200" {
		t.Fatalf("the file it shows already answered %s", got)
	}
	f.FilesUpload.Versions[0].AuthorID = f.Bruno.ID
	for _, blob := range s.Uploads {
		if blob.ID == earlier {
			blob.UploaderID = f.Bruno.ID
		}
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, path, `{"blobId":"`+earlier+`"}`); got != "422 fileInUse" {
		t.Fatalf("the earlier file answered %s", got)
	}
	if n := len(listOf(t, s, f.Token, path)); n != 2 {
		t.Fatalf("%d versions after the refusals", n)
	}

	for _, c := range []struct{ token, want string }{{f.Token, "200"}, {"minka_pat_client", "404"}} {
		if got, _ := answerTo(t, s, c.token, http.MethodGet, "/api/v1/uploads/"+earlier, ""); got != c.want {
			t.Errorf("the earlier file answered %s, want %s", got, c.want)
		}
	}
	if got, _ := answerTo(t, s, "minka_pat_client", http.MethodGet, "/api/v1/uploads/"+file.ID, ""); got != "200" {
		t.Errorf("the file the upload shows answered the client %s", got)
	}

	// Whoever stored a file fetches it, a client too, also once it is an
	// earlier file of an upload.
	for _, blob := range s.Uploads {
		if blob.ID == earlier {
			blob.UploaderID = f.Client.ID
		}
	}
	if got, _ := answerTo(t, s, "minka_pat_client", http.MethodGet, "/api/v1/uploads/"+earlier, ""); got != "200" {
		t.Errorf("the earlier file answered the client who stored it %s", got)
	}
}
