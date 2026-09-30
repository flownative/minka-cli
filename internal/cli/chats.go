package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
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
not read. Their own messages never count, and neither do removed ones or lines
an import brought.`,
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
		Long: `Notes that the person of the token has read a chat up to a message number, by
default the highest number among its latest messages. That need not be the
number of the last message: a transcript is ordered by moment, not by number.
The mark only grows: an older number changes nothing, and a number beyond what
the chat has counted is cut down to that.`,
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
				latest, err := s.client.Messages(a.ctx, args[0], api.MessagesQuery{})
				if err != nil {
					return s.fail(err)
				}
				seq = highestSeq(latest.Value)
			}
			if err := s.client.MarkChatRead(a.ctx, args[0], seq); err != nil {
				return s.fail(err)
			}

			summary := fmt.Sprintf("Marked chat %s read up to message %d", args[0], seq)
			if given {
				// The server may keep another number than the one given and
				// does not say which one it kept.
				summary += "; the server keeps no more than the chat has counted and never lowers the mark"
			}
			return a.ok(success{
				data:    map[string]any{"chatId": args[0], "seq": seq},
				summary: summary,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().IntVar(&seq, "seq", 0, "Message number read up to (default: the highest among the latest messages)")

	return cmd
}

func (a *app) newMessagesCmd() *cobra.Command {
	return group("messages", "Read, write and react to the messages of a chat",
		a.newMessagesListCmd(),
		a.newMessagesCreateCmd(),
		a.newMessagesEditCmd(),
		a.newMessagesDeleteCmd(),
		a.newMessagesReactCmd(),
		a.newMessagesUnreactCmd(),
	)
}

func (a *app) newMessagesListCmd() *cobra.Command {
	var before, after, limit int

	cmd := &cobra.Command{
		Use:   "list <chatId>",
		Short: "Read messages of a chat, oldest first",
		Long: `Reads messages of a chat in the order they were said, oldest first. Without a
flag it gives the latest 50.

seq is the number of a message in its chat: what the read mark counts and what
--before takes. It does not order the transcript. A line an import brings later
takes the next number and stands at the moment it was said, so the numbers down
a page need not rise. Numbers may be missing as well: a chat counts every
change with the same counter.
--before <seq> gives the messages said in front of the message with that
number. To read back, give the seq of the first message of a page, not the
lowest. A number no message carries stands for the highest number below it, and
that message then closes the page.
--after <changeSeq> gives everything that changed after that change number:
new messages, edits and removals in one answer. Remember the highest changeSeq
you have seen and ask for what came after it. --limit sets how many messages
come, up to 200. A removed message is gone from the transcript: only --after
carries it, as removed: true without a text, so that a client drops it.`,
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
					{Action: "read", Cmd: fmt.Sprintf("minka chats mark-read %s --seq %d", args[0], highestSeq(list)), Description: "Mark the chat read up to the highest number of these"},
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
  minka messages create 0199... - --client-message-id "$(uuidgen)" < note.txt`,
		Args: requireArgs("chatId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clientMessageID != "" {
				id, ok := asID(clientMessageID)
				if !ok {
					return usageErrorf(cmd, "--client-message-id has to be a UUID")
				}
				clientMessageID = id
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
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <chatId> <messageId> <text|->",
		Short: "Change a message you wrote",
		Long: `Changes the text of a message. Only its author can. Sending the text it has
already changes nothing.

` + messageFormatHelp + `

Markdown cannot hold a file or a mention, and a reference to another record
survives only as its address. A new text that would drop any of them is
refused unless --drop-attachments is given; send the bodySource of the message
back with --format html to keep them.`,
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
			dropped, err := a.keepsTheAttachmentsOfAMessage(s, args[0], args[1], body, format == "text", dropAttachments)
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)
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
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a message that has attachments, mentions or references, which drops them")

	return cmd
}

// keepsTheAttachmentsOfAMessage holds a new text against the message it
// replaces. No request reads a single message, so the transcript is read back
// page by page until it turns up. Where the chat or the message is not there,
// the message is somebody else's or stands in a chat of an archived project,
// Minka refuses the edit whatever is sent, and its answer comes instead of the
// guard's. Any other failure to read stops the edit: the guard cannot tell
// what it would drop.
func (a *app) keepsTheAttachmentsOfAMessage(s *session, chatID, messageID, body string, plainText, allowed bool) ([]string, error) {
	const message = "the message has %s, and the new text would drop them"
	const hint = "Change the bodySource of the message from messages list --json and send it back with --format html, or give --drop-attachments."
	current, err := a.findMessage(s, chatID, messageID)
	if err != nil {
		if e := s.fail(err); e.Code != CodeNotFound {
			return nil, e
		}
		return nil, nil
	}
	if current == nil || current.Removed {
		return nil, nil
	}
	text := firstNonEmpty(current.BodySource, current.BodyHTML)
	loss := richtext.Dropped(text, body)
	if plainText {
		loss = richtext.DroppedByText(text, body)
	}
	dropped, refusal := refusedLoss(loss, allowed, message, hint)
	if refusal == nil {
		return dropped, nil
	}
	me, err := a.meID(s)()
	if err != nil {
		return nil, err
	}
	if current.Author.ID != me {
		return nil, nil
	}
	archived, err := s.client.Projects(a.ctx, true)
	if err != nil {
		return nil, s.fail(err)
	}
	for _, project := range archived.Value {
		if project.Status != "archived" {
			continue
		}
		chats, err := s.client.Chats(a.ctx, project.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		for _, chat := range chats.Value {
			if chat.ID == chatID {
				return nil, nil
			}
		}
	}
	return nil, refusal
}

// findMessage reads the transcript of a chat backwards from its end until the
// message turns up, or gives nil once a page comes back empty. How many
// messages a page holds is the server's to decide.
func (a *app) findMessage(s *session, chatID, messageID string) (*api.ChatMessage, error) {
	limit := 200
	query := api.MessagesQuery{Limit: &limit}
	seen := map[int]bool{}
	for {
		page, err := s.client.Messages(a.ctx, chatID, query)
		if err != nil {
			return nil, err
		}
		for i := range page.Value {
			if strings.EqualFold(page.Value[i].ID, messageID) {
				return &page.Value[i], nil
			}
		}
		if len(page.Value) == 0 || seen[page.Value[0].Seq] {
			return nil, nil
		}
		before := page.Value[0].Seq
		seen[before] = true
		query.Before = &before
	}
}

func (a *app) newMessagesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <chatId> <messageId>",
		Short: "Remove a message you wrote",
		Long: `Removes a message. Only its author can. The message is gone: it stands in no
page of the transcript any more and counts as nothing unread. Whoever asks with
--after what changed still learns of it, as removed: true without a text.
Removing it again succeeds.`,
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

// highestSeq is the number a reader of these messages has read up to. The last
// message need not carry it: a transcript is ordered by moment, and a line
// brought in later stands further up with a higher number.
func highestSeq(messages []api.ChatMessage) int {
	highest := 0
	for _, message := range messages {
		highest = max(highest, message.Seq)
	}
	return highest
}

// messageText is a message as Markdown, so that a file or a mention in it is
// seen at all: the plain text the server keeps beside a message has neither.
func messageText(message api.ChatMessage) string {
	if source := firstNonEmpty(message.BodySource, message.BodyHTML); richtext.CarriesMarkup(source) {
		return richtext.ToMarkdown(source)
	}
	return message.BodyText
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
		fmt.Fprintf(w, "%s\n%s\n", header, messageText(message))
		if line := fileLine(message.Attachments); line != "" {
			fmt.Fprintf(w, "%s\n", line)
		}
		if line := reactionLine(message.Reactions); line != "" {
			fmt.Fprintf(w, "%s\n", line)
		}
	}
}
