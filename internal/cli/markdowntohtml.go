package cli

import (
	"fmt"

	"github.com/flownative/minka-cli/internal/markdown"
)

// markdownToHTML converts a text given as Markdown, what naming it and
// formatFlag the flag that would send it as HTML. A text that is mostly HTML
// was written in HTML and is refused: converted, it would arrive as its own
// source code, and the notice saying so is easily cut off.
func markdownToHTML(text, what, formatFlag string, options ...markdown.Option) (markdown.Result, error) {
	result := markdown.ToHTML(text, options...)
	if result.MostlyHTML {
		return markdown.Result{}, &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("nothing was sent: the text is mostly HTML but is read as Markdown, so %s would arrive as visible source code", what),
			Hint:    fmt.Sprintf("To send HTML, give %s html. HTML meant to be shown as code belongs in a fenced code block between lines of three backticks (```).", formatFlag),
		}
	}
	return result, nil
}
