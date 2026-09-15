// Package richtext turns the rich text HTML of a card description or a comment
// back into Markdown for reading in a terminal.
//
// The result is meant for people and agents who read a card and may edit it:
// fed back through the markdown package it gives the same elements again.
// Text is escaped where Markdown would read it as markup, and a line break
// becomes a hard break. It only has to cover the elements the server keeps in
// a description.
package richtext

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/flownative/minka-cli/internal/markdown"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	whitespace        = regexp.MustCompile(`\s+`)
	orderedListMarker = regexp.MustCompile(`^\d+[.)]`)
	entityReference   = regexp.MustCompile(`^&#?[A-Za-z0-9]+;`)
)

// servedMarkup is the server's rule for a value that holds markup: a tag of a
// served element written out to its closing angle bracket. "a<i und b<s" is
// plain text, although i and s are served elements.
var servedMarkup = func() *regexp.Regexp {
	names := make([]string, 0, len(markdown.AllowedElements))
	for name := range markdown.AllowedElements {
		names = append(names, name)
	}
	alternatives := strings.Join(names, "|")
	alternatives += "|action-text-attachment"
	return regexp.MustCompile(`(?i)<(?:` + alternatives + `)(?:\s[^<>]*)?/?>|</(?:` + alternatives + `)\s*>`)
}()

// CarriesMarkup reports whether a value holds a tag of an element the server
// serves. The server keeps a value without one as plain text, as it does with
// comments written before the editor, and such a value is shown as it is.
func CarriesMarkup(value string) bool {
	return servedMarkup.MatchString(value)
}

// HasAttachments reports whether served rich text shows a picture or a file
// attached on the web. The server serves an attachment as a figure with the
// class "attachment", an img or a link to download; a text sent back without
// the attachment element it stores drops the attachment.
func HasAttachments(served string) bool {
	if strings.TrimSpace(served) == "" {
		return false
	}
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(served), context)
	if err != nil {
		return true
	}
	var found func(*html.Node) bool
	found = func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			switch {
			case n.Data == "img", n.Data == "action-text-attachment":
				return true
			case n.Data == "a" && hasAttribute(n, "download"):
				return true
			case n.Data == "figure" && strings.Contains(" "+attribute(n, "class")+" ", " attachment "):
				return true
			}
		}
		for _, child := range children(n) {
			if found(child) {
				return true
			}
		}
		return false
	}
	for _, n := range nodes {
		if found(n) {
			return true
		}
	}
	return false
}

func hasAttribute(n *html.Node, name string) bool {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return true
		}
	}
	return false
}

// ToMarkdown converts rich text HTML into Markdown. HTML that cannot be
// parsed is returned unchanged.
func ToMarkdown(fragment string) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		return fragment
	}

	return blocks(nodes)
}

func isBlock(name string) bool {
	switch name {
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "blockquote", "pre", "ul", "ol", "li", "table", "figure", "figcaption":
		return true
	}
	return false
}

type part struct {
	text string
	list bool
}

// blocks renders a sequence of nodes as paragraphs separated by blank lines.
func blocks(nodes []*html.Node) string {
	return joinParts(blockParts(nodes), false)
}

// blockParts renders each block of a sequence of nodes. Inline content
// between block elements becomes a paragraph of its own.
func blockParts(nodes []*html.Node) []part {
	var parts []part
	var run strings.Builder
	flush := func() {
		if paragraph := paragraphLines(run.String()); paragraph != "" {
			parts = append(parts, part{text: paragraph})
		}
		run.Reset()
	}

	for _, n := range nodes {
		if n.Type == html.ElementNode && isBlock(n.Data) {
			flush()
			if text := block(n); text != "" {
				parts = append(parts, part{text: text, list: n.Data == "ul" || n.Data == "ol"})
			}
			continue
		}
		run.WriteString(inline(n))
	}
	flush()

	return parts
}

// joinParts separates blocks by a blank line. Inside a list item a nested list
// follows directly, because a blank line would make the list a loose one.
func joinParts(parts []part, inListItem bool) string {
	var out strings.Builder
	for i, p := range parts {
		if i > 0 {
			if inListItem && p.list {
				out.WriteString("\n")
			} else {
				out.WriteString("\n\n")
			}
		}
		out.WriteString(p.text)
	}
	return out.String()
}

func block(n *html.Node) string {
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return strings.Repeat("#", int(n.Data[1]-'0')) + " " + paragraphLines(inlineChildren(n))
	case "hr":
		return "---"
	case "table":
		return table(n)
	case "blockquote":
		return prefixLines(blocks(children(n)), "> ", "> ")
	case "pre":
		content := strings.TrimRight(textContent(n), "\n")
		fence := codeFence(content)
		language := attribute(n, "data-language")
		if !markdown.AttributePatterns["pre.data-language"].MatchString(language) {
			language = ""
		}
		return fence + language + "\n" + content + "\n" + fence
	case "ul", "ol":
		var items []string
		number := 1
		if start, err := strconv.Atoi(attribute(n, "start")); n.Data == "ol" && err == nil && start >= 0 {
			number = start
		}
		for _, item := range children(n) {
			if item.Type != html.ElementNode || item.Data != "li" {
				continue
			}
			marker := "- "
			if n.Data == "ol" {
				marker = fmt.Sprintf("%d. ", number)
				number++
			}
			items = append(items, prefixLines(joinParts(blockParts(children(item)), true), marker, strings.Repeat(" ", len(marker))))
		}
		return strings.Join(items, "\n")
	default:
		return blocks(children(n))
	}
}

// inline renders inline content. A line break is kept as "\n" and turned into
// a hard break once the paragraph is complete.
func inline(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeText(whitespace.ReplaceAllString(n.Data, " "))
	case html.ElementNode:
	default:
		return ""
	}

	if n.Data == "code" {
		return inlineCode(strings.ReplaceAll(textContent(n), "\n", " "))
	}

	content := inlineChildren(n)
	switch n.Data {
	case "br":
		return "\n"
	case "strong", "b":
		return wrapInline(content, "**")
	case "em", "i":
		return wrapInline(content, "*")
	case "del", "s":
		return wrapInline(content, "~~")
	case "action-text-attachment":
		return attachment(n)
	case "img":
		// A picture Minka serves for an attachment. Markdown can show it, but
		// the Markdown conversion sends an image back as a link.
		alt := escapeText(attribute(n, "alt"))
		if src := attribute(n, "src"); src != "" {
			return "![" + alt + "](" + linkDestination(src) + ")"
		}
		return alt
	case "a":
		href := attribute(n, "href")
		title := attribute(n, "title")
		label := strings.TrimSpace(content)
		destination := linkDestination(href)
		if title != "" {
			destination += ` "` + strings.ReplaceAll(title, `"`, `\"`) + `"`
		}
		switch {
		case href == "":
			return content
		case title == "" && (label == "" || label == escapeText(href)) && (strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://")):
			return "<" + href + ">"
		case label == "":
			return "[" + escapeText(href) + "](" + destination + ")"
		default:
			return "[" + label + "](" + destination + ")"
		}
	}

	return content
}

// attachment shows a file or a mention that stands in a text. Markdown has no
// form that keeps one, so the view is for reading only.
func attachment(n *html.Node) string {
	if attribute(n, "content-type") == "application/vnd.minka.person" {
		if content := strings.TrimSpace(attribute(n, "content")); content != "" {
			return escapeText(content)
		}
		return "@"
	}
	name := attribute(n, "filename")
	if name == "" {
		name = attribute(n, "caption")
	}
	if name == "" {
		name = "attachment"
	}
	url := attribute(n, "url")
	switch {
	case url == "":
		return escapeText("[" + name + "]")
	case strings.HasPrefix(attribute(n, "content-type"), "image/"):
		return "![" + escapeText(name) + "](" + linkDestination(url) + ")"
	default:
		return "[" + escapeText(name) + "](" + linkDestination(url) + ")"
	}
}

// table renders a table as a Markdown table. The first row is the header,
// which is how the editor writes a table as well.
func table(n *html.Node) string {
	var rows [][]string
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		for _, child := range children(node) {
			if child.Type != html.ElementNode {
				continue
			}
			switch child.Data {
			case "tr":
				var cells []string
				for _, cell := range children(child) {
					if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
						text := strings.ReplaceAll(paragraphLines(inlineChildren(cell)), "\\\n", " ")
						cells = append(cells, strings.ReplaceAll(text, "|", `\|`))
					}
				}
				rows = append(rows, cells)
			case "thead", "tbody", "tfoot":
				collect(child)
			}
		}
	}
	collect(n)
	if len(rows) == 0 {
		return ""
	}

	columns := 0
	for _, row := range rows {
		columns = max(columns, len(row))
	}
	line := func(cells []string) string {
		padded := make([]string, columns)
		copy(padded, cells)
		return "| " + strings.Join(padded, " | ") + " |"
	}
	separator := make([]string, columns)
	for i := range separator {
		separator[i] = "---"
	}

	lines := []string{line(rows[0]), line(separator)}
	for _, row := range rows[1:] {
		lines = append(lines, line(row))
	}
	return strings.Join(lines, "\n")
}

func inlineChildren(n *html.Node) string {
	var out strings.Builder
	for _, child := range children(n) {
		out.WriteString(inline(child))
	}
	return out.String()
}

// wrapInline puts markers around content. Spaces at its edges go outside the
// markers, where Markdown expects them.
func wrapInline(content, marker string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	start := strings.Index(content, trimmed)
	return content[:start] + marker + trimmed + marker + content[start+len(trimmed):]
}

// escapeText puts a backslash before the characters Markdown would read as
// markup. An underscore inside a word starts no emphasis and stays as it is.
func escapeText(s string) string {
	runes := []rune(s)
	var out strings.Builder
	for i, r := range runes {
		switch r {
		case '\\', '*', '`', '[', ']', '<', '~':
			out.WriteRune('\\')
		case '_':
			if i == 0 || i == len(runes)-1 || !isWordRune(runes[i-1]) || !isWordRune(runes[i+1]) {
				out.WriteRune('\\')
			}
		case '&':
			if entityReference.MatchString(string(runes[i:min(len(runes), i+34)])) {
				out.WriteRune('\\')
			}
		}
		out.WriteRune(r)
	}
	return out.String()
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// escapeLineStart escapes what Markdown reads as a block marker at the start
// of a line: a heading, a list item, a quote or a heading underline.
func escapeLineStart(line string) string {
	if line == "" {
		return line
	}
	switch line[0] {
	case '#', '-', '+', '>', '=':
		return `\` + line
	}
	if marker := orderedListMarker.FindString(line); marker != "" {
		return marker[:len(marker)-1] + `\` + line[len(marker)-1:]
	}
	return line
}

// paragraphLines trims every line of a paragraph, escapes line starts and
// joins the lines with hard breaks.
func paragraphLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = escapeLineStart(strings.TrimSpace(line))
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\\\n")
}

func linkDestination(href string) string {
	if strings.ContainsAny(href, " ()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">"
	}
	return href
}

// codeFence is a fence longer than any run of backticks in the content.
func codeFence(content string) string {
	return strings.Repeat("`", max(3, longestBacktickRun(content)+1))
}

// inlineCode wraps code in more backticks than it contains itself. Code that
// starts or ends with a backtick gets a space inside the delimiters, which
// Markdown removes again.
func inlineCode(code string) string {
	if code == "" {
		return ""
	}
	delimiter := strings.Repeat("`", longestBacktickRun(code)+1)
	if strings.HasPrefix(code, "`") || strings.HasSuffix(code, "`") {
		return delimiter + " " + code + " " + delimiter
	}
	return delimiter + code + delimiter
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

func children(n *html.Node) []*html.Node {
	var list []*html.Node
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		list = append(list, child)
	}
	return list
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type == html.ElementNode && n.Data == "br" {
		return "\n"
	}
	var out strings.Builder
	for _, child := range children(n) {
		out.WriteString(textContent(child))
	}
	return out.String()
}

func attribute(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func prefixLines(s, first, rest string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		prefix := rest
		if i == 0 {
			prefix = first
		}
		if line == "" && i > 0 {
			lines[i] = strings.TrimRight(prefix, " ")
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
