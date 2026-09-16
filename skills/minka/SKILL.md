---
name: minka
description: >
  Work with a Minka instance through the minka CLI: list projects and boards,
  read a board with its columns and cards, show a card with description and
  comments, create and edit cards, move and complete them, comment on them,
  read and write chat messages.
  Use for any task on a Minka board, such as tracking development tasks.
---

# minka

`minka` talks to the JSON API of a Minka instance. Every command prints JSON
when stdout is not a terminal, so an agent gets structured output by default.

## Agent invariants

- Pass `--json` explicitly. Read `ok`, then `data`; on failure read `code`,
  `retryable`, `hint` and `meta.apiCode`.
- Never put a token on the command line. Sign in once with
  `minka auth login --host <url> --with-token < file`, or set `MINKA_TOKEN`
  together with `MINKA_HOST`; the environment token is only sent to that host.
- Pass long or multi-line text through stdin with `-`, never through shell
  quoting: `--description -` for cards, `<text>` = `-` for comments. Stdin
  has to be a pipe or a file, not a terminal.
- IDs are UUIDs. Take them from earlier output, never guess them. A value that
  is no UUID is refused with exit 1 before any request.
- Retry only when `retryable` is true, which only happens for reads. After a
  failed write, read the card back before trying again: the change may have
  reached the server. A 404 (`not_found`) means the thing does not exist or
  belongs to a project the token's person is not in.
- `minka cards done` is safe to repeat: a card already in Done is left alone.
- `minka cards delete` moves a card to the trash. Repeating it changes
  nothing on the server, but a card in the trash reads like one that never
  existed, so a second delete exits 2 instead of 0. After a failed delete,
  exit 2 on a retry means the card is gone. Delete only cards you created or
  were asked to remove.

## Output modes

| Goal | Flag | Output |
|------|------|--------|
| Structured with hints | `--json` | `{ok, data, summary, notice, breadcrumbs}` |
| Only the resource | `--quiet` / `-q` | the API resource as JSON |
| Reading as a person | `--styled` | Markdown-like text |

`data` is the API resource unchanged. `notice` carries warnings, such as
formatting the Markdown conversion had to change. `breadcrumbs` suggest the
next commands.

Errors: `{"ok": false, "error", "code", "retryable", "hint", "meta": {"apiCode", "httpStatus"}}`.

## Exit codes

| Exit | code | Meaning |
|------|------|---------|
| 0 | | success |
| 1 | `usage`, `config` | wrong command line, unreadable or insecure config file |
| 2 | `not_found` | does not exist or not visible to you |
| 3 | `auth_required` | not signed in, token refused (401) |
| 4 | `forbidden` | account may not use the API (403) |
| 6 | `network` | host unreachable, timeout (retryable) |
| 7 | `api_error` | server error or unexpected answer (5xx retryable) |
| 8 | `ambiguous` | a column, person or step given by name matches more than one |
| 9 | `validation` | the server refused the input, see `meta.apiCode` |

Validation codes in `meta.apiCode`: `cardTitleMissing`, `cardTitleTooLong`,
`cardTitleHasControlCharacters` (a title is one line: no line breaks or tabs),
`cardDescriptionTooLong`, `cardDescriptionNotReadable`, `commentMissing`,
`commentTooLong`, `commentHasControlCharacters` (line breaks and tabs are
fine), `commentNotReadable`, `cardDueDateNotADate`, `personNotAProjectMember`, `personIdMissing`,
`stepMissing`, `stepTooLong`, `stepHasControlCharacters`, `stepCompletedMissing`,
`chatMessageMissing`, `chatMessageTooLong`, `chatMessageHasControlCharacters`,
`chatMessageNotReadable`, `chatMessageDraftNotAnIdentifier`, `chatMessagesLimitNotANumber`,
`chatReadSeqMissing`, `nothingToChange`, `columnIdMissing`, `malformedBody`,
`unsupportedMediaType`.

Minka answers everything you may not see with 404, never 403, so exit 4 is
not expected in practice.

## Introspection

- `minka commands --json`: every command with usage and flags.
- `minka <command> --help`: description and examples.

## Quick reference

```bash
minka auth status --json
minka projects list --json
minka boards list <projectId> --json
minka boards show <boardId> --json
minka cards mine --json
minka cards show <cardId> --json
minka cards create --board <boardId> --title "Title" [--column <column>] [--assignee <person>]... [--due YYYY-MM-DD] [--description - < body.md] --json
minka cards edit <cardId> [--title "New"] [--due YYYY-MM-DD] [--description - < body.md] --json
minka cards move <cardId> --column <column> [--top | --bottom | --after <cardId> | --before <cardId>] --json
minka cards done <cardId> --json
minka cards delete <cardId> --json
minka cards restore <cardId> --json
minka people list <projectId> --json
minka cards assign <cardId> <person> --json
minka cards unassign <cardId> <person> --json
minka cards subscribe <cardId> <person> --json
minka cards unsubscribe <cardId> <person> --json
minka steps list <cardId> --json
minka steps create <cardId> "Text" --json
minka steps complete <cardId> <step> --json
minka steps reopen <cardId> <step> --json
minka steps delete <cardId> <step> --json
minka comments create <cardId> - --json < comment.txt
minka comments edit <cardId> <commentId> - [--format html] --json < comment.txt
minka comments delete <cardId> <commentId> --json
minka chats list <projectId> --json
minka messages list <chatId> [--before <seq> | --after <changeSeq>] [--limit <n>] --json
minka messages create <chatId> - [--client-message-id <uuid>] --json < message.txt
minka messages edit <chatId> <messageId> - --json < message.txt
minka messages delete <chatId> <messageId> --json
minka chats mark-read <chatId> [--seq <seq>] --json
minka conversations list --json
minka conversations with <person> [--project <projectId>] --json
minka uploads create <file> (--project <projectId> | --conversation <conversationId>) [--name <name>] [--content-type <type>] --json
```

`<column>` is a column ID, a type (`triage`, `not_now`, `done`) or a title
such as `Doing`.

## Your own cards

- `cards mine` lists the cards the account of the token is assigned to, across
  every project it is a member of, as a bare array of the same cards
  `cards show` gives. Each carries `project {id, name}` and `board {id, title}`
  besides `projectId` and `boardId`.
- Cards in Done, cards in the trash and cards of a project that was left are
  not among them, so the list is what is still to do.
- The order is the order of the page: projects by name, then by `dueOn` with
  the overdue ones first and the ones without a date last.
- It is the cheapest way to answer "what am I on?": one request instead of a
  board per project.
- `cards delete` puts a card in the trash and `cards restore <cardId>` takes it
  back out, on top of the column it hung in. Only a card that is in the trash
  can be restored: a second restore, a card that was never trashed and a card
  whose board or project is in the trash all exit 2. Nothing lists the trash,
  so keep the ID of a card you delete if you may want it back.

## Boards

- Column types: `triage` (landing zone for new cards), `column` (working
  columns), `not_now`, `done`.
- A new card lands in Triage unless `--column` names Triage or a working
  column. Not now and Done take no new cards; create, then move.
- Moving into Done completes a card (`completedAt`, `completedBy`); moving out
  reopens it. Without placement a moved card goes to the top. Done is ordered
  by completion and refuses `--bottom`, `--after` and `--before`.
- A column title that equals a type or another column's title is ambiguous
  (exit 8); the hint lists the candidate IDs.
- Done lists only the 25 cards completed last; `cardCount` counts all.
  Read an older card by its ID.

## People and assignments

- A card has any number of assignees, listed in `assignees` as `{id, name}`
  in the order they were assigned.
- `<person>` is an ID, `me`, an e-mail address or a name (case-insensitive,
  it has to match one person; two people with that name give exit 8).
- Only members of the card's project can be assigned; `minka people list
  <projectId>` shows them. A UUID of somebody else gives exit 9 with
  `meta.apiCode` `personNotAProjectMember`.
- Assigning somebody who is assigned already succeeds, and the summary says
  so. Taking off somebody who is not assigned succeeds as well, and people who
  left the project can still be taken off.
- `cards create --assignee <person>` (repeatable) looks the people up first
  and creates the card together with them in one request. Somebody outside
  the project leaves no card behind (exit 9, `personNotAProjectMember`).
- New cards go to the top of their column.
- A card read by itself carries `subscribers` as `{id, name}`, sorted by
  name: the people it notifies about comments, its due date and completion.
  Minka subscribes people by itself when they create a card, are assigned or
  comment. `cards subscribe <cardId> <person>` adds somebody, `cards
  unsubscribe` takes somebody off and keeps Minka from adding them again.
  Both succeed when nothing changes; only project members can be named
  (`personNotAProjectMember`, exit 9).

## Steps

- A card read by itself carries `steps` as `{id, title, completed}` in the
  order it shows them. The board carries only `stepCount` and
  `completedStepCount` per card; read the card for the steps themselves.
- `<step>` is a step ID, its position starting at 1, or its text
  (case-insensitive; a number always means a position; a text on two steps
  gives exit 8 with the positions).
- `steps create` adds a step at the end. A step is one line of at most 500
  characters: `stepMissing`, `stepTooLong` and `stepHasControlCharacters` exit 9.
- Completing a completed step, reopening an open one and deleting a step that
  is gone all succeed. Deleting does not ask.
- Renaming and reordering steps are not possible through the API yet.

## Due dates

- A card carries `dueOn` as `YYYY-MM-DD`, or null. It is a day, with no time
  and no zone.
- Set it with `cards create --due YYYY-MM-DD` or `cards edit --due YYYY-MM-DD`;
  remove it with `cards edit --due ""`. Relative words such as `tomorrow` are
  not understood: compute the day yourself.
- A text that is no existing day (`2026-02-30`, `28.09.2026`, `2026-9-8`)
  exits 9 with `meta.apiCode` `cardDueDateNotADate`.

## Chats

- `chats list <projectId>` gives `{id, name, unreadCount, appUrl}`.
  `unreadCount` counts own messages as well, removed ones never.
- `messages list <chatId>` gives messages oldest first as `{id, seq,
  changeSeq, author, bodyHtml, bodyText, createdAt, editedAt, removed}`, the
  latest 50 by default, `--limit` up to 200.
  - Read back with `--before <seq>` (messages in front of that one).
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
- `messages edit` and `messages delete` only work on your own messages;
  somebody else's exits 2. Deleting twice succeeds.
- `chats mark-read <chatId>` marks the chat read up to its last message, or up
  to `--seq`. The mark only grows.

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
- A conversation is text: files and mentions belong to projects, and
  `bodySource` is the text itself.
- Conversations are not among the chats of a project, and the chats of a
  project are not among the conversations.

## Descriptions and comments

- Descriptions are Markdown by default and sent as HTML. Minka keeps every
  heading level, bold, italic, strikethrough, inline code, links with titles,
  lists (numbered ones also from another start), quotes, code blocks with
  their language, tables and horizontal rules. Images become links, links
  other than http, https, mailto, tel and relative ones become plain text.
  These changes are reported in `notice`, and on stderr for text output and
  `--quiet`.
- Markdown that holds text but nothing Minka keeps (only link reference
  definitions, for example) is refused, so it cannot remove a description by
  accident. To remove a description, give `--description ""`.
- `--description-format html` sends HTML as it is; the server sanitizes it.
- Every text comes twice: stored (`description`, `body`, `bodyHtml`) and as
  its source (`descriptionSource`, `bodySource`). Files and mentions stand in
  the source as `<action-text-attachment>` elements with fresh signatures and
  their `url`, `filename` and `content-type`. `cards show --styled` shows a
  picture as a Markdown image, a file as a link and a mention as `@Name`.
- Markdown cannot hold files or mentions. To change a description that has
  them, take `descriptionSource` from `minka cards show <cardId> --quiet`,
  change it and send it back with `cards edit --description - --description-format html`.
  A new description that would drop them is refused (exit 1) unless
  `--drop-attachments` is given.
- `cards show --styled` prints the description as Markdown, which can be edited
  and sent back with `cards edit --description -`.
- Comments and messages carry `reactions`: `[{content, people}]`, the same
  content counted together, in the order it was first left in, and an empty
  array where nobody reacted. `content` is mostly an emoji but may be up to 20
  characters of text. `cards show --styled` and `messages list --styled` put
  them in a line under the comment or the message. Reacting is not in the API
  yet, so the CLI can only read them.
- Comments are rich text like descriptions. `comments create` converts
  Markdown and keeps every line break, so plain text arrives as typed;
  `--format html` sends HTML as it is. `cards show --styled` shows comments as
  Markdown; older comments without markup come back as plain text and are
  shown as they are.
- `comments edit <cardId> <commentId>` and `comments delete` work on your own
  comments only; somebody else's exits 2. A removed comment leaves nothing
  behind, so a second delete exits 2 as well. Like `cards edit`, `comments
  edit` refuses a new text that would drop attachments or mentions unless
  `--drop-attachments` is given; send the comment's `bodySource` back with
  `--format html` to keep them.

## Uploads

- `uploads create <file> --project <projectId>` puts a file into a project and
  answers with `{id, filename, contentType, size, kind, appUrl,
  attachmentSource}`. `--conversation <conversationId>` puts it into a private
  conversation instead. Exactly one of the two is given.
  `kind` is `image` for a picture, which is shown where it stands, and `file`
  for everything else, which is handed over.
- `attachmentSource` is the whole point: the `<action-text-attachment>` element
  with a signature only the server can make. Put it into a `descriptionSource`
  or a `bodySource`, unchanged, and send that back as HTML:

```bash
SOURCE=20 20 12 61 79 80 81 98 33 100 204 250 395 398 399 400minka uploads create shot.png --project <projectId> --quiet | jq -r .attachmentSource)
minka cards show <cardId> --quiet | jq -r .descriptionSource > text.html
printf '\n%s' "$SOURCE" >> text.html
minka cards edit <cardId> --description - --description-format html < text.html
```

- The attachment belongs to the circle it went into: a file of a project hangs
  in the texts of that project, a file of a conversation in the messages of
  that conversation. Anywhere else the server drops it while saving, without an
  error, so read the text back if it matters.
- A file is kept only once a text carrying it is saved. An upload no saved text
  points to is removed after seven days.
- The media type comes from the name of the file, and from its first bytes
  where the name says nothing. `--content-type` gives it instead. The server
  reads the bytes itself: a file that says it is a picture and is not exits 9
  with `meta.apiCode` `contentTypeMismatch`, a broken picture `notReadable`.
- At most 50 MiB. A larger file exits 1 before it travels, and a server that
  refuses one exits 9 with `tooLarge`.
- `--name` gives the file another name in Minka than it has here.

## Common workflows

Find open work on a board:

```bash
minka boards show <boardId> --quiet \
  | jq '.columns[] | select(.type != "done") | {title, cards: [.cards[] | {id, title}]}'
```

Create a card with a multi-line description:

```bash
minka cards create --board <boardId> --title "Revoke tokens from the UI" --description - --json <<'EOF'
Tokens can only be revoked on the command line.

- list tokens of a person
- revoke with confirmation
EOF
```

Finish a task and leave a note:

```bash
minka cards done <cardId> --json
printf 'Done in %s' "$(git rev-parse --short HEAD)" | minka comments create <cardId> - --json
```
