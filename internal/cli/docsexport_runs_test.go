package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// A second run reads again only the pages Minka changed since, and those that
// refer to something outside the export, and writes only what changed: a
// page changed in Minka, the indexes that list a page put away, and that
// page's file removed. A name stays with its page while the title gives it,
// so a page of the same title put in front takes the next free name.
func TestASecondRunBringsTheExportUpToDate(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	first := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if first.DocumentsRead != 6 {
		t.Fatalf("the first run read %d pages", first.DocumentsRead)
	}
	documentReads := requestsTo(e, "GET", "/api/v1/documents/"+toolsPageID)
	costsReads := requestsTo(e, "GET", "/api/v1/documents/"+f.DocumentInFolder.ID)

	unchanged := runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data
	again := decodeExport(t, unchanged)
	if len(again.Written) != 0 || again.Unchanged != 11 || len(again.Removed) != 0 || again.DocumentsRead != 1 {
		t.Fatalf("an unchanged book: %+v", again)
	}
	for _, list := range []string{"written", "removed", "gone"} {
		if _, isList := decode(t, unchanged)[list].([]any); !isList {
			t.Fatalf("%s is no list: %s", list, unchanged)
		}
	}
	// The page that refers to a page outside the book is read again, the
	// others not.
	if requestsTo(e, "GET", "/api/v1/documents/"+toolsPageID) != documentReads+1 || requestsTo(e, "GET", "/api/v1/documents/"+f.DocumentInFolder.ID) != costsReads {
		t.Fatal("the pages read again are not the ones that may have changed")
	}
	if requestsTo(e, "GET", "/api/v1/uploads/"+pictureBlobID+"/original") != 1 {
		t.Fatal("the picture lying there as written was downloaded again")
	}

	b.costs.Edit(`<h2>Prices</h2><p>Five hundred.</p>`)
	b.other.Trashed = true
	b.tools.Edit(`<p>Only words now.</p>`)
	newer := &fakeapi.Document{ID: newToolsID, Title: "Tools", FolderID: setupChapterID, Body: `<p>New.</p>`,
		CreatedAt: b.tools.CreatedAt, UpdatedAt: b.tools.UpdatedAt, CreatorID: f.Bruno.ID}
	f.Files.Documents = append(f.Files.Documents, newer)
	f.Files.Arranged[setupChapterID] = []string{newToolsID, toolsPageID, deepChapterID}

	third := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if want := []string{"README.md", "setup/README.md", "setup/tools-2.md", "setup/tools.md", "what-the-tent-costs.md"}; !slices.Equal(third.Written, want) {
		t.Fatalf("written %q, want %q", third.Written, want)
	}
	if want := []string{"setup/tools.assets/plan.png"}; !slices.Equal(third.Removed, want) || third.Unchanged != 5 {
		t.Fatalf("removed %q, unchanged %d", third.Removed, third.Unchanged)
	}
	if page := readFile(t, filepath.Join(out, "setup", "tools-2.md")); !strings.Contains(page, `id: "`+newToolsID+`"`) {
		t.Fatalf("the new page did not take the free name:\n%s", page)
	}
	if page := readFile(t, filepath.Join(out, "setup", "tools.md")); !strings.Contains(page, `id: "`+toolsPageID+`"`) {
		t.Fatalf("the page lost its name to the one put in front:\n%s", page)
	}
	if _, err := os.Stat(filepath.Join(out, "setup", "tools.assets")); !os.IsNotExist(err) {
		t.Fatalf("the emptied directory of the picture stays: %v", err)
	}
	if got := filesIn(t, out); slices.Contains(got, "setup/tools.assets/plan.png") || len(got) != 11 {
		t.Fatalf("files %q", got)
	}
}

// Nothing the export did not write is written over or removed: a path in the
// way is refused before anything is written, a file the export wrote and
// somebody changed only with --force, and a directory holding files without
// a manifest only with --force, which leaves those files alone.
func TestAnExportLeavesWhatItDidNotWriteAlone(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	writeFile(t, filepath.Join(out, "notes.txt"), "mine", 0o644)

	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "holds files and no export") || !strings.Contains(env.Hint, "--force") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
	if got := filesIn(t, out); !slices.Equal(got, []string{"notes.txt"}) {
		t.Fatalf("files %q", got)
	}

	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force")
	if readFile(t, filepath.Join(out, "notes.txt")) != "mine" {
		t.Fatal("a file of somebody else was written over")
	}

	// A page whose file somebody put there first.
	writeFile(t, filepath.Join(out, "setup", "deep", "new.md"), "my notes", 0o644)
	f.Files.Documents = append(f.Files.Documents, &fakeapi.Document{ID: newToolsID, Title: "New", FolderID: deepChapterID, Body: `<p>New.</p>`,
		CreatedAt: b.tools.CreatedAt, UpdatedAt: b.tools.UpdatedAt, CreatorID: f.Bruno.ID})
	b.costs.Edit(`<p>Changed.</p>`)
	costsBefore := readFile(t, filepath.Join(out, "what-the-tent-costs.md"))
	for _, args := range [][]string{{}, {"--force"}} {
		env, r = e.runJSON("", append([]string{"docs", "export", f.Folder.ID, "--out", out}, args...)...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "1 path in "+out+" is in the way") || !strings.Contains(string(env.Data), `"setup/deep/new.md"`) {
			t.Fatalf("%v: error %q, data %s", args, env.Error, env.Data)
		}
	}
	if readFile(t, filepath.Join(out, "setup", "deep", "new.md")) != "my notes" || readFile(t, filepath.Join(out, "what-the-tent-costs.md")) != costsBefore {
		t.Fatal("a refused run wrote something")
	}
	if err := os.Remove(filepath.Join(out, "setup", "deep", "new.md")); err != nil {
		t.Fatal(err)
	}

	// A file of the export changed since: refused, then written over.
	writeFile(t, filepath.Join(out, "readme-2.md"), "edited by hand", 0o644)
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "1 file the export wrote") || !strings.Contains(string(env.Data), `"readme-2.md"`) || !strings.Contains(env.Hint, "--force") {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
	if readFile(t, filepath.Join(out, "readme-2.md")) != "edited by hand" {
		t.Fatal("a changed file was written over without --force")
	}
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force")
	if !strings.Contains(readFile(t, filepath.Join(out, "readme-2.md")), "Not the index.") || readFile(t, filepath.Join(out, "notes.txt")) != "mine" {
		t.Fatal("--force did not write the page, or wrote over a file of somebody else")
	}

	// A file of the export changed since and gone from Minka stays without
	// --force.
	writeFile(t, filepath.Join(out, "setup", "tools-2.md"), "kept", 0o644)
	b.other.Trashed = true
	_, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if readFile(t, filepath.Join(out, "setup", "tools-2.md")) != "kept" {
		t.Fatal("a changed file was removed without --force")
	}
}

// A run removes only files its manifest names, inside the directory and not
// behind a link; a manifest that names a path outside is refused whole.
func TestAnExportRemovesOnlyWhatItsManifestNamesInside(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	base := t.TempDir()
	out := filepath.Join(base, "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	outside := filepath.Join(base, "outside.txt")
	writeFile(t, outside, "outside", 0o644)
	elsewhere := filepath.Join(base, "elsewhere")
	writeFile(t, filepath.Join(elsewhere, "page.md"), "elsewhere", 0o644)
	if err := os.Symlink(elsewhere, filepath.Join(out, "linked")); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(out, ".minka-export.json")
	original := readFile(t, manifestPath)
	withFile := func(path, content string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(original), &m); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(content))
		m["files"] = append(m["files"].([]any), map[string]any{"path": path, "kind": "document", "id": newToolsID, "sha256": hex.EncodeToString(sum[:])})
		raw, _ := json.Marshal(m)
		return string(raw)
	}

	writeFile(t, manifestPath, withFile("../outside.txt", "outside"), 0o644)
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out, "--force")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, `names "../outside.txt", which lies outside it`) {
		t.Fatalf("error %q", env.Error)
	}
	if readFile(t, outside) != "outside" {
		t.Fatal("a file outside the directory was removed")
	}

	writeFile(t, manifestPath, withFile("linked/page.md", "elsewhere"), 0o644)
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force")
	if readFile(t, filepath.Join(elsewhere, "page.md")) != "elsewhere" {
		t.Fatal("a file behind a link was removed")
	}
	if strings.Contains(readFile(t, manifestPath), "linked/page.md") {
		t.Fatal("the manifest still claims the file behind the link")
	}

	// A directory of the export that became a link is not written through.
	deep := filepath.Join(out, "setup", "deep")
	if err := os.RemoveAll(deep); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, deep); err != nil {
		t.Fatal(err)
	}
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out, "--force")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(string(env.Data), `"setup/deep"`) {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
	if got := filesIn(t, elsewhere); !slices.Equal(got, []string{"page.md"}) {
		t.Fatalf("written behind the link: %q", got)
	}

	// Another folder's export is not this one's to clean up.
	env, r = e.runJSON("", "docs", "export", setupChapterID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "holds the export of the folder "+f.Folder.ID) {
		t.Fatalf("error %q", env.Error)
	}
}

// In HTML a page is the stored HTML in a minimal page, the headings with
// their anchors as ids, the links and the picture as in Markdown.
func TestABookIsExportedAsHTML(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--format", "html")

	want := []string{
		".minka-export.json", "etc-passwd.html", "index.html", "readme-2.html",
		"setup/deep/index.html", "setup/deep/ueber-groesse.html", "setup/index.html",
		"setup/tools-2.html", "setup/tools.assets/plan.png", "setup/tools.html", "site-plan.pdf", "what-the-tent-costs.html",
	}
	if got := filesIn(t, out); !slices.Equal(got, want) {
		t.Fatalf("files %q, want %q", got, want)
	}
	page := readFile(t, filepath.Join(out, "setup", "tools.html"))
	for _, piece := range []string{
		"<!doctype html>", "<title>Tools</title>", `<meta name="minka:id" content="` + toolsPageID + `">`,
		`<h2 id="h-zugang-pruefen">Zugang prüfen</h2>`, `<h2 id="h-zugang-pruefen-2">Zugang prüfen</h2>`,
		`<a href="../what-the-tent-costs.html#h-prices">What the tent costs</a>`,
		`<a href="` + e.server.URL + `/projects/` + f.Project.ID + `/files/` + f.Files.ID + `/` + f.Document.ID + `">How the stage goes up</a>`,
		`<a href="tools-2.html#h-hammer">the other tools</a>`, `<img src="tools.assets/plan.png" alt="The plan"/>`,
		`<aside data-callout="warning">`, `<pre data-language="mermaid">`,
	} {
		if !strings.Contains(page, piece) {
			t.Fatalf("page lacks %q:\n%s", piece, page)
		}
	}
	if index := readFile(t, filepath.Join(out, "index.html")); !strings.Contains(index, `<li><a href="setup/tools-2.html">Tools</a> (not in navigation)</li>`) {
		t.Fatalf("index:\n%s", index)
	}

	// Another format into the same directory takes the place of the first.
	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if len(data.Removed) != 9 || slices.Contains(filesIn(t, out), "index.html") {
		t.Fatalf("removed %q", data.Removed)
	}
}

// A folder in no book is read as a tree, the folder and its tree in two
// requests however deep it goes, its uploads downloaded beside the pages; one
// in the archive as well.
func TestAFolderInNoBookIsReadAsATree(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	out := filepath.Join(t.TempDir(), "plans")

	env := runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	data := decodeExport(t, env.Data)
	if data.IsBook || data.Uploads != 1 || data.Documents != 1 || !strings.HasPrefix(env.Summary, "Exported folder Plans to ") {
		t.Fatalf("data %+v, summary %q", data, env.Summary)
	}
	if got := filesIn(t, out); !slices.Equal(got, []string{".minka-export.json", "README.md", "site-plan.pdf", "what-the-tent-costs.md"}) {
		t.Fatalf("files %q", got)
	}
	index := readFile(t, filepath.Join(out, "README.md"))
	if !strings.Contains(index, `kind: "folder"`) || !strings.Contains(index, "- [site-plan.pdf](site-plan.pdf) (file)") {
		t.Fatalf("index:\n%s", index)
	}
	var paths []string
	for _, request := range e.server.Requests() {
		if request.Method == "GET" && !strings.HasPrefix(request.Path, "/api/v1/documents/") && !strings.HasPrefix(request.Path, "/api/v1/uploads/") {
			paths = append(paths, request.Path)
		}
	}
	if want := []string{"/api/v1/files-folders/" + f.Folder.ID, "/api/v1/files-folders/" + f.Folder.ID + "/tree"}; !slices.Equal(paths, want) {
		t.Fatalf("the tree was read with %q", paths)
	}
	if crumbFor(t, env.Breadcrumbs, "refresh") != "minka docs export "+f.Folder.ID+" --out "+shellWord(out) {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}

	archived := filepath.Join(t.TempDir(), "last-year")
	data = decodeExport(t, runOK(t, e, "docs", "export", f.ArchivedFolder.ID, "--out", archived).Data)
	if data.Documents != 1 || !slices.Equal(filesIn(t, archived), []string{".minka-export.json", "README.md", "what-the-tent-cost-last-year.md"}) {
		t.Fatalf("an archived folder: %+v", data)
	}
}

// A Minka without the tree of a folder answers its address with a page of its
// web: a book is read as a book, without uploads, and every page is read on
// every run, since the book names no version; a folder in no book is walked
// through its tool, and without the tool refused with the way to name it.
func TestAnOlderMinkaReadsABookOrWalksAFolder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.KnowsNoTree = true
	out := filepath.Join(t.TempDir(), "plans")

	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "--tool <toolId>") || !strings.Contains(env.Hint, "older") {
		t.Fatalf("hint %q", env.Hint)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("a refused export made its directory")
	}

	env = runOK(t, e, "docs", "export", f.Folder.ID, "--tool", f.Files.ID, "--out", out)
	data := decodeExport(t, env.Data)
	if data.IsBook || data.Uploads != 1 || data.Documents != 1 || !slices.Equal(filesIn(t, out), []string{".minka-export.json", "README.md", "site-plan.pdf", "what-the-tent-costs.md"}) {
		t.Fatalf("data %+v", data)
	}
	if crumbFor(t, env.Breadcrumbs, "refresh") != "minka docs export "+f.Folder.ID+" --tool "+f.Files.ID+" --out "+shellWord(out) {
		t.Fatalf("breadcrumbs %v", env.Breadcrumbs)
	}

	f.Folder.IsBook = true
	book := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", book)
	again := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", book).Data)
	if !again.IsBook || again.Uploads != 0 || again.DocumentsRead != 1 || again.Unchanged != 2 {
		t.Fatalf("a book of an older Minka: %+v", again)
	}
}

// What the CLI can tell by itself goes nowhere.
func TestAnExportNeedsADirectoryAndAFormat(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	noRequestGoesOut(t, e, "docs", "export", f.Folder.ID)
	noRequestGoesOut(t, e, "docs", "export", f.Folder.ID, "--out", t.TempDir(), "--format", "pdf")
	noRequestGoesOut(t, e, "docs", "export", "plans", "--out", t.TempDir())
	noRequestGoesOut(t, e, "docs", "export", f.Folder.ID, "--out", t.TempDir(), "--tool", "documents")
}

// The text names the counts, every file written or removed, and where to
// start reading.
func TestTheTextOfAnExportSaysWhereToStart(t *testing.T) {
	b := newExportBook(t)
	out := filepath.Join(t.TempDir(), "book")
	text := b.e.run("", "docs", "export", deepChapterID, "--out", out, "--styled")
	want := "Exported chapter Deep to " + out + ": 2 written, 0 unchanged, 0 removed\n" +
		"Format: markdown\nFolders: 1, pages: 1 (1 read), uploads: 0, files in pages: 0\n" +
		"  + README.md\n  + ueber-groesse.md\nStart at: " + filepath.Join(out, "README.md") + "\n"
	if text.stdout != want {
		t.Fatalf("text:\n%s\nwant:\n%s", text.stdout, want)
	}
}
