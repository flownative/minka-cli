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

Warnings are in `notice` with `--json`, on stderr otherwise, so that
`--quiet | jq` never has anything foreign in the stream.

## Errors

`{ok: false, error, code, retryable, hint, meta: {apiCode, httpStatus}}`.
`meta.apiCode` is the case code of Minka (`cardTitleTooLong`), `code` the
class, `hint` the next step.

| Exit | `code` | Meaning |
|---|---|---|
| 0 | — | ok |
| 1 | `usage`, `config` | the call or the config file |
| 2 | `not_found` | not found, or not visible |
| 3 | `auth_required` | not signed in |
| 4 | `forbidden` | forbidden |
| 6 | `network` | no complete response |
| 7 | `api_error` | server or protocol error |
| 8 | `ambiguous` | ambiguous input |
| 9 | `validation` | refused by Minka |

`retryable` is true only for reading requests, and for `messages create` with
`--client-message-id`, because a second attempt then writes nothing new. A
failed write may have arrived; there the `hint` advises reading the state back
(C-012).

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
