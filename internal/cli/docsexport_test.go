package cli

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// The entries of the book the export tests read, with identifiers that say
// what they are.
const (
	setupChapterID = "0192f3a4-5b6c-4d7e-8f90-0000000e0001"
	deepChapterID  = "0192f3a4-5b6c-4d7e-8f90-0000000e0002"
	toolsPageID    = "0192f3a4-5b6c-4d7e-8f90-0000000e0003"
	otherToolsID   = "0192f3a4-5b6c-4d7e-8f90-0000000e0004"
	readmePageID   = "0192f3a4-5b6c-4d7e-8f90-0000000e0005"
	escapingPageID = "0192f3a4-5b6c-4d7e-8f90-0000000e0006"
	umlautPageID   = "0192f3a4-5b6c-4d7e-8f90-0000000e0007"
	pictureBlobID  = "0192f3a4-5b6c-4d7e-8f90-0000000e0008"
	newToolsID     = "0192f3a4-5b6c-4d7e-8f90-0000000e0009"
)

// exportBook makes the folder of the fixture a book: a chapter with two pages
// of the same title, one of them with a picture, references inside and
// outside the book and the shapes of a document, a chapter below it, and at
// the top pages whose titles are a reserved name and a path.
type exportBook struct {
	e       *testEnv
	tools   *fakeapi.Document
	other   *fakeapi.Document
	costs   *fakeapi.Document
	picture *fakeapi.Upload
}

func newExportBook(t *testing.T) *exportBook {
	t.Helper()
	e := newTestEnv(t)
	f := e.fixture
	tool := f.Files
	book := f.Folder
	book.IsBook = true
	moment := f.DocumentInFolder.UpdatedAt

	picture := &fakeapi.Upload{ID: pictureBlobID, Circle: f.Project.ID, Filename: "Plan.PNG", ContentType: "image/png",
		Content: onePixelPNG(), Size: len(onePixelPNG()), UploaderID: f.Bruno.ID, StandsInAText: true}
	e.server.Uploads = append(e.server.Uploads, picture)

	f.DocumentInFolder.Body = `<h2>Prices</h2><p>Four hundred.</p>`
	page := func(id, title, folderID, body string) *fakeapi.Document {
		return &fakeapi.Document{ID: id, Title: title, FolderID: folderID, Body: body, CreatedAt: moment, UpdatedAt: moment, CreatorID: f.Bruno.ID}
	}
	tools := page(toolsPageID, "Tools", setupChapterID,
		`<h2>Zugang prüfen</h2><p>See <minka-ref kind="document" id="`+f.DocumentInFolder.ID+`" anchor="h-prices"></minka-ref>, `+
			`<minka-ref kind="document" id="`+f.Document.ID+`"></minka-ref> and <a href="`+e.server.URL+`/go/item/`+otherToolsID+`#h-hammer">the other tools</a>.</p>`+
			`<action-text-attachment sgid="`+picture.Handle()+`" caption="The plan" presentation="gallery"></action-text-attachment>`+
			`<aside data-callout="warning"><header>Careful</header><p>Sharp.</p></aside>`+
			`<pre data-language="mermaid">graph LR<br>A --&gt; B</pre>`+
			`<h2>Zugang prüfen</h2><p>Again.</p>`)
	other := page(otherToolsID, "Tools", setupChapterID, `<h2>Hammer</h2>`)
	other.OutOfNavigation = true
	tool.Folders = append(tool.Folders,
		&fakeapi.Folder{ID: setupChapterID, Title: "Setup", ParentID: book.ID, CreatedAt: moment, CreatorID: f.Bruno.ID},
		&fakeapi.Folder{ID: deepChapterID, Title: "Deep", ParentID: setupChapterID, CreatedAt: moment, CreatorID: f.Bruno.ID})
	tool.Documents = append(tool.Documents, tools, other,
		page(readmePageID, "README", book.ID, `<p>Not the index.</p>`),
		page(escapingPageID, "../../etc/passwd", book.ID, `<p>Nowhere else.</p>`),
		page(umlautPageID, "Über Größe", deepChapterID, `<p>Groß.</p>`))
	tool.Arranged = map[string][]string{
		book.ID:        {setupChapterID, f.DocumentInFolder.ID, readmePageID, escapingPageID},
		setupChapterID: {toolsPageID, otherToolsID, deepChapterID},
		deepChapterID:  {umlautPageID},
	}
	return &exportBook{e: e, tools: tools, other: other, costs: f.DocumentInFolder, picture: picture}
}

// filesIn lists what a directory holds, relative and with forward slashes.
func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, _ := filepath.Rel(dir, p)
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

type exportData struct {
	Out       string   `json:"out"`
	IsBook    bool     `json:"isBook"`
	Folders   int      `json:"folders"`
	Documents int      `json:"documents"`
	Uploads   int      `json:"uploads"`
	Assets    int      `json:"assets"`
	Written   []string `json:"written"`
	Unchanged int      `json:"unchanged"`
	// DocumentsRead are the pages read from Minka.
	DocumentsRead int      `json:"documentsRead"`
	Removed       []string `json:"removed"`
}

func decodeExport(t *testing.T, raw json.RawMessage) exportData {
	t.Helper()
	var data exportData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("data %s: %s", raw, err)
	}
	return data
}

// A book goes into a directory as Minka orders it: a directory per chapter, a
// file per page and per upload, an index in each, the picture beside its page,
// the headings with Minka's anchors, a reference inside the book a link to the
// file at its section, one outside its address in Minka.
func TestABookIsExportedAsATreeOfFiles(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "plans")

	env := runOK(t, e, "docs", "export", setupChapterID, "--out", out)
	data := decodeExport(t, env.Data)
	if !data.IsBook || data.Folders != 2 || data.Documents != 3 || data.Assets != 1 || data.Uploads != 0 || data.Unchanged != 0 || len(data.Removed) != 0 {
		t.Fatalf("data %+v", data)
	}
	if env.Summary != "Exported chapter Setup to "+out+": 6 written, 0 unchanged, 0 removed" {
		t.Fatalf("summary %q", env.Summary)
	}

	// The whole book from the book itself.
	whole := filepath.Join(t.TempDir(), "book")
	env = runOK(t, e, "docs", "export", f.Folder.ID, "--out", whole)
	want := []string{
		".minka-export.json", "README.md", "etc-passwd.md", "readme-2.md",
		"setup/README.md", "setup/deep/README.md", "setup/deep/ueber-groesse.md",
		"setup/tools-2.md", "setup/tools.assets/plan.png", "setup/tools.md", "site-plan.pdf", "what-the-tent-costs.md",
	}
	if got := filesIn(t, whole); !slices.Equal(got, want) {
		t.Fatalf("files %q, want %q", got, want)
	}
	if env.Summary != "Exported book Plans to "+whole+": 11 written, 0 unchanged, 0 removed" {
		t.Fatalf("summary %q", env.Summary)
	}

	index := readFile(t, filepath.Join(whole, "README.md"))
	wantIndex := `---
title: "Plans"
id: "` + f.Folder.ID + `"
appUrl: "` + e.server.URL + `/projects/` + f.Project.ID + `/files/` + f.Files.ID + `/folders/` + f.Folder.ID + `"
kind: "book"
order: "manual"
book: "Plans"
bookId: "` + f.Folder.ID + `"
---

# Plans

- [Setup](setup/README.md)
  - [Tools](setup/tools.md)
  - [Tools](setup/tools-2.md) (not in navigation)
  - [Deep](setup/deep/README.md)
    - [Über Größe](setup/deep/ueber-groesse.md)
- [What the tent costs](what-the-tent-costs.md)
- [README](readme-2.md)
- [../../etc/passwd](etc-passwd.md)
- [site-plan.pdf](site-plan.pdf) (file)
`
	if index != wantIndex {
		t.Fatalf("index:\n%s\nwant:\n%s", index, wantIndex)
	}

	page := readFile(t, filepath.Join(whole, "setup", "tools.md"))
	wantPage := `---
title: "Tools"
id: "` + toolsPageID + `"
appUrl: "` + e.server.URL + `/projects/` + f.Project.ID + `/files/` + f.Files.ID + `/` + toolsPageID + `"
inNavigation: true
updatedAt: "` + decode(t, runOK(t, e, "docs", "show", toolsPageID).Data)["updatedAt"].(string) + `"
---

# Tools

<a id="h-zugang-pruefen"></a>
## Zugang prüfen

See [What the tent costs](../what-the-tent-costs.md#h-prices), <` + e.server.URL + `/projects/` + f.Project.ID + `/files/` + f.Files.ID + `/` + f.Document.ID + `> and [the other tools](tools-2.md#h-hammer).

![The plan](tools.assets/plan.png)

> [!WARNING] Careful
>
> Sharp.

` + "```mermaid\ngraph LR\nA --> B\n```" + `

<a id="h-zugang-pruefen-2"></a>
## Zugang prüfen

Again.
`
	if page != wantPage {
		t.Fatalf("page:\n%s\nwant:\n%s", page, wantPage)
	}
	if picture := readFile(t, filepath.Join(whole, "setup", "tools.assets", "plan.png")); picture != string(onePixelPNG()) {
		t.Fatal("the picture is not the file Minka holds")
	}
	if requestsTo(e, "GET", "/api/v1/uploads/"+pictureBlobID+"/original") != 2 {
		t.Fatalf("the picture was not downloaded once per export: %d", requestsTo(e, "GET", "/api/v1/uploads/"+pictureBlobID+"/original"))
	}
}
