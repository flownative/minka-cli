---
name: minka
description: >
  Work with a Minka instance through the minka CLI: list projects and boards,
  read a board with its columns and cards, show a card with description and
  comments, create and edit cards, move, hold and complete them, comment on them,
  read and write chat messages, and say which cards you are working on right
  now.
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
  neither is refused with exit 1 before any request. An ID may be written in
  capitals; it is sent in lowercase, the form Minka keeps it in.
- Retry only when `retryable` is true, which only happens for reads. After a
  failed write, read the card back before trying again: the change may have
  reached the server. A 404 (`not_found`) means the thing does not exist,
  belongs to a project the token's person is not in, or its project is in the
  trash.
- Exit 9 with `meta.apiCode` `projectArchived` means the project is archived:
  it can be read, nothing can be written, and trying again does not help. Tell
  the person; putting it back in use is done in Minka on the web.
- Say what you have in hand: `minka work start <card>` when you begin on a
  card and `minka work stop <card>` when you are done with it. It shows on the
  board only while `minka heartbeat` keeps reporting it, which the mod of
  `minka setup` or `minka hooks install` does once and for all.
- Take a card up from its back: `minka cards notes show <card>` before you
  begin, because whoever was on it before left the state of the work there,
  and `minka cards notes set <card> - --expect <updatedAt>` at every milestone,
  so that the next session can take the card over. Comments are the other
  thing — the conversation with people.
- Name cards with `#MNK-1` or their URL, other entries with their URL, never
  only as text. Minka turns such a name into a reference that links both ways;
  a title alone links nothing.
- Read before you answer: comment with
  `minka comments create <card> - --if-latest <newest commentId|none>`, the
  newest comment you read. Where somebody commented since, nothing is written
  and it exits 11 (`unread`) with those comments in `data`: read them, then
  decide again. `minka comments unread <card>` shows what this machine has not
  shown yet; `minka config set comments.requireRead true` makes every comment
  without `--if-latest` wait for that (`--force` writes anyway).
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
| 11 | `unread` | a comment was not written: the card has comments you have not read, in `data` |

Validation codes in `meta.apiCode`: `cardTitleMissing`, `cardTitleTooLong`,
`cardTitleHasControlCharacters` (a title is one line: no line breaks or tabs),
`cardDescriptionTooLong`, `cardDescriptionNotReadable`, `commentMissing`,
`commentTooLong`, `commentHasControlCharacters` (line breaks and tabs are
fine), `commentNotReadable`, `cardDueDateNotADate`, `personNotAProjectMember`, `personIdMissing`,
`stepMissing`, `stepTooLong`, `stepHasControlCharacters`, `stepCompletedMissing`,
`stepDueDateNotADate`, `cardArchived`,
`chatMessageMissing`, `chatMessageTooLong`, `chatMessageHasControlCharacters`,
`chatMessageNotReadable`, `chatMessageDraftNotAnIdentifier`, `chatMessagesLimitNotANumber`,
`chatReadSeqMissing`, `nothingToChange`, `columnIdMissing`, `onHoldEnabledMissing`,
`columnHasNoOnHoldArea`, `systemColumnHasNoOnHoldArea`, `malformedBody`,
`unsupportedMediaType`, `projectArchived`.

That is not all of them: Minka has a code for every refusal of every endpoint,
far more than the CLI can meet, and the list above holds the ones that come up
most. Codes of their own stand where their subject is described — projects,
cards on hold, people and assignments, steps, the working notes on the back of
a card, due dates, reactions and uploads. Read the code you got, do not expect
it in a list.

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
  token, offers to install this skill, and installs the mod (`--no-mod` leaves
  it out); `data.mod.state` is `installed`, `current`, `written` (Claude Code
  was not told, `data.mod.reason` says why), `not installed` (the mod could not
  be written) or `skipped`. Without a terminal it exits 1; take `minka auth
  login --host <url> --with-token < file`, `minka skill install` and `minka
  hooks install` instead.
- `minka auth logout --host <url>`: forget the token of a host. The token stays
  valid until it is revoked on the server.
- `minka config get <key> --json` and `minka config set <key> <value> --json`:
  the settings of this machine, so far `comments.requireRead`.

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
minka projects edit <projectId> [--name "New name"] [--description - < about.txt] --json
minka boards list <projectId> --json
minka boards show <boardId> --json
minka boards archive <boardId> [--before <cardId>] [--limit <n>] [--all] --json
minka cards mine --json
minka cards show <cardId> --json
minka cards create --board <boardId> --title "Title" [--column <column>] [--assignee <person>]... [--due YYYY-MM-DD] [--description - < body.md] --json
minka cards edit <cardId> [--title "New"] [--due YYYY-MM-DD] [--description - < body.md] --json
minka cards move <cardId> --column <column> [--on-hold] [--top | --bottom | --after <cardId> | --before <cardId>] --json
minka cards done <cardId> --json
minka cards archive <cardId> --json
minka cards unarchive <cardId> --json
minka cards delete <cardId> --json
minka cards restore <cardId> --json
minka columns create --board <boardId> --title "Review" --json
minka columns edit <column> [--board <boardId>] [--title "New"] [--color <colour> | --no-color] --json
minka columns move <column> --board <boardId> (--first | --after <column> | --before <column>) --json
minka columns delete <column> [--board <boardId>] --json
minka columns on-hold <column> [--board <boardId>] [--off] --json
minka trash list <projectId> --json
minka people list <projectId> --json
minka conversations people <conversationId> --json
minka cards assign <cardId> <person> --json
minka cards unassign <cardId> <person> --json
minka cards subscribe <cardId> <person> --json
minka cards unsubscribe <cardId> <person> --json
minka cards notes show <cardId> --json
minka cards notes set <cardId> - --expect <updatedAt|none> --json < notes.md
minka steps list <cardId> --json
minka steps create <cardId> "Text" --json
minka steps edit <cardId> <step> --assignee me --due 2026-10-09 --json
minka steps complete <cardId> <step> --json
minka steps reopen <cardId> <step> --json
minka steps delete <cardId> <step> --json
minka comments list <cardId> --json
minka comments unread <cardId> --json
minka comments create <cardId> - [--if-latest <commentId|none> | --force] --json < comment.txt
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
minka uploads download <blobId> [--variant original|display] [-o <path>|-o -] [--force] --json
minka docs tools <projectId> --json
minka docs list <toolId> [--folder <folderId>] --json
minka docs show <documentId> --json
minka docs create --tool <toolId> --title "Title" --audience everybody|chosen|nobody [--notify <person>]... [--folder <folderId>] [--body - < body.md] --json
minka docs edit <documentId> [--title "New"] [--body - < body.md] (--expect <updatedAt> | --force) --json
minka docs versions list <documentId> --json
minka docs versions show <documentId> <versionId> --json
minka docs uploads show <uploadId> --json
minka docs uploads create <file> --tool <toolId> [--folder <folderId>] [--title "Title"] [--description - < about.md] --json
minka docs uploads create --blob <blobId> --tool <toolId> [--folder <folderId>] [--title "Title"] --json
minka docs uploads edit <uploadId> [--title "New"] [--description - < about.md] (--expect <updatedAt> | --force) --json
minka docs uploads versions list <uploadId> --json
minka docs uploads versions create <uploadId> (<file> | --blob <blobId>) [--expect <updatedAt>] --json
minka docs comments create (--document <documentId> | --upload <uploadId>) - --json < comment.md
minka docs comments edit (--document <documentId> | --upload <uploadId>) <commentId> - [--format html] --json < comment.md
minka docs comments delete (--document <documentId> | --upload <uploadId>) <commentId> --json
minka docs comments react (--document <documentId> | --upload <uploadId>) <commentId> 👍 --json
minka docs comments unreact (--document <documentId> | --upload <uploadId>) <commentId> 👍 --json
minka todolists tools <projectId> --json
minka todolists list <toolId> [--archived] --json
minka todolists show <listId> --json
minka todolists create --tool <toolId> --title "Title" [--description - < body.md] --json
minka todolists templates --json
minka todolists start --tool <toolId> --template <templateId> [--title "Title"] --json
minka todolists edit <listId> [--title "New"] [--description - < body.md] --json
minka todolists archive <listId> --json
minka todolists unarchive <listId> --json
minka todolists delete <listId> --json
minka todolists comments list <listId> --json
minka todolists comments create <listId> - --json < comment.md
minka todolists comments edit <listId> <commentId> - [--format html] --json < comment.md
minka todolists comments delete <listId> <commentId> --json
minka todolists comments react <listId> <commentId> 👍 --json
minka todolists comments unreact <listId> <commentId> 👍 --json
minka todogroups create --list <listId> --title "Title" [--color <colour>] --json
minka todogroups edit <groupId> [--title "New"] [--color <colour>] --json
minka todogroups move <groupId> (--top | --after <groupId> | --before <groupId>) --json
minka todogroups delete <groupId> --json
minka todos mine [--limit <1-100>] [--after <todoId>] --json
minka todos show <todoId> --json
minka todos completed <listId> [--limit <n>] [--before <next>] --json
minka todos create --list <listId> --title "Title" [--group <groupId>] [--notes - < notes.md] [--starts-on <day>] [--due <day>] [--assignee <person>]... [--notify-when-done <person>]... --json
minka todos edit <todoId> [--title "New"] [--notes - < notes.md] [--starts-on <day>|""] [--due <day>|""] [--assignee <person>]... [--no-assignees] [--notify-when-done <person>]... [--no-notify-when-done] --json
minka todos complete <todoId> --json
minka todos reopen <todoId> --json
minka todos move <todoId> (--top | --after <todoId> | --before <todoId>) [--list <listId>] [--group <groupId> | --no-group] --json
minka todos delete <todoId> --json
minka todos subtasks list <todoId> --json
minka todos subtasks create <todoId> "Text" [--assignee <person>] [--due <day>] --json
minka todos subtasks edit <todoId> <subtask> [--title "New"] [--assignee <person>|""] [--due <day>|""] --json
minka todos subtasks complete <todoId> <subtask> --json
minka todos subtasks reopen <todoId> <subtask> --json
minka todos subtasks move <todoId> <subtask> (--top | --after <subtask> | --before <subtask>) --json
minka todos subtasks delete <todoId> <subtask> --json
minka todos repeat <todoId> [--rhythm <rhythm>] [--weekdays <days>] [--interval <n>] [--day <n>] [--weekday <day>] [--instance <1-4|last>] [--month <n>] [--ends-on <day>] [--lead-days <n>] --json
minka todos series list <listId> --json
minka todos series show <seriesId> --json
minka todos series edit <seriesId> (--expect <updatedAt> | --force) [--title "New"] [--notes - < notes.md] [schedule flags] [--starts-on <day>] [--ends-on <day>|""] [--lead-days <n>] [--assignee <person>]... [--no-assignees] [--notify-when-done <person>]... [--no-notify-when-done] --json
minka todos series stop <seriesId> --json
minka todos series resume <seriesId> --json
minka todos series show-now <seriesId> --json
minka todos comments list <todoId> --json
minka todos comments create <todoId> - --json < comment.md
minka todos comments edit <todoId> <commentId> - [--format html] --json < comment.md
minka todos comments delete <todoId> <commentId> --json
minka todos comments react <todoId> <commentId> 👍 --json
minka todos comments unreact <todoId> <commentId> 👍 --json
minka checkins tools <projectId> --json
minka checkins list <toolId> --json
minka checkins show <questionId> --json
minka checkins create --tool <toolId> --title "Question?" --rhythm daily|weekly|everyOtherWeek|monthly --weekday <day>... --time HH:MM --ask <person>... --json
minka checkins edit <questionId> [--title "New"] [--rhythm <rhythm>] [--weekday <day>]... [--time HH:MM] [--ask <person>]... --json
minka checkins pause <questionId> --json
minka checkins resume <questionId> --json
minka checkins reminders --json
minka checkins answers list <questionId> [--person <person>] [--on <day>] [--limit <1-100>] [--before <answerId>] --json
minka checkins answers show <answerId> --json
minka checkins answers create <questionId> - [--asked-for <day>] [--format html] --json < answer.md
minka checkins answers edit <answerId> - [--format html] --json < answer.md
minka posts tools <projectId> --json
minka posts categories <projectId> --json
minka posts list <toolId> [--limit <n>] [--before <next>] --json
minka posts list <toolId> --archived --json
minka posts show <postId> --json
minka posts create --tool <toolId> --title "Title" --body - --audience everybody|chosen|nobody [--notify <person>]... [--category <categoryId>] --json < post.md
minka posts edit <postId> [--title "New"] [--body - < post.md] [--category <categoryId> | --no-category] --json
minka posts pin <postId> --json
minka posts unpin <postId> --json
minka posts archive <postId> --json
minka posts unarchive <postId> --json
minka posts delete <postId> --json
minka posts comments list <postId> --json
minka posts comments create <postId> - --json < comment.md
minka posts comments edit <postId> <commentId> - [--format html] --json < comment.md
minka posts comments delete <postId> <commentId> --json
minka posts comments react <postId> <commentId> 👍 --json
minka posts comments unreact <postId> <commentId> 👍 --json
minka events tools <projectId> --json
minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD --json
minka events mine --from YYYY-MM-DD --to YYYY-MM-DD --json
minka events settings <toolId> --content eventsAndDatedItems|eventsOnly --json
minka events show <eventId> --json
minka events create --tool <toolId> --title "Title" (--starts-at <moment> --ends-at <moment> | --first-day YYYY-MM-DD [--last-day YYYY-MM-DD]) [--repeat] [--rhythm <rhythm> ...] [--time-zone <zone>] [--participant <person>]... [--description - < event.md] [--join-url <url>] [--highlighted] --json
minka events edit <eventId> [--title "New"] [<time>] [--repeat [<details>] | --no-repeat] [--participant <person>... | --no-participants] [--description -] [--join-url <url>] [--highlighted=false] --json
minka events delete <eventId> --json
minka events comments list <eventId> --json
minka events comments create <eventId> - --json < comment.md
minka events comments edit <eventId> <commentId> - [--format html] --json < comment.md
minka events comments delete <eventId> <commentId> --json
minka events comments react <eventId> <commentId> 👍 --json
minka events comments unreact <eventId> <commentId> 👍 --json
minka work start <card> [--session <id>] --json
minka work stop [<card>] [--all] [--session <id>] --json
minka work list [--session <id>] --json
minka heartbeat [--session <id>] --json
minka hooks install --json
minka hooks status --json
minka import basecamp [--project <basecampProjectId>... | --active-only] (--out <dir> | --dry-run) [--calendars] [--upload-versions] --json
```

`<column>` is a column ID, a type (`triage`, `not_now`, `done`) or a title
such as `Doing`.

## Your own cards

- `cards mine` lists the cards the account of the token is assigned to, across
  every project it is a member of, as a bare array of the same cards
  `cards show` gives. Each carries `project {id, name}` and `board {id, title}`
  besides `projectId` and `boardId`.
- Cards in Done, cards completed in another column, cards in the trash, cards
  in the archive of their board, cards of an archived project and cards of a
  project that was left are not among them, so the list is what is still to
  do. A card that is not in the list is therefore not necessarily done: it may
  have been archived.
- The order is the order of the page: projects by name, then by `dueOn` with
  the overdue ones first and the ones without a date last.
- It is the cheapest way to answer "what am I on?": one request instead of a
  board per project.
- `cards delete` puts a card in the trash and `cards restore <cardId>` takes it
  back out, on top of the column it hung in. Only a card that is in the trash
  can be restored: a second restore, a card that was never trashed and a card
  whose board or project is in the trash all exit 2.
- A card in the archive of its board goes into the trash from there, and a
  restore puts it back on the board, not into the archive: the restored card
  says `archived: false`.
- `trash list <projectId>` says what was taken out of a project, the newest
  first and at most 200 (a `notice` says when there are more). Every entry is
  `{kind, id, title, toolType, board, column, trashedAt, trashedBy,
  restorable, blockedBy, appUrl}`.
- `kind` is `tool`, `column`, `card`, `post`, `todoList`, `todo`, `todoGroup`,
  `checkinQuestion`, `checkinAnswer`, `forward`, `folder`, `document` or
  `upload`, and Minka may name further kinds.
- `board` is a board only for a `card` and a `column`. For every other kind the
  same field carries the tool the thing hung in, which the text view marks as a
  tool; `column` is there for a card alone.
- `restorable` says whether `cards restore` takes the entry, which only a card
  is. `blockedBy` names what has to come back first: `board`, `column`, `tool`,
  `todoList`, `todoGroup`, `question`, `folder` or `archivedFolder` — the last
  one lies in an archive, not in the trash. Everything that is no card comes
  back in the web only.

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
- `projects edit <projectId>` renames a project (`--name`) or changes its
  description (`--description`, `-` for stdin, `""` to clear it); at least one
  of the two, or exit 1 before any request. What is not given stays, even if
  somebody changed it meanwhile. The description is plain text: no Markdown is
  converted, no mention or reference is made. The answer is the project as
  `projects list` gives it. An empty name exits 9 with `projectNameMissing`;
  `projectNameTooLong` (200 characters), `projectNameHasControlCharacters` and
  `projectDescriptionTooLong` (2000) are the other refusals. Only the team may
  edit: a client of the project gets exit 2 like an outsider. A host whose
  Minka is too old for the command also exits 2, with a hint that says so.
  Rename a project only when you were asked to.

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
- Done lists only the 25 cards completed last; `cardCount` counts all of
  them. Read an older card by its ID.

## Columns

- A column carries `color`: one of `white`, `red`, `orange`, `yellow`,
  `green`, `blue`, `aqua`, `purple`, `gray`, `pink` and `brown` for a working
  column, always `green` for Done, null for Triage and Not now.
- `columns create --board <boardId> --title <title>` adds a working column
  behind the others, in the colour the board holds least of. It takes neither
  a place nor a colour: move and colour it afterwards.
- `columns edit <column> [--title] [--color | --no-color]` changes what is
  given, at least one of them. Triage, Not now and Done take a title of their
  own too, and `--title ""` gives them back the name of their type; a working
  column exits 9 with `columnTitleMissing` on it. `--no-color` takes the colour
  away: the column reads `white` then, which is how a column without a colour
  looks. Any other word for `--color` exits 9 with `columnColorUnknown`, and a
  colour or `--no-color` for Triage, Not now or Done exits 9 with
  `systemColumnHasNoColor` and leaves the title as it was too.
  `columnTitleTooLong` (200 characters) and `columnTitleHasControlCharacters`
  are the other refusals.
- A Minka that does not take a colour away yet answers `--no-color` alone
  with exit 9 and `nothingToChange`, and `--no-color` with `--title` with exit
  7: the title was written, the colour stayed, and `data` is the column.
  `--color white` gives the same look on any Minka.
- `columns move <column> --board <boardId>` with exactly one of `--first`,
  `--after <column>` and `--before <column>` places a working column among the
  working columns. The neighbour has to be another working column of the same
  board, or the command exits 1 before anything moves: Minka would put the
  column first without a word next to a system column or itself. Triage, Not
  now and Done exit 9 with `systemColumnCannotMove`.
- `columns delete <column>` puts a working column into the trash of its
  project, and its cards go with it: the cards standing in it cannot be read,
  moved or restored until the column is back, and only the web brings it back.
  A card of the column in the archive of the board stays readable. `trash list`
  names it with `kind` `column`. Triage, Not now and Done exit 9 with
  `systemColumnCannotBeRemoved`; deleting again exits 2.
- Without `--board`, `<column>` has to be its ID; with it, a type or a title
  works as well. Only the team changes columns: a client of the project gets
  exit 2 like an outsider, whatever the write. An archived project exits 9
  with `projectArchived`, a write that would change nothing included.

## Cards on hold

- A working column can have an area for the cards that are waiting. Triage,
  Not now and Done cannot. A card on hold stays in its column, apart from the
  rest and in an order of its own.
- A column carries `onHoldEnabled`, `onHoldCardCount` and `onHoldCards`, in
  the order they stand on hold. `cards` and `cardCount` leave the cards on
  hold out. A card carries `onHold`.
- `columns on-hold <column>` switches the area on, `--off` switches it off and
  hands its cards back to the end of the column, in the order they stood on
  hold. Without `--board`, `<column>` has to be its ID. A system column exits
  9 with `meta.apiCode` `systemColumnHasNoOnHoldArea`, on and off alike.
- `cards move <cardId> --column <column> --on-hold` puts a card on hold; the
  placement flags then place it among the cards on hold. A column of the board
  that has no such area exits 9 with `meta.apiCode` `columnHasNoOnHoldArea`,
  Triage, Not now and Done among them: on a move that code is the answer, not
  `systemColumnHasNoOnHoldArea`. A move without `--on-hold` takes a card off
  hold, also within its own column.
- A card cannot be created on hold, by design: it comes into being in a
  column and goes on hold by a move. A card restored
  from the trash comes back off hold.

## The archive of a board

- Every card carries `archived`. It is true for a card in the archive of its
  board: the card stands in no column, it can be read, and every write on it
  exits 9 with `meta.apiCode` `cardArchived`.
- Such a card still carries a `column`, and that is the column it was taken
  from, which may have been deleted from the board since. The text view of
  `cards show` therefore prints `Archived: yes, was in <column>` and no
  `Column:` line, and the summary names the archive of the board instead of a
  column.
- A card in the archive is in no list of the board: `boards show` shows what
  stands in the columns, and `cards mine` leaves it out. `boards archive
  <boardId>` is the one list it is in; read the card itself by its `id`.
- `cards archive <cardId>` puts a card away, `cards unarchive <cardId>` brings
  it back. Putting away a card that lies in the archive exits 9 with
  `cardArchived`, bringing back one that stands on its board exits 9 with
  `cardNotArchived`, and an archived project exits 9 with `projectArchived`
  before either. A card in the **trash** exits 2 (`not_found`) on both, and
  comes back with `cards restore` instead — the trash is not the archive.
- A card brought back lands at the top of the column it was taken from, or in
  the first working column where that column is gone from the board, or in
  Triage on a board that has none; the answer names the column. It comes back
  **off hold**, whatever it was when it was put away, which nothing in the
  answer says: the command says it in a `notice`.
- The board leaves a card of an archive out of the set an agent says it is
  working on, so a card in hand drops out of `work list` with the next signal.
  `cards archive` does not take it out by itself.

### Reading the archive

- `boards archive <boardId> [--before <cardId>] [--limit <n>] [--all]` lists the
  cards a board has put away, the one put away last first. Each carries `id`,
  `shortId`, `title`, `archivedAt`, `archivedBy` and `appUrl` — and nothing
  else: the card itself is read with `cards show <cardId>`.
- A page holds 50 cards by default and **at most 100**. A larger `--limit` is
  refused here with exit 1 (`usage`), and that refusal is a safeguard, not a
  restriction: the server cuts a larger page down to 100 without saying so, and
  the only sign its answer carries for the end of the archive is a page shorter
  than asked for. Asking for 500 would therefore hand you 100 cards that read
  like the whole archive. Do not work around it by asking the API directly.
- `--before <cardId>` gives the page behind that card. Take it from the
  `breadcrumbs` of a page rather than working it out; `--before` needs the `id`,
  not the short identifier. A card that no longer lies in the archive of that
  board exits 9 with `meta.apiCode` `archivedCardsBeforeUnknown`: somebody took
  it back onto the board or into the trash. That is no end of the list — read
  the archive again from the beginning.
- `--all` walks every page to the end in one run and works around the weakness
  above by itself: it keeps the ids of the whole last page and falls back through
  them when the one it named is gone, it settles an empty answer behind a full
  page by reading behind the card before the cursor rather than taking it for the
  end, and it lists every card once. A page that carries the run no further ends
  it with exit 7 (`api_error`) and what it read.
- **What a run of `--all` promises:** every card that lay in the archive for the
  whole run appears exactly once, under one reservation: the run checks the end of
  the list by asking behind another card of the last page, and where no card of
  that page is left to ask behind, an answer that carried nothing is all it has
  for the end. A card that left the archive in that moment reads the same, and
  cards behind it were not read. Such a run exits 0 and says so in its `notice`;
  do not report its result as the whole archive. A page of one card is in that
  place from the first request, a larger page where the fallback has used up the
  cards of the page it last read. **What it does not promise:** the archive at
  one moment. A card put away during the run lands on the first page, which the
  run has already passed, and can be missed; a card taken out of the archive
  during the run vanishes from the result with no trace, and the API says
  nothing about either. A run of more than one page says this in its `notice`,
  together with how many pages it read, how many cards came twice and whether it
  had to begin again. Do not report the result of a multi-page run as a count of
  what the archive holds right now.
- A run that ends on `archivedCardsBeforeUnknown` still hands over what it read:
  the error envelope carries that as `data` with the notices, and the exit code
  9 says that something is missing from it.

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

- A card read by itself carries `steps` as `{id, title, completed, assignees,
  dueOn}` in the order it shows them; `assignees` in the order they were put
  on, somebody who left the project included, `dueOn` as `YYYY-MM-DD` or null. The board carries only `stepCount` and
  `completedStepCount` per card; read the card for the steps themselves.
- `<step>` is a step ID, its position starting at 1, or its text
  (case-insensitive; a number always means a position; a text on two steps
  gives exit 8 with the positions).
- `steps create` adds a step at the end. A step is one line of at most 500
  characters: `stepMissing`, `stepTooLong` and `stepHasControlCharacters` exit 9.
  It comes without people and without a due day; give them with `steps edit`.
- `steps edit <cardId> <step>` changes the people and the due day in one write;
  what is not given stays. `--assignee` (repeatable) is the WHOLE set: whoever
  is on the step and not named again is taken off; `--no-assignees` takes
  everybody off. `--due YYYY-MM-DD`, `--due ""` takes the day off. A person is
  an ID, `me`, an e-mail address or a name. Only a member of the project who
  sees the board can be on a step: anybody else refuses the whole write with
  `personNotAProjectMember` (exit 9), and nothing is written. A day that is no
  day is `stepDueDateNotADate`, a card in the archive `cardArchived` (exit 9).
  A Minka without the people and the day of a step answers
  `stepCompletedMissing`, and the hint says so.
- Completing a completed step, reopening an open one and deleting a step that
  is gone all succeed. Deleting does not ask.
- Renaming and reordering steps are not possible through the API yet.

## Working notes

The back of a card is where the session working on it keeps the state of that
work, instead of in a chat or in a file of its own: what the card is scoped to,
the decisions taken, the packages, branches and pull requests with what became
of them, the next step, the open questions. It is the handover — another
session, on another machine, takes the card over by reading it. Comments are
the other thing: the conversation with people.

The back is one text. It is light Markdown, stored and printed as it is, never
converted, and writing it notifies nobody and leaves no line in the timeline.

How to keep it:

- **Replace, do not append.** The back is a state, not a log. Rewrite what
  changed and drop what no longer holds. What happened is in the comments, in
  Git and in the pull requests.
- **Keep it short.** Whoever takes the card over reads all of it. Say where the
  work stands, not how it got there.
- **Write at the milestones**, not as you go: work taken up, a review come
  back, a pull request opened, something merged, a decision taken, a question
  you cannot answer yourself. In between, work.
- **Read the back before you begin,** and never write without having read.
- **`show` first, then `set` with `--expect <updatedAt>`.** The guard is what
  makes a write that would wipe out somebody else's notes fail instead. With
  `--json`, `show` hands you the `set` to run with `--expect` filled in, in its
  `breadcrumbs`; a `set` that went through hands back the same with the time it
  has just written, so a second write in the same session needs no second read.
- **`--force` is how one session writes over another's notes unasked.** It is
  not the way past a refusal: a refusal means somebody wrote while you were
  writing, and their text is in `data`. Merge yours into their `body` and send
  it again with their `updatedAt`. Take `--force` only where the person you work
  for has asked for it — whether anybody else writes these notes is not
  something you can know.

What the two commands do:

- `cards notes show <cardId>` gives `{cardId, body, updatedAt, updatedBy,
  appUrl}`. Notes nobody has written have `body` `""` and `updatedAt` and
  `updatedBy` null. The card itself does not say whether it has notes.
- `cards notes set <cardId> -` replaces the notes as a whole with stdin; `""`
  clears them. It needs `--expect <updatedAt>` — the `updatedAt` you read, or
  `none` for notes nobody had written yet — or `--force`, and the two exclude
  each other (exit 1). Notes somebody changed since the time `--expect` names
  are left as they stand: exit 9, `meta.apiCode` `notesChanged`, and `data`
  holds the notes as they stand now. Sending the text that stands already is no
  conflict.
- At most 100000 characters, counted as Minka stores them; a longer text exits
  9 with `cardNotesTooLong` before it is sent. Control characters other than
  line breaks and tabs exit 9 with `cardNotesHasControlCharacters`.
- `--expect` takes the time in any form Minka reads it: up to six digits of a
  second, under `Z` or an offset. It has to be the stored time exactly, so
  send back the `updatedAt` you read and nothing built by hand.
- In text output `notes show` prints the body alone; who changed it last and
  the `--expect` to write over it go to standard error, so that the notes can
  be piped into a file and sent back unchanged.
- A card in its board's archive can be read, not written: exit 9 with
  `cardArchived`.

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
- **HTML that carries no element Minka serves is not refused.** It is escaped
  and kept as the characters it consists of, with the same answer as any other,
  so Markdown sent with `--description-format html` or `--format html` stands
  there afterwards as `# Heading`. The command says so in a `notice`, and
  nothing else will.
- Every text comes twice: stored (`description`, `body`, `bodyHtml`) and as
  its source (`descriptionSource`, `bodySource`). Files and mentions stand in
  the source as `<action-text-attachment>` elements with fresh signatures and
  their `url`, `filename` and `content-type`. `cards show --styled` shows a
  picture as a Markdown image, a file as a link and a mention as `@Name`.
- Markdown cannot hold files or mentions, nor `<minka-embed>` for an address
  Minka drew as a card; a `<minka-ref>` it holds only as its address (see
  "Pointing at a card or another record"). To change a description that has any
  of them, take `descriptionSource` from `minka cards show <cardId> --quiet`,
  change it and send it back with `cards edit --description - --description-format html`.
  A new description that would drop them is refused (exit 1) unless
  `--drop-attachments` is given — the refusal calls them all attachments,
  after the flag. This holds for every text below as well. Where Minka refuses
  the write anyway — an archived project, a card, post, document, list or
  to-do in an archive, somebody else's comment — there is no exit 1: Minka's
  own refusal comes, and `--drop-attachments` would change nothing about it.
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
  longer exits 9 with `reactionTooLong`, empty with `reactionMissing`, one with a
  control character in it with `reactionHasControlCharacters`.
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

## Pointing at a card or another record

- Write `#MNK-1` for a card with a short identifier, or paste the address of a
  card, a post, a chat, a chat line (a chat's address ending in
  `#chat-message-…`), a to-do, a to-do list, a document, an upload or a
  check-in question of the same Minka — in the words or as `<address>`. A line
  of a private conversation counts as well
  (`/messages/<id>#chat-message-…`); the conversation's address alone
  (`/messages/<id>`) is no reference.
  On saving, Minka turns each into a `<minka-ref>` for every way in: Markdown,
  HTML, comments, messages. `MNK-1` without `#`, anything in code, a link with
  words of its own (`[the card](…)`), an address of another host and a target
  you cannot see stay as written.
- Every answer adds what the reader needs to the element: `title`, `short-id`
  for a card that has one, and `href`. `--json` hands the text over exactly as
  served; sending it back as HTML is safe, because saving keeps only `kind` and
  `id`.
- `--styled` shows a visible reference as its address, `<https://…>`, for
  every kind, so the text stays editable: sent back as Markdown, a link showing
  its own address becomes the reference again. A card's short identifier reads
  in `/go/card/MNK-1`; the title is not shown — take it from `--json`, where
  every reference carries `title` and `short-id`. A target you cannot open, one
  in the trash or one that does not exist reads `(a card you cannot open)`, and
  likewise a post, chat, chat message, to-do, to-do list, document, file
  (`upload`) or question; any other kind reads `(a reference you cannot open)`.
- The guard against dropping counts references: a new text keeps a visible one
  when it holds its address (in the words or as `<address>`), `#MNK-1` for a
  card, or the element itself. A hidden one is kept only by the element, so an
  edit of its view is refused (exit 1) unless `--drop-attachments` is given;
  send the source back as HTML instead.

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
SOURCE=$(minka uploads create shot.png --project <projectId> --quiet | jq -r .attachmentSource)
minka cards show <cardId> --quiet | jq -r .descriptionSource > text.html
printf '\n%s' "$SOURCE" >> text.html
minka cards edit <cardId> --description - --description-format html < text.html
```

- The attachment belongs to the circle it went into: a file of a project hangs
  in the texts of that project, a file of a conversation in the messages of
  that conversation. Anywhere else the server drops it while saving, without an
  error, so read the text back if it matters.
- A file is kept for good only once a text or an upload of a Docs & Files tool
  carries it. One that nothing carries is kept for **at least** seven days and
  taken away some time after that — a floor, not a deadline you can count on.
- The `id` of the answer is what `docs uploads create --blob <blobId>` takes
  to let the file stand on its own in a Docs & Files tool. A file goes into one
  place only: a text or an upload.
- The bytes of a picture are not the ones that went in: Minka turns a picture
  the right way up, strips it of everything but its colour profile — the place
  a camera wrote into it among the rest — and encodes it again, so `size`, and
  the `byteSize` of a download later on, say what Minka keeps and not what the
  file weighs here. What it shows is the same picture, though a JPEG is encoded
  afresh and is therefore not the same to the last pixel. A GIF, an SVG and
  everything that is no picture keep their bytes.
- The media type comes from the name of the file, and from its first bytes
  where the name says nothing. `--content-type` gives it instead. The server
  reads the bytes itself and keeps the type they show. It refuses a file only
  where the bytes and the type disagree **and one of the two is a picture it
  shows** (AVIF, GIF, JPEG, PNG, WebP): exit 9 with `meta.apiCode`
  `contentTypeMismatch`. An SVG counts as a document. Otherwise the type the
  bytes show is kept without a word, whatever was sent.
- At most 100 MiB. A larger file exits 1 before it travels, and a server that
  refuses one exits 9 with `tooLarge`.
- `--name` gives the file another name in Minka than it has here.

## Fetching a file back

- Every text says which files hang in it. `cards show` carries `attachments`
  for the description and one list per comment, `messages list` one per
  message: `{id, filename, contentType, byteSize, isImage, url}`, the file as
  Minka keeps it. A text without files carries an empty list. The board view
  of a card carries none; read the card itself.
- `uploads download <blobId>` fetches the bytes. `<blobId>` is the `id` of an
  attachment, so reading a card is enough to fetch its picture — nothing has
  to be taken out of the text.
- Without `-o` the file is written into the current directory under the name
  it has in Minka. A file of that name that is already there is not touched:
  exit 1, and `--force` replaces it. `-o <path>` names a target,
  `-o -` puts the bytes on standard output and the word about them on
  standard error; `--json` and `--quiet` are refused together with `-o -`,
  because standard output cannot carry both.
- `data` is `{file, variant, path, byteSize}`. `file` is the answer of the API
  about the file, `path` where the bytes were written, `byteSize` how many
  arrived.
- `--variant` asks for one rendering. `original` is the whole file and is
  always there. `display` is the smaller rendering a large picture got — only
  `variants` says whether a file has one, so never assume it. There is no
  command that asks for `variants` alone: they stand in `data.file.variants`
  of a download that worked, and a `--variant` the file has not got exits 1
  before any byte travels, with `hint` naming the ones it does have. Fetching
  the original and reading `variants` off it is the way to find out.
- A file of a project or a conversation you are not in exits 2, the same
  answer an identifier that names nothing gives: the two cannot be told apart.
- What arrives has to weigh what the file says it weighs. Anything else exits
  7 and nothing is kept.

## Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body; an **upload** is a file standing on its own,
with a description. Both take comments, and the comments take reactions.

### Reading

- `docs tools <projectId>` lists the tools of a project; a project can have
  more than one.
- `docs list <toolId>` lists what stands at the top, `--folder <folderId>`
  what stands in a folder. It reaches **one folder deep**: to go further, call
  it again with a folder of the answer. Nothing is paged and nothing is cut,
  so a short list is the whole list.
- The order is the one the web shows: not alphabetical, not by date. Every
  document carries a `lead`, the first 240 characters of its body without
  markup, cut where the count runs out and not at a word.
- Drafts and entries in the archive are not listed, and neither is a folder in
  the archive. The inside of an archived folder **can** be listed by naming it,
  and then the answer and every line of it says `archived: true`.
- `docs show <documentId>` gives the document with its body, its files and its
  comments. `docs uploads show <uploadId>` gives an upload the same way, with
  its `description` and `descriptionSource`. A
  **draft is at no address**, its author included: exit 2.
- The bytes of an upload are fetched with `uploads download <blobId>`, where
  `<blobId>` is `file.id` of the upload — not the upload's own ID, which names
  the entry.

### Writing a document

- `docs create --tool <toolId> --title "..." --audience ...` writes a
  **published** document; there are no drafts here. `--folder` puts it in a
  folder, and without `--body` it is a title and nothing else.
- `--audience` is required: `everybody`, `chosen` with one `--notify <person>`
  per person, or `nobody`. A word that is none of the three exits 1 before a
  request goes out; which people go with which audience is Minka's rule and the
  CLI keeps none of its own. `--notify` under any audience but `chosen` is exit 9
  `notifyIdsWithoutChosenAudience`, and `chosen` naming nobody is exit 9
  `notifyIdsMissing`. Both are **your input to change** — name people, or choose
  the audience that takes them — and the `hint` says which flag. `--notify`
  takes an ID, `me`, an e-mail address or a name; a name or an address is
  looked up among the members of the project the tool stands in.
- **`personCannotSeeIt` is not your input to fix.** The person you named exists,
  is a member of the project, and simply may not be told: they work with the
  project as a client, or are an agent working for a client. Such a person is
  told only about a document in a folder shown to clients, in a tool shown to
  clients, and only the topmost folder above the document counts. Nothing was
  written, and **sending the same request again will be refused again, however
  often you try**. The only thing that gets the document written is to **name
  somebody else**; which tools and folders clients are shown is decided in
  Minka on the web. The people are met one at a time in the order you gave them,
  and of one person `personNotAProjectMember` is answered before this, so a list
  holding both is answered for whichever name comes first.
- A person named more than once is told once.
- `docs edit <documentId>` changes the title, the body or both. `--body ""`
  empties the body. **`--title ""` does not empty the title:** a document needs
  one, so it exits 9 with `documentTitleMissing`. A title can only be replaced.

### Putting a file into a tool

- `docs uploads create <file> --tool <toolId>` puts a file of this machine on
  top of the tool, or of `--folder <folderId>`. Minka takes it in two requests
  — the file is stored in the project of the tool, then put into the tool — and
  the command does both. It reads the tool for its project first, so a tool you
  cannot open exits 2 **before the file travels**. `data` is the upload, as
  `docs uploads show` gives it. You need no `--project`: an old script that
  still gives one keeps working where it names the tool's project, gets a
  `notice` that it is no longer needed, and exits 2 where it names another.
- A step that fails ends the run, and nothing after it goes out. Where the file
  was stored and the second step was refused, the error's `data` is the stored
  file and the `hint` names `--blob <blobId>`: **try again with `--blob`
  instead of the file**, and it does not travel a second time.
- `docs uploads create --blob <blobId> --tool <toolId>` puts in a file stored
  earlier — by `uploads create --project` or by a run that failed at the
  second step. `--project`, `--name` and `--content-type` belong to storing a
  file and exit 1 beside `--blob`.
- Without `--title` the upload is named after the file. **`--title ""` exits 1
  before anything goes out**, because Minka would refuse it only after the file
  travelled.
- `--description` is Markdown converted here, as the body of a document is.
  Somebody `@`-mentioned in it is told; nobody else is, and there is no
  `--audience` or `--notify` for an upload. You are subscribed to your own
  upload.
- **`fileUnknown` tells no cause apart.** The file may have been swept after
  standing in nothing, never stored, be of another project, or be somebody
  else's. With `--blob` the one remedy worth trying is to store it again:
  give the file itself in place of `--blob`. Where the same run stored it a
  moment ago it cannot have been swept, and the `hint` says so instead.
- **`fileInUse`**: the file of yours stands in an upload or a text already. A
  file goes into one place; give the file itself to store it anew.

### Changing an upload

- `docs uploads edit <uploadId>` changes the title, the description or both,
  guarded as `docs edit` is: `--expect <updatedAt>` or `--force`, a conflict
  exits 9 with `uploadChanged` and the upload in `data`, and the guard is blind
  for one second.
- `--description ""` empties the description. **`--title ""` does not empty the
  title**: exit 9 `uploadTitleMissing`.
- **`docs uploads edit` never changes the file.** Minka ignores a file sent
  with a change, so the command has no flag for it. Another file goes in with
  `docs uploads versions create`, below.
- The description is held against the files and mentions it carries, as a
  document's body is: a Markdown description that would drop them exits 1
  unless `--drop-attachments` is given. Send the `descriptionSource` back with
  `--description-format html` to keep them. People newly mentioned are told.

### Versions

- `docs versions list <documentId>` lists every version of a document, oldest
  first, with its `number`, its `author` and `writtenAt`; the newest is
  `current`. `author` is `null` for a version written before Minka kept who
  wrote it. `docs versions show <documentId> <versionId>` gives one version
  with its `title`, its `body` (Markdown in text, the stored HTML in JSON) and
  the files the body names. **A file in a version's body shows only in that
  list**: the stored body names it by no more than a handle, so it reads as
  `[attachment]` there.
- `docs uploads versions list <uploadId>` lists every file an upload showed,
  oldest first; `current` is the one it shows now. An earlier file is fetched
  with `uploads download <blobId>`, `<blobId>` being `file.id` of its line.
- **The versions are the team's alone.** A client of the project gets exit 2,
  also for a document or upload it is shown, and so does an earlier file it
  did not store itself.
- Nothing is paged. A document in the archive, below an archived folder or in
  an archived project still has its versions read. Making an earlier version
  of a document the current one is done in Minka on the web.
- `docs uploads versions create <uploadId> <file>` **replaces the file of an
  upload**: title and description stay, and the file it showed stays as an
  earlier version. It takes the two requests of `docs uploads create`: it reads
  the upload and its versions first, so an upload you cannot reach — or reach
  only as a client — exits 2 **before the file travels**, and one in the
  archive or below an archived folder exits 9 there. Then it stores the file in
  the upload's project and gives it to the upload. A refused second step hands
  over the stored file in `data`, and `--blob <blobId>` takes that step alone.
  **On a 409 `uploadChanged` `data` is the upload as it stands instead**, and
  the stored file is named only in the `hint`, with its `--blob`.
- **An earlier file of the same upload is `fileInUse`.** To give an upload an
  earlier file back, download it and give it as a file, which stores its bytes
  anew. `--blob` naming the file the upload shows already writes nothing: exit
  0 with a `notice` saying so.
- `--expect <updatedAt>` is optional here: without it Minka still never writes
  over a title or description changed in the meantime, and nothing is lost,
  because the file the upload showed stays as a version. With it a change since
  your read exits 9 `uploadChanged` with the upload in `data`, blind for one
  second as every guard on an entry.

### The body, and the trap under it

**Minka refuses no body.** A body that carries no element it serves — raw
Markdown, plain text — is not an error: it is escaped and kept as the
characters it consists of, and the answer is the same 201 or 200 as any other.
`# Heading` then stands in the document as `# Heading`, and only the web ever
shows it. The same holds for the description of an upload.

So `--body` is **Markdown by default and converted here**, exactly as a card's
description is. Send HTML yourself only with `--body-format html`, and then the
command's `notice` is the only warning you will ever get.

### Writing over a document somebody else may have written

- `docs edit` needs `--expect <updatedAt>` — the `updatedAt` you read — or
  `--force`, and the two exclude each other. A document written over since is
  then left as it stands: exit 9, `meta.apiCode` `documentChanged`, and `data`
  holds the document as it stands now. Merge your text into its `body` and send
  it again with its `updatedAt`.
- **`--expect none` is not a value here.** It belongs to the notes of a card,
  where it means "no notes yet". On a document `expectedUpdatedAt: null` would
  mean "no expectation at all", which is the opposite, so the CLI refuses the
  word and sends no such field.
- **The guard is blind for one second.** Minka keeps the moment of a document
  and of an upload to the whole second. A change somebody else made **in the
  same second as your read** carries the moment you read and passes the guard.
  So after a write that matters, **read it back** with `docs show` or `docs
  uploads show` and check that it says what you wrote. There is no flag for this and no
  answer that mentions it.
- Send the `updatedAt` back **exactly as you read it**. It always ends
  `.000000Z`. A value that is no moment of that shape — one without a zone
  among them — never leaves the CLI: it exits 1 (`usage`) before a request
  goes out.

### Comments

- `docs comments create|edit|delete|react|unreact` take
  `--document <documentId>` **or** `--upload <uploadId>`, exactly one of the
  two. The two kinds have addresses of their own, and the identifier of one is
  nothing at the address of the other, so the flag says which it is.
- The text is Markdown by default, as for a card. Only the author can change or
  remove a comment; somebody else's answers exit 2.
- A document or an upload inside an archived folder shows `archived: true`,
  and its comments take no write: a new one, a change, a removal, a reaction
  and taking one back exit 9 with `documentArchived` or `uploadArchived`. An
  older Minka (v0.41.0) still changes and removes **your own comment** there,
  so `docs comments edit` refuses a text that drops files or mentions unless
  `--drop-attachments` is given, and a newer one then answers exit 9.
- Reactions live on the comments alone. A document and an upload themselves
  take none through the API, although the web offers it.

### What does not exist

There is no delete, trash, archive, unarchive or move of a document; no
creating, renaming or deleting a folder, so the CLI writes only into folders
that already exist; no reading of a tool's archive; no reading of a draft, by
anybody; no replacing the file of an upload, nor deleting, archiving or moving
one. All of it is done in Minka on the web.

### Codes of this part

`documentTitleMissing`, `documentTitleTooLong`,
`documentTitleHasControlCharacters`, `documentBodyTooLong`,
`documentBodyNotReadable`, `documentAudienceMissing`, `personNotAProjectMember`,
`personCannotSeeIt` (a person to replace, not input to fix), `notifyIdsMissing`,
`notifyIdsWithoutChosenAudience`,
`folderUnknown`, `folderArchived`, `documentArchived`, `uploadArchived`,
`documentChanged` (409, with the document in `data`), `uploadTitleMissing`,
`uploadTitleTooLong`, `uploadTitleHasControlCharacters`,
`uploadDescriptionTooLong`, `uploadDescriptionNotReadable`, `uploadChanged`
(409, with the upload in `data`), `fileUnknown`, `fileInUse`, the codes of
storing a file (`contentTypeMismatch`, `tooLarge`), and the comment codes a
card knows. An entry in the archive, or one in a folder in the archive, can be
read and takes no write: exit 9 with `documentArchived` or `uploadArchived`.

## To-do lists and groups

A To-dos tool holds **lists**, the newest on top. A list has a title, an
optional description and **groups**: named headings in an order of their own.
A to-do stands in its list directly or in one of its groups. The to-dos
themselves and their subtasks are `todos …`, see the next part.

### Reading

- `todolists tools <projectId>` lists the To-dos tools of a project in use; a
  project can have more than one.
- `todolists list <toolId>` lists the lists on the page of the tool, in its
  order; `--archived` lists **the archive instead**, the list archived last
  first. A list is on one of the two, never on both, so one missing from the
  first is looked for in the second. Nothing is paged and nothing is cut. A line
  carries the whole description and how many to-dos are open and done.
- `todolists show <listId>` gives the list with its description, its files,
  the open to-dos standing in no group (`todos`), the groups in their order with
  the colour (`color`, `null` for none) and the open to-dos of each, and the
  comments. **Done to-dos are counted
  (`completedCount`) and not listed.** A list in the archive is shown too and
  says `archived: true`.
- A group has no address to read it at: its ID and its place come from
  `todolists show`.

### Writing

- `todolists create --tool <toolId> --title "..."` starts a list **on top** of
  the tool. `--description` is Markdown by default and converted here, as for a
  card; with `--description-format html` a text carrying no element Minka serves
  is kept as its characters, and the `notice` is the only warning you get.
- `todolists edit <listId>` changes the title, the description or both; what
  you do not give stays. `--description ""` empties the description. **`--title
  ""` does not empty the title**: exit 9 `todoListTitleMissing`. A description
  with files or mentions is not replaced unless `--drop-attachments` is given;
  to keep them, edit `descriptionSource` and send it with
  `--description-format html`.
- `todolists archive` and `todolists unarchive` move a list in and out of the
  archive of its tool; it comes back to the place it had. `todolists delete`
  puts a list into the trash from either place, its groups and to-dos with it.
- `todolists comments create|edit|delete|react|unreact <listId> …` work as
  under a card, and `todolists comments list <listId>` reads them, the oldest
  first. Only the author changes or removes a comment; somebody else's is
  exit 2.
- `todogroups create --list <listId> --title "..."` adds a group **at the end**
  of the list, with the colour `--color` names or none. `todogroups edit
  <groupId>` changes the title, the colour or both, and needs at least one of
  `--title` and `--color`; both are written together or not at all. A colour
  is one of `white`, `red`, `orange`, `yellow`, `green`, `blue`, `aqua`,
  `purple`, `gray`, `pink` and `brown`, spelt exactly so. **`--color white`
  takes the colour away**: a group given white reads `color: null`. Any other
  value, `""` included, is exit 9 `todoGroupColorUnknown`, and nothing of the
  edit is written. `todogroups delete` trashes a group **with its to-dos**,
  which then leave the list and its counts.
- `todogroups move <groupId>` needs **exactly one** of `--top`, `--after
  <groupId>` and `--before <groupId>`; the neighbour has to be another group of
  the same list, and the group itself or any other is exit 9
  `todoPlaceUnknown`. A group never leaves its list.

### Lists from templates

The organization's **templates** are lists of a hidden template library, the
team's alone: a client, and anybody outside the team, is in none.

- `todolists templates` lists the templates in use, in their order, as lines of
  `todolists list`, and names the library's `projectId` and `toolId`. A person
  in no library is exit 2.
- `todolists start --tool <toolId> --template <templateId>` makes a list **on
  top** of a To-dos tool of a project: the template's description, groups with
  their colours, to-dos with notes and subtasks. It starts fresh: nothing done,
  **no days at all**, no comments, no series. The title is the template's unless
  `--title` is given (`--title ""` is exit 9 `todoListTitleMissing`). There is
  no flag for a description, days or people: Minka takes none here.
- **Read `leftOff`.** People on the template's to-dos and subtasks who are no
  members of the project are not carried; `data.leftOff` names them (`[]` for
  nobody), and so do the `summary` and the text. Somebody no longer on the team
  is left off without being named.
- **Every call makes a new list**: a second run is a second list, so read the
  tool first where a run may already have happened.
- A template is read and changed as the list it is (`todolists show|edit`,
  `todogroups …`, `todos …`) with the library's IDs; a new template is
  `todolists create --tool <library toolId>`, the archived ones are `todolists
  list <library toolId> --archived`. In a template a day is exit 9
  `templateTakesNoDates` (taking one off is fine) and ticking off a to-do or a
  subtask `templateTakesNoTicks`.
- Refusals of `start`: exit 2 for a tool that is unknown, not yours or **no
  longer in use**; then exit 9 `templateUnknown` (unknown, in the trash, a list
  of a project, or you are in no library), `templateIntoLibrary` (`--tool` is
  the library's own), `templateArchived`, a title code, `projectArchived`.
  Without `--title` an archived template is `templateArchived` before
  everything after it; with `--title` the title is checked first and
  `projectArchived` comes before `templateArchived`.

### The archive of a list

A list in the archive is read and takes **no write** — no change, no new
group, no comment, no reaction — until it is back: exit 9 `todoListArchived`,
and the `hint` names the `todolists unarchive` to run. Its **groups are at no
address at all**: every `todogroups` command on one exits 2, as if it did not
exist. Archiving a list twice is `todoListArchived`, bringing back one in use
`todoListNotArchived`. In an archived project `projectArchived` comes before
those three and before `todoPlaceUnknown`, but after what is wrong with the
request itself: a title or a description Minka refuses is answered as such,
and a list or a group that is not there is exit 2.

### What does not exist here

Moving or copying a list into another tool or project, restoring from the
trash are not offered by this CLI; making one from a template is, above.

### Codes of this part

`todoListTitleMissing`, `todoListTitleTooLong`,
`todoListTitleHasControlCharacters`, `todoListDescriptionTooLong`,
`todoListDescriptionNotReadable`, `todoGroupTitleMissing`,
`todoGroupTitleTooLong`, `todoGroupTitleHasControlCharacters`,
`todoGroupColorUnknown`, `todoListArchived`, `todoListNotArchived`,
`todoPlaceUnknown` (a neighbour that is no other group of the same list),
`templateUnknown`, `templateArchived`, `templateIntoLibrary`,
`templateTakesNoDates`, `templateTakesNoTicks`, all exit 9; `projectArchived`
before any of the archive codes.

## To-dos and subtasks

A **to-do** has a title, notes, a start and a due day, **assignees**, people
**told when it is done**, and **subtasks**: lines of work under it, each with
one optional assignee and one optional due day.

### Reading

- `todos show <todoId>` gives the to-do with `list`, `group`, `notes`,
  `startsOn`, `dueOn`, `isOverdue`, `assignees`, `notified`, `subtasks`,
  `listArchived` and the comments. A to-do of an archived list is read too.
- `todos completed <listId>` reads the **done** to-dos of a list, the one done
  last first, a page at a time: `data` is `{todos, next}`. Hand `next` to
  `--before` exactly as it came (it is no ID); it is `null` on the last page.
- `todos mine` lists your **open** to-dos across all projects: those you are
  assigned to and those you hold an open subtask of (`isAssigned: false`, and
  `subtasks` holds only your own open ones). Done ones, those of an archived
  list and of an archived project are not among them. A bare array, a page at
  a time: **Minka marks the end by nothing but a page shorter than asked for**,
  so a full page says in the `notice` where the next one starts
  (`--after <the last todoId>`). `--limit` is 1 to 100, 50 unless given.

### Writing

- `todos create --list <listId> --title "..."` adds a to-do **at the end** of
  the list, or of the group given with `--group`. `--notes` is Markdown by
  default, as for a card.
- Days are `YYYY-MM-DD`. **A start is kept only on a day before the due day**:
  a start alone, or one on the due day, becomes the due day. Taking the due day
  off (`--due ""`) while a start is left makes the start the due day; give
  `--starts-on ""` as well to take both off. A due day moved onto the start
  takes the start away. The `notice` says when Minka kept a day otherwise than
  you meant it; the exit code does not.
- `--assignee` and `--notify-when-done` take an ID, `me`, an e-mail address or a
  name, repeatable; Minka takes a person named twice once. **On `todos edit`
  they are the whole set**: whoever is not named again is taken off. `--no-assignees` and `--no-notify-when-done` take
  everybody off. Somebody who is no member of the project is exit 9
  `personNotAProjectMember`, and nothing is written.
- `todos edit` changes only what you give. `--title ""` is exit 9
  `todoTitleMissing`; `--notes ""` empties the notes. Notes with files or
  mentions are not replaced unless `--drop-attachments` is given.
- `todos complete` and `todos reopen` succeed twice as well. They leave the
  subtasks as they are.
- `todos move <todoId>` needs **exactly one** of `--top`, `--after <todoId>`
  and `--before <todoId>`. `--list` takes it into another list of the **same**
  To-dos tool. Without `--group` or `--no-group` a to-do **keeps its group
  within its own list**, and stands in no group in another list. The to-do
  named as neighbour has to stand in the list **and the group** the to-do goes
  to, so to go next to a to-do of another group, name that group with
  `--group`. Anything else is exit 9 `todoPlaceUnknown`; another tool or
  project is done on the web.
- `todos delete` puts a to-do into the trash with its subtasks.
- `todos subtasks …` name a subtask as `steps` do: by ID, by position from 1
  or by its text. `create` adds one at the end; `edit` changes the text,
  `--assignee` (one person, `""` for nobody) or `--due` (`""` takes it off);
  `complete` and `reopen` succeed twice; `move` needs exactly one of `--top`,
  `--after` and `--before`; `delete` succeeds for one that is gone already.
  A subtask has no trash.
- `todos comments create|edit|delete|react|unreact <todoId> …` work as under a
  card, and `todos comments list <todoId>` reads them, the oldest first. A done
  to-do takes comments as an open one does. Only the author changes or removes
  a comment; somebody else's is exit 2. `todos comments edit` and `todolists
  comments edit` read the to-do or the list first, for the files and mentions
  a comment of yours may carry.

### The archive, and the order of the refusals

A to-do of a list in the archive is read and takes **no write** at all — no
change, no move, no subtask, no comment or reaction, not even a tick: exit 9
`todoListArchived`, and the `hint` names `todolists unarchive`. No to-do
moves into an archived list either. What Minka says, in this order: exit 2 for
a list or a to-do that is not there, whatever the request says, then what is
wrong with the request itself (a title, a line of a subtask, the days), then
`projectArchived`, then `todoListArchived`, then the place
(`todoPlaceUnknown`) or the people (`personNotAProjectMember`). Somebody else's
comment is exit 2 only after all of these: under an archived list it is
`todoListArchived` as well.

### Codes of this part

`todoTitleMissing`, `todoTitleTooLong`, `todoTitleHasControlCharacters`,
`todoNotesTooLong`, `todoNotesNotReadable`, `todoDatesNotADate`,
`todoDatesOutOfOrder` (a start after the due day), `subtaskMissing`,
`subtaskTooLong`, `subtaskHasControlCharacters`, `subtaskDueNotADate`,
`personNotAProjectMember`, `todoPlaceUnknown`, `todoListArchived`,
`todosLimitNotANumber`, `completedTodosBeforeUnknown` (a `--before` that is
no `next`), `todosAfterUnknown` (an `--after` of none of your projects), all
exit 9.

## Repeating to-dos

A **series** makes a to-do again and again. It is made **from a to-do with a
due day**, which becomes its first occurrence; every further occurrence is an
ordinary to-do at the end of the list, due on its day, with `series: {id,
occursOn}` in `todos show` and `seriesId` on its line in `todolists show`. A
series is **never deleted, only stopped**.

### Reading

- `todos series list <listId>` gives every series of a list, stopped ones too.
- `todos series show <seriesId>` gives `schedule`, `leadDays`, `state`
  (`running`, `stopped`, or `finished`: past its end), `nextOccursOn` (the day
  of the next occurrence), `nextShowsOn` (the day it appears in the list, null
  where it comes with the next round or is there already), `canShowNow`,
  `assignees`, `notified`, `notes`, `notesSource`, `attachments` (the files of
  the notes) and `updatedAt`. **Take the days from Minka**; the CLI never
  works them out, and "today" is the installation's day.

### Writing

- `todos repeat <todoId>` makes the series; it starts on the to-do's due day.
  Without `--rhythm` it is weekly on that weekday. The rhythms and the details
  each takes: `daily` (`--weekdays`), `weekly` (`--weekdays`, `--interval`),
  `monthlyOnDay` (`--day`, `--interval`), `monthlyOnWeekday` (`--weekday`,
  `--instance` 1-4 or `last`, `--interval`), `yearly` (`--month`, `--day`).
  Weekdays are `1`-`7` or names such as `mon`. What is not given follows from
  the first day. A detail the rhythm does not use is a usage error when you
  give `--rhythm`, and a `notice` otherwise (Minka ignores it).
- `todos series edit` sends **only the flags you give**, never the stored
  details, and needs `--expect <updatedAt>` (from `todos series show`) or
  `--force`. While rhythm and first day stay, `--interval 2` alone keeps the
  day; a new `--rhythm` or `--starts-on` takes the details not given from the
  first day. `--ends-on ""` takes the end off. `--assignee` and
  `--notify-when-done` are the **whole set**. It changes the occurrences made
  from now on. Written over since you read it: exit 9 `todoSeriesChanged`,
  and `data` is the series as it stands. A change that asks for what stands
  already writes nothing and is no conflict.
- `todos series edit --notes` takes Markdown, `-` for stdin, `""` to empty
  them, as `todos edit` does; `todos repeat` takes no notes, the series starts
  with the to-do's. **To keep the files and mentions, send `notesSource` back**
  with `--notes-format html`: notes that would drop them are refused (exit 1)
  unless `--drop-attachments` is given. Minka drops a file or person from
  outside the project without a word and tells nobody the notes mention, so
  read the series back.
- `todos series stop` stops it; the to-dos it made stay. `todos series resume`
  continues a stopped one with its first day **after today**; missed days are
  not made up.
- `todos series show-now` makes the upcoming occurrence at once: `data` is
  `{series, todoId}`, where `todoId` is the new to-do. A series holds at most
  one occurrence not yet due.

### Codes of this part

`todoToRepeatUnknown`, `todoSeriesNeedsDueDay`, `todoRepeatsAlready`,
`todoSeriesScheduleInvalid`, `todoSeriesDatesNotADate`,
`todoSeriesDatesOutOfOrder`, `todoSeriesLeadDaysInvalid`, `todoNotesTooLong`,
`todoNotesNotReadable`, `personNotAProjectMember`, `todoSeriesStopped` (stop
or show-now of a stopped series), `todoSeriesNotStopped` (resume of one that
is not stopped, a finished one included), `todoSeriesOccurrenceAhead`,
`todoSeriesFinished`, `todoListArchived`, `projectArchived` and
`todoSeriesChanged` (409), all exit 9. A series of a list you may not read is
exit 2.

## Check-ins

A **question** of a Check-ins tool asks chosen members of the project on a
schedule; each person asked gets a reminder and answers it. `checkins …` reads
and writes the questions, `checkins answers …` the answers, and `checkins
reminders` lists what you were asked and have not answered.

### Reading

- `checkins tools <projectId>` lists the Check-ins tools in use;
  `checkins list <toolId>` their questions, the oldest first; `checkins show
  <questionId>` one question with `schedule` (`rhythm`, `weekdays`, `timeOfDay`,
  `startsOn`), `paused` and `asked`.
- `asked` names only the people asked **who are still members and may see the
  question**, in the order of their names. Whoever it does not show stays
  asked unseen, and no edit takes them off.
- `checkins answers list <questionId>` lists the answers, the newest day
  first: `groupOn` is the day an answer belongs to. `--person` (ID, `me`,
  address or name) and `--on <day>` narrow them. A bare array, a page at a
  time: **Minka marks the end by nothing but a page shorter than asked for**,
  so a full page says in the `notice` where the next one starts (`--before <the
  last answerId>`). `--limit` is 1 to 100, 50 unless given.
- `checkins answers show <answerId>` gives the answer with `bodySource`, its
  files and its comments.
- `checkins reminders` lists your open reminders across all projects, the one
  asked last first: per question only the latest day it asked you for, open
  until an answer of yours belongs to that day or a later one. Paused
  questions and archived projects are not among them. **At most 100, and no
  page beyond**: a list that long says in the `notice` there may be more; those
  are the ones asked earlier, and answering some brings them into view.

### Writing a question

- `checkins create --tool <toolId> --title "..." --rhythm <rhythm> --weekday
  <day> --time HH:MM --ask <person>`. `--rhythm` is Minka's word: `daily` (on
  each weekday given), `weekly`, `everyOtherWeek` or `monthly` (the first of the
  weekday in a month); every rhythm but `daily` takes exactly one weekday.
  `--weekday` is `1` (Monday) to `7` or a name (`mon`, `monday`), repeatable or
  as `mon,wed,fri`. `--time` holds in the time zone of each person asked. The
  question starts on your today.
- `--ask` on `checkins create` takes an ID, `me`, an e-mail address or a name;
  a name or an address is looked up among the members of the project the tool
  stands in. Only members of the project who may see the question can be asked:
  anybody else, a client of the project or an agent working for one included,
  is exit 9 `personNotAProjectMember`.
- `checkins edit` changes only what you give; the start day never changes.
  **`--weekday` and `--ask` are the whole set.** The weekdays not given are the
  question's own, so `--rhythm weekly` on a question asked Monday to Friday
  needs one `--weekday`. **Every other week is counted from the first day of
  its weekday on or after `startsOn`**, so a new weekday can move the question
  into the other set of weeks; an edit of the rhythm or the weekday of such a
  question names the first three of its dates, counted from `startsOn`, in the
  `notice`; for an older question those lie in the past.
- An edit without `--ask` leaves the people asked as they are. `--ask` takes
  off only people shown in `asked` and not named again: a member who left the
  project and a client the question is not shown to stay asked, so the people
  read from `checkins show` and named back take nobody off.
- Only a member who may see the question can be asked. A question set up here
  is shown to no client, so naming a client of the project, or an agent
  working for one, is exit 9 `personNotAProjectMember` and nothing is written.
  Should somebody named still be missing from `asked`, the `notice` names them
  as not asked.
- `checkins pause` and `checkins resume` succeed twice as well. The days a
  question was paused are not asked afterwards.

### Answering

- `checkins answers create <questionId> -` answers as the account of the
  token; the text is Markdown with every line break kept, as for a comment.
  Anybody in the project may answer, as often as they like, the same day too.
- `--asked-for <day>` ties the answer to a reminder: give `askedFor` of
  `checkins reminders`. A day the question was not put to you for is exit 9
  `answerDayNotAsked`. Without it the answer belongs to the day of the latest
  asking by the question's schedule, in your time zone, whether the question
  was paused then or asks you at all (or today, before the first asking);
  `groupOn` says which, and it never changes.
- `checkins answers edit <answerId> -` changes your own answer; somebody
  else's is exit 2, as if it were not there, also where it carries files. The
  same text again succeeds. Your own answer with files or mentions is not
  replaced unless `--drop-attachments` is given.

### The order of the refusals

What Minka says, in this order. First exit 2 for a tool, a question or an
answer that is not there, somebody else's answer included, whatever the
request says. For a new question then the wording, the weekdays, the time and
the rhythm, then `questionNobodyAsked`, then `projectArchived`, then
`personNotAProjectMember`. For an edit the wording, the schedule, then
`questionNobodyAsked`, `projectArchived` and `personNotAProjectMember`. For an
answer `answerDayNotADate`, then the text (`answerTooLong`,
`answerNotReadable`, `answerMissing`), then `answerDayNotAsked`, then
`projectArchived`. Pausing or resuming a question of an archived project is
`projectArchived` whatever state it is in.

### Codes of this part

`questionTitleMissing`, `questionTitleTooLong`,
`questionTitleHasControlCharacters`, `questionWeekdaysMissing`,
`questionTimeInvalid`, `questionScheduleInvalid`, `questionNobodyAsked`,
`personNotAProjectMember`, `answerDayNotADate`, `answerDayNotAsked`,
`answerMissing`, `answerTooLong`, `answerNotReadable`, `answersBeforeUnknown`
(a `--before` that is no answer to the question), `projectArchived`, all exit 9.

## Posts

A Posts tool — the message board — holds **posts**: a title, a body, a kind
it may be filed under, and comments. `posts …` reads and writes them;
`posts comments …` the comments under one.

### Reading

- `posts tools <projectId>` lists the Posts tools in use; `posts categories
  <projectId>` the kinds a post can be filed under, which every tool of the
  project shares.
- `posts list <toolId>` lists the posts that went out: **pinned ones first**,
  the one pinned last on top, then the newest. It is a page, `{posts, next}`:
  `next` is null on the last page, and otherwise goes to `--before` exactly as
  it came. `--limit` is 50 unless given; Minka cuts a page above 100, which
  `next` then says. No draft is listed, your own included, and nothing of the
  archive or the trash.
- `posts list <toolId> --archived` lists the archive, the one archived last
  first, with `archivedAt` and `archivedBy`. It is not paged.
- `posts show <postId>` gives the post with `body`, `bodySource`, its files,
  `category`, `pinned`, `archived` and its comments. **A draft and a post in the
  trash are at no address**, their author included: exit 2.

### Writing

- `posts create --tool <toolId> --title "..." --body - --audience ...` sends a
  post out; there are no drafts here. The body is Markdown, converted here, and
  **a post says something**: a body without a word and without a file is exit 9
  `postBodyMissing`. `--category <categoryId>` files it under a kind.
- `--audience` and `--notify` work as for a document: `everybody`, `chosen`
  with one `--notify <person>` per person, or `nobody`; the hint of
  `notifyIdsMissing` and `notifyIdsWithoutChosenAudience` says which flag to
  change. A person named more than once is told once.
- **A post written here is shown to no client of the project.** Naming a client,
  or an agent working for one, is always exit 9 `personCannotSeeIt`: nothing
  was written, the same request is refused again, and the only way on is to
  name somebody else. `everybody` refuses nobody; it puts the clients on the
  post's list, but they are not notified and cannot open it.
- `posts edit <postId>` changes the title, the body, the kind or several;
  what is not given stays, and the change tells nobody. Anybody in the project
  may change any post. `--title ""` is exit 9 `postTitleMissing` and `--body ""`
  exit 9 `postBodyMissing`: neither can be emptied. `--no-category` takes the
  kind off. A post with files or mentions is not replaced by a body without
  them unless `--drop-attachments` is given.
- `posts pin` puts a post on top of its list; there is no limit, and pinning a
  pinned post (or unpinning one that is not) succeeds and moves nothing.
- `posts archive` takes a post off the list into the archive: readable, and
  every write on it — comments included — is exit 9 `postArchived` until
  `posts unarchive`. A pinned post keeps its pin there and is on top again when
  it comes back. Archiving a post in the trash is exit 9 `postInTrash`.
- `posts delete` puts a post into the trash, from the list or from the archive;
  anybody in the project may. It comes back on the web only.
- `posts comments create|edit|delete|react|unreact <postId> …` work as under a
  card. Only the author changes or removes a comment; somebody else's is exit 2.

### The order of the refusals

For a new post: exit 2 for a tool that is not there **before** anything else,
then the title, the body, the audience and the people it names, then
`projectArchived`, then `postCategoryUnknown`, then the people one at a time
(`personNotAProjectMember`, `personCannotSeeIt`). For an edit: exit 2, then the
title and the body, then `projectArchived`, `postArchived` and
`postCategoryUnknown`. For pin, unpin, a comment and archiving:
`projectArchived` before `postArchived` — except that archiving a post in the
trash says `postInTrash` first. `posts edit --body` and `posts comments edit`
read the post first; `posts comments edit` reads you (`me`) as well where the
comment carries files or mentions, and both read the archived projects where
their guard over files is about to refuse. That guard never comes before
`projectArchived` or `postArchived`.

### Codes of this part

`postTitleMissing`, `postTitleTooLong`, `postTitleHasControlCharacters`,
`postBodyMissing`, `postBodyTooLong`, `postBodyNotReadable`,
`postCategoryUnknown`, `notifyIdsMissing`, `notifyIdsWithoutChosenAudience`,
`personNotAProjectMember`, `personCannotSeeIt` (a person to replace, not input
to fix), `postArchived`, `postNotArchived`, `postInTrash`,
`postsLimitNotANumber`, `postsBeforeUnknown`, `projectArchived` and the comment
and reaction codes a card knows, all exit 9.

## Calendars and events

A calendar holds **events**: one-off, all-day or repeating. `events …` reads
and writes them; `events tools <projectId>` lists the calendars in use. There
is no way back out of the trash through the API.

### Reading

- `events list <toolId> --from <day> --to <day>` lists every event with an
  occurrence in the span, both days included and **both required**, at most
  366 days, not paged. `data` is `{from, to, timeZone, content, events,
  datedItems}`. The days are **yours**: the zone of your preferences in Minka,
  else the installation's, named by `timeZone`.
- **`datedItems` are no events.** While the calendar's `content` is
  `eventsAndDatedItems` (the default), they are the open to-dos and cards of
  the project due in the span, **whoever they belong to**, the earliest first,
  then by title with only A to Z lowercased (`10` before `9`), then by `id`:
  `kind` (`todo`, `card`), `toolId` (To-dos tool or board), `cardId`, `title`,
  `dueOn`, `placeTitle` (list or board) and `appUrl`. With `eventsOnly` the
  list is empty. They have a day and no time; read one with `todos show` or
  `cards show`. The text view puts them under a heading of their own.
- `events mine --from <day> --to <day>` is **My calendar**, across every
  project of yours that is not archived: `events` you **take part in** (each
  line with `toolId` and `project` besides), and `datedItems` assigned **to
  you**, steps included (`kind` `step`, `cardId` its card, `placeTitle` the
  card, `appUrl` opens the card), in the order of `events list`. A to-do of
  which only a subtask is yours is not there, nor is a card in Done or
  completed in another column, nor its steps. The span is refused as for
  `events list`; a calendar's `content` does not matter here.
- **Every event stands in the span once, a series too.** Lay out an agenda from
  `occurrences`, the times it takes place at in the span, never empty and in
  the order they start. The event's own `startsAt`/`endsAt`/`firstDay`/
  `lastDay` are its first occurrence, which for a series may lie years before
  the span. A line has no description.
- `events show <eventId>` gives the event with `description`,
  `descriptionSource`, its files, `schedule`, `timeZone`, `joinUrl`,
  `highlighted` and `participants`.
- A timed event carries `startsAt` and `endsAt` in UTC and `firstDay`/`lastDay`
  null; an all-day one the reverse, `allDay` says which. `schedule` is null for
  an event that does not repeat; its fields are those of a to-do series.

### Writing

- **The time** is `--starts-at` and `--ends-at` together, ISO 8601 **with an
  offset** (`2026-10-05T09:00:00+02:00`, `…Z`), or `--first-day` and optionally
  `--last-day`. A moment without an offset is exit 9 `eventTimeNotReadable`.
  An edit that names any of the four names the whole time anew.
- **Repeating:** on `create`, `--repeat` alone repeats weekly on the weekday
  of the first day; on `edit`, it keeps the stored schedule; `--rhythm`, `--weekdays`, `--interval`, `--day`, `--weekday`,
  `--instance`, `--month`, `--ends-on` work as for `todos repeat`. The details
  must fall on the first day of the event (`eventRepetitionDoesNotFit`). A
  timed series keeps its clock time in `--time-zone` (an IANA name such as
  `Europe/Berlin`, with its capitals; `CET` and `Europe/Kiev` are taken,
  `+02:00` is refused), else in the zone it has, else in yours.
- `events edit` sends only what was given. **Without `--repeat` or a detail, a
  series keeps its first day:** moving it is exit 9
  `eventRepetitionKeepsItsFirstDay`; give the new time **with** `--repeat` (and
  the details that hold the new day). `--no-repeat` stops the repetition.
- `--participant` names members, the **whole set** on an edit;
  `--no-participants` takes everybody off. An event the team adds is shown to
  no client: naming a client is exit 9 `personCannotSeeIt`.
- **No guard: the last write wins.** There is no `--expect` here.
- `--join-url ""` takes the link off, `--highlighted=false` the highlight. A
  description with files or mentions is not replaced by one without them
  unless `--drop-attachments` is given.
- `events delete` puts an event into the trash, a series with all its days; a
  client only an event of their own.
- `events settings <toolId> --content eventsAndDatedItems|eventsOnly` chooses,
  for everybody, whether the calendar's span carries what is due. Minka's
  words, as written: anything else is exit 9 `calendarContentUnknown`. Only
  the team chooses: a client is exit 2, after the value and the archive.

### Comments

- `events comments create|edit|delete|react|unreact <eventId> …` work as under
  a card, and `events comments list <eventId>` reads them, the oldest first.
  **`events show` carries only `commentCount`**: the comments, with their
  `bodySource`, come from `events comments list`.
- A series has its comments **on the event as a whole**, never on one
  occurrence. A calendar no longer in use takes comments; an event in the
  trash is exit 2; in an archived project every write is exit 9
  `projectArchived`, reading stays open.
- Only the author changes or removes a comment; somebody else's is exit 2.
  `events comments edit` reads the event and its comments first, for the
  files and mentions a comment of yours may carry.

### The order of the refusals

Exit 2 for a calendar or event not there **before** anything else; then the
type of every field (400); then title, time, `--join-url`, a participant that
is no ID, the schedule, the zone, `--ends-on`, the schedule against the first
day, the description; then `projectArchived`; then the people
(`personNotAProjectMember`, `personCannotSeeIt`).

### Codes of this part

`eventsSpanNotADay`, `eventsSpanOutOfOrder`, `eventsSpanTooLong`,
`eventTitleMissing`, `eventTitleTooLong`, `eventTitleHasControlCharacters`,
`eventTimeMissing`, `eventTimeAmbiguous`, `eventTimeNotReadable`,
`eventEndsBeforeItStarts`, `eventJoinUrlNotAnAddress`,
`eventDescriptionTooLong`, `eventDescriptionNotReadable`,
`eventRepetitionScheduleInvalid`, `eventRepetitionDatesNotADate`,
`eventRepetitionDatesOutOfOrder`, `eventRepetitionTimeZoneUnknown`,
`eventRepetitionDoesNotFit`, `eventRepetitionKeepsItsFirstDay`,
`calendarContentUnknown`, `personNotAProjectMember`, `personCannotSeeIt`,
`projectArchived`, all exit 9.

## Saying which cards you are working on

The board can show a card as alive while somebody works on it. Only an agent
can say so — with a person's token every signal is refused — and it works like
this:

- **`minka work start <card>` alone shows nothing.** It only remembers the
  card in the list of your session. What tells Minka is `minka heartbeat`, and
  until one has run, no board has heard a word. `work start` says so in its
  `notice` while nothing has ever reported that list.
- **Every session has a list of its own,** and its heartbeat reports that list
  and no other, as a client of its own. When your session ends, its cards go
  dark with it instead of being reported by the other sessions on the machine.
  The session is the one Claude Code names in `$CLAUDE_CODE_SESSION_ID` to the
  commands you run, and the mod reports under the same name; subagents share
  the session that spawned them, so their steps count for it. `--session <id>`
  names it explicitly, for the mod; you do not need it. Without either, a
  command speaks for the list of no session, which the mod never reports, and
  `work start` says so in its `notice` inside Claude Code.
- **The mod does the reporting.** `minka setup` installs it, and `minka hooks
  install` installs it again: code that runs inside the Claude Code process,
  for every project, and runs `minka heartbeat` at the start of every turn,
  after every tool call and every thirty seconds. So a tool call that runs for
  an hour keeps its tile alight, and a session's cards go dark three minutes
  after its process ended. Install it once, then `work start` is all a session
  does. It needs `claude` on the PATH, version 2.1.287 or later, the first that
  loads mods; without it, `hooks install` exits 1 with the two `claude plugin`
  commands to run in `hint`, and with an older one it exits 1 and says to
  update Claude Code. `setup` says either and goes on. It takes effect in
  sessions started afterwards. `minka hooks status --json` says in
  `data.mod.state` (`current`, `stale`, `not installed`) and `data.mod.enabled`
  what became of it, and in `data.mod.reason` when Claude Code is too old.
  `--from-mod` and `--idle` of `minka heartbeat` are the mod's own business —
  never pass them yourself. Removing the mod is `claude plugin uninstall
  minka-heartbeat@minka-cli` and `claude plugin marketplace remove minka-cli`.
- An earlier minka wrote a `PostToolUse` hook into `~/.claude/settings.json`
  that ran `minka heartbeat`. `minka hooks status --json` names it in
  `data.oldHook` as a leftover, and `minka hooks install` takes that entry out
  once Claude Code has the mod, leaving everything else in the file as it was;
  `data.oldHook.removed` says that it did. With a Claude Code too old for mods
  the hook stays.
- Without the mod — a Claude Code older than 2.1.287, or an agent that is not
  Claude Code — nothing reports for you: run `minka heartbeat` yourself every
  minute or two while you work on a card, since the board forgets it three
  minutes after the last signal.
- `minka work stop <card>` takes a card out again, and the next signal gives it
  up at once. Do it when you are done with a card; otherwise it stays alive
  until it runs out. It asks nothing of the network and the card may be named
  by its short identifier, but it needs a host like every other command, so
  without a sign-in it exits 3. A card your session does not hold is taken out
  of the list of whichever session on this machine holds it.
- `minka cards move` and `minka cards done` take the card out of every list by
  themselves and tell the board at once, where the card lands in another
  column, is put on hold or reaches Done; a card that only changes place where
  it already is stays in the list. Only a card your own session held is
  reported at once; one another session held goes with that session's next
  signal. The `notice` says where it happened, and
  whether the board has heard of it yet. So take a card up in that order: move
  it into the working column first, then `minka work start`.
- `minka work stop` without a card stops the one card your session holds. With
  several it exits 1 and changes nothing: name the card, or take all of your
  session's cards with `--all`.
- `minka work list --json` shows your session's list: `data.session`, then
  `data.cards` with `id`, `shortId`, `title` and `startedAt`, `data.elsewhere`
  with the cards of the other sessions on this machine, each with its
  `session`, `data.sentAt` when a signal of your session last went out,
  `data.reportedAt` when one last arrived, and `data.failure` what the host
  made of the last one that did not. The `notice` advises `minka hooks install`
  only while nothing has ever gone out; a signal that was turned away is no
  reason to install anything.
- **A card in the archive of its board is not taken at work.** `minka work
  start` on one exits 1 and says so, because no board shows a card in an archive
  as alive: take it out of the archive first.
- A card that is archived while somebody has it in hand is left out by Minka: it
  answers the signal with 200 and `leftOut`, notes the other cards of the set,
  and the card is taken out of the list of this machine, so that one card nobody
  can work on does not put out the tiles of the rest. The shorter list is
  reported with the next beat. It stands afterwards in `data.letGo` of `minka
  work list` and of `minka heartbeat --json`, with `id`, `shortId`, `title`,
  `reason` and `at`, and in the `notice` of `work list` in words, until it has
  stood there eight hours. So a card that is missing from `work list` and named
  in `letGo` was archived, not forgotten.
- A `reason` this CLI does not know takes no card out of the list: the card
  stays, the word is reported as it came, and the `notice` says so. Act on it
  yourself.
- `minka heartbeat` always exits 0 and prints nothing unless you ask with
  `--json`, which names the session it reported for in `data.session`. Not
  signed in, no network, a token that was revoked: none of it stops your work,
  and none of it is reported as a failure. If the board shows nothing, run
  `minka heartbeat --json` and read `data.sent` and `data.reason`, which names
  the case with the status and the code of the API, such as "(HTTP 403,
  agentsOnly)" for a token that belongs to a person and not to an agent. `minka
  work list --json` carries the same words in `failure` for the last signal that
  reached the host.
- Never write the store yourself.
- Minka forgets a card three minutes after the last signal, so a session that
  ends without `work stop` goes quiet by itself, three minutes after the last
  beat of its mod. Other sessions never extend that. A session that sits idle,
  with no tool call running, keeps its cards alight for twenty-five minutes
  after its last step, not for as long as Claude Code stays open.
- The list gives a card up eight hours after your session was last at it. Your
  heartbeat says so for the cards you started, so a card you keep working on is
  kept however long the work lasts. A card started outside a session — by a
  person at a terminal — goes by the moment it was started instead. `work list`
  carries a `notice` when a card was given up, and `minka work start` brings it
  back.
- A card you may not see exits 2 at `work start`; the server itself would pass
  it over without a word. There is no way to read back what Minka holds.

## Importing from Basecamp

- `import basecamp --out <dir>` extracts projects of a Basecamp
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
- Without `--project` the run takes every project the Basecamp identity can
  see, the active and the archived ones; an archived project comes with
  `status` `archived`. `--active-only` leaves the archived ones out. Projects
  in the trash are never taken. The projects stand in the bundle in the order
  of their IDs. An identity that sees no project (no active one with
  `--active-only`) exits 2.
- `--project` takes the numeric ID of a Basecamp project, names exactly the
  projects to take whatever their status, and is repeated for several.
  Anything that is no number exits 1 before any request, and so does
  `--project` together with `--active-only`.
- `data.invalidAddresses` lists, by ID only, the people who come without an
  e-mail address, and so without a login, because the one Basecamp holds is
  none; `notice` says how many. The run still exits 0 and running it again
  changes nothing: only somebody with access to the account can repair the
  address there. Never look the address up to quote it.
- Every address a bundle carries — a person's `email`, the sender of a
  forwarded mail, the sender of an answer by mail — is read the way Minka reads
  it, no wider: one that would stop the applier is left out here instead. One
  pair of quotation marks around the whole of an address comes off before it is
  read, which is the one thing a run repairs rather than passes on;
  `data.addressesUnquoted` counts the records it happened to — one mailbox
  written that way in many records counts in each of them — a `notice` says how
  many, and no output names an address.
- `data.creatorProbesRefused` counts the tools asked for who created a project
  that Basecamp refuses this identity. They are passed over and a `notice`
  says how many: the run exits 0, and a project may come without a creator
  because of it. An identity that may see nothing at all still exits 4.
- So far a bundle holds the structure, the boards, the message boards, the
  chats, the check-ins, the to-dos, the forwarded mail and the Docs & Files:
  the people the
  projects refer to
  (members and whoever a record names; `data.peopleLeftOut` counts the rest of
  the account's list, which holds more for an admin than for anybody else, the
  people written do not differ) with their avatars, the projects with their
  members, the card tables, chats, message boards, questionnaires, to-do
  sets, inboxes and Docs & Files tools that are switched on, of each card table
  the columns, the
  cards with
  the archived and the trashed ones, their steps, comments and boosts, of each
  message board its messages as posts, the archived and the trashed ones
  included, with their categories, comments and boosts, of each chat its lines
  as messages with their boosts, of each questionnaire its questions with
  their answers, comments and boosts, of each to-do set its lists with
  their groups and to-dos, of each inbox its forwarded mail with the answers
  by mail, the comments and the boosts of each, and of each Docs & Files tool
  its folders, documents and uploads with the comments, the boosts and the files
  of each. `data.skippedTools` counts what
  was left out, per
  kind and whether it was switched on; `data.counts` is what the manifest
  says.
- A person comes with `retired: true` unless Basecamp calls them a user:
  somebody who was deleted and an outsider are authors and nothing more,
  whichever identity ran the import. A chatbot or integration, a deleted one
  included, comes with `kind: "bot"` and without an address or `client`, a
  deleted one with `retired: true`; `data.bots` counts them.
  `data.botsAlsoNamedElsewhere` names deleted people who wrote as an
  integration and as a person, and a `notice` says so: Minka never turns a bot
  back into a human, so look at them before the bundle is applied.
- What the source says about clients comes with the bundle: `client` on every
  person, `company` on a client alone, `clientAccess` and `clientCompany` on a
  project, and `visibleToClients` on a tool, a post, a to-do list, a check-in
  question and every folder, document and upload of Docs & Files. That word is
  always the record's own and never the word of its tool:
  a post of a board the clients see can be one the source keeps from them. Where
  the source says nothing, nothing is written and Minka leaves what it holds
  alone. A record that follows what it hangs on — a to-do, a group, a card, an
  answer, a comment — carries none, and `data.visibilityLeftToTheContainer`
  counts the words the source offered there. `data.peopleAsClients` counts the
  clients and `data.clientMemberships` the memberships Minka leaves out, because
  a client is no member of a project there; for the same reason no question is
  said to be asked of a client, which `data.askedLeftOut.clients` counts. A
  company the source names without an identifier ends the run where the record
  that names it comes in: written away as none it
  would tell Minka to drop the company it holds. An Email forwards tool carries
  its word, and a mail and an answer follow it. **An entry of Docs & Files is
  the one exception to the rule above:** a folder, a document and an upload
  carry the word at **every** depth, and not only at the top of their tool,
  because Minka reads it below the top as well — something hidden from clients
  below a folder the source shows makes Minka hide that folder with everything
  in it. An entry inside a folder that the source says nothing about is named in
  `data.docs.silentAboutClientsInsideAFolder` — the kind, the `sourceId` and the
  project, never a title — with a `notice`: nothing is written for it, so it can
  hide nothing above it, and where the folder at the top of its tool is shown to
  clients that word was the one thing which would have. It is named rather than
  counted because the only thing to do about it is to compare it against the
  source. No output ever names a person or a company.
- `--calendars` brings the calendar of each project (Basecamp's Schedule) as a
  tool of `kind` `calendar` and its entries as events. Only use it for a Minka
  whose import applies events: any other stops at the first calendar, which is
  why a run without it skips the calendar like a kind Minka has nothing for and
  says nothing of calendars. A timed event carries its two moments, an all-day
  one its first and last day; an entry that repeats is one event with its
  schedule. `data.calendars.calendars[]` counts per calendar what was written,
  the trashed ones, those that repeat and those that come with their series,
  names in `repetitionNotMapped` each part of a schedule that did not fit (the
  event then comes once), and in `allDayInInstants` and `timedInDays` the
  entries left out because the source says their days in moments or the other
  way round, and in `timesMinkaRefuses` those left out because they end before
  they start or start before the year 1. `data.calendars.commentsUnderSingleDays` counts the comments
  written under one day of a series, which a bundle has no place for. An entry
  on the shelf or drafted is in `data.recordsLeftOut`. A run never asks for a
  single day of a series: Basecamp makes a record of the day where somebody
  does.
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
- A to-do set becomes a tool of kind `todos`. Its lists stand in the bundle from
  the bottom up where they are in use, because Minka puts every new list on top,
  the ones the source has put away standing before them as it lists them, where
  their order decides nothing, and the groups and the
  to-dos of a list in the order of the list. `data.todoListsByStatus` splits the
  lists and `data.todosByStatus` the to-dos. A
  list knows the shelf and the trash, a group and a to-do the trash alone: one the
  source has put on the shelf is left out and counted. `data.todosLeftOut` counts
  per kind (`lists`, `groups`, `todos`) what the bundle holds none of, whatever
  the reason, and `data.todoPlacesLeftOut` names every list and group that took
  records with it, with how many groups and to-dos fell with it; a `notice` says
  how many such places there were. A to-do can hang on a to-do set itself and
  stand in no list of it, which a bundle has no place for: it is left out,
  `data.todosWithoutAList` names it with its `sourceId` and its project, and a
  `notice` says so — move it into a list at the source and a later run carries it.
  Nothing hangs on a group,
  which the format has no word for; a list and a to-do carry comments, boosts and
  the files of their text. A to-do is never said to be older than the tick the
  source put on it, or Minka would never write the tick. Subtasks are not carried:
  `data.todosWithSubtasks` and `data.subtasksLeftOut` count what fell away, and a
  `notice` says so. A to-do that comes again at the source comes as an ordinary
  one, because Minka has no repeating to-do: the schedule stays behind and
  `data.todosThatRepeat` counts the ones a bundle carries, with a `notice`. Two differences a later run
  cannot repair, each with a
  `notice`: it leaves the people of a to-do as they stand in Minka
  (`data.todosWithPeople`) and reopens nothing it once ticked off
  (`data.todosNotDone`).
- An inbox becomes a tool of kind `forwards`, and its mails `forwards` with
  their `forwardreplies`. `title` is the subject, `fromAddress` the sender, who
  stays an address and never becomes a person, `fromName` always `null`, and the
  body the mail as the source holds it, line breaks and all: Minka turns those
  into paragraphs. `data.forwards.byStatus` splits the mails; what the source
  keeps on its shelf comes into the trash, where it can be brought back, and
  `data.forwards.fromTheShelf` counts it. A subject nobody wrote becomes
  `(no subject)` and one longer than 200 characters or over several lines is
  folded and cut short (`data.forwards.withoutASubject`,
  `data.forwards.subjectsChanged`). A mail whose sender is no e-mail address
  Minka takes is left out and named by ID in
  `data.forwards.withoutAnAddress`; a sender inside a pair of quotation marks
  comes out without them and is counted in `data.addressesUnquoted`. Where what
  a mail says would reach Minka as nothing at all, the run writes a line of its
  own into the body
  (`data.forwards.givenALineOfTheRun`), because such a mail would be lost on
  every run. A file that might still come is not that: the mail waits
  for it, and a later run brings both, while one Basecamp itself refuses is a
  file that never comes. A mail nobody is named for goes to the
  owner of the organization
  (`data.forwards.withoutAForwarder`). An answer by mail is a record of its own
  and no comment: it belongs to somebody of the account or, for one from outside
  it, to the address alone and never to both
  (`data.forwards.repliesFromOutside`); one naming neither is left out and named
  (`data.forwards.repliesWithoutASender`), and a boost on an answer is counted
  and not carried (`data.forwards.reactionsOnRepliesLeftOut`), whether the answer
  itself comes or not. Comments and
  boosts hang on the mail itself. A body longer than the 1,000,000 characters
  Minka holds is counted in `data.forwards.longerThanMinkaTakes` and comes as it
  is: Minka passes such a mail over on every run, with everything hanging on it.
  No output ever names a subject or an address.
- A Docs & Files tool becomes a tool of kind `files`, and what stands in it
  `folders`, `documents` and `uploads`. A folder stands in the bundle after the
  folder it is in. For an entry in use the order of the file decides, bottom up,
  because Minka puts every new one on top of its kind; for one that has left its
  place it decides nothing, Minka ordering an archive by the moment a record was
  put away, so those stand first as the source lists them. There is no position in
  the format. `data.docs.foldersByStatus`, `documentsByStatus` and
  `uploadsByStatus` split them. The status of an entry is its **own** and never
  the status of its folder: what stands in a folder the source archived or threw
  away carries `active` and goes along through the folder in Minka. A document
  carries its title, its text, its files, its comments and its boosts; a folder
  takes neither comments nor boosts, here as in Basecamp, so neither is asked
  for. An upload is one stored file beside a text, and its title is **what the
  source calls it**, which Basecamp fills from the name of the file and anybody
  can rename;
  the name of the file stands in where the source calls it nothing
  (`data.docs.titlesFromTheFileName`) and `(no name)` where there is neither
  (`data.docs.withoutAName`). A title Minka would refuse is folded to fit
  (`data.docs.namesMadeIntoATitle`), a name too long the way Minka shortens one:
  the extension stays and the stem is cut. With `--upload-versions` the files an
  upload showed before come as its `versions`, oldest first and the file it shows now last, each
  earlier one a file of its own (`data.docs.versions.earlierFiles`); a version
  Basecamp lists without its file is left out and named
  (`data.docs.versions.withoutTheirFile`), and an upload whose list of versions
  Basecamp would not hand out, or that does not say which file is current, comes
  with the file it shows alone (`data.docs.versions.historiesUnread`). Such an
  earlier file is fetched at its version's address with the token of the Basecamp
  CLI, which goes to Basecamp's API and nowhere else. Without the flag no
  version is asked for and the report has no `data.docs.versions`. An upload Basecamp names no stored file for — one in a
  cloud — is left out and named by ID (`data.docs.withoutAStoredFile`), with its
  comments and its boosts (`data.docs.commentsWithoutTheirUpload`,
  `boostsWithoutTheirUpload`), because no run will ever have bytes for it. What
  says so is the record and never the shape of the address: the bytes of an
  upload's file are asked for **by the identifier of the upload and its
  project**, the address the source gives being one no run follows. A
  document or a description longer than the 1,000,000 characters Minka holds is
  counted (`data.docs.longerThanMinkaTakes`) and comes as it is. A record that
  lost a file no run will bring is named (`data.docs.onlyAFileNoRunWillBring`),
  and the kinds end differently because a text is optional and a file is not: a
  document whose whole text was that file arrives **without words**, its title,
  comments and boosts with it; an upload has no purpose without its file and is
  **left out** with its comments and its boosts — its file gone at the source,
  above what Minka takes, in a cloud, or refused by Basecamp itself. A file that
  might still come is not that: `why` of its entry in `data.missingFiles` says
  `failedThisTime` or `notUnderstood`, the upload comes and waits for it, and a
  later run brings both whole. A dry run downloads nothing and cannot know a
  refusal, so it counts an upload a run leaves out. `data.docs.leftOut` counts per kind
  what the bundle holds none of, and `data.docs.placesLeftOut` names every
  folder and every tool that took entries with it. A folder the source has put
  away that would not say what stands in it is passed over and counted
  (`data.docs.foldersThatSaidNothingOfWhatIsInThem`): what is in use below it is
  in no bundle and nothing can count it. No output ever names a file,
  a folder or a document.
- A record the source describes with a word a bundle has none for is left out
  and the run goes on: a chat line of an unknown kind or with a status other
  than `active`, a column of a kind a bundle has no role for, a card, a
  post, a comment, a question or an answer with a status a bundle does not
  know, a draft among them, a group or a to-do the source has put on the
  shelf, a forwarded mail of a status that is none of the three, an answer
  by mail that says anything but `active`, and a folder, a document or an
  upload of Docs & Files whose status is none of the three;
  `data.recordsLeftOut` calls the mail and the answer `forward` and `reply`. `data.recordsLeftOut` names each with the kind of
  record, the field, the word the source said there, where it stood and how
  many records it cost, and a `notice` counts the records. A record whose times
  are missing or are no times still ends the run: it would be put in the wrong
  place. A time is a moment written one way, `2024-02-05T17:00:00.000Z` or
  `2024-02-05T17:00:00+01:00`: a relative word such as `now`, a time without a
  zone, and a moment of the right shape that no calendar has end the run, named
  with the record and the field, and so does a day that is not `YYYY-MM-DD`
  where a card is due, where a to-do starts or is due, or where an answer
  belongs. The message says what stood there only where it has the shape of a
  time and is short: a value of another shape is not repeated, because that field
  then holds whatever the source put in it, and a long one is not repeated
  because a message is no place to carry it.
- A title or a name Minka would refuse — empty, holding a control character such
  as a line break, longer than 200 characters (a step 500) — never ends the run:
  a control character becomes a blank, what is too long is cut and ends in an
  ellipsis, and nothing becomes `(no title)` or `(no name)`; a system column
  without a name keeps Minka's. `data.titlesMadeToFit` names each record by kind
  and `sourceId` with what was done (`none`, `folded`, `cut`), never the title,
  and a `notice` counts them. A to-do that starts after the day it is due comes
  without its start, named in `data.todosStartingAfterTheyAreDue`. A comment or
  a chat line loses the control characters Minka refuses
  (`data.textsCleaned`), a project description over 2000 characters is cut at
  a word with an ellipsis (`data.descriptionsCut`), and a kind of post without
  an icon gets 📝 (`data.categoriesGivenAnEmoji`).
- A question of a check-in carries the rhythm, the weekdays counted from Monday,
  the time of day, the day the rhythm is counted from and whom it is asked of,
  which are the subscribers of the question and not whoever answered it. Only a
  member of the project who is no client, is still there and has an address can
  be asked; `data.askedLeftOut` counts the rest by reason, one reason each and
  in the order just named. `data.nobodyAsked` names every question that comes
  asking nobody at all, which will ask nobody again. A question whose schedule
  Minka cannot express is left out with its answers, and `data.questionsLeftOut`
  names it with the field that made it impossible; one whose schedule has run
  out comes paused; one asked every other week is anchored on the day it was
  last answered, which `data.anchorsInferred` names, and left out where nobody
  ever answered it.
  Comments, boosts and files hang on an answer, never on a question.
  `data.answersWithoutABody` and `data.answersTrashed` count the answers left
  out, `data.answersWithoutADay` those Basecamp names no day for, which take the
  day they were written in the zone of whoever wrote them.
- A post, a card, a to-do, a document and an upload carry `subscribers`: whom
  the source says follow the item, by `sourceId`, in the order of their IDs.
  Minka takes the list as the whole truth and takes everybody else off the item,
  the writer included, so `[]` means nobody follows it. Only a member of the
  project who is no client, is still there and has an address is named;
  `data.subscribers.leftOff` counts the rest by reason, one reason each, and
  `data.subscribers.lists` the records that carry a list. An item in the trash
  carries none and is not asked. Where Basecamp answers the list as not found or
  forbidden the record carries no `subscribers` at all, Minka follows it as it
  did before lists came, and `data.subscribers.unread` names it (`record`,
  `sourceId`, `projectSourceId`, `why`: `notFound` or `forbidden`); a `notice`
  counts them, and the run still exits 0. An answer that is not plainly the
  whole list ends the run: no list or a `null` one, an entry that is `null` or
  has no ID, or a list whose length differs from the `count` beside it. So does
  any other failure.
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
  as text and is not counted. A video or a player embedded in a text, an
  `<iframe>`, becomes `<import-embed url="…"></import-embed>` in its place:
  `data.embeds` counts them, `data.embedsLeftOut` the frames that are left
  out because Minka would not take their address, which a `notice` counts as
  well. A line that was a video alone is a message; one whose only frame is
  left out says nothing. A link to a card, a message, a chat or an entry of
  a calendar of the account, or to a line of a chat or a day of an entry,
  becomes `<import-reference kind="…" ref="…" href="…">words</import-reference>`,
  a day naming its event, whether the record is in the bundle or not and
  with or without `--calendars`, with the address as it stood and the words
  of the link as plain text: `data.references` counts them by `cards`,
  `posts`, `tools` and `events`. `data.longestBody` is the longest text per kind
  (`cards`, `comments`, `posts`, `messages`, `answers`, `todolists`, `todos`)
  in characters, and
  `data.longestBodyRecords` names under the same kinds which record it was
  measured on (`record`, `sourceId`, `in`, `inSourceId`, `characters`), so that
  a number can be looked at at the source; a kind whose longest text has no
  characters is missing there. None of them fails a run.
- Every reference to a file has a record in `attachments.jsonl` with the same
  ID (`attachment-<id>` for a file of a text, `blob-<key>` for one dropped
  into a chat) and its bytes under `files/<id>/`, whatever their size and
  type. `data.filesExpected` counts the files the references name, each
  once, and `data.filesBytesExpected` what Basecamp says they weigh: that is
  all `--dry-run` knows of them, so run it first to see what a run would
  fetch. After a real run `data.filesFetched` were downloaded,
  `data.filesReused` taken over from the bundle the run replaced, which a
  run does by itself when the file there still has the size and the digest
  of its record, `data.filesBytes` is what they weigh together, and
  `data.filesGone` counts the files Basecamp lists and no longer has: they
  have a record with `gone: true` and a `notice` counts them. Every run asks
  for such a file again and brings it when it is back; nothing has to be
  done about one that stays gone. `data.fileNamesChanged` counts the names
  that had to be made plain, `data.captionsDropped` the captions over 1000
  characters that are left off their reference,
  `data.filesOfSeveralOwners` the owners a file turned up under after the one it
  was fetched for: every owner of a file gets a record of its own, named
  `<the file>.<kind>-<owner>`, with the same bytes and one download, because a
  file stands in one place in Minka and a record naming another's file would lose
  it where the two stand in different projects. So a bundle can hold the same
  bytes twice. It counts owners met and not records written, and
  `counts.attachments` is `data.avatars.count` plus `data.filesExpected` plus
  `data.filesOfSeveralOwners`, less the entries of `data.missingFiles` whose
  `ownerKind` is not `person`. None of them fails a run. No output says the name of a file, a
  caption or an address; do not quote them either.
- A request that fails for a reason that may pass (network, rate limit, a
  5xx, 408 or 429 of Basecamp or of a gateway in front of it) is asked again
  by the command itself, up to four times with pauses of 2, 10 and 30
  seconds, and at most 100 times in one run. `data.requestsRetried` counts
  those further attempts, 0 when none was needed; it fails no run. A run that
  still ends on such a failure, or because Basecamp kept failing, is
  `retryable` (exit 6 for the network, else 7): nothing was written, run it
  again later.
- `--dry-run` walks Basecamp the same way, downloads and writes nothing, and
  answers with the same counts and with what a run would fetch. `--out` is
  not needed then.
- The bundle is built next to `--out` and takes its place at the end: a failed
  run leaves what was there. An earlier bundle of Basecamp is replaced as a
  whole; a directory that holds anything else is refused with exit 1, by
  `--dry-run --out <dir>` as well.
- One run at a time works on an `--out`: a second one exits 1 (`config`) and
  names the lock the first holds. A run that was killed holds none. `--out`
  must not be a symbolic link.
- A file that cannot be fetched does not end the run: the bundle is written
  without it and can be applied, and `data.missingFiles` names each file with
  its owner, the `reason` and **`why`**, which says whose word the loss is and
  is the field to read:
  - `failedThisTime` — it may pass. Exit 6 and `retryable`, the report as `data`
    of the error envelope. Do not treat such a run as lost: run the same command
    again and it fetches what is missing and takes the rest over.
  - `refusedBySource` — Basecamp itself has no such file or hands it to nobody.
    Where **every** missing file says this, the run ends 0, with a notice naming
    them and the count in the `summary`. No run will bring those, the records
    that stood on one are already written without it or left out, and running
    again changes nothing: look at what is named and settle it at the source.
  - `notUnderstood` — a guard of the CLI refused, or an answer nobody could
    read. Exit 7, not retryable, wherever one of them is among the files.
    **Nothing is written off**: the records that stood on such a file keep
    waiting, and a later run brings it once the reason is gone. A few of them are
    a shape nobody has met; many at once mean this CLI or the Basecamp CLI it
    reads through has changed, which is the thing to look at — not the files.
  A download that brings nothing for a minute is given up and counts
  as missing, `failedThisTime`. A
  Basecamp CLI that lost its sign-in while files were fetched ends the run
  with exit 3 and nothing written, and an avatar answered with 401 ends it the
  same way — a sign-in that is gone is no word about a picture, and every person
  of the account would lose theirs. A run that ends that way, or is
  stopped, fetches its files again: only a bundle in place hands files
  over.
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

Take a card over and write its state back:

```bash
minka cards notes show <cardId> --json > back.json
jq -rj .data.body back.json > back.md
# rewrite back.md into the state as it stands now, then send it back
minka cards notes set <cardId> - --expect "$(jq -r '.data.updatedAt // "none"' back.json)" --json < back.md
```

`jq -rj` writes the body without a line ending of its own, so what goes back is
what was read.

Finish a task and leave a comment:

```bash
minka cards done <cardId> --json
printf 'Done in %s' "$(git rev-parse --short HEAD)" | minka comments create <cardId> - --json
```
