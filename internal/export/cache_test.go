package export

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

func versionOf(number int) *int { return &number }

// A page is taken from what an earlier run read only where the tree names the
// same version at the same moment, and where every reference in it leads to a
// page or an upload of the export: Minka gives a reference elsewhere the title
// it has when the page is read.
func TestOnlyTheSameVersionWithReferencesInsideIsTakenAgain(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	page := &node{kind: kindDocument, id: "page", title: "Page", parent: root, updatedAt: "2026-10-16T10:00:00.000000Z", version: versionOf(3)}
	other := &node{kind: kindDocument, id: "other", title: "Other", parent: root}
	file := &node{kind: kindUpload, id: "file", title: "File", parent: root}
	root.children = []*node{page, other, file}
	inside := entriesBelow(root)
	cached := func(body string, version int, updatedAt string) map[string]cachedPage {
		return map[string]cachedPage{"page": {Version: version, UpdatedAt: updatedAt, Document: pageAsRead{Body: body, UpdatedAt: "2026-10-16T10:00:00.000000Z"}}}
	}
	moment := page.updatedAt

	for _, c := range []struct {
		name   string
		cache  map[string]cachedPage
		usable bool
	}{
		{"the same version", cached(`<p>Here.</p>`, 3, moment), true},
		{"references inside", cached(`<p><minka-ref kind="document" id="OTHER"></minka-ref> <minka-ref kind="upload" id="file"></minka-ref></p>`, 3, moment), true},
		{"another version", cached(`<p>Here.</p>`, 2, moment), false},
		{"another moment", cached(`<p>Here.</p>`, 3, "2026-10-16T10:00:01.000000Z"), false},
		{"a page outside", cached(`<p><minka-ref kind="document" id="elsewhere"></minka-ref></p>`, 3, moment), false},
		{"a card", cached(`<p><minka-ref kind="card" id="other"></minka-ref></p>`, 3, moment), false},
		{"a folder of the export", cached(`<minka-ref kind="folder" id="root"></minka-ref>`, 3, moment), false},
		{"nothing read", nil, false},
	} {
		document, usable := cachedPageOf(c.cache, page, inside)
		if usable != c.usable {
			t.Errorf("%s: usable %v", c.name, usable)
		}
		if usable && (document.Title != "Page" || document.ID != "page" || document.Body != c.cache["page"].Document.Body) {
			t.Errorf("%s: document %+v", c.name, document)
		}
	}

	// A client's tree names no version: the page is read again.
	page.version = nil
	if _, usable := cachedPageOf(cached(`<p>Here.</p>`, 3, moment), page, inside); usable {
		t.Error("a page without a version was taken from the cache")
	}
}

// recordingSource is a tree of two pages; the one it is told is gone answers
// 404, and every read is noted.
type recordingSource struct {
	stubSource
	gone  string
	mu    sync.Mutex
	reads []string
}

func (s *recordingSource) Document(_ context.Context, id string) (api.Result[api.Document], error) {
	s.mu.Lock()
	s.reads = append(s.reads, id)
	s.mu.Unlock()
	if id == s.gone {
		return api.Result[api.Document]{}, &api.Error{StatusCode: http.StatusNotFound}
	}
	return api.Result[api.Document]{Value: api.Document{ID: id, Body: "<p>Fresh.</p>"}}, nil
}

// A page taken from the cache that refers to a page gone when it is read is
// read again: its reference leads out of the export now, and only Minka knows
// what it shows.
func TestACachedPageReferringToAPageGoneIsReadAgain(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	moment := "2026-10-16T10:00:00.000000Z"
	referring := &node{kind: kindDocument, id: "referring", parent: root, updatedAt: moment, version: versionOf(1)}
	unrelated := &node{kind: kindDocument, id: "unrelated", parent: root, updatedAt: moment, version: versionOf(1)}
	gone := &node{kind: kindDocument, id: "gone", parent: root, updatedAt: moment, version: versionOf(1)}
	root.children = []*node{referring, unrelated, gone}
	cache := map[string]cachedPage{
		"referring": {Version: 1, UpdatedAt: moment, Document: pageAsRead{Body: `<minka-ref kind="document" id="gone"></minka-ref>`}},
		"unrelated": {Version: 1, UpdatedAt: moment, Document: pageAsRead{Body: `<p>Cached.</p>`}},
	}
	source := &recordingSource{gone: "gone"}

	left, read, err := readDocuments(context.Background(), source, root, 2, cache)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(source.reads)
	if !slices.Equal(source.reads, []string{"gone", "referring"}) || read != 2 {
		t.Fatalf("reads %q, counted %d", source.reads, read)
	}
	if len(left) != 1 || left[0] != gone || len(root.children) != 2 {
		t.Fatalf("gone %v, children %d", left, len(root.children))
	}
	if referring.fromCache || referring.document.Body != "<p>Fresh.</p>" || !unrelated.fromCache || unrelated.document.Body != "<p>Cached.</p>" {
		t.Fatalf("referring %+v, unrelated %+v", referring.document, unrelated.document)
	}
}
