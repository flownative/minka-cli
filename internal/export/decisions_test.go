package export

import (
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	pageTool  = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	otherTool = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a6c"
)

func renderedWithDecisions(t *testing.T, body string, blocks []api.DecisionBlock) string {
	t.Helper()
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(body), context)
	if err != nil {
		t.Fatal(err)
	}
	container := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	for _, n := range nodes {
		container.AppendChild(n)
	}
	readOutDecisions(container, blocks)
	var out strings.Builder
	for n := container.FirstChild; n != nil; n = n.NextSibling {
		_ = xhtml.Render(&out, n)
	}
	return out.String()
}

// A page in HTML carries each block's marker as a comment and its rows as a
// quote; rows go only where position and tool are the block's own.
func TestAnHTMLPageCarriesTheRowsOfItsOwnBlocksOnly(t *testing.T) {
	body := `<minka-decisions tool="` + pageTool + `"></minka-decisions><p>Between</p><minka-decisions tool="` + pageTool + `"></minka-decisions>`
	blocks := []api.DecisionBlock{
		{Position: 0, ToolID: pageTool, Tool: &api.DecisionsToolReference{ID: pageTool, Title: "Decisions"}, Summary: "Decisions",
			Decisions: []api.DecisionBlockEntry{{Title: "We meet <on> Mondays", Status: api.DecisionStatus{Name: "In force"}}}},
		{Position: 1, ToolID: otherTool, Tool: &api.DecisionsToolReference{ID: otherTool, Title: "Other"}, Summary: "Other"},
	}

	page := renderedWithDecisions(t, body, blocks)
	want := `<!-- minka:decisions tool="` + pageTool + `" --><blockquote><p>` + markdown.ReadOutLead + ` Decisions</p><ul><li>We meet &lt;on&gt; Mondays — In force</li></ul></blockquote>` +
		`<p>Between</p><!-- minka:decisions tool="` + pageTool + `" -->`
	if page != want {
		t.Fatalf("page\n%s\nwant\n%s", page, want)
	}
}
