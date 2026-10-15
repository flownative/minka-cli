# Output, errors and exit codes

What the CLI prints is a contract: fields may be added, never disappear or be
renamed. The conventions come from
[basecamp-cli](https://github.com/basecamp/basecamp-cli), so that an agent that
knows one of the two can read the other (C-001).

## Three output modes

Text on a terminal, JSON otherwise.

| Mode | Output |
|---|---|
| `--json` | envelope `{ok, data, summary, notice, breadcrumbs}` |
| `--quiet` | only `data` |
| `--styled` | text, even without a terminal |

`data` is the resource of the API, passed through raw — not assembled anew. A
field the API gains reaches every client this way, without the CLI knowing it
(C-002).

The text view prints what holds of the thing it shows, which is not always the
same set of lines: where a line would name a place something does not have, it
is left out rather than filled in. A card in the archive of its board gets no
`Column:` line — it stands in none, and the column it names is the one it was
taken from — and says `Archived: yes, was in <column>` instead. What `--json`
hands over is untouched by that: the resource carries every field the API
sends, whatever the text view makes of it.

Where one command reads several answers, `data` is the list of their records as
they came, element by element, and nothing around it: `boards archive --all`
hands over the JSON of every card of every page unchanged, so a field the API
gains still reaches a reader (C-002). What the pages meant together — how many
were read, what came twice, what cannot be promised — is in `notice`, never
woven into the records.

Warnings are in `notice` with `--json`, on stderr otherwise, so that
`--quiet | jq` never has anything foreign in the stream.

## A newer minka

Every answer of Minka, a refusal included, may name the latest minka in
`Minka-CLI-Latest` and the oldest one it supports in `Minka-CLI-Minimum`, each
a plain `x.y.z` or absent. The CLI remembers both and says, at most once a day,
that its own version is behind (C-129):

- `--json`: the envelope of a success and of an error carries
  `"cliUpdate": {"current", "latest", "minimum"}`, each of the last two only
  where Minka names it. Nothing beside the envelope, on stdout or on stderr.
- Text: one line on stderr after the output, naming the new version and
  `brew upgrade minka`; below the minimum a warning of two lines instead. The
  command runs and exits as it would without it.
- `--quiet`: nothing, since there is no envelope to carry it.

The text and the envelope each have a day of their own, so an agent's commands
do not take the line from the person at the terminal. Only a release `x.y.z`
(with or without the `v` of its tag) compares itself, and every line and
`current` name it without the `v`, the way Minka names versions: `dev`, a build of a
checkout that is not a tag, a `-dirty` one or a prerelease never shows the
notice. `MINKA_NO_UPDATE_NOTICE` set to anything but empty or `0` switches the
line and the field off; `doctor` reports the state either way.

## Errors

`{ok: false, error, code, retryable, hint, meta: {apiCode, httpStatus}, data,
notice}`. `meta.apiCode` is the case code of Minka (`cardTitleTooLong`), `code`
the class, `hint` the next step. `meta` is there when a server answered.
`data` and `notice` are there only when a command got its work done but not
all of it: `data` is the result it would have printed, `notice` its warnings —
or, for `unread`, when it wrote nothing because of what it read: `data` is the
comments that stopped it.

| Exit | `code` | Meaning |
|---|---|---|
| 0 | — | ok |
| 1 | `usage`, `config` | the call, or this machine: the config file, a CLI of a foreign system that is missing, a bundle that cannot be written |
| 2 | `not_found` | not found, or not visible |
| 3 | `auth_required` | not signed in |
| 4 | `forbidden` | forbidden |
| 6 | `network` | no complete response |
| 7 | `api_error` | server or protocol error |
| 8 | `ambiguous` | ambiguous input |
| 9 | `validation` | refused by Minka, a write somebody else got in before included |
| 11 | `unread` | a comment not written: its card has comments the caller has not read ([Comments](comments.md)) |
| 12 | `timeout` | a wait that ran out with nothing new ([Waiting and coordinating](waiting-and-coordinating.md)) |

The codes up to 9 are those of basecamp-cli, which uses 5 for a rate limit and
10 for an account limit as well. A class it has no code for starts at 11, clear
of every code it uses (C-001, C-101).

Such a command fails so that a caller notices, and the result is not lost:
the error envelope with `data` is printed with `--quiet` as well, and the text
output prints the result before the error. `import basecamp` ends this way
when a file it could not fetch may still come on a later run, or when it could
not place the failure at all (C-069).

`retryable` is true only for reading requests, and for `messages create` with
`--client-message-id`, because a second attempt then writes nothing new. A
failed write may have arrived; there the `hint` advises reading the state back
(C-012).

## When the answer is a file

`uploads download` fetches bytes, and bytes are no envelope. They go to disk,
and the envelope says what happened: `data` is `{file, variant, path,
byteSize}`, where `file` is the resource of the API unchanged and the three
others are what the CLI did with it. With `-o -` the bytes themselves are the
answer, so standard output carries them alone and the word about them goes to
stderr; `--json` and `--quiet` are refused there, because standard output
cannot carry two answers at once.

The CLI does not send `?download` with that request. It changes the headers
around the same bytes, not the bytes: `attachment` instead of `inline` with the
same name, an octet stream instead of the type of audio or video a player
would take (a picture keeps its type), no redirect to the store and no
ranges. The name of the file on disk comes from
the resource, never from a header, so nothing of it reaches the user
([C-121](../decisions.md#c-121)).

## Commands

Plural group plus action (`cards move`, `comments react`). A command that
fetches a resource adds `breadcrumbs`: the next sensible calls, each with a
ready-made command line.

Free text never comes through shell quoting alone: description, comment and
message take `-` for stdin. Titles are single-line and go into flags (C-007).

Where one address names two different things, a flag names the circle instead
of putting two kinds of ID in the same position: `uploads create <file>
--project <id> | --conversation <id>` (C-010).

What the API offers under the same ID gets no second set of commands: the ID
of a private conversation goes to `messages …` and `chats mark-read` (C-009).

A thing read by its ID alone is shown by `show` in its own group (`chats show
<chatId>`). A tool has no group of its own: one tool is `<group> tool
<toolId>`, beside `<group> tools <projectId>` for the tools of a project
(`todolists tool`, `posts tool`, `docs tool`, `checkins tool`, `events tool`,
C-120).
