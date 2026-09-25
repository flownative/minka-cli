# Decision register

Per decision: what holds, why, what for, where it is implemented. For looking
things up, not for reading in one go. IDs stay and are never reused; when a
decision is overturned, the new one gets a new ID and the old one the line
"superseded by".

Decisions about the **API** are kept by Minka itself
(`/Users/robert/Sites/minka/Documentation/decisions.md`, E-…). Here stands only
what the CLI decides for itself.

<a id="c-001"></a>
## C-001 — The conventions of basecamp-cli, not its scope

*Applies to: command names, output, exit codes*

Commands are named plural group plus action, the output follows basecamp-cli.
Not adopted: its scope and its feature areas.

**Why:** An agent that knows one of the two CLIs can read the other without
learning it. **Implemented as:** `internal/cli`, see
[Output](stable/output-contract.md).

<a id="c-002"></a>
## C-002 — `data` is the resource of the API, passed through raw

*Applies to: every output with `--json` and `--quiet`*

The CLI passes the JSON of the API through unchanged instead of assembling it
anew.

**Why:** A field the API gains reaches every client at once, and the CLI
cannot distort anything. **Implemented as:** `api.Result.Raw`,
`internal/cli/output.go`.

<a id="c-003"></a>
## C-003 — Exit codes are a contract

*Applies to: every error path*

A class of error has exactly one exit code; the table is in
`internal/cli/errors.go` and in the SKILL.md.

**Why:** An agent decides by the exit code whether to retry, ask or give up —
without reading text. **Implemented as:** `internal/cli/errors.go`.

<a id="c-004"></a>
## C-004 — A token goes to its own host only

*Applies to: sign-in, every request*

`MINKA_TOKEN` counts only together with `MINKA_HOST` and only for that host;
for any other host the token comes from the configuration file, or there is
none.

**Why:** Otherwise a `--host` would carry the token of one instance to a
foreign one. **Implemented as:** `internal/cli/root.go` (`session`), see
[Token](stable/security.md).

<a id="c-005"></a>
## C-005 — Tests run against a rebuilt API

*Applies to: all tests except the live run*

`internal/fakeapi` mirrors the contract, down to the byte-identical 401 and 404
bodies. When the API changes, the fake is adapted first.

**Why:** Tests against a running instance would be slow, unreliable, and would
touch other people's data. **Implemented as:** `internal/fakeapi`.

<a id="c-006"></a>
## C-006 — Every safeguard has a sabotage proof

*Applies to: file permissions, escaping, allowlist, locks*

Take the safeguard out in a copy, compile, see the test go red. A proof that
does not compile does not count; if the test stays green, it gets sharpened.

**Why:** A test that stays green without the safeguard secures nothing.
**Implemented as:** a procedure, see [Testing](stable/testing.md).

<a id="c-007"></a>
## C-007 — Free text comes through stdin

*Applies to: description, comment, message*

Multi-line text is read from stdin with `-`; titles are single-line and go
into flags.

**Why:** Shell quoting is the most common source of errors for an agent that
sends text with quotation marks and line breaks. **Implemented as:**
`app.readValue`.

<a id="c-008"></a>
## C-008 — Markdown in, Markdown out

*Applies to: descriptions, comments, messages*

The CLI takes Markdown and shows Markdown; the source of a text is what counts,
and attachments and mentions are lost only with `--drop-attachments`.

**Why:** What the CLI shows should be editable and sendable back without
losing anything. **Implemented as:** `internal/markdown`, `internal/richtext`,
see [Rich text](stable/rich-text.md).

<a id="c-009"></a>
## C-009 — What has the same address gets the same command

*Applies to: private conversations*

The ID of a conversation goes to `messages list|create|edit|delete|react|unreact`
and `chats mark-read`; commands of their own exist only for what exists only
there (`conversations list|with|people`).

**Why:** The API serves both under the same endpoints; a second set of
commands would be a doubled surface for the same thing. **Implemented as:**
`internal/cli/conversations.go`.

<a id="c-010"></a>
## C-010 — The circle of an upload is in a flag

*Applies to: `uploads create`*

The file is the argument, the circle a flag: `--project` or `--conversation`,
exactly one of the two.

**Why:** Two kinds of ID in the same position would leave the CLI guessing
which address to send to. **Implemented as:** `internal/cli/uploads.go`.

<a id="c-011"></a>
## C-011 — Writing live happens only where it is cleared

*Applies to: every live run*

Allowed are the test instance with the agent account and, on the local
instance, the board and chat "…Abnahme". Everything else is decided by Robert
or Shaun.

**Why:** A write reaches real people and other people's data.
**Implemented as:** [Live check](guides/live-check.md).

<a id="c-012"></a>
## C-012 — `retryable` only where a second attempt does no harm

*Applies to: every error*

True for reading requests and for `messages create` with
`--client-message-id`; false otherwise, with the hint to read the state back.

**Why:** A repeated write would otherwise create a card or a comment twice.
**Implemented as:** `internal/cli/errors.go`.

<a id="c-013"></a>
## C-013 — What the server is sure to refuse, the CLI refuses first

*Applies to: uploads, IDs, required flags*

A file over 100 MiB, a directory, an ID that is not a UUID, a call without a
circle: exit 1, without a request.

**Why:** Saves a useless transfer and gives an error that says what to do.
**Implemented as:** `internal/cli/uploads.go`, `requireArgs`.

<a id="c-014"></a>
## C-014 — The SKILL.md is the contract for agents

*Applies to: `skills/minka/SKILL.md`*

It lives in the repository, is brought up to date with every change to
commands or behaviour, and describes what an agent needs to know — not what
the CLI does internally.

**Why:** Agents are the main users; their description would otherwise be the
first thing to go stale. **Implemented as:** `skills/minka/SKILL.md`.

<a id="c-015"></a>
## C-015 — Work reaches `main` through a pull request

*Applies to: every change to the repository*

A change lives on a branch of its own and is merged through a pull request
once GitHub Actions ran the checks on it: formatting, vet, tests, tidy
dependencies, a build, known vulnerabilities.

**Why:** A release is cut from `main`, so `main` has to be releasable at any
time, and a pull request is where a second pair of eyes looks. **Implemented
as:** `.github/workflows/test.yml`, see [Testing](stable/testing.md).

<a id="c-016"></a>
## C-016 — The skill travels inside the binary

*Applies to: `skill show|install|status`, installing and upgrading*

The binary carries the SKILL.md and writes it out on request; it is not fetched
from the network and not installed by a separate package.

**Why:** The description an agent reads then always fits the commands of the
version that is installed. **Implemented as:** `skills/embed.go`,
`internal/cli/skill.go`, see [The skill](stable/skill.md).

<a id="c-017"></a>
## C-017 — A differing skill is reported, never replaced

*Applies to: every command, `skill install`*

When the installed skill differs from the one the binary carries, a `notice`
says so; only `skill install` writes, and not through a directory of skills or
a skill directory that is a symbolic link.

**Why:** A tool that writes into a home directory unasked, or through a link
into somebody's checkout, changes files nobody asked it to touch.
**Implemented as:** `staleSkillNotice`, `installSkill`.

<a id="c-018"></a>
## C-018 — `setup` is for a person, the single steps are for an agent

*Applies to: `setup`, `auth login`, `skill install`*

`setup` asks questions and refuses without a terminal; everything it does is
also a command of its own that asks nothing.

**Why:** An agent that gets asked a question hangs, and a person should not
have to know three commands to begin. **Implemented as:**
`internal/cli/setup.go`, see [First steps](stable/first-steps.md).

<a id="c-019"></a>
## C-019 — `doctor` reports and exits 0

*Applies to: `doctor`*

A failed check is data, `data.healthy` and `checks[].status`, not an error of
the command.

**Why:** An exit code names one class of error, and a broken setup can have
several at once; the report names each with its hint. **Implemented as:**
`internal/cli/doctor.go`.

<a id="c-020"></a>
## C-020 — Homebrew installs, no script from the network

*Applies to: installing, upgrading, the README*

The CLI is installed and upgraded through the Homebrew tap, or by hand from an
archive with a checksum. There is no install script and no self-update.

**Why:** A script piped into a shell is run unread, and a package manager
already knows how to upgrade. **Implemented as:**
`.github/workflows/release.yml`, see [Releases](stable/releases.md).

<a id="c-021"></a>
## C-021 — While the repository is private, the formula builds from source

*Applies to: `Formula/minka.rb` in the tap*

The formula clones the tag over SSH and builds with Go; the archives of a
release are for installing by hand.

**Why:** Release files of a private repository need a token on every machine,
SSH access to the repository everybody who may install already has.
**Implemented as:** `.github/formula/minka.rb.tpl`.

<a id="c-022"></a>
## C-022 — A short identifier is looked up, then the ID takes over

*Applies to: every command that takes a card*

`MNK-1A` is accepted wherever a card is named. The command asks the API for the
card once and goes on with its UUID; answers and breadcrumbs name the UUID.

**Why:** People call cards by their short identifier, but it stops naming the
card when a board changes its prefix, and every other address takes the UUID.
**Implemented as:** `session.resolveCard`, `requireID`.

<a id="c-023"></a>
## C-023 — Foreign history is read through the source's own CLI

*Applies to: `import basecamp`, every later extractor whose source has a CLI*

The extractor runs `basecamp api get` for every request instead of speaking
the Basecamp API; it keeps no Basecamp token and no HTTP client for it.

**Why:** The Basecamp CLI brings sign-in, token refresh, rate limiting and
retries, and a Kubernetes job that extracts later ships both binaries.
**Implemented as:** `internal/basecamp` (`CLI`), see
[Importing](stable/import.md).

<a id="c-024"></a>
## C-024 — A run replaces the bundle as a whole, staged next to the target

*Applies to: `import`, `internal/bundle`*

A bundle is built in a directory beside `--out` and renamed into place at the
end; only a bundle of the same format and source is replaced.

**Why:** A bundle that mixes two runs, or half of one, would be applied as if
it were whole, and a rename within one directory cannot end halfway.
**Implemented as:** `bundle.Writer`, see [Importing](stable/import.md).

<a id="c-025"></a>
## C-025 — A file that cannot be fetched is left out, and the run fails at the end

*Applies to: `import`, every attachment of a bundle*

The bundle is written without the file and its record and the report names it;
the command exits 6 when trying again may help, else 7, the report as `data`.

**Why:** One missing picture must not cost a run of hours, and exit 0 would
hide that the bundle is incomplete.
**Implemented as:** `Extractor.attach`, `incompleteImport`.

<a id="c-026"></a>
## C-026 — A mention refers only to somebody the account lists

*Applies to: `import basecamp`, every text of a bundle*

The person is read from the signed ID, then the rendered avatar, then the
editor's attribute; anybody the people list lacks stays a name in the text.

**Why:** A mention names an ID alone, and without a record of the person there
is nothing to write the reference to; a text may claim what nobody signed.
**Implemented as:** `mentionedPerson`, `registry.referByID`.

<a id="c-027"></a>
## C-027 — A chat is read to its end and held before its first line is written

*Applies to: `import basecamp`, the lines of a chat*

All pages of a chat are read, then its lines are ordered oldest first and
written; one chat is held at a time, and a page is let go of once it is read.

**Why:** Basecamp lists a chat newest first and can neither reverse nor count
it, while a bundle is read from the top; one chat at a time bounds the memory.
**Implemented as:** `Extractor.extractChat`, `getList`.

<a id="c-028"></a>
## C-028 — The walk goes kind by kind across the projects of a run

*Applies to: `import basecamp`, the order of every file of a bundle*

The projects with their tools and boards come first, then the message boards
of all projects, then the chats; nothing of a later kind is written before.

**Why:** Each file of a kind is streamed and never rewritten, and the comments
of posts follow those of cards, the reactions those of what they hang on.
**Implemented as:** `Extractor.Extract`, see [Importing](stable/import.md).

<a id="c-029"></a>
## C-029 — A request that failed for a reason that may pass is asked again

*Applies to: `import basecamp`, every request through the Basecamp CLI*

Network, rate limit and an API failure of 5xx, 408 or 429 are tried up to four
times with growing pauses, 100 retries a run; then the run ends, retryable.

**Why:** A run of hours must not end on one failure that passes, and the
Basecamp CLI gives up at once on a status it does not know, a gateway's 525.
**Implemented as:** `basecamp.Retrying`, `failure`, see
[Importing](stable/import.md).

<a id="c-030"></a>
## C-030 — Files are streamed in, or taken over from the bundle before

*Applies to: `import basecamp`, every file of a text or a chat*

`basecamp files download --out -` streams a file into the staged bundle under
a name the extractor makes plain; a file the old bundle holds intact is linked.

**Why:** A file of any size must not pass through memory or get a name the
source chose, and a run that replaces the bundle would fetch thousands again.
**Implemented as:** `basecamp.Downloader`, `Extractor.attachFile`,
`bundle.Writer.StoreFile` and `ReuseFile`, see [Importing](stable/import.md).

<a id="c-031"></a>
## C-031 — Every link to a card, a post or a chat of the account is a reference

*Applies to: `import basecamp`, every text of a bundle*

A link whose address shows a card, a message, a chat or a line of a chat of
the account becomes an `<import-reference>`, whatever its words and whether
the record is in the bundle or not; any other link stays a link.

**Why:** The applier resolves what it knows and falls back to the link for
the rest, so marking more loses nothing and marking less loses a reference.
**Implemented as:** `Extractor.referenceTo`, `links`, see
[Importing](stable/import.md).

<a id="c-032"></a>
## C-032 — A fetched file is one this command made itself

*Applies to: `uploads download`, the file it writes*

A name out of an answer has to be a plain name, and the bytes land in a file
created with `O_EXCL` and 0600. A file of that name that is already there
stands; `--force` takes the entry away first — the link itself, never what it
points at — and a directory under that name is refused.

**Why:** The server has a say in which entry of the directory is meant, so
nothing standing under that name may be followed, kept or destroyed.
**Implemented as:** `cli.writeTheBytes`, see
[Token, hosts and requests](stable/security.md).

<a id="c-033"></a>
## C-033 — The bytes of a file follow a redirect, the token does not

*Applies to: `GET /uploads/{blobId}/{variant}`*

This one request follows up to five redirects, because a store that keeps the
bytes answers with an address of its own. The `Authorization` header comes off
for every address that is not the host signed in to, and such an address has to
be https. Every request carrying JSON still follows nothing.

**Why:** Go keeps the header within a domain, a host below it included, so
relying on it would hand the token to a neighbour of our own host.
**Implemented as:** `api.Client.followTheBytes`, see
[Token, hosts and requests](stable/security.md).

<a id="c-034"></a>
## C-034 — The bytes are fetched where they lie, not through Minka

*Applies to: `uploads download`*

The request for the bytes carries no `?download=1`, which would keep every
answer inside Minka and spare the client the redirect.

**Why:** A picture that is shown where it stands is the one file Minka hands
over to its store; asking the redirect away would send every one of those
through a worker of the application instead. Everything else comes through it
either way.
**Implemented as:** `api.Client.FileBytes`, which follows under the rules of
[C-033](#c-033).

<a id="c-035"></a>
## C-035 — A test bounds time, it does not bet on the pace of a program

*Applies to: tests that run the Basecamp CLI or another program*

A test may set an upper bound that only a break trips. It may not make the pace
of a program its precondition, and what it has to know about a process it takes
from the code that started it, not from the program. A test where the pace is
the behaviour itself is the named exception.

**Why:** A program that is ended before it has run a line of its own leaves no
note about itself, so a test that waits for one fails for the load of the
machine. **Implemented as:** `basecamp.Downloader.download`, which names the
process as soon as there is one, see [Testing](stable/testing.md).

<a id="c-036"></a>
## C-036 — The cards being worked on are one list per machine

*Applies to: `work start`, `work stop`, `work list`, `heartbeat`*

They lie in `work.json` beside the configuration, read and written under a lock
of the kernel, and every signal reports their union.

**Why:** The server keeps one set per agent and client, so sessions with lists
of their own would take each other's cards away with every signal.
**Implemented as:** `internal/work`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-037"></a>
## C-037 — The client of an agent is a name that means nothing

*Applies to: the field `clientId` of every signal*

It is made once at random and kept in the configuration, never the host name
and nothing about the machine or the person.

**Why:** It travels into Minka's database and its logs, where a host name would
say who sits at which machine for as long as those are kept.
**Implemented as:** `config.NewClientID`, `config.File.ClientID`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-038"></a>
## C-038 — The signal never disturbs the work it runs in

*Applies to: `heartbeat`*

It exits 0 whatever happens, says nothing unless asked, and where nothing is
remembered it asks nothing of the network at all.

**Why:** It runs after every tool call, and an expired token, a host that is
down or a person's token is nothing the agent could act on from there.
**Implemented as:** `cli.app.signal`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-039"></a>
## C-039 — The settings of Claude Code are added to, or left alone

*Applies to: `hooks install`*

Everything the file holds is kept, a hook that is already there is not written
twice, and a file shaped otherwise than Claude Code writes it is refused with
the entry to add by hand.

**Why:** That file belongs to whoever wrote it, and a command that mangles it
is worse than one that refuses. **Implemented as:** `cli.addHeartbeatHook`,
`heartbeatHookIn`, see [Saying what you are working on](stable/at-work.md).

<a id="c-040"></a>
## C-040 — A card nobody starts again is given up after eight hours

*Applies to: the list of `internal/work`*

A card is dropped when it has stood eight hours without a `work start` of its
own. Nothing of the contract asks for this; the server forgets a card three
minutes after the last signal, and this is the client's own rule.

**Why:** A session that dies says nothing more about its cards, and every other
session on the machine would keep reporting them for as long as it runs. Eight
hours is a working day: long enough for the work itself, short enough that
nothing glows through the night. It hangs on the moment a card was started, so
it can cut a long piece of work short as well — `work start` again says that
the card is still the one.
**Implemented as:** `work.Host.ForgetWhatRanOut`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-041"></a>
## C-041 — A card that is moved on is one nobody has in hand

*Applies to: `cards move`, `cards done`*

A card that lands in another column, is put on hold or reaches Done leaves the
list of the machine, and the list is reported at once; one that only changes
place where it already is stays. Where the card was not in the list, nothing
is written and no signal goes out. What it costs is the order in which a card
is taken up: move it first, then `work start`.

**Why:** Whoever moves a card on has let go of it, and a tile that keeps
glowing says the opposite; the server cannot do it instead, because a signal
carries the whole set and the last one holds.
**Implemented as:** `cli.app.letGoOfTheCard`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-042"></a>
## C-042 — A write over the notes of a card says what it replaces

*Applies to: `cards notes set`*

The command needs `--expect <updatedAt>` (or `none`) or `--force`; there is no
default. It does not read the notes itself to send what it read.

**Why:** Notes are overwritten as a whole, and agents and people write the
same card. A guard only protects what the writer read before it began to
write; one read by the command a moment before its write would guard nothing
and only look safe. Naming `--force` makes writing over another's edit a
choice, not an accident.
**Implemented as:** `cli.notesGuard`, see
[Rich text in both directions](stable/rich-text.md).
