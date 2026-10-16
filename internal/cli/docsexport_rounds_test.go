package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// planReplacedByAnUpload exports a walked folder with a chapter "Plan", then
// puts the chapter into the trash and an upload of the same name without an
// extension in its place, before the run that is to meet it.
func planReplacedByAnUpload(t *testing.T) (*testEnv, string) {
	t.Helper()
	e := newTestEnv(t)
	f := e.fixture
	out := filepath.Join(t.TempDir(), "plans")
	moment := f.DocumentInFolder.UpdatedAt
	chapter := &fakeapi.Folder{ID: setupChapterID, Title: "Plan", ParentID: f.Folder.ID, CreatedAt: moment, CreatorID: f.Bruno.ID}
	f.Files.Folders = append(f.Files.Folders, chapter)
	f.Files.Documents = append(f.Files.Documents, &fakeapi.Document{ID: toolsPageID, Title: "Inside", FolderID: setupChapterID,
		Body: `<p>In.</p>`, CreatedAt: moment, UpdatedAt: moment, CreatorID: f.Bruno.ID})
	runOK(t, e, "docs", "export", f.Folder.ID, "--tool", f.Files.ID, "--out", out)
	if !slices.Contains(filesIn(t, out), "plan/inside.md") {
		t.Fatalf("files %q", filesIn(t, out))
	}

	chapter.Trashed = true
	file := &fakeapi.Upload{ID: pictureBlobID, Circle: f.Project.ID, Filename: "plan", ContentType: "application/octet-stream",
		Content: []byte("plan"), Size: 4, UploaderID: f.Bruno.ID}
	e.server.Uploads = append(e.server.Uploads, file)
	f.Files.Uploads = append(f.Files.Uploads, &fakeapi.FilesUpload{ID: otherToolsID, Title: "Plan", BlobID: file.ID, FolderID: f.Folder.ID,
		CreatedAt: moment, UpdatedAt: moment, CreatorID: f.Bruno.ID})
	return e, out
}

// A directory of the export whose place a file takes, an upload without an
// extension named as the chapter was, goes when it holds nothing but files
// of the export that go.
func TestAFileTakesThePlaceOfADirectoryOfTheExport(t *testing.T) {
	e, out := planReplacedByAnUpload(t)
	runOK(t, e, "docs", "export", e.fixture.Folder.ID, "--tool", e.fixture.Files.ID, "--out", out)
	if readFile(t, filepath.Join(out, "plan")) != "plan" {
		t.Fatalf("files %q", filesIn(t, out))
	}
}

// A directory of the export in a file's way stays, and refuses the run, as
// soon as it holds a file of somebody else, a link, or a directory the
// export did not make.
func TestADirectoryInAFilesWayStaysWhereItHoldsMore(t *testing.T) {
	for name, put := range map[string]func(t *testing.T, dir string){
		"a file of somebody else": func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "mine.md"), "mine", 0o644) },
		"a link in place of a file of the export": func(t *testing.T, dir string) {
			outside := filepath.Join(t.TempDir(), "outside.md")
			writeFile(t, outside, "outside", 0o644)
			if err := os.Remove(filepath.Join(dir, "inside.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, "inside.md")); err != nil {
				t.Fatal(err)
			}
		},
		"a directory of somebody else": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "mine"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, out := planReplacedByAnUpload(t)
			put(t, filepath.Join(out, "plan"))
			env, r := e.runJSON("", "docs", "export", e.fixture.Folder.ID, "--tool", e.fixture.Files.ID, "--out", out, "--force")
			expectCode(t, r, ExitUsage)
			if !strings.Contains(string(env.Data), `"plan"`) || readFile(t, filepath.Join(out, "plan", "inside.md")) == "" {
				t.Fatalf("error %q, data %s", env.Error, env.Data)
			}
		})
	}
}

// A person's change to a file of the export is still seen after a run that
// died halfway, which named the file in the manifest without writing it.
func TestAChangeAfterAFailedRunIsStillSeen(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	// Another file in the same place, which is to be downloaded and is not.
	newer := &fakeapi.Upload{ID: newToolsID, Circle: f.Project.ID, Filename: "Plan.PNG", ContentType: "image/png",
		Content: []byte("another picture"), Size: 15, UploaderID: f.Bruno.ID, StandsInAText: true, Swept: true}
	e.server.Uploads = append(e.server.Uploads, newer)
	b.tools.Edit(strings.Replace(b.tools.Body, b.picture.Handle(), newer.Handle(), 1))
	if _, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out); r.code == ExitOK {
		t.Fatal("an export without its picture went through")
	}

	newer.Swept = false
	picture := filepath.Join(out, "setup", "tools.assets", "plan.png")
	writeFile(t, picture, "drawn over by hand", 0o644)
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "1 file the export wrote in "+out+" has been changed since") || !strings.Contains(string(env.Data), `"setup/tools.assets/plan.png"`) || readFile(t, picture) != "drawn over by hand" {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
}

// A directory without a manifest keeps even what looks like a file a run
// left.
func TestTheLeftoversAreTheExportsOnly(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	other := filepath.Join(t.TempDir(), "book")
	writeFile(t, filepath.Join(other, ".minka-export-notes"), "mine", 0o644)
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", other, "--force")
	if readFile(t, filepath.Join(other, ".minka-export-notes")) != "mine" {
		t.Fatal("a file in a directory without a manifest was taken for a leftover")
	}
}

// A file that landed in a run killed before it noted it is refused as maybe
// changed, never denied as the export's, and written over with --force.
func TestAFileOfAKilledRunIsWrittenOverWithForce(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	b.picture.Swept = true
	if _, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out); r.code == ExitOK {
		t.Fatal("an export without its picture went through")
	}
	b.picture.Swept = false
	picture := filepath.Join(out, "setup", "tools.assets", "plan.png")
	writeFile(t, picture, "landed", 0o644)

	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "may have been changed since it was written") || !strings.Contains(env.Hint, "--force") || readFile(t, picture) != "landed" {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force")
	if readFile(t, picture) != string(onePixelPNG()) {
		t.Fatal("--force did not write the picture")
	}
}

// A directory in a file's way that holds a file named without a hash stays:
// such a file is never removed unasked.
func TestADirectoryInAFilesWayKeepsAFileNamedWithoutAHash(t *testing.T) {
	e, out := planReplacedByAnUpload(t)
	editManifest(t, out, func(m map[string]any) {
		for _, entry := range m["files"].([]any) {
			if entry := entry.(map[string]any); entry["path"] == "plan/inside.md" {
				entry["sha256"] = ""
			}
		}
	})
	env, r := e.runJSON("", "docs", "export", e.fixture.Folder.ID, "--tool", e.fixture.Files.ID, "--out", out, "--force")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(string(env.Data), `"plan"`) || readFile(t, filepath.Join(out, "plan", "inside.md")) == "" {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
}

// An entry of a kind a later Minka sends in a book is left out of the tree,
// the counts and the export, and the rest is read as ever.
func TestAnEntryOfAnUnknownKindIsLeftOutOfABook(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	e.server.LaterBookEntries = []any{map[string]any{"kind": "diagram", "id": newToolsID, "title": "flow.svg", "appUrl": e.server.URL + "/x"}}

	env := runOK(t, e, "docs", "book", f.Folder.ID)
	if env.Summary != "Book Plans: 2 chapters and 6 pages" {
		t.Fatalf("summary %q", env.Summary)
	}
	if text := e.run("", "docs", "book", f.Folder.ID, "--styled"); strings.Contains(text.stdout, "flow.svg") {
		t.Fatalf("text:\n%s", text.stdout)
	}
	out := filepath.Join(t.TempDir(), "book")
	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if data.Documents != 6 || strings.Contains(readFile(t, filepath.Join(out, "README.md")), "flow") {
		t.Fatalf("data %+v", data)
	}
	if requestsTo(e, "GET", "/api/v1/documents/"+newToolsID) != 0 {
		t.Fatal("the entry of the unknown kind was read as a page")
	}
}
