package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// putFile runs docs uploads create with a file of this machine, into the tool
// of the fixture.
func putFile(e *testEnv, rest ...string) []string {
	return slices.Concat([]string{"docs", "uploads", "create", aFileToUpload(e), "--tool", e.fixture.Files.ID}, rest)
}

// storedFile is a file of Bruno's in the project of the fixture that stands
// nowhere yet, as "uploads create --project" leaves one.
func storedFile(e *testEnv) *fakeapi.Upload {
	file := &fakeapi.Upload{ID: storedBlobID, Circle: e.fixture.Project.ID, Filename: "notes.txt", ContentType: "text/plain",
		Content: []byte("Four hundred."), Size: 13, UploaderID: e.fixture.Bruno.ID}
	e.server.Uploads = append(e.server.Uploads, file)
	return file
}

// requestsTo counts what went out to one address with one method.
func requestsTo(e *testEnv, method, path string) int {
	count := 0
	for _, request := range e.server.Requests() {
		if request.Method == method && request.Path == path {
			count++
		}
	}
	return count
}

func toolUploads(e *testEnv) string {
	return "/api/v1/files-tools/" + e.fixture.Files.ID + "/uploads"
}

func projectUploads(e *testEnv) string {
	return "/api/v1/projects/" + e.fixture.Project.ID + "/uploads"
}

// A file of this machine goes in with both requests in one run: the tool is
// read for its project, the file stored there, and then put into the tool, on
// top of it and named after the file.
func TestAFileIsPutIntoAToolInOneRun(t *testing.T) {
	e := newTestEnv(t)

	env := runOK(t, e, putFile(e)...)
	upload := decode(t, env.Data)
	if upload["title"] != "notes.txt" || upload["toolId"] != e.fixture.Files.ID {
		t.Fatalf("upload %v", upload)
	}
	file := decode(t, rawOf(t, upload, "file"))
	if file["filename"] != "notes.txt" || file["id"] == "" {
		t.Fatalf("the upload does not name the file it stands for: %v", file)
	}

	var sent []string
	for _, request := range e.server.Requests() {
		sent = append(sent, request.Method+" "+request.Path)
	}
	want := []string{"GET /api/v1/files-tools/" + e.fixture.Files.ID, "POST " + projectUploads(e), "POST " + toolUploads(e)}
	if !slices.Equal(sent, want) {
		t.Fatalf("requests %v, want %v", sent, want)
	}
	body := e.lastWriteBody()
	if _, named := body["title"]; named {
		t.Fatalf("a title nobody gave was sent: %v", body)
	}
	if body["blobId"] != file["id"] {
		t.Fatalf("blobId %v, want the file just stored, %v", body["blobId"], file["id"])
	}
	if top := idsOf(t, decode(t, runOK(t, e, "docs", "list", e.fixture.Files.ID).Data), "uploads"); len(top) != 1 || top[0] != upload["id"] {
		t.Fatalf("the uploads at the top of the tool are %v", top)
	}
}

// Minka names an upload without a title after its file and refuses an empty
// one, the latter only after the file travelled. So an empty title is refused
// here before anything goes out, and one that is left out is not sent.
func TestAnEmptyTitleIsRefusedBeforeTheFileTravels(t *testing.T) {
	for _, title := range []string{"", "  \t", "\x0b", "\x00 "} {
		e := newTestEnv(t)
		env, r := e.runJSON("", putFile(e, "--title", title)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Hint, "Leave --title out") {
			t.Fatalf("hint %q", env.Hint)
		}
		if sent := e.server.Requests(); len(sent) != 0 {
			t.Fatalf("%q: %d requests went out, the first to %s", title, len(sent), sent[0].Path)
		}
	}

	e := newTestEnv(t)
	named := decode(t, runOK(t, e, putFile(e, "--title", "The plan")...).Data)
	if named["title"] != "The plan" || e.lastWriteBody()["title"] != "The plan" {
		t.Fatalf("title %v", named["title"])
	}
}

// A title is replaced and never removed: an empty one goes out and Minka
// refuses it, where a description is emptied by an empty one.
func TestATitleOfAnUploadIsReplacedAndItsDescriptionEmptied(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload

	env, r := e.runJSON("", "docs", "uploads", "edit", upload.ID, "--title", "", "--force")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "uploadTitleMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if body := e.lastWriteBody(); body["title"] != "" {
		t.Fatalf("an empty title did not reach Minka as one: %v", body)
	}

	runOK(t, e, "docs", "uploads", "edit", upload.ID, "--title", "Site plan", "--force")
	if body := e.lastWriteBody(); body["title"] != "Site plan" {
		t.Fatalf("body %v", body)
	} else if _, sent := body["description"]; sent {
		t.Fatalf("a description nobody gave was sent: %v", body)
	}
	if upload.Description != "<p>Where everything stands</p>" {
		t.Fatalf("a change of the title touched the description: %q", upload.Description)
	}

	emptied := decode(t, runOK(t, e, "docs", "uploads", "edit", upload.ID, "--description", "", "--force").Data)
	if description, sent := e.lastWriteBody()["description"]; !sent || description != "" {
		t.Fatalf(`--description "" sent %v (%v)`, description, sent)
	}
	if emptied["description"] != "" || emptied["title"] != "Site plan" {
		t.Fatalf("description %v, title %v", emptied["description"], emptied["title"])
	}
}

// A description is Markdown turned into the HTML Minka keeps, on the way in
// and on a change, because Markdown sent as it is would stand there as its
// characters.
func TestADescriptionReachesMinkaAsMarkup(t *testing.T) {
	e := newTestEnv(t)

	runOK(t, e, putFile(e, "--description", "Where the **stage** goes")...)
	if body := e.lastWriteBody(); body["description"] != "<p>Where the <strong>stage</strong> goes</p>" {
		t.Fatalf("description %v", body["description"])
	}

	runOK(t, e, "docs", "uploads", "edit", e.fixture.FilesUpload.ID, "--description", "# Plan", "--force")
	if body := e.lastWriteBody(); body["description"] != "<h1>Plan</h1>" {
		t.Fatalf("description %v", body["description"])
	}

	env := runOK(t, e, "docs", "uploads", "edit", e.fixture.FilesUpload.ID, "--description", "# Plan", "--description-format", "html", "--force")
	if !strings.Contains(env.Notice, "characters it consists of") {
		t.Fatalf("notice %q does not say what becomes of HTML without markup", env.Notice)
	}
}

// The file of an upload is not among what a change can reach, so no flag
// pretends otherwise and no change carries one.
func TestAChangeOfAnUploadNamesNoFile(t *testing.T) {
	root := newApp(t.Context(), strings.NewReader(""), nil, nil, func(string) string { return "" }).newRootCmd()
	edit, _, err := root.Find([]string{"docs", "uploads", "edit"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"blob", "file", "blob-id"} {
		if edit.Flags().Lookup(name) != nil {
			t.Errorf("docs uploads edit has --%s, and Minka ignores a file sent there", name)
		}
	}

	e := newTestEnv(t)
	runOK(t, e, "docs", "uploads", "edit", e.fixture.FilesUpload.ID, "--title", "New", "--description", "New words", "--force")
	for _, request := range e.server.WritingRequests() {
		if strings.Contains(string(request.Body), "blobId") {
			t.Fatalf("%s %s carries a file: %s", request.Method, request.Path, request.Body)
		}
	}
}

// A step that fails ends the run: a file Minka did not store is put nowhere,
// and a tool that is at no address stores no file at all.
func TestNoStepIsTakenAfterOneThatFailed(t *testing.T) {
	t.Run("the file was not stored", func(t *testing.T) {
		e := newTestEnv(t)
		path := filepath.Join(t.TempDir(), "shot.png")
		if err := os.WriteFile(path, []byte("no picture at all"), 0o600); err != nil {
			t.Fatal(err)
		}
		env, r := e.runJSON("", "docs", "uploads", "create", path, "--tool", e.fixture.Files.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "contentTypeMismatch" || !strings.Contains(env.Hint, "--content-type") {
			t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
		}
		if requestsTo(e, http.MethodPost, projectUploads(e)) != 1 || requestsTo(e, http.MethodPost, toolUploads(e)) != 0 {
			t.Fatalf("requests %v", e.server.WritingRequests())
		}
	})
	t.Run("the project is archived", func(t *testing.T) {
		e := newTestEnv(t)
		e.fixture.Project.Archived = true
		env, r := e.runJSON("", putFile(e)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeProjectArchived || requestsTo(e, http.MethodPost, toolUploads(e)) != 0 {
			t.Fatalf("apiCode %v, requests %v", env.Meta["apiCode"], e.server.WritingRequests())
		}
	})
	t.Run("the tool is at no address", func(t *testing.T) {
		e := newTestEnv(t)
		env, r := e.runJSON("", "docs", "uploads", "create", aFileToUpload(e), "--tool", e.fixture.OtherFiles.ID)
		expectCode(t, r, ExitNotFound)
		if env.Hint != filesToolNotFoundHint {
			t.Fatalf("hint %q", env.Hint)
		}
		if writes := e.server.WritingRequests(); len(writes) != 0 {
			t.Fatalf("%d writes went out, the first to %s", len(writes), writes[0].Path)
		}
	})
}

// --project was once how the file found its project. A script that still
// names it keeps working where it names the project the tool stands in, is
// told that the flag is no longer needed, and is refused before the file
// travels where it names another.
func TestAProjectStillNamedIsHeldAgainstTheTool(t *testing.T) {
	t.Run("the project of the tool", func(t *testing.T) {
		e := newTestEnv(t)
		env := runOK(t, e, putFile(e, "--project", strings.ToUpper(e.fixture.Project.ID))...)
		if !strings.Contains(env.Notice, "--project is no longer needed") {
			t.Fatalf("notice %q", env.Notice)
		}
		if requestsTo(e, http.MethodPost, projectUploads(e)) != 1 || requestsTo(e, http.MethodPost, toolUploads(e)) != 1 {
			t.Fatalf("requests %v", e.server.WritingRequests())
		}
	})
	t.Run("the project of the tool, and the second step refused", func(t *testing.T) {
		e := newTestEnv(t)
		env, r := e.runJSON("", putFile(e, "--project", e.fixture.Project.ID, "--folder", e.fixture.ArchivedFolder.ID)...)
		expectCode(t, r, ExitValidation)
		if !strings.Contains(env.Notice, "--project is no longer needed") {
			t.Fatalf("notice %q", env.Notice)
		}
	})
	t.Run("another project", func(t *testing.T) {
		e := newTestEnv(t)
		e.fixture.OtherProject.MemberIDs = append(e.fixture.OtherProject.MemberIDs, e.fixture.Bruno.ID)
		env, r := e.runJSON("", putFile(e, "--project", e.fixture.OtherProject.ID)...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Error, "stands in project "+e.fixture.Project.ID) || !strings.Contains(env.Hint, "leave it out") {
			t.Fatalf("error %q, hint %q: the project of the tool and the remedy are not named", env.Error, env.Hint)
		}
		if writes := e.server.WritingRequests(); len(writes) != 0 {
			t.Fatalf("%d writes went out, the first to %s", len(writes), writes[0].Path)
		}
	})
	t.Run("a project that is no ID", func(t *testing.T) {
		e := newTestEnv(t)
		_, r := e.runJSON("", putFile(e, "--project", "Test-Projekt")...)
		expectCode(t, r, ExitUsage)
		if len(e.server.Requests()) != 0 {
			t.Fatal("a request went out")
		}
	})
}

// Where the file was stored and Minka refused to put it into the tool, the
// transfer is not lost: the error hands over the stored file, and --blob puts
// it in without sending it again.
func TestAStoredFileIsHandedOverWhereTheSecondStepWasRefused(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", putFile(e, "--folder", e.fixture.ArchivedFolder.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFolderArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	stored := blobID(t, decode(t, env.Data))
	if !strings.Contains(env.Hint, "--blob "+stored) || !strings.Contains(env.Hint, "at least seven days") {
		t.Fatalf("hint %q does not say how to try again without sending the file", env.Hint)
	}

	runOK(t, e, "docs", "uploads", "create", "--blob", stored, "--tool", e.fixture.Files.ID)
	if requestsTo(e, http.MethodPost, projectUploads(e)) != 1 {
		t.Fatal("the file was sent a second time")
	}
}

// Minka gives one answer for every file it does not know, swept or never
// there, and the remedy is to store the file again — unless this very run
// stored it a moment ago, where it cannot have been swept.
func TestAnUnknownFileIsToldApartByWhetherThisRunStoredIt(t *testing.T) {
	t.Run("named with --blob", func(t *testing.T) {
		e := newTestEnv(t)
		storedFile(e).Swept = true
		env, r := e.runJSON("", "docs", "uploads", "create", "--blob", storedBlobID, "--tool", e.fixture.Files.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeFileUnknown || !strings.Contains(env.Hint, "store it again") || !strings.Contains(env.Hint, "at least seven days") {
			t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
		}
		if env.Data != nil && string(env.Data) != "null" {
			t.Fatalf("data %s, although nothing was stored", env.Data)
		}
	})
	t.Run("stored in this run", func(t *testing.T) {
		e := newTestEnv(t)
		e.server.AfterStoring = func(file *fakeapi.Upload) { file.Swept = true }
		env, r := e.runJSON("", putFile(e)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeFileUnknown {
			t.Fatalf("apiCode %v", env.Meta["apiCode"])
		}
		if !strings.Contains(env.Hint, "stored in this same run") || strings.Contains(env.Hint, "store it again") {
			t.Fatalf("hint %q does not say that storing it again is not the remedy", env.Hint)
		}
		if stored := blobID(t, decode(t, env.Data)); !strings.Contains(env.Hint, "--blob "+stored) {
			t.Fatalf("hint %q", env.Hint)
		}
	})
	t.Run("in use already, stored in this run", func(t *testing.T) {
		e := newTestEnv(t)
		e.server.AfterStoring = func(file *fakeapi.Upload) { file.StandsInAText = true }
		env, r := e.runJSON("", putFile(e)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeFileInUse || strings.Contains(env.Hint, "--blob") {
			t.Fatalf("apiCode %v, hint %q points at naming the file again", env.Meta["apiCode"], env.Hint)
		}
	})
	t.Run("in use already", func(t *testing.T) {
		e := newTestEnv(t)
		storedFile(e).StandsInAText = true
		env, r := e.runJSON("", "docs", "uploads", "create", "--blob", storedBlobID, "--tool", e.fixture.Files.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeFileInUse || !strings.Contains(env.Hint, "one place only") {
			t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
		}
	})
}

// --blob names a file stored already, so the flags that describe storing one
// have nothing to do there, and a run gives one of the two.
func TestAFileOrABlobIsGivenAndNeverBoth(t *testing.T) {
	for name, args := range map[string][]string{
		"neither":                {"docs", "uploads", "create", "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
		"both":                   {"docs", "uploads", "create", "notes.txt", "--blob", storedBlobID, "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
		"blob with name":         {"docs", "uploads", "create", "--blob", storedBlobID, "--name", "x.txt", "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
		"blob with project":      {"docs", "uploads", "create", "--blob", storedBlobID, "--project", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f7", "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
		"blob with content-type": {"docs", "uploads", "create", "--blob", storedBlobID, "--content-type", "text/plain", "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
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

// A change guarded by the moment that was read is refused where somebody wrote
// since, and hands over the upload as it stands.
func TestAnUploadIsWrittenOverOnlyWhereItStandsAsItWasRead(t *testing.T) {
	e := newTestEnv(t)
	upload := e.fixture.FilesUpload
	updatedAt := decode(t, runOK(t, e, "docs", "uploads", "show", upload.ID).Data)["updatedAt"].(string)

	written := runOK(t, e, "docs", "uploads", "edit", upload.ID, "--title", "Site plan", "--expect", updatedAt)
	if e.lastWriteBody()["expectedUpdatedAt"] != updatedAt || !strings.Contains(written.Notice, "whole second") {
		t.Fatalf("body %v, notice %q", e.lastWriteBody(), written.Notice)
	}

	env, r := e.runJSON("", "docs", "uploads", "edit", upload.ID, "--title", "Site plan, final", "--expect", updatedAt)
	expectCode(t, r, ExitValidation)
	current := decode(t, env.Data)
	if env.Meta["apiCode"] != apiCodeUploadChanged || current["title"] != "Site plan" {
		t.Fatalf("apiCode %v, data %v", env.Meta["apiCode"], current["title"])
	}
	if !strings.Contains(env.Hint, "--expect "+current["updatedAt"].(string)) {
		t.Fatalf("hint %q", env.Hint)
	}

	for name, one := range map[string]struct {
		args []string
		says string
	}{
		"neither": {[]string{"--title", "New"}, "which upload"},
		"none":    {[]string{"--title", "New", "--expect", "none"}, "notes of a card"},
		"nothing": {[]string{"--force"}, "nothing to change"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", slices.Concat([]string{"docs", "uploads", "edit", upload.ID}, one.args)...)
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, one.says) || len(e.server.WritingRequests()) != 0 {
				t.Fatalf("error %q, %d writes", env.Error, len(e.server.WritingRequests()))
			}
		})
	}
}
