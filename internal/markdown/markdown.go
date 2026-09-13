// Package markdown converts Markdown into the HTML that Minka keeps in a card
// description.
//
// The server reduces every description to a short list of elements (see
// AllowedElements) and silently unwraps everything else. The converter
// therefore only writes elements from that list and says in a warning what it
// had to change, instead of letting the server drop formatting unseen.
package markdown

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// AllowedElements are the elements the server keeps in a description, each
// with the attributes it may carry. Anything the converter writes has to be
// in here.
var AllowedElements = map[string][]string{
	"a":          {"href"},
	"blockquote": nil,
	"br":         nil,
	"del":        nil,
	"div":        nil,
	"em":         nil,
	"h1":         nil,
	"li":         nil,
	"ol":         nil,
	"p":          nil,
	"pre":        nil,
	"strong":     nil,
	"ul":         nil,
}

const (
	WarningHeadings    = "headings below level 1 are sent as bold paragraphs, Minka keeps one heading level"
	WarningImages      = "images are not supported and are sent as links"
	WarningTables      = "tables are not supported and are sent as preformatted text"
	WarningRules       = "horizontal rules are not supported and were left out"
	WarningRawHTML     = "HTML inside Markdown is sent as visible text; use --description-format html to send HTML"
	WarningListStart   = "ordered lists always start at 1 in Minka"
	WarningUnsafeLinks = "links Minka does not keep (only http, https, mailto, tel and relative URLs) were sent as plain text"
)

type Result struct {
	HTML     string
	Warnings []string // each at most once, in the order they came up
}

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough, extension.Table, extension.Linkify),
).Parser()

// ToHTML converts Markdown into description HTML. Blank input gives an empty
// string, which removes a description.
func ToHTML(source string) Result {
	if strings.TrimSpace(source) == "" {
		return Result{}
	}

	src := []byte(source)
	document := markdownParser.Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))

	var out bytes.Buffer
	c := &converter{source: src, w: bufio.NewWriter(&out), writer: ghtml.DefaultWriter}
	_ = ast.Walk(document, c.visit)
	_ = c.w.Flush()

	return Result{HTML: strings.TrimSpace(out.String()), Warnings: c.warnings}
}

type converter struct {
	source    []byte
	w         *bufio.Writer
	writer    ghtml.Writer
	warnings  []string
	linkDepth int // greater than 0 inside a link, where no further link may start
}

func (c *converter) visit(node ast.Node, entering bool) (ast.WalkStatus, error) {
	switch n := node.(type) {
	case *ast.Document, *ast.TextBlock:
		// Only their children produce output.
	case *ast.Heading:
		if n.Level == 1 {
			c.tag(entering, "h1")
		} else if entering {
			c.warn(WarningHeadings)
			c.write("<p><strong>")
		} else {
			c.write("</strong></p>\n")
		}
	case *ast.Paragraph:
		c.tag(entering, "p")
	case *ast.Blockquote:
		c.containerTag(entering, "blockquote")
	case *ast.List:
		name := "ul"
		if n.IsOrdered() {
			name = "ol"
			if entering && n.Start != 1 {
				c.warn(WarningListStart)
			}
		}
		c.containerTag(entering, name)
	case *ast.ListItem:
		c.tag(entering, "li")
	case *ast.FencedCodeBlock:
		if entering {
			c.preformatted(n.Lines())
		}
		return ast.WalkSkipChildren, nil
	case *ast.CodeBlock:
		if entering {
			c.preformatted(n.Lines())
		}
		return ast.WalkSkipChildren, nil
	case *ast.HTMLBlock:
		if entering {
			c.warn(WarningRawHTML)
			lines := n.Lines()
			if n.HasClosure() {
				lines.Append(n.ClosureLine)
			}
			c.preformatted(lines)
		}
		return ast.WalkSkipChildren, nil
	case *ast.ThematicBreak:
		if entering {
			c.warn(WarningRules)
		}
	case *ast.Text:
		if entering {
			c.text(n)
		}
	case *ast.String:
		if entering {
			c.writer.RawWrite(c.w, n.Value)
		}
	case *ast.CodeSpan:
		// Minka has no inline code. The backticks stay, so the reader still
		// sees what was code.
		if entering {
			c.write("`")
			c.writer.RawWrite(c.w, bytes.ReplaceAll(plainText(n, c.source), []byte("\n"), []byte(" ")))
			c.write("`")
		}
		return ast.WalkSkipChildren, nil
	case *ast.Emphasis:
		name := "em"
		if n.Level == 2 {
			name = "strong"
		}
		c.inlineTag(entering, name)
	case *east.Strikethrough:
		c.inlineTag(entering, "del")
	case *ast.Link:
		if !isSafeURL(n.Destination) {
			if entering {
				c.warn(WarningUnsafeLinks)
			}
			return ast.WalkContinue, nil
		}
		if entering {
			c.openLink(n.Destination)
			c.linkDepth++
		} else {
			c.write("</a>")
			c.linkDepth--
		}
	case *ast.AutoLink:
		if entering {
			c.autoLink(n)
		}
		return ast.WalkSkipChildren, nil
	case *ast.Image:
		if entering {
			c.image(n)
		}
		return ast.WalkSkipChildren, nil
	case *ast.RawHTML:
		if entering {
			c.warn(WarningRawHTML)
			for i := 0; i < n.Segments.Len(); i++ {
				segment := n.Segments.At(i)
				c.writer.RawWrite(c.w, segment.Value(c.source))
			}
		}
		return ast.WalkSkipChildren, nil
	case *east.Table:
		if entering {
			c.warn(WarningTables)
			c.table(n)
		}
		return ast.WalkSkipChildren, nil
	}

	return ast.WalkContinue, nil
}

func (c *converter) write(s string) {
	_, _ = c.w.WriteString(s)
}

func (c *converter) warn(warning string) {
	for _, existing := range c.warnings {
		if existing == warning {
			return
		}
	}
	c.warnings = append(c.warnings, warning)
}

func (c *converter) tag(entering bool, name string) {
	if entering {
		c.write("<" + name + ">")
	} else {
		c.write("</" + name + ">\n")
	}
}

func (c *converter) containerTag(entering bool, name string) {
	if entering {
		c.write("<" + name + ">\n")
	} else {
		c.write("</" + name + ">\n")
	}
}

func (c *converter) inlineTag(entering bool, name string) {
	if entering {
		c.write("<" + name + ">")
	} else {
		c.write("</" + name + ">")
	}
}

func (c *converter) text(n *ast.Text) {
	value := n.Value(c.source)
	if n.IsRaw() {
		c.writer.RawWrite(c.w, value)
	} else {
		c.writer.Write(c.w, value)
	}
	if n.HardLineBreak() {
		c.write("<br>")
	} else if n.SoftLineBreak() {
		c.write("\n")
	}
}

func (c *converter) preformatted(lines *text.Segments) {
	var content bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		content.Write(segment.Value(c.source))
	}
	c.write("<pre>")
	c.writer.RawWrite(c.w, bytes.TrimRight(content.Bytes(), "\n"))
	c.write("</pre>\n")
}

func (c *converter) openLink(destination []byte) {
	c.write(`<a href="`)
	_, _ = c.w.Write(util.EscapeHTML(util.URLEscape(destination, true)))
	c.write(`">`)
}

func (c *converter) autoLink(n *ast.AutoLink) {
	destination := n.URL(c.source)
	if n.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(destination), []byte("mailto:")) {
		destination = append([]byte("mailto:"), destination...)
	}
	label := n.Label(c.source)
	if c.linkDepth > 0 || !isSafeURL(destination) {
		if c.linkDepth == 0 {
			c.warn(WarningUnsafeLinks)
		}
		c.writer.RawWrite(c.w, label)
		return
	}
	c.openLink(destination)
	c.writer.RawWrite(c.w, label)
	c.write("</a>")
}

func (c *converter) image(n *ast.Image) {
	c.warn(WarningImages)
	label := plainText(n, c.source)
	if len(bytes.TrimSpace(label)) == 0 {
		label = n.Destination
	}
	// A link cannot hold another link, so an image inside one keeps only its
	// label.
	if c.linkDepth > 0 {
		c.writer.RawWrite(c.w, label)
		return
	}
	if !isSafeURL(n.Destination) {
		c.warn(WarningUnsafeLinks)
		c.writer.RawWrite(c.w, label)
		return
	}
	c.openLink(n.Destination)
	c.writer.RawWrite(c.w, label)
	c.write("</a>")
}

// table writes a table as aligned plain text in a pre element, the closest
// form the server keeps.
func (c *converter) table(table *east.Table) {
	var rows [][]string
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, strings.TrimSpace(string(plainText(cell, c.source))))
		}
		rows = append(rows, cells)
	}

	var widths []int
	for _, cells := range rows {
		for i, cell := range cells {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	var lines []string
	for r, cells := range rows {
		padded := make([]string, len(cells))
		for i, cell := range cells {
			padded[i] = cell + strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell))
		}
		lines = append(lines, strings.TrimRight(strings.Join(padded, " | "), " "))
		if r == 0 {
			separators := make([]string, len(widths))
			for i, width := range widths {
				separators[i] = strings.Repeat("-", width)
			}
			lines = append(lines, strings.Join(separators, "-|-"))
		}
	}

	c.write(fmt.Sprintf("<pre>%s</pre>\n", html.EscapeString(strings.Join(lines, "\n"))))
}

var urlIgnoredCharacters = regexp.MustCompile(`[\x00-\x20\x7f]`)

var urlSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tel": true}

// isSafeURL applies the rule the server applies to an href, to the URL as the
// converter writes it: only http, https, mailto, tel and relative URLs, no
// empty and no protocol-relative ones. The server removes any other href, and
// the link would disappear without a warning.
func isSafeURL(destination []byte) bool {
	candidate := strings.ToLower(urlIgnoredCharacters.ReplaceAllString(string(util.URLEscape(destination, true)), ""))
	if candidate == "" || strings.HasPrefix(candidate, "//") {
		return false
	}
	if strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "#") || strings.HasPrefix(candidate, "?") {
		return true
	}
	colon := strings.Index(candidate, ":")
	slash := strings.Index(candidate, "/")
	if colon == -1 || (slash != -1 && slash < colon) {
		return true
	}
	return urlSchemes[candidate[:colon]]
}

// plainText is the text of a node and its descendants with the markup left out.
// It is not escaped.
func plainText(node ast.Node, source []byte) []byte {
	var out bytes.Buffer
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			out.Write(t.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				out.WriteByte(' ')
			}
		case *ast.String:
			out.Write(t.Value)
		case *ast.AutoLink:
			out.Write(t.Label(source))
		case *ast.RawHTML:
			for i := 0; i < t.Segments.Len(); i++ {
				segment := t.Segments.At(i)
				out.Write(segment.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})

	return out.Bytes()
}
