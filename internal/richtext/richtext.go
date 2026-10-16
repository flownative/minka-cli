// Package richtext turns the rich text HTML of a card description or a comment
// back into Markdown for reading in a terminal.
//
// The result is meant for people and agents who read a card and may edit it:
// fed back through the markdown package it gives the same elements again.
// Text is escaped where Markdown would read it as markup, and a line break
// becomes a hard break. It only has to cover the elements the server keeps in
// a description. The exceptions are what a text carries rather than writes: a
// file, a mention or an embed comes back as a link or as words, and a
// reference to another record as its address, which the server turns into the
// reference again, or as words where the reader cannot open its target.
package richtext

import (
	"fmt"
	"regexp"
	"slices"
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

// carriedElements are what a stored text holds beside the elements Markdown
// writes: the file or person hanging in it, the card an address became, and
// the reference to another record. Each one is its attributes and nothing
// else, and Markdown can hold none of them.
var carriedElements = []string{"action-text-attachment", "minka-embed", "minka-ref"}

// servedMarkup is the server's rule for a value that holds markup: a tag of a
// served element written out to its closing angle bracket. "a<i und b<s" is
// plain text, although i and s are served elements.
var servedMarkup = func() *regexp.Regexp {
	names := make([]string, 0, len(markdown.AllowedElements)+len(carriedElements))
	for name := range markdown.AllowedElements {
		names = append(names, name)
	}
	names = append(names, carriedElements...)
	alternatives := strings.Join(names, "|")
	return regexp.MustCompile(`(?i)<(?:` + alternatives + `)(?:\s[^<>]*)?/?>|</(?:` + alternatives + `)\s*>`)
}()

// CarriesMarkup reports whether a value holds a tag of an element the server
// serves. The server keeps a value without one as plain text, as it does with
// comments written before the editor, and such a value is shown as it is.
func CarriesMarkup(value string) bool {
	return servedMarkup.MatchString(value)
}

// HasAttachments reports whether rich text carries something Markdown cannot
// hold: a file, a person named in it, an address the server drew as a card, a
// reference to another record. The stored text holds each of them as an
// element that is its attributes and nothing else; served, an attachment has
// become a figure with the class "attachment", an img or a link to download.
// A text sent back without those elements drops what they stood for.
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
			case n.Data == "img", slices.Contains(carriedElements, n.Data):
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

// Loss is what of a text a replacement would drop.
type Loss struct {
	// Attachments: files, mentions or embeds, of which the replacement
	// carries none.
	Attachments bool
	// References: a reference to another record that the replacement holds
	// neither as the element nor in a form the server turns back into it.
	References bool
}

// Any reports whether anything would be dropped.
func (l Loss) Any() bool {
	return l.Attachments || l.References
}

// String names what would be dropped, for a refusal or a notice.
func (l Loss) String() string {
	switch {
	case l.Attachments && l.References:
		return "attachments or mentions and references to other records"
	case l.References:
		return "references to other records"
	default:
		return "attachments or mentions"
	}
}

// Dropped holds a replacement against the text it replaces. A file, a mention
// or an embed only survives in a text that carries such elements itself. A
// reference also survives as a link that shows its own address, or as
// "#MNK-1" for a card with a short identifier, since the server turns both
// back into it on saving. A reference whose target the reader may not open
// has no address to be written by, so only the element keeps it.
func Dropped(current, replacement string) Loss {
	return dropped(current, contentsOf(replacement))
}

// DroppedByText is Dropped for a replacement sent as plain text, which the
// server keeps as the characters it consists of: nothing in it is markup,
// and every address and name stands in its words. A text that carries a tag
// of a served element the server takes as HTML all the same, and so does
// this.
func DroppedByText(current, replacement string) Loss {
	if CarriesMarkup(replacement) {
		return Dropped(current, replacement)
	}
	has := contents{elements: map[string]bool{}, autolinks: map[string]bool{}}
	has.addWords(replacement)
	return dropped(current, has)
}

func dropped(current string, has contents) Loss {
	had := contentsOf(current)
	loss := Loss{Attachments: had.attachments && !has.attachments}
	for _, ref := range had.references {
		if !has.keeps(ref) {
			loss.References = true
		}
	}
	return loss
}

// addressInWords is where the server finds an address in the words of a
// text; the punctuation of the sentence after it is not part of it.
var addressInWords = regexp.MustCompile(`https?://[^\s<>"']+`)

type servedReference struct {
	kind, id, shortID, href string
}

type contents struct {
	attachments bool
	references  []servedReference
	elements    map[string]bool
	autolinks   map[string]bool
	words       []string
}

// addWords takes the words of a text in: the addresses the server finds in
// them, and what is left once they are gone, where a name glued to an address
// would have been swallowed by it.
func (c *contents) addWords(text string) {
	for _, address := range addressInWords.FindAllString(text, -1) {
		c.autolinks[withoutTrailingSlash(strings.TrimRight(address, ".,;:!?'\")]}"))] = true
	}
	c.words = append(c.words, addressInWords.ReplaceAllString(text, " "))
}

// withoutTrailingSlash compares addresses the way the server recognizes them:
// one slash at the end makes no difference.
func withoutTrailingSlash(address string) string {
	return strings.TrimSuffix(strings.TrimSpace(address), "/")
}

func (c contents) keeps(ref servedReference) bool {
	switch {
	case c.elements[ref.kind+"/"+ref.id]:
		return true
	case ref.href != "" && c.autolinks[withoutTrailingSlash(ref.href)]:
		return true
	case ref.shortID != "":
		name := regexp.MustCompile(`(?:^|[^\p{L}\p{N}#])#` + regexp.QuoteMeta(ref.shortID) + `(?:$|[^\p{L}\p{N}-])`)
		return slices.ContainsFunc(c.words, name.MatchString)
	}
	return false
}

// contentsOf gathers what of a text Markdown cannot write: whether it carries
// a file, a mention or an embed, its references, and the links, addresses and
// words the server would read a reference from. Code, the words of any link
// and what a browser never shows as text are no such place.
func contentsOf(served string) contents {
	c := contents{elements: map[string]bool{}, autolinks: map[string]bool{}}
	if strings.TrimSpace(served) == "" {
		return c
	}
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(served), context)
	if err != nil {
		c.attachments = true
		return c
	}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			c.addWords(n.Data)
			return
		}
		if n.Type != html.ElementNode {
			return
		}
		switch {
		case n.Data == "minka-ref":
			ref := servedReference{
				kind:    strings.TrimSpace(attribute(n, "kind")),
				id:      strings.TrimSpace(attribute(n, "id")),
				shortID: strings.TrimSpace(attribute(n, "short-id")),
				href:    strings.TrimSpace(attribute(n, "href")),
			}
			c.references = append(c.references, ref)
			c.elements[ref.kind+"/"+ref.id] = true
			return
		case n.Data == "img", n.Data == "action-text-attachment", n.Data == "minka-embed":
			c.attachments = true
			return
		case n.Data == "a" && hasAttribute(n, "download"):
			c.attachments = true
			return
		case n.Data == "figure" && strings.Contains(" "+attribute(n, "class")+" ", " attachment "):
			c.attachments = true
		case n.Data == "a":
			href := withoutTrailingSlash(attribute(n, "href"))
			if href != "" && withoutTrailingSlash(textContent(n)) == href {
				c.autolinks[href] = true
			}
			return
		case n.Data == "code", n.Data == "pre", n.Data == "script", n.Data == "style", n.Data == "textarea", n.Data == "template":
			return
		}
		for _, child := range children(n) {
			visit(child)
		}
	}
	for _, n := range nodes {
		visit(n)
	}
	return c
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
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "blockquote", "pre", "ul", "ol", "li", "table", "figure", "figcaption",
		"aside", "header", "dl", "dt", "dd":
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
		appendInline(&run, inline(n))
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
	case "aside":
		switch {
		case calloutAlerts[attribute(n, "data-callout")] != "":
			return calloutBlock(n)
		case hasAttribute(n, "data-variables"):
			return variablesBlock(n)
		}
		return blocks(children(n))
	case "dl":
		if hasAttribute(n, "data-tiles") {
			return tilesBlock(n)
		}
		return blocks(children(n))
	case "pre":
		content := strings.TrimRight(textContent(n), "\n")
		fence := codeFence(content)
		language := attribute(n, "data-language")
		if !markdown.AttributePatterns["pre.data-language"].MatchString(language) || language == markdown.PlainLanguage {
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
	case "minka-ref":
		return reference(n)
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

// unopenedTargets are the words for a reference whose target the reader may
// not open, in trash or never was: the server then sends only its kind and
// identifier, and all three cases read alike.
var unopenedTargets = map[string]string{
	"card":        "a card you cannot open",
	"post":        "a post you cannot open",
	"chat":        "a chat you cannot open",
	"chatmessage": "a chat message you cannot open",
	"todo":        "a to-do you cannot open",
	"todolist":    "a to-do list you cannot open",
	"document":    "a document you cannot open",
	"upload":      "a file you cannot open",
	"question":    "a question you cannot open",
	"event":       "an event you cannot open",
}

// reference shows a reference to another record by its address, as an
// autolink: sent back, the link that shows its own address is what the server
// turns into the reference again, and a card's short identifier can be read in
// it. Titles are left out, because words of their own would make the server
// keep a mere link. A target the reader may not open comes with nothing but
// its kind and identifier, and the view says so rather than leaving a gap in
// the sentence; it cannot come back from Markdown.
func reference(n *html.Node) string {
	href := strings.TrimSpace(attribute(n, "href"))
	switch {
	case href != "" && isAutolinkable(href):
		return "<" + href + ">"
	case href != "":
		return "[" + escapeText(href) + "](" + linkDestination(href) + ")"
	}
	// Minka always sends an address with a title; without one only the words
	// that were sent are left.
	var label []string
	if shortID := strings.TrimSpace(attribute(n, "short-id")); shortID != "" {
		label = append(label, "#"+shortID)
	}
	if title := strings.TrimSpace(whitespace.ReplaceAllString(attribute(n, "title"), " ")); title != "" {
		label = append(label, title)
	}
	if len(label) > 0 {
		return escapeText(strings.Join(label, " "))
	}
	words, known := unopenedTargets[attribute(n, "kind")]
	if !known {
		words = "a reference you cannot open"
	}
	return "(" + words + ")"
}

// isAutolinkable reports whether an address can stand between angle brackets
// and come back as a link to itself.
func isAutolinkable(href string) bool {
	return (strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://")) && !strings.ContainsAny(href, " <>\t\n")
}

// calloutAlerts are the GitHub alerts a callout's kind is written as.
var calloutAlerts = map[string]string{
	"note":    markdown.AlertNote,
	"warning": markdown.AlertWarning,
}

// calloutBlock renders a callout as a GitHub alert, its title beside the
// marker as Obsidian writes one. GitHub shows a quote with the title in its
// first line instead; without a title the alert is GitHub's own.
func calloutBlock(n *html.Node) string {
	head := "[!" + calloutAlerts[attribute(n, "data-callout")] + "]"
	content := children(n)
	for i, child := range content {
		if child.Type != html.ElementNode {
			continue
		}
		if child.Data == "header" {
			if title := oneLine(paragraphLines(inlineChildren(child))); title != "" {
				head += " " + title
			}
			content = append(slices.Clone(content[:i]), content[i+1:]...)
		}
		break
	}
	if body := blocks(content); body != "" {
		head += "\n\n" + body
	}
	return prefixLines(head, "> ", "> ")
}

// variablesBlock renders a block of variables as its table under the
// marker that makes it one again. The columns are the editor's: name,
// label, hint, default, then a value per profile.
func variablesBlock(n *html.Node) string {
	var found *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		for _, child := range children(node) {
			if found == nil && child.Type == html.ElementNode && child.Data == "table" {
				found = child
			}
			find(child)
		}
	}
	find(n)
	if found == nil {
		return blocks(children(n))
	}
	return markdown.MarkerVariables + "\n" + table(found)
}

// tilesBlock renders a group of link tiles as a list under the marker that
// makes it one again: each tile its reference's address and then its
// description.
func tilesBlock(n *html.Node) string {
	var lines []string
	tile := func(term, description *html.Node) {
		line := "- "
		if term != nil {
			line += oneLine(paragraphLines(inlineChildren(term)))
		}
		if description != nil {
			if words := oneLine(paragraphLines(inlineChildren(description))); words != "" {
				line += " " + words
			}
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	var term *html.Node
	for _, child := range children(n) {
		if child.Type != html.ElementNode {
			continue
		}
		switch child.Data {
		case "div":
			var dt, dd *html.Node
			for _, part := range children(child) {
				if part.Type == html.ElementNode && part.Data == "dt" && dt == nil {
					dt = part
				}
				if part.Type == html.ElementNode && part.Data == "dd" && dd == nil {
					dd = part
				}
			}
			tile(dt, dd)
		case "dt":
			if term != nil {
				tile(term, nil)
			}
			term = child
			continue
		case "dd":
			tile(term, child)
		}
		term = nil
	}
	if term != nil {
		tile(term, nil)
	}
	return markdown.MarkerTiles + "\n" + strings.Join(lines, "\n")
}

// oneLine joins the lines of a paragraph, for a place that holds one line.
func oneLine(paragraph string) string {
	return strings.ReplaceAll(paragraph, "\\\n", " ")
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
		appendInline(&out, inline(child))
	}
	return out.String()
}

// appendInline adds a piece of inline Markdown. An exclamation mark of the
// text before a link would turn the link into an image, so it gets escaped.
func appendInline(out *strings.Builder, piece string) {
	before := out.String()
	if strings.HasPrefix(piece, "[") && strings.HasSuffix(before, "!") {
		backslashes := len(before) - 1 - len(strings.TrimRight(before[:len(before)-1], `\`))
		if backslashes%2 == 0 {
			out.Reset()
			out.WriteString(before[:len(before)-1] + `\!`)
		}
	}
	out.WriteString(piece)
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
