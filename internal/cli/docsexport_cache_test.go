package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

const linksPageID = "0192f3a4-5b6c-4d7e-8f90-0000000e000a"

// withALinksPage adds a page to the setup chapter that refers to two pages of
// the book, one of them at a section, and links to one of them by its address.
func withALinksPage(b *exportBook) *fakeapi.Document {
	f := b.e.fixture
	links := &fakeapi.Document{ID: linksPageID, Title: "Links", FolderID: setupChapterID, CreatedAt: b.costs.CreatedAt, UpdatedAt: b.costs.UpdatedAt, CreatorID: f.Bruno.ID,
		Body: `<p><minka-ref kind="document" id="` + b.costs.ID + `" anchor="h-prices"></minka-ref>, <a href="/go/item/` + b.costs.ID + `">the costs</a> and ` +
			`<minka-ref kind="document" id="` + umlautPageID + `"></minka-ref>.</p>`}
	f.Files.Documents = append(f.Files.Documents, links)
	f.Files.Arranged[setupChapterID] = append(f.Files.Arranged[setupChapterID], linksPageID)
	return links
}

// A page Minka has not changed is not read again, and still follows the pages
// it refers to: renamed, the reference carries the new title and the link the
// new file; moved, the link goes to the new place. Only the page that changed
// is read.
func TestAPageNotReadAgainFollowsThePagesItRefersTo(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	withALinksPage(b)
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	linksPage := filepath.Join(out, "setup", "links.md")
	if page := readFile(t, linksPage); !strings.Contains(page, "[What the tent costs](../what-the-tent-costs.md#h-prices), [the costs](../what-the-tent-costs.md) and [Über Größe](deep/ueber-groesse.md).") {
		t.Fatalf("page:\n%s", page)
	}
	linksReads := requestsTo(e, "GET", "/api/v1/documents/"+linksPageID)
	costsReads := requestsTo(e, "GET", "/api/v1/documents/"+b.costs.ID)

	b.costs.Rename("What the tent costs now")
	for _, document := range f.Files.Documents {
		if document.ID == umlautPageID {
			document.FolderID = f.Folder.ID // Über Größe moves up to the book
		}
	}
	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if page := readFile(t, linksPage); !strings.Contains(page, "[What the tent costs now](../what-the-tent-costs-now.md#h-prices), [the costs](../what-the-tent-costs-now.md) and [Über Größe](../ueber-groesse.md).") {
		t.Fatalf("the page did not follow:\n%s", page)
	}
	if requestsTo(e, "GET", "/api/v1/documents/"+linksPageID) != linksReads || requestsTo(e, "GET", "/api/v1/documents/"+b.costs.ID) != costsReads+1 {
		t.Fatal("the page that changed was not read, or the one that did not was")
	}
	// The tools page refers to a page outside the book, and is read on every run.
	if data.DocumentsRead != 2 {
		t.Fatalf("%d pages read", data.DocumentsRead)
	}
	if !strings.Contains(readFile(t, filepath.Join(out, "what-the-tent-costs-now.md")), "# What the tent costs now") {
		t.Fatal("the renamed page was not written under its new name")
	}
}

// A page whose body changed is read again, and its file says what it says
// now; the page referring to it stays as it is.
func TestAChangedPageIsReadAgain(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	withALinksPage(b)
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	b.costs.Edit(`<h2>Prices</h2><p>Five hundred.</p>`)
	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if !strings.Contains(readFile(t, filepath.Join(out, "what-the-tent-costs.md")), "Five hundred.") {
		t.Fatal("the changed page is stale")
	}
	if len(data.Written) != 1 || data.Written[0] != "what-the-tent-costs.md" {
		t.Fatalf("written %q", data.Written)
	}
}

// A reference to something outside the export carries the title Minka gives
// it when the page is read, so a page holding one is read on every run.
func TestAPageReferringOutsideIsReadOnEveryRun(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--format", "html")

	f.Document.Rename("How the stage goes up now")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--format", "html")
	if page := readFile(t, filepath.Join(out, "setup", "tools.html")); !strings.Contains(page, ">How the stage goes up now</a>") {
		t.Fatalf("the reference outside is stale:\n%s", page)
	}
}

// A page's file that went missing is written again from what the manifest
// keeps, without reading the page.
func TestAMissingFileOfAnUnchangedPageIsWrittenAgain(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	before := readFile(t, filepath.Join(out, "what-the-tent-costs.md"))
	reads := requestsTo(e, "GET", "/api/v1/documents/"+b.costs.ID)
	if err := os.Remove(filepath.Join(out, "what-the-tent-costs.md")); err != nil {
		t.Fatal(err)
	}

	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	if readFile(t, filepath.Join(out, "what-the-tent-costs.md")) != before || requestsTo(e, "GET", "/api/v1/documents/"+b.costs.ID) != reads {
		t.Fatal("the page was not written again as it was, or it was read")
	}
}

// A folder Minka does not find answers with its error, not with a page of its
// web, and is not taken for a sign of an older Minka.
func TestAFolderNotFoundIsNoOlderMinka(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Folder.Trashed = true
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", filepath.Join(t.TempDir(), "plans"))
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Hint, "--tool") || strings.Contains(env.Error, "tree") || requestsTo(e, "GET", "/api/v1/files-folders/"+f.Folder.ID+"/book") != 0 {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}

// A client is told no versions, so every page is read on every run.
func TestAClientReadsEveryPageAgain(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	f.Folder.ShownToClients = true
	asClient(e)
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	b.costs.Edit(`<h2>Prices</h2><p>Five hundred.</p>`)
	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if data.DocumentsRead != data.Documents || !strings.Contains(readFile(t, filepath.Join(out, "what-the-tent-costs.md")), "Five hundred.") {
		t.Fatalf("a client's run: %+v", data)
	}
	if manifest := readFile(t, filepath.Join(out, ".minka-export.json")); strings.Contains(manifest, `"pages"`) {
		t.Fatal("the manifest keeps pages a client cannot take again")
	}
}
