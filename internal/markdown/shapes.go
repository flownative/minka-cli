package markdown

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// The shapes of a document that Markdown has no element for are written in
// conventions other Markdown tools read as well: a callout is a GitHub alert,
// a group of link tiles a list and a block of variables a table, each of the
// last two under an HTML comment that names it. Elsewhere the comment is
// hidden and the list or table is shown as such.
const (
	MarkerTiles     = "<!-- minka:tiles -->"
	MarkerVariables = "<!-- minka:variables -->"

	// The words of a callout's kind, as a GitHub alert names them.
	AlertNote    = "NOTE"
	AlertWarning = "WARNING"
)

const (
	WarningTileWithoutAddress = "a link tile starts with an address or a #short identifier of the record it names; a line without one was sent as a paragraph"
	WarningTileOfOneLine      = "a link tile is one line; a list under the tiles marker with more in an item was sent as a list"
	WarningEmptyTile          = "an empty line under the tiles marker was left out"
)

const (
	markerTiles     = "tiles"
	markerVariables = "variables"
)

var markerComment = regexp.MustCompile(`^<!--\s*minka:(tiles|variables)\s*-->$`)

// markerOf names the shape an HTML block marks, or nothing.
func markerOf(node ast.Node, source []byte) string {
	if node == nil {
		return ""
	}
	block, ok := node.(*ast.HTMLBlock)
	if !ok || block.HTMLBlockType != ast.HTMLBlockType2 {
		return ""
	}
	var content bytes.Buffer
	lines := block.Lines()
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		content.Write(segment.Value(source))
	}
	if block.HasClosure() {
		content.Write(block.ClosureLine.Value(source))
	}
	match := markerComment.FindSubmatch(bytes.TrimSpace(content.Bytes()))
	if match == nil {
		return ""
	}
	return string(match[1])
}

// marks reports whether a marker of that kind gives a block its shape.
// Minka keeps no block of variables inside a callout and unwraps it there,
// so such a marker gives nothing its shape.
func marks(kind string, node ast.Node) bool {
	switch node.(type) {
	case *ast.List:
		return kind == markerTiles
	case *east.Table:
		return kind == markerVariables && !standsInACallout(node)
	}
	return false
}

// markerBefore names the shape the HTML comment right before a block gives
// it, or nothing where there is none or it names another shape.
func (c *converter) markerBefore(node ast.Node) string {
	if node == nil {
		return ""
	}
	kind := markerOf(node.PreviousSibling(), c.source)
	if kind == "" || !marks(kind, node) {
		return ""
	}
	return kind
}

// isMarkerOfTheNextBlock reports whether an HTML block is a marker the block
// after it takes its shape from. Such a marker writes nothing itself.
func isMarkerOfTheNextBlock(block *ast.HTMLBlock, source []byte) bool {
	kind := markerOf(block, source)
	return kind != "" && block.NextSibling() != nil && marks(kind, block.NextSibling())
}

// callout is a quote that starts with an alert marker: the callout of that
// kind, with the words beside the marker as its title.
type callout struct {
	ast.BaseBlock
	kind string
}

var kindCallout = ast.NewNodeKind("MinkaCallout")

func (n *callout) Kind() ast.NodeKind { return kindCallout }

func (n *callout) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Kind": n.kind}, nil)
}

type calloutTitle struct {
	ast.BaseBlock
}

var kindCalloutTitle = ast.NewNodeKind("MinkaCalloutTitle")

func (n *calloutTitle) Kind() ast.NodeKind { return kindCalloutTitle }

func (n *calloutTitle) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var alertMarker = regexp.MustCompile(`^\[!([A-Za-z]+)\][ \t]*`)

var calloutKinds = map[string]string{
	strings.ToLower(AlertNote):    "note",
	strings.ToLower(AlertWarning): "warning",
}

// markCallouts turns every quote that opens with "[!NOTE]" or "[!WARNING]"
// into a callout. Minka keeps no callout inside another, so a quote within a
// callout stays a quote.
func markCallouts(document ast.Node, source []byte) {
	var quotes []*ast.Blockquote
	_ = ast.Walk(document, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if quote, ok := n.(*ast.Blockquote); ok && entering {
			quotes = append(quotes, quote)
		}
		return ast.WalkContinue, nil
	})
	for _, quote := range quotes {
		if !standsInACallout(quote) {
			markCallout(quote, source)
		}
	}
}

func standsInACallout(node ast.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if _, ok := parent.(*callout); ok {
			return true
		}
	}
	return false
}

func markCallout(quote *ast.Blockquote, source []byte) {
	paragraph, ok := quote.FirstChild().(*ast.Paragraph)
	if !ok || paragraph.Lines().Len() == 0 {
		return
	}
	line := paragraph.Lines().At(0)
	match := alertMarker.FindSubmatchIndex(line.Value(source))
	if match == nil {
		return
	}
	kind, known := calloutKinds[strings.ToLower(string(line.Value(source)[match[2]:match[3]]))]
	if !known {
		return
	}

	// The marker is literal text at the start of the line; what follows it on
	// that line is the title.
	markerEnd := line.Start + match[1]
	var marker []*ast.Text
	for child := paragraph.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			return
		}
		marker = append(marker, textNode)
		if textNode.Segment.Stop >= markerEnd || textNode.SoftLineBreak() || textNode.HardLineBreak() {
			break
		}
	}
	last := marker[len(marker)-1]
	for _, textNode := range marker[:len(marker)-1] {
		paragraph.RemoveChild(paragraph, textNode)
	}
	lineEnded := false
	if last.Segment.Stop > markerEnd {
		last.Segment = last.Segment.WithStart(markerEnd)
	} else {
		lineEnded = last.SoftLineBreak() || last.HardLineBreak()
		paragraph.RemoveChild(paragraph, last)
	}

	title := &calloutTitle{}
	for child := paragraph.FirstChild(); child != nil && !lineEnded; {
		next := child.NextSibling()
		if textNode, ok := child.(*ast.Text); ok && (textNode.SoftLineBreak() || textNode.HardLineBreak()) {
			textNode.SetSoftLineBreak(false)
			textNode.SetHardLineBreak(false)
			lineEnded = true
		}
		title.AppendChild(title, child)
		child = next
	}

	box := &callout{kind: kind}
	quote.Parent().ReplaceChild(quote.Parent(), quote, box)
	if len(bytes.TrimSpace(plainText(title, source))) > 0 {
		box.AppendChild(box, title)
	}
	for child := quote.FirstChild(); child != nil; {
		next := child.NextSibling()
		if child != paragraph || paragraph.HasChildren() {
			box.AppendChild(box, child)
		}
		child = next
	}
}

// shortIdentifier is a card's "#MNK-1" at the start of a tile, which Minka
// turns into a reference as it does an address of its own.
var shortIdentifier = regexp.MustCompile(`^#\p{L}[\p{L}\p{N}]{1,9}-[\p{L}\p{N}]{1,12}`)

// isOfTiles reports whether every item of a list is one line, as a tile is.
func isOfTiles(list *ast.List) bool {
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.ChildCount() > 1 {
			return false
		}
	}
	return true
}

// tiles writes a list under the tiles marker as a group of link tiles, in
// the shape the editor stores: each item a tile whose term is the address it
// starts with and whose description is the rest of its line. An item without
// an address cannot be a tile and stands between two groups as a paragraph,
// where Minka would put it as well.
func (c *converter) tiles(list *ast.List) {
	open := false
	closeGroup := func() {
		if open {
			c.write("</dl>\n")
			open = false
		}
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		line := item.FirstChild()
		if line == nil {
			c.warn(WarningEmptyTile)
			continue
		}
		term, rest := c.tileTerm(line)
		if term == "" {
			closeGroup()
			c.warn(WarningTileWithoutAddress)
			c.write("<p>" + c.inlines(line.FirstChild()) + "</p>\n")
			continue
		}
		if !open {
			c.write(`<dl data-tiles="">`)
			open = true
		}
		c.write("<div><dt>" + term + "</dt>")
		if description := strings.TrimSpace(c.inlines(rest)); description != "" {
			c.write("<dd>" + description + "</dd>")
		}
		c.write("</div>")
	}
	closeGroup()
}

// tileTerm splits a tile's line into the address or short identifier it
// starts with, written, and the first node of the rest.
func (c *converter) tileTerm(line ast.Node) (string, ast.Node) {
	switch first := line.FirstChild().(type) {
	case *ast.AutoLink:
		// Minka makes a reference only of an address of its own, which is a
		// web address; it keeps any other as two paragraphs.
		if !isWebAddress(first.URL(c.source)) {
			return "", nil
		}
		term := c.render(func() { c.autoLink(first) })
		return term, withoutLeadingSpace(first.NextSibling(), c.source)
	case *ast.Text:
		value := first.Segment.Value(c.source)
		name := shortIdentifier.Find(value)
		if name == nil {
			return "", nil
		}
		if after, _ := utf8.DecodeRune(value[len(name):]); len(value) > len(name) && (after == '-' || unicode.IsLetter(after) || unicode.IsDigit(after)) {
			return "", nil
		}
		if len(value) > len(name) {
			first.Segment = first.Segment.WithStart(first.Segment.Start + len(name))
			return string(name), withoutLeadingSpace(first, c.source)
		}
		return string(name), withoutLeadingSpace(first.NextSibling(), c.source)
	}
	return "", nil
}

func isWebAddress(url []byte) bool {
	lower := bytes.ToLower(url)
	return bytes.HasPrefix(lower, []byte("https://")) || bytes.HasPrefix(lower, []byte("http://"))
}

// withoutLeadingSpace drops the space between a tile's term and its
// description.
func withoutLeadingSpace(node ast.Node, source []byte) ast.Node {
	textNode, ok := node.(*ast.Text)
	if !ok {
		return node
	}
	value := textNode.Segment.Value(source)
	trimmed := bytes.TrimLeft(value, " \t")
	textNode.Segment = textNode.Segment.WithStart(textNode.Segment.Start + len(value) - len(trimmed))
	return node
}

// inlines writes a node and the siblings after it.
func (c *converter) inlines(first ast.Node) string {
	return c.render(func() {
		for node := first; node != nil; node = node.NextSibling() {
			_ = ast.Walk(node, c.visit)
		}
	})
}

// render gives what a write produces instead of writing it.
func (c *converter) render(write func()) string {
	var out bytes.Buffer
	saved := c.w
	c.w = bufio.NewWriter(&out)
	write()
	_ = c.w.Flush()
	c.w = saved
	return out.String()
}
