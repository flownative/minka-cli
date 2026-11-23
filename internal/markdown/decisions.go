package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// A decisions block is a stored query on the decisions of one Decisions tool:
// Minka keeps the element with identifiers only and lists the rows afresh on
// every reading. Markdown writes it as an HTML comment on a line of its own,
// which every Markdown tool hides.
const (
	// DecisionsElement is the element Minka stores for a decisions block.
	DecisionsElement = "minka-decisions"

	// ReadOutLead opens the quote of rows a reading view prints for a
	// decisions block. Such a quote is never part of a text, and a text that
	// holds one is refused.
	ReadOutLead = "Read out, not part of the text:"

	// DecisionsMarkerForm is how a decisions marker is written, for a
	// refusal and the help.
	DecisionsMarkerForm = `<!-- minka:decisions tool="<id>" label="<id>" status="<id>" decided-in="<id>" applies-to="<id>" -->`
)

// DecisionsAttributes are the attributes of a decisions block in the order
// the marker is written in: the tool, which is required, then every filter.
var DecisionsAttributes = []string{"tool", "label", "status", "decided-in", "applies-to"}

// RefusalReadOutRows is the refusal of a text that holds the rows a reading
// view printed: they can only have come in by copying.
const RefusalReadOutRows = "the text holds rows read out for a decisions block (a quote that opens with \"" + ReadOutLead + "\"); they are no part of the text"

var (
	decisionsMarkerStart = regexp.MustCompile(`^<!--\s*minka:decisions(?:\s|-->|$)`)
	decisionsMarker      = regexp.MustCompile(`^<!--\s*minka:decisions((?:\s+[^\s="]+="[^"]*")*)\s*-->$`)
	decisionsAttribute   = regexp.MustCompile(`([^\s="]+)="([^"]*)"`)
	identifier           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// htmlBlockContent is the source of an HTML block, its closing line included.
func htmlBlockContent(block *ast.HTMLBlock, source []byte) []byte {
	var content bytes.Buffer
	lines := block.Lines()
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		content.Write(segment.Value(source))
	}
	if block.HasClosure() {
		content.Write(block.ClosureLine.Value(source))
	}
	return bytes.TrimSpace(content.Bytes())
}

// isDecisionsMarker reports whether an HTML block is meant as a decisions
// marker, well formed or not.
func isDecisionsMarker(node ast.Node, source []byte) bool {
	block, ok := node.(*ast.HTMLBlock)
	return ok && block.HTMLBlockType == ast.HTMLBlockType2 && decisionsMarkerStart.Match(htmlBlockContent(block, source))
}

// DecisionsElementOf writes the element of a decisions marker, or says why
// the marker cannot be one. Only the form is checked here: whether the
// identifiers name a tool, label, status, event or card of the project is
// Minka's to say. A value is lowercased, as Minka writes identifiers, and an
// empty one is left out, as Minka leaves it out.
func DecisionsElementOf(marker string) (string, error) {
	marker = strings.TrimSpace(marker)
	if end := strings.Index(marker, "-->"); end >= 0 && end+len("-->") < len(marker) {
		return "", fmt.Errorf("a decisions marker stands alone on its line, and nothing may follow its -->: %q", marker[end+len("-->"):])
	}
	match := decisionsMarker.FindStringSubmatch(marker)
	if match == nil {
		return "", fmt.Errorf(`a decisions marker is written %s: each attribute as name="value", the value in double quotes`, DecisionsMarkerForm)
	}
	values := map[string]string{}
	for _, attribute := range decisionsAttribute.FindAllStringSubmatch(match[1], -1) {
		name, value := attribute[1], strings.ToLower(strings.TrimSpace(attribute[2]))
		if !isDecisionsAttribute(name) && isDecisionsAttribute(strings.ToLower(name)) {
			return "", fmt.Errorf("the attributes of a decisions block are written in lower case: %q, not %q", strings.ToLower(name), name)
		}
		if !isDecisionsAttribute(name) {
			return "", fmt.Errorf("a decisions block has no attribute %q; it takes %s", name, strings.Join(DecisionsAttributes, ", "))
		}
		if _, twice := values[name]; twice {
			return "", fmt.Errorf("the %q of a decisions block is given twice", name)
		}
		if value != "" && !identifier.MatchString(value) {
			return "", fmt.Errorf("the %q of a decisions block has to be an identifier, such as 0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", name)
		}
		values[name] = value
	}
	if values["tool"] == "" {
		return "", fmt.Errorf(`a decisions block names its Decisions tool in "tool"`)
	}
	element := "<" + DecisionsElement
	for _, name := range DecisionsAttributes {
		if value := values[name]; value != "" {
			element += " " + name + `="` + value + `"`
		}
	}
	return element + "></" + DecisionsElement + ">", nil
}

func isDecisionsAttribute(name string) bool {
	for _, known := range DecisionsAttributes {
		if name == known {
			return true
		}
	}
	return false
}

// decisionsBlock writes a decisions marker as its element and reports
// whether the HTML block was one.
func (c *converter) decisionsBlock(block *ast.HTMLBlock) bool {
	if !isDecisionsMarker(block, c.source) {
		return false
	}
	element, err := DecisionsElementOf(string(htmlBlockContent(block, c.source)))
	if err != nil {
		c.refuse(err.Error())
		return true
	}
	c.write(element + "\n")
	return true
}

// opensWithReadOutRows reports whether a quote is rows a reading view
// printed for a decisions block.
func opensWithReadOutRows(quote *ast.Blockquote, source []byte) bool {
	paragraph, ok := quote.FirstChild().(*ast.Paragraph)
	if !ok || paragraph.Lines().Len() == 0 {
		return false
	}
	first := paragraph.Lines().At(0)
	return bytes.HasPrefix(bytes.TrimSpace(first.Value(source)), []byte(ReadOutLead))
}

// refuse keeps the first reason a text cannot be sent.
func (c *converter) refuse(reason string) {
	if c.refusal == "" {
		c.refusal = reason
	}
}
