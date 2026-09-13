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
