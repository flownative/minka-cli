---
name: minka
description: >
  Work with a Minka instance through the minka CLI: list projects and boards,
  read a board with its columns and cards, show a card with description and
  comments, create and edit cards, move, hold and complete them, comment on them,
  read and write chat messages, and say which cards you are working on right
  now; also documents and files, to-dos, check-ins, posts, events, reminders
  and the import from Basecamp.
  Use for any task on a Minka board, such as tracking development tasks, and
  always before taking up a card, moving it, commenting on it or writing its
  working notes, and when coordinating a board or waiting for work on one.
---

# minka

`minka` talks to the JSON API of a Minka instance. Every command prints JSON
when stdout is not a terminal, so an agent gets structured output by default.

## Agent invariants

**Before you take up a card, comment on one or stay on a board, read
[references/working-on-a-board.md](references/working-on-a-board.md):** how to
work on a board so that its people can follow and steer the work. This file
holds the commands an agent needs every day; what else there is stands in the
references listed under "Where the rest stands".

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
- To wait for news — a card moved, a comment, an assignment — never poll a
  board: run `minka notifications wait --json` (in Claude Code with
  `run_in_background`), which exits when something arrives. Waiting marks
  nothing read; `minka notifications mark-read <id>` does, with the `id` of a
  line. Coordinating a board and the whole flow:
  [references/waiting-and-coordinating.md](references/waiting-and-coordinating.md).
- Keep the work steps of a card as steps (`minka cards steps create <card> "…"`,
  `minka cards steps complete`), never as a list in the card's description or a
  comment: a step can be ticked off, carries people and a day, and the board
  counts it.
- Take a card up from its back: `minka cards notes show <card>` before you
  begin, because whoever was on it before left the state of the work there,
  and `minka cards notes set <card> - --expect <updatedAt>` at every milestone,
  so that the next session can take the card over. Research, concepts and
  screenshots go on the back as well, with `minka cards notes attachments
  create <card> <file>`, never into a file only your machine has. Comments are
  the other thing — the conversation with people. How to keep the back: see
  [references/working-notes.md](references/working-notes.md).
- When a comment, a description or a message points at another card, write it
  as `#MNK-1`; point at any other entry by pasting its bare address, the
  `appUrl` that `--json` gives for it. Only these forms become a reference:
  Minka links it, and in a project the page of the card, post, to-do, to-do
  list, document, file, check-in question or event named lists it among what
  points at it (on the web; a chat or a chat line lists nothing, and nothing
  written in a private conversation is listed anywhere). Never write `MNK-1`
  without `#`, never a Markdown link with words of its own such as
  `[MNK-1](https://<host>/go/card/MNK-1)`, and never a title alone: each of
  them stays plain text or a plain link, and nothing points back. Example:
  `Blocked by #MNK-AS; the spec is in https://<host>/projects/…/posts/…/<postId>`.
  See [references/texts.md](references/texts.md), "Pointing at a card or
  another record", for every kind of address. Outside Minka, in a commit or a
  pull request, a card is named by its short address and one comment by its
  `appUrl`, `…#comment-<id>`: the same section says how.
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

At most once a day an envelope, of a success or an error, carries
`cliUpdate: {current, latest, minimum}`: a newer minka exists. Finish the task
as usual. If you talk to a person, mention it once per conversation at the
end, briefly: `brew upgrade minka` brings better compatibility with Minka and
new features. Never upgrade by yourself, and never write it into a comment, a
card or a commit.

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
| 12 | `timeout` | `minka notifications wait --timeout` ran out with nothing new; wait again |

Validation codes in `meta.apiCode`: `cardTitleMissing`, `cardTitleTooLong`,
`cardTitleHasControlCharacters` (a title is one line: no line breaks or tabs),
`cardDescriptionTooLong`, `cardDescriptionNotReadable`, `commentMissing`,
`commentTooLong`, `commentHasControlCharacters` (line breaks and tabs are
fine), `commentNotReadable`, `cardDueDateNotADate`, `personNotAProjectMember`, `personIdMissing`,
`stepMissing`, `stepTooLong`, `stepHasControlCharacters`, `stepCompletedMissing`,
`stepDueDateNotADate`, `stepPlaceUnknown`, `cardArchived`,
`chatMessageMissing`, `chatMessageTooLong`, `chatMessageHasControlCharacters`,
`chatMessageNotReadable`, `chatMessageDraftNotAnIdentifier`, `chatMessagesLimitNotANumber`,
`chatReadSeqMissing`, `nothingToChange`, `columnIdMissing`, `onHoldEnabledMissing`,
`columnHasNoOnHoldArea`, `systemColumnHasNoOnHoldArea`, `malformedBody`,
`unsupportedMediaType`, `projectArchived`, `coordinatorTaken`,
`coordinatorSessionInvalid`, `cursorUnknown`, `notificationsWaitInvalid`,
`notificationsLimitInvalid`.

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
  valid until it is revoked on the server. `--revoke` revokes it there first
  and removes the entry only then; when the revocation fails the entry is kept
  and the exit code is the failure's (a `retryable` one may simply be run
  again). A token Minka itself no longer accepts counts as revoked, with a
  `notice`; a 401 of a proxy in front of Minka does not. A token in
  `$MINKA_TOKEN` is never revoked, and a `notice` says so. `data` is `{host,
  removed, revoked}`.
- `minka config get <key> --json` and `minka config set <key> <value> --json`:
  the settings of this machine, so far `comments.requireRead`.

## This skill

The binary carries this description, so it fits the commands of the version
that is installed.

- `minka skill show --styled`: print it.
- `minka skill install`: write it to `~/.claude/skills/minka/SKILL.md`, with
  its references beside it, or into another directory of skills with
  `--dir <dir>`. A directory of
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
minka cards move <cardId> --column <column> [--on-hold] [--top | --bottom | --after <cardId> | --before <cardId>] --json
minka cards done <cardId> --json
minka people list <projectId> --json
minka cards assign <cardId> <person> --json
minka cards unassign <cardId> <person> --json
minka cards notes show <cardId> --json
minka cards notes set <cardId> - --expect <updatedAt|none> --json < notes.md
minka cards steps list <cardId> --json
minka cards steps create <cardId> "Text" --json
minka cards steps edit <cardId> <step> --assignee me --due 2026-10-09 --json
minka cards steps edit <cardId> <step> --title "Text" --json
minka cards steps move <cardId> <step> (--top | --bottom | --after <step> | --before <step>) --json
minka cards steps complete <cardId> <step> --json
minka cards steps reopen <cardId> <step> --json
minka cards steps delete <cardId> <step> --json
minka comments list <cardId> --json
minka comments unread <cardId> --json
minka comments create <cardId> - [--if-latest <commentId|none> | --force] --json < comment.txt
minka comments edit <cardId> <commentId> - [--format html] --json < comment.txt
minka comments delete <cardId> <commentId> --json
minka comments react <cardId> <commentId> 👍 --json
minka comments unreact <cardId> <commentId> 👍 --json
minka notifications wait [--board <boardId>]... [--column <column>]... [--timeout 30m] --json
minka notifications count --json
minka notifications show <notificationId> --json
minka notifications mark-read <notificationId> --json
minka work start <card> [--session <id>] --json
minka work stop [<card>] [--all] [--session <id>] --json
minka work list [--session <id>] --json
minka search <term>... (--folder <folderId> | --tool <toolId> | both) [--limit N] --json
```

`<column>` is a column ID, a type (`triage`, `not_now`, `done`) or a title
such as `Doing`.

## Where the rest stands

This file holds what an agent on a board needs every day. Everything else
stands in a reference beside it; where a reference has commands of its own,
they stand at its top. Read the one you need, not all of them.

| For | Read |
|-----|------|
| How to work on a board: the roles of the columns, taking up a card, asking a person, handing over | [references/working-on-a-board.md](references/working-on-a-board.md) |
| Waiting for news instead of polling, a line of the bell, coordinating a board | [references/waiting-and-coordinating.md](references/waiting-and-coordinating.md) |
| Your own cards and the trash, projects, boards and short identifiers, columns and their commands, cards on hold, the archive of a board, people, assignments and subscribers, steps, due dates | [references/cards.md](references/cards.md) |
| The back of a card: how to keep the working notes, their commands, the attachments | [references/working-notes.md](references/working-notes.md) |
| Descriptions and comments in detail, reactions, naming somebody in a text, pointing at a card or another record, callouts, diagrams, link tiles and variables | [references/texts.md](references/texts.md) |
| Saying which cards you are working on: `work`, `heartbeat`, the mod | [references/work-and-heartbeat.md](references/work-and-heartbeat.md) |
| Chats of a project and private conversations | [references/chats.md](references/chats.md) |
| Putting a file into a text, fetching a file back | [references/files.md](references/files.md) |
| Docs & Files: documents, uploads, folders, versions, the trash and the archive of a tool, searching a tool or a folder with `minka search` | [references/docs-and-files.md](references/docs-and-files.md) |
| To-do lists, groups, to-dos, subtasks, repeating to-dos | [references/todos.md](references/todos.md) |
| Check-ins: questions and answers | [references/checkins.md](references/checkins.md) |
| Posts of the message board | [references/posts.md](references/posts.md) |
| Calendars and events | [references/calendars.md](references/calendars.md) |
| Reminders ("Remind me") | [references/reminders.md](references/reminders.md) |
| Importing from Basecamp | [references/basecamp-import.md](references/basecamp-import.md) |

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
LATEST=$(minka comments list <cardId> --quiet | jq -r 'last.id // "none"')
printf 'Done in %s' "$(git rev-parse --short HEAD)" | minka comments create <cardId> - --if-latest "$LATEST" --json
```
