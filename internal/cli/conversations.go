package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const conversationHelp = `A conversation is a private talk between two people, without a project. Its ID
is a chat ID: read it with messages list, write in it with messages create,
change and remove with messages edit and messages delete, and note how far it
is read with chats mark-read. Files and mentions work there too: upload with
uploads create --conversation, and name the other person with the markup from
conversations people.`

func (a *app) newConversationsCmd() *cobra.Command {
	return group("conversations", "List private conversations, begin one and see who is in it",
		a.newConversationsListCmd(),
		a.newConversationsWithCmd(),
		a.newConversationsPeopleCmd(),
	)
}

func (a *app) newConversationsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your private conversations, the newest first",
		Long:  "Lists the private conversations of the account of the token.\n\n" + conversationHelp,
		Args:  requireArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			conversations, err := s.client.Conversations(a.ctx)
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    conversations.Raw,
				summary: plural(len(conversations.Value), "conversation", "conversations"),
				breadcrumbs: []Breadcrumb{
					{Action: "read", Cmd: "minka messages list <conversationId>", Description: "Read a conversation"},
					{Action: "write", Cmd: "minka messages create <conversationId> \"Text\"", Description: "Say something in a conversation"},
				},
				text: func(w io.Writer) {
					if len(conversations.Value) == 0 {
						fmt.Fprintln(w, "No conversations.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tWITH\tUNREAD\tLAST")
					for _, conversation := range conversations.Value {
						last := ""
						if conversation.LastMessageAt != nil {
							last = formatTime(*conversation.LastMessageAt)
						}
						if conversation.LastMessageText != nil {
							last += "  " + *conversation.LastMessageText
						}
						fmt.Fprintf(table, "%s\t%s\t%d\t%s\n", conversation.ID, conversation.Name, conversation.UnreadCount, last)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newConversationsWithCmd() *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "with <person>",
		Short: "Begin a private conversation with somebody, or find the one you have",
		Long: `Answers with the conversation with that person, and begins it where there is
none. Asking again is no mistake: two people have one conversation, and the
same one comes back.

The person is given by ID. To name them by e-mail address or name instead,
give --project: the members of that project are looked up for the name.

` + conversationHelp,
		Example: `  minka conversations with 0199...
  minka conversations with anke@example.com --project 0199...`,
		Args: requireArgs("person"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectID != "" {
				if err := requireID(cmd, "--project", &projectID); err != nil {
					return err
				}
			}
			s, err := a.session()
			if err != nil {
				return err
			}

			personID, isID := asID(args[0])
			if !isID {
				if projectID == "" {
					return usageErrorf(cmd, "%q is no person ID: give the ID, or --project to look the name up among its members", args[0])
				}
				members, err := s.client.People(a.ctx, projectID)
				if err != nil {
					return s.fail(err)
				}
				person, err := resolvePerson(args[0], members.Value, a.meID(s))
				if err != nil {
					return err
				}
				personID = person.ID
			}

			conversation, err := s.client.StartConversation(a.ctx, personID)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "Nobody of your organization has this ID. A conversation with yourself is none either."
				}
				return e
			}

			summary := "Conversation with " + conversation.Value.Name
			return a.ok(success{
				data:    conversation.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "read", Cmd: "minka messages list " + conversation.Value.ID, Description: "Read the conversation"},
					{Action: "write", Cmd: "minka messages create " + conversation.Value.ID + " \"Text\"", Description: "Say something"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s: %s\n%s\n", summary, conversation.Value.ID, conversation.Value.AppURL)
				},
			})
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project whose members a name or an e-mail address is looked up in")

	return cmd
}
