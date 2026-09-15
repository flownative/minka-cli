package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

const commentFormatHelp = `The text is Markdown by default and is sent as HTML, the way a description is,
except that every line break is kept: plain text arrives as it was typed.
--format html sends HTML as it is; the server reduces it to what it keeps. Give
- as the text to read it from stdin.`

const commentAuthorHint = "It does not exist, was removed, or somebody else wrote it: only the author can change or remove a comment."

func (a *app) newCommentsCmd() *cobra.Command {
	return group("comments", "Write, change and remove comments on cards",
		a.newCommentsCreateCmd(),
		a.newCommentsEditCmd(),
		a.newCommentsDeleteCmd(),
	)
}

// commentBody turns the text of a comment into what is sent.
func commentBody(cmd *cobra.Command, text, format string) (string, []string, error) {
	switch format {
	case "html":
		return text, nil, nil
	case "markdown":
		result := markdown.ToHTML(text, markdown.KeepLineBreaks())
		if result.HTML == "" && strings.TrimSpace(text) != "" {
			return "", nil, &Error{Code: CodeUsage, Message: "the comment has nothing Minka keeps"}
		}
		return result.HTML, result.Warnings, nil
	default:
		return "", nil, usageErrorf(cmd, "--format is markdown or html, not %q", format)
	}
}

func (a *app) newCommentsCreateCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Write a comment on a card",
		Long:  "Writes a comment on a card.\n\n" + commentFormatHelp,
		Example: `  minka comments create 0199... "Done in abc1234"
  minka comments create 0199... - < summary.md`,
		Args: requireArgs("cardId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[1])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
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

func (a *app) newCommentsEditCmd() *cobra.Command {
	var format string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <cardId> <commentId> <text|->",
		Short: "Change a comment you wrote",
		Long: `Changes the text of a comment. Only its author can.

` + commentFormatHelp + `

Files and mentions stand in a comment as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource of the comment from
cards show --json, change it and send it back with --format html. A new text
that would drop them is refused unless --drop-attachments is given.`,
		Example: `  minka comments edit 0199... 0199... "Done in def5678"`,
		Args:    requireArgs("cardId", "commentId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[2])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			// Files and mentions stand in the text as elements Markdown cannot
			// hold, so only a text that carries them keeps them.
			for _, current := range card.Value.Comments {
				if current.ID != args[1] {
					continue
				}
				if richtext.HasAttachments(firstNonEmpty(current.BodySource, current.Body)) && !richtext.HasAttachments(body) {
					if !dropAttachments {
						return &Error{
							Code:    CodeUsage,
							Message: "the comment has attachments or mentions, and the new text would drop them",
							Hint:    "Change the bodySource of the comment from cards show --json and send it back with --format html, or give --drop-attachments.",
						}
					}
					notices = append(notices, "the attachments and mentions of the comment were dropped")
				}
			}

			comment, err := s.client.EditComment(a.ctx, args[0], args[1], body)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = commentAuthorHint
				}
				return e
			}

			summary := fmt.Sprintf("Changed comment %s on card %s", args[1], args[0])
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments or mentions, which drops them")

	return cmd
}

func (a *app) newCommentsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <cardId> <commentId>",
		Short: "Remove a comment you wrote",
		Long: `Removes a comment. Only its author can. A removed comment is gone, not marked:
removing it again exits with 2 (not_found), like a comment that never existed.`,
		Args: requireArgs("cardId", "commentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteComment(a.ctx, args[0], args[1]); err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = commentAuthorHint
				}
				return e
			}

			summary := fmt.Sprintf("Removed comment %s from card %s", args[1], args[0])
			return a.ok(success{
				data:        map[string]any{"cardId": args[0], "commentId": args[1], "deleted": true},
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
