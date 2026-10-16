package cli

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

func uploadVersionsPath(e *testEnv) string {
	return "/api/v1/files-uploads/" + e.fixture.FilesUpload.ID + "/versions"
}

// replaceFile runs docs uploads versions create with a file of this machine,
// for the upload of the fixture.
func replaceFile(e *testEnv, rest ...string) []string {
	return slices.Concat([]string{"docs", "uploads", "versions", "create", e.fixture.FilesUpload.ID, aFileToUpload(e)}, rest)
}

// The versions of a document are listed oldest first, the one written before
// versions were kept without an author, and each is shown with its body as
// Markdown in text and as the stored HTML in JSON.
func TestTheVersionsOfADocumentAreListedAndShown(t *testing.T) {
	e := newTestEnv(t)
	document := e.fixture.Document.ID
	runOK(t, e, "docs", "edit", document, "--title", "Up it goes", "--force")

	env := runOK(t, e, "docs", "versions", "list", document)
	versions := listOfObjects(t, env.Data)
	if len(versions) != 2 || versions[0]["author"] != nil || versions[1]["current"] != true {
		t.Fatalf("versions %v", versions)
	}
	first := versions[0]["id"].(string)
	if env.Summary != "2 versions of the document" || !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool {
		return b.Cmd == "minka docs versions show "+document+" "+first
	}) {
		t.Fatalf("summary %q, breadcrumbs %v", env.Summary, env.Breadcrumbs)
	}
	listed := e.run("", "docs", "versions", "list", document, "--styled")
	if !strings.Contains(listed.stdout, "(not kept)") || !strings.Contains(listed.stdout, "current") {
		t.Fatalf("text output:\n%s", listed.stdout)
	}

	shown := decode(t, runOK(t, e, "docs", "versions", "show", document, strings.ToUpper(first)).Data)
	if shown["title"] != "How the stage goes up" || shown["body"] != "<p>Eight people on <strong>Friday</strong>.</p>" {
		t.Fatalf("version %v", shown)
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Path != "/api/v1/documents/"+document+"/versions/"+first {
		t.Fatalf("asked %s", last.Path)
	}
	text := e.run("", "docs", "versions", "show", document, first, "--styled")
	if !strings.Contains(text.stdout, "Eight people on **Friday**.") || !strings.Contains(text.stdout, "Version: 1\n") {
		t.Fatalf("text output:\n%s", text.stdout)
	}
}

// A version hands out its body as Minka stores it, where a file carries no
// name, so the file shows in the list of files the body names and nowhere
// else.
func TestTheFilesOfAVersionAreListedBesideItsBody(t *testing.T) {
	e := newTestEnv(t)
	document := e.fixture.Document
	file := storedFile(e)
	tag := `<action-text-attachment sgid="` + file.Handle() + `" content-type="text/plain" url="` + e.server.URL +
		`/x" filename="notes.txt" filesize="13" presentation="gallery"></action-text-attachment>`
	document.Versions = []*fakeapi.DocumentVersion{{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f0", Title: document.Title,
		Body: "<p>See the notes</p>" + tag, WrittenAt: document.UpdatedAt}}

	shown := decode(t, runOK(t, e, "docs", "versions", "show", document.ID, document.Versions[0].ID).Data)
	if body := shown["body"].(string); strings.Contains(body, "filename=") || !strings.Contains(body, `sgid="`+file.Handle()+`"`) {
		t.Fatalf("body %q is not the stored form", body)
	}
	attachments, _ := shown["attachments"].([]any)
	if len(attachments) != 1 || attachments[0].(map[string]any)["id"] != file.ID {
		t.Fatalf("attachments %v", shown["attachments"])
	}

	text := e.run("", "docs", "versions", "show", document.ID, document.Versions[0].ID, "--styled")
	if !strings.Contains(text.stdout, "## Files (1)") || !strings.Contains(text.stdout, file.ID+"  notes.txt") {
		t.Fatalf("text output:\n%s", text.stdout)
	}
}

// The versions are the team's alone: a client of the project is answered as
// somebody outside it, also for a document and an upload it is shown, and the
// hint says so.
func TestAClientReachesNoVersion(t *testing.T) {
	e := newTestEnv(t)
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
	versionID := listOfObjects(t, runOK(t, e, "docs", "versions", "list", e.fixture.Document.ID).Data)[0]["id"].(string)
	e.vars["MINKA_TOKEN"] = "minka_pat_client"

	runOK(t, e, "docs", "show", e.fixture.Document.ID)
	for name, args := range map[string][]string{
		"the versions of a document": {"docs", "versions", "list", e.fixture.Document.ID},
		"a version":                  {"docs", "versions", "show", e.fixture.Document.ID, versionID},
		"the files of an upload":     {"docs", "uploads", "versions", "list", e.fixture.FilesUpload.ID},
	} {
		t.Run(name, func(t *testing.T) {
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitNotFound)
			if !strings.Contains(env.Hint, "the team's alone") {
				t.Fatalf("hint %q", env.Hint)
			}
		})
	}
}

// A file of this machine replaces the file of an upload in one run: the upload
// is read for its project and its versions, the file stored there and given to
// the upload, whose earlier file stays as a version.
func TestAnUploadIsGivenAnotherFileInOneRun(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload
	earlier := upload.BlobID

	env := runOK(t, e, replaceFile(e)...)
	answer := decode(t, env.Data)
	file := decode(t, rawOf(t, answer, "file"))
	if file["filename"] != "notes.txt" || file["id"] == earlier || answer["title"] != "site-plan.pdf" {
		t.Fatalf("upload %v", answer)
	}
	if !strings.HasPrefix(env.Summary, "Gave upload site-plan.pdf the file notes.txt") || env.Notice != "" {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}

	var sent []string
	for _, request := range e.server.Requests() {
		sent = append(sent, request.Method+" "+request.Path)
	}
	want := []string{"GET /api/v1/files-uploads/" + upload.ID, "GET " + uploadVersionsPath(e), "POST " + projectUploads(e), "POST " + uploadVersionsPath(e)}
	if !slices.Equal(sent, want) {
		t.Fatalf("requests %v, want %v", sent, want)
	}
	if body := e.lastWriteBody(); body["blobId"] != file["id"] || len(body) != 1 {
		t.Fatalf("body %v", body)
	}

	versions := listOfObjects(t, runOK(t, e, "docs", "uploads", "versions", "list", upload.ID).Data)
	if len(versions) != 2 || decode(t, rawOf(t, versions[0], "file"))["id"] != earlier || versions[1]["current"] != true {
		t.Fatalf("versions %v", versions)
	}
	listed := e.run("", "docs", "uploads", "versions", "list", upload.ID, "--styled")
	if !strings.Contains(listed.stdout, earlier) || !strings.Contains(listed.stdout, "site-plan.pdf") {
		t.Fatalf("text output:\n%s", listed.stdout)
	}
}

// What the reads before the transfer refuse costs no transfer: an upload that
// is at no address, and one a client may read but not give a file.
func TestNoFileTravelsForAnUploadThatCannotTakeIt(t *testing.T) {
	for name, c := range map[string]struct {
		token string
		hint  string
	}{
		"an outsider": {"minka_pat_stranger", uploadNotFoundHint},
		"a client":    {"minka_pat_client", uploadVersionsNotFoundHint},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
			e.vars["MINKA_TOKEN"] = c.token
			env, r := e.runJSON("", replaceFile(e)...)
			expectCode(t, r, ExitNotFound)
			if env.Hint != c.hint {
				t.Fatalf("hint %q", env.Hint)
			}
			if writes := e.server.WritingRequests(); len(writes) != 0 {
				t.Fatalf("%d writes went out, the first to %s", len(writes), writes[0].Path)
			}
		})
	}
}

// An upload in the archive, or below an archived folder, takes no file, and the
// read before the transfer says so: no file is stored to stand in nothing.
func TestNoFileTravelsForAnUploadInTheArchive(t *testing.T) {
	for name, archive := range map[string]func(e *testEnv){
		"the upload":        func(e *testEnv) { e.fixture.FilesUpload.Archived = true },
		"a folder above it": func(e *testEnv) { e.fixture.Folder.Archived = true },
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			archive(e)
			env, r := e.runJSON("", replaceFile(e)...)
			expectCode(t, r, ExitValidation)
			if !strings.Contains(env.Error, "lies in the archive") || !strings.Contains(env.Hint, "on the web") {
				t.Fatalf("error %q, hint %q", env.Error, env.Hint)
			}
			if writes := e.server.WritingRequests(); len(writes) != 0 {
				t.Fatalf("%d writes went out, the first to %s", len(writes), writes[0].Path)
			}
		})
	}
}

// Where the file was stored and Minka refused to give it to the upload, the
// error hands over the stored file, and --blob gives it without sending it
// again.
func TestAStoredFileIsHandedOverWhereTheUploadRefusedIt(t *testing.T) {
	e := newTestEnv(t)
	// The upload goes into the archive while the file travels, after the
	// read that would have stopped the run.
	e.server.AfterStoring = func(*fakeapi.Upload) { e.fixture.FilesUpload.Archived = true }

	env, r := e.runJSON("", replaceFile(e)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeUploadArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	stored := blobID(t, decode(t, env.Data))
	if !strings.Contains(env.Hint, "--blob "+stored) || !strings.Contains(env.Hint, "at least seven days") {
		t.Fatalf("hint %q does not say how to try again without sending the file", env.Hint)
	}

	e.server.AfterStoring = nil
	e.fixture.FilesUpload.Archived = false
	runOK(t, e, "docs", "uploads", "versions", "create", e.fixture.FilesUpload.ID, "--blob", stored)
	if requestsTo(e, http.MethodPost, projectUploads(e)) != 1 || e.fixture.FilesUpload.BlobID != stored {
		t.Fatalf("the file was sent a second time, or not given: %s", e.fixture.FilesUpload.BlobID)
	}
}

// The file the upload shows already writes nothing, and the run says so
// rather than reporting a new version.
func TestTheFileTheUploadShowsWritesNothing(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload

	env := runOK(t, e, "docs", "uploads", "versions", "create", upload.ID, "--blob", upload.BlobID)
	if !strings.Contains(env.Notice, "shows this file already") || !strings.Contains(env.Summary, "nothing was written") {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}
	if n := len(listOfObjects(t, runOK(t, e, "docs", "uploads", "versions", "list", upload.ID).Data)); n != 1 {
		t.Fatalf("%d versions", n)
	}
}

// An earlier file of the upload stands in it already, so it comes back only
// with its bytes stored anew, which the hint says.
func TestAnEarlierFileComesBackOnlyStoredAnew(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload
	earlier := upload.BlobID
	for _, file := range e.server.Uploads {
		if file.ID == earlier {
			file.UploaderID = e.fixture.Bruno.ID
		}
	}
	runOK(t, e, replaceFile(e)...)

	env, r := e.runJSON("", "docs", "uploads", "versions", "create", upload.ID, "--blob", earlier)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFileInUse || !strings.Contains(env.Hint, "earlier file of this same upload") || !strings.Contains(env.Hint, "store its bytes anew") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	storedFile(e).Swept = true
	env, r = e.runJSON("", "docs", "uploads", "versions", "create", upload.ID, "--blob", storedBlobID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFileUnknown || !strings.Contains(env.Hint, "store it again") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// --expect refuses a file for an upload somebody wrote over since it was read,
// hands over the upload as it stands, and still names the file this run
// stored.
func TestAFileIsGivenOnlyToAnUploadAsItWasRead(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload
	updatedAt := decode(t, runOK(t, e, "docs", "uploads", "show", upload.ID).Data)["updatedAt"].(string)
	runOK(t, e, "docs", "uploads", "edit", upload.ID, "--title", "Site plan", "--force")

	env, r := e.runJSON("", replaceFile(e, "--expect", updatedAt)...)
	expectCode(t, r, ExitValidation)
	current := decode(t, env.Data)
	if env.Meta["apiCode"] != apiCodeUploadChanged || current["title"] != "Site plan" {
		t.Fatalf("apiCode %v, data %v", env.Meta["apiCode"], current)
	}
	if e.lastWriteBody()["expectedUpdatedAt"] != updatedAt {
		t.Fatalf("body %v", e.lastWriteBody())
	}
	stored := ""
	for _, file := range e.server.Uploads {
		if file.Filename == "notes.txt" {
			stored = file.ID
		}
	}
	if !strings.Contains(env.Hint, "--expect "+current["updatedAt"].(string)) || !strings.Contains(env.Hint, "--blob "+stored) {
		t.Fatalf("hint %q", env.Hint)
	}

	written := runOK(t, e, "docs", "uploads", "versions", "create", upload.ID, "--blob", stored, "--expect", current["updatedAt"].(string))
	if !strings.Contains(written.Notice, "whole second") {
		t.Fatalf("notice %q", written.Notice)
	}
}

// What the command cannot send never leaves it.
func TestAReplacementThatCannotBeSentExitsBeforeAnyRequest(t *testing.T) {
	upload := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	for name, args := range map[string][]string{
		"neither":                 {"docs", "uploads", "versions", "create", upload},
		"both":                    {"docs", "uploads", "versions", "create", upload, "notes.txt", "--blob", storedBlobID},
		"three arguments":         {"docs", "uploads", "versions", "create", upload, "notes.txt", "more.txt"},
		"blob with name":          {"docs", "uploads", "versions", "create", upload, "--blob", storedBlobID, "--name", "x.txt"},
		"blob with content-type":  {"docs", "uploads", "versions", "create", upload, "--blob", storedBlobID, "--content-type", "text/plain"},
		"expect none":             {"docs", "uploads", "versions", "create", upload, "--blob", storedBlobID, "--expect", "none"},
		"expect without a zone":   {"docs", "uploads", "versions", "create", upload, "--blob", storedBlobID, "--expect", "2026-09-26T14:03:12"},
		"an upload that is no ID": {"docs", "uploads", "versions", "create", "site-plan", "--blob", storedBlobID},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.Requests()) != 0 {
				t.Fatal("a request went out")
			}
		})
	}
}

// --expect none is refused with a hint that names only the flags the command
// has: a replacement writes without an expectation by leaving --expect out,
// and has no --force.
func TestExpectNoneNamesOnlyTheFlagsTheCommandHas(t *testing.T) {
	upload := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	for name, c := range map[string]struct {
		args      []string
		withForce bool
	}{
		"a replacement": {[]string{"docs", "uploads", "versions", "create", upload, "--blob", storedBlobID, "--expect", "none"}, false},
		"an edit":       {[]string{"docs", "uploads", "edit", upload, "--title", "New", "--expect", "none"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", c.args...)
			expectCode(t, r, ExitUsage)
			if strings.Contains(env.Hint, "--force") != c.withForce {
				t.Fatalf("hint %q", env.Hint)
			}
			if !strings.Contains(env.Error, "an upload always has an updatedAt") {
				t.Fatalf("error %q", env.Error)
			}
		})
	}
}
