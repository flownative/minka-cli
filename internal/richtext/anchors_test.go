package richtext

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The anchors of the headings are Minka's: "h-" and a slug, umlauts spelled
// out, accents dropped by decomposing the letter, other characters a dash, a
// repeat counted on.
func TestTheAnchorsOfHeadingsAreMinkas(t *testing.T) {
	headings := []string{"Setup", "Setup", "Über die Größe", "Café crème", " ¿? ", "Привет, мир", "A -- b", "h-setup-2", "Setup", "Ångström", "Łódź", "한국"}
	want := []string{"h-setup", "h-setup-2", "h-ueber-die-groesse", "h-cafe-creme", "h-section", "h-привет-мир", "h-a-b", "h-h-setup-2", "h-setup-3", "h-angstroem", "h-łodz",
		// Decomposed as Minka decomposes it: the syllables fall apart into
		// their letters, which stay.
		"h-한국"}
	if got := HeadingAnchors(headings); !slices.Equal(got, want) {
		t.Fatalf("anchors %q, want %q", got, want)
	}
}

// Only the shape Minka gives is an anchor; it keeps no other fragment on a
// reference.
func TestAnAnchorHasTheShapeMinkaGives(t *testing.T) {
	for value, want := range map[string]bool{
		"h-setup": true, "h-setup-2": true, "h-привет-мир": true,
		"h-": false, "setup": false, "h-Setup ": false, "h--a": false, "h-a-": false, "comment-1": false,
		"h-" + strings.Repeat("a", 198): true, "h-" + strings.Repeat("a", 199): false,
	} {
		if IsAnchor(value) != want {
			t.Errorf("IsAnchor(%q) = %v", value, !want)
		}
	}
}

// A heading is read as Minka reads it for its anchor: every heading wherever
// it stands, a line break a space, a reference or a file no words at all.
func TestTheWordsOfAHeadingAreReadAsMinkaReadsThem(t *testing.T) {
	nodes := parseFragment(`<h2>Zugang  <em>prüfen</em></h2><aside data-callout="note"><h3>Line<br>break</h3></aside>` +
		`<h3><minka-ref kind="document" id="x" title="Other"></minka-ref> Setup</h3><p>text</p><h4>a<p>b</p></h4>`)
	GiveHeadingsTheirAnchors(nodes)
	var ids []string
	for _, heading := range headingsOf(nodes) {
		ids = append(ids, attribute(heading, "id"))
	}
	if !slices.Equal(ids, []string{"h-zugang-pruefen", "h-line-break", "h-setup", "h-a-b"}) {
		t.Fatalf("anchors %q", ids)
	}
}

// A heading that carries its anchor keeps it in Markdown as the anchor
// GitHub and the rest honour, right above it; one without stays as it was.
func TestAHeadingWithItsAnchorKeepsItInMarkdown(t *testing.T) {
	nodes := parseFragment(`<h2>Zugang prüfen</h2><p>Erst.</p><blockquote><h3>Zugang prüfen</h3></blockquote><h4 id="not an anchor">Plain</h4>`)
	GiveHeadingsTheirAnchors(nodes[:3])
	var out strings.Builder
	for _, n := range nodes {
		_ = html.Render(&out, n)
	}
	want := "<a id=\"h-zugang-pruefen\"></a>\n## Zugang prüfen\n\nErst.\n\n> <a id=\"h-zugang-pruefen-2\"></a>\n> ### Zugang prüfen\n\n#### Plain"
	if got := ToMarkdown(out.String()); got != want {
		t.Fatalf("markdown:\n%s\nwant:\n%s", got, want)
	}
	if got := ToMarkdown(`<h2>Zugang prüfen</h2>`); got != "## Zugang prüfen" {
		t.Fatalf("without an id: %q", got)
	}
}

func parseFragment(fragment string) []*html.Node {
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		panic(err)
	}
	return nodes
}
