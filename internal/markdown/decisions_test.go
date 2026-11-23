package markdown

import (
	"slices"
	"strings"
	"testing"
)

const (
	blockTool   = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	blockStatus = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a6c"
	blockLabel  = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a7d"
)

// A decisions marker becomes the element Minka stores, its attributes in one
// order, so that a block read and written back stays the same element.
func TestADecisionsMarkerBecomesItsElement(t *testing.T) {
	tests := []struct {
		name, markdown, html string
	}{
		{"tool alone", `<!-- minka:decisions tool="` + blockTool + `" -->`, `<minka-decisions tool="` + blockTool + `"></minka-decisions>`},
		{
			"filters in the order of the element",
			`<!-- minka:decisions status="` + blockStatus + `" tool="` + blockTool + `" label="` + blockLabel + `" -->`,
			`<minka-decisions tool="` + blockTool + `" label="` + blockLabel + `" status="` + blockStatus + `"></minka-decisions>`,
		},
		{"an identifier in capitals", `<!-- minka:decisions tool="` + strings.ToUpper(blockTool) + `" -->`, `<minka-decisions tool="` + blockTool + `"></minka-decisions>`},
		{"an empty filter is none", `<!-- minka:decisions tool="` + blockTool + `" label="" -->`, `<minka-decisions tool="` + blockTool + `"></minka-decisions>`},
		{"other spacing", `<!--minka:decisions   tool="` + blockTool + `"-->`, `<minka-decisions tool="` + blockTool + `"></minka-decisions>`},
		{
			"between paragraphs",
			"Before\n\n<!-- minka:decisions tool=\"" + blockTool + "\" -->\n\nAfter",
			"<p>Before</p>\n<minka-decisions tool=\"" + blockTool + "\"></minka-decisions>\n<p>After</p>",
		},
		{
			"in a callout",
			"> [!NOTE]\n> <!-- minka:decisions tool=\"" + blockTool + "\" -->",
			"<aside data-callout=\"note\">\n<minka-decisions tool=\"" + blockTool + "\"></minka-decisions>\n</aside>",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ToHTML(test.markdown)
			if result.HTML != test.html {
				t.Errorf("HTML\n%s\nwant\n%s", result.HTML, test.html)
			}
			if result.Refusal != "" || len(result.Warnings) > 0 || result.MostlyHTML {
				t.Errorf("refusal %q, warnings %v, mostly HTML %v", result.Refusal, result.Warnings, result.MostlyHTML)
			}
		})
	}
}

// A marker that cannot be a block is refused instead of reaching the text as
// visible source or as a block Minka refuses.
func TestAMalformedDecisionsMarkerIsRefused(t *testing.T) {
	tests := []struct {
		name, markdown, refusal string
	}{
		{"no tool", `<!-- minka:decisions label="` + blockLabel + `" -->`, `"tool"`},
		{"no attributes", `<!-- minka:decisions -->`, `"tool"`},
		{"an unknown attribute", `<!-- minka:decisions tool="` + blockTool + `" summary="` + blockLabel + `" -->`, `no attribute "summary"`},
		{"single quotes", `<!-- minka:decisions tool='` + blockTool + `' -->`, "double quotes"},
		{"no quotes", `<!-- minka:decisions tool=` + blockTool + ` -->`, "double quotes"},
		{"a value that is no identifier", `<!-- minka:decisions tool="` + blockTool + `" status="holds" -->`, `"status"`},
		{"text after the marker", `<!-- minka:decisions tool="` + blockTool + `" --> and more`, `nothing may follow its -->`},
		{"an attribute in capitals", `<!-- minka:decisions TOOL="` + blockTool + `" -->`, `lower case: "tool"`},
		{"an attribute twice", `<!-- minka:decisions tool="` + blockTool + `" tool="` + blockTool + `" -->`, "twice"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ToHTML("Before\n\n" + test.markdown)
			if !strings.Contains(result.Refusal, test.refusal) {
				t.Fatalf("refusal %q, want one naming %s", result.Refusal, test.refusal)
			}
		})
	}
}

// The rows a reading view prints are never part of a text: one that holds
// them can only have copied them, and is refused wherever they stand.
func TestATextHoldingReadOutRowsIsRefused(t *testing.T) {
	marker := `<!-- minka:decisions tool="` + blockTool + `" -->`
	for name, source := range map[string]string{
		"right below the marker":   marker + "\n> " + ReadOutLead + " Decisions\n> - DEC-1 We meet — In force",
		"with a line joined to it": marker + "\n> " + ReadOutLead + " Decisions\nA line of one's own",
		"after a paragraph":        marker + "\n\nWords\n\n> " + ReadOutLead + " Decisions",
		"without the marker":       "Words\n\n> " + ReadOutLead + " Decisions",
		"in a list":                "- item\n\n  > " + ReadOutLead + " Decisions",
	} {
		result := ToHTML(source)
		if result.Refusal != RefusalReadOutRows || !result.RefusedReadOutRows {
			t.Errorf("%s: refusal %q", name, result.Refusal)
		}
	}
	if result := ToHTML(marker + "\n> A quote of one's own"); result.Refusal != "" || !strings.Contains(result.HTML, "<blockquote>") {
		t.Errorf("a quote of one's own: %q, refusal %q", result.HTML, result.Refusal)
	}
}

// A marker inside a paragraph is no block of its own and stays visible text,
// as any other HTML there.
func TestADecisionsMarkerInsideAParagraphIsText(t *testing.T) {
	result := ToHTML(`Words <!-- minka:decisions tool="` + blockTool + `" --> more`)
	if strings.Contains(result.HTML, "<minka-decisions") || !slices.Contains(result.Warnings, WarningRawHTML) {
		t.Fatalf("HTML %q, warnings %v", result.HTML, result.Warnings)
	}
}
