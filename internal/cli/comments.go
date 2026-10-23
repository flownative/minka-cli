package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

const commentFormatHelp = `The text is Markdown by default and is sent as HTML, the way a description is,
except that every line break is kept: plain text arrives as it was typed.
--format html sends HTML as it is; the server reduces it to what it keeps. A
text that is mostly HTML is refused without it. Give - as the text to read it
from stdin.`

const commentAuthorHint = "It does not exist, was removed, or somebody else wrote it: only the author can change or remove a comment."

func (a *app) newCommentsCmd() *cobra.Command {
	return group("comments", "Read, write, change, remove and react to comments on cards",
		a.newCommentsListCmd(),
		a.newCommentsUnreadCmd(),
		a.newCommentsCreateCmd(),
		a.newCommentsEditCmd(),
		a.newCommentsDeleteCmd(),
		a.newCommentsReactCmd(),
		a.newCommentsUnreactCmd(),
	)
}

// commentBody turns the text of a comment into what is sent.
func commentBody(cmd *cobra.Command, text, format string) (string, []string, error) {
	switch format {
	case "html":
		return text, markupNotice(text), nil
	case "markdown":
		result, err := markdownToHTML(text, "the comment", "--format", markdown.KeepLineBreaks())
		if err != nil {
			return "", nil, err
		}
		if result.HTML == "" && strings.TrimSpace(text) != "" {
			return "", nil, &Error{Code: CodeUsage, Message: "the comment has nothing Minka keeps"}
		}
		return result.HTML, result.Warnings, nil
	default:
		return "", nil, usageErrorf(cmd, "--format is markdown or html, not %q", format)
	}
}

// markupNotice says what becomes of a text the server finds no markup in: it
// is kept as the characters it consists of, escaped, and the answer says
// nothing about it. Markdown sent as HTML therefore stands there afterwards as
// the characters somebody typed.
func markupNotice(text string) []string {
	if strings.TrimSpace(text) == "" || richtext.CarriesMarkup(text) {
		return nil
	}
	return []string{"the text carries no element Minka serves, so it is kept as the characters it consists of, not as markup"}
}

func (a *app) newCommentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <cardId>",
		Short: "List the comments on a card, the oldest first",
		Long: `Lists every comment on a card, the oldest first, with its ID and reactions.
Nothing is paged. cards show carries the same comments together with the card.

Both note the comments as read on this machine, for comments unread and for
comments create.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, card, raw, err := a.readCardComments(&args[0])
			if err != nil {
				return err
			}
			return a.ok(success{
				data:        raw,
				summary:     plural(len(card.Comments), "comment", "comments") + " on card " + card.ID,
				notices:     a.rememberShown(s, card),
				breadcrumbs: commentBreadcrumbs(card),
				text:        func(w io.Writer) { renderCommentsOrNone(w, card.Comments, "No comments.") },
			})
		},
	}
}

func (a *app) newCommentsUnreadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unread <cardId>",
		Short: "List the comments on a card not yet shown on this machine",
		Long: `Lists the comments on a card that this machine has not shown yet to whoever
the token signs in as, the oldest first, and notes them as read.

cards show, comments list and comments unread note every comment they show.
A comment written here counts as read as well. A changed comment does not
count as new: it keeps its place and its ID. The marks lie beside the
configuration file and are forgotten for a card not read for 90 days.`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, card, raw, err := a.readCardComments(&args[0])
			if err != nil {
				return err
			}
			known, notices := a.knownComments(s, card.ID)
			unread, served := someOf(card, raw, func(c api.Comment) bool { return !known[c.ID] })
			notices = append(notices, a.rememberShown(s, card)...)

			return a.ok(success{
				data:        served,
				summary:     plural(len(unread), "unread comment", "unread comments") + " on card " + card.ID,
				notices:     notices,
				breadcrumbs: commentBreadcrumbs(card),
				text:        func(w io.Writer) { renderCommentsOrNone(w, unread, "No unread comments.") },
			})
		},
	}
}

// readCardComments reads a card together with its comments as they were
// served. Minka hands every comment of a card over with the card, oldest first
// and unpaged, so the last one is the newest there is.
func (a *app) readCardComments(cardID *string) (*session, api.Card, []json.RawMessage, error) {
	s, err := a.session()
	if err != nil {
		return nil, api.Card{}, nil, err
	}
	if err := s.resolveCard(a.ctx, cardID); err != nil {
		return nil, api.Card{}, nil, err
	}
	card, err := s.client.Card(a.ctx, *cardID)
	if err != nil {
		return nil, api.Card{}, nil, s.fail(err)
	}
	raw, err := commentsOfCard(card.Raw)
	if err != nil {
		return nil, api.Card{}, nil, &Error{Code: CodeAPI, Message: err.Error()}
	}
	return s, card.Value, raw, nil
}

func (a *app) newCommentsCreateCmd() *cobra.Command {
	var format, ifLatest string
	var force bool

	cmd := &cobra.Command{
		Use:   "create <cardId> <text|->",
		Short: "Write a comment on a card",
		Long: "Writes a comment on a card.\n\n" + commentFormatHelp + `

--if-latest <commentId> writes only while that comment is still the newest on
the card, and --if-latest none only while the card has none. Otherwise nothing
is written: the comments that came after it are shown and noted as read, and
the command exits with 11 (unread). A comment the card no longer has shows all
of them. The card is read right before the comment is sent, but a comment
written in that moment can still slip past.

With comments.requireRead switched on (minka config set comments.requireRead
true), a comment without --if-latest is refused the same way while the card
has comments not yet shown on this machine; --force writes it all the same.`,
		Example: `  minka comments create 0199... "Done in abc1234"
  minka comments create 0199... - < summary.md
  minka comments create 0199... "Agreed" --if-latest 0199...`,
		Args: requireArgs("cardId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			guarded := cmd.Flags().Changed("if-latest")
			if guarded {
				if force {
					return usageErrorf(cmd, "--force and --if-latest exclude each other: one writes whatever came in, the other only if nothing did")
				}
				if ifLatest != ifLatestNone {
					id, ok := asID(ifLatest)
					if !ok {
						return usageErrorf(cmd, "--if-latest takes the ID of a comment or none, not %q", ifLatest)
					}
					ifLatest = id
				}
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
			mustHaveRead := false
			if !guarded && !force {
				var settingNotices []string
				if mustHaveRead, settingNotices, err = a.requireRead(s); err != nil {
					return err
				}
				notices = append(notices, settingNotices...)
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}

			var vouchedFor []string
			if guarded || mustHaveRead {
				card, err := s.client.Card(a.ctx, args[0])
				if err != nil {
					return s.fail(err)
				}
				raw, err := commentsOfCard(card.Raw)
				if err != nil {
					return &Error{Code: CodeAPI, Message: err.Error()}
				}
				if guarded {
					if refusal := a.notTheNewest(s, card.Value, raw, ifLatest); refusal != nil {
						return refusal
					}
					// Whoever named the newest comment has read that one; what
					// stands before it may never have been shown to them.
					if ifLatest != ifLatestNone {
						vouchedFor = []string{ifLatest}
					}
				} else {
					known, unknownNotices := a.knownComments(s, card.Value.ID)
					if refusal := a.notRead(s, card.Value, raw, known, unknownNotices); refusal != nil {
						return refusal
					}
				}
			}

			comment, err := s.client.WriteComment(a.ctx, args[0], body)
			if err != nil {
				return s.fail(err)
			}
			notices = append(notices, a.rememberComments(s, args[0], append(vouchedFor, comment.Value.ID))...)

			return a.ok(success{
				data:        comment.Raw,
				summary:     "Commented on card " + args[0],
				notices:     notices,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text:        func(w io.Writer) { writtenComment(w, "Commented on card "+args[0], comment.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().StringVar(&ifLatest, "if-latest", "", "Write only while this comment ID is the newest on the card, or none while it has none")
	cmd.Flags().BoolVar(&force, "force", false, "Write even where comments.requireRead finds comments not yet read")

	return cmd
}

// notTheNewest refuses a comment whose writer named another comment as the
// newest than the card has. What came after the named one is shown and noted
// as read; where the card no longer has it, nothing can be told apart, so all
// of them are.
func (a *app) notTheNewest(s *session, card api.Card, raw []json.RawMessage, named string) *Error {
	newest := newestComment(card.Comments)
	if named == newest {
		return nil
	}
	at := -1
	for i, comment := range card.Comments {
		if comment.ID == named {
			at = i
		}
	}
	newer, served := someOf(card, raw, func(c api.Comment) bool {
		return at < 0 || indexOfComment(card.Comments, c.ID) > at
	})
	e := a.unreadRefusal(s, card, newer, served, nil)
	switch {
	case named == ifLatestNone:
		e.Message = fmt.Sprintf("card %s has %s; nothing was written", card.ID, plural(len(newer), "comment", "comments"))
	case at < 0 && len(newer) == 0:
		e.Message = fmt.Sprintf("comment %s is gone from card %s, which has no comments left; nothing was written", named, card.ID)
	case at < 0:
		e.Message = fmt.Sprintf("comment %s is gone from card %s, so every comment counts as new; nothing was written", named, card.ID)
	default:
		e.Message = fmt.Sprintf("%s came after %s on card %s; nothing was written", plural(len(newer), "comment", "comments"), named, card.ID)
	}
	return e
}

// notRead refuses a comment on a card that has comments this machine has not
// shown, and shows exactly those.
func (a *app) notRead(s *session, card api.Card, raw []json.RawMessage, known map[string]bool, notices []string) *Error {
	unread, served := someOf(card, raw, func(c api.Comment) bool { return !known[c.ID] })
	if len(unread) == 0 {
		return nil
	}
	e := a.unreadRefusal(s, card, unread, served, notices)
	e.Message = fmt.Sprintf("card %s has %s not read here; nothing was written", card.ID, plural(len(unread), "comment", "comments"))
	return e
}

func (a *app) unreadRefusal(s *session, card api.Card, comments []api.Comment, served []json.RawMessage, notices []string) *Error {
	notices = append(notices, a.rememberComments(s, card.ID, commentIDs(comments))...)
	return &Error{
		Code:    CodeUnread,
		Hint:    fmt.Sprintf("Read them; they now count as read. Then comment again, guarded by the newest: --if-latest %s", newestComment(card.Comments)),
		Data:    served,
		Notices: notices,
		Text:    func(w io.Writer) { renderEntryComments(w, comments) },
	}
}

func indexOfComment(comments []api.Comment, id string) int {
	for i, comment := range comments {
		if comment.ID == id {
			return i
		}
	}
	return -1
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
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			card, err := s.client.Card(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}
			// A comment under a card of the archive takes no text at all.
			dropped, err := a.keepsTheAttachmentsOfAComment(s, card.Value.ProjectID, closedIf(card.Value.Archived), card.Value.Comments, args[1], body, dropAttachments,
				"Change the bodySource of the comment from cards show --json and send it back with --format html, or give --drop-attachments.")
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)

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
				text:        func(w io.Writer) { writtenComment(w, summary, comment.Value) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments, mentions or references, which drops them")

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
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
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
