package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// showBodyHelp says what docs show and posts show print of a body and of its
// decisions blocks.
const showBodyHelp = `The body in the text view holds a decisions block as its marker line only.
Its rows follow in a section "Decisions blocks" after the body, one entry per
block in the order of the body, each named by its position and its marker's
attributes, as a quote that opens with "` + markdown.ReadOutLead + `". The rows
are read out, never written: a body sent back with such a quote is refused. In
JSON the rows are in decisionBlocks.

--body prints the body alone as Markdown, exactly what --body of the edit
takes back, and the updatedAt for --expect on standard error. It goes with
neither --json nor --quiet.`

// bodyAlone sets a command up to print a body as the only output, and refuses
// the flags that would put an envelope beside it.
func (a *app) bodyAlone(cmd *cobra.Command) error {
	if a.jsonFlag || a.quietFlag {
		return usageErrorf(cmd, "--body prints the body alone on standard output, where --json and --quiet put their own answer")
	}
	a.bytesAreTheOutput = true
	return nil
}

// writeBodyAlone prints the editable Markdown of a body on standard output
// and what an edit guards with on standard error.
func (a *app) writeBodyAlone(w io.Writer, body, updatedAt string) {
	if markdownBody := richtext.ToMarkdown(body); markdownBody != "" {
		fmt.Fprintln(w, markdownBody)
	}
	if updatedAt != "" {
		fmt.Fprintf(a.stderr, "To write over it: --expect %s\n", updatedAt)
	}
}

// renderDecisionBlocks prints the rows of a text's decisions blocks after
// the text, so that the body above holds only what can be sent back.
func renderDecisionBlocks(w io.Writer, blocks []api.DecisionBlock) {
	if len(blocks) == 0 {
		return
	}
	fmt.Fprintf(w, "\n## Decisions blocks (%d)\n", len(blocks))
	for _, block := range blocks {
		fmt.Fprintf(w, "\n### Position %d: %s\n\n%s\n", block.Position, decisionBlockAttributes(block), richtext.DecisionRows(block))
	}
}

// decisionBlockAttributes names a block as its marker does, so that a reader
// finds the marker it belongs to.
func decisionBlockAttributes(block api.DecisionBlock) string {
	values := map[string]*string{"tool": &block.ToolID, "label": block.LabelID, "status": block.StatusID, "decided-in": block.DecidedInID, "applies-to": block.AppliesToID}
	var attributes []string
	for _, name := range markdown.DecisionsAttributes {
		if value := values[name]; value != nil && *value != "" {
			attributes = append(attributes, name+`="`+*value+`"`)
		}
	}
	return strings.Join(attributes, " ")
}
