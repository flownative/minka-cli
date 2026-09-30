package richtext

import (
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/markdown"
)

func TestToMarkdown(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"empty", "", ""},
		{"plain text as the server stores it", "<div>line one<br>line two</div>", "line one\\\nline two"},
		{"paragraphs", "<p>a</p><p>b</p>", "a\n\nb"},
		{"heading", "<h1>Title</h1><p>text</p>", "# Title\n\ntext"},
		{"lower headings", "<h2>a</h2><h3>b</h3>", "## a\n\n### b"},
		{"deep headings", "<h4>a</h4><h6>b</h6>", "#### a\n\n###### b"},
		{"ordered list starting elsewhere", `<ol start="3"><li>x</li><li>y</li></ol>`, "3. x\n4. y"},
		{"code block with a language", `<pre data-language="go"><code>x</code></pre>`, "```go\nx\n```"},
		{"code block with a language that is none", `<pre data-language="a b"><code>x</code></pre>`, "```\nx\n```"},
		{"link with a title", `<p><a href="https://example.com" title="Home">site</a> <a href="https://x.y" title="X">https://x.y</a></p>`, `[site](https://example.com "Home") [https://x.y](https://x.y "X")`},
		{"table", `<figure class="lexxy-content__table-wrapper"><table><tbody><tr><th>a</th><th>b|c</th></tr><tr><td>1</td></tr></tbody></table></figure>`, "| a | b\\|c |\n| --- | --- |\n| 1 |  |"},
		{"attachment as the server serves it", `<figure class="attachment attachment--image"><img src="https://minka.example/f/1.png" alt="Plan" width="800"><figcaption>Der Plan</figcaption></figure>`, "![Plan](https://minka.example/f/1.png)\n\nDer Plan"},
		{"mention", `<p>Hallo <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment></p>`, "Hallo @Anke"},
		{"picture in the source", `<action-text-attachment sgid="x" content-type="image/png" url="https://minka.example/f/zelt.png" filename="zelt.png"></action-text-attachment>`, "![zelt.png](https://minka.example/f/zelt.png)"},
		{"file in the source", `<p><action-text-attachment sgid="x" content-type="application/pdf" url="https://minka.example/f/plan.pdf" filename="plan.pdf"></action-text-attachment></p>`, "[plan.pdf](https://minka.example/f/plan.pdf)"},
		{"stored attachment without an address", `<action-text-attachment sgid="x" caption="Das Zelt"></action-text-attachment>`, "\\[Das Zelt\\]"},
		{"marks Markdown has no syntax for", "<p><u>u</u> <s>s</s> <i>i</i> x<sub>2</sub> <mark>m</mark></p>", "u ~~s~~ *i* x2 m"},
		{"rule", "<p>a</p><hr><p>b</p>", "a\n\n---\n\nb"},
		{"inline code", "<p><code>x *y*</code> and <code>a`b</code> and <code>`c</code></p>", "`x *y*` and ``a`b`` and `` `c ``"},
		{"code block", "<pre><code>a &lt;b&gt;\n</code></pre>", "```\na <b>\n```"},
		{"inline", "<p><strong>b</strong> <em>i</em> <del>d</del></p>", "**b** *i* ~~d~~"},
		{"spaces at the edge of inline elements", "<p>a<strong> b </strong>c</p>", "a **b** c"},
		{"links", `<p><a href="https://example.com">site</a> <a href="https://x.y">https://x.y</a> <a href="/rel">/rel</a> <a href="/a b">x</a></p>`, "[site](https://example.com) <https://x.y> [/rel](/rel) [x](</a b>)"},
		{"lists", "<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul><ol><li>x</li><li>y</li></ol>", "- a\n- b\n  - c\n\n1. x\n2. y"},
		{"list item with paragraphs", "<ul><li><p>a</p><p>b</p></li></ul>", "- a\n\n  b"},
		{"quote", "<blockquote><p>q1</p><p>q2</p></blockquote>", "> q1\n>\n> q2"},
		{"pre", "<pre>a &lt;b&gt;\n  c\n</pre>", "```\na <b>\n  c\n```"},
		{"pre with a fence inside", "<pre>x ``` y</pre>", "````\nx ``` y\n````"},
		{"markup characters in text", "<div>*a* _b_ snake_case `c` [d] &lt;e&gt; ~f~ \\ &amp;amp; AT&amp;T</div>", "\\*a\\* \\_b\\_ snake_case \\`c\\` \\[d\\] \\<e> \\~f\\~ \\\\ \\&amp; AT&T"},
		{"line starts that look like markup", "<div># h<br>- item<br>1. one<br>&gt; quote<br>===</div>", "\\# h\\\n\\- item\\\n1\\. one\\\n\\> quote\\\n\\==="},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ToMarkdown(test.html); got != test.want {
				t.Errorf("\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// What a card shows can be edited and sent back without losing or gaining
// elements.
func TestMarkdownOfADescriptionConvertsBackToTheSameHTML(t *testing.T) {
	sources := []string{
		"# Title\n\nSome **bold** and *em* with [a link](https://example.com) and <https://example.org>.\n\n- one\n- two\n  - nested\n\n1. first\n2. second\n\n> quoted\n\n````\ncode ``` <x>\n````",
		"Text with \\*stars\\*, \\_under\\_, snake_case, a \\`tick\\`, \\[brackets\\] and \\<angle>.  \nAfter a hard break.",
		"\\# not a heading\\\n\\- not a list\\\n1\\. not a list either\\\n\\> not a quote",
		"AT&T \\&copy; ~~gone~~ and [a path](/projects/x)",
		"#### Four\n\n##### Five\n\n1. one\n\ntext\n\n3. three\n4. four\n\n| a | *b* |\n| --- | --- |\n| 1 | [x](https://example.com \"T\") |\n| 2 |  |",
		"## Second\n\n### Third\n\n---\n\nUse `a <b>` and `` c`d `` and `` `e ``.\n\n```go\nfunc main() {}\n```",
	}

	for _, source := range sources {
		original := markdown.ToHTML(source).HTML
		markdownOfIt := ToMarkdown(original)
		if roundTrip := markdown.ToHTML(markdownOfIt).HTML; roundTrip != original {
			t.Errorf("\nsource:   %q\noriginal: %q\nmarkdown: %q\nback:     %q", source, original, markdownOfIt, roundTrip)
		}
	}
}

func TestTextWithoutMarkupStaysText(t *testing.T) {
	for _, stored := range []string{
		"<div>*not em* and # x</div>",
		"<div>1. not a list</div>",
		"<p>- b</p>",
		"<div>&lt;b&gt;not bold&lt;/b&gt;</div>",
	} {
		back := markdown.ToHTML(ToMarkdown(stored)).HTML
		for _, element := range []string{"<em>", "<ol>", "<ul>", "<h1>", "<b>"} {
			if strings.Contains(back, element) {
				t.Errorf("%q came back as %q", stored, back)
			}
		}
	}

	if back := markdown.ToHTML(ToMarkdown("<div>line one<br>line two</div>")).HTML; back != "<p>line one<br>line two</p>" {
		t.Errorf("line break lost: %q", back)
	}
}

// Every shape a reference comes in, as Minka serves it for the reader: kind
// and identifier stored, title, short identifier and address added on the way
// out, nothing beyond kind and identifier for a target the reader cannot open.
func TestReferencesAreShownByTheirAddress(t *testing.T) {
	const card = `<minka-ref kind="card" id="01a0" title="Kassenprüfung" short-id="MNK-1" href="https://minka.example/go/card/MNK-1"></minka-ref>`
	const cardLink = "<https://minka.example/go/card/MNK-1>"
	tests := []struct {
		name string
		html string
		want string
	}{
		{"card with a short identifier", "<p>See " + card + ".</p>", "See " + cardLink + "."},
		{"card on a board without short identifiers", `<p><minka-ref kind="card" id="01a0" title="Zelt" href="https://minka.example/projects/p/boards/b/cards/01a0"></minka-ref></p>`, "<https://minka.example/projects/p/boards/b/cards/01a0>"},
		{"post", `<p><minka-ref kind="post" id="01a0" title="Zeltverleih" href="https://minka.example/projects/p/posts/t/01a0"></minka-ref></p>`, "<https://minka.example/projects/p/posts/t/01a0>"},
		{"chat", `<p><minka-ref kind="chat" id="01a0" title="Allgemein" href="https://minka.example/projects/p/chats/c"></minka-ref></p>`, "<https://minka.example/projects/p/chats/c>"},
		{"card the reader cannot open", `<p>Siehe <minka-ref kind="card" id="01a0"></minka-ref></p>`, "Siehe (a card you cannot open)"},
		{"post the reader cannot open", `<p><minka-ref kind="post" id="01a0"></minka-ref></p>`, "(a post you cannot open)"},
		{"chat the reader cannot open", `<p><minka-ref kind="chat" id="01a0"></minka-ref></p>`, "(a chat you cannot open)"},
		{"kind the CLI does not know yet, visible", `<p><minka-ref kind="todo" id="01a0" title="Kaufen" href="https://minka.example/t/1"></minka-ref></p>`, "<https://minka.example/t/1>"},
		{"kind the CLI does not know yet, hidden", `<p><minka-ref kind="todo" id="01a0"></minka-ref></p>`, "(a reference you cannot open)"},
		{"title with Markdown characters stays out of the view", "<p><minka-ref kind=\"card\" id=\"01a0\" title=\"*Zelt* [neu] &lt;b&gt; a`b` &quot;q&quot;\" short-id=\"MNK-1\" href=\"https://minka.example/go/card/MNK-1\"></minka-ref></p>", cardLink},
		{"no address", `<p>See <minka-ref kind="card" id="01a0" title="*Zelt*" short-id="MNK-1"></minka-ref></p>`, "See #MNK-1 \\*Zelt\\*"},
		{"no address at the start of a line", `<p><minka-ref kind="card" id="01a0" title="Zelt" short-id="MNK-1"></minka-ref> is done</p>`, "\\#MNK-1 Zelt is done"},
		{"address that is no web address", `<p><minka-ref kind="post" id="01a0" title="P" href="/projects/p/posts/t/1"></minka-ref></p>`, "[/projects/p/posts/t/1](/projects/p/posts/t/1)"},
		{"address with a space", `<p><minka-ref kind="post" id="01a0" title="P" href="https://minka.example/a b"></minka-ref></p>`, "[https://minka.example/a b](<https://minka.example/a b>)"},
		{"in a list", "<ul><li>" + card + "</li><li><minka-ref kind=\"card\" id=\"01a1\"></minka-ref></li></ul>", "- " + cardLink + "\n- (a card you cannot open)"},
		{"in a heading", "<h2>About " + card + "</h2>", "## About " + cardLink},
		{"in a quote", "<blockquote><p>" + card + "</p></blockquote>", "> " + cardLink},
		{"in a table", "<figure class=\"lexxy-content__table-wrapper\"><table><tbody><tr><th>a</th></tr><tr><td>" + card + "</td></tr></tbody></table></figure>", "| a |\n| --- |\n| " + cardLink + " |"},
		{"in emphasis", "<p><strong>" + card + "</strong></p>", "**" + cardLink + "**"},
		{"two in a sentence", "<p>" + card + " and <minka-ref kind=\"chat\" id=\"01a2\" title=\"Allgemein\" href=\"https://minka.example/c\"></minka-ref></p>", cardLink + " and <https://minka.example/c>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ToMarkdown(test.html); got != test.want {
				t.Errorf("\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// The view of a visible reference, sent back, is a link that shows its own
// address, which is what Minka turns into the reference again.
func TestAReferenceComesBackAsALinkShowingItsAddress(t *testing.T) {
	served := `<p>See <minka-ref kind="card" id="01a0" title="*Zelt*" short-id="MNK-1" href="https://minka.example/go/card/MNK-1"></minka-ref>.</p>`
	want := `<p>See <a href="https://minka.example/go/card/MNK-1">https://minka.example/go/card/MNK-1</a>.</p>`
	back := markdown.ToHTML(ToMarkdown(served)).HTML
	if back != want {
		t.Errorf("\n got: %q\nwant: %q", back, want)
	}
	if loss := Dropped(served, back); loss.Any() {
		t.Errorf("the reference counts as lost after its own view came back: %+v", loss)
	}
}

// An exclamation mark before a link would make the link an image.
func TestAnExclamationMarkBeforeALinkStaysText(t *testing.T) {
	for served, want := range map[string]string{
		`<p>Look!<a href="https://example.com">site</a></p>`:                  `Look\![site](https://example.com)`,
		`<p>a\!<a href="https://example.com">site</a></p>`:                    `a\\\![site](https://example.com)`,
		`<p>Look!<minka-ref kind="post" id="1" href="/p"></minka-ref></p>`:    `Look\![/p](/p)`,
		`<p>Look! <a href="https://example.com">site</a></p>`:                 `Look! [site](https://example.com)`,
		`<p>Look!<strong><a href="https://example.com">site</a></strong></p>`: `Look!**[site](https://example.com)**`,
	} {
		got := ToMarkdown(served)
		if got != want {
			t.Errorf("%s\n got: %q\nwant: %q", served, got, want)
		}
		if back := markdown.ToHTML(got).HTML; strings.Contains(back, "<img") || !strings.Contains(back, "!") {
			t.Errorf("%s came back as %q", served, back)
		}
	}
}

func TestDropped(t *testing.T) {
	const visible = `<minka-ref kind="card" id="01a0" title="Zelt" short-id="MNK-1" href="https://minka.example/go/card/MNK-1"></minka-ref>`
	const visibleWithoutShortID = `<minka-ref kind="post" id="01a1" title="P" href="https://minka.example/projects/p/posts/t/01a1"></minka-ref>`
	const hidden = `<minka-ref kind="card" id="01a2"></minka-ref>`
	const file = `<action-text-attachment sgid="x" content-type="application/pdf"></action-text-attachment>`
	const cardLink = `<a href="https://minka.example/go/card/MNK-1">https://minka.example/go/card/MNK-1</a>`
	tests := []struct {
		name                    string
		current, replacement    string
		attachments, references bool
	}{
		{"nothing to lose", "<p>a</p>", "<p>b</p>", false, false},
		{"reference kept as the element", "<p>" + visible + "</p>", `<p><minka-ref kind="card" id="01a0"></minka-ref> new</p>`, false, false},
		{"reference kept as a link showing its address", "<p>" + visible + "</p>", "<p>new " + cardLink + "</p>", false, false},
		{"reference kept with a trailing slash", "<p>" + visible + "</p>", `<p><a href="https://minka.example/go/card/MNK-1/">https://minka.example/go/card/MNK-1</a></p>`, false, false},
		{"reference kept as its short identifier", "<p>" + visible + "</p>", "<p>see #MNK-1.</p>", false, false},
		{"reference kept as its address in the words", "<p>" + visible + "</p>", "<p>see https://minka.example/go/card/MNK-1.</p>", false, false},
		{"reference kept as its address in plain text", "<p>" + visible + "</p>", "see (https://minka.example/go/card/MNK-1/)", false, false},
		{"reference lost", "<p>" + visible + "</p>", "<p>gone</p>", false, true},
		{"reference lost to a longer address in the words", "<p>" + visible + "</p>", "<p>https://minka.example/go/card/MNK-12</p>", false, true},
		{"reference lost to a short identifier glued to an address", "<p>" + visible + "</p>", "<p>https://foreign.example/#MNK-1 and https://foreign.example/?q=#MNK-1</p>", false, true},
		{"reference lost to a link with a short identifier as its words", "<p>" + visible + "</p>", `<p><a href="https://elsewhere.example">#MNK-1</a></p>`, false, true},
		{"reference lost to a link with the address as its words", "<p>" + visible + "</p>", `<p><a href="https://elsewhere.example">https://minka.example/go/card/MNK-1</a></p>`, false, true},
		{"reference lost to text no browser shows", "<p>" + visible + "</p>", "<p>x</p><script>#MNK-1</script><style>#MNK-1 {}</style><textarea>#MNK-1</textarea>", false, true},
		{"reference lost to an address with two slashes at its end", "<p>" + visible + "</p>", "<p>https://minka.example/go/card/MNK-1//</p>", false, true},
		{"reference lost to a link with words of its own", "<p>" + visible + "</p>", `<p><a href="https://minka.example/go/card/MNK-1">the card</a></p>`, false, true},
		{"reference lost to code", "<p>" + visible + "</p>", "<p><code>#MNK-1</code> <code>https://minka.example/go/card/MNK-1</code></p>", false, true},
		{"reference lost to a short identifier something touches", "<p>" + visible + "</p>", "<p>#MNK-12 x#MNK-1 #MNK-1-2 ##MNK-1x</p>", false, true},
		{"reference lost to another address", "<p>" + visible + "</p>", `<p><a href="https://minka.example/go/card/MNK-2">https://minka.example/go/card/MNK-2</a></p>`, false, true},
		{"reference without short identifier kept by its address", "<p>" + visibleWithoutShortID + "</p>", `<p><a href="https://minka.example/projects/p/posts/t/01a1">https://minka.example/projects/p/posts/t/01a1</a></p>`, false, false},
		{"hidden reference lost to its words", "<p>" + hidden + "</p>", "<p>(a card you cannot open)</p>", false, true},
		{"hidden reference kept as the element", "<p>" + hidden + "</p>", "<p>" + hidden + " new</p>", false, false},
		{"one of two references lost", "<p>" + visible + " " + visibleWithoutShortID + "</p>", "<p>" + cardLink + "</p>", false, true},
		{"file lost", "<p>" + file + "</p>", "<p>x</p>", true, false},
		{"file lost although a reference stays", "<p>" + file + visible + "</p>", "<p>" + cardLink + "</p>", true, false},
		{"file kept, reference lost", "<p>" + file + visible + "</p>", "<p>" + file + "</p>", false, true},
		{"both lost", "<p>" + file + visible + "</p>", "<p>x</p>", true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Dropped(test.current, test.replacement)
			if got.Attachments != test.attachments || got.References != test.references {
				t.Errorf("got %+v, want attachments %v, references %v", got, test.attachments, test.references)
			}
		})
	}
}

func TestCarriesMarkup(t *testing.T) {
	for value, want := range map[string]bool{
		"<p>text</p>":                true,
		"<div>a<br>b</div>":          true,
		"text with <br/> in it":      true,
		"plain text":                 false,
		"a < b and <notanelement> x": false,
		"<pre>":                      true,
		"<abbr>x</abbr>":             false,
		"wenn a<i und b<s dann":      false,
		"<p class=\"x\">text</p>":    true,
		"text</strong >":             true,
		"<p title=\"never closed":    false,
		"<I>shouting</I>":            true,
		`<action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment>`: true,
		// The server keeps these two as well, so a text that holds nothing
		// else is markup and is not escaped into characters.
		`<minka-embed url="https://example.com/x"></minka-embed>`: true,
		`<minka-ref kind="card" id="0199"></minka-ref>`:           true,
	} {
		if got := CarriesMarkup(value); got != want {
			t.Errorf("CarriesMarkup(%q) = %v", value, got)
		}
	}
}

func TestHasAttachments(t *testing.T) {
	for served, want := range map[string]bool{
		"":            false,
		"<p>text</p>": false,
		`<figure class="lexxy-content__table-wrapper"><table><tbody><tr><td>x</td></tr></tbody></table></figure>`: false,
		`<figure class="attachment attachment--image"><img src="/f/1" alt="Plan"></figure>`:                       true,
		`<figure class="attachment attachment--file"><a href="/f/2" download="plan.pdf">plan.pdf</a></figure>`:    true,
		`<p>see <img src="/f/3"></p>`:                                true,
		`<p><a href="/f/4" download="plan.pdf">plan.pdf</a></p>`:     true,
		`<action-text-attachment sgid="x"></action-text-attachment>`: true,
		`<p><a href="https://example.com">link</a></p>`:              false,
		// An embed and a reference are their attributes and nothing else, so
		// a text sent back without them loses what they stood for.
		`<p>see <minka-embed url="https://example.com/x"></minka-embed></p>`: true,
		`<p><minka-ref kind="card" id="0199"></minka-ref></p>`:               true,
	} {
		if got := HasAttachments(served); got != want {
			t.Errorf("HasAttachments(%q) = %v", served, got)
		}
	}
}

// Plain text is kept as the characters it consists of, so an address in angle
// brackets is an address in the words. A text with a tag of a served element
// in it the server takes as HTML, code and links and elements included.
func TestDroppedByText(t *testing.T) {
	const visible = `<minka-ref kind="card" id="01a0" title="Zelt" short-id="MNK-1" href="https://minka.example/go/card/MNK-1"></minka-ref>`
	const hidden = `<minka-ref kind="card" id="01a2"></minka-ref>`
	for _, test := range []struct {
		current, text string
		lost          bool
	}{
		{"<p>" + visible + "</p>", "See <https://minka.example/go/card/MNK-1> now", false},
		{"<p>" + visible + "</p>", "See #MNK-1 now", false},
		{"<p>" + visible + "</p>", "See https://foreign.example/#MNK-1", true},
		{"<p>" + visible + "</p>", "See <code>#MNK-1</code>", true},
		{"<p>" + visible + "</p>", `<a href="https://x.example">https://minka.example/go/card/MNK-1</a>`, true},
		{"<p>" + visible + "</p>", "<pre>#MNK-1</pre>", true},
		{"<p>" + visible + "</p>", `See <a href="https://minka.example/go/card/MNK-1">https://minka.example/go/card/MNK-1</a>`, false},
		{"<p>" + hidden + "</p>", "See " + hidden + " now", false},
		{"<p>" + visible + "</p>", "a<i und b<s #MNK-1", false},
		{"<p>" + hidden + "</p>", "&lt;minka-ref kind=\"card\" id=\"01a2\">", true},
	} {
		if got := DroppedByText(test.current, test.text); got.References != test.lost {
			t.Errorf("%q: references lost %v, want %v", test.text, got.References, test.lost)
		}
	}
}
