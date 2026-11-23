package export

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/flownative/minka-cli/internal/richtext"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The two formats an export writes.
const (
	FormatMarkdown = "markdown"
	FormatHTML     = "html"
)

// The kinds of files an export writes, as its manifest names them.
const (
	fileIndex    = "index"
	fileDocument = "document"
	fileAsset    = "asset"
	fileUpload   = "upload"
)

// The pages of Minka an address in a text may name an entry by.
var (
	documentPage = regexp.MustCompile(`^/projects/[^/]+/files/[^/]+/([^/]+)/?$`)
	uploadPage   = regexp.MustCompile(`^/projects/[^/]+/files/[^/]+/uploads/([^/]+)/?$`)
	folderPage   = regexp.MustCompile(`^/projects/[^/]+/files/[^/]+/folders/([^/]+)/?$`)
	itemPage     = regexp.MustCompile(`^/go/item/([^/]+)/?$`)
	yamlControls = regexp.MustCompile(`[\x{7f}-\x{9f}]`)
)

// unopenedReferences are the words for a reference the reader cannot open, in
// a page where no Markdown view says them.
var unopenedReferences = map[string]string{
	"document": "a document you cannot open",
	"upload":   "a file you cannot open",
}

// layout knows where every entry of the export goes and turns what a text
// says about Minka into links between the files.
type layout struct {
	format  string
	host    *url.URL // whose addresses name entries of Minka
	entries map[string]*node
}

func newLayout(format string, host *url.URL, root *node) *layout {
	return &layout{format: format, host: host, entries: entriesBelow(root)}
}

func (l *layout) extension() string {
	if l.format == FormatHTML {
		return "html"
	}
	return "md"
}

func (l *layout) indexName() string {
	if l.format == FormatHTML {
		return "index.html"
	}
	return "README.md"
}

// pathOf is the file an entry is written to: a folder's index, a document's
// page or an upload's file.
func (l *layout) pathOf(n *node) string {
	switch n.kind {
	case kindFolder:
		return path.Join(n.dirOf(), l.indexName())
	case kindUpload:
		return path.Join(n.placeOf(), withExtension(n.stem, extensionOf(n.file.Filename)))
	}
	return path.Join(n.placeOf(), n.stem+"."+l.extension())
}

// assetsDirOf is where the files hanging in a document go: beside it, named
// after it. No stem holds a dot, so no entry is ever called so.
func (l *layout) assetsDirOf(n *node) string {
	return path.Join(n.placeOf(), n.stem+".assets")
}

// asset is a file hanging in a document, downloaded beside it.
type asset struct {
	path   string
	blobID string
}

// rendered is a document as a file and the files hanging in it.
type rendered struct {
	content []byte
	assets  []asset
}

// renderDocument writes a document as a page of the format: its headings with
// their anchors, a file hanging in it as a link to the downloaded file, a
// reference to an entry of the export as a link to its file, anything else
// as its address in Minka.
func (l *layout) renderDocument(n *node) rendered {
	document := n.document
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(document.Body), context)
	if err != nil {
		nodes = []*xhtml.Node{{Type: xhtml.TextNode, Data: document.Body}}
	}
	// One parent for all, so that what stands at the top can be replaced too.
	container := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	for _, top := range nodes {
		container.AppendChild(top)
	}
	richtext.GiveHeadingsTheirAnchors([]*xhtml.Node{container})

	files := &assetNames{dir: l.assetsDirOf(n), byBlob: map[string]string{}, taken: map[string]bool{}}
	l.rewrite(container, n, files)
	if l.format == FormatHTML {
		readOutDecisions(container, document.DecisionBlocks)
	}
	var body strings.Builder
	for top := container.FirstChild; top != nil; top = top.NextSibling {
		_ = xhtml.Render(&body, top)
	}

	// Title, address and navigation are the tree's, as the index shows them,
	// whether the body was read now or by an earlier run.
	fields := []field{
		{"title", n.title}, {"id", n.id}, {"appUrl", n.appURL},
		{"inNavigation", n.inNavigation}, {"updatedAt", document.UpdatedAt},
	}
	var out string
	if l.format == FormatHTML {
		out = htmlPage(n.title, fields, "<h1>"+html.EscapeString(n.title)+"</h1>\n"+body.String())
	} else {
		out = frontmatter(fields) + "\n# " + markdownLine(n.title) + "\n"
		if markdown := richtext.ToMarkdownWithDecisions(body.String(), document.DecisionBlocks); markdown != "" {
			out += "\n" + markdown + "\n"
		}
	}
	return rendered{content: []byte(out), assets: files.assets}
}

// assetNames gives the files of one document their names: the file's own name
// as a slug, counted on where two meet, one file once however often it hangs
// in the text.
type assetNames struct {
	dir    string
	byBlob map[string]string
	taken  map[string]bool
	assets []asset
}

func (a *assetNames) pathOf(blobID, filename string) string {
	if existing, found := a.byBlob[blobID]; found {
		return existing
	}
	extension := extensionOf(filename)
	base := stemOf(strings.TrimSuffix(filename, path.Ext(filename)))
	if extension == "" {
		base = stemOf(filename)
	}
	name := withExtension(base, extension)
	for count := 2; a.taken[sameName(name)]; count++ {
		name = withExtension(base+"-"+strconv.Itoa(count), extension)
	}
	a.taken[sameName(name)] = true
	p := path.Join(a.dir, name)
	a.byBlob[blobID] = p
	a.assets = append(a.assets, asset{path: p, blobID: blobID})
	return p
}

// rewrite turns what a stored text carries into what the format shows, below
// one node.
func (l *layout) rewrite(n *xhtml.Node, document *node, files *assetNames) {
	if n.Type == xhtml.ElementNode {
		switch n.Data {
		case "action-text-attachment":
			l.rewriteAttachment(n, document, files)
			return
		case "minka-ref":
			l.rewriteReference(n, document)
			return
		case "minka-embed":
			if l.format == FormatHTML {
				if address := attribute(n, "url"); address != "" {
					replaceWith(n, link(address, address))
				}
			}
			return
		case "a":
			if target, fragment, found := l.entryAt(attribute(n, "href")); found {
				setAttribute(n, "href", l.linkTo(target, document, fragment))
			}
		}
	}
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		l.rewrite(child, document, files)
		child = next
	}
}

// rewriteAttachment makes a file hanging in the text a link to the file
// downloaded beside the document, a picture an image. A mention stays words,
// and a file the document no longer lists stays as the text shows it.
func (l *layout) rewriteAttachment(n *xhtml.Node, document *node, files *assetNames) {
	if attribute(n, "content-type") == "application/vnd.minka.person" {
		if l.format == FormatHTML {
			replaceWith(n, &xhtml.Node{Type: xhtml.TextNode, Data: attribute(n, "content")})
		}
		return
	}
	blobID, _, found := cutLast(attribute(n, "sgid"), "--")
	if !found {
		return
	}
	for _, file := range document.document.Attachments {
		if !strings.EqualFold(file.ID, blobID) {
			continue
		}
		target := relative(document.placeOf(), files.pathOf(file.ID, file.Filename))
		words := strings.TrimSpace(attribute(n, "caption"))
		if words == "" {
			words = file.Filename
		}
		if file.IsImage {
			replaceWith(n, &xhtml.Node{Type: xhtml.ElementNode, Data: "img", DataAtom: atom.Img,
				Attr: []xhtml.Attribute{{Key: "src", Val: target}, {Key: "alt", Val: words}}})
			return
		}
		replaceWith(n, link(target, words))
		return
	}
}

// rewriteReference makes a reference to an entry of the export a link to its
// file, at the section it names. Any other keeps its address in Minka: as the
// Markdown view writes it, and as a link in a page.
func (l *layout) rewriteReference(n *xhtml.Node, document *node) {
	kind, id := attribute(n, "kind"), attribute(n, "id")
	title := strings.TrimSpace(attribute(n, "title"))
	anchor := attribute(n, "anchor")
	if kind != "document" || !richtext.IsAnchor(anchor) {
		anchor = ""
	}
	// The tree's title, not the one Minka gave the reference when the page
	// was read: a page taken from the cache follows a rename all the same.
	if target := l.entries[keyOf(kind, id)]; target != nil && (kind == kindDocument || kind == kindUpload) {
		replaceWith(n, link(l.linkTo(target, document, anchor), target.title))
		return
	}
	if l.format != FormatHTML {
		return
	}
	address := attribute(n, "href")
	switch {
	case address != "" && title != "":
		replaceWith(n, link(address, title))
	case address != "":
		replaceWith(n, link(address, address))
	default:
		words, known := unopenedReferences[kind]
		if !known {
			words = "a reference you cannot open"
		}
		replaceWith(n, &xhtml.Node{Type: xhtml.TextNode, Data: "(" + words + ")"})
	}
}

// entryAt finds the entry of the export an address of Minka names: the page
// of a document, an upload or a folder, or "/go/item/…", on the host signed
// in to or without a host.
func (l *layout) entryAt(address string) (*node, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Path == "" {
		return nil, "", false
	}
	if parsed.Host != "" && (l.host == nil || !strings.EqualFold(parsed.Scheme, l.host.Scheme) || !strings.EqualFold(parsed.Host, l.host.Host)) {
		return nil, "", false
	}
	if parsed.Host == "" && (parsed.Scheme != "" || !strings.HasPrefix(parsed.Path, "/")) {
		return nil, "", false
	}
	candidates := []struct {
		page  *regexp.Regexp
		kinds []string
	}{
		{uploadPage, []string{kindUpload}},
		{folderPage, []string{kindFolder}},
		{documentPage, []string{kindDocument}},
		{itemPage, []string{kindDocument, kindUpload, kindFolder}},
	}
	for _, candidate := range candidates {
		match := candidate.page.FindStringSubmatch(parsed.Path)
		if match == nil {
			continue
		}
		for _, kind := range candidate.kinds {
			if target := l.entries[keyOf(kind, match[1])]; target != nil {
				fragment := ""
				if kind == kindDocument && richtext.IsAnchor(parsed.Fragment) {
					fragment = parsed.Fragment
				}
				return target, fragment, true
			}
		}
		return nil, "", false
	}
	return nil, "", false
}

// linkTo is the link from a document's file to an entry's file, at a section
// where one is named.
func (l *layout) linkTo(target, from *node, anchor string) string {
	address := relative(from.placeOf(), l.pathOf(target))
	if anchor != "" {
		address += "#" + anchor
	}
	return address
}

// renderIndex writes the index of a folder: what stands in it and below it in
// Minka's order, each a link to its file.
func (l *layout) renderIndex(folder *node, kind string, book *bookOfIndex) []byte {
	fields := []field{{"title", folder.title}, {"id", folder.id}, {"appUrl", folder.appURL}, {"kind", kind}}
	if folder.order != "" {
		fields = append(fields, field{"order", folder.order})
	}
	if book != nil {
		fields = append(fields, field{"book", book.title}, field{"bookId", book.id})
	}
	dir := folder.dirOf()
	if l.format == FormatHTML {
		var list strings.Builder
		l.htmlEntries(&list, folder, dir)
		return []byte(htmlPage(folder.title, fields, "<h1>"+html.EscapeString(folder.title)+"</h1>\n"+list.String()))
	}
	var out strings.Builder
	out.WriteString(frontmatter(fields))
	out.WriteString("\n# " + markdownLine(folder.title) + "\n")
	if len(folder.children) > 0 {
		out.WriteString("\n")
		l.markdownEntries(&out, folder, dir, "")
	}
	return []byte(out.String())
}

type bookOfIndex struct{ id, title string }

func (l *layout) markdownEntries(out *strings.Builder, folder *node, dir, indent string) {
	for _, n := range folder.children {
		fmt.Fprintf(out, "%s- [%s](%s)%s\n", indent, markdownLine(n.title), markdownDestination(relative(dir, l.pathOf(n))), entryNote(n))
		if n.kind == kindFolder {
			l.markdownEntries(out, n, dir, indent+"  ")
		}
	}
}

func (l *layout) htmlEntries(out *strings.Builder, folder *node, dir string) {
	if len(folder.children) == 0 {
		return
	}
	out.WriteString("<ul>\n")
	for _, n := range folder.children {
		fmt.Fprintf(out, "<li><a href=\"%s\">%s</a>%s", html.EscapeString(relative(dir, l.pathOf(n))), html.EscapeString(n.title), html.EscapeString(entryNote(n)))
		if n.kind == kindFolder {
			out.WriteString("\n")
			l.htmlEntries(out, n, dir)
		}
		out.WriteString("</li>\n")
	}
	out.WriteString("</ul>\n")
}

// entryNote says what a line of an index stands for beyond its title.
func entryNote(n *node) string {
	switch {
	case n.kind == kindDocument && !n.inNavigation:
		return " (not in navigation)"
	case n.kind == kindUpload:
		return " (file)"
	}
	return ""
}

type field struct {
	name  string
	value any
}

// frontmatter writes the fields as YAML, every string quoted the way JSON
// quotes it, which YAML reads as the same string. JSON leaves DEL and the C1
// controls as they are, which YAML takes in no quoted string, so they are
// escaped as well.
func frontmatter(fields []field) string {
	var out strings.Builder
	out.WriteString("---\n")
	for _, f := range fields {
		value, _ := json.Marshal(f.value)
		escaped := yamlControls.ReplaceAllStringFunc(string(value), func(r string) string {
			return fmt.Sprintf(`\u%04x`, []rune(r)[0])
		})
		fmt.Fprintf(&out, "%s: %s\n", f.name, escaped)
	}
	out.WriteString("---\n")
	return out.String()
}

// htmlPage is a minimal page: the fields as meta elements, the body as it is.
func htmlPage(title string, fields []field, body string) string {
	var head strings.Builder
	for _, f := range fields {
		value := fmt.Sprint(f.value)
		if f.name == "appUrl" {
			fmt.Fprintf(&head, "<link rel=\"canonical\" href=\"%s\">\n", html.EscapeString(value))
		}
		fmt.Fprintf(&head, "<meta name=\"minka:%s\" content=\"%s\">\n", f.name, html.EscapeString(value))
	}
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>" + html.EscapeString(title) + "</title>\n" +
		head.String() + "</head>\n<body>\n" + body + "\n</body>\n</html>\n"
}

// markdownLine escapes what Markdown would read as markup in a title.
func markdownLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	var out strings.Builder
	for _, r := range text {
		if strings.ContainsRune("\\`*_[]<>#!|~", r) {
			out.WriteRune('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// markdownDestination is a link's target as Markdown reads it whole.
func markdownDestination(target string) string {
	if strings.ContainsAny(target, " ()<>") {
		return "<" + target + ">"
	}
	return target
}

func link(address, words string) *xhtml.Node {
	a := &xhtml.Node{Type: xhtml.ElementNode, Data: "a", DataAtom: atom.A, Attr: []xhtml.Attribute{{Key: "href", Val: address}}}
	a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: words})
	return a
}

func replaceWith(old, replacement *xhtml.Node) {
	if old.Parent == nil {
		return
	}
	old.Parent.InsertBefore(replacement, old)
	old.Parent.RemoveChild(old)
}

func attribute(n *xhtml.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func setAttribute(n *xhtml.Node, name, value string) {
	for i, attr := range n.Attr {
		if attr.Key == name {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: name, Val: value})
}

// cutLast splits at the last separator, as Minka reads a file's handle: its
// identifier, two dashes, the signature.
func cutLast(s, separator string) (before, after string, found bool) {
	at := strings.LastIndex(s, separator)
	if at < 0 {
		return s, "", false
	}
	return s[:at], s[at+len(separator):], true
}
