# Importing from Basecamp

`minka import basecamp` is an extractor: it walks a Basecamp account and writes
a bundle in Minka's import format. The format is Minka's
(`Documentation/stable/02-architecture/import-format.md` there) and the only
thing the two sides share; Minka's applier reads the bundle, the CLI never
writes history into Minka itself.

```
minka import basecamp --project <id> [--project <id> …] --out <dir> [--dry-run]
```

The command talks to Basecamp only, and only reads there. It needs no Minka
host and no token.

## Through the Basecamp CLI

The extractor does not speak the Basecamp API itself. It runs
`basecamp api get <path> --agent` for every request (C-023): the Basecamp CLI
brings the sign-in, the refreshing of the token, rate limiting and retries,
and no Basecamp token ever passes through minka. The Basecamp CLI has to be
installed and signed in.

- The path stands behind `--` and `BASECAMP_NONINTERACTIVE=1` is set: nothing
  in a path is read as a flag, and a question would hang a run nobody watches.
- The Basecamp CLI answers one page per call and does not say whether another
  follows. Lists are read with `?page=1, 2, …` until a page comes back empty; a
  page that repeats the one before it ends the run instead of looping. A list
  that grows by exactly a page between two requests looks the same and ends
  the run as well, and the message says both. A page is read into its records
  when it arrives and let go of; of the page before it a digest is kept.
- Only paths of the account reach the Basecamp CLI. Where an answer names an
  absolute address, as a dock entry does, it has to lie under the address of
  the account and is turned into a path; anything else is refused, because the
  Basecamp CLI signs every request it makes.
- A project is named by its numeric ID of at most 18 digits. Anything else is
  a usage error before a request goes out and before a directory is made.
- The address of a project has to have the shape
  `https://<host>/<account>/projects/<id>.json`, and all projects of a run the
  same account: the account names the bundle, and its address is what every
  other address is held against.
- A run that is stopped is `interrupted`: when the context is done, or when
  SIGINT or SIGTERM ended the Basecamp CLI, which the signal may reach first.
  A Basecamp CLI that any other signal ended, such as the kill of a system out
  of memory, is an `api_error` that names the signal.

Its failures keep their class:

| Basecamp CLI | `code` | Exit |
|---|---|---|
| not installed | `config` | 1 |
| not found (2) | `not_found` | 2 |
| not signed in (3) | `auth_required` | 3 |
| forbidden (4) | `forbidden` | 4 |
| rate limit (5) | `api_error`, retryable | 7 |
| network (6) | `network`, retryable | 6 |
| anything else | `api_error` | 7 |


The exit code alone is no class: a program that crashes exits 2 as well. A
class counts only when the failure object `{ok: false, …}` stands on stdout
and its `code`, where it has one, says the same. Everything else is an
`api_error` that carries what the Basecamp CLI printed on stderr, cut to 500
bytes. Stdout is never quoted, only measured: it may hold the data of a
request that failed halfway, names and addresses included.

A bundle that cannot be written is `config`; an `--out` that holds something
else is `usage`.

What Basecamp says and a bundle cannot carry ends the run as `api_error`
instead of being passed on: a status of a project, a card, a post or a
comment other than `active`, `archived` or `trashed`, which a draft would be,
a line of a chat that is not `active`, because a message of a bundle has no
status, a record without `created_at` or `updated_at`, a time that is none
where records are ordered by it, a kind of column a bundle has no role for, a
kind of line a bundle does not know, a second Triage, Not now or Done on one
board, a boost that does not say whose it is. A card, a post and a comment
are held to this where the walk first meets them, before anything that hangs
on them is asked for, and the lines of a chat before the first of them is
written. Such a message names the record by its ID and what is missing. Of
what Basecamp said it quotes the status or the kind of column or line a
bundle does not know, and never a title, a name, a text or an address.

## What the Basecamp identity needs

The run sees what the identity sees that the Basecamp CLI is signed in as. The
same holds for an unattended job, which needs such an identity of its own.

- It may see the projects named. One it may not see is `not_found`.
- It may see the e-mail addresses of other people. Basecamp masks them with
  `•` (U+2022) for an identity that may not, which is anybody who is neither of
  the account's own company nor an admin; only its own address comes through.
  A bundle with masks in it would make people nobody can sign in as.

The people list of the account is read first and checked as a whole, before a
project is asked for, a file is fetched or anything is written, in a dry run
as well, and whoever of it comes into the bundle later:

- One masked address, anywhere in it, ends the run as `forbidden`, exit 4, and
  the hint names what the identity lacks. The same holds for the creator of a
  record who is not in the list.
- An address looks like one when it is something without whitespace, one `@`,
  and a domain of at least two labels made of letters of any script, digits
  and hyphens, none of them empty. Whitespace around it is taken away first.
- An address that is none without a mask is a flaw in the data of the source,
  which only its owner can repair there, and must not cost the import of an
  account. The person comes with `email: null`, which makes a person without a
  login; the address is not repaired. The report names them under
  `invalidAddresses`, by `sourceId` only and in the order of the numbers, a
  `notice` says how many, and the run exits 0: the bundle is whole and a
  repeat would change nothing. The same holds for a creator.
- When more than half of the addresses of the list are none
  (`sourceProblemShare`), that is no flaw in the data any more but an identity
  or a source that hides addresses, with another glyph than `•` for instance,
  and the account would come through as people without logins. The run ends
  as `api_error` with a hint to look at what the identity may see.

A message counts the addresses concerned out of those given. No output ever
names an address or whose it is, and `invalidAddresses` holds IDs, never names.

## What a bundle holds so far

The structure, the boards, the message boards and the chats:
`manifest.json`, `people.jsonl`, `projects.jsonl`, `tools.jsonl`,
`categories.jsonl`, `columns.jsonl`, `cards.jsonl`, `steps.jsonl`,
`posts.jsonl`, `messages.jsonl` or its parts, `comments.jsonl`,
`reactions.jsonl`, `attachments.jsonl` and `files/`. A file of a kind the
projects have nothing of is missing. The files of texts and chats do not come
yet, only the references to them.

The walk goes kind by kind across the projects of a run (C-028): the projects
with their tools and boards, then the message boards of all projects, then
the chats. So comments and reactions stand in the order of what they hang
on: those of cards, those of posts, those of lines.

- **People** are the ones a record of the bundle refers to, and nobody else:
  the members of the projects named and whoever a record names: its creator,
  an assignee, who completed a card or a step, who gave a boost, who is
  mentioned in a text. The people list of an account is what somebody is
  looked up in, not what is written: most of an account has no place where
  only some projects move to.
  `peopleLeftOut` in the report counts them; `counts.people` is who was
  written. The first number depends on what the list holds for the identity
  that asks, which is more for an admin than for anybody else; the people
  written do not. Referring to a person is the one way into the bundle, so every
  reference leads to a person who is written. People are written at the end
  of the run, when it is known whom the records refer to.
  What the account's list says about somebody wins over what a record says;
  somebody the list does not hold comes as the record shows them. An empty
  e-mail address becomes `null`, and so does one that is none, see above.
  `retired` follows the kind of person Basecamp names (`personable_type`),
  not the list: a `User`, and a `Client`, comes with `retired: false`, and
  every other kind with `retired: true`, a bot (`Integration`), what is left
  of somebody who was deleted (`Tombstone`), a `DummyUser`, an `Outsider` and
  a kind nobody knows yet, wherever somebody comes from. The list says
  nothing here, because it depends on who asks: for an admin it holds every
  kind of person, for anybody else users alone, and both give the same
  people. Only of somebody whose kind Basecamp does not say does the list
  decide: in the list or a member of a project is not retired, named by a
  record alone is. A bot and somebody deleted cannot be asked for one by one.
  A bot comes with `email: null` as Basecamp gives it, somebody deleted and
  an outsider with the address Basecamp keeps, which is held to the same
  rules as every address.
  A person carries `createdAt` and `updatedAt` like any record: somebody of
  whom Basecamp says neither ends the run as `api_error`, named by ID, where
  a record first refers to them.
- **Avatars** are attachments of their person, `avatar-<personId>`, downloaded
  without a sign-in and only for people of the bundle. Basecamp serves a
  picture it made from initials at the same kind of address, with the same
  headers and the same type as one somebody uploaded, so they cannot be told
  apart and every avatar is taken; the report says so
  (`avatars.initialsDistinguishable: false`). The picture is stored as it
  comes, without converting it. `file` is relative to `files/`.
- **Projects** come with their members. Basecamp names no creator of a
  project, but it makes the first tools of a project in the same moment and
  in the name of whoever created it. So `createdBy` is the creator of a tool
  that was made within five seconds after the project
  (`projectCreationWindow`), compared as moments; a tool somebody added later
  never names the creator, and without such a tool `createdBy` is `null`. The
  tools the run extracts are looked at first. Only when none of them answers
  are up to three others asked for (`maxCreatorProbes`), those that are
  switched off included, in the order of their IDs, which is the order they
  were made in, and only until one answers. Such a tool that is not there, or
  that lies outside the account, is passed over. The creator comes into the
  bundle like anybody a record names.
  The description is plain text there and HTML in the bundle: escaped, a
  paragraph per block, a `<br>` per line break.
- **Tools** are the dock entries that are switched on and that Minka has a
  counterpart for: `kanban_board` → `board`, `chat` → `chat`, `message_board`
  → `posts`, several of a kind where a project has several. Creator and
  timestamps come from the tool itself. Every other entry is counted as
  skipped, per kind and whether it was switched on, and shown in the report.
- **Columns** are the lists of a card table: `Kanban::Triage` → `triage`,
  `Kanban::NotNowColumn` → `notnow`, `Kanban::Column` → `work`,
  `Kanban::DoneColumn` → `done`. They are written in the order of a board:
  Triage, Not now, the work columns by their position, Done. Basecamp gives
  the three columns every table has no position, so `position` is the number
  of the column in that order. The colour of a column is left out.
- **Cards** are those of every column, and the archived and trashed ones.
  Basecamp lists the cards of a column itself, and the archived and the
  trashed ones of a project in one list across all its card tables, those
  that are switched off included. Such a card names its column and no table,
  so it goes to the board its column is of; a card whose column is in no
  extracted board, because the board is switched off or the column is gone,
  is left out and counted (`cardsLeftOut`).
  A walk has no snapshot: a card that is moved between two requests can be
  missed or met twice, and one met twice is written once.
  The cards of a board are written oldest first, by `created_at` and then by
  ID, across all columns and whatever their status: the applier hands out the
  short identifiers in that order. `position` counts from 1 through the
  column without a gap and is what the applier orders the column by: the
  cards by their position in Basecamp, where a card without one (in Done)
  keeps its place in the list, then the cards on hold, then the archived and
  the trashed ones as Basecamp lists them.
  A card that was set aside within its column belongs to that column, never
  to the list Basecamp keeps such cards in, and carries `onHold: true`; the
  key is left out otherwise. `completedAt` and `completedBy` come from the
  completion Basecamp records, `dueOn` as given, `assignees` in the order of
  their IDs. The note is the stored one (`content`), and `null` when empty.
- **Steps** come with the card, which shows them itself; only when it has
  more than it shows is their list asked for. Whom a step is assigned to and
  when it is due is left out, because a step in Minka has neither.
- **Categories** are the kinds of post of a project, each once: those the
  list of the project holds, and those only a post that is written shows.
  Basecamp lists the categories of the first message board of a project
  alone; a further board has its own, which are in no list and cannot be
  asked for, so such a category is what a post shows of it, and as old as the
  oldest post that carries it. `emoji` is the icon, `createdBy` is `null`.
  They are written in the order of their names, whatever the case of a
  letter, then of their IDs, and `position` is the number in that order,
  because Basecamp gives none. A category without a name ends the run as
  `api_error`, named by its ID, because the applier would stop at it; an
  empty icon comes as it is. The list has no pages and answers every page
  with all of it, so it is read once and without a page, and only for a
  project with a message board that is extracted.
- **Posts** are the messages of every message board, and the archived and
  trashed ones. Basecamp lists the messages of a board itself, and the
  archived and the trashed ones of a project in one list across all its
  boards, those that are switched off included; a message of a board that is
  not extracted is left out and counted (`postsLeftOut`). The posts of a
  board are written oldest first, by `created_at` and then by ID, whatever
  their status, the boards in the order of the tools. `title` is the subject
  of the message, because what Basecamp calls its title begins with the emoji
  of the category. `categorySourceId` is `null` for a message without a
  category. The text is the rendered one, as of a comment; an empty one comes
  as `body: ""` and is counted (`postsWithEmptyBody`): whether a post may say
  nothing is the applier's to decide.
- **Messages** are the lines of every chat: what somebody wrote, plain or
  formatted, what a bot wrote, and a file somebody dropped, see Texts.
  Basecamp lists a chat newest first, cannot be asked to turn it round and
  does not say how long it is, so a chat is read to its end and held as a
  whole, one chat at a time, and written oldest first, by `created_at` and
  then by ID (C-027). A walk has no snapshot here either: a line that arrives
  while the walk goes on moves every page, so a line can be met twice, and is
  written once, or be missed where one is deleted meanwhile. A line somebody
  wrote that says nothing is left out and counted (`messagesWithEmptyBody`):
  nobody comes into the bundle for it, its boosts are not asked for and no
  part ends at it. Nothing is what a reader sees nothing of: whitespace,
  characters without a width, `<br>` and empty `<div>` and `<p>`. A
  reference to a file or a person, a link, a rule and an embedded frame say
  something. A line comes as long as it is: nothing is shortened.
- **Parts.** The messages are written in parts: `messages-0001.jsonl` and on,
  numbered through the bundle in the order of the chats, which is the order
  of the projects and their tools. A part holds lines of one chat. It ends
  once it holds 10,000 lines, but only where the second changes that the
  lines were created in, so it may hold somewhat more: the applier keeps the
  lines of one second in the order of their file by counting them, anew in
  every part. The second is that of the moment, whatever zone and fraction a
  time is written with. A chat without a line has no part and leaves no gap
  in the numbers. The manifest names the parts in their order under
  `parts.messages`, and `counts.messages` counts all lines. Only a bundle
  whose lines came to a single part carries the one file `messages.jsonl`
  instead, and its manifest has no `parts`. A bundle never carries both. The
  writer of a bundle does this for any kind that is said to come in parts: it
  counts, and whoever adds the records says where a part may end.
- **Comments** are those of cards and of posts, `parentKind` `card` or `post`,
  per parent oldest first, by `created_at` and then by ID: the applier reads
  the order of the comments of a parent from the file. Basecamp hands out the
  rendered text of a comment only. A comment in the trash is left out with
  its boosts and counted (`commentsTrashed`); any other comes, the archived
  comment of an archived card as well.
- **Reactions** are the boosts of a card (`targetKind` `card`), of a post
  (`post`), of a comment (`comment`) and of a line (`message`), with `content`
  as it is, a text of some words included: what Minka takes for a reaction is
  the applier's to decide. The boosts of a card or a post stand before those
  of its comments. A boost has one time, which is both `createdAt` and
  `updatedAt`.

Left out besides: the colours, wormholes between boards, who follows a card
or a post and whom a post told, and being on hold as anything more than the
flag.

What Basecamp does not hand out, so that no bundle carries it:

- Whether a message is pinned, and since when. `pinnedAt` is `null` on every
  post.
- A draft of a message. It is in no list; a status that says so would end the
  run.
- A line somebody deleted, and the lines Basecamp writes itself, such as who
  joined a chat.

IDs are the numeric IDs of Basecamp as strings; `sourceInstance` is the ID of
the account, taken from the address of a project. Times are written the way
Basecamp writes them, milliseconds included. People, projects, tools and
attachments are ordered by numeric ID, steps, comments and reactions by their
parents in the order of their file and then by position or age, whatever
order Basecamp answers in: two runs over an unchanged account write the same
bytes, apart from `extractedAt`. Only where Basecamp gives cards no position,
in Done and among the archived and the trashed, does its own order count.

A card, a post and a line cost no request of their own: a list shows them in
full. Comments are asked for only where a card or a post says it has some,
boosts only where a card, a post, a comment or a line says so. The requests
go one after the other, never two at a time: the Basecamp CLI keeps to the
rate Basecamp allows.

### Texts

A bundle never carries the embeds of its source. In the note of a card, the
body of a post, of a comment and of a line, every `<bc-attachment>` is
replaced as a whole, with the figure Basecamp rendered into it:

- A mention becomes `<import-mention ref="<personId>">Name</import-mention>`,
  with the name the list of the account has. The person is looked for in the
  signed ID of the embed first, then in the avatar Basecamp rendered, then in
  what the editor wrote next to it (C-026). Only somebody the list of the
  account knows becomes a reference, and comes into the bundle like anybody a
  record names. Any other mention stays the name that stood in the text and
  is counted (`mentionsUnresolved`); for the stored note of a card, which
  shows no name, the name comes from the rendered one.
- A file becomes `<import-attachment ref="attachment-<id>"></import-attachment>`
  with the ID of the file Basecamp lists next to the text (`filesInBodies`).
  The listed file is found by the signed ID of the embed, then by the key of
  the stored file, which stands in the address of the embed
  (`…/blobs/<key>/download/…` in `href`, else `url`), then by the ID of the
  attachment the signed ID names. An older text carries the signed ID of its
  day and the list the one of today, so the first alone would lose such
  files. What an embed names counts only when a listed file confirms it: the
  listed file is what says where the file is fetched from. An embedded file
  that is not listed cannot be named: its caption stays as text, for the
  stored note of a card the one of the rendered note, the rest is dropped, it
  is counted (`filesDropped`) and a `notice` says so. No attachment record is
  written for a file of a text yet, and nothing is downloaded.
- A horizontal rule becomes `<hr>`.
- The preview of a link (`application/vnd.basecamp.opengraph-embed`) becomes
  the link, `<a href="…">title</a>`, with the address as the text where the
  preview has no title (`linkPreviews`). The address is the `href` of the
  embed, else its `url`. Only an absolute address over http or https becomes
  a link: of any other, the title stays as plain text, or nothing, and it is
  not counted.
- A picture at a foreign address, an embed of the kind `image` without a
  signed ID, is no stored file. It becomes a link to its `url` with its
  `caption` as the text, by the rules of the preview of a link, and never a
  picture: a bundle fetches nothing from an address somebody wrote into a
  text (`picturesLinked`, which counts what became a link). A picture with a
  signed ID, and one of a kind such as `image/png`, is a file as above.
- An embed of any other kind that carries no signed ID stays as it is, and so
  does what is no embed at all, such as a `<figure>` with the `<iframe>` of a
  video.

A line of a chat that is a file somebody dropped has no text. Its body is one
`<import-attachment ref="blob-<key>"></import-attachment>` per file, in the
order of the line, and counts as `filesInBodies`. Basecamp gives such a file
no ID, so the key of the stored file names it, which stands in the address it
is downloaded from (`…/blobs/<key>/download/…`), else in the one it is
previewed at (`…/blobs/<key>/previews/…`). A key is made of letters, digits,
`-` and `_`, because it becomes part of the ID of an attachment. A file
without such a key is dropped and counted (`filesDropped`); a line that keeps
no file is left out with its boosts and counted (`uploadLinesLeftOut`), and
nobody comes into the bundle for it. A plain line and the line of a bot go
the way of every text and, holding no embed, pass byte for byte.

The kind of an embed is its content type without parameters, in any case. A
signed ID is base64, `--` and a signature, which stands behind the last `--`
and is not checked. Today the base64 holds an envelope Ruby serialised, with
the global ID (`gid://bc3/Person/<id>?expires_in`) readable in it; an older
one holds JSON, `{"_rails": {"message", "exp", "pur"}}`, whose `message` is
the serialised ID as base64 again, in the usual alphabet or the one for
addresses, with or without padding. A global ID counts as a whole: nothing of
a word before it, nothing of a number or a path after it.

Every other token passes through with the bytes it was read from: Minka
sanitises a body, and the extractor adds no second opinion. What goes besides
an embed is what belongs to one: whitespace between an embed and its
rendering, an end tag of an embed without its beginning, and a tag the text
breaks off in. A text is read token by token, never parsed into a tree,
because a tree moves a figure out of a paragraph; an embed that is never
closed costs no more of the text than its figure, and one that closes itself
nothing.

A further kind is a further step of the walk that adds its records to the same
writer, feeds the creators it meets into the same people and attaches its
files the same way as an avatar is attached.

## A run

- The bundle is built in a directory next to `--out`,
  `.<name>.staging-<random>`, its parts included, and takes its place with a
  rename at the end; the bundle of the run before is moved aside first and
  removed last (C-024).
  A run that fails or is interrupted, by Ctrl-C or by SIGTERM, leaves what was
  there: the earlier bundle, or nothing, not even the directories it made on
  the way to `--out`.
- When the new bundle cannot take its place, the old one is put back; when
  that fails too, the error names where it lies. An old bundle that cannot be
  removed afterwards is a `notice` with its path, not a failure.
- A run that is killed cannot tidy up. The next run into the same `--out` does:
  it removes what was being built, which is personal data nobody will use, and
  puts a bundle back that was moved aside when no new one took its place. It
  touches only real directories named exactly `.<name>.staging-<digits>`, with
  or without `.previous`: the name of `--out` never works as a pattern, and a
  link is left alone.
- One run at a time works on an `--out`. A run holds an advisory lock of the
  kernel (`flock`) on `.<name>.lock` next to it, from before the tidying up
  until the bundle is in place, and removes the file at the end. A second run
  is refused as `config` with the path of the lock. The kernel drops the lock
  with the process, so a run that was killed locks nobody out; it leaves the
  empty file behind, which the next run takes over and removes. minka is
  released for macOS and Linux; on a system without `flock` it builds and
  does not lock.
- An `--out` that is a symbolic link is refused as `config`: the bundle would
  take the place of the link, and the one behind it would stay where nobody
  looks for it.
- A bundle of an earlier run in `--out` is replaced as a whole. It counts as
  one only when its `manifest.json` says `format` 1 and the same `source`: a
  directory with anything else in it is refused, a web application with a
  manifest of its own and a bundle of another source included, and the refusal
  names the first entries that are in the way. The check
  happens before Basecamp is asked and again before the replacing.
- `--dry-run` walks Basecamp the same way, downloads no avatar, writes nothing
  and reports the same counts and the same parts. It needs no `--out`; given
  one, it checks it the way the real run does, so the refusal shows
  beforehand.
- The report carries `counts` as the manifest has them, `avatars`,
  `skippedTools`, `missingFiles`, `cardsByStatus`, `cardsOnHold`,
  `cardsLeftOut`, `commentsTrashed`, `postsByStatus`, `postsLeftOut`,
  `postsWithEmptyBody`, `messagesByType` (`text`, `richText`, `upload`,
  `integration`), `messagesWithEmptyBody`, `uploadLinesLeftOut`, `parts` as
  the manifest has them, `mentionsUnresolved`, `filesInBodies`,
  `filesDropped`, `linkPreviews`, `picturesLinked`, `longestBody` and the
  absolute path of the bundle. `longestBody` is the longest text per kind,
  `cards`, `comments`, `posts` and `messages`, in characters: whoever sets
  the limits of where a bundle goes has to know it.
  No output names a title, a person or what stands in a text.
- A run prints nothing while it goes on, in any mode: the output contract
  knows results, warnings and errors only.

## Files that cannot be fetched

One file that is not to be had does not cost a run (C-025). Its attachment
record is left out, whatever pointed to it points to nothing (`avatar: null`),
and the bundle is written and put in place: it is whole and can be applied.
`missingFiles` names each file with `attachmentSourceId`, `ownerKind`,
`ownerSourceId`, `reason` and `retryable`. The rule holds for every file of a
bundle, not for avatars only. Only an interruption ends the run.

Such a run fails, so that a caller notices: `network`, exit 6 and retryable
when at least one file failed for a reason that may pass by itself, else
`api_error`, exit 7. The report is in the output all the same: as `data` of
the error envelope with `--json` and `--quiet`, as text before the error
otherwise. A later run replaces the bundle and brings what it can get then.

A file is fetched over https only, redirects included and at most five of
them, with a timeout of 30 seconds and at most 5 MiB, which is enforced while
reading. It has to be a picture of a known type: an error page must not end up
in a bundle. A reason never carries the address of the file, which names a
person.

## A bundle is personal data

A bundle holds names, e-mail addresses and pictures. Its directories are 0700
and its files 0600. It is written outside of any repository and deleted once
it has been applied. Nothing out of a real bundle appears in the repository:
not in a test, a fixture, a document or a commit message. Tests run against
`internal/fakebasecamp`, an invented account, and never call the Basecamp CLI
or the network.
