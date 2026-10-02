package cli

import (
	"fmt"
	"io"
	"net/http"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// commentedKind is one kind of item whose comments are read and written below
// the command of that kind, with the item named as the first argument: one
// kind of item hangs there, so no flag has to say which.
type commentedKind struct {
	parent string // the command the comments stand under, such as "posts"
	noun   string // the item as the help and the summaries name it
	arg    string // the name of the argument that takes its ID, which is also the key of the item in the answer of a removal
	kind   api.CommentedKind

	// read finds what a changed comment is held against: the project of the
	// item, whether it takes any text at all, and its comments.
	read func(a *app, s *session, id string) (commentedItem, error)
	// refusal classifies a failed request about the item.
	refusal func(s *session, err error, id string) *Error

	reactHelp string // what the help of react says beyond the reactions themselves
}

type commentedItem struct {
	projectID string
	closed    func() (bool, error)
	comments  []api.Comment
	// refusal classifies a failed write with what the read learnt, such as
	// the list a to-do stands in; nil leaves it to the kind.
	refusal func(s *session, err error) *Error
}

func (k commentedKind) item(id string) api.Commented {
	return api.Commented{Kind: k.kind, ID: id}
}

func (k commentedKind) example(rest string) string {
	return "  minka " + k.parent + " comments " + rest
}

// commentRefusal names the author on a 404, since only the author changes or
// removes a comment and Minka answers somebody else's as one that never was.
func (k commentedKind) commentRefusal(s *session, err error, id string) *Error {
	return k.namingTheAuthor(k.refusal(s, err, id))
}

func (k commentedKind) namingTheAuthor(e *Error) *Error {
	if e.Code == CodeNotFound {
		e.Hint = fmt.Sprintf("The %s does not exist or is not visible to you, the comment was removed or hangs under another %s, or somebody else wrote it: only the author can change or remove a comment.", k.noun, k.noun)
	}
	return e
}

// reactionRefusal is a refusal of a reaction, which anybody may leave: an
// address that has nothing is the item or the comment.
func (k commentedKind) reactionRefusal(s *session, err error, id string) *Error {
	e := k.refusal(s, err, id)
	if e.Code == CodeNotFound {
		e.Hint = fmt.Sprintf("The %s does not exist or is not visible to you, or the comment was removed or hangs under another %s. The comments of a %s: minka %s comments list <%s>", k.noun, k.noun, k.noun, k.parent, k.arg)
	}
	return reactionError(e)
}

func (k commentedKind) breadcrumbs(id string) []Breadcrumb {
	return []Breadcrumb{{Action: "show", Cmd: "minka " + k.parent + " show " + id, Description: "Read the " + k.noun + " with its comments"}}
}

func (a *app) newItemCommentsCmd(k commentedKind) *cobra.Command {
	return group("comments", "Read, write, change, remove and react to comments on a "+k.noun,
		a.newItemCommentsListCmd(k),
		a.newItemCommentsCreateCmd(k),
		a.newItemCommentsEditCmd(k),
		a.newItemCommentsDeleteCmd(k),
		a.newItemCommentsReactCmd(k),
		a.newItemCommentsUnreactCmd(k),
	)
}

func (a *app) newItemCommentsListCmd(k commentedKind) *cobra.Command {
	return &cobra.Command{
		Use:   "list <" + k.arg + ">",
		Short: "List the comments on a " + k.noun + ", the oldest first",
		Long: fmt.Sprintf(`Lists every comment on a %s, the oldest first, with its reactions.
Nothing is paged. %s show carries the same comments together with the %s.`, k.noun, k.parent, k.noun),
		Args: requireArgs(k.arg),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			comments, err := s.client.CommentsOn(a.ctx, k.item(args[0]))
			if err != nil {
				return k.refusal(s, err, args[0])
			}
			return a.ok(success{
				data:        comments.Raw,
				summary:     plural(len(comments.Value), "comment", "comments"),
				breadcrumbs: k.breadcrumbs(args[0]),
				text: func(w io.Writer) {
					if len(comments.Value) == 0 {
						fmt.Fprintln(w, "No comments.")
						return
					}
					renderEntryComments(w, comments.Value)
				},
			})
		},
	}
}

func (a *app) newItemCommentsCreateCmd(k commentedKind) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "create <" + k.arg + "> <text|->",
		Short: "Comment on a " + k.noun,
		Long:  "Writes a comment on a " + k.noun + ".\n\n" + commentFormatHelp,
		Example: k.example(`create 0199... "Looks right to me"`) + "\n" +
			k.example("create 0199... - < comment.md"),
		Args: requireArgs(k.arg, "text"),
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
			comment, err := s.client.WriteCommentOn(a.ctx, k.item(args[0]), body)
			if err != nil {
				return k.refusal(s, err, args[0])
			}

			summary := "Commented on " + k.noun + " " + args[0]
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: k.breadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")

	return cmd
}

func (a *app) newItemCommentsEditCmd(k commentedKind) *cobra.Command {
	var format string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <" + k.arg + "> <commentId> <text|->",
		Short: "Change a comment you wrote on a " + k.noun,
		Long: "Changes the text of a comment. Only its author can.\n\n" + commentFormatHelp + fmt.Sprintf(`

Files and mentions stand in a comment as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource of the comment from
%s show --json, change it and send it back with --format html. A new text
that would drop them from a comment of yours is refused unless
--drop-attachments is given; the command reads the %s first to know, and
where the comment carries files or mentions, the person of the token to know
whose it is. Where it is about to refuse, it reads the archived projects as
well: in an archived project Minka refuses any text, so the request goes out
and projectArchived is the answer.`, k.parent, k.noun),
		Example: k.example(`edit 0199... 0199... "Looks right after all"`),
		Args:    requireArgs(k.arg, "commentId", "text"),
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
			under, err := k.read(a, s, args[0])
			if err != nil {
				return k.refusal(s, err, args[0])
			}
			dropped, err := a.keepsTheAttachmentsOfAComment(s, under.projectID, under.closed, under.comments, args[1], body, dropAttachments,
				"Change the bodySource of the comment from "+k.parent+" show --json and send it back with --format html, or give --drop-attachments.")
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)

			comment, err := s.client.EditCommentOn(a.ctx, k.item(args[0]), args[1], body)
			if err != nil && under.refusal != nil {
				return k.namingTheAuthor(under.refusal(s, err))
			}
			if err != nil {
				return k.commentRefusal(s, err, args[0])
			}

			summary := fmt.Sprintf("Changed comment %s on %s %s", args[1], k.noun, args[0])
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: k.breadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments, mentions or references, which drops them")

	return cmd
}

func (a *app) newItemCommentsDeleteCmd(k commentedKind) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <" + k.arg + "> <commentId>",
		Short: "Remove a comment you wrote on a " + k.noun,
		Long: `Removes a comment. Only its author can. A removed comment is gone, not marked:
removing it again exits with 2 (not_found), like a comment that never existed.`,
		Args: requireArgs(k.arg, "commentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteCommentOn(a.ctx, k.item(args[0]), args[1]); err != nil {
				return k.commentRefusal(s, err, args[0])
			}

			summary := fmt.Sprintf("Removed comment %s from %s %s", args[1], k.noun, args[0])
			return a.ok(success{
				data:        map[string]any{k.arg: args[0], "commentId": args[1], "deleted": true},
				summary:     summary,
				breadcrumbs: k.breadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newItemCommentsReactCmd(k commentedKind) *cobra.Command {
	long := "Leaves a reaction on a comment.\n\n" + reactionHelp
	if k.reactHelp != "" {
		long += "\n\n" + k.reactHelp
	}
	return &cobra.Command{
		Use:     "react <" + k.arg + "> <commentId> <content>",
		Short:   "React to a comment on a " + k.noun,
		Long:    long,
		Example: k.example("react 0199... 0199... 👍"),
		Args:    requireArgs(k.arg, "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.ReactToCommentOn(a.ctx, k.item(args[0]), args[1], args[2])
			if err != nil {
				return k.reactionRefusal(s, err, args[0])
			}

			summary := fmt.Sprintf("Reacted %s to comment %s", args[2], args[1])
			if comment.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("You had reacted %s to comment %s already", args[2], args[1])
			}
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				breadcrumbs: k.breadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newItemCommentsUnreactCmd(k commentedKind) *cobra.Command {
	return &cobra.Command{
		Use:   "unreact <" + k.arg + "> <commentId> <content>",
		Short: "Take a reaction off a comment on a " + k.noun,
		Long:  "Takes a reaction off a comment.\n\n" + reactionHelp,
		Args:  requireArgs(k.arg, "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.UnreactFromCommentOn(a.ctx, k.item(args[0]), args[1], args[2]); err != nil {
				return k.reactionRefusal(s, err, args[0])
			}

			summary := fmt.Sprintf("Took %s off comment %s", args[2], args[1])
			return a.ok(success{
				data:        map[string]any{k.arg: args[0], "commentId": args[1], "content": args[2]},
				summary:     summary,
				breadcrumbs: k.breadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}
