---
name: minka
description: >
  Work with a Minka instance through the minka CLI: list projects and boards,
  read a board with its columns and cards, show a card with description and
  comments, create and edit cards, move and complete them, comment on them.
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
- `minka cards delete` moves a card to the trash and is not repeatable: a
  card in the trash reads like one that never existed, so a second delete
  exits 2. Delete only cards you created or were asked to remove.

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
| 8 | `ambiguous` | a column title matches more than one column |
| 9 | `validation` | the server refused the input, see `meta.apiCode` |

Validation codes in `meta.apiCode`: `cardTitleMissing`, `cardTitleTooLong`,
`cardTitleHasControlCharacters` (a title is one line: no line breaks or tabs),
`cardDescriptionTooLong`, `cardDescriptionNotReadable`, `commentMissing`,
`commentTooLong`, `commentHasControlCharacters` (line breaks and tabs are
fine), `nothingToChange`, `columnIdMissing`, `malformedBody`,
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
minka cards show <cardId> --json
minka cards create --board <boardId> --title "Title" [--column <column>] [--description - < body.md] --json
minka cards edit <cardId> [--title "New"] [--description - < body.md] --json
minka cards move <cardId> --column <column> [--top | --bottom | --after <cardId> | --before <cardId>] --json
minka cards done <cardId> --json
minka cards delete <cardId> --json
minka comments create <cardId> - --json < comment.txt
```

`<column>` is a column ID, a type (`triage`, `not_now`, `done`) or a title
such as `Doing`.

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

## Descriptions and comments

- Descriptions are Markdown by default and sent as HTML. Minka keeps
  paragraphs, headings `#` to `###`, bold, italic, strikethrough, inline code,
  links, lists, quotes, code blocks and horizontal rules. Deeper headings
  become level 3, tables become preformatted text, images become links, links
  other than http, https, mailto, tel and relative ones become plain text.
  These changes are reported in `notice`, and on stderr for text output and
  `--quiet`. The language of a code block is not kept.
- Markdown that holds text but nothing Minka keeps (only link reference
  definitions, for example) is refused, so it cannot remove a description by
  accident. To remove a description, give `--description ""`.
- Saving a description in the web editor currently drops horizontal rules.
- `--description-format html` sends HTML as it is; the server sanitizes it.
- `cards show --styled` prints the description as Markdown, which can be edited
  and sent back with `cards edit --description -`.
- Comments are plain text. Markdown in a comment is kept as typed, not
  rendered.

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
