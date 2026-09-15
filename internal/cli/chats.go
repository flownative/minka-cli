package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/spf13/cobra"
)

const messageAuthorHint = "It does not exist, or somebody else wrote it: only the author can change or remove a message."

func (a *app) newChatsCmd() *cobra.Command {
	return group("chats", "List the chats of a project and mark them read",
		a.newChatsListCmd(),
		a.newChatsMarkReadCmd(),
	)
}

func (a *app) newChatsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <projectId>",
		Short: "List the chats of a project with what is unread in them",
		Long: `Lists the chats of a project. unreadCount is what the person of the token has
not read; their own messages and removed ones count as well.`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			chats, err := s.client.Chats(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    chats.Raw,
				summary: plural(len(chats.Value), "chat", "chats"),
				breadcrumbs: []Breadcrumb{
					{Action: "messages", Cmd: "minka messages list <chatId>", Description: "Read the latest messages of a chat"},
				},
				text: func(w io.Writer) {
					if len(chats.Value) == 0 {
						fmt.Fprintln(w, "No chats.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tNAME\tUNREAD")
					for _, chat := range chats.Value {
						fmt.Fprintf(table, "%s\t%s\t%d\n", chat.ID, chat.Name, chat.UnreadCount)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newChatsMarkReadCmd() *cobra.Command {
	var seq int

	cmd := &cobra.Command{
		Use:   "mark-read <chatId>",
		Short: "Note how far a chat has been read",
		Long: `Notes that the person of the token has read a chat up to a message, by default
up to its last one. The mark only grows: an older number changes nothing, and a
number beyond the last message counts as the last one.`,
		Example: `  minka chats mark-read 0199...
  minka chats mark-read 0199... --seq 42`,
		Args: requireArgs("chatId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			given := cmd.Flags().Changed("seq")
			if given && seq < 0 {
				return usageErrorf(cmd, "--seq is a message number of at least 0")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if !given {
				one := 1
				latest, err := s.client.Messages(a.ctx, args[0], api.MessagesQuery{Limit: &one})
				if err != nil {
					return s.fail(err)
				}
				if n := len(latest.Value); n > 0 {
					seq = latest.Value[n-1].Seq
				}
			}
			if err := s.client.MarkChatRead(a.ctx, args[0], seq); err != nil {
				return s.fail(err)
			}

			summary := fmt.Sprintf("Marked chat %s read up to message %d", args[0], seq)
			return a.ok(success{
				data:    map[string]any{"chatId": args[0], "seq": seq},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().IntVar(&seq, "seq", 0, "Message number read up to (default: the last message)")

	return cmd
}

func (a *app) newMessagesCmd() *cobra.Command {
	return group("messages", "Read and write the messages of a chat",
		a.newMessagesListCmd(),
		a.newMessagesCreateCmd(),
		a.newMessagesEditCmd(),
		a.newMessagesDeleteCmd(),
	)
}

func (a *app) newMessagesListCmd() *cobra.Command {
	var before, after, limit int

	cmd := &cobra.Command{
		Use:   "list <chatId>",
		Short: "Read messages of a chat, oldest first",
		Long: `Reads messages of a chat, oldest first. Without a flag it gives the latest 50.

--before <seq> gives the messages in front of that message, for reading back.
--after <changeSeq> gives everything that changed after that change number:
new messages, edits and removals in one answer. Remember the highest changeSeq
you have seen and ask for what came after it. --limit sets how many messages
come, up to 200. A removed message comes along with removed: true and no text.`,
		Example: `  minka messages list 0199...
  minka messages list 0199... --before 120 --limit 50
  minka messages list 0199... --after 431`,
		Args: requireArgs("chatId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			var query api.MessagesQuery
			if flags.Changed("before") && flags.Changed("after") {
				return usageErrorf(cmd, "give --before or --after, not both")
			}
			if flags.Changed("before") {
				if before < 1 {
					return usageErrorf(cmd, "--before is a message number of at least 1")
				}
				query.Before = &before
			}
			if flags.Changed("after") {
				if after < 0 {
					return usageErrorf(cmd, "--after is a change number of at least 0")
				}
				query.After = &after
			}
			if flags.Changed("limit") {
				query.Limit = &limit
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			messages, err := s.client.Messages(a.ctx, args[0], query)
			if err != nil {
				return s.fail(err)
			}

			list := messages.Value
			var breadcrumbs []Breadcrumb
			if len(list) > 0 {
				highestChange := 0
				for _, message := range list {
					highestChange = max(highestChange, message.ChangeSeq)
				}
				breadcrumbs = []Breadcrumb{
					{Action: "earlier", Cmd: fmt.Sprintf("minka messages list %s --before %d", args[0], list[0].Seq), Description: "Read the messages in front of these"},
					{Action: "changes", Cmd: fmt.Sprintf("minka messages list %s --after %d", args[0], highestChange), Description: "Read what changed after these"},
					{Action: "read", Cmd: fmt.Sprintf("minka chats mark-read %s --seq %d", args[0], list[len(list)-1].Seq), Description: "Mark the chat read up to the last of these"},
				}
			}

			return a.ok(success{
				data:        messages.Raw,
				summary:     plural(len(list), "message", "messages"),
				breadcrumbs: breadcrumbs,
				text:        func(w io.Writer) { renderMessages(w, list) },
			})
		},
	}

	flags := cmd.Flags()
	flags.IntVar(&before, "before", 0, "Only messages in front of this message number")
	flags.IntVar(&after, "after", 0, "Only what changed after this change number")
	flags.IntVar(&limit, "limit", 0, "How many messages, up to 200 (default 50)")

	return cmd
}

const messageFormatHelp = `The text is Markdown by default and is sent as HTML, the way a comment is:
every line break is kept, and addresses become links. --format text sends the
text as it is, and the server keeps it as plain text with paragraphs, line
breaks and links. --format html sends HTML as it is; the server reduces it to
what it keeps. Give - as the text to read it from stdin.`

// messageBody turns the text of a chat message into what is sent.
func messageBody(cmd *cobra.Command, text, format string) (string, []string, error) {
	switch format {
	case "text", "html":
		return text, nil, nil
	case "markdown":
		result := markdown.ToHTML(text, markdown.KeepLineBreaks())
		if result.HTML == "" && strings.TrimSpace(text) != "" {
			return "", nil, &Error{Code: CodeUsage, Message: "the message has nothing Minka keeps"}
		}
		return result.HTML, result.Warnings, nil
	default:
		return "", nil, usageErrorf(cmd, "--format is markdown, text or html, not %q", format)
	}
}

func (a *app) newMessagesCreateCmd() *cobra.Command {
	var clientMessageID, format string

	cmd := &cobra.Command{
		Use:   "create <chatId> <text|->",
		Short: "Write a message in a chat",
		Long: `Writes a message in a chat.

` + messageFormatHelp + `

--client-message-id names the message with a UUID of your own. The same one
sent again writes nothing new and answers with the message written before, so
a message whose sending failed can be sent again without appearing twice.`,
		Example: `  minka messages create 0199... "Tent is booked"
  minka messages create 0199... - --client-message-id "$(uuidgen | tr A-Z a-z)" < note.txt`,
		Args: requireArgs("chatId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clientMessageID != "" && !uuidPattern.MatchString(clientMessageID) {
				return usageErrorf(cmd, "--client-message-id has to be a UUID")
			}
			if _, _, err := messageBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the message", args[1])
			if err != nil {
				return err
			}
			body, notices, err := messageBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			message, err := s.client.PostMessage(a.ctx, args[0], body, clientMessageID)
			if err != nil {
				e := s.fail(err)
				switch {
				case clientMessageID != "" && (e.Code == CodeNetwork || e.Code == CodeAPI && e.HTTPStatus >= 500):
					e.Retryable = true
					e.Hint = "Send it again with the same --client-message-id: the message is written only once."
				case e.Hint == writeMayHaveHappened:
					e.Hint += " With --client-message-id, sending again is safe."
				}
				return e
			}

			summary := fmt.Sprintf("Wrote message %d in chat %s", message.Value.Seq, args[0])
			if message.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("Message %d was written with this --client-message-id already", message.Value.Seq)
			}
			return a.ok(success{
				data:    message.Raw,
				summary: summary,
				notices: notices,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&clientMessageID, "client-message-id", "", "A UUID that makes sending this message again safe")
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown, text or html")

	return cmd
}

func (a *app) newMessagesEditCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "edit <chatId> <messageId> <text|->",
		Short: "Change a message you wrote",
		Long: `Changes the text of a message. Only its author can. Sending the text it has
already changes nothing.

` + messageFormatHelp,
		Args: requireArgs("chatId", "messageId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := messageBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the message", args[2])
			if err != nil {
				return err
			}
			body, notices, err := messageBody(cmd, text, format)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			message, err := s.client.EditMessage(a.ctx, args[0], args[1], body)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = messageAuthorHint
				}
				return e
			}

			summary := fmt.Sprintf("Changed message %d in chat %s", message.Value.Seq, args[0])
			return a.ok(success{
				data:    message.Raw,
				summary: summary,
				notices: notices,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown, text or html")

	return cmd
}

func (a *app) newMessagesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <chatId> <messageId>",
		Short: "Remove a message you wrote",
		Long: `Removes a message. Only its author can. The message stays in the transcript,
marked as removed and without its text. Removing it again succeeds.`,
		Args: requireArgs("chatId", "messageId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteMessage(a.ctx, args[0], args[1]); err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = messageAuthorHint
				}
				return e
			}

			summary := fmt.Sprintf("Removed message %s from chat %s", args[1], args[0])
			return a.ok(success{
				data:    map[string]any{"chatId": args[0], "messageId": args[1], "deleted": true},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func renderMessages(w io.Writer, messages []api.ChatMessage) {
	if len(messages) == 0 {
		fmt.Fprintln(w, "No messages.")
		return
	}
	for i, message := range messages {
		if i > 0 {
			fmt.Fprintln(w)
		}
		header := fmt.Sprintf("#%d %s, %s", message.Seq, message.Author.Name, formatTime(message.CreatedAt))
		if message.Removed {
			fmt.Fprintf(w, "%s (removed)\n", header)
			continue
		}
		if message.EditedAt != nil {
			header += " (edited)"
		}
		fmt.Fprintf(w, "%s\n%s\n", header, message.BodyText)
	}
}
