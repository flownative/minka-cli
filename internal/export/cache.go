package export

import (
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// cachedPage is a page's body as Minka handed it out, kept in the manifest so
// that the next run need not read a page Minka has not changed since. Its
// file is rendered anew from it on every run, against the tree of that run:
// a page whose file depends on another page's title or place follows that
// page without being read again.
type cachedPage struct {
	// Version and UpdatedAt are what the tree named when the body was read.
	// The body is at least as new: the tree is read before the pages. Every
	// change of a page's title or body makes a new version.
	Version   int        `json:"version"`
	UpdatedAt string     `json:"updatedAt"`
	Document  pageAsRead `json:"document"`
}

// pageAsRead is what a page's file takes from the page as read; its title,
// address and place come from the tree.
type pageAsRead struct {
	UpdatedAt   string           `json:"updatedAt"`
	Body        string           `json:"body"`
	Attachments []api.Attachment `json:"attachments"`
}

// cachedPageOf is the page an earlier run read, where it is the version the
// tree names now and its references lead only into the export. Minka gives a
// reference the title and address of its target as they stand when the page
// is read, so one that leads elsewhere — a card, a post, a page outside —
// may have changed without the page; a reference into the export is
// rendered from this run's tree instead.
func cachedPageOf(cache map[string]cachedPage, n *node, inside map[string]*node) (*api.Document, bool) {
	if n.version == nil || n.updatedAt == "" {
		return nil, false
	}
	page, found := cache[n.id]
	if !found || page.Version != *n.version || page.UpdatedAt != n.updatedAt || !referencesStayInside(page.Document.Body, inside) {
		return nil, false
	}
	return &api.Document{
		ID: n.id, Title: n.title, AppURL: n.appURL,
		Body: page.Document.Body, UpdatedAt: page.Document.UpdatedAt, Attachments: page.Document.Attachments,
	}, true
}

// cacheOf is what the manifest keeps of the pages this run exports: those of
// a version the tree named, read now or earlier.
func cacheOf(root *node) map[string]cachedPage {
	cache := map[string]cachedPage{}
	root.each(func(n *node) {
		if n.kind != kindDocument || n.document == nil || n.version == nil || n.updatedAt == "" {
			return
		}
		cache[n.id] = cachedPage{Version: *n.version, UpdatedAt: n.updatedAt, Document: pageAsRead{
			UpdatedAt: n.document.UpdatedAt, Body: n.document.Body, Attachments: n.document.Attachments,
		}}
	})
	return cache
}

// referencesStayInside says whether every reference in a body names a page
// or an upload of the export.
func referencesStayInside(body string, inside map[string]*node) bool {
	if !strings.Contains(body, "<minka-ref") {
		return true
	}
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(body), context)
	if err != nil {
		return false
	}
	var stays func(n *xhtml.Node) bool
	stays = func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && n.Data == "minka-ref" {
			kind := attribute(n, "kind")
			if kind != kindDocument && kind != kindUpload || inside[keyOf(kind, attribute(n, "id"))] == nil {
				return false
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if !stays(child) {
				return false
			}
		}
		return true
	}
	for _, n := range nodes {
		if !stays(n) {
			return false
		}
	}
	return true
}
