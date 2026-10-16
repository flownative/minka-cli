package markdown

import (
	"slices"
	"testing"
)

const tileAddress = "https://minka.example/go/card/MNK-1"

const tileLink = `<a href="` + tileAddress + `">` + tileAddress + `</a>`

// Each shape is written as the editor stores it, so that Minka keeps what
// was sent and the page draws it.
func TestShapesAreWrittenAsTheEditorStoresThem(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		html     string
		warnings []string
	}{
		{"callout", "> [!NOTE]\n> Text", "<aside data-callout=\"note\">\n<p>Text</p>\n</aside>", nil},
		{"callout with a title", "> [!NOTE] Before *you* start\n> Text", "<aside data-callout=\"note\">\n<header>Before <em>you</em> start</header>\n<p>Text</p>\n</aside>", nil},
		{"callout with its title apart", "> [!WARNING] Careful\n>\n> One\n>\n> - two", "<aside data-callout=\"warning\">\n<header>Careful</header>\n<p>One</p>\n<ul>\n<li>two</li>\n</ul>\n</aside>", nil},
		{"callout with a title only", "> [!WARNING] Careful", "<aside data-callout=\"warning\">\n<header>Careful</header>\n</aside>", nil},
		{"callout kind in any case", "> [!warning]\n> x", "<aside data-callout=\"warning\">\n<p>x</p>\n</aside>", nil},
		{"callout with a hard break after its title", "> [!NOTE] Title  \n> x", "<aside data-callout=\"note\">\n<header>Title</header>\n<p>x</p>\n</aside>", nil},
		{"callout with a link in its title", "> [!NOTE] See <https://example.com>\n> x", "<aside data-callout=\"note\">\n<header>See <a href=\"https://example.com\">https://example.com</a></header>\n<p>x</p>\n</aside>", nil},
		{"alert Minka has no kind for stays a quote", "> [!TIP]\n> x", "<blockquote>\n<p>[!TIP]\nx</p>\n</blockquote>", nil},
		{"marker after words stays a quote", "> a [!NOTE]", "<blockquote>\n<p>a [!NOTE]</p>\n</blockquote>", nil},
		{"escaped marker stays a quote", "> \\[!NOTE\\] x", "<blockquote>\n<p>[!NOTE] x</p>\n</blockquote>", nil},
		{"callout in a callout stays a quote", "> [!NOTE]\n> > [!WARNING]\n> > x", "<aside data-callout=\"note\">\n<blockquote>\n<p>[!WARNING]\nx</p>\n</blockquote>\n</aside>", nil},
		{"callout in a quote", "> > [!NOTE]\n> > x", "<blockquote>\n<aside data-callout=\"note\">\n<p>x</p>\n</aside>\n</blockquote>", nil},
		{
			"tiles",
			MarkerTiles + "\n- <" + tileAddress + "> The *card*\n- " + tileAddress + "\n- #MNK-2 Second",
			`<dl data-tiles=""><div><dt>` + tileLink + `</dt><dd>The <em>card</em></dd></div><div><dt>` + tileLink + `</dt></div><div><dt>#MNK-2</dt><dd>Second</dd></div></dl>`,
			nil,
		},
		{"tiles with a marker of other spacing", "<!--minka:tiles   -->\n- <" + tileAddress + ">", `<dl data-tiles=""><div><dt>` + tileLink + `</dt></div></dl>`, nil},
		{"tiles after a blank line", MarkerTiles + "\n\n* <" + tileAddress + ">", `<dl data-tiles=""><div><dt>` + tileLink + `</dt></div></dl>`, nil},
		{
			"line without an address between tiles",
			MarkerTiles + "\n- <" + tileAddress + ">\n- [words](" + tileAddress + ")\n- #MNK-1-2\n- <" + tileAddress + ">",
			`<dl data-tiles=""><div><dt>` + tileLink + `</dt></div></dl>` + "\n" +
				`<p><a href="` + tileAddress + `">words</a></p>` + "\n<p>#MNK-1-2</p>\n" +
				`<dl data-tiles=""><div><dt>` + tileLink + `</dt></div></dl>`,
			[]string{WarningTileWithoutAddress},
		},
		{"address Minka does not keep", MarkerTiles + "\n- <ftp://example.com/x>", "<p>ftp://example.com/x</p>", []string{WarningTileWithoutAddress, WarningUnsafeLinks}},
		{"tile of more than one line", MarkerTiles + "\n- <" + tileAddress + ">\n\n  more", "<ul>\n<li><p>" + tileLink + "</p>\n<p>more</p>\n</li>\n</ul>", []string{WarningTileOfOneLine}},
		{
			"variables",
			MarkerVariables + "\n| Name | Label | Hint | Default | Live |\n|---|---|---|---|---|\n| `node_name` | Node | e.g. wk-1 | | wk-a |",
			`<aside data-variables=""><figure class="lexxy-content__table-wrapper"><table><tbody>` + "\n" +
				`<tr><th class="lexxy-content__table-cell--header">Name</th><th class="lexxy-content__table-cell--header">Label</th><th class="lexxy-content__table-cell--header">Hint</th><th class="lexxy-content__table-cell--header">Default</th><th class="lexxy-content__table-cell--header">Live</th></tr>` + "\n" +
				`<tr><th class="lexxy-content__table-cell--header"><code>node_name</code></th><td>Node</td><td>e.g. wk-1</td><td></td><td>wk-a</td></tr>` + "\n" +
				`</tbody></table></figure></aside>`,
			nil,
		},
		{"marker of variables before a list is text", MarkerVariables + "\n- a", "<pre data-language=\"plain\">&lt;!-- minka:variables --&gt;</pre>\n<ul>\n<li>a</li>\n</ul>", []string{WarningRawHTML}},
		{"marker of tiles before a table is text", MarkerTiles + "\n| a |\n|---|", "<pre data-language=\"plain\">&lt;!-- minka:tiles --&gt;</pre>\n<figure class=\"lexxy-content__table-wrapper\"><table><tbody>\n<tr><th class=\"lexxy-content__table-cell--header\">a</th></tr>\n</tbody></table></figure>", []string{WarningRawHTML}},
		{
			"marker of variables inside a callout is text",
			"> [!NOTE]\n> " + MarkerVariables + "\n> | Name |\n> |---|",
			"<aside data-callout=\"note\">\n<pre data-language=\"plain\">&lt;!-- minka:variables --&gt;</pre>\n<figure class=\"lexxy-content__table-wrapper\"><table><tbody>\n<tr><th class=\"lexxy-content__table-cell--header\">Name</th></tr>\n</tbody></table></figure>\n</aside>",
			[]string{WarningRawHTML},
		},
		{"tiles inside a callout", "> [!NOTE]\n> " + MarkerTiles + "\n> - <" + tileAddress + ">", "<aside data-callout=\"note\">\n<dl data-tiles=\"\"><div><dt>" + tileLink + "</dt></div></dl>\n</aside>", nil},
		{"mail address is no tile", MarkerTiles + "\n- <mail@example.com> Write\n- <mailto:a@example.com>", "<p><a href=\"mailto:mail@example.com\">mail@example.com</a> Write</p>\n<p><a href=\"mailto:a@example.com\">mailto:a@example.com</a></p>", []string{WarningTileWithoutAddress}},
		{"empty line of tiles", MarkerTiles + "\n- <" + tileAddress + ">\n-", `<dl data-tiles=""><div><dt>` + tileLink + `</dt></div></dl>`, []string{WarningEmptyTile}},
		// Markdown reads a marker with "---" under it as a heading, which no
		// callout starts with; a blank line before the rule makes it one.
		{"rule right under a marker makes a heading of it", "> [!NOTE]\n> ---", "<blockquote>\n<h2>[!NOTE]</h2>\n</blockquote>", nil},
		{"rule after a blank line in a callout", "> [!NOTE]\n>\n> ---", "<aside data-callout=\"note\">\n<hr>\n</aside>", nil},
		{"placeholders stay as they are", "Drain {{node_name}}.\n\n```sh\nkubectl drain {{node_name}}\n```", "<p>Drain {{node_name}}.</p>\n<pre data-language=\"sh\">kubectl drain {{node_name}}</pre>", nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ToHTML(test.markdown)
			if result.HTML != test.html {
				t.Errorf("HTML\n got: %q\nwant: %q", result.HTML, test.html)
			}
			if !slices.Equal(result.Warnings, test.warnings) {
				t.Errorf("warnings\n got: %q\nwant: %q", result.Warnings, test.warnings)
			}
		})
	}
}

// In a comment every line break is kept; the title still ends with its line.
func TestACalloutKeepsTheLineBreaksOfAComment(t *testing.T) {
	got := ToHTML("> [!NOTE] Title\n> one\n> two", KeepLineBreaks()).HTML
	if want := "<aside data-callout=\"note\">\n<header>Title</header>\n<p>one<br>two</p>\n</aside>"; got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}
