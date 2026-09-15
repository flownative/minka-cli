package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/spf13/cobra"
)

func (a *app) newCommentsCmd() *cobra.Command {
	return group("comments", "Write comments on cards", a.newCommentsCreateCmd())
}

func (a *app) newCommentsCreateCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Write a comment on a card",
		Long: `Writes a comment on a card. The text is Markdown by default and is sent as
HTML, the way a description is, except that every line break is kept: plain
text arrives as it was typed. --format html sends HTML as it is; the server
reduces it to what it keeps. Give - as the text to read it from stdin.`,
		Example: `  minka comments create 0199... "Done in abc1234"
  minka comments create 0199... - < summary.md`,
		Args: requireArgs("cardId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "markdown" && format != "html" {
				return usageErrorf(cmd, "--format is markdown or html, not %q", format)
			}
			text, err := a.readValue("the comment", args[1])
			if err != nil {
				return err
			}

			body := text
			var notices []string
			if format == "markdown" {
				result := markdown.ToHTML(text, markdown.KeepLineBreaks())
				if result.HTML == "" && strings.TrimSpace(text) != "" {
					return &Error{Code: CodeUsage, Message: "the comment has nothing Minka keeps"}
				}
				body, notices = result.HTML, result.Warnings
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.WriteComment(a.ctx, args[0], body)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        comment.Raw,
				summary:     "Commented on card " + args[0],
				notices:     notices,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Commented on card %s.\n", args[0])
				},
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")

	return cmd
}
