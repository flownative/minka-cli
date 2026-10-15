package cli

import (
	"net/http"
	"strings"
	"testing"
)

// orderKind is one kind of entry of a Docs & Files tool as the commands that
// put a tool in order name it.
type orderKind struct {
	group []string // the words before the action
	base  string   // the address of the kind
}

var orderKinds = map[string]orderKind{
	"document": {[]string{"docs"}, "/api/v1/documents/"},
	"upload":   {[]string{"docs", "uploads"}, "/api/v1/files-uploads/"},
	"folder":   {[]string{"docs", "folders"}, "/api/v1/files-folders/"},
}

func (k orderKind) args(action string, rest ...string) []string {
	return append(append(append([]string{}, k.group...), action), rest...)
}

// Every kind of entry goes into the trash and comes back out of it, and into
// the archive and back, at its own address; what each write answers is the
// entry as Minka hands it over.
func TestEveryKindOfEntryGoesIntoTheTrashAndTheArchiveAndBack(t *testing.T) {
	for noun, kind := range orderKinds {
		t.Run(noun, func(t *testing.T) {
			e := newTestEnv(t)
			id := map[string]string{"document": e.fixture.Document.ID, "upload": e.fixture.FilesUpload.ID, "folder": e.fixture.Folder.ID}[noun]

			env := runOK(t, e, kind.args("delete", id)...)
			if data := decode(t, env.Data); data["id"] != id || data["deleted"] != true {
				t.Fatalf("data %s", env.Data)
			}
			if crumbFor(t, env.Breadcrumbs, "restore") != strings.Join(append([]string{"minka"}, kind.args("restore", id)...), " ") {
				t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
			}
			if last := e.server.WritingRequests()[len(e.server.WritingRequests())-1]; last.Method != http.MethodDelete || last.Path != kind.base+id {
				t.Fatalf("sent %s %s", last.Method, last.Path)
			}

			env = runOK(t, e, kind.args("restore", id)...)
			place := map[string]string{"document": "the top of Documents", "upload": "Plans in Documents", "folder": "the top of Documents"}[noun]
			if !strings.HasPrefix(env.Summary, "Brought "+noun+" ") || !strings.HasSuffix(env.Summary, "out of the trash into "+place) {
				t.Fatalf("summary %q", env.Summary)
			}
			env = runOK(t, e, kind.args("archive", id)...)
			if decode(t, env.Data)["archived"] != true || crumbFor(t, env.Breadcrumbs, "archive") != "minka docs list "+e.fixture.Files.ID+" --archived" {
				t.Fatalf("data %s, breadcrumbs %v", env.Data, env.Breadcrumbs)
			}
			env = runOK(t, e, kind.args("unarchive", id)...)
			if decode(t, env.Data)["archived"] != false || !strings.HasSuffix(env.Summary, "out of the archive into "+place) {
				t.Fatalf("data %s", env.Data)
			}
			for _, sent := range e.server.WritingRequests()[1:] {
				if sent.Method != http.MethodPost || !strings.HasPrefix(sent.Path, kind.base+id+"/") {
					t.Fatalf("sent %s %s", sent.Method, sent.Path)
				}
			}
		})
	}
}

// A move names its place: a folder of the tool, or its top level, which goes
// out as null. Naming neither or both stops the command before anything goes
// out.
func TestAMoveNamesExactlyOnePlace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, flags := range map[string][]string{
		"neither": {},
		"both":    {"--folder", f.Folder.ID, "--top-level"},
	} {
		t.Run(name, func(t *testing.T) {
			before := len(e.server.Requests())
			_, r := e.runJSON("", append([]string{"docs", "move", f.Document.ID}, flags...)...)
			expectCode(t, r, ExitUsage)
			if len(e.server.Requests()) != before {
				t.Fatal("a request went out")
			}
		})
	}

	env := runOK(t, e, "docs", "move", f.Document.ID, "--folder", f.Folder.ID)
	if body := e.lastWriteBody(); body["folderId"] != f.Folder.ID {
		t.Fatalf("body %v", body)
	}
	if env.Summary != "Moved document How the stage goes up to Plans in Documents" || f.Document.FolderID != f.Folder.ID {
		t.Fatalf("summary %q", env.Summary)
	}

	env = runOK(t, e, "docs", "uploads", "move", f.FilesUpload.ID, "--top-level")
	body := e.lastWriteBody()
	if value, given := body["folderId"]; !given || value != nil {
		t.Fatalf("body %v: the top level goes out as null", body)
	}
	if env.Summary != "Moved upload site-plan.pdf to the top of Documents" || f.FilesUpload.FolderID != "" {
		t.Fatalf("summary %q", env.Summary)
	}
}

// What Minka refuses a write that puts a tool in order comes back as exit 9
// with a hint that names what to do about it; the same code can mean another
// folder to another write, and the hint says which one.
func TestARefusedOrderSaysWhatToDo(t *testing.T) {
	for name, c := range map[string]struct {
		prepare func(e *testEnv) []string
		code    string
		hint    []string
	}{
		"bringing back what is not archived": {func(e *testEnv) []string {
			return []string{"docs", "unarchive", e.fixture.Document.ID}
		}, "documentNotArchived", []string{"minka docs archive " + "{document}"}},
		"archiving what is archived": {func(e *testEnv) []string {
			return []string{"docs", "archive", e.fixture.ArchivedDocument.ID}
		}, apiCodeDocumentArchived, []string{"already", "--archived"}},
		"moving what is archived": {func(e *testEnv) []string {
			return []string{"docs", "move", e.fixture.ArchivedDocument.ID, "--top-level"}
		}, apiCodeDocumentArchived, []string{"minka docs unarchive " + "{archived}"}},
		"trashing below an archived folder": {func(e *testEnv) []string {
			return []string{"docs", "delete", e.fixture.DocumentBelowTheArchivedFolder.ID}
		}, apiCodeFolderArchived, []string{"not taken out on its own", "minka docs folders unarchive <folderId>", "minka docs show"}},
		"bringing back what went with its folder": {func(e *testEnv) []string {
			e.fixture.DocumentBelowTheArchivedFolder.Archived = true
			return []string{"docs", "unarchive", e.fixture.DocumentBelowTheArchivedFolder.ID}
		}, apiCodeFolderArchived, []string{"only that folder brings it back", "archivedWithAFolderAbove"}},
		"moving into an archived folder": {func(e *testEnv) []string {
			return []string{"docs", "move", e.fixture.Document.ID, "--folder", e.fixture.ArchivedFolder.ID}
		}, apiCodeFolderArchived, []string{"--folder names", "--top-level"}},
		"moving into another tool": {func(e *testEnv) []string {
			return []string{"docs", "uploads", "move", e.fixture.FilesUpload.ID, "--folder", e.fixture.OtherFiles.ID}
		}, apiCodeFolderUnknown, []string{"within its tool"}},
		"moving a folder into itself": {func(e *testEnv) []string {
			return []string{"docs", "folders", "move", e.fixture.Folder.ID, "--folder", e.fixture.Folder.ID}
		}, apiCodeFolderIntoItself, []string{"outside the one that moves", "--top-level"}},
		"moving an archived folder": {func(e *testEnv) []string {
			return []string{"docs", "folders", "move", e.fixture.ArchivedFolder.ID, "--top-level"}
		}, apiCodeFolderArchived, []string{"nothing in the archive moves"}},
		"renaming an archived folder": {func(e *testEnv) []string {
			return []string{"docs", "folders", "edit", e.fixture.ArchivedFolder.ID, "--title", "Old"}
		}, apiCodeFolderArchived, []string{"keeps its title", "minka docs folders unarchive " + "{archivedFolder}"}},
		"a folder without a title": {func(e *testEnv) []string {
			return []string{"docs", "folders", "create", "--tool", e.fixture.Files.ID, "--title", " "}
		}, apiCodeFolderTitleMissing, []string{"never emptied"}},
		"a folder renamed to nothing": {func(e *testEnv) []string {
			return []string{"docs", "folders", "edit", e.fixture.Folder.ID, "--title", ""}
		}, apiCodeFolderTitleMissing, []string{"never emptied"}},
		"a folder in an archived one": {func(e *testEnv) []string {
			return []string{"docs", "folders", "create", "--tool", e.fixture.Files.ID, "--title", "New", "--folder", e.fixture.ArchivedFolder.ID}
		}, apiCodeFolderArchived, []string{"takes nothing new", "minka docs folders unarchive"}},
		"a version of an archived document": {func(e *testEnv) []string {
			version := listOfObjects(t, runOK(t, e, "docs", "versions", "list", e.fixture.ArchivedDocument.ID).Data)[0]["id"].(string)
			return []string{"docs", "versions", "restore", e.fixture.ArchivedDocument.ID, version}
		}, apiCodeDocumentArchived, []string{"minka docs unarchive " + "{archived}"}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args := c.prepare(e)
			env, r := e.runJSON("", args...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != c.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], c.code)
			}
			ids := strings.NewReplacer("{document}", e.fixture.Document.ID, "{archived}", e.fixture.ArchivedDocument.ID, "{archivedFolder}", e.fixture.ArchivedFolder.ID)
			for _, word := range c.hint {
				if !strings.Contains(env.Hint, ids.Replace(word)) {
					t.Fatalf("hint %q does not say %q", env.Hint, ids.Replace(word))
				}
			}
		})
	}
}

// A client of the project puts away only what they made and brings nothing
// back, and every 404 says so beside what else it can mean.
func TestAClientIsToldWhatOnlyTheTeamDoes(t *testing.T) {
	e := newTestEnv(t)
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
	version := listOfObjects(t, runOK(t, e, "docs", "versions", "list", e.fixture.Document.ID).Data)[0]["id"].(string)
	runOK(t, e, "docs", "delete", e.fixture.DocumentInFolder.ID)
	e.vars["MINKA_TOKEN"] = "minka_pat_client"

	for name, c := range map[string]struct {
		args []string
		hint string
	}{
		"somebody else's into the trash":   {[]string{"docs", "uploads", "delete", e.fixture.FilesUpload.ID}, "puts away only what they made"},
		"somebody else's into the archive": {[]string{"docs", "folders", "archive", e.fixture.Folder.ID}, "puts away only what they made"},
		"out of the archive":               {[]string{"docs", "unarchive", e.fixture.ArchivedDocument.ID}, "Only the team brings anything back"},
		"out of the trash":                 {[]string{"docs", "restore", e.fixture.DocumentInFolder.ID}, "Only the team brings anything back"},
		"a version":                        {[]string{"docs", "versions", "restore", e.fixture.Document.ID, version}, "only the team restores a version"},
	} {
		t.Run(name, func(t *testing.T) {
			env, r := e.runJSON("", c.args...)
			expectCode(t, r, ExitNotFound)
			if !strings.Contains(env.Hint, c.hint) {
				t.Fatalf("hint %q", env.Hint)
			}
		})
	}

	env := runOK(t, e, "docs", "folders", "create", "--tool", e.fixture.Files.ID, "--title", "From the client")
	own := decode(t, rawOf(t, decode(t, env.Data), "folder"))["id"].(string)
	runOK(t, e, "docs", "folders", "delete", own)
}

// Only what lies in the trash by itself comes back, and the hint names the
// ways something else does.
func TestWhatIsNotInTheTrashByItselfDoesNotComeBack(t *testing.T) {
	e := newTestEnv(t)
	runOK(t, e, "docs", "folders", "delete", e.fixture.Folder.ID)

	env, r := e.runJSON("", "docs", "uploads", "restore", e.fixture.FilesUpload.ID)
	expectCode(t, r, ExitNotFound)
	for _, word := range []string{"went in with a folder above it", "minka docs folders restore <folderId>", "blockedBy"} {
		if !strings.Contains(env.Hint, word) {
			t.Fatalf("hint %q does not say %q", env.Hint, word)
		}
	}
	_, r = e.runJSON("", "docs", "restore", e.fixture.Document.ID)
	expectCode(t, r, ExitNotFound)
}

// A folder is made where it is asked for and answers with its ID; whether it
// is shown to clients counts only at the top of the tool, and the command says
// so where it is asked for further down.
func TestAFolderIsMadeAndRenamed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "docs", "folders", "create", "--tool", f.Files.ID, "--title", "Offers", "--visible-to-clients")
	if body := e.lastWriteBody(); body["title"] != "Offers" || body["visibleToClients"] != true || body["folderId"] != nil {
		t.Fatalf("body %v", body)
	}
	if env.Notice != "" || env.Summary != "Made folder Offers at the top of Documents" || !f.Files.Folders[0].ShownToClients {
		t.Fatalf("notice %q, summary %q", env.Notice, env.Summary)
	}
	made := decode(t, rawOf(t, decode(t, env.Data), "folder"))["id"].(string)
	if crumbFor(t, env.Breadcrumbs, "list") != "minka docs list "+f.Files.ID+" --folder "+made {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}

	env = runOK(t, e, "docs", "folders", "create", "--tool", f.Files.ID, "--title", "2027", "--folder", f.Folder.ID, "--visible-to-clients")
	if !strings.Contains(env.Notice, "counts only at the top of the tool") || env.Summary != "Made folder 2027 in Plans in Documents" {
		t.Fatalf("notice %q, summary %q", env.Notice, env.Summary)
	}
	if f.Files.Folders[0].ShownToClients {
		t.Fatal("a folder further down was shown to clients")
	}

	env = runOK(t, e, "docs", "folders", "edit", made, "--title", "Offers 2027")
	if env.Summary != "Renamed folder "+made+" to Offers 2027" {
		t.Fatalf("summary %q", env.Summary)
	}

	for name, args := range map[string][]string{
		"a folder without a title": {"docs", "folders", "create", "--tool", f.Files.ID},
		"a rename without a title": {"docs", "folders", "edit", made},
	} {
		t.Run(name, func(t *testing.T) {
			before := len(e.server.Requests())
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.Requests()) != before {
				t.Fatal("a request went out")
			}
		})
	}
}

// The archive of a tool is listed from any folder, with who put each entry
// away and whether only a folder above it brings it back.
func TestTheArchiveOfAToolIsListed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.DocumentBelowTheArchivedFolder.Archived = true

	env := runOK(t, e, "docs", "list", f.Files.ID, "--archived")
	entries := listOfObjects(t, env.Data)
	if len(entries) != 3 || env.Summary != "3 entries in the archive" {
		t.Fatalf("summary %q, entries %v", env.Summary, entries)
	}
	if crumbFor(t, env.Breadcrumbs, "unarchive-folder") != "minka docs folders unarchive <folderId>" {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}

	text := e.run("", "docs", "list", f.Files.ID, "--archived", "--styled")
	for _, line := range []string{
		"- " + f.ArchivedFolder.ID + "  folder Last year",
		"by Anke",
		"- " + f.DocumentBelowTheArchivedFolder.ID + "  document What the tent cost last year",
		"with a folder above it, which alone brings it back",
	} {
		if !strings.Contains(text.stdout, line) {
			t.Fatalf("text output lacks %q:\n%s", line, text.stdout)
		}
	}

	before := len(e.server.Requests())
	_, r := e.runJSON("", "docs", "list", f.Files.ID, "--archived", "--folder", f.Folder.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Fatal("a request went out")
	}
}

// An earlier version becomes the newest one, and the answer is the document.
func TestAnEarlierVersionIsWrittenAsTheNewest(t *testing.T) {
	e := newTestEnv(t)
	document := e.fixture.Document
	first := listOfObjects(t, runOK(t, e, "docs", "versions", "list", document.ID).Data)[0]["id"].(string)
	runOK(t, e, "docs", "edit", document.ID, "--body", "Nine people.", "--force")

	env := runOK(t, e, "docs", "versions", "restore", document.ID, first)
	if decode(t, env.Data)["body"] != "<p>Eight people on <strong>Friday</strong>.</p>" || len(document.Versions) != 3 {
		t.Fatalf("data %s, %d versions", env.Data, len(document.Versions))
	}
	if crumbFor(t, env.Breadcrumbs, "versions") != "minka docs versions list "+document.ID {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}
	if last := e.server.WritingRequests()[len(e.server.WritingRequests())-1]; last.Path != "/api/v1/documents/"+document.ID+"/versions/"+first+"/restore" {
		t.Fatalf("sent %s", last.Path)
	}
}

// The trash names how each kind of entry it holds comes back, and only the
// kinds that can.
func TestTheTrashNamesHowAnEntryOfDocsAndFilesComesBack(t *testing.T) {
	e := newTestEnv(t)
	runOK(t, e, "docs", "delete", e.fixture.Document.ID)

	env := runOK(t, e, "trash", "list", e.fixture.Project.ID)
	if crumbFor(t, env.Breadcrumbs, "restore-document") != "minka docs restore <documentId>" {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action == "restore-upload" || crumb.Action == "restore-folder" {
			t.Fatalf("breadcrumbs %v name a kind the trash does not hold", env.Breadcrumbs)
		}
	}
	text := e.run("", "trash", "list", e.fixture.Project.ID, "--styled")
	if !strings.Contains(text.stdout, "document  "+e.fixture.Document.ID) || !strings.Contains(text.stdout, "yes") {
		t.Fatalf("text output:\n%s", text.stdout)
	}
}

// The text of a write says what happened, the ID and where the entry is.
func TestTheTextOfAnOrderSaysWhatHappened(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "docs", "archive", e.fixture.Document.ID, "--styled")
	expectCode(t, r, ExitOK)
	want := "Put document How the stage goes up into the archive of Documents.\nID: " + e.fixture.Document.ID + "\n"
	if !strings.HasPrefix(r.stdout, want) || !strings.Contains(r.stdout, "/files/") {
		t.Fatalf("text output:\n%s", r.stdout)
	}
}
