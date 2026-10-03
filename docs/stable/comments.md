# Comments on a card

The comments on a card are read with `cards show` and `comments list`, the ones
not yet shown with `comments unread`, and written with `comments create`,
`edit` and `delete`; `react` and `unreact` leave a reaction. A comment can be
written so that it waits for the comments before it to be read
([C-101](../decisions.md#c-101)).

Everything here was read in Minka's code — `BoardApiController::cardResource`,
`writeCommentAction`, `CommentQueryService::findCommentsOn`, `CommentResource`,
`IsUuid::create` and the migration that makes `minka_comment`.

## What a read hands over

Minka has no address that lists the comments of a card. They come with the
card, all of them and unpaged, oldest first: ordered by `createdAt`, which it
keeps to the microsecond, and then by the ID, which it makes from the clock in
milliseconds. The last comment of the card is therefore the newest there is,
and the CLI takes that order as it comes instead of sorting again.

`comments list` hands over `data` as the list of the comments as they were
served, `comments unread` the part of it not yet shown. The text view of both
names each comment with its ID, which the guard below takes.

## Reading before writing

A comment answers what was said before it. Two guards keep one from being
written past an answer its writer has not read. Both are opt-in: whoever uses
neither notices nothing.

**`--if-latest <commentId|none>`** writes only while that comment is the newest
on the card, `none` only while the card has none. Otherwise nothing is written:
the comments after it are handed over in `data` and shown in the text, the
command exits with **11** (`unread`), and the `hint` names the newest comment to
guard the next try with. A comment the card no longer has — removed, or never on
it — leaves nothing to tell older from newer, so every comment of the card is
shown and the message says that the comment is gone; on a card with no comments
left the refusal stands as well, and names `none` as the guard to take. A changed comment keeps its ID and its place, so an edit is never new.
`--if-latest` and `--force` exclude each other.

The guard is a read of the card followed at once by the write; Minka has no
condition the write could carry. A comment that arrives between the two is
written past all the same. The window is the time of one request.

**`comments.requireRead`**, switched on with `minka config set
comments.requireRead true`, makes every `comments create` without `--if-latest`
read the card first and refuse in the same way while the card has comments not
yet shown on this machine — exactly those are shown. `--force` writes all the
same. Off, as it is by default, a comment reads nothing first. A token from
`$MINKA_TOKEN` needs no configuration file, so where that file cannot be read
the setting counts as off and a `notice` says so; with a token from the file,
the file is needed anyway and its refusal stands.

A refusal of either guard shows the comments it names, so they count as read:
reading them in the refusal is the point, and the next try goes through unless
somebody wrote again.

## What counts as read

A comment counts as read once this machine has shown it in full —
`cards show`, `comments list`, `comments unread`, or the refusal of a guard — or
written it. A comment guarded by `--if-latest` that went through marks the
comment it named as well, and nothing else: a comment before it, one imported
with an earlier moment for instance, may never have been shown to its writer.

The marks lie in `comments-read.json` beside the configuration file, 0600 in a
directory of 0700, per host, per reader and per card. A reader is a digest of
the token: two people signed in on one machine read for themselves, and a new
token for the same person starts with every comment unread. Every session of
one token on the machine shares the marks, the way it shares the list of cards
at work.

- The mark of a card is the **set of comments known**, not the newest one. A
  comment that comes in between the read and the write of a guarded comment, or
  one written with an earlier moment, as an import does, stays unread.
- Marks are only ever added to, under an advisory lock of the kernel on
  `comments-read.lock`, and the file is replaced whole by a rename. A writer
  waits up to 5 seconds for the one before it. Each holds the lock for one read
  and one write of this file and never across a request, so that is plenty
  while several write at once; a holder that is stuck costs the marks of that
  command, with the notice below, and never the read itself. A reader takes no
  lock.
- A read that adds no comment to a mark written less than a day ago leaves the
  file alone, so reading a card again costs no write.
- The mark of a card not written for 90 days is dropped with the next write,
  so the file does not grow with every card ever read. Such a card has every
  comment unread again, which says too much rather than too little.
- A mark that cannot be read or written fails nothing. A read still shows what
  it read, with a `notice` that the comments count as unread again; a guard
  that cannot read the marks takes every comment as unread, and `--force` is the
  way past it.

## Settings

`minka config get <key>` and `minka config set <key> <value>` read and change
the settings of the configuration file, for every host on the machine. Hosts
and tokens are not among them; `auth` keeps those. So far there is one:

| Key | Values | Default |
|---|---|---|
| `comments.requireRead` | `true`, `false` | `false` |

## Only cards

The guards stand on the comments of a card. The comments under posts, to-dos,
to-do lists, events, documents and uploads have none: the marks are kept by an
ID of the item and would carry them, but nobody asked for them there.
