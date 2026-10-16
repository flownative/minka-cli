package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/export"
	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/localfile"
)

// editManifest changes the manifest of an export as a person or a broken
// program might.
func editManifest(t *testing.T, out string, change func(m map[string]any)) {
	t.Helper()
	name := filepath.Join(out, ".minka-export.json")
	var m map[string]any
	if err := json.Unmarshal([]byte(readFile(t, name)), &m); err != nil {
		t.Fatal(err)
	}
	change(m)
	raw, _ := json.Marshal(m)
	writeFile(t, name, string(raw), 0o644)
}

func sha256Of(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func modeOf(t *testing.T, name string) os.FileMode {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// A new file and directory get the mode any program gives one under the
// umask; a file written over keeps the mode its owner gave it.
func TestAnExportKeepsTheModesOfItsFiles(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	base := t.TempDir()
	out := filepath.Join(base, "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	reference := filepath.Join(base, "reference")
	if err := os.WriteFile(reference, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	referenceDir := filepath.Join(base, "reference-dir")
	if err := os.Mkdir(referenceDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if got, want := modeOf(t, filepath.Join(out, "README.md")), modeOf(t, reference); got != want {
		t.Fatalf("a new file has %v, want %v", got, want)
	}
	if got, want := modeOf(t, filepath.Join(out, "setup")), modeOf(t, referenceDir); got != want {
		t.Fatalf("a new directory has %v, want %v", got, want)
	}

	// Closed to others, and open to all beyond what the umask leaves.
	page, open := filepath.Join(out, "what-the-tent-costs.md"), filepath.Join(out, "setup", "tools.md")
	for name, mode := range map[string]os.FileMode{page: 0o600, open: 0o666} {
		if err := os.Chmod(name, mode); err != nil {
			t.Fatal(err)
		}
	}
	b.costs.Edit(`<p>Changed.</p>`)
	b.tools.Edit(`<p>Changed.</p>`)
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	for name, mode := range map[string]os.FileMode{page: 0o600, open: 0o666} {
		if !strings.Contains(readFile(t, name), "Changed.") || modeOf(t, name) != mode {
			t.Fatalf("%s was not written, or lost its mode: %v", name, modeOf(t, name))
		}
	}
}

// What a manifest says is held against the directory: a directory outside
// it, another host, a version this minka does not know, a file of it that is
// now a link, and a directory of it that is now a link.
func TestAManifestIsHeldAgainstTheDirectory(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	base := t.TempDir()
	out := filepath.Join(base, "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	original := readFile(t, filepath.Join(out, ".minka-export.json"))
	restore := func() { writeFile(t, filepath.Join(out, ".minka-export.json"), original, 0o644) }

	// An empty directory beside the export, which an rmdir would take.
	beside := filepath.Join(base, "x")
	if err := os.Mkdir(beside, 0o755); err != nil {
		t.Fatal(err)
	}
	editManifest(t, out, func(m map[string]any) { m["directories"] = append(m["directories"].([]any), "../x") })
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out, "--force")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, `names "../x", which lies outside it`) {
		t.Fatalf("error %q", env.Error)
	}
	if _, err := os.Stat(beside); err != nil {
		t.Fatalf("a directory beside the export was removed: %v", err)
	}

	restore()
	editManifest(t, out, func(m map[string]any) { m["host"] = "https://elsewhere.example" })
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "holds an export from https://elsewhere.example") {
		t.Fatalf("error %q", env.Error)
	}

	restore()
	editManifest(t, out, func(m map[string]any) { m["version"] = 2 })
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "is no manifest this minka can read") {
		t.Fatalf("error %q", env.Error)
	}

	// A page of the export that became a link to a file outside.
	restore()
	outside := filepath.Join(base, "outside.md")
	writeFile(t, outside, "outside", 0o644)
	page := filepath.Join(out, "readme-2.md")
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, page); err != nil {
		t.Fatal(err)
	}
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", out, "--force")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(string(env.Data), `"readme-2.md"`) || readFile(t, outside) != "outside" {
		t.Fatalf("error %q, data %s", env.Error, env.Data)
	}
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}

	// A file of the manifest gone from Minka that is now a link, and a
	// directory of it that is now a link to an empty directory.
	if err := os.Symlink(outside, filepath.Join(out, "gone.md")); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(empty, filepath.Join(out, "linked")); err != nil {
		t.Fatal(err)
	}
	editManifest(t, out, func(m map[string]any) {
		m["files"] = append(m["files"].([]any), map[string]any{"path": "gone.md", "kind": "document", "id": newToolsID, "sha256": sha256Of("outside")})
		m["directories"] = append(m["directories"].([]any), "linked")
	})
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out, "--force")
	if readFile(t, outside) != "outside" {
		t.Fatal("the file behind the link was changed")
	}
	for _, link := range []string{"gone.md", "linked"} {
		if info, err := os.Lstat(filepath.Join(out, link)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s was taken away: %v", link, err)
		}
	}
}

// --out names a directory, and only one run writes into it at a time.
func TestAnExportNeedsADirectoryOfItsOwn(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	file := filepath.Join(t.TempDir(), "book")
	writeFile(t, file, "a file", 0o644)
	env, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", file)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "is no directory") || readFile(t, file) != "a file" {
		t.Fatalf("error %q", env.Error)
	}

	// The lock lies beside the config, named after the directory itself, so
	// another name of it meets the same lock.
	base := t.TempDir()
	out := filepath.Join(base, "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(out, alias); err != nil {
		t.Fatal(err)
	}
	lockPath, err := export.LockPath(filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "export-locks"), out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("no lock beside the config: %v", err)
	}
	lock, err := localfile.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	b.costs.Edit(`<p>Changed.</p>`)
	env, r = e.runJSON("", "docs", "export", f.Folder.ID, "--out", alias)
	lock.Release()
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "another export is writing into") || strings.Contains(readFile(t, filepath.Join(out, "what-the-tent-costs.md")), "Changed.") {
		t.Fatalf("error %q", env.Error)
	}
}

// A run that fails after its first write leaves a manifest that names every
// file it may have written, so the next run carries on without --force and
// takes away what the broken one left while writing.
func TestARunThatFailedHalfwayIsCarriedOn(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	b.picture.Swept = true
	_, r := e.runJSON("", "docs", "export", f.Folder.ID, "--out", out)
	if r.code == ExitOK {
		t.Fatal("an export without its picture went through")
	}
	if manifest := readFile(t, filepath.Join(out, ".minka-export.json")); !strings.Contains(manifest, `"path": "setup/tools.assets/plan.png"`) {
		t.Fatalf("the manifest does not name the picture it was about to write:\n%s", manifest)
	}

	b.picture.Swept = false
	writeFile(t, filepath.Join(out, ".minka-export-1234"), "half", 0o600)
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	if got := filesIn(t, out); len(got) != 12 || !slices.Contains(got, "setup/tools.assets/plan.png") || slices.Contains(got, ".minka-export-1234") {
		t.Fatalf("files %q", got)
	}
}

// A file of Minka other than the one written is downloaded, however alike the
// bytes on disk are to what was written.
func TestAnotherFileOfMinkaIsDownloadedAgain(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)

	newer := &fakeapi.Upload{ID: newToolsID, Circle: f.Project.ID, Filename: "Plan.PNG", ContentType: "image/png",
		Content: []byte("another picture"), Size: 15, UploaderID: f.Bruno.ID, StandsInAText: true}
	e.server.Uploads = append(e.server.Uploads, newer)
	b.tools.Edit(strings.Replace(b.tools.Body, b.picture.Handle(), newer.Handle(), 1))
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	if readFile(t, filepath.Join(out, "setup", "tools.assets", "plan.png")) != "another picture" {
		t.Fatal("the picture was not downloaded again")
	}
}

// An address on another host names nothing of the export, however much it
// looks like one of Minka's.
func TestALinkToAnotherHostStaysAsItIs(t *testing.T) {
	b := newExportBook(t)
	e, f := b.e, b.e.fixture
	out := filepath.Join(t.TempDir(), "book")
	// The scheme of the host signed in to, so that only the host differs.
	elsewhere := strings.Split(e.server.URL, "://")[0] + "://elsewhere.example/go/item/" + b.costs.ID
	b.tools.Edit(`<p><a href="` + elsewhere + `">elsewhere</a> and <a href="/go/item/` + b.costs.ID + `">here</a></p>`)
	runOK(t, e, "docs", "export", f.Folder.ID, "--out", out)
	page := readFile(t, filepath.Join(out, "setup", "tools.md"))
	if !strings.Contains(page, "[elsewhere]("+elsewhere+")") || !strings.Contains(page, "[here](../what-the-tent-costs.md)") {
		t.Fatalf("page:\n%s", page)
	}
}
