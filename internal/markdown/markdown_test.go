package markdown

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// everything uses every construct of Markdown the converter knows about.
const everything = "# Title\n\n" +
	"## Sub heading\n\n### Third level\n\n#### Fourth level\n\n##### Fifth\n\n###### Sixth\n\n" +
	"Some *emphasis*, **strong**, ***both***, ~~gone~~ and `inline code`.\n" +
	"A soft break  \nand a hard break.\n\n" +
	"> A quote\n> with two lines\n\n" +
	"- one\n- two\n  - nested\n\n" +
	"3. three\n4. four\n\n" +
	"```go\nfunc main() {\n\tprintln(\"<b>\")\n}\n```\n\n" +
	"```{weird}\nx\n```\n\n" +
	"    indented code\n\n" +
	"---\n\n" +
	"| Name | Value |\n| :--- | ---: |\n| a | **b** |\n\n" +
	"![alt text](https://example.com/image.png \"title\")\n\n" +
	"[link](https://example.com \"with title\") <https://example.com/auto> and www.example.com and mail@example.com\n\n" +
	"[bad](javascript:alert(1))\n\n" +
	"<div onclick=\"alert(1)\">raw block</div>\n\n" +
	"Inline <span style=\"color:red\">raw</span> html.\n\n" +
	"Umlaute äöü & <less> \"quotes\" \\*escaped\\*\n"

func TestOnlyElementsTheServerKeepsAreWritten(t *testing.T) {
	result := ToHTML(everything)

	tokenizer := html.NewTokenizer(strings.NewReader(result.HTML))
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken && tokenType != html.EndTagToken && tokenType != html.SelfClosingTagToken {
			if tokenType == html.CommentToken || tokenType == html.DoctypeToken {
				t.Errorf("unexpected %v in output", tokenType)
			}
			continue
		}
		token := tokenizer.Token()
		allowedAttributes, ok := AllowedElements[token.Data]
		if !ok {
			t.Errorf("element <%s> is not kept by the server:\n%s", token.Data, result.HTML)
			continue
		}
		for _, attr := range token.Attr {
			if !slices.Contains(allowedAttributes, attr.Key) {
				t.Errorf("attribute %s on <%s> is not kept by the server", attr.Key, token.Data)
			}
			if pattern, checked := AttributePatterns[token.Data+"."+attr.Key]; checked && !pattern.MatchString(attr.Val) {
				t.Errorf("attribute %s=%q on <%s> is removed by the server", attr.Key, attr.Val, token.Data)
			}
		}
	}
}

func TestConversions(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		html     string
		warnings []string
	}{
		{"empty", "  \n\n ", "", nil},
		{"paragraph", "Hello", "<p>Hello</p>", nil},
		{"headings of every level", "# 1\n\n## 2\n\n### 3\n\n#### 4\n\n##### 5\n\n###### 6", "<h1>1</h1>\n<h2>2</h2>\n<h3>3</h3>\n<h4>4</h4>\n<h5>5</h5>\n<h6>6</h6>", nil},
		{"emphasis", "*a* **b** ~~c~~", "<p><em>a</em> <strong>b</strong> <del>c</del></p>", nil},
		{"inline code", "run `make <x>`", "<p>run <code>make &lt;x&gt;</code></p>", nil},
		{"hard break", "a  \nb", "<p>a<br>b</p>", nil},
		{"code block with a language", "```Go\n<b>x</b>\n```", `<pre data-language="go"><code>&lt;b&gt;x&lt;/b&gt;</code></pre>`, nil},
		{"code block with a language the server does not keep", "```{weird}\nx\n```", "<pre><code>x</code></pre>", nil},
		{"indented code block", "    a < b", "<pre><code>a &lt; b</code></pre>", nil},
		{"tight list", "- a\n- b", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>", nil},
		{"ordered list starting elsewhere", "3. a", "<ol start=\"3\">\n<li>a</li>\n</ol>", nil},
		{"ordered list starting at 1", "1. a", "<ol>\n<li>a</li>\n</ol>", nil},
		{"quote", "> q", "<blockquote>\n<p>q</p>\n</blockquote>", nil},
		{"link with a title", `[x](https://example.com "t & u")`, `<p><a href="https://example.com" title="t &amp; u">x</a></p>`, nil},
		{"autolink email", "<mail@example.com>", `<p><a href="mailto:mail@example.com">mail@example.com</a></p>`, nil},
		{"unsafe link", "[x](javascript:alert(1))", "<p>x</p>", []string{WarningUnsafeLinks}},
		{"ftp link", "[x](ftp://a/b)", "<p>x</p>", []string{WarningUnsafeLinks}},
		{"protocol-relative link", "[x](//evil.example)", "<p>x</p>", []string{WarningUnsafeLinks}},
		{"empty link", "[x]()", "<p>x</p>", []string{WarningUnsafeLinks}},
		{"relative links", "[a](docs/a) [b](/b) [c](#c) [d](mailto:d@example.com) [e](tel:+49)", `<p><a href="docs/a">a</a> <a href="/b">b</a> <a href="#c">c</a> <a href="mailto:d@example.com">d</a> <a href="tel:+49">e</a></p>`, nil},
		{"data image", "![x](data:image/png;base64,AAAA)", "<p>x</p>", []string{WarningImages, WarningUnsafeLinks}},
		{"image inside a link", "[![alt](https://i.example/p.png)](https://example.com)", `<p><a href="https://example.com">alt</a></p>`, []string{WarningImages}},
		{"link reference definition only", "[x]: https://example.com", "", nil},
		{"image", "![alt](https://example.com/i.png)", `<p><a href="https://example.com/i.png">alt</a></p>`, []string{WarningImages}},
		{"rule", "a\n\n---\n\nb", "<p>a</p>\n<hr>\n<p>b</p>", nil},
		{"inline html is text", "a <b>b</b>", "<p>a &lt;b&gt;b&lt;/b&gt;</p>", []string{WarningRawHTML}},
		{"html block is text", "<div>x</div>", "<pre>&lt;div&gt;x&lt;/div&gt;</pre>", []string{WarningRawHTML}},
		{"entities and escapes", `AT&T \*x\* &amp;`, "<p>AT&amp;T *x* &amp;</p>", nil},
		{"umlauts", "äöü ß", "<p>äöü ß</p>", nil},
		{
			"table as the editor writes it",
			"| a | *b* |\n|---|:--:|\n| ccc | d |",
			`<figure class="lexxy-content__table-wrapper"><table><tbody>` + "\n" +
				`<tr><th class="lexxy-content__table-cell--header">a</th><th class="lexxy-content__table-cell--header"><em>b</em></th></tr>` + "\n" +
				`<tr><td>ccc</td><td>d</td></tr>` + "\n" +
				`</tbody></table></figure>`,
			nil,
		},
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

func TestEachWarningIsGivenOnce(t *testing.T) {
	result := ToHTML("![a](https://example.com/a.png) ![b](https://example.com/b.png)")
	if !slices.Equal(result.Warnings, []string{WarningImages}) {
		t.Errorf("got %q", result.Warnings)
	}
}
