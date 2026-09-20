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
- IDs are UUIDs. Take them from earlier output, never guess them. A card may
  also be named by its short identifier, such as `MNK-1A`. A value that is
  neither is refused with exit 1 before any request.
- Retry only when `retryable` is true, which only happens for reads. After a
  failed write, read the card back before trying again: the change may have
  reached the server. A 404 (`not_found`) means the thing does not exist,
  belongs to a project the token's person is not in, or its project is in the
  trash.
- Exit 9 with `meta.apiCode` `projectArchived` means the project is archived:
  it can be read, nothing can be written, and trying again does not help. Tell
  the person; putting it back in use is done in Minka on the web.
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
| 1 | `usage`, `config` | wrong command line, unreadable or insecure config file, a missing Basecamp CLI, a bundle that cannot be written |
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
`unsupportedMediaType`, `projectArchived`.

Minka answers everything you may not see with 404, never 403, so exit 4 is
not expected in practice.

## Introspection

- `minka commands --json`: every command with usage and flags.
- `minka <command> --help`: description and examples.
- `minka version --json`: the version of this minka.
- `minka doctor --json`: checks the config file, the sign-in, whether the host
  answers and accepts the token, and the installed skill. It is a report and
  exits 0 whatever it finds: read `data.healthy`, and for every entry of
  `data.checks` its `status` (`ok`, `warn`, `fail`, or `skip` where a check
  before it failed), `detail` and `hint`. Run
  it first when a command fails in a way you do not understand.
- `minka setup` is for a person on a terminal: it asks for the host and the
  token and offers to install this skill. Without a terminal it exits 1; take
  `minka auth login --host <url> --with-token < file` and `minka skill install`
  instead.
- `minka auth logout --host <url>`: forget the token of a host. The token stays
  valid until it is revoked on the server.

## This skill

The binary carries this description, so it fits the commands of the version
that is installed.

- `minka skill show --styled`: print it.
- `minka skill install`: write it to `~/.claude/skills/minka/SKILL.md`, or
  into another directory of skills with `--dir <dir>`. A directory of
  skills or a skill directory that is a symbolic link is refused (exit 1)
  rather than written through, and draws no notice.
- `minka skill status --json`: `data.state` is `current`, `differs` or
  `not installed`.

After an upgrade the installed copy is older than the binary. minka never
replaces it by itself; every command then carries a `notice` that says so.
When you see it, run `minka skill install` and read the skill again.

## Quick reference

```bash
minka auth status --json
minka me --json
minka projects list [--archived] --json
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
minka trash list <projectId> --json
minka people list <projectId> --json
minka conversations people <conversationId> --json
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
minka comments react <cardId> <commentId> 👍 --json
minka comments unreact <cardId> <commentId> 👍 --json
minka chats list <projectId> --json
minka messages list <chatId> [--before <seq> | --after <changeSeq>] [--limit <n>] --json
minka messages create <chatId> - [--client-message-id <uuid>] --json < message.txt
minka messages edit <chatId> <messageId> - --json < message.txt
minka messages delete <chatId> <messageId> --json
minka messages react <chatId> <messageId> 👍 --json
minka messages unreact <chatId> <messageId> 👍 --json
minka chats mark-read <chatId> [--seq <seq>] --json
minka conversations list --json
minka conversations with <person> [--project <projectId>] --json
minka uploads create <file> (--project <projectId> | --conversation <conversationId>) [--name <name>] [--content-type <type>] --json
minka import basecamp --project <basecampProjectId> [--project <basecampProjectId>]... (--out <dir> | --dry-run) --json
```

`<column>` is a column ID, a type (`triage`, `not_now`, `done`) or a title
such as `Doing`.

## Your own cards

- `cards mine` lists the cards the account of the token is assigned to, across
  every project it is a member of, as a bare array of the same cards
  `cards show` gives. Each carries `project {id, name}` and `board {id, title}`
  besides `projectId` and `boardId`.
- Cards in Done, cards in the trash, cards of an archived project and cards of
  a project that was left are not among them, so the list is what is still to
  do.
- The order is the order of the page: projects by name, then by `dueOn` with
  the overdue ones first and the ones without a date last.
- It is the cheapest way to answer "what am I on?": one request instead of a
  board per project.
- `cards delete` puts a card in the trash and `cards restore <cardId>` takes it
  back out, on top of the column it hung in. Only a card that is in the trash
  can be restored: a second restore, a card that was never trashed and a card
  whose board or project is in the trash all exit 2.
- `trash list <projectId>` says what was taken out of a project, the newest
  first and at most 200 (a `notice` says when there are more). Every entry is
  `{kind, id, title, toolType, board, column, trashedAt, trashedBy,
  restorable, blockedBy, appUrl}`. `kind` is `card`, `column` or `tool`;
  `restorable` says whether `cards restore` takes it, `blockedBy` which of
  board and column is in the way. Columns and tools come back in the web only.

## Projects

- `projects list` gives the projects in use as `{id, name, description,
  status, appUrl}`; `status` is `active` or `archived`. `--archived` lists the
  archived ones instead.
- An archived project stays readable under every address: boards, cards,
  chats, people, trash. Every write into it exits 9 with `projectArchived`;
  only `chats mark-read` still works. Somebody who is no member gets exit 2 as
  ever and does not learn that it is archived.
- `messages create` is refused like every other write, also with a
  `--client-message-id` that was sent before the project was archived.
- A host whose Minka does not know archived projects yet makes `projects list
  --archived` exit 7 rather than list projects in use as archived.

## Boards

- A card carries `shortId`, such as `MNK-1A`, or null where its board shows no
  short identifiers. It is what people call a card by. The text views show it
  beside the title; it is no part of the title. It is no lasting address: it
  changes when the board changes its prefix, and the `id` never does.
- Wherever a command takes `<cardId>`, the short identifier works as well, in
  any case: `minka cards show mnk-1a`, `minka comments create MNK-1A -`,
  `minka cards move MNK-1A --column Doing --after MNK-2B`. The command looks
  the card up first and goes on with its `id`; answers and `breadcrumbs` name
  the `id`, which stays when a board changes its prefix.
- Exit 2 for a short identifier means that no card you can see goes by it: it
  does not exist, its board shows no short identifiers, the card is in the
  trash or in a project you are not in, or two of your organizations use the
  prefix. `cards restore` therefore needs the `id`, which `trash list` gives.
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
  `unreadCount` never counts your own messages, removed ones or lines an
  import brought. The same holds for a conversation.
- `messages list --styled` shows a message as Markdown when it carries markup,
  so a file in it is a Markdown image or link and a mention is `@Name`. The
  plain `bodyText` beside every message has neither.
- `messages list <chatId>` gives messages oldest first as `{id, seq,
  changeSeq, author, bodyHtml, bodyText, createdAt, editedAt, removed}`, the
  latest 50 by default, `--limit` up to 200.
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
- `messages edit` and `messages delete` only work on your own messages;
  somebody else's exits 2. Deleting twice succeeds.
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
  them in a line under the comment or the message.
- `comments react <cardId> <commentId> <content>` and `messages react <chatId>
  <messageId> <content>` leave a reaction, `comments unreact` and `messages
  unreact` take one away. A reaction is an emoji or at most 20 characters;
  longer exits 9 with `reactionTooLong`, empty with `reactionMissing`.
- Reacting the same way twice changes nothing and succeeds. Taking one away is
  not bound to whoever left it: it takes your own reaction of that text, and
  where you left none, the one somebody else left last. Taking away what is
  not there succeeds as well.
- A message of a private conversation takes a reaction the same way, under the
  ID of the conversation; somebody outside the two exits 2.
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

## Naming somebody in a text

- `people list <projectId>` and `conversations people <conversationId>` give
  every person a `kind` (`human`, `agent` or `bot`), a `principal` (the human
  an agent acts for, otherwise null) and a `mention: {handle, markup}`. An
  agent that stepped down is in no such list any more, however long its name
  stands on what it once wrote. `markup` is the
  `<action-text-attachment>` element that names them.
- Put the markup into a text unchanged and send it as HTML — Markdown cannot
  hold it:

```bash
MARKUP=$(minka people list <projectId> --quiet | jq -r '.[] | select(.name == "Anke") | .mention.markup')
printf '%s please have a look' "$MARKUP" | minka comments create <cardId> - --format html
```

- The person named is notified and becomes a subscriber of the card.
- A mention holds in the circle it was asked for: one of a project belongs in
  texts of that project, one of a conversation in that conversation. Anywhere
  else the server drops it while saving, without an error.
- `cards show --styled` and `messages list --styled` show a mention as `@Name`.

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

## Importing from Basecamp

- `import basecamp --project <id> --out <dir>` extracts projects of a Basecamp
  account into an import bundle, the directory Minka's applier reads foreign
  history from. It talks to Basecamp only, and only reads there; it needs no
  Minka host and no token.
- It runs the Basecamp CLI, which has to be installed and signed in. Without
  it the command exits 1 (`config`), without a sign-in 3; the `hint` names
  `basecamp auth login`. A project that Basecamp does not show exits 2.
- Exit 4 with "masks e-mail addresses" means the Basecamp identity that is
  signed in may not see the addresses of other people. Nothing was written and
  trying again does not help: tell the person that the run needs an identity
  of the account's own company, or an admin.
- `--project` takes the numeric ID of a Basecamp project and is repeated for
  several. Anything that is no number exits 1 before any request.
- `data.invalidAddresses` lists, by ID only, the people who come without an
  e-mail address, and so without a login, because the one Basecamp holds is
  none; `notice` says how many. The run still exits 0 and running it again
  changes nothing: only somebody with access to the account can repair the
  address there. Never look the address up to quote it.
- So far a bundle holds the structure, the boards, the message boards and the
  chats: the people the projects refer to (members and whoever a record
  names; `data.peopleLeftOut` counts the rest of the account's list, which
  holds more for an admin than for anybody else, the people written do not
  differ) with their
  avatars, the projects with their members, the card tables, chats and
  message boards that are switched on, of each card table the columns, the
  cards with the archived and the trashed ones, their steps, comments and
  boosts, of each message board its messages as posts, the archived and the
  trashed ones included, with their categories, comments and boosts, and of
  each chat its lines as messages with their boosts. `data.skippedTools`
  counts what was left out, per kind and whether it was switched on;
  `data.counts` is what the manifest says. The files of texts and chats do
  not come yet, only the references to them.
- A person comes with `retired: true` unless Basecamp calls them a user: a
  bot, somebody who was deleted and an outsider are authors and nothing more,
  whichever identity ran the import.
- `data.cardsByStatus` splits the cards, `data.cardsOnHold` counts those set
  aside within their column, `data.cardsLeftOut` the archived and trashed
  cards whose column is in no extracted board, `data.commentsTrashed` the
  comments left out because they are in the trash.
- `data.postsByStatus` splits the posts, `data.postsLeftOut` counts the
  archived and trashed posts of a message board that is not extracted,
  `data.postsWithEmptyBody` the posts that are a title alone. Basecamp hands
  out no drafts and tells of no pin, so every post comes with `pinnedAt`
  `null`.
- `data.messagesByType` splits the messages by what a line was in its chat:
  `text`, `richText`, `upload`, `integration`; `data.messagesWithEmptyBody`
  counts the lines somebody wrote that say nothing, which are left out. A
  line that was files alone becomes a body of references to them;
  `data.uploadLinesLeftOut` counts the lines of which Basecamp names no
  file, which are left out and which a `notice` counts as well. Lines
  somebody deleted and the lines Basecamp writes itself are not in its API.
- The messages are written in parts, `messages-0001.jsonl` and on, each with
  lines of one chat; a part ends once it holds 10,000 lines, where the second
  changes, so it may hold somewhat more. `data.parts.messages` and the
  manifest name them in their order. Only a bundle whose lines came to a
  single part carries `messages.jsonl` instead, and `data.parts` is missing.
- In the texts a mention and a file become a reference and the preview of a
  link the link: `data.filesInBodies` counts the files referred to,
  `data.filesDropped` the files Basecamp lists nothing for, of which only the
  caption stays and which a `notice` counts as well,
  `data.mentionsUnresolved` the mentions of somebody the account does not
  list any more, which stay a name, `data.linkPreviews` the previews that
  became links, `data.picturesLinked` the pictures at a foreign address that
  became a link to it; one whose address is none of the web leaves its title
  as text and is not counted. `data.longestBody` is the longest text per kind
  (`cards`, `comments`, `posts`, `messages`) in characters. None of them fails a run.
- A request that fails for a reason that may pass (network, rate limit, a
  5xx, 408 or 429 of Basecamp or of a gateway in front of it) is asked again
  by the command itself, up to four times with pauses of 2, 10 and 30
  seconds, and at most 100 times in one run. `data.requestsRetried` counts
  those further attempts, 0 when none was needed; it fails no run. A run that
  still ends on such a failure, or because Basecamp kept failing, is
  `retryable` (exit 6 for the network, else 7): nothing was written, run it
  again later.
- `--dry-run` walks Basecamp the same way, downloads and writes nothing, and
  answers with the same counts. `--out` is not needed then.
- The bundle is built next to `--out` and takes its place at the end: a failed
  run leaves what was there. An earlier bundle of Basecamp is replaced as a
  whole; a directory that holds anything else is refused with exit 1, by
  `--dry-run --out <dir>` as well.
- One run at a time works on an `--out`: a second one exits 1 (`config`) and
  names the lock the first holds. A run that was killed holds none. `--out`
  must not be a symbolic link.
- A file that cannot be fetched does not end the run. The bundle is written
  without it and can be applied, and the command fails at the end: exit 6 and
  `retryable` when running again may bring the file, else exit 7. The report
  is `data` of the error envelope then, and `data.missingFiles` names each
  file with its owner and the `reason`. Do not treat such a run as lost.
- A bundle holds personal data: names, e-mail addresses, pictures. Write it
  outside of any repository, do not quote from it, and delete it once it has
  been applied.

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
