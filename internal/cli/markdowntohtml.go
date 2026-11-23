package cli

import (
	"fmt"

	"github.com/flownative/minka-cli/internal/markdown"
)

// decisionsMarkerHint says how a decisions block is written and where its
// identifiers come from, for a marker refused here and a block Minka refuses.
const decisionsMarkerHint = `A decisions block is a line of its own: ` + markdown.DecisionsMarkerForm + `, "tool" required and every filter optional. The identifiers come from: minka decisions tools <projectId> (tool), minka decisions statuses <toolId> (status), minka labels list --project <projectId> (label of the project; a label of the Decisions tool counts too), minka events tools <projectId> and minka events list <toolId> --from <day> --to <day> (decided-in, an event; applies-to, an event or a card), minka cards show <cardId> (applies-to, a card by its ID).`

// readOutRowsHint answers a text that took in the rows of a reading view.
const readOutRowsHint = "The rows of a decisions block are read out by docs show, posts show and docs export, never written: Minka lists them afresh. Keep the block's marker line and take the quote out. docs show --body and posts show --body print the body alone, as it is sent back."

// markdownToHTML converts a text given as Markdown, what naming it and
// formatFlag the flag that would send it as HTML. A text that is mostly HTML
// was written in HTML and is refused: converted, it would arrive as its own
// source code, and the notice saying so is easily cut off.
func markdownToHTML(text, what, formatFlag string, options ...markdown.Option) (markdown.Result, error) {
	result := markdown.ToHTML(text, options...)
	if result.Refusal != "" {
		hint := decisionsMarkerHint
		if result.RefusedReadOutRows {
			hint = readOutRowsHint
		}
		return markdown.Result{}, &Error{Code: CodeUsage, Message: "nothing was sent: " + result.Refusal, Hint: hint}
	}
	if result.MostlyHTML {
		return markdown.Result{}, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("nothing was sent: the text is mostly HTML but is read as Markdown, so %s would arrive as visible source code", what),
			Hint:    fmt.Sprintf("To send HTML, give %s html. HTML meant to be shown as code belongs in a fenced code block between lines of three backticks (```).", formatFlag),
		}
	}
	return result, nil
}
