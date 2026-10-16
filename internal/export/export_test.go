package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
)

// A title becomes a name that is no path, holds no dot and does not differ
// from another in case alone; where nothing of it is left it is "untitled".
func TestATitleBecomesASafeName(t *testing.T) {
	for title, want := range map[string]string{
		"../../etc/passwd":       "etc-passwd",
		"..":                     untitled,
		"   ":                    untitled,
		".hidden":                "hidden",
		`C:\Windows\System32`:    "c-windows-system32",
		"Über Größe":             "ueber-groesse",
		"Plan v2.1 (final)":      "plan-v2-1-final",
		"한국":                     "한국",
		strings.Repeat("ab", 60): strings.Repeat("ab", 40),
		strings.Repeat("ä", 50):  strings.Repeat("ae", 40),
	} {
		got := stemOf(title)
		if got != want {
			t.Errorf("stemOf(%q) = %q, want %q", title, got, want)
		}
		if !isLocal(got) || strings.ContainsAny(got, "./\\") {
			t.Errorf("stemOf(%q) = %q is no plain name", title, got)
		}
	}
}

// Two entries of one directory never get the same name, also where a file
// system ignores case, and no entry takes the index or a device's name.
func TestTheEntriesOfADirectoryGetNamesOfTheirOwn(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	titles := []struct{ kind, title, filename string }{
		{kindDocument, "Setup", ""}, {kindFolder, "setup", ""}, {kindDocument, "SETUP", ""},
		{kindDocument, "README", ""}, {kindDocument, "Index", ""}, {kindDocument, "con", ""},
		{kindUpload, "Setup.pdf", "setup.PDF"}, {kindDocument, "Setup 2", ""}, {kindDocument, "Setup", ""},
		// Lower case alone tells the two apart; a file system that ignores
		// case does not.
		{kindDocument, "Λόγος", ""}, {kindDocument, "ΛΟΓΟΣ", ""},
	}
	for i, entry := range titles {
		root.children = append(root.children, &node{kind: entry.kind, id: string(rune('a' + i)), title: entry.title, file: api.Attachment{Filename: entry.filename}, parent: root})
	}
	nameTree(root, nil)
	var stems []string
	for _, n := range root.children {
		stems = append(stems, n.stem)
	}
	want := []string{"setup", "setup-2", "setup-3", "readme-2", "index-2", "con-2", "setup-4", "setup-2-2", "setup-5", "λογος", "λογοσ-2"}
	if !slices.Equal(stems, want) {
		t.Fatalf("stems %q, want %q", stems, want)
	}
	l := newLayout(FormatMarkdown, nil, root)
	if got := l.pathOf(root.children[6]); got != "setup-4.pdf" {
		t.Fatalf("upload at %q", got)
	}
}

// A name an earlier run gave stays with its entry while its title still gives
// it and nobody took it first.
func TestAnEntryKeepsItsEarlierName(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	newer := &node{kind: kindDocument, id: "new", title: "Setup", parent: root}
	older := &node{kind: kindDocument, id: "old", title: "Setup", parent: root}
	renamed := &node{kind: kindDocument, id: "renamed", title: "Other", parent: root}
	root.children = []*node{newer, older, renamed}
	nameTree(root, map[string]earlierName{
		"document/old":     {dir: "", stem: "setup"},
		"document/renamed": {dir: "", stem: "setup-2"},
	})
	if newer.stem != "setup-2" || older.stem != "setup" || renamed.stem != "other" {
		t.Fatalf("stems %q %q %q", newer.stem, older.stem, renamed.stem)
	}

	// A name given in another directory is no name here.
	moved := &node{kind: kindDocument, id: "moved", title: "Setup", parent: root}
	root.children = []*node{moved}
	nameTree(root, map[string]earlierName{"document/moved": {dir: "chapter", stem: "setup-2"}})
	if moved.stem != "setup" {
		t.Fatalf("stem %q", moved.stem)
	}
}

// A link from one file to another goes up and down the tree as far as needed.
func TestALinkBetweenTwoFilesIsRelative(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"", "a.md", "a.md"},
		{"", "setup/a.md", "setup/a.md"},
		{"setup", "a.md", "../a.md"},
		{"setup/deep", "setup/a.md", "../a.md"},
		{"setup/deep", "other/deep/a.md", "../../other/deep/a.md"},
		{"setup", "setup/a.assets/p.png", "a.assets/p.png"},
	} {
		if got := relative(c.from, c.to); got != c.want {
			t.Errorf("relative(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

// Only a clean relative path inside the directory belongs to an export.
func TestOnlyAPathInsideBelongsToTheExport(t *testing.T) {
	for p, want := range map[string]bool{
		"a.md": true, "setup/a.md": true, "a.assets/p.png": true,
		"": false, "../a.md": false, "/etc/passwd": false, "a/../../b": false, "a//b": false, "./a": false,
		`a\b`: false, ManifestName: false, "..": false,
	} {
		if isLocal(p) != want {
			t.Errorf("isLocal(%q) = %v", p, !want)
		}
	}
}

// stubSource answers with a book of two pages, one of which is gone when it
// is read.
type stubSource struct{}

func (stubSource) BookOf(context.Context, string) (api.Result[api.Book], error) {
	return api.Result[api.Book]{Value: api.Book{ID: "book", Title: "Book", Entries: []api.BookEntry{
		{Kind: "page", ID: "kept", Title: "Kept", InNavigation: true},
		{Kind: "page", ID: "gone", Title: "Gone", InNavigation: true},
	}}}, nil
}

func (stubSource) FilesEntriesOf(context.Context, string, string) (api.Result[api.FilesEntries], error) {
	return api.Result[api.FilesEntries]{}, &api.Error{StatusCode: http.StatusNotFound}
}

func (stubSource) Document(_ context.Context, id string) (api.Result[api.Document], error) {
	if id == "gone" {
		return api.Result[api.Document]{}, &api.Error{StatusCode: http.StatusNotFound}
	}
	return api.Result[api.Document]{Value: api.Document{ID: id, Title: "Kept", Body: "<p>Here.</p>"}}, nil
}

func (stubSource) FileBytes(context.Context, string, string) (*api.FileBytes, error) {
	return &api.FileBytes{Body: io.NopCloser(strings.NewReader(""))}, nil
}

// A page gone between the tree and its body is left out and named, and the
// index does not list it.
func TestAPageGoneMeanwhileIsLeftOut(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book")
	report, err := Run(context.Background(), stubSource{}, Options{FolderID: "book", Out: out, Parallel: 2, LockDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Gone) != 1 || report.Gone[0].ID != "gone" || report.Documents != 1 {
		t.Fatalf("report %+v", report)
	}
	index, err := os.ReadFile(filepath.Join(out, "README.md"))
	if err != nil || strings.Contains(string(index), "Gone") || !strings.Contains(string(index), "- [Kept](kept.md)") {
		t.Fatalf("index %s, %v", index, err)
	}
}

// failingSource is a book of one page with a picture whose download breaks
// off.
type failingSource struct{ stubSource }

func (failingSource) BookOf(context.Context, string) (api.Result[api.Book], error) {
	return api.Result[api.Book]{Value: api.Book{ID: "book", Title: "Book", Entries: []api.BookEntry{{Kind: "page", ID: "kept", Title: "Kept", InNavigation: true}}}}, nil
}

func (failingSource) Document(_ context.Context, id string) (api.Result[api.Document], error) {
	return api.Result[api.Document]{Value: api.Document{ID: id, Title: "Kept",
		Body:        `<action-text-attachment sgid="blob--signature"></action-text-attachment>`,
		Attachments: []api.Attachment{{ID: "blob", Filename: "p.png", IsImage: true}}}}, nil
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func (failingSource) FileBytes(context.Context, string, string) (*api.FileBytes, error) {
	return &api.FileBytes{Body: io.NopCloser(brokenBody{})}, nil
}

// A download that breaks off is the network's failure, not the disk's.
func TestADownloadThatBreaksOffIsANetworkFailure(t *testing.T) {
	_, err := Run(context.Background(), failingSource{}, Options{FolderID: "book", Out: filepath.Join(t.TempDir(), "book"), Parallel: 1, LockDir: t.TempDir()})
	var network *api.NetworkError
	var write *WriteError
	if !errors.As(err, &network) || errors.As(err, &write) {
		t.Fatalf("error %T %v", err, err)
	}
}

// A reference whose anchor has not the shape of one links the page without
// it.
func TestOnlyAnAnchorGoesIntoALink(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	target := &node{kind: kindDocument, id: "target", title: "Target", stem: "target", parent: root}
	from := &node{kind: kindDocument, id: "from", title: "From", stem: "from", parent: root, document: &api.Document{
		Body: `<p><minka-ref kind="document" id="target" anchor="comment-1"></minka-ref> <minka-ref kind="document" id="target" anchor="h-a"></minka-ref></p>`}}
	root.children = []*node{target, from}
	page := string(newLayout(FormatMarkdown, nil, root).renderDocument(from).content)
	if !strings.Contains(page, "[Target](target.md) [Target](target.md#h-a)") {
		t.Fatalf("page:\n%s", page)
	}
}

// A plan that would write outside the directory is refused before anything.
func TestAPlanOutsideTheDirectoryIsRefused(t *testing.T) {
	out := t.TempDir()
	w := newWriter(out, nil, true)
	var refusal *RefusalError
	if err := w.check([]planned{{path: "../escape.md"}}, nil); !errors.As(err, &refusal) {
		t.Fatalf("error %v", err)
	}
}

// countingSource holds the reads of pages until four are open at once, or a
// fifth comes, and counts how many were open at most.
type countingSource struct {
	stubSource
	mu         sync.Mutex
	open       int
	most       int
	full       chan struct{} // closed once four are open
	beyond     chan struct{} // closed once a fifth is
	released   chan struct{}
	fullOnce   sync.Once
	beyondOnce sync.Once
}

func (c *countingSource) Document(ctx context.Context, id string) (api.Result[api.Document], error) {
	c.mu.Lock()
	c.open++
	c.most = max(c.most, c.open)
	switch c.open {
	case 4:
		c.fullOnce.Do(func() { close(c.full) })
	case 5:
		c.beyondOnce.Do(func() { close(c.beyond) })
	}
	c.mu.Unlock()
	<-c.released
	c.mu.Lock()
	c.open--
	c.mu.Unlock()
	return api.Result[api.Document]{Value: api.Document{ID: id}}, nil
}

// No more pages are read at once than asked for, and as many as asked for.
func TestPagesAreReadAFewAtATime(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	for i := range 12 {
		root.children = append(root.children, &node{kind: kindDocument, id: fmt.Sprint(i), parent: root})
	}
	source := &countingSource{full: make(chan struct{}), beyond: make(chan struct{}), released: make(chan struct{})}
	done := make(chan error)
	go func() {
		_, err := readDocuments(context.Background(), source, root, 4)
		done <- err
	}()

	// Once four are open, a fifth would come at once if anything let it; the
	// pause only gives it the chance, the test does not wait on it to pass.
	select {
	case <-source.full:
	case <-time.After(10 * time.Second):
		t.Fatal("four pages were never read at once")
	}
	select {
	case <-source.beyond:
	case <-time.After(100 * time.Millisecond):
	}
	close(source.released)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the reads never ended")
	}
	if source.most != 4 {
		t.Fatalf("%d pages read at once", source.most)
	}
}

// What a run that ended left without noting it is refused as maybe changed:
// written over and removed with --force, never taken out of the manifest
// while it lies there. Bytes the plan writes are no change, and a file the
// manifest does not name is nobody's to claim, however alike.
func TestAFileOfARunThatEndedIsWrittenOverOnlyWithForce(t *testing.T) {
	out := t.TempDir()
	for name, content := range map[string]string{"same.md": "planned", "other.md": "a person's", "known.md": "planned", "picture.png": "bytes", "dropped.md": "dropped"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := &manifest{Files: []manifestFile{
		{Path: "same.md"}, {Path: "other.md"}, {Path: "picture.png", BlobID: "blob"}, {Path: "dropped.md"},
		// Written over right before the run ended: the old hash, the new bytes.
		{Path: "known.md", SHA256: hashOf([]byte("earlier"))},
	}}
	plan := []planned{
		{path: "same.md", content: []byte("planned")}, {path: "other.md", content: []byte("planned")},
		{path: "known.md", content: []byte("planned")}, {path: "picture.png", blobID: "blob"},
	}

	var refusal *RefusalError
	err := newWriter(out, old, false).check(plan, nil)
	if !errors.As(err, &refusal) || !slices.Equal(refusal.Paths, []string{"dropped.md", "other.md", "picture.png"}) ||
		!strings.Contains(refusal.Message, "may have been changed since it was written") {
		t.Fatalf("error %v", err)
	}

	w := newWriter(out, old, true)
	if err := w.check(plan, nil); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if err := w.removeStale(plan, nil); err != nil || !slices.Equal(w.removed, []string{"dropped.md"}) {
		t.Fatalf("removed %q, %v", w.removed, err)
	}

	// Without --force nothing goes, and the file stays in the manifest.
	if err := os.WriteFile(filepath.Join(out, "dropped.md"), []byte("dropped"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = newWriter(out, &manifest{Files: []manifestFile{{Path: "dropped.md"}}}, false)
	if err := w.check(nil, nil); !errors.As(err, &refusal) || !strings.Contains(refusal.Message, "may have been changed since it was written") {
		t.Fatalf("error %v", err)
	}
	if _, known := w.current["dropped.md"]; !known {
		t.Fatal("the file left the manifest")
	}

	w = newWriter(out, &manifest{}, false)
	if err := w.check([]planned{{path: "same.md", content: []byte("planned")}}, nil); !errors.As(err, &refusal) || !strings.Contains(refusal.Message, "in the way") {
		t.Fatalf("a file of somebody else with the planned bytes was claimed: %v", err)
	}
}

// DEL and the C1 controls, which YAML takes in no quoted string, are escaped.
func TestTheFrontmatterEscapesWhatYAMLRefuses(t *testing.T) {
	got := frontmatter([]field{{"title", "a\u007fb\u0085c\u009fd\u00a0e\u0001"}})
	want := "---\ntitle: \"a\\u007fb\\u0085c\\u009fd\u00a0e\\u0001\"\n---\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// inspectingSource reads the manifest on disk while a picture is downloaded.
type inspectingSource struct {
	failingSource
	out      string
	manifest string
}

func (s *inspectingSource) FileBytes(context.Context, string, string) (*api.FileBytes, error) {
	raw, _ := os.ReadFile(filepath.Join(s.out, ManifestName))
	s.manifest = string(raw)
	return &api.FileBytes{Body: io.NopCloser(strings.NewReader("picture"))}, nil
}

// The manifest on disk names every file of the plan before the first is
// written, so a run killed in the middle leaves no file it does not name.
func TestTheManifestNamesAFileBeforeItIsWritten(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book")
	source := &inspectingSource{out: out}
	if _, err := Run(context.Background(), source, Options{FolderID: "book", Out: out, Parallel: 1, LockDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source.manifest, `"path": "kept.assets/p.png"`) || !strings.Contains(source.manifest, `"path": "kept.md"`) {
		t.Fatalf("manifest during the run:\n%s", source.manifest)
	}
}
