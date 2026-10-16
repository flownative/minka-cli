package richtext

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// The anchor of a heading is Minka's rule, so that an anchor written here
// opens the same section of the page there: "h-" and the heading as a slug,
// unique within the text. The prefix keeps it apart from the page's own ids.
const (
	anchorPrefix    = "h-"
	namelessHeading = "section"
	maxAnchorLength = 200
	headingElements = "h1 h2 h3 h4 h5 h6"
)

var (
	anchorForm = regexp.MustCompile(`^h-[\p{L}\p{N}]+(?:-[\p{L}\p{N}]+)*$`)
	spelledOut = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
)

// HeadingAnchors are the anchors of the headings of one text, in their order;
// a repeated one is counted on from "-2". Each heading is its words with the
// whitespace collapsed.
func HeadingAnchors(headings []string) []string {
	taken := map[string]bool{}
	anchors := make([]string, len(headings))
	for i, heading := range headings {
		base := anchorPrefix + Slug(heading)
		anchor := base
		for count := 2; taken[anchor]; count++ {
			anchor = base + "-" + strconv.Itoa(count)
		}
		taken[anchor] = true
		anchors[i] = anchor
	}
	return anchors
}

// IsAnchor says whether a value has the shape of a heading's anchor, which is
// the only fragment Minka keeps on a reference to a document.
func IsAnchor(value string) bool {
	return len([]rune(value)) <= maxAnchorLength && anchorForm.MatchString(value)
}

// Slug is a heading in lower case, umlauts spelled out, accents dropped and
// every run of other characters one dash; letters of other scripts stay. It
// is "section" where nothing is left.
func Slug(heading string) string {
	lowered := spelledOut.Replace(strings.ToLower(strings.TrimSpace(heading)))
	var slug strings.Builder
	dash := false
	for _, r := range norm.NFD.String(lowered) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if dash && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			dash = false
			slug.WriteRune(r)
		default:
			dash = true
		}
	}
	if slug.Len() == 0 {
		return namelessHeading
	}
	return slug.String()
}

// GiveHeadingsTheirAnchors sets on every heading of the nodes the id Minka's
// page gives it, in document order, so that a reader goes to "#h-…" as on the
// page. The API serves no heading with an id; one with an id is rendered
// with its anchor by ToMarkdown.
func GiveHeadingsTheirAnchors(nodes []*html.Node) {
	headings := headingsOf(nodes)
	texts := make([]string, len(headings))
	for i, heading := range headings {
		texts[i] = headingText(heading)
	}
	for i, anchor := range HeadingAnchors(texts) {
		setAttribute(headings[i], "id", anchor)
	}
}

func headingsOf(nodes []*html.Node) []*html.Node {
	var found []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && isHeading(n) {
			found = append(found, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	for _, n := range nodes {
		visit(n)
	}
	return found
}

func isHeading(n *html.Node) bool {
	return len(n.Data) == 2 && strings.Contains(headingElements, n.Data)
}

// onALineOfTheirOwn are the elements Minka's text of a heading breaks the line
// around, which the anchor reads as a space.
var onALineOfTheirOwn = map[string]bool{
	"aside": true, "blockquote": true, "dd": true, "div": true, "dl": true, "dt": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true, "hr": true,
	"li": true, "ol": true, "p": true, "pre": true, "table": true, "tbody": true, "tr": true, "ul": true,
}

func headingText(heading *html.Node) string {
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			text.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br":
			text.WriteByte(' ')
		case n.Type == html.ElementNode && n.Data == "minka-embed":
			text.WriteString(" " + attribute(n, "url") + " ")
		}
		ownLine := n != heading && n.Type == html.ElementNode && onALineOfTheirOwn[n.Data]
		if ownLine {
			text.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if ownLine {
			text.WriteByte(' ')
		}
	}
	visit(heading)
	return strings.Join(strings.Fields(text.String()), " ")
}

func setAttribute(n *html.Node, name, value string) {
	for i, attr := range n.Attr {
		if attr.Key == name {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: name, Val: value})
}
