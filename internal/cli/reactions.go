package cli

import (
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

const reactionHelp = `A reaction is short: an emoji, or at most 20 characters of text. Reacting the
same way twice changes nothing and succeeds.

Taking one away is not bound to whoever left it: unreact takes away your own
reaction of that text, and where you left none, the one somebody else left
last. Taking away what is not there succeeds as well.`

// reactionError says which of the refusals of the server was met.
func reactionError(e *Error) *Error {
	switch e.APICode {
	case "reactionMissing":
		e.Hint = "A reaction needs something to say: an emoji or a short word."
	case "reactionTooLong":
		e.Hint = "A reaction is at most 20 characters."
	case "reactionHasControlCharacters":
		e.Hint = "A reaction is one line of text without control characters."
	}
	return e
}

func (a *app) newCommentsReactCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "react <cardId> <commentId> <content>",
		Short:   "React to a comment",
		Long:    "Leaves a reaction on a comment.\n\n" + reactionHelp,
		Example: `  minka comments react 0199... 0199... 👍`,
		Args:    requireArgs("cardId", "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			comment, err := s.client.ReactToComment(a.ctx, args[0], args[1], args[2])
			if err != nil {
				return reactionError(s.fail(err))
			}

			summary := fmt.Sprintf("Reacted %s to comment %s", args[2], args[1])
			if comment.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("You had reacted %s to comment %s already", args[2], args[1])
			}
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newCommentsUnreactCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "unreact <cardId> <commentId> <content>",
		Short:   "Take a reaction off a comment",
		Long:    "Takes a reaction off a comment.\n\n" + reactionHelp,
		Example: `  minka comments unreact 0199... 0199... 👍`,
		Args:    requireArgs("cardId", "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			if err := s.client.UnreactFromComment(a.ctx, args[0], args[1], args[2]); err != nil {
				return reactionError(s.fail(err))
			}

			summary := fmt.Sprintf("Took %s off comment %s", args[2], args[1])
			return a.ok(success{
				data:        map[string]any{"cardId": args[0], "commentId": args[1], "content": args[2], "removed": true},
				summary:     summary,
				breadcrumbs: cardBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newMessagesReactCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "react <chatId> <messageId> <content>",
		Short:   "React to a message",
		Long:    "Leaves a reaction on a message of a chat.\n\n" + reactionHelp,
		Example: `  minka messages react 0199... 0199... 👍`,
		Args:    requireArgs("chatId", "messageId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			message, err := s.client.ReactToMessage(a.ctx, args[0], args[1], args[2])
			if err != nil {
				return reactionError(s.fail(err))
			}

			summary := fmt.Sprintf("Reacted %s to message %s", args[2], args[1])
			if message.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("You had reacted %s to message %s already", args[2], args[1])
			}
			return a.ok(success{
				data:    message.Raw,
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newMessagesUnreactCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "unreact <chatId> <messageId> <content>",
		Short:   "Take a reaction off a message",
		Long:    "Takes a reaction off a message of a chat.\n\n" + reactionHelp,
		Example: `  minka messages unreact 0199... 0199... 👍`,
		Args:    requireArgs("chatId", "messageId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.UnreactFromMessage(a.ctx, args[0], args[1], args[2]); err != nil {
				return reactionError(s.fail(err))
			}

			summary := fmt.Sprintf("Took %s off message %s", args[2], args[1])
			return a.ok(success{
				data:    map[string]any{"chatId": args[0], "messageId": args[1], "content": args[2], "removed": true},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
