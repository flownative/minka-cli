package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rewrite of the stored texts by Minka's maintenance makes no version, so a
// run takes the page from the manifest as it was; --read-all reads every page
// again and keeps what it read for the next run.
func TestReadAllReadsAPageMinkaRewroteWithoutAVersion(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	page := filepath.Join(out, "what-the-tent-costs.md")

	b.costs.Body = `<h2>Prices</h2><p>Four hundred, tidied.</p>`
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	if strings.Contains(readFile(t, page), "tidied") {
		t.Fatal("a page of the same version was read again without --read-all")
	}

	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--read-all").Data)
	if data.DocumentsRead != data.Documents || !strings.Contains(readFile(t, page), "tidied") {
		t.Fatalf("--read-all: %+v", data)
	}
	data = decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
	if !strings.Contains(readFile(t, page), "tidied") || data.DocumentsRead != 1 {
		t.Fatalf("the next run went back to the body before: %+v", data)
	}
}

// Pages kept in another format are read as none: every page is read again,
// and the manifest holds its files as ever.
func TestPagesKeptInAnotherFormatAreReadAgain(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	for _, format := range []any{2.0, nil} {
		editManifest(t, out, func(m map[string]any) {
			if format == nil {
				delete(m, "pagesFormat")
			} else {
				m["pagesFormat"] = format
			}
		})
		data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out).Data)
		if data.DocumentsRead != data.Documents || len(data.Written) != 0 || data.Unchanged != 11 {
			t.Fatalf("pages in format %v: %+v", format, data)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(out, ".minka-export.json")), `"pagesFormat": 1`) {
		t.Fatal("the manifest names no format of its pages")
	}
}

// An export read from a tree is not shrunk to what a Minka without trees
// gives on one such answer: the run is refused and nothing changes, and
// --force exports the way that Minka allows.
func TestAnExportFromATreeIsNotShrunkOnAnAnswerWithoutTrees(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	e.server.KnowsNoTree = true
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "did not answer as a Minka that reads the tree") || !strings.Contains(env.Hint, "--force") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
	if _, err := os.Stat(filepath.Join(out, "site-plan.pdf")); err != nil || requestsTo(e, "GET", "/api/v1/files-folders/"+f.Folder.ID+"/book") != 0 {
		t.Fatalf("the refused run changed the export: %v", err)
	}

	data := decodeExport(t, runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force").Data)
	if data.Uploads != 0 || len(data.Removed) != 1 || data.Removed[0] != "site-plan.pdf" {
		t.Fatalf("--force: %+v", data)
	}
	// Read as a book now, it is brought up to date as one.
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
}

// A Minka without trees that finds no book for the folder either says both,
// not that the folder is missing.
func TestAnOlderMinkaWithoutABookSaysSo(t *testing.T) {
	e := newTestEnv(t)
	e.server.KnowsNoTree = true
	env, r := e.runJSON("", "docs", "export", e.fixture.Folder.ID, "--out", filepath.Join(t.TempDir(), "plans"))
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "answers as a Minka without the tree of a folder, and finds no book the folder stands in") {
		t.Fatalf("error %q", env.Error)
	}
}
