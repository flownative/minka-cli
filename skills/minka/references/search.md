# Search

```bash
minka search <term>... [--kind <kind>[,<kind>]]... [--project <projectId>] [--sort best|recent] [--limit N] [--offset <next>] --json
minka search <term>... (--tool <toolId> | --folder <folderId> | both) [--kind files] [--sort best|recent] [--limit N] [--offset <next>] --json
minka search [flags] --json -- -<word> <term>...
```

`minka search` finds what you may read, the way the search of Minka on the
web does: cards, comments, posts, to-do lists, groups and to-dos, folders,
documents, uploads, events, the lines of the chats of your projects and of
your own private conversations. Drafts, the trash, archived projects and lines
taken back are never found.

Search before you ask a person or create something: whether a card for the
work exists already, where a decision was written down, which document holds
a passage. It is cheaper than walking boards, lists and folders.

## Asking

1. The term: `minka search tent costs --json`. The words are one term, joined
   by a space; double quotes inside it ask for a phrase (`minka search '"site
   plan"'`), a word with a dash in front leaves out what holds it. A term that
   starts with a dash goes last, behind `--`, and every flag, `--json`
   included, has to stand before the `--`: `minka search --kind todos --json
   -- -draft tent`. Minka cuts a term at 200 characters; a `notice` says so.
2. Narrow it. `--kind` takes `board`, `posts`, `todos`, `files`, `calendar`,
   `chat` and `conversation`, repeated or separated by commas
   (`--kind board,posts`); a comment counts toward the tool of what it stands
   under. Any other word exits 1 before any request. `--project <projectId>`
   narrows to one project and leaves private conversations out.
3. Within Docs & Files: `--tool <toolId>` searches the documents and uploads of
   a tool, `--folder <folderId>` those of a folder and everything beneath it,
   both a folder within a tool. Use it rather than walking `docs list` when you
   look for a passage of a long text, such as a chapter of a handbook. Beside
   them only documents and uploads are found: `--kind` has to name `files`
   there, and `--project` is refused, since a tool lies in one project
   already. Both exit 1 before any request.

## Reading the hits

`data.hits` holds each hit as Minka sent it: `{kind, id, targetKind, targetId,
title, excerpt, section, projectId, toolId, project, shortId, shortUrl,
updatedAt, archived, appUrl, apiUrl}`.

- `kind` is `card`, `comment`, `post`, `todoList`, `todo`, `todoGroup`,
  `folder`, `document`, `upload`, `event`, `chat` or `conversation`.
- `id` is the hit itself: the comment, the message. `targetKind` and
  `targetId` are what it leads to: the card, post, to-do, list, document,
  upload or event a comment stands under, the chat or conversation of a line,
  else the item itself.
- `title` is the item's; for a comment the one of the item it stands under,
  for a line of a chat the chat's, for a private conversation the other
  person's name. `project` is the project's name; `projectId`, `toolId` and
  `project` are `null` for a private conversation.
- `shortId` is set for a card, and for a comment under one, where the board
  shows short identifiers: write it as `#MNK-1` in a text.
- `excerpt` is the plain text around the match. A document hit names the
  heading its match stands under in `section` (`{heading, anchor}`), and
  `appUrl` then ends in `#<anchor>`, such as `#h-setup-2`; a comment's ends in
  `#comment-<id>`, a line's in `#chat-message-<id>`. `appUrl` is the address
  to hand a person.
- `archived` is true for an item in the archive, or below an archived folder
  or list: it can be read and takes no write.

The text names each hit on one line: its ID, its kind and title, `#MNK-1`,
`› Heading`, `(in Project, archived)`; `comment on card …`, `chat line in …`,
`message with …`.

## Opening a hit

The first `show` breadcrumb reads what the first hit leads to. By
`targetKind`: `minka cards show`, `posts show`, `todolists show`, `todos show`,
`docs folders show`, `docs show`, `docs uploads show` or `events show` with
`targetId`. A to-do group has no address of its own: read its list. A line of a
chat or a conversation is read in its transcript with `minka messages list
<targetId>`, which starts at the newest end; `appUrl` opens it at the line on
the web.

## More hits

`--sort` is `best` (the default) or `recent`; any other word exits 1, because
Minka would take it as `best` without a word. Minka serves 25 hits a page.
`--limit N` (25 unless given) is how many hits to read: the command reads
further pages only while it lacks hits, and stops inside a page where the
limit falls. Where more are left, `data.next` is a number in a string, and the
`next` breadcrumb and a `notice` give the whole command to read on, to which
flags can be appended; hand `next` to `--offset` exactly as it came. Without
more, `next` is `null`.

Minka serves no hits beyond the 525th (offset 500). The command reads no
further, `next` is `null` there, and a `notice` says that there may be more
and to narrow the search with `--kind`, `--project`, `--tool`, `--folder` or a
sharper term. `--offset` above 500 exits 1. Hits can move between two pages
while somebody writes: a run is no picture of one moment.

## Refusals

A project, tool or folder that is not there, that you may not read, or a
folder outside the tool `--tool` names, exits 2 (`not_found`). A Minka that
searches only within a Docs & Files tool or folder answers a search without
`--tool` or `--folder` with exit 2 as well, saying so: give `--tool` or
`--folder` there.
