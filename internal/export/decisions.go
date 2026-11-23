package export

import (
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// holdsDecisionBlocks says whether a body has a decisions block. Its rows are
// read afresh with the page and change without it, so such a page is never
// taken from an earlier run.
func holdsDecisionBlocks(body string) bool {
	return strings.Contains(strings.ToLower(body), "<"+markdown.DecisionsElement)
}

// readOutDecisions puts at each decisions block of a page its marker, as a
// comment, and its rows as the Markdown file shows them: a quote that says it
// is no part of the text.
func readOutDecisions(container *xhtml.Node, blocks []api.DecisionBlock) {
	byPosition := map[int]api.DecisionBlock{}
	for _, block := range blocks {
		byPosition[block.Position] = block
	}
	var elements []*xhtml.Node
	var collect func(*xhtml.Node)
	collect = func(n *xhtml.Node) {
		if richtext.IsDecisionsElement(n) {
			elements = append(elements, n)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(container)

	for position, element := range elements {
		marker := richtext.DecisionsMarker(element)
		if marker == "" {
			element.Parent.RemoveChild(element)
			continue
		}
		element.Parent.InsertBefore(&xhtml.Node{Type: xhtml.CommentNode, Data: strings.TrimSuffix(strings.TrimPrefix(marker, "<!--"), "-->")}, element)
		block, found := byPosition[position]
		if !found || !strings.EqualFold(block.ToolID, strings.TrimSpace(attribute(element, "tool"))) {
			element.Parent.RemoveChild(element)
			continue
		}
		replaceWith(element, rowsAsAQuote(block))
	}
}

func rowsAsAQuote(block api.DecisionBlock) *xhtml.Node {
	lead, items := richtext.DecisionRowWords(block)
	quote := &xhtml.Node{Type: xhtml.ElementNode, Data: "blockquote", DataAtom: atom.Blockquote}
	paragraph := &xhtml.Node{Type: xhtml.ElementNode, Data: "p", DataAtom: atom.P}
	paragraph.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: markdown.ReadOutLead + " " + lead})
	quote.AppendChild(paragraph)
	if len(items) > 0 {
		list := &xhtml.Node{Type: xhtml.ElementNode, Data: "ul", DataAtom: atom.Ul}
		for _, item := range items {
			entry := &xhtml.Node{Type: xhtml.ElementNode, Data: "li", DataAtom: atom.Li}
			entry.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: item})
			list.AppendChild(entry)
		}
		quote.AppendChild(list)
	}
	return quote
}
