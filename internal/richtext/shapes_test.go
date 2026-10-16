package richtext

import (
	"testing"

	"github.com/flownative/minka-cli/internal/markdown"
)

const servedTileReference = `<minka-ref kind="document" id="01a0" title="Setup" href="https://minka.example/projects/p/files/t/01a0"></minka-ref>`

const tileAddress = "https://minka.example/projects/p/files/t/01a0"

// Each shape of a document as Minka serves it, read in the convention the
// markdown package writes it back from.
func TestShapesAreReadInTheirConventions(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"callout", `<aside data-callout="note"><p>Text</p></aside>`, "> [!NOTE]\n>\n> Text"},
		{"callout with a title", `<aside data-callout="warning"><header>Heads <strong>up</strong></header><p>One</p><ul><li>two</li></ul></aside>`, "> [!WARNING] Heads **up**\n>\n> One\n>\n> - two"},
		{"callout with a title only", `<aside data-callout="note"><header>Only</header></aside>`, "> [!NOTE] Only"},
		{"callout with a break in its title", `<aside data-callout="note"><header>a<br>b</header></aside>`, "> [!NOTE] a b"},
		{"callout between blocks", "<p>a</p>\n<aside data-callout=\"note\">\n<p>b</p>\n</aside>\n<p>c</p>", "a\n\n> [!NOTE]\n>\n> b\n\nc"},
		{"words that look like a marker", `<blockquote><p>[!NOTE] x</p></blockquote>`, `> \[!NOTE\] x`},
		{"aside of another kind keeps its words", `<aside data-callout="tip"><p>x</p></aside><aside><p>y</p></aside>`, "x\n\ny"},
		{"header outside a callout", `<header>x</header><p>y</p>`, "x\n\ny"},
		{"mermaid diagram as the editor writes it", `<pre data-language="mermaid">graph LR<br>A[Start] --&gt; B["&lt;b&gt;End&lt;/b&gt;"]</pre>`, "```mermaid\ngraph LR\nA[Start] --> B[\"<b>End</b>\"]\n```"},
		{"code block with the language for none", `<pre data-language="plain" data-highlight-language="plain">x<br>  y</pre>`, "```\nx\n  y\n```"},
		{"code block as it was written before", `<pre data-language="go"><code>a
b</code></pre>`, "```go\na\nb\n```"},
		{"tiles", `<dl data-tiles=""><div><dt>` + servedTileReference + `</dt><dd>The <em>setup</em></dd></div><div><dt>` + servedTileReference + `</dt></div></dl>`, markdown.MarkerTiles + "\n- <" + tileAddress + "> The *setup*\n- <" + tileAddress + ">"},
		{"tile the reader cannot open", `<dl data-tiles=""><div><dt><minka-ref kind="event" id="01a0"></minka-ref></dt><dd>Party</dd></div></dl>`, markdown.MarkerTiles + "\n- (an event you cannot open) Party"},
		{"tile with a break in its description", `<dl data-tiles=""><div><dt>` + servedTileReference + `</dt><dd>a<br>b</dd></div></dl>`, markdown.MarkerTiles + "\n- <" + tileAddress + "> a b"},
		{"terms without their tile", `<dl data-tiles=""><dt>` + servedTileReference + `</dt><dd>One</dd><dt>` + servedTileReference + `</dt></dl>`, markdown.MarkerTiles + "\n- <" + tileAddress + "> One\n- <" + tileAddress + ">"},
		{"description list of another kind", `<dl><dt>a</dt><dd>b</dd></dl>`, "a\n\nb"},
		{
			"variables as the editor writes them",
			`<aside data-variables=""><figure class="lexxy-content__table-wrapper"><table><tbody><tr><th class="lexxy-content__table-cell--header"><p>Name</p></th><th class="lexxy-content__table-cell--header"><p>Label</p></th><th class="lexxy-content__table-cell--header"><p>Hint</p></th><th class="lexxy-content__table-cell--header"><p>Default</p></th><th class="lexxy-content__table-cell--header"><p>Live</p></th></tr><tr><th class="lexxy-content__table-cell--header"><p>node_name</p></th><td><p>Node</p></td><td><p></p></td><td><p></p></td><td><p>wk-a</p></td></tr></tbody></table></figure></aside>`,
			markdown.MarkerVariables + "\n| Name | Label | Hint | Default | Live |\n| --- | --- | --- | --- | --- |\n| node_name | Node |  |  | wk-a |",
		},
		{"variables around a bare table", `<aside data-variables=""><table><tbody><tr><th>Name</th></tr><tr><th><code>host</code></th></tr></tbody></table></aside>`, markdown.MarkerVariables + "\n| Name |\n| --- |\n| `host` |"},
		{"placeholders", `<p>Drain {{node_name}}.</p>`, "Drain {{node_name}}."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ToMarkdown(test.html); got != test.want {
				t.Errorf("\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// What the CLI writes comes back from its own view unchanged, and so does a
// document written in the editor, in the form the CLI writes it.
func TestShapesComeBackFromTheirView(t *testing.T) {
	sources := []string{
		"> [!NOTE] Before you *start*\n> Drain {{node_name}} first.\n>\n> - one\n> - two",
		"> [!WARNING]\n> No title.\n\n> just a quote",
		"```mermaid\ngraph LR\nA[Start] --> B[\"<b>End</b>\"]\n```\n\n```\nplain & <simple>\n\n  indented\n```",
		markdown.MarkerTiles + "\n- <" + tileAddress + "> The *setup*\n- <" + tileAddress + ">",
		markdown.MarkerVariables + "\n| Name | Label | Hint | Default | Staging | Live |\n| --- | --- | --- | --- | --- | --- |\n| `node_name` | Node | e.g. wk-1 |  | wk-s | wk-a |",
		"- a\n\n" + markdown.MarkerTiles + "\n- <" + tileAddress + ">\n\nafter",
	}
	for _, source := range sources {
		original := markdown.ToHTML(source)
		if len(original.Warnings) > 0 {
			t.Errorf("%q gave warnings %q", source, original.Warnings)
		}
		markdownOfIt := ToMarkdown(original.HTML)
		if roundTrip := markdown.ToHTML(markdownOfIt).HTML; roundTrip != original.HTML {
			t.Errorf("\nsource:   %q\noriginal: %q\nmarkdown: %q\nback:     %q", source, original.HTML, markdownOfIt, roundTrip)
		}
	}

	// As the editor stores it and Minka serves it: cells hold paragraphs, a
	// code block names its language twice, nothing stands between the blocks.
	const editor = `<h2>Setup</h2><aside data-callout="warning"><header>Heads <strong>up</strong></header><p>Drain {{node_name}}.</p><p>Second.</p></aside>` +
		`<pre data-language="mermaid" data-highlight-language="mermaid">graph TD<br>A --&gt; B</pre>` +
		`<dl data-tiles=""><div><dt>` + servedTileReference + `</dt><dd>Words</dd></div></dl>` +
		`<aside data-variables=""><figure class="lexxy-content__table-wrapper"><table><tbody><tr><th class="lexxy-content__table-cell--header"><p>Name</p></th><th class="lexxy-content__table-cell--header"><p>Label</p></th></tr><tr><th class="lexxy-content__table-cell--header"><p>node_name</p></th><td><p>Node</p></td></tr></tbody></table></figure></aside><p>end</p>`
	const written = "<h2>Setup</h2>\n" +
		"<aside data-callout=\"warning\">\n<header>Heads <strong>up</strong></header>\n<p>Drain {{node_name}}.</p>\n<p>Second.</p>\n</aside>\n" +
		"<pre data-language=\"mermaid\">graph TD<br>A --&gt; B</pre>\n" +
		`<dl data-tiles=""><div><dt><a href="` + tileAddress + `">` + tileAddress + `</a></dt><dd>Words</dd></div></dl>` + "\n" +
		`<aside data-variables=""><figure class="lexxy-content__table-wrapper"><table><tbody>` + "\n" +
		`<tr><th class="lexxy-content__table-cell--header">Name</th><th class="lexxy-content__table-cell--header">Label</th></tr>` + "\n" +
		`<tr><th class="lexxy-content__table-cell--header">node_name</th><td>Node</td></tr>` + "\n" +
		"</tbody></table></figure></aside>\n<p>end</p>"
	back := markdown.ToHTML(ToMarkdown(editor))
	if back.HTML != written || len(back.Warnings) > 0 {
		t.Errorf("\n got: %q %q\nwant: %q", back.HTML, back.Warnings, written)
	}
	if loss := Dropped(editor, back.HTML); loss.Any() {
		t.Errorf("the tile's reference counts as lost: %+v", loss)
	}
}

// A value made of nothing but a shape is markup to Minka, so it is not kept
// as the characters it consists of.
func TestTheElementsOfTheShapesAreMarkup(t *testing.T) {
	for _, value := range []string{
		`<aside data-callout="note">x</aside>`,
		`<header>x</header>`,
		`<dl data-tiles="">x</dl>`,
		`<dt>x</dt>`,
		`<dd>x</dd>`,
		`text </aside>`,
	} {
		if !CarriesMarkup(value) {
			t.Errorf("CarriesMarkup(%q) = false", value)
		}
	}
	if CarriesMarkup("<asides> and <dlx>") {
		t.Error("an element of another name counts as markup")
	}
}
