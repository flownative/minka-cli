package richtext

import (
	"fmt"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// rowsNamespace carries the read-out rows of a block on its element while a
// text is converted. The parser gives an attribute of an HTML element no
// namespace, so no stored text can bring one along.
const rowsNamespace = "minka-cli"

// ToMarkdownWithDecisions is ToMarkdown for a text that is read and not sent
// back, such as an exported page: below each decisions marker it prints that
// block's rows, as a quote that says it is no part of the text. The way back
// into HTML refuses a text that holds such a quote.
func ToMarkdownWithDecisions(fragment string, decisionBlocks []api.DecisionBlock) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		return fragment
	}
	WithDecisionRows(nodes, decisionBlocks)
	return blocks(nodes)
}

// WithDecisionRows puts each block's rows on its element, by position, for
// the conversion into Markdown to print below the marker.
func WithDecisionRows(nodes []*html.Node, decisionBlocks []api.DecisionBlock) {
	byPosition := map[int]api.DecisionBlock{}
	for _, block := range decisionBlocks {
		byPosition[block.Position] = block
	}
	position := 0
	var number func(*html.Node)
	number = func(n *html.Node) {
		if IsDecisionsElement(n) {
			// A block of another query belongs to another text than this one,
			// and its rows would stand at the wrong place.
			if block, found := byPosition[position]; found && strings.EqualFold(block.ToolID, strings.TrimSpace(attribute(n, "tool"))) {
				n.Attr = append(n.Attr, html.Attribute{Namespace: rowsNamespace, Key: "rows", Val: DecisionRows(block)})
			}
			position++
		}
		for _, child := range children(n) {
			number(child)
		}
	}
	for _, n := range nodes {
		number(n)
	}
}

// IsDecisionsElement reports whether a node is the element of a decisions
// block.
func IsDecisionsElement(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Namespace == "" && n.Data == markdown.DecisionsElement
}

// DecisionsMarker is the marker a decisions block is written as, its
// attributes in a fixed order, so that reading and writing back keeps the
// element as it was. An element without a tool, which Minka never stores,
// has none.
func DecisionsMarker(n *html.Node) string {
	if strings.TrimSpace(attribute(n, "tool")) == "" {
		return ""
	}
	marker := "<!-- minka:decisions"
	for _, name := range markdown.DecisionsAttributes {
		value := strings.TrimSpace(attribute(n, name))
		if value == "" || strings.ContainsAny(value, `"<>`) {
			continue
		}
		marker += " " + name + `="` + value + `"`
	}
	return marker + " -->"
}

// decisionsBlock writes a decisions block as its marker and below it the
// rows where a reading view put them in.
func decisionsBlock(n *html.Node) string {
	marker := DecisionsMarker(n)
	if marker == "" {
		return ""
	}
	for _, attr := range n.Attr {
		if attr.Namespace == rowsNamespace && attr.Val != "" {
			return marker + "\n" + attr.Val
		}
	}
	return marker
}

// DecisionRows are the rows of a decisions block as a reading view prints
// them: a quote that opens with the words saying it is no part of the text,
// then the summary and one line per decision.
func DecisionRows(block api.DecisionBlock) string {
	lead, items := DecisionRowWords(block)
	lines := []string{"> " + markdown.ReadOutLead + " " + escapeText(lead)}
	for _, item := range items {
		lines = append(lines, "> - "+escapeText(item))
	}
	return strings.Join(lines, "\n")
}

// DecisionRowWords are the words of a block's rows, unescaped, from the
// structured fields: the summary, and a line per decision. A block of a tool
// the reader cannot see lists nothing.
func DecisionRowWords(block api.DecisionBlock) (string, []string) {
	if block.Tool == nil {
		return "(a Decisions tool you cannot see)", nil
	}
	summary := block.Summary
	if block.Frozen {
		summary += " (as locked)"
	}
	var items []string
	for _, decision := range block.Decisions {
		items = append(items, oneLineOfWords(decisionLine(decision)))
	}
	if len(items) == 0 {
		items = []string{"(no decisions)"}
	}
	return oneLineOfWords(summary), items
}

// decisionLine is one decision of a block in the order of Minka's own list:
// short identifier and title, status, votes, the note beside them and what
// changed since a lock.
func decisionLine(decision api.DecisionBlockEntry) string {
	first := decision.Title + " — " + decision.Status.Name
	if decision.ShortID != nil && *decision.ShortID != "" {
		first = *decision.ShortID + " " + first
	}
	parts := []string{first}
	if decision.VotesFor != nil || decision.VotesAgainst != nil || decision.Abstentions != nil {
		parts = append(parts, fmt.Sprintf("%d for, %d against, %d abstaining", count(decision.VotesFor), count(decision.VotesAgainst), count(decision.Abstentions)))
	}
	if decision.VotesNote != nil && *decision.VotesNote != "" {
		parts = append(parts, *decision.VotesNote)
	}
	if today := decision.Today; today != nil {
		switch {
		case today.SupersededBy != nil && today.SupersededBy.ShortID != nil && *today.SupersededBy.ShortID != "":
			parts = append(parts, "today: superseded by "+*today.SupersededBy.ShortID)
		case today.SupersededBy != nil:
			parts = append(parts, "today: superseded by "+today.SupersededBy.Title)
		case today.Status != nil:
			parts = append(parts, "today: "+today.Status.Name)
		}
		if today.Reworded {
			parts = append(parts, "today: reworded")
		}
	}
	return strings.Join(parts, " · ")
}

func count(votes *int) int {
	if votes == nil {
		return 0
	}
	return *votes
}

func oneLineOfWords(text string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(text, " "))
}
