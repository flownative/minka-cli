package export

import (
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// A page's title is the tree's, as the index shows it, also where the body
// kept was read under another title.
func TestAPageTakesItsTitleFromTheTree(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	page := &node{kind: kindDocument, id: "page", title: "New title", stem: "new-title", parent: root, appURL: "https://minka.example/page",
		document: &api.Document{ID: "page", Title: "Old title", AppURL: "https://minka.example/old", Body: "<p>Text.</p>"}}
	root.children = []*node{page}
	for _, format := range []string{FormatMarkdown, FormatHTML} {
		out := string(newLayout(format, nil, root).renderDocument(page).content)
		if strings.Contains(out, "Old title") || strings.Contains(out, "/old") || !strings.Contains(out, "New title") {
			t.Fatalf("%s:\n%s", format, out)
		}
	}
}

// An upload the tree names without its file is left out, as an entry this
// export cannot write.
func TestAnUploadWithoutItsFileIsLeftOut(t *testing.T) {
	root := &node{kind: kindFolder, id: "root"}
	nodes := nodesOfBook([]api.BookEntry{
		{Kind: "upload", ID: "bare", Title: "bare.pdf"},
		{Kind: "upload", ID: "file", Title: "plan.pdf", File: &api.Attachment{ID: "blob", Filename: "plan.pdf"}},
	}, root)
	if len(nodes) != 1 || nodes[0].id != "file" || nodes[0].file.ID != "blob" {
		t.Fatalf("nodes %+v", nodes)
	}
}
