// Package markdown converts Markdown into the HTML that Minka keeps in rich
// text, such as a card description or a comment.
//
// The server reduces every value to a list of elements and attributes (see
// AllowedElements and AttributePatterns) and silently unwraps everything else.
// The converter therefore only writes what is on that list and says in a
// warning what it had to change, instead of letting the server drop formatting
// unseen.
package markdown

import (
	"bufio"
	"bytes"
	"fmt"
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

// AllowedElements are the elements the server keeps in rich text, each with
// the attributes it may carry.
var AllowedElements = map[string][]string{
	"a":          {"href", "title"},
	"aside":      {"data-callout", "data-variables"},
	"blockquote": nil,
	"br":         nil,
	"code":       nil,
	"dd":         nil,
	"del":        nil,
	"div":        nil,
	"dl":         {"data-tiles"},
	"dt":         nil,
	"em":         nil,
	"figure":     {"class"},
	"h1":         nil,
	"h2":         nil,
	"h3":         nil,
	"h4":         nil,
	"h5":         nil,
	"h6":         nil,
	"header":     nil,
	"hr":         nil,
	"i":          nil,
	"li":         {"value"},
	"mark":       {"style"},
	"ol":         {"start"},
	"p":          nil,
	"pre":        {"data-language", "data-highlight-language"},
	"s":          nil,
	"strong":     nil,
	"sub":        nil,
	"sup":        nil,
	"table":      nil,
	"tbody":      nil,
	"td":         nil,
	"th":         {"class"},
	"tr":         nil,
	"u":          nil,
	"ul":         nil,
}

// AttributePatterns are the values the server accepts for the attributes that
// are checked, keyed "element.attribute". Any other value is removed.
var AttributePatterns = map[string]*regexp.Regexp{
	"aside.data-callout":          regexp.MustCompile(`^(?:note|warning)$`),
	"aside.data-variables":        regexp.MustCompile(`^$`),
	"dl.data-tiles":               regexp.MustCompile(`^$`),
	"figure.class":                regexp.MustCompile(`^lexxy-content__table-wrapper$`),
	"li.value":                    regexp.MustCompile(`^-?\d{1,9}$`),
	"ol.start":                    regexp.MustCompile(`^-?\d{1,9}$`),
	"pre.data-language":           regexp.MustCompile(`^[a-z0-9+#-]{1,32}$`),
	"pre.data-highlight-language": regexp.MustCompile(`^[a-z0-9+#-]{1,32}$`),
	"th.class":                    regexp.MustCompile(`^lexxy-content__table-cell--header$`),
}

// The class names the editor writes for a table, so that it can edit a table
// written here.
const (
	tableWrapperClass = "lexxy-content__table-wrapper"
	headerCellClass   = "lexxy-content__table-cell--header"
	maxLinkTitle      = 200
)

const (
	WarningImages      = "images are not supported and are sent as links"
	WarningRawHTML     = "HTML inside Markdown is sent as visible text; to send HTML, set the format flag of the text to html (--format, --description-format, --body-format or --notes-format)"
	WarningUnsafeLinks = "links Minka does not keep (only http, https, mailto, tel and relative URLs) were sent as plain text"
)

type Result struct {
	HTML     string
	Warnings []string // each at most once, in the order they came up

	// MostlyHTML says that more than half of the lines that hold anything
	// belong to HTML blocks: a text written in HTML and sent as Markdown by
	// mistake, which would arrive as its own source code.
	MostlyHTML bool
}

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough, extension.Table, extension.Linkify),
).Parser()

// Option changes how ToHTML converts.
type Option func(*converter)

// KeepLineBreaks turns every line break into a break in the text, as a comment
// box does, instead of joining the lines of a paragraph.
func KeepLineBreaks() Option {
	return func(c *converter) {
		c.keepLineBreaks = true
	}
}

// ToHTML converts Markdown into rich text HTML. Blank input gives an empty
// string, which removes a description.
func ToHTML(source string, options ...Option) Result {
	if strings.TrimSpace(source) == "" {
		return Result{}
	}

	src := []byte(source)
	document := markdownParser.Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	markCallouts(document, src)

	var out bytes.Buffer
	c := &converter{source: src, w: bufio.NewWriter(&out), writer: ghtml.DefaultWriter}
	for _, option := range options {
		option(c)
	}
	_ = ast.Walk(document, c.visit)
	_ = c.w.Flush()

	return Result{
		HTML:       strings.TrimSpace(out.String()),
		Warnings:   c.warnings,
		MostlyHTML: 2*c.htmlLines > nonBlankLines(source),
	}
}

func nonBlankLines(source string) int {
	count := 0
	for _, line := range strings.Split(source, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

type converter struct {
	source    []byte
	w         *bufio.Writer
	writer    ghtml.Writer
	warnings  []string
	linkDepth int // greater than 0 inside a link, where no further link may start
	htmlLines int // the lines that hold anything inside HTML blocks shown as text

	keepLineBreaks bool
}

func (c *converter) visit(node ast.Node, entering bool) (ast.WalkStatus, error) {
	switch n := node.(type) {
	case *ast.Document, *ast.TextBlock:
		// Only their children produce output.
	case *ast.Heading:
		c.tag(entering, fmt.Sprintf("h%d", n.Level))
	case *ast.Paragraph:
		c.tag(entering, "p")
	case *ast.Blockquote:
		c.containerTag(entering, "blockquote")
	case *callout:
		if entering {
			c.write(`<aside data-callout="` + n.kind + `">` + "\n")
		} else {
			c.write("</aside>\n")
		}
	case *calloutTitle:
		c.tag(entering, "header")
	case *ast.List:
		if c.markerBefore(n) == markerTiles {
			switch {
			case isOfTiles(n) && entering:
				c.tiles(n)
				return ast.WalkSkipChildren, nil
			case isOfTiles(n):
				return ast.WalkContinue, nil
			case entering:
				c.warn(WarningTileOfOneLine)
			}
		}
		switch {
		case !n.IsOrdered():
			c.containerTag(entering, "ul")
		case !entering:
			c.write("</ol>\n")
		case n.Start != 1 && AttributePatterns["ol.start"].MatchString(fmt.Sprint(n.Start)):
			c.write(fmt.Sprintf("<ol start=\"%d\">\n", n.Start))
		default:
			c.write("<ol>\n")
		}
	case *ast.ListItem:
		c.tag(entering, "li")
	case *ast.FencedCodeBlock:
		if entering {
			language := strings.ToLower(string(n.Language(c.source)))
			if !AttributePatterns["pre.data-language"].MatchString(language) {
				language = PlainLanguage
			}
			c.codeBlock(n.Lines(), language)
		}
		return ast.WalkSkipChildren, nil
	case *ast.CodeBlock:
		if entering {
			c.codeBlock(n.Lines(), PlainLanguage)
		}
		return ast.WalkSkipChildren, nil
	case *ast.HTMLBlock:
		if entering && !isMarkerOfTheNextBlock(n, c.source) {
			c.warn(WarningRawHTML)
			lines := n.Lines()
			if n.HasClosure() {
				lines.Append(n.ClosureLine)
			}
			c.countHTMLLines(lines)
			c.codeBlock(lines, PlainLanguage)
		}
		return ast.WalkSkipChildren, nil
	case *ast.ThematicBreak:
		if entering {
			c.write("<hr>\n")
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
		if entering {
			c.write("<code>")
			c.writer.RawWrite(c.w, bytes.ReplaceAll(plainText(n, c.source), []byte("\n"), []byte(" ")))
			c.write("</code>")
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
	case *east.Table:
		variables := c.markerBefore(n) == markerVariables
		switch {
		case entering && variables:
			c.write(`<aside data-variables=""><figure class="` + tableWrapperClass + `"><table><tbody>` + "\n")
		case entering:
			c.write(`<figure class="` + tableWrapperClass + `"><table><tbody>` + "\n")
		case variables:
			c.write("</tbody></table></figure></aside>\n")
		default:
			c.write("</tbody></table></figure>\n")
		}
	case *east.TableHeader, *east.TableRow:
		if entering {
			c.write("<tr>")
		} else {
			c.write("</tr>\n")
		}
	case *east.TableCell:
		_, inHeader := n.Parent().(*east.TableHeader)
		// The editor writes the name of a variable as the header of its row.
		inHeader = inHeader || (n.PreviousSibling() == nil && c.markerBefore(n.Parent().Parent()) == markerVariables)
		switch {
		case inHeader && entering:
			c.write(`<th class="` + headerCellClass + `">`)
		case inHeader:
			c.write("</th>")
		default:
			c.inlineTag(entering, "td")
		}
	case *ast.Link:
		if !isSafeURL(n.Destination) {
			if entering {
				c.warn(WarningUnsafeLinks)
			}
			return ast.WalkContinue, nil
		}
		if entering {
			c.openLink(n.Destination, n.Title)
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
	}

	return ast.WalkContinue, nil
}

func (c *converter) countHTMLLines(lines *text.Segments) {
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		if len(bytes.TrimSpace(segment.Value(c.source))) > 0 {
			c.htmlLines++
		}
	}
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
	switch {
	case n.HardLineBreak(), n.SoftLineBreak() && c.keepLineBreaks:
		c.write("<br>")
	case n.SoftLineBreak():
		c.write("\n")
	}
}

// codeBlock writes lines as the editor stores a code block: the text of a
// pre that names its language, a break for each line break and no code
// element inside. The page paints the colours of the language only into a
// block of that shape. The text is escaped as the server escapes it, so that
// what is stored is what was sent.
func (c *converter) codeBlock(lines *text.Segments, language string) {
	var content bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		content.Write(segment.Value(c.source))
	}
	code := strings.ReplaceAll(strings.TrimRight(content.String(), "\r\n"), "\r\n", "\n")
	c.write(`<pre data-language="` + language + `">`)
	c.write(strings.ReplaceAll(codeEscaper.Replace(code), "\n", "<br>"))
	c.write("</pre>\n")
}

// PlainLanguage is the language the editor names for a code block without
// one.
const PlainLanguage = "plain"

var codeEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// openLink writes the start of a link. A title longer than the server keeps is
// left out.
func (c *converter) openLink(destination, title []byte) {
	c.write(`<a href="`)
	_, _ = c.w.Write(util.EscapeHTML(util.URLEscape(destination, true)))
	c.write(`"`)
	if len(title) > 0 && utf8.RuneCount(title) <= maxLinkTitle {
		c.write(` title="`)
		c.writer.RawWrite(c.w, title)
		c.write(`"`)
	}
	c.write(`>`)
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
	c.openLink(destination, nil)
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
	c.openLink(n.Destination, nil)
	c.writer.RawWrite(c.w, label)
	c.write("</a>")
}

var urlIgnoredCharacters = regexp.MustCompile(`[\x00-\x20\x7f]`)

var urlSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tel": true}

// isSafeURL applies the rule the server applies to an href, to the URL as the
// converter writes it: only http, https, mailto, tel and relative URLs, no
// empty and no protocol-relative ones. The server removes any other href, and
// the link would disappear without a warning.
func isSafeURL(destination []byte) bool {
	// A browser reads a backslash at the start of a URL as a slash, so "\\host"
	// leaves Minka just as "//host" does.
	if raw := strings.ReplaceAll(urlIgnoredCharacters.ReplaceAllString(string(destination), ""), `\`, "/"); strings.HasPrefix(raw, "//") {
		return false
	}
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
