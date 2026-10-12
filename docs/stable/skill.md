# The skill

`skills/minka/SKILL.md` is the contract for agents (C-014). It travels inside
the binary, so that what an agent reads fits the commands of the version that
is installed (C-016). The references beside it under
`skills/minka/references/` travel, install and count with it.

## A small core and its references

An agent loads SKILL.md whole whenever the skill applies, so it holds only what
an agent on a board needs every day (C-126): the invariants, output modes,
exit codes, introspection, the commands for cards, boards, columns, steps,
comments, notifications and `work`, and a table that names, for each subject,
the reference to read. Its `description` names taking up, moving and
commenting on a card and coordinating or waiting on a board, so that the skill
is loaded in exactly those moments.

Everything else stands in a reference of its own subject: the detail of cards,
the back of a card, texts, saying what one works on, chats, files, Docs &
Files, to-dos, check-ins, posts, calendars, reminders, the Basecamp import,
waiting and coordinating, and how to work on a board. Where a reference has
commands of its own, they stand at its top. SKILL.md points to every
reference, and a pointer between the files names the file it leads to and,
where it names a section, one the file has; a test holds all of it against
the files the binary carries.

A reference is never renamed or removed lightly: `skill install` leaves a file
it no longer carries where it is, so an installed copy would keep it, stale.

## Commands

| Command | Does |
|---|---|
| `skill show` | prints the skill the binary carries; `--json` hands over the references in `data.references` |
| `skill install [--dir <dir>]` | writes it to `<dir>/minka/SKILL.md` and its references beside it; without `--dir` to `~/.claude/skills`, where Claude Code reads it for every project |
| `skill status [--dir <dir>]` | `data.state`: `current`, `differs` or `not installed` |

The reported path is that of `SKILL.md`, absolute. Each file is written to a
temporary file and renamed into place, so a file that is a symbolic link is
replaced, not written through. A directory of skills, the directory of this
skill in it, or a directory of the skill below that, that is a symbolic link
is refused before anything is written: it belongs to a checkout somebody works in
or to the store of another tool, and writing through it would change a file
there. Links further up, such as a home directory kept elsewhere, are where
somebody chose to keep their own files.

## After an upgrade

The installed copy is compared by content with the one the binary carries,
file by file: a reference missing or older beside `SKILL.md` makes it differ,
because a pointer to it would lead nowhere. A file the binary does not carry
is left alone and counts for nothing.
When it differs, every command says so — in `notice` with `--json`, on stderr
otherwise — and names `minka skill install`. The CLI never replaces the file by
itself: nothing is written into a home directory unasked (C-017). Only the
default place is watched; a copy installed with `--dir` is checked with
`skill status --dir`. A skill behind a symbolic link is somebody else's to keep
current and draws no notice, since installing over it would be refused.

## The skill and the command tree

A test holds the skill, `SKILL.md` and its references together, against the
command tree: every command has to be named in it behind `minka` or, with its
group, in inline code, and every call
of `minka` — at the start of a line, in inline code, behind a pipe or inside
`$( )` — has to be a command. A command cannot be added, renamed or removed without the skill
following.
