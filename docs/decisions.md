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

*Applies to: `setup`, `auth login`, `skill install`, `hooks install`*

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
the Basecamp API; it keeps no Basecamp token. The one exception is the earlier
file of an upload (C-104).

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

**Superseded by [C-069](#c-069)**, which keeps everything below but the one
answer: a run whose missing files the source itself refused ends 0, because no
repeat can bring them. A report of such a run read against this entry would look
like a run that went through with nothing missing.

*Applies to: `import`, every attachment of a bundle*

The bundle is written without the file and its record and the report names it;
the command exits 6 when trying again may help, else 7, the report as `data`.

**Why:** One missing picture must not cost a run of hours, and exit 0 would
hide that the bundle is incomplete. This is no case of [C-050](#c-050): a bundle
can express the file, the source merely does not hand it over, so the run says at
the end that a later one has something left to bring.
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

**Superseded in one point by [C-115](#c-115)**: a run with `--keep-files`
takes a file over from the files an earlier run kept as well, before it looks
at the bundle before. Everything else below holds.

*Applies to: `import basecamp`, every file of a text or a chat*

`basecamp files download --out -` streams a file into the staged bundle under
a name the extractor makes plain; a file the old bundle holds intact is linked.

**Why:** A file of any size must not pass through memory or get a name the
source chose, and a run that replaces the bundle would fetch thousands again.
**Implemented as:** `basecamp.Downloader`, `Extractor.attachFile`,
`bundle.Writer.StoreFile` and `ReuseFile`, see [Importing](stable/import.md).

<a id="c-031"></a>
## C-031 — Every link to a card, a post, a chat or an event of the account is a reference

*Applies to: `import basecamp`, every text of a bundle*

A link whose address shows a card, a message, a chat or a line of it, or an
entry of a calendar or a day of it, of the account becomes an
`<import-reference>`, whatever its words, whether the record is in the bundle
or not and with or without `--calendars`; any other link stays a link.

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

*Superseded by [C-103](#c-103).*

<a id="c-037"></a>
## C-037 — The client of an agent is a name that means nothing

*Applies to: the field `clientId` of every signal*

It is made once at random and kept in the configuration, never the host name
and nothing about the machine or the person. A session reports as that name, a
dot and sixteen hex digits of a SHA-256 of the session; the list of no session
as the name alone.

**Why:** It travels into Minka's database and its logs, where a host name would
say who sits at which machine for as long as those are kept.
**Implemented as:** `config.NewClientID`, `config.File.ClientID`,
`cli.clientIDOf`, see
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

*Superseded by [C-109](#c-109).*

<a id="c-040"></a>
## C-040 — A card is given up eight hours after its session was last at it

*Applies to: the list of `internal/work`*

A card remembers the session that took it up, and that session says after every
tool call that it is still at it, at most once a minute. Eight hours after the
last of those the card is dropped. A card taken up where nothing says who is
asking goes by the moment it was started. Nothing of the contract asks for any
of this; the server forgets a card three minutes after the last signal, and
this is the client's own rule.

**Why:** A session that dies says nothing more about its cards, so they would
stay in its list for ever; the sign of life is the session's own, so no other
session's work keeps them. Eight hours is a working day: long enough for the
work itself, short enough that a list nobody reports is not kept.
**Implemented as:** `work.Host.StillAtIt`, `ForgetWhatRanOut` and `lastSeen`,
see [Saying what you are working on](stable/at-work.md).

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

<a id="c-043"></a>
## C-043 — One process per host reports while nobody else can

*Applies to: `minka heartbeat`, and the keepalive it starts*

A beat that finds a card in hand and no keepalive of its host starts one,
detached and under a lock of its own. It reports every session by that
session's own last beat, and is at no work: no session name, and neither mark
written. It takes back an empty list before it lets go of it; a silent store
and a jumped clock stop it without a word.

**Why:** A long tool call makes no signal for longer than the three minutes the
server counts one for, and a process that could pass for a session at work would
keep a card alight for a working day with nobody left to stop it.
**Implemented as:** `cli.app.keepTilesAlive`, `cli.tick.decide`,
`cli.app.reportEverySessionAtWork`, `cli.app.somebodyIsAtWork`,
`work.HoldKeepalive`, see
[Saying what you are working on](stable/at-work.md).

*Superseded by [C-107](#c-107).*

<a id="c-044"></a>
## C-044 — A host is read, or it is no host

*Applies to: `--host`, `$MINKA_HOST`, the default host of the configuration*

A value that names no machine is refused, and a default host the CLI cannot read
is refused where it is read, so that no session comes about under it. Signing out
is the one command that takes an entry as the file writes it, so that one written
by hand can be taken out again.

**Why:** Such a value would become a key of the configuration and of the list
of cards, and a session under it would carry it into everything that prints a
host.
A key such a value already made in the list of cards is left where it lies:
nothing walks the hosts of that list, so it is inert, and the file is written
with the mode of the configuration beside it. **Implemented as:**
`config.NormalizeHost`, `cli.defaultHostOf`, `cli.app.hostToSignOutOf`, see
[Token, hosts and requests](stable/security.md).

<a id="c-045"></a>
## C-045 — A check-in asks its subscribers, and only those Minka can ask

*Applies to: `asked` of `questions.jsonl`*

`asked` comes from the subscribers of the question, not from whoever answered
it, and holds only those the bundle carries as members of the project who are
still there and have an address. The first run decides: a later one never
touches `asked` again.

**Why:** The subscribers are whom Basecamp hands the question to; whoever
answered is neither all of them nor only them. Somebody Minka cannot ask ends
its run in a failure that carries no file and no line while its dry run stays
green, and the next run hits the same wall. The filter guards against the case,
not against the version of the applier that happens to stand today.
**Implemented as:** `basecamp.Extractor.askedOf`, see
[Importing from Basecamp](stable/import.md).

<a id="c-046"></a>
## C-046 — A check-in asked every other week is anchored on the day it was last answered

*Applies to: `startsOn` of an `everyOtherWeek` question*

`startsOn` is the most recent day an answer of the question belongs to, stepped
back to the weekday it is asked on. A question nobody ever answered is left out
instead of anchored.

**Why:** Minka counts the weeks from the first matching day at or after
`startsOn`, and Basecamp says nothing about which of two weeks it asks in. The
last day answered is the one day that proves which week the source is in. A
guess would put half of these questions a week out, and nothing in Minka would
notice. **Implemented as:** `basecamp.anchoredOn`, see
[Importing from Basecamp](stable/import.md).

<a id="c-047"></a>
## C-047 — A day the source does not name is the day its author saw

*Applies to: `groupOn` of an answer, `startsOn` of a question without a start*

Where Basecamp names no day, the day is read from the time the record was
written, in the zone of whoever wrote it as Basecamp names it. Without a zone,
or with one nobody knows, the run ends. The zone database travels in the binary.

**Why:** Basecamp writes every time as UTC and Minka knows no zone of an
organisation, so any other zone would give the day of nobody. A day that is off
by one is a day nobody can tell from a right one afterwards, which is worth more
than a run that goes through. **Implemented as:** `basecamp.dayOf`, see
[Importing from Basecamp](stable/import.md).

<a id="c-048"></a>
## C-048 — A schedule Minka cannot express leaves the question out, and the run goes on

*Applies to: `questions.jsonl`, the report of a run*

A question whose schedule Minka has no words for is left out with its answers
and named in the report with the field that made it impossible. The run goes on.

**Why:** One question of a whole account would otherwise cost the extraction of
all of it, and a run of hours would have to be repeated for a single fortnightly
question nobody ever answered. Nothing here is final: the question uses no
`sourceId`, so a later run writes it as soon as Minka has a word for its
schedule, or as soon as the question is answered — unlike `asked`, which the
first run settles. The line it stands on is [C-050](#c-050); its own is the
report, and the report is the part somebody acts on: a counted record says how
many are gone, a named one says *which* question is gone, and it has to, because
with a question all its answers fall. What decides the form is therefore not how
many records there are but how much hangs on one. Besides, no single word makes a
schedule impossible but a reading of several fields together, `group_on` among
them, which is no word of the source at all. **Implemented as:**
`basecamp.scheduleOf`,
`basecamp.Extractor.leaveQuestionOut`, see
[Importing from Basecamp](stable/import.md).

<a id="c-049"></a>
## C-049 — A kind of chat line a bundle has no word for leaves the line out, and the run goes on

*Superseded by [C-050](#c-050)*, which says the same for every kind of record of
a bundle, with the same report, the same counting and the same reason. Nothing of
this entry was left over.

<a id="c-050"></a>
## C-050 — The run stops where it would write something wrong, and leaves out what a bundle cannot express

**Superseded in one point by [C-111](#c-111)**: a title or a name Minka refuses
no longer ends the run, and neither does a to-do that starts after it is due or
an event that ends before it starts. Everything else below holds.

*Applies to: `import`, every record of a bundle, the report of a run*

A record the source describes with a word a bundle has none for is left out and
counted in `recordsLeftOut`, with the kind of record, the field, the word and
where it stood; the run goes on. Where going on would put something wrong into
the bundle, the run ends as `api_error` instead: a record without its times or
with a time that is none, a day or a zone that would give somebody else's day, an
address outside the account, a title Minka refuses, a second Triage, Not now or
Done on one board, a reaction Basecamp does not say whose it is, and the status
of a project the run was asked for by name.

**Why:** A bundle has no field to say an unknown word in, so such a record is
lost whatever the run does, and one exotic record must not cost a whole account.
A record put in the wrong place, or laid at the feet of somebody who never did
it, is the other thing: a mistake nobody can tell from the truth afterwards. A
field the format leaves open is no way out where it reads as something else —
`createdBy` takes `null`, and for a reaction an empty one reads as the author of
what it hangs on. Nothing left out is final: the source keeps it, and a later run
brings it once a bundle has a word for it.
**Implemented as:** `basecamp.Extractor.met`, `basecamp.Extractor.leaveOut`,
`basecamp.aMoment`, `basecamp.requireTimestamps`, `basecamp.lineTypes`,
`basecamp.columnRoles`, see
[Importing from Basecamp](stable/import.md).

<a id="c-051"></a>
## C-051 — A to-do is never said to be older than its completion

*Applies to: `todos.jsonl`, the `updatedAt` of a to-do*

A to-do whose completion is newer than the moment the source says it was last
touched is written as touched when it was done; `completedAt` stays as it is.

**Why:** Minka passes a record over whole whose `updatedAt` has not moved since
the run before, so such a to-do would stay open here for ever with nobody able to
see why — and the later of two moments the source itself said is no guess.
**Implemented as:** `basecamp.touchedAtLeastWhenDone`, see
[Importing from Basecamp](stable/import.md).

<a id="c-052"></a>
## C-052 — What falls with a place a bundle cannot carry is asked for and counted

*Applies to: the lists, groups and to-dos of a bundle, its folders, documents
and uploads, the report of a run*

What stood in a place a bundle does not carry is asked for although none of it is
written, counted per kind and named with the place it fell with: the groups and
to-dos of a to-do list, the to-dos of a group, and the folders, documents and
uploads of a folder of Docs & Files, whose own folders are places of their own,
so the walk grows while it goes. Such a walk asks for what it counts and for
nothing else, and the one failure it passes over is `not_found`. A **tool** that
is switched off is asked nothing: one the source has switched off is a tool the
run does not read at all, so what the project's own lists of records it has put
away hand out of it is counted where it is met, and an entry no place ever
claimed is counted without a place to name it against.

**Why:** [C-050](#c-050) promises that what is left out is counted, and a place
that took a subtree with it silently would make every number of a run say that a
bundle holds more of the source than it does. The requests are the price, so the
walk is fenced to them, and a place Basecamp no longer has is passed over rather
than ending a run over records it left out anyway — the only place where a wrong
assumption about an answer counts too few instead of ending the run, which is why
the [Status](work-in-progress/status.md) names it.
**Implemented as:** `basecamp.Extractor.countWhatFellWithAPlace`,
`basecamp.Extractor.countWhatFellWithAVault`,
`basecamp.Extractor.countWhatStandsInAFolderLeftOut`,
`basecamp.countWhatWasNeverPlaced`, see
[Importing from Basecamp](stable/import.md).

<a id="c-053"></a>
## C-053 — A card the host will not have leaves the list, and an unknown word takes nothing

*Applies to: `heartbeat`, `work list`, the list of the machine*

A card the host names in `leftOut` with a word this CLI knows leaves the list of
the machine, and the shorter list is reported with the next beat, as every
changed list is. A word it does not know takes no card out of anything: the note
is written, the card stays, and the report says what the host called it. What
was let go stands in `work list` as `letGo` and is swept with the list.

**Why:** One card the host will not have must not put out the tiles of
everything else a machine is at work on, and a list it keeps for ever is one it
keeps reporting in vain. The reason has to outlive the card, because the list
itself cannot say why something is missing from it and the hook that met the
answer is silent by [C-038](#c-038). The line is the one of
[C-050](#c-050): act on what is understood, pass on what is not — no word of a
host takes a card out of the list of a machine by itself.
**Implemented as:** `cli.app.writeDownWhatTheHostWouldNotHave`,
`work.Host.LetGoOf` and `NoteNotTaken`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-054"></a>
## C-054 — The page of an archive is bounded here, and a walk ends only on nothing

*Applies to: `boards archive`, `api.Client.ArchivedCards`, `api.Client.WholeArchiveOf`*

A `--limit` outside 1 to 100 is refused as a usage error rather than cut down to
the bound, a walk of several pages ends only on an answer that carried no card and
refuses one that carries it no further, and what it promises is every card of the
whole run exactly once — not the archive of one moment, and not an end it had
nothing left to check against, both of which it says in a `notice`.

**Why:** The server cuts a larger page down in silence and marks the end of a
list by nothing but fewer cards than asked for, so a run that trusted either
would truncate an archive and report success. **Implemented as:**
`api.ArchivePageMax`, `api.Client.WholeArchiveOf`, `cli.app.wholeArchive`, see
[The archive of a board](stable/archive.md).

<a id="c-055"></a>
## C-055 — A record says for itself whether clients see it

*Applies to: `import basecamp`, `visibleToClients`, `client`, `company`, `clientAccess`*

Whether clients see a record is read from that record and never from the tool it
stands in, and where the source says nothing, nothing is written. A record that
follows its container carries no such word and is counted where the source offers
one, `client` is written either way, a company stands on a client alone, and a
client is never said to be asked a question. A project's access and its company
stand together or not at all. A company without an identifier or with a name
Minka refuses ends the run, where the record that names it comes in.

**Why:** A record that took the word of its tool would show a client what the
source hides, and what a client has seen cannot be taken back; a company written
away as none tells Minka to drop the one it holds, and a project that names a
company and no access is one whose clients have none there.
**Implemented as:** `basecamp.visibleToClientsOf`, `basecamp.visibilityOfItsOwn`,
`basecamp.Extractor.clientsOf`, `basecamp.companyOf`, `basecamp.registry.refer`,
`basecamp.seenPerson.aClient`, see
[Importing from Basecamp](stable/import.md).

<a id="c-056"></a>
## C-056 — What nobody can repair at the source is made to fit

**Superseded in one point by [C-111](#c-111)**: the title of a card, a post, a
to-do, a folder or a document no longer ends the run; it is made to fit as a
subject is. Everything else below holds.

*Applies to: `import basecamp`, the `title` and the `fromAddress` of a forward,
the `title` of an upload*

Two titles a bundle carries are nobody's to put right where they stand: the
subject of a forwarded mail, and the title of an upload of Docs & Files, which
the source fills from the name of a file. Either is made into a title Minka
takes: every character nobody can see becomes a blank, what is left is trimmed,
and what is left of nothing is called `(no subject)` for a mail and `(no name)`
for an upload. What is longer than 200 characters is cut — a subject before the
last character, ending in an ellipsis, and an upload's title **the way Minka
shortens the name of a file** (`ItemTitle::fromFilename`), the extension staying
and the stem cut to make room, with no ellipsis. A
sender that is no e-mail address Minka takes leaves the **mail** out, named by
its `sourceId`, and the run goes on. All of it is counted and named in a
`notice`.

**Why:** A subject is whatever a mail server let through, and an upload's title
is whatever a file system allowed the name of its file to be: neither is
anybody's to repair at the source, so a run that refused one would never come to
an end, however often it is started. The title of a card, a post, a to-do, a
folder or a document still ends the run, because that one somebody can put right
where it stands. An upload's own title is taken where the source gives one and
the name of its file stands in where it does not, Minka deriving nothing from a
file and refusing an upload without a title; the fold is what keeps either from
ending a walk of hours over a name nobody typed. It follows Minka's own
shortening so that one file carries the same title whether it was uploaded there
or brought here.
`fromAddress` is what a forward is about and the field takes nothing else, so a
mail whose sender is none cannot be
carried at all — and ending a walk of hours over one mail would leave a whole
account unimported. The address is read as Minka reads it ([C-059](#c-059)), so
that what a bundle carries is always taken.
**Implemented as:** `basecamp.aTitleMinkaTakes`, `basecamp.aFileNameAsATitle`,
`basecamp.anAddressMinkaTakes`, see [Importing from Basecamp](stable/import.md).

<a id="c-057"></a>
## C-057 — A mail that would arrive with nothing in it carries a line of the run

*Applies to: `import basecamp`, the `body` of a forward and of an answer by mail*

Where the body of a mail would reach Minka as nothing at all — the mail says
nothing, what it says is a file that will **never** come, it is nothing but an
embed, or it holds nothing but what Minka takes off the edges of a text — the
run writes one line of its own behind the body and counts the mails that carry
one. The tag of the file stays where it stood, and an embed gets its address as
a link and no sentence. A file that might still come is not one that
will never come: the mail waits for it, and only a refusal of the source is such
a file ([C-069](#c-069)).

**Why:** `body` is a field of a forward that must not be empty, so writing one
is the only way the mail comes at all: Minka writes such a body empty and
remembers the mail as finished, or passes it over before it is remembered and
passes it over again on every run afterwards; either way the subject, the
sender, the answers and the comments are lost. The extractor is the one place
that knows a file can never come, and a run that said so of one it could simply
not fetch would take the mail from every run after it. An embed is Minka's own
reading of an address, so a mail that hangs on one alone gets the address as a
link, which every reading keeps, and no sentence.
A **document** gets no such line, and neither does the description of an upload:
those fields are optional and Minka writes a document without words, so the tag
of the file that will never come is taken out instead and the record arrives
wordless with everything on it. The difference between the two kinds is the
field, not the loss — and nobody puts invented words into something a person
wrote where there is a way not to.
**Implemented as:** `basecamp.Extractor.aBodyThatSaysSomething`,
`basecamp.Extractor.whatWouldArriveOf`,
`basecamp.Extractor.theFileNeverArrives`,
`basecamp.Extractor.wordlessWhereItsOnlyFileNeverComes`, see
[Importing from Basecamp](stable/import.md).

<a id="c-058"></a>
## C-058 — The shelf of a forwarded mail becomes the trash

*Applies to: `import basecamp`, the `status` of a forward*

A forwarded mail the source keeps on its shelf is written `trashed`, and the run
says how many.

**Why:** Minka gives a forward no shelf and stops a run at the word `archived`,
while its trash holds the mail with everything on it and hands it back on one
click, so nothing is lost and nothing is said falsely. Leaving it out as
[C-050](#c-050) leaves out what a bundle cannot express would lose the mail for
ever: no later run can bring it either, because Minka will have no shelf for it.
**Implemented as:** `basecamp.Extractor.writeForward`, see
[Importing from Basecamp](stable/import.md).

<a id="c-059"></a>
## C-059 — One rule for every address, and one pair of quotation marks comes off

*Applies to: `import basecamp`, the `email` of a person and the `fromAddress` of
a forward and of an answer by mail*

Every address a bundle carries is held to one rule, and that rule is Minka's
reading of an address: PHP's `filter_var` with the plain local part of
`EmailAddress` on top of it, read from that code and measured against it. Three
places where it is not Minka's, each of them narrower or beside the reading and
none of them wider:

- A domain written as a bracketed literal address of a machine is left out,
  although Minka takes it.
- A NUL at either end is left out, although Minka trims one away and takes what
  is left: `strings.TrimSpace` takes off what PHP's `trim` takes off, apart from
  that one byte.
- Minka lowercases an address before it reads it; a bundle carries the case the
  source gave. So a bundle can hold one mailbox in two spellings, and Minka
  makes them one without a word. Nothing here changes that.

One pair of quotation marks around the whole of an address comes off before the
address is read, counted per record in `addressesUnquoted` and named in a
`notice`.

**Why:** Minka reads all three fields through one value object and stops the
import where it refuses one, after a walk of hours that cannot be repeated
cheaply, so a rule wider than Minka's writes a bundle that cannot be applied,
and one narrower than Minka's loses somebody for nothing. Two rules in two
places drift apart. A bracketed literal is nobody's mailbox and a NUL around an
address is nobody's either, and reading either exactly as PHP reads it is a risk
in the direction that stops a run, where being narrower costs at most one
address. The quotation marks are the one place a run repairs what the source
says instead of passing it on: a mail header is nobody's to repair where it
stands, Minka refuses every such address, and the reading alone would turn away
what the stripping saves. It is that pair and nothing else — no other character
is trimmed, unfolded or mended, and a mark at one end alone is no pair — and the
number in the report keeps the repair from being a silent one.
**Implemented as:** `basecamp.anAddressMinkaTakes`,
`basecamp.minkaTakesTheAddress`, see
[Importing from Basecamp](stable/import.md).

<a id="c-060"></a>
## C-060 — A to-do that stands in no list is left out, counted and named

*Applies to: `todos.jsonl`, the to-dos of a To-dos tool, the report of a run*

Basecamp lets a to-do hang on a To-dos tool itself, in no list and no group of
it. A bundle carries none: such a to-do is left out, counted in
`todosLeftOut.todos` and named in `todosWithoutAList` with its `sourceId` and
its project. No list of the tool holds it, so the to-dos of every tool that is
switched on are asked for under the tool's own identifier, what is open and what
is done, and a failure of that request ends the run. Two other ways were weighed
and are not taken: the identifier is **not** written into `listSourceId`, where
it would make two kinds of record answer to one identifier and Minka resolve the
reference wrongly or not at all, and **no list is invented**, because a list
nobody created is a record nobody can explain at the source.

**Why:** [C-050](#c-050) promises that what a bundle cannot carry is counted,
and this was the shape that slipped through it — the walk goes list by list, so
a to-do standing in none was never fetched, never written and never counted, and
one on the shelf raised a number that named nothing. The naming turns the loss
into a repair: whoever reads the report moves the to-do into a list at the
source, and the next run carries it with everything that hangs on it. An
identifier and a project are all it takes, so nothing of what the to-do says is
in the report.
**Implemented as:** `basecamp.Extractor.leaveOutTheTodosWithoutAList`, see
[Importing from Basecamp](stable/import.md).

<a id="c-063"></a>
## C-063 — An upload whose file the source refuses is left out with everything pointing at it

*Applies to: `import basecamp`, the uploads of a Docs & Files tool*

An upload whose file no run will ever bring is left out and named by its
`sourceId`, and its comments and its boosts are neither asked for nor written.
Four things leave it without one, all of them the source's own word: a file that
lives in a cloud, which Basecamp names no stored file for
(`docs.withoutAStoredFile`), one the source no longer has, one above what Minka
takes, and a download Basecamp refuses (all three
`docs.onlyAFileNoRunWillBring`). What fell with either list is counted and a
`notice` says so. Where the file left a record behind, that record stays as the
trace of what became of it. **What says an upload has a file is the record and
never the shape of the address**, and **what says no run will bring it is
Basecamp's own answer and nothing else:** a failure that may pass and one this
run could not place both leave the upload written and waiting.

**Why:** an upload has no purpose without its file: `file` takes the `sourceId`
of a file of the bundle and nothing else, and one Minka cannot store leaves the
upload waiting there on every run, for ever, saying nothing to anybody. An
honest gap in the report is worth more than that. What pointed at it falls with
it ([C-052](#c-052)): a comment naming an upload the bundle does not carry stops
the applier, and carrying it would cost the import rather than the one upload.
The record and not the address decides, and only the source's own answer counts
as a refusal, because either reading that is wrong by one shape leaves **every**
file of a tool out of the bundle, counted and named and gone, where a file that
cannot be fetched costs one exit code and a later run ([C-069](#c-069)).
**Implemented as:** `basecamp.fileOfAnUpload`,
`basecamp.Extractor.theFileDidNotCome`, `theFileNeverArrives`,
`basecamp.Extractor.writeUpload`, see
[Importing from Basecamp](stable/import.md).

<a id="c-064"></a>
## C-064 — Every entry of Docs & Files says for itself what clients see, at every depth

*Applies to: `import basecamp`, the `visibleToClients` of a folder, a document
and an upload*

A folder, a document and an upload carry what the source says about clients
wherever they stand, and not only at the top of their tool. This is the one
place where a record standing inside another carries the word: everywhere else
what hangs on something follows it and says nothing ([C-055](#c-055)).

**Why:** Minka reads the word below the top as well, and there it is the one
thing that can hide a folder at the top: where something below a top folder the
source shows is hidden from clients, the import hides that top folder with
everything in it and names it. An entry that left the word out would be
imported in the open under a folder clients see, and what a client has seen
cannot be taken back. The rule is read from
`WhatClientsSee::followTheFolderAbove()` and from nothing else, Minka's prose
having said the opposite of it.
**Implemented as:** `basecamp.visibilityOfItsOwn`,
`basecamp.Extractor.writeFolder`, `writeDocument`, `writeUpload`, see
[Importing from Basecamp](stable/import.md).

<a id="c-065"></a>
## C-065 — The order of the file decides for what is in use and for nothing else

*Applies to: `import basecamp`, the order of the lists of a To-dos tool and of
the folders, documents and uploads of a Docs & Files tool*

A record **in use** stands in its file from the bottom up, because Minka puts
every new one on top of its kind, so the order the source shows comes out again.
A record that has **left its place** stands before all of them, in the order the
source lists it, and that order is not turned round.

**Why:** Minka orders an archive by the moment a record was put away — `archived_at
DESC` and then the item's own identifier for the folders, documents and uploads,
and `archived_at DESC` then the list's position for the to-do lists — and its
import takes that moment from the record's own `updatedAt`. So what the file says
about the order of such records decides nothing, except that the position of a
to-do list separates two archived in the same second, which is the position the
file gave them in the first place. Turning them
round would be work for nothing and a promise the code would then have to keep;
saying nothing about them would leave the next reader of `inFileOrder` to measure
it again.
**Implemented as:** `basecamp.inFileOrder`, see
[Importing from Basecamp](stable/import.md).

<a id="c-066"></a>
## C-066 — Every owner of a file gets a record of its own

*Applies to: `import basecamp`, `attachments.jsonl` and the files of a bundle*

Where one file of the source turns up under a **second owner**, the bundle writes
a record for that owner too: the same bytes, and a `sourceId` made of the
source's own name for the file and the owner it belongs to. The bytes are fetched
for the first record and linked to from the others. `filesOfSeveralOwners` counts
the owners beyond the first, met and not written.

**Why:** a file stands in one place in Minka. Its blob belongs to the project of
the record that brought it in, and only a blob of that project is ever attached
to a place in it, so a record that named another's file is shown none of it where
the two stand in different projects. One record for the first owner alone loses
such a file silently, because every check on a bundle asks whether a reference
finds a record and none asks whether the referring record may own it. Two blobs
for one file is what Minka would rather pay, and its `ownedBy` stays as strict as
it is. The owner and not the order of the walk names the record, because Minka
takes a record over by its identifier and one that moved when another record
moved would store the same bytes again.
**Implemented as:** `basecamp.Extractor.claimFile` and `shareTheFile`,
`bundle.Writer.ShareFile`, see [Importing from Basecamp](stable/import.md).

<a id="c-067"></a>
## C-067 — Three guards, three mappings, and never a null expectation

*Applies to: `docs edit`, `docs uploads edit`, `cards notes set`*

`--expect <updatedAt>` on a document sends the string **verbatim**;
`--force` leaves `expectedUpdatedAt` **out of the body** rather than sending
`null`; `--expect none` is refused as a usage error. The notes of a card keep
their own mapping, where `none` sends `null`.

**Why:** `null` means "no expectation, last write wins" on a document and an
upload, and "no notes have been written yet" on the back of a card, so one
helper over both would invert the meaning on one of them — and a document,
whose moment is kept to the whole second, cannot see a change made in the same
second as the read, which is why every guarded write says so rather than
promising more than it holds.
**Implemented as:** `cli.entryGuard`, `api.UpdateDocumentInput`,
`api.UpdateFilesUploadInput`, see
[Docs & Files](stable/docs-and-files.md).

<a id="c-068"></a>
## C-068 — A text the server finds no markup in is not refused but said out loud

*Applies to: every text sent as HTML — `cards create`, `cards edit`,
`comments create`, `comments edit`, `docs create`, `docs edit`, and so
`--description-format html`, `--format html` and `--body-format html`*

Markdown is converted here, and where a caller sends HTML itself a text that
carries no element the server serves draws a `notice` naming what becomes of
it. It is not refused.

**Why:** the server asks whether a value carries served markup before it cleans
anything, and keeps one that does not as the characters it consists of, with
the same 201 or 200 as any other answer — so `# Heading` lands as `# Heading`
and only the web ever shows it; refusing what the server accepts would be
wrong about it, and saying nothing would leave the one loss nothing else
reports.
**Implemented as:** `cli.markupNotice`, `cli.documentBody`,
`cli.descriptionHTML`, see [Rich text in both directions](stable/rich-text.md)
and [Docs & Files](stable/docs-and-files.md).

<a id="c-069"></a>
## C-069 — The exit code of a run that lost a file says whose word the loss is

*Applies to: `import`, every attachment of a bundle, supersedes
[C-025](#c-025)*

The bundle is written without the file and its record, and the report names it
under `missingFiles` with `why`: `failedThisTime`, `refusedBySource` or
`notUnderstood`. The strongest of them decides the end — `api_error`, exit 7,
where one could not be placed, else `network`, exit 6 and retryable, where one
may pass, else 0 with the loss in the `notice` and in the `summary`. **Only
`refusedBySource` decides anything about the file**, and a failure this run
raised before it asked anybody is never that, whatever class it carries.

**Why:** the exit code is a contract ([C-003](#c-003)) and says what to do next:
run again, look at the source, or look at this program. A file the source itself
refuses gives a repeat nothing to do, so it is left out, counted and named as
every record a bundle cannot carry is ([C-050](#c-050)), and an exit code that
can never be cleared teaches whoever watches it to stop reading it. A failure
nobody could place says nothing about the file: read as a refusal, one wrong
reading — a guard of ours, an answer nobody can read, a Basecamp CLI that
changed after an upgrade, a sign-in that ran out — would write off every file of
an account and call the run a success.
**Implemented as:** `basecamp.whoseWordAMissingFileIs`,
`basecamp.howTheDownloadEnded`, `basecamp.ourOwnRefusal`,
`cli.incompleteImport`, see [Importing from Basecamp](stable/import.md).

<a id="c-070"></a>
## C-070 — Which people go with which audience is Minka's rule alone

**Superseded in one point by [C-092](#c-092)**: `--notify` takes a name or an
e-mail address as well, looked up among the members of the tool's project.
Everything else below holds.

*Applies to: `docs create --audience` and `--notify`*

The CLI checks only that `--audience` is one of its three words and that a
`--notify` is an ID or `me`; everything else goes out as given and Minka
answers it, exit 9, with a hint that names what to change.

**Why:** a second copy of Minka's rule would be a second thing to keep in step,
and whether a named person may be told cannot be judged here at all.
**Implemented as:** `cli.checkAudience`, `cli.audienceHint`, `cli.documentRefusal`, see
[Docs & Files](stable/docs-and-files.md).

<a id="c-071"></a>
## C-071 — The lists are `todolists`, the groups `todogroups`, and `todos` waits for the to-dos

*Applies to: every command on a To-dos tool*

The lists of a To-dos tool and the tools themselves are `todolists …`
(`todolists tools <projectId>` beside `todolists list <toolId>`), the groups of
a list `todogroups …`. `todos …` is left free for the to-dos and their subtasks.

**Why:** basecamp-cli names the same three things `todolists`, its groups a
group of their own and `todos` the to-dos ([C-001](#c-001)), and a group
`todos` holding lists, groups and to-dos alike would put three kinds of ID into
the same position of one command tree. Minka's own addresses split the same way:
`todo-lists/…`, `todo-groups/…`, `todos/…`.
**Implemented as:** `cli.newTodoListsCmd`, `cli.newTodoGroupsCmd`, see
[To-do lists and groups](stable/todo-lists.md).

<a id="c-072"></a>
## C-072 — A group is moved to a place that is named

*Applies to: `todogroups move`*

Exactly one of `--top`, `--after <groupId>` and `--before <groupId>` is
required; none, two and a neighbour that is no ID are usage errors before a
request goes out. There is no `--bottom`. The group named as its own
neighbour goes to Minka, which answers `todoPlaceUnknown`, or `projectArchived`
in an archived project.

**Why:** Minka moves a group without a neighbour to the top of its list, with
the same 200 as any move, so a forgotten flag would reorder a list without a
word. Given both neighbours, Minka checks that both are groups of the list and
places the group behind the previous one alone, so the next one would be a
place asked for and ignored. A neighbour that is no ID is refused here for the
reason of [C-013](#c-013); whether a group may stand next to another is
Minka's rule and stays there, as in [C-070](#c-070). The address has no
bottom, and the CLI cannot find the last group without the list, which the ID
of a group does not name.
**Implemented as:** `cli.newTodoGroupsMoveCmd`, see
[To-do lists and groups](stable/todo-lists.md).

<a id="c-073"></a>
## C-073 — The people of a to-do are given as the whole set

*Applies to: `todos create`, `todos edit`*

`--assignee` and `--notify-when-done` name the whole set a to-do is to carry,
`--no-assignees` and `--no-notify-when-done` send an empty list, never null, and
no command adds or takes off one person.

**Why:** Minka's address takes the whole set and reads null as no change, so an
empty set sent as null would do nothing, and one person added by a read before
the write would take off whoever somebody else put on in between.
**Implemented as:** `cli.newTodosEditCmd`, `api.UpdateTodoInput`, see
[To-dos and subtasks](stable/todos.md).

<a id="c-074"></a>
## C-074 — A to-do is moved to a place that is named

*Applies to: `todos move`, `todos subtasks move`*

Exactly one of `--top`, `--after` and `--before`, as for a group
([C-072](#c-072)); `--group` and `--no-group` exclude each other, and a
neighbour of another place goes to Minka, which answers `todoPlaceUnknown`.

**Why:** Minka moves a to-do or a subtask without a neighbour to the top of its
place with the same 200 as any move, and where a to-do may stand is Minka's rule
([C-070](#c-070)). **Implemented as:** `cli.newTodosMoveCmd`,
`cli.newSubtasksMoveCmd`, `api.MoveTodoInput`, see
[To-dos and subtasks](stable/todos.md).

<a id="c-075"></a>
## C-075 — A day Minka keeps otherwise than it was sent is said out loud

*Applies to: `todos create`, `todos edit`*

The CLI sends the days it is given, and where the answer carries no start
although one was sent, or carried before an edit that sent none, or a due day
although an edit without a start took it off, a `notice` says so and names the
day Minka kept.

**Why:** Minka makes a start alone the due day and keeps no start without one,
with the same success as any write, so the notice is the only word about it;
judging the days before sending would be a second copy of Minka's rule.
**Implemented as:** `cli.dayNotices`, see [To-dos and subtasks](stable/todos.md).

<a id="c-076"></a>
## C-076 — A page of one's own to-dos is bounded here, a page of done ones is not

*Applies to: `todos mine`, `todos completed`, `posts list`, `api.Client.MyTodos`*

`todos mine --limit` outside 1 to 100 is a usage error, and a full page names
the next one in a `notice`. `todos completed --limit` goes to Minka as given.

**Why:** a page of one's own to-dos ends in nothing but being short, so a page
cut down in silence would read as the whole list ([C-054](#c-054)); a page of
done to-dos carries `next`, which says there is more whatever Minka cut. A page of
posts carries `next` as well, so `posts list --limit` goes out as given.
**Implemented as:** `api.MyTodosPageMax`, `cli.newTodosMineCmd`, see
[To-dos and subtasks](stable/todos.md).

<a id="c-077"></a>
## C-077 — The questions are `checkins`, their answers `checkins answers`

*Applies to: every command on a Check-ins tool*

The tools and their questions are `checkins …` (`checkins tools <projectId>`
beside `checkins list <toolId>`), the answers `checkins answers …`, and the
reminders of the person of the token `checkins reminders`.

**Why:** a Check-ins tool is one thing to its users, and its questions are what
the web lists under that name, so they take the noun as the lists of a To-dos
tool take `todolists` ([C-071](#c-071)). The answers are a second kind of ID
and get a group of their own, nested as `todos subtasks` is, so that no position
of the tree takes a question in one command and an answer in the next.
Minka's own addresses split the same way: `checkins/…`, `questions/…`,
`answers/…`.
**Implemented as:** `cli.newCheckinsCmd`, `cli.newCheckinAnswersCmd`, see
[Check-ins](stable/checkins.md).

<a id="c-078"></a>
## C-078 — A schedule goes out as given, and only a word that is no weekday stops here

**Superseded in one point by [C-092](#c-092)**: `checkins create --ask` takes a
name or an e-mail address as well, looked up among the members of the tool's
project. Everything else below holds.

*Applies to: `checkins create`, `checkins edit`*

`--rhythm` is sent as the word given, `--weekday` as a number or as the number
of a weekday's name, `--time` as written; a number outside the week and a
weekday named twice go out as well. Only a `--weekday` that is neither a number
nor a name is a usage error. `--weekday` and `--ask` are the whole set on an
edit, and there is no `--no-ask`. `checkins create --ask` takes an ID or `me`.

**Why:** which schedules exist is Minka's rule, and a second copy of it would
be a second thing to keep in step ([C-070](#c-070)); a word that names no
weekday cannot be put into the list of numbers the address takes. Minka takes
the people and the weekdays as whole sets, and an empty set of people is always
refused, so a flag for it would be a flag that never works. The ID of a tool
names no project, so a new question cannot look up a name, as a new document
cannot ([C-070](#c-070)).
**Implemented as:** `cli.weekdayNumbers`, `cli.peopleOfATool`,
`api.UpdateCheckinQuestionInput`, see [Check-ins](stable/checkins.md).

<a id="c-079"></a>
## C-079 — A page of answers is bounded here, and a full list of reminders says so

*Applies to: `checkins answers list`, `checkins reminders`,
`api.Client.CheckinAnswers`*

`checkins answers list --limit` outside 1 to 100 is a usage error, the size
always goes out, and a full page names the next one in a `notice`, keeping
`--person` and `--on`. `checkins reminders` says in a `notice` that there may be
more where Minka hands over 100.

**Why:** a page of answers is a bare array that ends in nothing but being
short, and Minka cuts a larger page down in silence, so a page cut that way
would read as the whole list ([C-054](#c-054), [C-076](#c-076)). The reminders
take no page at all, so the one thing left to do is to say that the list may be
cut.
**Implemented as:** `api.CheckinAnswersPageMax`, `api.CheckinRemindersMax`,
`cli.newCheckinAnswersListCmd`, `cli.newCheckinsRemindersCmd`, see
[Check-ins](stable/checkins.md).

<a id="c-080"></a>
## C-080 — What a question shows otherwise than it was written is said out loud, and what only changes its form is not

*Applies to: `checkins create`, `checkins edit`*

A `notice` names whoever `--ask` named and `asked` does not show, as not
asked, and the first three days of a question asked every other week after an
edit of its rhythm or weekdays. Sorted weekdays and a two-digit time draw
nothing.

**Why:** a success alone does not say who is left out or which weeks are
asked, and a notice about mere form would teach whoever reads them to stop.
**Implemented as:** `cli.askedNotices`, `cli.fortnightNotice`, see
[Check-ins](stable/checkins.md).

<a id="c-081"></a>
## C-081 — The posts are `posts`, their comments `posts comments`, their archive `posts list --archived`

*Applies to: every command on a Posts tool*

The tools, the kinds and the posts are `posts …` (`posts tools <projectId>`
and `posts categories <projectId>` beside `posts list <toolId>`), the comments
`posts comments …` with the post as the first argument, and the archive of a
tool `posts list <toolId> --archived`, as for a to-do list. Moving a post in and
out of the archive is `posts archive` and `posts unarchive`, and the trash
`posts delete`, as for a card.

**Why:** A post is what the web and Minka's addresses name (`posts/…`,
`posts-tools/…`), and one kind of item hangs under `posts comments`, so the
post takes the first position there as the card does under `comments` and no
flag has to say which kind it is ([C-010](#c-010)). The archive is a list of
the same tool, which `todolists list --archived` already reads that way
([C-071](#c-071)). **Implemented as:** `cli.newPostsCmd`,
`cli.newItemCommentsCmd`, `api.Commented`, see [Posts](stable/posts.md).

<a id="c-082"></a>
## C-082 — The guard over files stands only where the write could land

*Applies to: every text the CLI replaces, comments and answers included*

The guard over files refuses only where Minka would take the write; where Minka
refuses whatever is sent, the request goes out and its refusal is the answer.
One's own comment below an archived folder keeps the guard, since v0.41.0
changes it there. **Why:** a guard in front of a certain refusal points at a
flag that only leads on to it, and a guard too few costs the files.
**Implemented as:** `cli.guardOverFiles`, `cli.keepsTheAttachmentsOfAComment`,
`cli.closedToComments`, see [Rich text](stable/rich-text.md) and
[Docs & Files](stable/docs-and-files.md).

<a id="c-083"></a>
## C-083 — An ID goes out in lowercase

*Applies to: every command that takes an ID, as an argument, a flag or among
the people it names*

Whatever case an ID is typed in, it is written in lowercase before it is
compared or sent. A short identifier of a card is not written in lowercase;
it is looked up in capitals.

**Why:** Lowercase is the form Minka keeps and hands out an ID in, and the one
every version of it reads. A Minka that reads IDs in any case names the same
thing by one in capitals; one from before named nothing by it and answered
something untrue — a person as no member, a step or a subtask not taken off
with a success all the same. The name of a draft of a chat message is refused
in capitals to this day.
**Implemented as:** `cli.asID`, `cli.requireID`, `cli.requireArgs`, see
[Token, hosts and requests](stable/security.md).

<a id="c-084"></a>
## C-084 — A list of who follows an item is written only where the source handed it out

*Applies to: `import`, the `subscribers` of posts, cards, to-dos, documents, uploads and events*

`subscribers` is written only from a whole list Basecamp handed out, `[]` included;
a trashed, not found or forbidden record goes without it and ends 0.
**Why:** Minka takes the list as the whole truth, so a guessed one unsubscribes silently.
**Implemented as:** `basecamp.Extractor.subscribersOf`, `basecamp.handedTo`, see
[Importing from Basecamp](stable/import.md).

<a id="c-085"></a>
## C-085 — A file goes into a tool in one run, and a refused second step hands over what the first stored

**Superseded in one point by [C-092](#c-092)**: the file finds its project
through the tool, not through `--project`. Everything else below holds.

*Applies to: `docs uploads create`, `docs uploads edit`*

`docs uploads create <file>` looks the tool up in `--project`, stores the file
and puts it into the tool; a failed step ends the run. A refused second step
hands over the stored file, and `--blob <blobId>` takes that step alone. An
empty `--title` is refused before anything goes out. `fileUnknown` says whether
this run stored the file; `docs uploads edit` has no flag for the file.

**Why:** Minka refuses a tool of another project and an empty title only after
up to 100 MiB travelled and a retry would send them again, a file stored a
moment ago cannot have been swept, and a flag for the file would promise a
change Minka silently ignores.
**Implemented as:** `cli.newDocsUploadsCreateCmd`, `cli.filesUploadRefusal`,
`api.AddFilesUploadInput`, see [Docs & Files](stable/docs-and-files.md).

<a id="c-086"></a>
## C-086 — The colour of a group goes as it is named, and Minka is the one to refuse it

*Applies to: `import`, the `color` of `todogroups`; `todogroups create`,
`todogroups edit`, `columns edit`, `labels create` and `labels edit`*

The extractor writes the colour of a group as Basecamp names it whenever it is
a string with something in it, `white` included, unchecked, never lowercased
or mapped; where Basecamp names none, the field is left out. The CLI sends
what `--color` says, `""` included, and names the colours in the hint once
Minka has refused one: eleven for a group or a column, twelve for a label. `--color white` takes a colour away, and so does
`columns edit --no-color`, which sends Minka's `null`; there is no `--color
none`.

**Why:** Minka reads Basecamp's names as its own and warns about the rest, so
a list kept here as well would only go stale beside it. `white` is what
Basecamp says of a group nobody coloured, and it is the one word that takes a
colour away in Minka on a later run: left out, a colour taken away at the
source would stay in Minka for ever. Minka answers `null` and `white` alike
with no colour, so one way to say it is enough, and it is the one Minka's own
error message names. `--no-color` exists all the same because the column API
names `null` as the way to take a colour away, and it ends in white. The hint
carries a copy of Minka's eleven names, and no test holds it against Minka,
only against the fake: that is accepted knowingly, because the copy only feeds
a hint and never a refusal.
**Implemented as:** `basecamp.groupColor`, `cli.newTodoGroupsEditCmd`,
`cli.todoGroupColorUnknownHint`, `cli.columnRefusal`, `cli.labelRefusal`, see [Importing from
Basecamp](stable/import.md), [To-do lists and groups](stable/todo-lists.md) and
[Columns](stable/columns.md).

<a id="c-087"></a>
## C-087 — What stands in a list or a group on the shelf or in the trash comes in use

*Applies to: `import basecamp`, the `status` of `todogroups` and `todos`*

A group or to-do marked `inherits_status` is written `active`; one with a
status of its own stays `trashed` or is left out (C-050). The run trusts the
mark: were an own status under such a parent marked too (not seen live, see
[Status](work-in-progress/status.md)), one trashed of its own would be in use.
**Why:** In Minka the list or group alone holds the shelf and the trash, and
restoring it restores all in it. **Implemented as:** `basecamp.bundleStatus`.

<a id="c-088"></a>
## C-088 — Without `--project` a run takes every project, archived ones too

*Applies to: `import basecamp`, `--project`, `--active-only`*

No `--project`: active and archived lists, ID order, a project in both lists
walked once; `--active-only` drops the archived. `--project` is "exactly
these", a usage error with `--active-only`. No trash, stars or hill charts.
**Why:** Basecamp lists active projects alone, and Robert decided the archived
ones go along. **Implemented as:** `basecamp.Extractor.ExtractEveryProject`.

<a id="c-089"></a>
## C-089 — A series edit sends only the flags given, guarded like a document

*Applies to: `todos repeat`, `todos series …`*

Flags mirror Minka's fields (`--weekdays`, `--weekday`, `--instance` 1-4 or
`last`, …); an edit sends only those given, needs `--expect` or `--force`, and
reads the series only for names and for the files of its notes; one that wrote
nothing says so where the `--expect` moment stayed. A detail `--rhythm` does
not use is a usage error, one Minka ignored a notice. `show-now` answers
`{series, todoId}` from `Location`; no day is computed. **Why:** echoed
details pin what Minka lets follow a new first day. **Implemented as:**
`cli.scheduleFlags`, `cli.newTodoSeriesEditCmd`, `api.ScheduleInput`.

<a id="c-090"></a>
## C-090 — What of a to-do's schedule does not fit costs its series, a time of day only itself

*Applies to: `import basecamp`, `repetition` and `repetitionNotMapped` of to-dos*

`repetition_schedule` maps as Minka's format says. An `hour` or `minute` other
than 0 is left off the series; every other part that does not fit
(`week_instance` `-1`, a `days` entry the first day contradicts, …) leaves the
to-do without a series, named by Basecamp's field. A done to-do carries its
schedule but counts as coming without a series.
**Why:** A missing series is named and can be made by hand; one on days nobody
chose makes to-dos somebody has to find and throw away.
**Implemented as:** `basecamp.repetitionOf`, see [Importing from Basecamp](stable/import.md).

<a id="c-091"></a>
## C-091 — A reference is shown by its address, so the view comes back as the reference

*Applies to: `internal/richtext`, every `--styled` text, the guard over files*

A visible `<minka-ref>` shows as `<href>`, whatever its kind, without title or
`#MNK-1`; a hidden one as `(a card you cannot open)`. The guard counts a visible
reference kept by its element, its address or `#short-id`, a hidden one only by
its element; `messages edit` has the guard too. `--json` keeps the served HTML.
**Why:** The view is the editable text (C-008), and only a link showing its
own address comes back from Markdown as the reference.
**Implemented as:** `richtext.reference`, `richtext.Dropped`, `cli.keepsTheAttachments`.

<a id="c-092"></a>
## C-092 — A tool names its project, so nothing asks for one beside it

*Applies to: `docs uploads create`, `docs create --notify`, `posts create
--notify`, `checkins create --ask`*

A file is stored in the project `GET files-tools/{id}` names; a hidden
`--project` other than the tool's exits 2 before the file travels, a matching
one draws a `notice`. `--notify` and `--ask` also take a name or an e-mail
address, looked up among the members of the tool's project, read only for such.
**Why:** the tool names its project, and a script giving `--project`, shipped
from v0.19.0 on, keeps working and never stores the file anywhere else.
**Implemented as:** `cli.projectOfFilesTool`, `cli.peopleOfATool`,
`cli.newDocsUploadsCreateCmd`, see [Docs & Files](stable/docs-and-files.md).

<a id="c-093"></a>
## C-093 — A project edit sends only the flags given, its description as plain text

*Applies to: `projects edit`*

Only `--name` and `--description` given are sent, without `--expect`; an edit
with neither flag is a usage error before any request. The description goes
out as typed (`-` for stdin), with no Markdown and no guard over files. A 404
names the client beside the outsider; one that is no JSON, an older Minka.
**Why:** Minka writes only the fields named and shows the description as text,
so nothing unnamed can be overwritten and nothing is lost.
**Implemented as:** `cli.newProjectsEditCmd`, `api.UpdateProjectInput`, see
[Projects](stable/projects.md).

<a id="c-094"></a>
## C-094 — A chatbot comes as a bot, and a bot carries no address

*Applies to: `import basecamp`, the people of a bundle, `bots` of the report*

`Integration` is a bot, and so is a kind other than `User`, `Client` or none
that wrote a `Chat::Lines::Integration` line the bundle carries: a deleted
chatbot, with `retired: true`. A bot has no `email`, `client` or `company`.
Such a Tombstone other records name too is in `botsAlsoNamedElsewhere`.
**Why:** Minka makes it a bot, ends a run over one with an address, never turns
it back; only lines tell a deleted chatbot from a deleted user.
**Implemented as:** `basecamp.seenPerson.bot`, `bundle.Bot`, see
[Importing from Basecamp](stable/import.md).

<a id="c-095"></a>
## C-095 — The comments of a to-do are `todos comments`, those of a list `todolists comments`, those of an event `events comments`

*Applies to: the comments under a to-do, a to-do list and an event*

Each stands below the command of its item, with the item as the first
argument, cut exactly as `posts comments` ([C-081](#c-081)).
**Why:** One kind of item hangs under each, so no flag has to say which; `show`
carries the comments of a to-do or a list, of an event only their count.
**Implemented as:** `cli.newItemCommentsCmd` with `todoComments`,
`todoListComments` and `eventComments`, see [To-dos](stable/todos.md) and
[Calendars and events](stable/calendar.md).

<a id="c-096"></a>
## C-096 — Calendars come into a bundle only where a run is asked for them

*Applies to: `import basecamp --calendars`, `tools` of kind `calendar`, `events`*

Without `--calendars` a `schedule` of a dock is skipped like a kind Minka has
nothing for, and the report says nothing of calendars; with it the calendar and
its entries are extracted as Minka's format says.
**Why:** A Minka whose import does not apply events stops at the first calendar,
so a bundle with one is only for a Minka that does.
**Implemented as:** `basecamp.Extractor.Calendars`, see [Importing from Basecamp](stable/import.md).

<a id="c-097"></a>
## C-097 — A run refuses an address that writes at the source before asking it

*Applies to: every request of `import basecamp`*

A path with `schedule_entries/` in it ends the run before it is asked, whatever
part of the run asks; calendars are read from their lists alone.
**Why:** Basecamp makes a record of a single day of a series where somebody asks
for it, and a run is a read of the account and nothing else.
**Implemented as:** `basecamp.readsOnly`, see [Importing from Basecamp](stable/import.md).

<a id="c-098"></a>
## C-098 — Calendars and their events are `events`, the calendars `events tools`

*Applies to: `events …`*

One group, cut like `posts`: `tools <projectId>`, `list <toolId>` with
`--from` and `--to` both required, `show`, `create --tool`, `edit`,
`delete`. Time flags mirror Minka's fields; `--repeat` sends a schedule,
`--no-repeat` null, and `--time-zone` without a schedule is a usage error.
**Why:** every tool reads `<group> tools`, and a flag Minka ignores must not
pass without a word. **Implemented as:** `cli.newEventsCmd`, `cli.eventFlags`,
`api.EventInput`, see [Calendars and events](stable/calendar.md).

<a id="c-099"></a>
## C-099 — Versions are `docs versions` and `docs uploads versions`, and another file is a version created

*Applies to: the versions of a document and the files of an upload*

`docs versions list|show` and `docs uploads versions list|create`; `create`
takes a file or `--blob` in the two steps of C-085 and reads the upload and its
versions before the file travels. `--expect` is optional, `--force` absent.
**Why:** the commands follow Minka's addresses (`uploads` holds stored files,
hence no `uploads replace`), a client may read an upload it may not give a
file, and without an expectation nothing is lost: Minka keeps the old file.
**Implemented as:** `cli.newDocsVersionsCmd`, `cli.newDocsUploadsVersionsCmd`,
`cli.uploadVersionRefusal`, see [Docs & Files](stable/docs-and-files.md).

<a id="c-100"></a>
## C-100 — The people and the day of a step are `cards steps edit`, never `cards steps create`

*Applies to: the steps of a card*

`cards steps edit` sends `--assignee` as the whole set, `--no-assignees` as
`[]` and `--due ""` as null, as `todos edit` does; `cards steps create` takes
neither.
**Why:** Minka adds a step from its line alone, and a second request behind it
could be refused after the step stands, leaving half of one command written.
**Implemented as:** `cli.newCardsStepsEditCmd`, `api.UpdateStepInput`,
`cli.stepRefusal`, see [Steps](stable/steps.md).

<a id="c-101"></a>
## C-101 — A comment can wait for the comments before it to be read, and exits 11 where they were not

*Applies to: `comments create`, `comments list`, `comments unread`, `cards show`, `config`*

`--if-latest <commentId|none>` and the setting `comments.requireRead` refuse a
comment with exit 11 (`unread`) and the unread comments in `data`; the marks are
the set of comments shown, per host, token digest and card, in
`comments-read.json`, added to under a lock waited for at most 5 seconds that never fails a read.
**Why:** a set and not the newest misses no comment that slipped in, and 11 is
the first code basecamp-cli does not use (10 is its account limit). **Implemented as:**
`cli.newCommentsCreateCmd`, `readmark`, `cli.newConfigCmd`, see
[Comments on a card](stable/comments.md).

<a id="c-102"></a>
## C-102 — A column is moved next to a working column of its board, which the CLI checks

*Applies to: `columns move`*

`--board` is required, and exactly one of `--first`, `--after <column>` and
`--before <column>`; the board is read, and a neighbour that is no other working
column of it is a usage error before the move goes out.
**Why:** Minka refuses only a neighbour on another board and still places a
column next to Triage, Not now, Done or itself as though it had been put first,
with the same 200, so only the board read here catches those. **Implemented
as:** `cli.newColumnsMoveCmd`, see [Columns](stable/columns.md).

<a id="c-103"></a>
## C-103 — Every session reports its own cards, as a client of its own

*Applies to: `work start`, `work stop`, `work list`, `heartbeat`, `cards move`, `cards done`*

The store keeps one list per host and session; a signal carries one session's
list under that session's client. The session is `--session`, else
`$CLAUDE_CODE_SESSION_ID`, else none, for typed commands and the mod alike; the
standard input of a beat is not read. A store of one list per machine is
handed out. **Why:** another session never keeps a dead one's cards alight,
and a beat going by a session no typed command knows would report nothing.
**Implemented as:** `work.Host`, `work.List`, `cli.app.agentSession`,
`cli.app.signal`, see [Saying what you are working on](stable/at-work.md).

<a id="c-104"></a>
## C-104 — An earlier file of an upload is fetched at its version, the Basecamp CLI's token on the API alone

*Applies to: `import basecamp --upload-versions`, the `versions` of an upload*

Only with `--upload-versions`, off by default, minka asks `basecamp auth token`
for the token per file, keeps it in memory, sends it only to the
version's address on `3.basecampapi.com`, follows no redirect and asks the
signed place on `storage.basecamp.com` without it; both are held to one shape first.
**Why:** no call of the Basecamp CLI brings an earlier file: `files download`
serves the current one for a version's address and ends well, and `api get`
cannot hand out bytes. **Implemented as:** `basecamp.Downloader.DownloadVersion`,
`Extractor.earlierFilesOf`, see [Importing](stable/import.md).

<a id="c-105"></a>
## C-105 — A mod in the Claude Code process beats for its session, idle or at work

*Applies to: the mod, `heartbeat --from-mod --idle`*

The mod lies in `<config dir>/claude-mod` and runs `heartbeat --session <id>
--from-mod` on every step and every 30 s, `--idle` while no tool call runs that
started less than two hours ago and was not abandoned. An idle beat marks
nothing and reports a list only for 25 minutes after the last step, never
relighting what the server forgot. **Why:** only the process knows it has
ended; a tool call that never returns would otherwise keep its session at work
for as long as the process runs, and an open window is nobody at work.
**Implemented as:** `mod.Files`, `cli.nobodyIsAtWorkOn`, `cli.app.claimTheSignal`,
see [Saying what you are working on](stable/at-work.md).

<a id="c-106"></a>
## C-106 — My calendar is `events mine`, what a calendar shows `events settings`, in Minka's words

*Applies to: `events mine`, `events settings`, `events list`*

`events mine --from --to` reads My calendar like `cards mine` and `todos mine`;
`events settings <toolId> --content eventsAndDatedItems|eventsOnly` chooses what
a calendar shows, the value sent as typed and refused by Minka alone. The text
views put dated items under a heading of their own, never among the events.
**Why:** `events tools` takes a project and cannot carry a subcommand, and
basecamp-cli changes the same choice with `schedule settings`; Minka's words
are those `data.content` hands back, as for `--rhythm`. A dated item has a day
and no time, so mixing it in would read as an event.
**Implemented as:** `cli.newEventsMineCmd`, `cli.newEventsSettingsCmd`,
`cli.renderDatedItems`, see [Calendars and events](stable/calendar.md).

<a id="c-107"></a>
## C-107 — The mod is the one thing that reports a Claude Code session; nothing reports after a beat

*Applies to: `heartbeat`, the mod*

A beat reports and ends; no process of minka's own reports between beats. A
Claude Code session reports through the mod alone; whatever runs without it,
such as an agent that is not Claude Code, runs `heartbeat` every minute or two
itself. The 25 minutes of an idle process and the 8 hours of C-040 stay.
**Why:** the mod beats during a long tool call from inside the process, which
alone knows it ended; with the hook gone, only a heartbeat run by hand would
start a keepalive, and its 25 minutes would outlive a session that ended.
**Implemented as:** `cli.newHeartbeatCmd`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-108"></a>
## C-108 — `setup` installs the mod, and goes on where Claude Code cannot take it

*Applies to: `setup`, `hooks install`, `hooks status`*

`setup` installs the mod after the sign-in and the skill, without asking;
`--no-mod` installs nothing. Only a `claude --version` of 2.1.287 or later is
told of it; where `claude` is older, missing or fails, the state is `written`
with a `reason`, the old hook stays, setup exits 0 and `hooks install` 1.
`--mod` is a hidden no-op, `--file` and `--command` are gone. **Why:** everybody
who signs in gets the reporting, a sign-in is worth keeping without Claude Code,
and an older one exits 0 on the install and never runs the mod.
**Implemented as:** `cli.app.installTheMod`, `cli.app.claudeLoadsMods`, `cli.app.setupMod`.

<a id="c-109"></a>
## C-109 — The old hook of minka is cut out of the settings, and nothing else

*Applies to: `hooks install`, `setup`, `hooks status`*

Once Claude Code has the mod, every `PostToolUse` hook of type `command` whose
command is `minka heartbeat` or `/…/minka heartbeat` is cut out of the text,
with what only held it; the cut changes no other byte, the file is written
atomically where a link points, its mode kept. No JSON, or a key twice on the
way to the hook, leaves the file alone. **Why:** the file belongs to whoever
wrote it, and Robert's word allows taking out minka's own entry and no more.
**Implemented as:** `cli.removeTheOldHook`, `cli.isTheHookOfMinka`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-110"></a>
## C-110 — A list from a template is `todolists start`, the templates `todolists templates`

*Applies to: `todolists templates`, `todolists start`*

The templates are listed by `todolists templates`, beside `todolists list`, and
a list is made from one by `todolists start --tool <toolId> --template
<templateId>`, not by a `--template` on `todolists create`. `--title` goes out
only where it is given, so the list takes the template's title otherwise.
There is no `templates` group of its own: a template is a list of the library
and is read and written with the commands of a list, and no flag offers what
the API does not take — a description, a day, people.

**Why:** Minka's one address takes two different bodies: with `template` it
reads no description and refuses in an order of its own, and the answer
carries `leftOff`, which a caller has to read. A command of its own says so in
its help and its result, where `create` would carry a flag that silences
another (C-071 keeps the lists under `todolists`).
**Implemented as:** `cli.newTodoListsTemplatesCmd`, `cli.newTodoListsStartCmd`,
`cli.startRefusal`, see [To-do lists and groups](stable/todo-lists.md).

<a id="c-111"></a>
## C-111 — A record somebody typed wrong costs the record at most, never the run

*Applies to: `import basecamp`, every title and name of a bundle, the days of a
to-do, the times of an event*

A title or a name Minka refuses — empty, with a control character, longer than
Minka holds — is made to fit as a subject is (C-056), the way Minka reads it: a
control character it refuses becomes a blank and one it reads as Windows-1252
stays, only what PHP's `trim` takes comes off, what is too long is cut and ends
in an ellipsis, nothing becomes `(no title)` or `(no name)`. A step holds 500.
`titlesMadeToFit` names each by kind and ID. A to-do that starts after it is due
comes without its start; an event that ends before it starts or lies in the
year 0 is left out. Both
are named. What the source itself wrote in a shape nobody can read still ends
the run.
**Why:** A walk of hours that ends on one character nobody can see never brings
an account across, and the value made to fit is the one Minka would take.
**Implemented as:** `basecamp.Extractor.titleOf`, `basecamp.aTitleOfAtMost`,
`basecamp.companyOf`, `basecamp.daysOfATodo`, `basecamp.timesOfAnEntry`, see
[Importing from Basecamp](stable/import.md).

<a id="c-112"></a>
## C-112 — A text, a description and an icon Minka refuses are made to fit too

**Superseded in one point by [C-114](#c-114)**: a kind of post without an icon
comes without an `emoji`, not with 📝, and `categoriesGivenAnEmoji` stays in
the report, always empty. Everything else below holds.

*Applies to: `import basecamp`, the `body` of a comment and a message, the
`description` of a project, the `emoji` of a category*

A comment and a line of a chat lose every control character but a tab and a
line break (`textsCleaned`). A description of a project longer than Minka's
2000 characters of plain text is cut after the last word that fits, ending in
an ellipsis within them (`descriptionsCut`). A kind of post without an icon
gets 📝 (`categoriesGivenAnEmoji`). Each is named by its ID.
**Why:** The applier stops at each of them, as it does at a title (C-111), and
each is what somebody typed or left empty, not what the source broke.
**Implemented as:** `basecamp.withoutControlCharacters`,
`basecamp.aDescriptionMinkaTakes`, `basecamp.placeholderEmoji`, see
[Importing from Basecamp](stable/import.md).

<a id="c-113"></a>
## C-113 — A reminder is `reminders list|set|delete` with `<kind> <item>`, the kind checked here

*Applies to: `reminders list`, `reminders set`, `reminders delete`*

The group is the plural of the resource, as everywhere, and the kind is a
position of its own beside the ID, taken in any case and sent in Minka's
spelling. A word that is no kind, neither or both of `--preset` and `--at`,
and an `--at` that is not RFC 3339 as Minka reads it are refused before a
request; `--outcome` goes out as written.
**Why:** Minka answers a misspelt kind with the 404 of a thing that is not
there, and it reads a moment in exactly one form, which a check here can
hold without refusing anything Minka takes.
**Implemented as:** `cli.newRemindersCmd`, `cli.reminderSubject`,
`cli.reminderRefusal`, see [Reminders](stable/reminders.md).

<a id="c-114"></a>
## C-114 — A post category is its name, the emoji in it; none is made up for the import

*Applies to: `posts categories`, the `category` of a post, `import basecamp`,
the `emoji` of a category in a bundle*

The CLI shows a kind of post by its name and reads no `emoji` beside it, which
Minka keeps empty for one release and then drops. A kind is still chosen by
its ID alone. A category the source gives no icon goes into a bundle without
`emoji`, neither empty nor 📝. The report keeps `categoriesGivenAnEmoji`,
deprecated and always empty, with neither a notice nor a line of text.
**Why:** Minka made the emoji part of the name (E-144) and takes a kind
without one, putting a given emoji in front of the name; a placeholder would
now be an icon nobody chose. A field of the output never disappears, as Minka
keeps its `emoji` empty rather than dropping it at once.
**Implemented as:** `api.PostCategory`, `cli.newPostsCategoriesCmd`,
`bundle.Category`, `basecamp.Extractor.writeCategories`, see
[Posts](stable/posts.md) and [Importing from Basecamp](stable/import.md).

<a id="c-115"></a>
## C-115 — A run with `--keep-files` keeps the files it brings for the next one

*Applies to: `import basecamp`, every file a run fetches or takes over, the
directory beside `--out`*

With `--keep-files` every file a run holds — fetched or taken over, of a text,
a chat, an upload or an earlier version of one, never an avatar — is linked
into `.<name>.kept` beside `--out` the moment it is whole, and its line in an
index there follows. A run that fails, is stopped or is killed leaves that
directory, and the next run with the flag takes a file over from there when
the index vouches for it, by the rules a file is taken over from a bundle by.
The walk through Basecamp runs anew every time; nothing of the records is
kept. Once a bundle is in place the directory goes. The files are taken over
only by the same build — a version that names its commit, never `dev` or a
`-dirty` one, which `make build` makes of a checkout with any change, an
untracked file included — reading the same account; anything else is refused
as `usage` with the reason. Which projects a run takes and whether it brings
calendars or earlier versions do not count. A run without the flag neither
takes kept files over nor removes them, and names them in a notice and in
`keptFiles`.

**Why:** A run over the whole account takes ten hours, nearly all of them
downloads, and one that failed late lost them all. Only files are
kept, because a record is written in one pass in an order across projects and
could be checked against nothing, while a file carries its own proof: its
identifier, its size at the source and a digest. A file is named the same
whichever projects or options a run has, which is why only the account and the
build count: an identifier means one thing within one account, and a build may
change how a file is named or what an identifier stands for, which no number
short of the commit could tell. The directory has its own name from before the
first file, because a run that is killed runs no handler and the next run
tidies away what was being built. A run without the flag leaves the files,
because removing hours of downloads over a forgotten flag is the loss the flag
is there to prevent, and says where they lie, because they are personal data
outside of any bundle. The bundle in `--out` stays after every run that put
it there, with or without the flag, until somebody deletes it.
**Implemented as:** `bundle.Writer.Keep`, `ReadsFrom`, `KeepFile`, `Kept`,
`bundle.KeptBeside`, `cli.buildNamesItsCode`, `cli.theKeptFiles`, see
[Importing from Basecamp](stable/import.md).

<a id="c-116"></a>
## C-116 — `--agent` writes a user of the account as the agent of another, for a while

*Applies to: `import basecamp --agent`, the people of a bundle, `agents` of the
report*

**Temporary:** for moving Flownative's own account over, three people; it goes
once that is done, with `basecamp/agents.go`, `bundle.Agent` and the calls
marked `the agents'`.

`--agent <agent ID>=<human ID>`, repeated, writes that person with
`kind: "agent"`, `principal` the human's `sourceId`, and no `email` key at
all. A value of another shape, an agent of themselves, one agent for two
humans and somebody named as an agent and as the human of one are refused as
`usage` before anything is asked of Basecamp. Agent and human must both be in
the account's people list as a `User` who is no client, which is checked as
`usage` once that list is read, before a project is asked for. An agent comes
like anybody else, only where a record refers to it; its human then comes
whether any record names them or not. Agents are written after everybody
else.
**Why:** Basecamp has no agents; only whoever runs the import knows who is
one. The checks are what Minka needs of an agent (one human, never another
agent, nobody it may not act for) and cost a run of hours nothing when made
before the walk. The agents to move are users of the company acting for
users of the company: a bot, somebody deleted or a client is a mistake in the
call, and somebody the list does not hold cannot be told to be none. No
address, because the agent must not be found by one and get a login. The
agent refers to the human, so the human is written; an agent nothing names
has nothing to carry. Writing the agents last puts the human's line before
the agent's, as Minka requires: it refuses an agent whose principal no
earlier line brought. An agent counts as having an address wherever an
address decides who is kept, because Minka gives it one. A member agent whose
human is no member of the project is not counted, because only Minka knows
whether the human is a member there already. Minka stops a run over an agent
it cannot make (E-263 there: a login of its own, another organization, agents
or bots of its own, the owner or admin role, the last member of a project its
human is not in); none of it can be seen in Basecamp, so the import docs advise
a dry run of Minka's import first.
**Implemented as:** `basecamp.ParseAgents`, `registry.takeAgents`,
`bringInThePrincipals`, `inWritingOrder`, `bundle.Agent`, see
[Importing from Basecamp](stable/import.md).

<a id="c-117"></a>
## C-117 — `notifications wait` goes on from a position kept per account and session, and exits 12 when its time runs out

*Applies to: `notifications wait`*

It is the group of the resource and an action (C-001), not a bare `wait`.
The position Minka hands out is kept beside the configuration per host, token
and session (C-103, no session included), moved on only after the lines are
printed and only from the one the process read; the first wait starts from
now, a forgotten one from now with a notice. Filters read what they leave
out. It asks in waits of at most 50 s, as long as Minka holds one, and
`--timeout` ends with exit 12
(`timeout`); a failed ask is no retry but a retryable exit, and an ask that
comes back early and empty is followed by a growing pause.
**Why:** a line may be printed twice but never lost, and basecamp-cli uses 10.
**Implemented as:** `newsmark.MoveOn`, `cli.newsWait`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md).

<a id="c-118"></a>
## C-118 — The own coordinator entry of a board is `boards coordinator claim|release|show`

*Applies to: `boards coordinator claim|release|show`, `boards show`*

One verb each under `boards`, for the own entry: no takeover, an entry named
as it came, an older Minka's 409 reported as is. `--session` is required.
**Why:** a group and an action (C-001), and Minka keeps one entry per account
and board that nobody else sees.
**Implemented as:** `cli.newBoardsCoordinatorCmd`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#coordinating).

<a id="c-119"></a>
## C-119 — `auth logout --revoke` revokes first and keeps the entry when that fails

*Applies to: `auth logout --revoke`, `DELETE /api/v1/me/token`*

The token is revoked at the entry's host, read like every host, before the
entry leaves the file. Minka's own 401 counts as revoked; any other failure,
a 401 of a front door included, keeps the entry and exits with its own code.

**Why:** The entry is the one copy of the token here, and only Minka's 401
says the token is dead, so a second run after a lost answer ends like the first.
**Implemented as:** `cli.app.revokeToken`, see
[Token, hosts and requests](stable/security.md).

<a id="c-120"></a>
## C-120 — One tool is `<group> tool <toolId>`, one chat is `chats show`

*Applies to: `chats show`, `todolists tool`, `posts tool`, `docs tool`,
`checkins tool`, `events tool`*

A chat has a group of its own and is read with `show`; a tool has none, so one
tool is `tool` in the group of what it holds, beside `tools <projectId>`.

**Why:** `show` in that group already names what the tool holds (`todolists
show` is a list). **Implemented as:** `cli.app.newOneToolCmd`,
`cli.app.newChatsShowCmd`, see [Output](stable/output-contract.md).

<a id="c-121"></a>
## C-121 — `uploads download` sends no `?download`

*Applies to: `uploads download`, `GET /api/v1/uploads/{blobId}/{variant}`*

The bytes are asked for without `download`.

**Why:** It changes only the headers around the same bytes, and the CLI takes
the name from the resource, not from a header. **Implemented as:**
`api.Client.FileBytes`, see [Output](stable/output-contract.md).

<a id="c-122"></a>
## C-122 — One line of the bell is `notifications show|mark-read <notificationId>`, its number `notifications count`

*Applies to: `notifications count`, `notifications show`, `notifications
mark-read`, `GET /api/v1/me/notifications/count`, `GET` and `POST
/api/v1/notifications/{notificationId}[/read]`*

The routes of the native app stand under `notifications` beside `wait`, and
take the `id` of a line a wait printed. Marking read is `mark-read`, not
`read`, as for a chat.

**Why:** `read` beside `show` would read as reading the line; `chats
mark-read` already names the same act (C-009). **Implemented as:**
`cli.app.newNotificationsShowCmd`, `cli.app.newNotificationsMarkReadCmd`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md).

<a id="c-123"></a>
## C-123 — A step is moved to a place that is named, its end found in the card

*Applies to: `cards steps move`, `POST /api/v1/cards/{cardId}/steps/{stepId}/moves`*

Exactly one of `--top`, `--bottom`, `--after <step>` and `--before <step>`, as
for a group and a to-do ([C-072](#c-072), [C-074](#c-074)), but with
`--bottom`: it sends the last other step of the card as read as the previous
one, or no neighbour where there is none. Where that step is gone by the
move, Minka's `stepPlaceUnknown` comes with a hint of its own. The step named as its own neighbour goes to
Minka, which answers `stepPlaceUnknown`.

**Why:** Minka moves a step without a neighbour to the top with the same 200
as any move, so a forgotten flag would reorder the steps without a word, and
given both neighbours it places the step behind the previous one alone. Unlike
a group, a step is named through the card the command reads anyway, so the
last step is known without a request of its own; Minka's tests send the end
the same way. Which neighbour a step may have is Minka's rule, as in
[C-070](#c-070). **Implemented as:** `cli.app.newCardsStepsMoveCmd`,
`api.MoveStepInput`, see [Steps](stable/steps.md).

<a id="c-124"></a>
## C-124 — The files of a back are `cards notes attachments`, shown as text only where they are text

*Applies to: `cards notes attachments list|create|show|download|delete`, `/api/v1/cards/{cardId}/attachments…`*

The files sit under the notes as a plural group with the usual actions; `create` does both requests and reads card
and back first, `show` prints a text alone and refuses a non-text with exit 1, `download` writes any file as `uploads
download` does. An attachment is named by ID, position or file name.

**Why:** A back is one thing with two parts, a group keeps the names of C-001, and a picture on a terminal is noise
while a text an agent reads back must arrive unchanged. **Implemented as:** `cli.newCardsNotesAttachmentsCmd`,
`api.CardBackFile`, see [The back of a card](stable/card-back.md).

<a id="c-125"></a>
## C-125 — The steps of a card are `cards steps`, with no top-level alias

*Applies to: `cards steps list|create|edit|move|complete|reopen|delete`*

What hangs on a card alone lives under `cards`, beside `cards notes`; `minka
steps` is an unknown command. What hangs on everything, such as `comments`,
keeps its own group.

**Why:** A step hangs on a card alone, like its notes, and an alias would keep
two names for one thing. **Implemented as:**
`cli.newCardsStepsCmd`, see [Steps](stable/steps.md).

<a id="c-126"></a>
## C-126 — The skill is a small core, the rest stands in references by subject

*Applies to: `skills/minka/SKILL.md`, `skills/minka/references/`*

SKILL.md holds the invariants, the daily commands of a board and a table that
names the reference for every other subject; where a reference has commands of
its own, they stand at its top. Text moves between the files, it is not kept
twice.

**Why:** Every agent that touches a card loads SKILL.md whole, and most need
none of the tools beside the board. **Implemented as:** `skills/minka/`, see
[The skill](stable/skill.md).

<a id="c-127"></a>
## C-127 — Outside Minka a card is named by its short address

*Applies to: `skills/minka/references/texts.md`, commits and pull requests, the text views of a card*

A commit, a pull request or any text outside Minka names a card by `shortUrl`,
and by `appUrl` only where the board has no prefix; against a Minka without
the field, the skill builds it as `https://<host>/go/card/<shortId>`. The CLI
passes `shortUrl` through, on a card and on the subject of a notification, and
its text views give it in place of `appUrl` where it is there and not null.

**Why:** The short address reads at a glance and stays valid when the prefix
changes, while `appUrl` carries two IDs nobody can read. **Implemented as:**
`skills/minka/`, `cli.cardAddress`, see [The skill](stable/skill.md).

<a id="c-128"></a>
## C-128 — Every text view of a comment gives its address

*Applies to: the text views of comments, the answers of `comments create` and `edit`, `skills/minka/references/texts.md`*

The text views give a comment's `appUrl`, its item's page anchored at the
comment, as a `URL:` line below it, and the writes of a comment on the line
below their confirmation; where Minka leaves the field out, nothing stands
there. Outside Minka, one comment is named by that address.

**Why:** It is the one address that opens the item at the comment, and an older
Minka must read the same as before. **Implemented as:** `cli.commentAddressLine`,
`cli.writtenComment`, see [Comments on a card](stable/comments.md).

<a id="c-129"></a>
## C-129 — A newer minka is named once a day, from what Minka's answers carry

*Applies to: every command, `doctor`, `cli-update.json`, the envelope field `cliUpdate`, `skills/minka/SKILL.md`*

The versions Minka names in `Minka-CLI-Latest` and `Minka-CLI-Minimum` are
remembered beside the configuration, without a request of their own; an answer
naming either header stands for both, one naming neither changes nothing.
Only a release `x.y.z` compares itself. Text gets a line on stderr, `--json`
the field `cliUpdate`, each at most once a day on a day of its own; `--quiet`
gets nothing. Below the minimum is a warning in `doctor`, behind is not.

**Why:** A hint that costs a request, breaks a pipe or fails a command would be
in the way more often than an old minka is. **Implemented as:**
`internal/cliupdate`, `cli.settleCLIUpdate`, `cli.doctorUpdateCheck`, see
[Output](stable/output-contract.md#a-newer-minka).

<a id="c-130"></a>
## C-130 — Each kind of entry of Docs & Files is kept in order in its own group

*Applies to: `docs`, `docs uploads` and `docs folders` with `delete`, `restore`, `archive`, `unarchive`, `move`; `docs folders create`, `edit`; `docs versions restore`; `docs list --archived`*

A document, an upload and a folder take the same five writes, each in the group
of its kind; a move names `--folder <folderId>` or `--top-level`, exactly one.
The archive of a tool is `docs list <toolId> --archived`, as for posts and lists.

**Why:** The ID of one kind is nothing at the address of another, so the group
says the kind as `--document` and `--upload` do for comments, and the cards'
`--top` names a position, not a place. **Implemented as:** `cli.orderCommands`,
`cli.entryOrderRefusal`, see [Docs & Files](stable/docs-and-files.md).

<a id="c-131"></a>
## C-131 — A release tells Minka its version, and a failed report never fails it

*Applies to: `.github/workflows/release.yml`, the job `announce`*

Once the formula points at a stable version, the release workflow sets it as
Minka's latest CLI version (`PUT /api/v1/operations/cli-version`), with a token
that may do nothing else, kept in the environment `release`. A missing token, a
refusal or no answer is a warning on the run, never a failed release.

**Why:** The update notice of C-129 is only right if nobody has to remember the
version and never names one `brew upgrade` cannot install, while a release
whose archives and formula are out is done. **Implemented as:** the job
`announce`, see [Releases](stable/releases.md#telling-minka-the-version).

<a id="c-132"></a>
## C-132 — `search` checks what Minka would let pass and reads only the pages its limit needs

*Applies to: `search`*

A missing `--tool`/`--folder`, a `--sort` other than `best` or `recent` and a
malformed `--offset` are exit 1 before any request. `--limit` reads Minka's
pages of 25 only while hits are lacking and may stop inside one; `data` is
`{hits, next}`, `next` the offset of the first hit left behind. The command to
read on puts the term first, and behind `--` only where it starts with a dash.

**Why:** Minka takes an unknown order as `best` without a word, and its offset
counts hits, so a cut page still says where the rest begins. **Implemented
as:** `cli.newSearchCmd`, `api.SearchUpTo`, see
[Docs & Files](stable/docs-and-files.md#searching-a-tool-or-a-folder).

<a id="c-133"></a>
## C-133 — Books and the order set by hand go out only as asked, and only what Minka wrote passes

*Applies to: `docs order`, `docs book`, `docs folders edit --book`/`--no-book`, `docs edit --in-navigation`/`--not-in-navigation`, every `move --after`/`--before`*

Exclusive pairs, `=false` switches, an unknown order and a neighbour that is
no ID or the entry itself are exit 1. The navigation alone needs neither
`--expect` nor `--force`. An answer without the mark or navigation asked for
is exit 7. Arranged by hand, `docs list` prints one mixed list.

**Why:** A flag-only PATCH writes only the flag and never answers 409, a Minka
without books says 200 to both, and three lists cannot show a mixed order.
**Implemented as:** `cli.newDocsOrderCmd`,
`cli.newDocsBookCmd`, `newEntryMoveCmd`, `newDocsEditCmd`, see
[Docs & Files](stable/docs-and-files.md#books-and-the-order-of-a-place).

<a id="c-134"></a>
## C-134 — The shapes of a document travel in conventions other Markdown tools read

*Applies to: `internal/markdown`, `internal/richtext`*

A callout is a GitHub alert with an optional title beside the marker, a Mermaid
diagram a `mermaid` code block, link tiles a list and a block of variables a
table, each under an HTML comment naming it; every code block is written as the
editor stores one, `pre[data-language]` text with `<br>`.

**Why:** elsewhere the alert reads as a quote and the comment is hidden, and
only the editor's code block gets its colours on the page. A release of the
CLI waits for a Minka release that keeps the shapes. **Implemented as:**
`markdown.ToHTML`, `richtext.ToMarkdown`, see
[Rich text](stable/rich-text.md#the-shapes-of-a-document).

<a id="c-135"></a>
## C-135 — An export writes only what its manifest names, and its links stay among its files

*Applies to: `docs export`, `internal/export`, `richtext.HeadingAnchors`*

Pages are files named by their title's slug, headings carry Minka's anchors,
references into the export are relative links. A manifest names every file
before it is written; anything else in the way refuses the run, `--force` or not.

**Why:** a refresh removes what Minka dropped, and only the manifest tells the
export's files from a person's. **Implemented as:** `export.Run`,
`cli.newDocsExportCmd`, see [Docs & Files](stable/docs-and-files.md#exporting-a-book-or-a-folder).

<a id="c-136"></a>
## C-136 — An export reads a tree in two requests and a page only when Minka changed it

*Applies to: `docs export`, `internal/export`*

Folder and tree are read with `…/{id}` and `…/tree`, books too; a page is taken
from the manifest where the tree names the version read and its references stay
inside the export; every page is rendered anew against the run's tree. Every
edit makes a version, a maintenance rewrite does not: `--read-all` reads all.
An export from a tree is refused, not shrunk, on an answer without trees.

**Why:** a refresh of a large book costs two requests, and a rendering from the
body kept follows renames and moves that no version of the referring page
records. **Implemented as:** `export.loadTree`, `export.readDocuments`,
`export.cachedPageOf`, see [Docs & Files](stable/docs-and-files.md#exporting-a-book-or-a-folder).

<a id="c-137"></a>
## C-137 — A text that is mostly HTML is not sent as Markdown

*Applies to: every command that converts Markdown, `internal/markdown`*

When more than half of the non-blank source lines belong to HTML blocks (the
markers of the shapes, code and inline HTML not counted), the command sends
nothing and exits 1 with `usage`, naming its own `…format html` flag and the
fenced code block. Below that, HTML stays visible text with a `notice`.

**Why:** such a text was meant as HTML and would arrive as its source code,
with a `notice` that is easily cut off; one snippet in Markdown is an example.
**Implemented as:** `markdown.Result.MostlyHTML`, `cli.markdownToHTML`, see
[Rich text](stable/rich-text.md#html-given-as-markdown).

<a id="c-138"></a>
## C-138 — A label named by its name is looked up in the list of its place

*Applies to: `labels edit`, `move` and `delete`, the neighbour of `move`*

A `<label>` that is no ID is found in `GET …/labels` of the place, compared as
Minka compares names (composed, spacing tidied, case-folded), among the place's
own labels only; a label of the project under a board, or none, exits 2 with a
hint naming the place to use. An ID is sent as it is, and nothing is read.

**Why:** the place routes take only an ID, and a name is what a person and an
agent know a label by; a lookup that matched otherwise than Minka would find a
label Minka would not, or miss one it would.
**Implemented as:** `cli.resolveLabel`, `cli.labelNameKey`, see
[Labels](stable/labels.md#naming-a-label).

<a id="c-139"></a>
## C-139 — On an item, an ID goes as labelId and anything else as name

*Applies to: `cards label`, `cards unlabel`, `docs label`, `docs unlabel`,
`docs uploads label`, `docs uploads unlabel`*

A `<label>` that is a UUID goes as `labelId` (path for `unlabel`), lowercased;
anything else as `name` (`?name=` for `unlabel`), and Minka resolves it. Never
both, and an empty name is refused with exit 1 before any request.

**Why:** Minka resolves names on items itself, among exactly the labels the
item can take, so a lookup here would only add a request and a second matcher;
both fields at once and an empty one are refusals Minka would answer anyway.
**Implemented as:** `cli.newItemLabelCmd`, `api.AssignLabel`,
`api.UnassignLabel`, see [Labels](stable/labels.md#labels-on-items).

<a id="c-140"></a>
## C-140 — Several labels go one request each, and the first refusal stops the run

*Applies to: the `label` and `unlabel` commands of items*

The labels are sent in the order given, one request each. The first refusal
ends the run with its code; the error names that label and carries
`data.applied` and `data.notApplied`. On success `data` is the item's labels
afterwards.

**Why:** the API takes one label per request; going on after a refusal would
leave a set nobody asked for, while stopping leaves a prefix the caller can
read off and finish.
**Implemented as:** `cli.newItemLabelCmd`, see
[Labels](stable/labels.md#labels-on-items).

<a id="c-141"></a>
## C-141 — The labels of a new card are named here and sent as IDs, each once

*Applies to: `cards create --label`*

An ID is sent as it is; a name is found in `GET /boards/{id}/labels`, read once
and only for a name, as Minka compares names: one found nowhere exits 2 before
any write. A label named twice goes once; two of one scope are left to Minka.

**Why:** the card route takes only IDs, and a scope check here would be a
second rule beside Minka's that the list alone cannot always decide.
**Implemented as:** `cli.labelsForANewCard`, see
[Labels](stable/labels.md#labels-on-a-new-card).

<a id="c-142"></a>
## C-142 — A scope rename goes as typed and answers with the place's labels

*Applies to: `labels rename-scope`*

`<scope>` and `<newScope>` are sent as typed, nothing is read first, and Minka
judges both; `data` is Minka's answer, the place's own labels afterwards, and
the text view lists them.

**Why:** Minka compares scopes, finds the labels and checks every new name in
one transaction; a lookup here would only add a request and a second matcher.
**Implemented as:** `cli.newLabelsRenameScopeCmd`, see
[Labels](stable/labels.md#writing-a-places-labels).

<a id="c-143"></a>
## C-143 — The icon of a label goes as it is named, and Minka is the one to refuse it

*Applies to: `labels create --icon`, `labels edit --icon` and `--no-icon`*

`--icon` is sent as typed, `""` included, and `--no-icon` as `null`; the CLI
keeps no list of Lucide's names and points to https://lucide.dev/icons once
Minka has refused one with `labelIconUnknown`.

**Why:** Minka bundles the whole Lucide set with its aliases and moves with
it, so a list here would only go stale beside it, as for colours (C-086).
**Implemented as:** `api.LabelInput`, `cli.labelRefusal`, see
[Labels](stable/labels.md#icons).

<a id="c-144"></a>
## C-144 — `cards mine --board --column` narrows on the server and here

*Applies to: `cards mine --board`, `cards mine --column`*

Both go as `boardId` and `columnId`, checked as IDs first; a column title needs
`--board` and is looked up there. The answer is narrowed again by each card's
`boardId` and `column.id`; a card without them refuses the filters, exit 7.
**Why:** an agent knows the ready column by its title, and a Minka that ignores
the filters must not hand an agent a card nobody released to it.
**Implemented as:** `cli.newCardsMineCmd`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#coordinating).

<a id="c-145"></a>
## C-145 — The skill lets an agent take only its own cards in a column with the role `ready`

*Applies to: the skill and its references on boards and coordinating*

Every agent takes only a card in a column with the role `ready` assigned to
its own account, hands out no work, asks on a card of several agents, puts
follow-ups unassigned into Triage (the inbox), and writes the back of its own
cards.
**Why:** the column says what is released and in which order, the assignment
for whom, so agents need no coordinator to keep them apart.
**Implemented as:** `skills/minka`, see [The skill](stable/skill.md).

<a id="c-146"></a>
## C-146 — A column role goes as typed, and an answer without it is no success

*Applies to: `columns edit --role` and `--no-role`*

`--role` is sent as typed and `--no-role` as `null`; Minka refuses an unknown
role, and an answer whose role is not the one sent exits 7.
**Why:** Minka owns the set of roles as it owns the colours (C-086), and a
Minka without roles answers 200 having written only the title.
**Implemented as:** `api.UpdateColumnInput`, `cli.newColumnsEditCmd`, see
[Columns](stable/columns.md#writing).

<a id="c-147"></a>
## C-147 — The conventions go as Markdown, and Minka alone decides who writes them

*Applies to: `boards conventions set` and `show`*

The text goes unconverted, from stdin with `-`; the CLI checks neither writer
nor text, and names Minka's 403 as `forbidden` with the way on. `show` prints
the text alone on stdout.
**Why:** Minka keeps Markdown here and refuses the writer before the text, so
a check here could only hide that refusal or drift from it.
**Implemented as:** `cli.newBoardsConventionsCmd`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#the-conventions-of-a-board).

<a id="c-148"></a>
## C-148 — `notifications wait --board` keeps a line about the board itself

*Applies to: `notifications wait --board`, `--column`*

A line whose subject is the board named by `--board` passes; with `--column`
it does not.
**Why:** `board.conventionsChanged` names no card, and a coordinator waits
with `--board` and must not miss that its conventions changed.
**Implemented as:** `cli.newsFilter`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#the-conventions-of-a-board).

<a id="c-149"></a>
## C-149 — `cards next` is the card Minka names, never one the CLI works out

*Applies to: `cards next`*

The card Minka's `next-card` names; none, or no `ready` column, is exit 0 with
`data` null. A Minka without the address is exit 2 with a message that says
so, never a fallback to an order computed here.
**Why:** one rule in one place, so that the board's people and every agent
agree on which card is next, and "nothing for you" is a normal answer.
**Implemented as:** `cli.newCardsNextCmd`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#coordinating).

<a id="c-150"></a>
## C-150 — The skill ranks a board's conventions below the agent's own user

*Applies to: the skill and its references on boards and coordinating*

The conventions describe only how work on the board is done, grant no right,
rank below the user's instructions; the agent reads them on a claim and on
`board.conventionsChanged`, tells its user when the writer changed, and never
writes them.
**Why:** agents follow the text like a skill, so it must not become a way
around the person the agent works for.
**Implemented as:** `skills/minka`, see [The skill](stable/skill.md).

<a id="c-151"></a>
## C-151 — The text view frames the conventions and shows their invisible characters

*Applies to: `boards show`, `boards coordinator claim`, `boards conventions show`*

The conventions stand between a line naming writer, version and time, saying
they grant no rights, and an end marker; what Minka refuses as invisible is
printed as `<U+XXXX>`, with a notice. `--json` stays as stored.
**Why:** agents read the text like instructions, so neither its headings nor
a bidi override or hidden tag may make it read otherwise than it is.
**Implemented as:** `cli.renderConventions`, `cli.visibleFormatCharacters`, see
[Waiting and coordinating](stable/waiting-and-coordinating.md#the-conventions-of-a-board).

<a id="c-152"></a>
## C-152 — Text from stdin loses one leading byte order mark

*Applies to: every text read with `-` from stdin*

`readValue` drops exactly one U+FEFF at the start of stdin; a second one, one
inside the text and a text argument go to Minka as they are.
**Why:** editors put the mark in front of files and nobody means it, while
Minka refuses it in notes and conventions; anything more is Minka's to judge.
**Implemented as:** `cli.readValue`, see
[The output contract](stable/output-contract.md).

<a id="c-153"></a>
## C-153 — The card of a card step goes out as Minka reads a card's name

*Applies to: `cards steps create --card`*

One leading `#` comes off, a short or long address of this host gives up the
short identifier or ID in it, an ID goes out in lowercase; anything else,
another host's address included, goes as given for Minka to answer.
**Why:** agents name cards as the skill teaches, `#MNK-1` or the address,
and an address of another Minka must not name a card of this one.
**Implemented as:** `cli.session.cardNameForMinka`, see [Steps](stable/steps.md#a-card-as-a-step).

<a id="c-154"></a>
## C-154 — A step becomes a card with `cards steps make-card`

*Applies to: `cards steps make-card`, `cards steps create --card`*

A card step is added with `cards steps create --card`, a step in words made a
card with `make-card`, `data` the new card; against a Minka without card
steps both exit 2 with a message that says so, never a fallback.
**Why:** both stay with the other step commands, and an old Minka's
`stepMissing` or 404 page would otherwise blame the input.
**Implemented as:** `cli.newCardsStepsMakeCardCmd`, `cli.addCardStep`, see [Steps](stable/steps.md#a-card-as-a-step).

<a id="c-155"></a>
## C-155 — A card step reads as its card, without a tick box

*Applies to: `cards show`, `cards steps list` and the answers of the step commands*

`↳ #MNK-12 Title  [Column] (people) <short address>`, the short address built
on `appUrl`'s host; `(a card you cannot open)` for an unseen one; `Step of:`
below a card's title. A step is never named by an empty text.
**Why:** the column is its state, so a box of its own would mislead, and an
unseen card's empty title must not be matched by `""`.
**Implemented as:** `cli.cardStepLine`, `cli.stepOfLine`, `cli.findLine`, see [Steps](stable/steps.md#a-card-as-a-step).
