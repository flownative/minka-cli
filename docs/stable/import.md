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
  page that repeats the one before it ends the run instead of looping.
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
instead of being passed on: a status of a project, a card or a comment other
than `active`, `archived` or `trashed`, a record without `created_at` or
`updated_at`, a time that is none where records are ordered by it, a kind of
column a bundle has no role for, a second Triage, Not now or Done on one
board, a boost that does not say whose it is. A card and a comment are held
to this where the walk first meets them, before anything that hangs on them
is asked for. Such a message names the record by its ID and what is missing.
Of what Basecamp said it quotes the status or the kind of column a bundle
does not know, and never a title, a name, a text or an address.

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

The structure and the boards: `manifest.json`, `people.jsonl`,
`projects.jsonl`, `tools.jsonl`, `columns.jsonl`, `cards.jsonl`,
`steps.jsonl`, `comments.jsonl`, `reactions.jsonl`, `attachments.jsonl` and
`files/`. Chats and message boards come as tools only, and the files embedded
in a text not yet.

- **People** are the ones a record of the bundle refers to, and nobody else:
  the members of the projects named and whoever a record names: its creator,
  an assignee, who completed a card or a step, who gave a boost, who is
  mentioned in a text. The people list of an account is what somebody is
  looked up in, not what is written: most of an account has no place where
  only some projects move to.
  `peopleLeftOut` in the report counts them; `counts.people` is who was
  written. Referring to a person is the one way into the bundle, so every
  reference leads to a person who is written. People are written at the end
  of the run, when it is known whom the records refer to.
  Who is in the account's list comes with `retired: false`, and so does a
  member of a project. Somebody only a record names, as its creator for
  instance, has left the account and comes as the record shows them, with
  `retired: true`. An empty e-mail address becomes `null`, and so does one
  that is none, see above.
- **Avatars** are attachments of their person, `avatar-<personId>`, downloaded
  without a sign-in and only for people of the bundle. Basecamp serves a
  picture it made from initials at the same kind of address, with the same
  headers and the same type as one somebody uploaded, so they cannot be told
  apart and every avatar is taken; the report says so
  (`avatars.initialsDistinguishable: false`). The picture is stored as it
  comes, without converting it. `file` is relative to `files/`.
- **Projects** come with their members. Basecamp names no creator, so
  `createdBy` is `null`. The description is plain text there and HTML in the
  bundle: escaped, a paragraph per block, a `<br>` per line break.
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
- **Comments** are those of cards, `parentKind` `card`, oldest first. Basecamp
  hands out the rendered text of a comment only. A comment in the trash is
  left out with its boosts and counted (`commentsTrashed`); any other comes,
  the archived comment of an archived card as well.
- **Reactions** are the boosts of a card (`targetKind` `card`) and of a
  comment (`comment`), with `content` as it is, a text of some words included:
  what Minka takes for a reaction is the applier's to decide. A boost has one
  time, which is both `createdAt` and `updatedAt`.

Left out besides: the colours, wormholes between boards, who follows a card,
and being on hold as anything more than the flag.

IDs are the numeric IDs of Basecamp as strings; `sourceInstance` is the ID of
the account, taken from the address of a project. Times are written the way
Basecamp writes them, milliseconds included. People, projects, tools and
attachments are ordered by numeric ID, steps, comments and reactions by their
parents in the order of their file and then by position or age, whatever
order Basecamp answers in: two runs over an unchanged account write the same
bytes, apart from `extractedAt`. Only where Basecamp gives cards no position,
in Done and among the archived and the trashed, does its own order count.

A card costs no request of its own: a list shows its cards in full. Comments
are asked for only where a card says it has some, boosts only where a card or
a comment says so.

### Texts

A bundle never carries the embeds of its source. In the note of a card and
the body of a comment, every `<bc-attachment>` is replaced as a whole, with
the figure Basecamp rendered into it:

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
- A horizontal rule becomes `<hr>`. An embed of any other kind that carries
  no signed ID stays as it is.

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
  `.<name>.staging-<random>`, and takes its place with a rename at the end;
  the bundle of the run before is moved aside first and removed last (C-024).
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
  and reports the same counts. It needs no `--out`; given one, it checks it
  the way the real run does, so the refusal shows beforehand.
- The report carries `counts` as the manifest has them, `avatars`,
  `skippedTools`, `missingFiles`, `cardsByStatus`, `cardsOnHold`,
  `cardsLeftOut`, `commentsTrashed`, `mentionsUnresolved`, `filesInBodies`,
  `filesDropped` and the absolute path of the bundle. No output names a
  title, a person or what stands in a text.
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
