package cli

import (
	"net/http"
	"strings"
	"testing"
)

// noRequestGoesOut runs a call the CLI has to refuse itself, with exit 1 and
// before anything reaches Minka.
func noRequestGoesOut(t *testing.T, e *testEnv, args ...string) {
	t.Helper()
	before := len(e.server.Requests())
	_, r := e.runJSON("", args...)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != before {
		t.Fatalf("%v: a request went out", args)
	}
}

// A folder is marked as a book and read as one tree of chapters and pages,
// from any folder of it; the text indents the tree and names what is left out
// of the navigation or ordered automatically.
func TestAFolderIsMarkedAndReadAsABook(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "docs", "folders", "edit", f.Folder.ID, "--book")
	if body := e.lastWriteBody(); len(body) != 1 || body["isBook"] != true {
		t.Fatalf("body %v: only the mark goes out", body)
	}
	if env.Summary != "Folder Plans is a book" || crumbFor(t, env.Breadcrumbs, "book") != "minka docs book "+f.Folder.ID {
		t.Fatalf("summary %q, breadcrumbs %v", env.Summary, env.Breadcrumbs)
	}
	chapter := decode(t, rawOf(t, decode(t, runOK(t, e, "docs", "folders", "create", "--tool", f.Files.ID, "--folder", f.Folder.ID, "--title", "Setup").Data), "folder"))["id"].(string)
	runOK(t, e, "docs", "order", f.Files.ID, "automatic", "--folder", chapter)
	tools := decode(t, rawOf(t, decode(t, runOK(t, e, "docs", "folders", "create", "--tool", f.Files.ID, "--folder", chapter, "--title", "Tools").Data), "folder"))["id"].(string)
	hammer := decode(t, runOK(t, e, "docs", "create", "--tool", f.Files.ID, "--folder", tools, "--title", "Hammer", "--audience", "nobody").Data)["id"].(string)
	f.DocumentInFolder.OutOfNavigation = true

	env = runOK(t, e, "docs", "book", chapter)
	if env.Summary != "Book Plans: 2 chapters and 2 pages" || decode(t, env.Data)["id"] != f.Folder.ID {
		t.Fatalf("summary %q, data %s", env.Summary, env.Data)
	}
	if crumbFor(t, env.Breadcrumbs, "navigation") != "minka docs edit <documentId> --not-in-navigation" {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}
	if last := e.server.Requests()[len(e.server.Requests())-1]; last.Method != http.MethodGet || last.Path != "/api/v1/files-folders/"+chapter+"/book" {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}

	text := e.run("", "docs", "book", f.Folder.ID, "--styled")
	for _, line := range []string{
		"# Plans\nBook: " + f.Folder.ID + "\nTool: Documents (" + f.Files.ID + ")\nOrder: manual\n",
		"\n- " + f.DocumentInFolder.ID + "  What the tent costs (not in navigation)\n" +
			"- " + chapter + "  Setup/ (ordered automatically)\n" +
			"  - " + tools + "  Tools/\n" +
			"    - " + hammer + "  Hammer\n",
	} {
		if !strings.Contains(text.stdout, line) {
			t.Fatalf("text output lacks %q:\n%s", line, text.stdout)
		}
	}

	// A place in a book names it; a book's folder says it is one.
	listing := e.run("", "docs", "list", f.Files.ID, "--folder", chapter, "--styled")
	if !strings.Contains(listing.stdout, "Book: Plans ("+f.Folder.ID+")\nOrder: automatic\n") {
		t.Fatalf("text output:\n%s", listing.stdout)
	}
	top := e.run("", "docs", "list", f.Files.ID, "--styled")
	if !strings.Contains(top.stdout, "  Plans (3 entries, book)") {
		t.Fatalf("text output:\n%s", top.stdout)
	}
	shown := e.run("", "docs", "show", f.DocumentInFolder.ID, "--styled")
	if !strings.Contains(shown.stdout, "Book: Plans ("+f.Folder.ID+")\nNavigation: left out of the navigation of its book\n") {
		t.Fatalf("text output:\n%s", shown.stdout)
	}

	env, r := e.runJSON("", "docs", "book", f.FolderShownToClients.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "stands in no book") || !strings.Contains(env.Hint, "--book") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// The mark is given or taken, never both, and an edit that changes nothing
// goes nowhere.
func TestTheMarkOfABookIsGivenOrTaken(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	noRequestGoesOut(t, e, "docs", "folders", "edit", f.Folder.ID, "--book", "--no-book")
	noRequestGoesOut(t, e, "docs", "folders", "edit", f.Folder.ID)
	noRequestGoesOut(t, e, "docs", "folders", "edit", f.Folder.ID, "--book=false")
	noRequestGoesOut(t, e, "docs", "folders", "edit", f.Folder.ID, "--no-book=false", "--title", "Plans 2027")

	f.Folder.IsBook = true
	env := runOK(t, e, "docs", "folders", "edit", f.Folder.ID, "--no-book", "--title", "Plans 2027")
	if body := e.lastWriteBody(); body["isBook"] != false || body["title"] != "Plans 2027" {
		t.Fatalf("body %v", body)
	}
	if env.Summary != "Renamed folder "+f.Folder.ID+" to Plans 2027, which is no book" || f.Folder.IsBook {
		t.Fatalf("summary %q", env.Summary)
	}
}

// What Minka refuses about a book says what to do.
func TestARefusedBookSaysWhatToDo(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.ArchivedFolder.IsBook = true

	env, r := e.runJSON("", "docs", "folders", "edit", f.Folder.ID, "--book")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeBookWithinABook || !strings.Contains(env.Hint, "--no-book") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	f.ArchivedFolder.IsBook, f.Folder.IsBook, f.FolderShownToClients.IsBook = false, true, true
	env, r = e.runJSON("", "docs", "folders", "move", f.FolderShownToClients.ID, "--folder", f.Folder.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeBookWithinABook || !strings.Contains(env.Hint, "outside every book") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	e.server.Tokens["minka_pat_client"] = f.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"
	env, r = e.runJSON("", "docs", "folders", "edit", f.FolderShownToClients.ID, "--no-book")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "Only the team marks a book") {
		t.Fatalf("hint %q", env.Hint)
	}
	env, r = e.runJSON("", "docs", "edit", f.Document.ID, "--not-in-navigation")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "Only the team chooses the navigation") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// The navigation of a document alone needs no guard, and sends one where it
// is given; a change of the text still needs one.
func TestADocumentLeavesTheNavigationOfItsBook(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Folder.IsBook = true

	env := runOK(t, e, "docs", "edit", f.DocumentInFolder.ID, "--not-in-navigation")
	if body := e.lastWriteBody(); len(body) != 1 || body["inNavigation"] != false {
		t.Fatalf("body %v: only the navigation goes out", body)
	}
	if env.Summary != "Document What the tent costs is out of the navigation of its book" || env.Notice != "" || !f.DocumentInFolder.OutOfNavigation {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}

	updatedAt := decode(t, env.Data)["updatedAt"].(string)
	env = runOK(t, e, "docs", "edit", f.DocumentInFolder.ID, "--in-navigation", "--expect", updatedAt)
	if body := e.lastWriteBody(); body["inNavigation"] != true || body["expectedUpdatedAt"] != updatedAt {
		t.Fatalf("body %v: --expect goes out as given", body)
	}
	// The second the guard cannot see belongs to a write of the text, and
	// there is none here.
	if env.Notice != "" {
		t.Fatalf("notice %q on the navigation alone", env.Notice)
	}

	env = runOK(t, e, "docs", "edit", f.Document.ID, "--not-in-navigation", "--force")
	if !strings.Contains(env.Notice, "stands in no book") {
		t.Fatalf("notice %q", env.Notice)
	}

	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID, "--in-navigation", "--not-in-navigation")
	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID, "--title", "New", "--not-in-navigation")
	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID, "--not-in-navigation", "--expect", "yesterday")
	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID)
	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID, "--in-navigation=false")
	noRequestGoesOut(t, e, "docs", "edit", f.Document.ID, "--not-in-navigation=false", "--title", "New", "--force")
}

// A move names one neighbour at most, as an ID, and the summary says where
// the entry landed.
func TestAMoveNamesANeighbour(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	noRequestGoesOut(t, e, "docs", "move", f.Document.ID, "--folder", f.Folder.ID, "--after", f.DocumentInFolder.ID, "--before", f.FilesUpload.ID)
	noRequestGoesOut(t, e, "docs", "move", f.Document.ID, "--folder", f.Folder.ID, "--after", "first")
	noRequestGoesOut(t, e, "docs", "move", f.Document.ID, "--folder", f.Folder.ID, "--after", strings.ToUpper(f.Document.ID))
	noRequestGoesOut(t, e, "docs", "folders", "move", f.Folder.ID, "--top-level", "--before", f.Folder.ID)

	env, r := e.runJSON("", "docs", "move", f.Document.ID, "--folder", f.Folder.ID, "--after", f.DocumentInFolder.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFolderOrderAutomatic || !strings.Contains(env.Hint, "minka docs order <toolId> manual") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	runOK(t, e, "docs", "order", f.Files.ID, "manual", "--folder", f.Folder.ID)
	env = runOK(t, e, "docs", "move", f.Document.ID, "--folder", f.Folder.ID, "--before", f.DocumentInFolder.ID)
	if body := e.lastWriteBody(); body["nextEntryId"] != f.DocumentInFolder.ID || body["previousEntryId"] != nil {
		t.Fatalf("body %v", body)
	}
	if env.Summary != "Moved document How the stage goes up to Plans in Documents, asked before "+f.DocumentInFolder.ID {
		t.Fatalf("summary %q", env.Summary)
	}
	env = runOK(t, e, "docs", "uploads", "move", f.FilesUpload.ID, "--folder", f.Folder.ID, "--after", f.Document.ID)
	if body := e.lastWriteBody(); body["previousEntryId"] != f.Document.ID || body["nextEntryId"] != nil {
		t.Fatalf("body %v", body)
	}
	if !strings.HasSuffix(env.Summary, ", asked after "+f.Document.ID) {
		t.Fatalf("summary %q", env.Summary)
	}

	text := e.run("", "docs", "list", f.Files.ID, "--folder", f.Folder.ID, "--styled")
	want := "## Entries (3), in the order set by hand\n\n- " + f.Document.ID + "  document How the stage goes up (1 comment)\n"
	if !strings.Contains(text.stdout, want) || !strings.Contains(text.stdout, "- "+f.FilesUpload.ID+"  file site-plan.pdf") {
		t.Fatalf("text output:\n%s", text.stdout)
	}
	if strings.Index(text.stdout, f.FilesUpload.ID) > strings.Index(text.stdout, f.DocumentInFolder.ID) {
		t.Fatalf("the upload placed after the document does not stand before the one after it:\n%s", text.stdout)
	}
}

// The order of a place is automatic or manual, and the CLI sends nothing
// else.
func TestAPlaceIsOrderedAutomaticallyOrByHand(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	noRequestGoesOut(t, e, "docs", "order", f.Files.ID, "alphabetical")
	noRequestGoesOut(t, e, "docs", "order", f.Files.ID, "manual", "--folder", "plans")

	env := runOK(t, e, "docs", "order", f.Files.ID, "manual")
	if body := e.lastWriteBody(); len(body) != 1 || body["order"] != "manual" {
		t.Fatalf("body %v", body)
	}
	if last := e.server.WritingRequests()[len(e.server.WritingRequests())-1]; last.Method != http.MethodPut || last.Path != "/api/v1/files-tools/"+f.Files.ID+"/order" {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}
	if env.Summary != "Documents is arranged by hand" || decode(t, env.Data)["order"] != "manual" {
		t.Fatalf("summary %q", env.Summary)
	}
	env = runOK(t, e, "docs", "order", f.Files.ID, "automatic", "--folder", f.Folder.ID)
	if body := e.lastWriteBody(); body["folderId"] != f.Folder.ID || env.Summary != "Plans orders its entries automatically" {
		t.Fatalf("body %v, summary %q", body, env.Summary)
	}

	env, r := e.runJSON("", "docs", "order", f.Files.ID, "manual", "--folder", f.ArchivedFolder.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeFolderArchived || !strings.Contains(env.Hint, "keeps its order") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	e.server.Tokens["minka_pat_client"] = f.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"
	env, r = e.runJSON("", "docs", "order", f.Files.ID, "manual")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "Only the team chooses the order") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// A Minka without books answers a mark and a navigation with 200 and writes
// neither; the command does not pass that for success, and says what was
// written beside it.
func TestAMinkaWithoutBooksIsNotTakenForOneThatWroteTheMark(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.KnowsNoBooks = true

	env, r := e.runJSON("", "docs", "folders", "edit", f.Folder.ID, "--book")
	expectCode(t, r, ExitAPI)
	if env.Error != "the answer does not carry the mark of a book asked for: this Minka may not know books" || decode(t, env.Data)["folder"] == nil {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
	env, r = e.runJSON("", "docs", "folders", "edit", f.Folder.ID, "--title", "Handbook", "--book")
	expectCode(t, r, ExitAPI)
	if !strings.HasSuffix(env.Error, ", and the rest of the change went through") || f.Folder.Title != "Handbook" || f.Folder.IsBook {
		t.Fatalf("error %q", env.Error)
	}
	env, r = e.runJSON("", "docs", "edit", f.Document.ID, "--not-in-navigation")
	expectCode(t, r, ExitAPI)
	if env.Error != "the answer does not carry the navigation asked for: this Minka may not know books" || f.Document.OutOfNavigation {
		t.Fatalf("error %q", env.Error)
	}
	env, r = e.runJSON("", "docs", "edit", f.Document.ID, "--title", "New", "--not-in-navigation", "--force")
	expectCode(t, r, ExitAPI)
	if !strings.HasSuffix(env.Error, ", and the rest of the change went through") {
		t.Fatalf("error %q", env.Error)
	}
}
