package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func (a *app) newCommentsCmd() *cobra.Command {
	return group("comments", "Write comments on cards", a.newCommentsCreateCmd())
}

func (a *app) newCommentsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Write a comment on a card",
		Long: `Writes a comment on a card. Minka keeps comments as plain text, so the text is
sent as it is: Markdown stays readable but is not rendered. Give - as the text
to read it from stdin.`,
		Example: `  minka comments create 0199... "Done in abc1234"
  minka comments create 0199... - < summary.txt`,
		Args: requireArgs("cardId", "text"),
		RunE: func(_ *cobra.Command, args []string) error {
			text, err := a.readValue("the comment", args[1])
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.WriteComment(a.ctx, args[0], text)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:        comment.Raw,
				summary:     "Commented on card " + args[0],
				breadcrumbs: cardBreadcrumbs(args[0]),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Commented on card %s.\n", args[0])
				},
			})
		},
	}
}
