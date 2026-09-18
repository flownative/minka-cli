# The skill

`skills/minka/SKILL.md` is the contract for agents (C-014). It travels inside
the binary, so that what an agent reads fits the commands of the version that
is installed (C-016).

## Commands

| Command | Does |
|---|---|
| `skill show` | prints the skill the binary carries |
| `skill install [--dir <dir>]` | writes it to `<dir>/minka/SKILL.md`; without `--dir` to `~/.claude/skills`, where Claude Code reads it for every project |
| `skill status [--dir <dir>]` | `data.state`: `current`, `differs` or `not installed` |

The reported path is absolute. The file is written to a temporary file and
renamed into place, so a `SKILL.md` that is a symbolic link is replaced, not
written through. A directory of skills, or the directory of this skill in it,
that is a symbolic link is refused: it belongs to a checkout somebody works in
or to the store of another tool, and writing through it would change a file
there. Links further up, such as a home directory kept elsewhere, are where
somebody chose to keep their own files.

## After an upgrade

The installed copy is compared by content with the one the binary carries.
When it differs, every command says so — in `notice` with `--json`, on stderr
otherwise — and names `minka skill install`. The CLI never replaces the file by
itself: nothing is written into a home directory unasked (C-017). Only the
default place is watched; a copy installed with `--dir` is checked with
`skill status --dir`. A skill behind a symbolic link is somebody else's to keep
current and draws no notice, since installing over it would be refused.

## The skill and the command tree

A test holds the skill against the command tree: every command has to be named
in the skill behind `minka` or, with its group, in inline code, and every call
of `minka` — at the start of a line, in inline code, behind a pipe or inside
`$( )` — has to be a command. A command cannot be added, renamed or removed without the skill
following.
