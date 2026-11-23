package richtext

import (
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
)

const (
	blockTool   = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"
	blockStatus = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a6c"
	blockEvent  = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a8e"
)

func pointer[T any](value T) *T {
	return &value
}

// A block reads as its marker, its attributes in the order the way back
// writes them, so reading, editing and writing back keeps the element.
func TestADecisionsBlockComesBackAsTheSameElement(t *testing.T) {
	stored := `<p>It holds:</p><minka-decisions applies-to="` + blockEvent + `" tool="` + blockTool + `" status="` + blockStatus + `"></minka-decisions><p>More</p>`
	canonical := `<minka-decisions tool="` + blockTool + `" status="` + blockStatus + `" applies-to="` + blockEvent + `"></minka-decisions>`

	read := ToMarkdown(stored)
	want := "It holds:\n\n<!-- minka:decisions tool=\"" + blockTool + "\" status=\"" + blockStatus + "\" applies-to=\"" + blockEvent + "\" -->\n\nMore"
	if read != want {
		t.Fatalf("Markdown\n%s\nwant\n%s", read, want)
	}
	back := markdown.ToHTML(read)
	if back.HTML != "<p>It holds:</p>\n"+canonical+"\n<p>More</p>" || back.Refusal != "" || len(back.Warnings) > 0 {
		t.Fatalf("back %q, refusal %q, warnings %v", back.HTML, back.Refusal, back.Warnings)
	}
	if again := ToMarkdown(back.HTML); again != read {
		t.Fatalf("read again\n%s\nwant\n%s", again, read)
	}
	if !CarriesMarkup(canonical) {
		t.Error("a text of a block alone counts as plain text, which Minka takes as HTML")
	}
}

// An export prints each block's rows below its marker, as a quote marked as
// read out, which a text sent back may not hold.
func TestAnExportPrintsTheRowsBelowTheMarker(t *testing.T) {
	stored := `<minka-decisions tool="` + blockTool + `"></minka-decisions><p>Between</p><minka-decisions tool="` + blockTool + `" status="` + blockStatus + `"></minka-decisions>`
	blocks := []api.DecisionBlock{
		{
			Position: 0, ToolID: blockTool, Tool: &api.DecisionsToolReference{ID: blockTool, Title: "Decisions"}, Summary: "Decisions",
			Decisions: []api.DecisionBlockEntry{
				{ShortID: pointer("DEC-2"), Title: "We meet *on* Mondays", Status: api.DecisionStatus{Name: "In force"}, VotesFor: pointer(5), VotesNote: pointer("unanimous")},
				{Title: "Unnumbered", Status: api.DecisionStatus{Name: "Proposed"}},
			},
		},
		{Position: 1, ToolID: blockTool, Tool: &api.DecisionsToolReference{ID: blockTool, Title: "Decisions"}, Summary: "Decisions · status In force"},
	}

	read := ToMarkdownWithDecisions(stored, blocks)
	want := `<!-- minka:decisions tool="` + blockTool + `" -->` + "\n" +
		"> " + markdown.ReadOutLead + " Decisions\n" +
		"> - DEC-2 We meet \\*on\\* Mondays — In force · 5 for, 0 against, 0 abstaining · unanimous\n" +
		"> - Unnumbered — Proposed\n\n" +
		"Between\n\n" +
		`<!-- minka:decisions tool="` + blockTool + `" status="` + blockStatus + `" -->` + "\n" +
		"> " + markdown.ReadOutLead + " Decisions · status In force\n" +
		"> - (no decisions)"
	if read != want {
		t.Fatalf("view\n%s\nwant\n%s", read, want)
	}

	if back := markdown.ToHTML(read); !back.RefusedReadOutRows {
		t.Fatalf("the rows of an export sent back are not refused: %q", back.HTML)
	}
}

// Rows go only where the element is the block they were read for.
func TestRowsOfAnotherBlockAreNotShown(t *testing.T) {
	stored := `<minka-decisions tool="` + blockTool + `"></minka-decisions>`
	other := []api.DecisionBlock{{Position: 0, ToolID: blockStatus, Tool: &api.DecisionsToolReference{ID: blockStatus}, Summary: "Other"}}
	if read := ToMarkdownWithDecisions(stored, other); strings.Contains(read, markdown.ReadOutLead) {
		t.Fatalf("view %q shows the rows of another block", read)
	}
}

func TestTheRowsSayWhatTheReaderCannotSeeAndWhatChangedSinceTheLock(t *testing.T) {
	unseen := DecisionRows(api.DecisionBlock{ToolID: blockTool, Summary: "List of decisions"})
	if unseen != "> "+markdown.ReadOutLead+" (a Decisions tool you cannot see)" {
		t.Errorf("unseen tool: %q", unseen)
	}

	frozen := DecisionRows(api.DecisionBlock{
		ToolID: blockTool, Tool: &api.DecisionsToolReference{ID: blockTool, Title: "Decisions"}, Summary: "Decisions", Frozen: true,
		Decisions: []api.DecisionBlockEntry{
			{ShortID: pointer("DEC-1"), Title: "Tent", Status: api.DecisionStatus{Name: "In force"}, Today: &api.DecisionBlockToday{SupersededBy: &api.DecisionBlockLater{ShortID: pointer("DEC-9"), Title: "Bigger tent"}, Reworded: true}},
			{ShortID: pointer("DEC-2"), Title: "Music", Status: api.DecisionStatus{Name: "Proposed"}, Today: &api.DecisionBlockToday{Status: &api.DecisionStatus{Name: "Rejected"}}},
			{ShortID: pointer("DEC-3"), Title: "Fee", Status: api.DecisionStatus{Name: "In force"}, Today: &api.DecisionBlockToday{}},
		},
	})
	want := "> " + markdown.ReadOutLead + " Decisions (as locked)\n" +
		"> - DEC-1 Tent — In force · today: superseded by DEC-9 · today: reworded\n" +
		"> - DEC-2 Music — Proposed · today: Rejected\n" +
		"> - DEC-3 Fee — In force"
	if frozen != want {
		t.Errorf("frozen\n%s\nwant\n%s", frozen, want)
	}
}
