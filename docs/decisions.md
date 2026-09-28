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
## C-040 — A card is given up eight hours after its session was last at it

*Applies to: the list of `internal/work`*

A card remembers the session that took it up, and that session says after every
tool call that it is still at it, at most once a minute. Eight hours after the
last of those the card is dropped. A card taken up where nothing says who is
asking goes by the moment it was started. Nothing of the contract asks for any
of this; the server forgets a card three minutes after the last signal, and
this is the client's own rule.

**Why:** A session that dies says nothing more about its cards, and every other
session on the machine would keep reporting them for as long as it runs — which
is why the sign of life has to be the session's and not the machine's: a signal
carries the whole list, so a signal as the sign of life would make one session
at work keep every dead session's cards alive. Eight hours is a working day:
long enough for the work itself, short enough that nothing glows through the
night.
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
detached and under a lock of its own. It is at no work: no session name, and
neither mark written. It reports an empty list before it stops; a silent store
and a jumped clock stop it without a word.

**Why:** A long tool call makes no signal for longer than the three minutes the
server counts one for, and a process that could pass for a session at work would
keep a card alight for a working day with nobody left to stop it.
**Implemented as:** `cli.app.keepTilesAlive`, `cli.tick.decide`,
`cli.app.somebodyIsAtWork`, `work.HoldKeepalive`, see
[Saying what you are working on](stable/at-work.md).

<a id="c-044"></a>
## C-044 — A host is read, or it is no host

*Applies to: `--host`, `$MINKA_HOST`, the default host of the configuration*

A value that names no machine is refused, and a default host the CLI cannot read
is refused where it is read, so that no session comes about under it. Signing out
is the one command that takes an entry as the file writes it, so that one written
by hand can be taken out again.

**Why:** Such a value would become a key of the configuration, a key of the
list of cards and part of the name of a lock, and a session under it would
carry it into everything that prints a host and into the argv of the keepalive.
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

*Applies to: `docs edit`, `cards notes set`*

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
**Implemented as:** `cli.documentGuard`, `api.UpdateDocumentInput`, see
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

*Applies to: `docs create --audience` and `--notify`*

The CLI checks that `--audience` is one of its three words and that a
`--notify` is an ID or `me`, and nothing more. `chosen` without a name and
names under another audience go out as they are given; Minka answers them with
`notifyIdsMissing` and `notifyIdsWithoutChosenAudience`, exit 9, and the hint
names the flag to change. `personCannotSeeIt` is exit 9 as well, not
retryable, and its hint says that only another name gets the document written.

**Why:** the CLI refused both mismatches itself while Minka had not yet
answered them. Now that it does, a second copy of the rule would be a second
thing to keep in step with the server, and the round trip it saved is the only
cost of leaving it out. Whether a named person may be told cannot be judged
here at all: the CLI does not know the project before the document exists, nor
which folder its clients are shown.
**Implemented as:** `cli.checkAudience`, `cli.documentRefusal`, see
[Docs & Files](stable/docs-and-files.md).
