# Chats and private conversations

## Commands

```bash
minka conversations people <conversationId> --json
minka chats list <projectId> --json
minka chats show <chatId> --json
minka messages list <chatId> [--before <seq> | --after <changeSeq>] [--limit <n>] --json
minka messages create <chatId> - [--client-message-id <uuid>] [--reply-to <messageId>] --json < message.txt
minka messages edit <chatId> <messageId> - --json < message.txt
minka messages delete <chatId> <messageId> --json
minka messages react <chatId> <messageId> 👍 --json
minka messages unreact <chatId> <messageId> 👍 --json
minka chats mark-read <chatId> [--seq <seq>] --json
minka conversations list --json
minka conversations with <person> [--project <projectId>] --json
```

## Chats

- `chats list <projectId>` gives `{id, name, unreadCount, appUrl}`.
  `unreadCount` never counts your own messages, removed ones or lines an
  import brought. The same holds for a conversation.
- `chats show <chatId>` gives one chat of a project by its ID alone as `{id,
  projectId, project {id, name}, name, appUrl}`. A private conversation stands
  in no project and exits 2 there.
- `messages list --styled` shows a message as Markdown when it carries markup,
  so a file in it is a Markdown image or link and a mention is `@Name`. The
  plain `bodyText` beside every message has neither.
- `messages list <chatId>` gives messages oldest first as `{id, seq,
  changeSeq, author, bodyHtml, bodyText, createdAt, editedAt, removed,
  replyTo}`, the latest 50 by default, `--limit` up to 200. `replyTo` is the
  `id` of the message a reply answers, null for most; the text view shows it
  as "In reply to #<seq>, <author>" where that message is on the page.
  - The list is ordered by moment (`createdAt`, then `seq`), not by `seq`. Keep
    the order it comes in; never sort by `seq`.
  - `seq` is the number of a message in its chat, the unit of the read mark and
    the cursor of `--before`. It is no position: a line an import brings later
    takes the next number and stands at the moment it was said, so a transcript
    may run 1, 7, 3, 6. Numbers are missing too: a chat counts every change
    with the same counter, so an edit, a removal or a reaction leaves a number
    unused. `changeSeq` is the field to catch up with.
  - Read back with `--before <seq>`, giving the `seq` of the first message of
    the page you have, not the lowest: the messages said in front of that one
    come. A number no message carries stands for the highest number below it,
    and that message closes the page.
  - Catch up with `--after <changeSeq>`: new messages, edits and removals in
    one answer. Keep the highest `changeSeq` you have seen.
  - A removed message is gone from the transcript: no page of it carries the
    message any more. Only `--after` names it, as `removed: true` with empty
    text, which is the sign to drop it from what you keep.
- `messages create` and `messages edit` take Markdown by default and send it
  as HTML, keeping every line break, as comments do. `--format text` sends the
  text as it is (the server keeps it as plain text with line breaks and
  links), `--format html` sends HTML. Always pass `--client-message-id` with a UUID of your own: sending it
  again then writes nothing new, and a failed send is `retryable`.
- `messages create --reply-to <messageId>` answers an earlier message, named by
  its `id`. It has to stand in the same chat and not be removed, otherwise exit
  9 with `chatMessageReplyToUnknown`. Only the reference is kept, so the quote
  above a reply always shows the words as they stand now; nobody is notified.
- `messages edit` and `messages delete` only work on your own messages;
  somebody else's exits 2. Deleting twice succeeds.
- `messages edit` refuses a new text that would drop a file, a mention or a
  reference of the message, like `comments edit`, unless `--drop-attachments`
  is given; send its `bodySource` back with `--format html` to keep them. It
  reads the transcript back until it finds the message, since no request reads
  one message alone.
- `chats mark-read <chatId>` marks the chat read up to `--seq`, which is the
  highest `seq` you have seen and not that of the last message of a list;
  without it, up to the highest among the latest messages. The mark only grows.
  `data.seq` is the number sent, not the mark the server kept.

## Private conversations

- A conversation is a chat without a project, between two people. `conversations
  list` gives `{id, name, counterpart {id, name}, lastMessageText,
  lastMessageAt, unreadCount, appUrl}`, the newest word first. `name` is the
  other person. `lastMessageText` is the last word without markup, its line
  breaks folded into spaces and cut to 200 characters with an ellipsis; it is
  a hint, not the message, and needs no shortening of your own. Both last
  fields are null while nothing has been said.
- `conversations with <person>` answers with the conversation with that person
  and begins it where there is none, so asking twice is no mistake. A person is
  given by ID; with `--project` a name or an e-mail address is looked up among
  the members of that project. Nobody of the organization, and your own ID,
  exit 2.
- The ID of a conversation is a chat ID: read it with `messages list`, write
  with `messages create`, change and remove with `messages edit` and `messages
  delete`, note how far it is read with `chats mark-read`. Everybody else
  exits 2 there.
- A conversation carries files and mentions too: `uploads create --conversation`
  puts a file in, and `conversations people <conversationId>` gives the two
  people with the markup that names them there. Somebody outside the two
  exits 2.
- Conversations are not among the chats of a project, and the chats of a
  project are not among the conversations.
