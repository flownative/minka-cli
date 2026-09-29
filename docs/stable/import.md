# Importing from Basecamp

`minka import basecamp` is an extractor: it walks a Basecamp account and writes
a bundle in Minka's import format. The format is Minka's
(`Documentation/stable/02-architecture/import-format.md` there) and the only
thing the two sides share; Minka's applier reads the bundle, the CLI never
writes history into Minka itself.

```
minka import basecamp [--project <id> … | --active-only] --out <dir> [--dry-run]
```

The command talks to Basecamp only, and only reads there. It needs no Minka
host and no token.

## Which projects a run takes

**Without `--project` a run takes every project the Basecamp identity can see,
the active and the archived ones** (C-088). It reads `projects.json`, which
holds the active projects alone, and `projects.json?status=archived`, each with
all its pages, once the people of the account are read. Every project is then
asked for by its ID, as a named one is, so an archived project comes with
`status` `archived` the way `--project` has always brought it. A project met in
both lists, as one that went on the shelf between the two requests is, is
walked once. **The two lists are no snapshot:** a project taken off the shelf
between them, or one that moves while their pages are walked, can be missed;
the run notices a project met twice and walks it once, but not one met
nowhere. `--active-only` leaves the archived list unasked.

- `--project` names **exactly** the projects to take, whatever their status,
  and is repeated for several; no list is read. Together with `--active-only`
  it is a usage error: the one says "exactly these", the other "pick among all
  of them", and a filter over named projects would drop a project somebody
  named without a word.
- **Projects in the trash are never taken** and never asked for, neither listed
  nor counted: the trash is outside the selection, as a project nobody named is
  outside a run with `--project`, and no report names what a run was not asked
  to take.
- The projects stand in the bundle **in the order of their IDs**, active and
  archived mixed, whichever way they were chosen, so that two runs over the
  same account write the same bytes.
- An identity whose lists hold no project — with `--active-only`, no active one
  — ends the run as `not_found` before anything is written; the `hint` names
  `basecamp projects list --all`, with `--status archived` for the shelf. Only
  a missing project carries that hint: anything else not found, a tool that
  answers 404 among it, gets one that says to look up what the message names.
- A listed ID is held to the same shape as a named one; anything else ends the
  run as `api_error` before the project is asked for.

## Through the Basecamp CLI

The extractor does not speak the Basecamp API itself. It runs
`basecamp api get <path> --agent` for every request, and
`basecamp files download` for every file of a text or a chat, see Files
(C-023): the Basecamp CLI brings the sign-in, the refreshing of the token,
rate limiting and retries, and no Basecamp token ever passes through minka.
The Basecamp CLI has to be installed and signed in.

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
  a usage error before a request goes out and before a directory is made, and
  so is `--project` together with `--active-only`.
- The address of a project has to have the shape
  `https://<host>/<account>/projects/<id>.json`, and all projects of a run the
  same account: the account names the bundle, and its address is what every
  other address is held against.
- A run that is stopped is `interrupted`: when the context is done, or when
  SIGINT or SIGTERM ended the Basecamp CLI, which the signal may reach first.
  A Basecamp CLI that any other signal ended, such as the kill of a system out
  of memory, is an `api_error` that names the signal.
- A request that fails for a reason that may pass is asked again before the
  run gives up (C-029): at most four times (`MaxAttempts`), after pauses of 2,
  10 and 30 seconds (`RetryPauses`). Such a reason is a failure of the
  network, the rate limit, and a failure of the API that may pass, see below.
  Every other failure ends the run at the first answer: not found, not signed
  in, forbidden, a request Basecamp refuses, an answer that is no JSON, a
  Basecamp CLI that crashed or that a signal ended. A run that is stopped
  during a pause ends at once as `interrupted`, without a further request.
  When the attempts are used up the run ends with the last failure, which is
  `retryable`. A run asks again at most 100 times (`MaxRetriesOfARun`): a
  source that fails now and then stays far below that, and one where every
  request needs its pauses would stretch a run by hours. The next failure
  that may pass then ends the run with its class, `retryable`, and a message
  that says Basecamp kept failing. Every request of a run goes through the
  one source that asks again (`Retrying`), the dry run included; the download
  of a file does not, see Files that cannot be fetched. `requestsRetried` in
  the report counts the requests that were asked again, each further attempt
  counted.

Its failures keep their class:

| Basecamp CLI | `code` | Exit |
|---|---|---|
| not installed | `config` | 1 |
| not found (2) | `not_found` | 2 |
| not signed in (3) | `auth_required` | 3 |
| forbidden (4) | `forbidden` | 4 |
| rate limit (5) | `api_error`, retryable | 7 |
| network (6) | `network`, retryable | 6 |
| API failure (7) that may pass | `api_error`, retryable | 7 |
| anything else | `api_error` | 7 |

The failure object of the Basecamp CLI is `{ok, error, code, retryable, hint,
meta: {request_id}}` and names no HTTP status. A failure of the API (exit 7,
`api_error`) may pass when the Basecamp CLI says `retryable: true`, which it
does for a gateway error (502, 503, 504), or when its message is, as a whole,
one of those the Basecamp CLI makes up around a status, `Request failed (HTTP
<status>)`, `Server error (<status>)` or `Gateway error (<status>)`, and the
status is 5xx, 408 or 429. Its own word is not enough: it calls a status it
does not know final, such as the 525 of a gateway that lost its connection to
Basecamp, and it does not try such a request again itself. A message Basecamp
sent instead names no status, and a text that merely quotes one proves
nothing; both stay final.

The message of a failure is what the Basecamp CLI says, as of every failure
of the API. Only a message of one of the fixed forms is known to hold a
status and nothing else. One that comes with `retryable: true` may be text
the server sent, which the Basecamp CLI cuts to 500 bytes, and minka passes
it on as it does for a failure that stays.

The exit code alone is no class: a program that crashes exits 2 as well. A
class counts only when the failure object `{ok: false, …}` stands on stdout
and its `code`, where it has one, says the same. Everything else is an
`api_error` that carries what the Basecamp CLI printed on stderr, cut to 500
bytes. Stdout is never quoted, only measured: it may hold the data of a
request that failed halfway, names and addresses included.

A bundle that cannot be written is `config`; an `--out` that holds something
else is `usage`.

## What a bundle cannot carry

The line runs between two things, and it is the same line in both directions
(C-050): **the run stops where going on would write something wrong, and leaves
the record out where a bundle cannot express it at all and its absence stays
nameable.**

Left out and counted, the run going on: a record the source describes with a
word a bundle has none for. A bundle has no field to say such a word in, so
the record is lost whatever the run does, and going on costs it and nothing
else. That is a kind of chat line a bundle does not know, a chat line whose
status is not `active`, because a message of a bundle has no status at all, a
kind of column a bundle has no role for, and a card, a post, a comment, a
question or an answer whose status is neither `active` nor `archived` nor
`trashed`, a draft among them, a to-do list of such a status, and a group or a
to-do whose status is neither `active` nor `trashed`, the shelf among them,
because a bundle gives neither of the two a shelf, a forwarded mail of a status
that is none of the three, an answer by mail that says anything but `active`,
because a bundle gives an answer no status at all, and a folder, a document or
an upload of Docs & Files whose status is none of the three. A shelf a group or
a to-do only inherits from its list or its group leaves nothing out, see Groups
under What a bundle holds so far (C-087).
`recordsLeftOut` names each
with the kind of record, the field, the word the source said there, the record
it stood in and how many were left out, counting records and not entries.
Nothing hangs on a record that is left out: nobody comes into the bundle for
it, and nothing of it is asked for.

**What stood in a place the bundle cannot carry falls with it and is asked for
so that it can be counted** (C-052). That is a to-do list or a group, and a
folder of Docs & Files or a whole Docs & Files tool: the folders, documents and
uploads below such a folder are counted in `docs.leftOut`, and
`docs.placesLeftOut` names every place with how much fell with it. A folder the
bundle leaves out is asked what stands in it, and a folder of that folder is a
place of its own, so the walk grows while it goes. A **tool that is switched
off** is asked nothing at all: a tool the source has switched off is one the run
does not read, as an inbox is, and what the project's own lists of records it
has put away hand out of it is counted where it is met.

**An upload whose file can never be had** is left out with its comments
and its boosts, named by its `sourceId` (C-063): an upload has no purpose
without its file, and no run will ever have bytes for what lies in a cloud,
for what the source no longer has, for what weighs more than Minka takes or
for a download Basecamp itself refuses. A download that failed for any other
reason is no such thing: the upload comes and waits for the run that brings the
file.
**A to-do that hangs on a To-dos tool itself** is left out although the source
says no word a bundle has none for (C-060): it stands in no list and no group,
and a bundle holds a to-do in a list alone. It is counted in `todosLeftOut` and
named in `todosWithoutAList` with its identifier and its project, which is what
somebody needs to find it at the source and move it into a list, after which a
later run carries it. Nothing of it is asked for and nothing hangs on it.

**Two things of a forwarded mail are neither** (C-056): what a mail calls
itself and whom it says it came from are nobody's to repair at the source, so a
subject Minka would refuse is **made into a title** — every character nobody can
see becomes a blank, what is left is trimmed, a mail without a subject is called
`(no subject)`, and what is longer than 200 characters is cut before the last
character and ends in an ellipsis. The run counts what it had to change
(`forwards.withoutASubject`, `forwards.subjectsChanged`) and a `notice` says so.
A sender that is no e-mail address Minka takes leaves the **mail** out, named by
its `sourceId` in `forwards.withoutAnAddress`, and nothing of it is asked for:
`fromAddress` takes nothing else and a bundle cannot say a mail without one.
The rule an address is held to is the same wherever one reaches a bundle, see
**One rule for every address** below. The title of a card, a post or a to-do
still ends the run, and so does that of a folder or a document: that one
somebody can repair where it stands.

The run ends as `api_error` where going on would put something wrong into the
bundle: a record without `created_at` or `updated_at` and a time that is none,
because the record would be placed wrongly among the others; a day of another
shape than `YYYY-MM-DD`; a time zone the run does not know, because the day
would be somebody else's; an address outside the account, because it is a
request the run may not make; a title Minka refuses, because the applier would
refuse it too, and for the same reason a start after the day a to-do is due; a
second Triage, Not now or Done on one board, because the cards of the second
would end up in a column nobody had; a boost that does not say
whose it is, because `createdBy` takes `null` and a reaction with an empty one
reads as the author of what it hangs on, which would lay it at the feet of
somebody who never gave it; and the status of a **project** the run was given by
name or found in a list of the account, because a run that was asked for that
project has to say that it cannot have it.

A moment is a date, a time of day, a fraction of a second where there is one,
and the zone it is meant in, with nothing around it: `2024-02-05T17:00:00.000Z`
and `2024-02-05T17:00:00+01:00`. Every moment the run reads is read where it is
read — `created_at` and `updated_at` of every kind of record, the moment a card,
a step or a to-do was completed, the moment a boost was given — and anything else ends
the run, named with the record and the field: a relative word such as `now` or
`tomorrow`, a time without a zone, a day written another way, and a moment of the
right shape that no calendar has, the thirtieth of February among them. The
applier draws the line in the same place, so neither side takes what the other
turns down: a line two programs draw differently is no line but the place where a
time changes its meaning without a word. A moment is read whether the bundle
computes with it or only carries it on, and the moment a card or a to-do was
completed therefore ends a run wherever Basecamp names one, not only for a card in
Done and not only for a to-do that is done.

The day a card is due is held to `YYYY-MM-DD` the same way, as the days a to-do
runs are and the day an answer belongs to is; a day of a **schedule** leaves its
question out instead, see Questions.

**Exactly one moment the run reads is forgiven:** the `created_at` of a tool that
is asked for nothing but the creator of a project. Such a tool is no record of a
bundle, nothing of it is written, and a moment that is none there says only that
the tool answers nothing about who made the project, as a tool without a creator
says nothing; the project then comes without a creator, see Projects. Every
moment of every record a bundle carries ends the run.

A message says what stood in the field only where it has the shape of a moment,
or of a day, and is at most 40 characters long (`maxTimeSaid`): the thirtieth of
February reads like a time and is none, and a reader who is not told the value
has nothing to go on. A value of any other shape is not repeated, because a
field that holds no time holds whatever the source put there, a name among it. A
longer one is not repeated either: the shape of a moment takes a fraction of a
second of any length, and a message that floods every log it reaches is a harm
of its own, however little it gives away. A day needs no such length, its shape
admitting ten characters and no other number.

A card, a post and a comment are held to all of this where the walk first meets
them, before anything that hangs on them is asked for, and the lines of a chat
before the first of them is written. A message of a failure names the record by
its ID and what is missing. Of what Basecamp said, the report and the message
quote a status, a kind of column, a kind of line and a time of the shape of one,
and never a title, a name, a text or an address.

## What the Basecamp identity needs

The run sees what the identity sees that the Basecamp CLI is signed in as. The
same holds for an unattended job, which needs such an identity of its own.

- It may see the projects named. One it may not see is `not_found`. Without
  `--project` the run takes what Basecamp lists for the identity, so a project
  it cannot see is missing from the bundle without a word.
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
- An address is read as Minka reads it, see **One rule for every address**
  below.
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
  as `api_error`, and the hint names both readings of that many: an identity
  that hides the addresses of others, or an account whose addresses are of a
  shape Minka takes none of.

A message counts the addresses concerned out of those given. No output ever
names an address or whose it is, and `invalidAddresses` holds IDs, never names.

## One rule for every address

Three fields of a bundle carry an e-mail address: the `email` of a person, the
`fromAddress` of a forwarded mail and the `fromAddress` of an answer by mail.
Minka reads all three through one value object, and an address it refuses stops
the import where it reads it, after a walk of hours. So all three are held to
one rule here, and that rule is Minka's (C-059):

- What Minka takes, this takes. The reading is PHP's `filter_var` with the
  plain local part of `EmailAddress` on top of it, measured against that reader
  rather than guessed: a local part of the ordinary characters in groups
  separated by single dots and never in quotation marks, of at most 64
  characters; a domain of at least two labels of letters, digits and hyphens,
  each at most 63 characters and beginning and ending with a letter or a digit,
  the last of them beginning with a letter; ASCII throughout; 254 characters in
  all. Whitespace around it comes off first, as Minka trims before it reads.
- Two shapes Minka takes are left out all the same, both of them nobody's
  mailbox: a domain written as a bracketed literal address of a machine,
  `[192.168.1.1]` or `[IPv6:…]`, and an address with a NUL at one end, which
  Minka trims away and this does not. Reading either exactly as PHP reads it is
  a risk in the direction that stops a run, and being narrower costs at most one
  address.
- One thing the rule does not do: Minka lowercases an address before it reads
  it, and a bundle carries the case the source gave. So a bundle can hold one
  mailbox in two spellings, which Minka makes one without a word.
- **One pair of quotation marks around the whole of an address comes off**
  before the address is read: `"ada@example.test"` becomes `ada@example.test`.
  Minka refuses every such address, a mail header is nobody's to repair where
  it stands, and the reading alone would turn away the addresses this saves.
  This is the **one place** a run repairs what the source says instead of
  passing it on, and it is that pair and nothing else: no other character is
  trimmed, unfolded or mended, and a mark at one end alone is no pair.
  `addressesUnquoted` counts the records it happened to, over people and mail
  alike — one mailbox written that way in many records counts in each of them —
  and a `notice` says how many. The one repair a run makes is a number in the
  report and never a silent change.
  A pair around a whole address is never a loss: an address Minka takes ends in
  a letter or a digit, and the quoted local part, `"ada b"@example.test`, keeps
  its closing mark before the `@` and is left as it stands.

An address the rule refuses costs its owner the login and the run nothing: a
person comes with `email: null` and is named in `invalidAddresses`, a mail is
left out and named in `forwards.withoutAnAddress`, an answer by mail whose
writer is from outside and has no usable address in
`forwards.repliesWithoutASender`.

## What a bundle holds so far

The structure, the boards, the message boards, the chats, the check-ins, the
to-dos, the forwarded mail and the Docs & Files: `manifest.json`,
`people.jsonl`, `projects.jsonl`, `tools.jsonl`,
`categories.jsonl`, `columns.jsonl`, `cards.jsonl`, `steps.jsonl`,
`posts.jsonl`, `questions.jsonl`, `answers.jsonl`, `todolists.jsonl`,
`todogroups.jsonl`, `todos.jsonl`, `forwards.jsonl`, `forwardreplies.jsonl`,
`folders.jsonl`, `documents.jsonl`, `uploads.jsonl`,
`messages.jsonl` or its
parts, `comments.jsonl`, `reactions.jsonl`, `attachments.jsonl` and `files/`
with the avatars and the files of texts, chats, answers, lists, to-dos, mails,
documents and uploads. A file of a kind the projects have nothing of is missing.

The walk goes kind by kind across the projects of a run (C-028): the projects
with their tools and boards, then the message boards of all projects, then
the chats, then the questionnaires, then the To-dos tools, then the
inboxes, and last the Docs & Files tools. So comments and
reactions stand in the order of what they hang on: those of cards, those of
posts, those of lines, those of answers, those of lists and of to-dos, those of
mails, those of documents and of uploads. Where a kind stands in that order
decides one thing and no more: which record of a file the bytes are fetched for,
so a file that hangs in a card and in a document alike is fetched for the card
and the document's record shares it (C-066). Every owner of a file has a record
of its own either way. Nothing a record points at gains
by being written earlier — the bundle is whole before Minka reads a line of it,
and Minka applies the kinds in an order of its own.

- **People** are the ones a record of the bundle refers to, and nobody else:
  the members of the projects of the run and whoever a record names: its creator,
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
  tools the run extracts are looked at first, in the order of their IDs,
  which is the order they were made in, so that the oldest of them decides
  where several were made with the project and name different people. Only
  when none of them answers are up to three others asked for
  (`maxCreatorProbes`), those that are switched off included, in the same
  order, and only until one answers; a refusal counts like an answer and uses
  one of the three up, so a tool behind it is never reached. Such a tool that
  is not there, that lies outside the account, or that the identity may not
  see, is passed over: one entry somebody may not open says nothing about the
  project, and the run takes what the rest gives. A refusal is counted
  (`creatorProbesRefused`) and a `notice` says so, because the project may
  come without a creator for it. An identity that may see nothing at all is
  refused where the people of the account, the project and its members are
  asked for, which ends the run as it always did. The creator comes into the
  bundle like anybody a record names.
  The description is plain text there and HTML in the bundle: escaped, a
  paragraph per block, a `<br>` per line break. `status` is the one the
  project itself answers with: `active`, `archived` or `trashed`.
  **A star is not carried** (C-088): whether somebody starred a project
  (`starred`, `star_url`) is nobody's in Minka, and the run never reads it.
- **Tools** are the dock entries that are switched on and that Minka has a
  counterpart for: `kanban_board` → `board`, `chat` → `chat`, `message_board`
  → `posts`, `questionnaire` → `checkins`, `todoset` → `todos`, `inbox` →
  `forwards`, several of a
  kind where a project has several. Creator and
  timestamps come from the tool itself. Every other entry is counted as
  skipped, per kind and whether it was switched on, and shown in the report.
- **Columns** are the lists of a card table: `Kanban::Triage` → `triage`,
  `Kanban::NotNowColumn` → `notnow`, `Kanban::Column` → `work`,
  `Kanban::DoneColumn` → `done`. They are written in the order of a board:
  Triage, Not now, the work columns by their position, Done. Basecamp gives
  the three columns every table has no position, so `position` is the number
  of the column in that order. The colour of a column is left out. A kind of
  list a bundle has no role for is left out and counted, see What a bundle
  cannot carry; it takes the cards standing in it with it, which are never asked
  for, and the archived and the trashed ones among them count as cards whose
  column is in no extracted board.
- **Cards** are those of every column, and the archived and trashed ones.
  Basecamp lists the cards of a column itself, and the archived and the
  trashed ones of a project in one list across all its card tables, those
  that are switched off included. Such a card names its column and no table,
  so it goes to the board its column is of; a card whose column is in no
  extracted board, because the board is switched off, the column is gone or the
  column is of a kind a bundle has no role for, is left out and counted
  (`cardsLeftOut`).
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
  completion Basecamp records, `dueOn` as given and held to the shape of a day,
  `assignees` in the order of their IDs. The note is the stored one (`content`), and `null` when empty.
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
  reference to a file or a person, a link, a rule and an embed say
  something: a line that was a video alone is a line, and one whose only
  frame is left out, see Texts, says nothing. A line comes as long as it is:
  nothing is shortened.
  A bundle has a word for four kinds of line: `Chat::Lines::Text`,
  `Chat::Lines::RichText`, `Chat::Lines::Upload` and
  `Chat::Lines::Integration`. Basecamp grows kinds of its own, and a line of
  one of them is left out and counted, as is one whose status is not `active`,
  see What a bundle cannot carry. The kind of a line is looked at before its
  status and before its times, so such a line is left out for its kind whatever
  else is wrong with it, and it is counted once. Whoever adds a word for a kind
  has seen what Basecamp sends under it.
  Nothing of this is final: the line stays where it is, and a later run brings
  it as soon as a bundle has a word for it.
  **A known limit of that:** where two lines carry the same second and one of
  them comes only with a later run, the order of the two can turn round. Minka
  puts a line at its `createdAt` plus the number of its line in the file, in
  microseconds, so among lines of one second the number in the new file decides,
  and the line that came late can land before one that stood after it in the
  chat. It shifts the order by less than a second, and Basecamp gives nothing
  finer.
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
- **Questions** are the questions of every questionnaire, oldest first, by
  `created_at` and then by ID. A questionnaire lists the questions it asks; a
  walk is no snapshot, so a question met twice is written once.
  The schedule of Basecamp becomes the one Minka has. The weekdays are Minka's:
  Basecamp counts Sunday as 0 and Saturday as 6, Minka Monday as 1 and Sunday as
  7, so every day but Sunday keeps its number and only Sunday moves; a day named
  twice is one day, and they come in order. `rhythm` is `weekly` where one
  weekday is chosen and `daily` where several are, because Minka's `daily` means
  "on each of the chosen weekdays" and the two are the same thing for one day.
  The frequencies Basecamp sends are `every_day`, `on_certain_days`,
  `every_week`, `every_other_week` and `every_month`. The first three all carry
  the days they are asked on, so they are the same thing and come to `weekly` or
  `daily` by the number of days; `every_other_week` becomes `everyOtherWeek` and
  `every_month` becomes `monthly`, both of which take exactly one weekday: what
  Basecamp asks on one weekday every month is the first such day of the month,
  which is what Minka's `monthly` means.
  `timeOfDay` is `HH:MM`.
  `status` is the one the source has, so a question it has shelved does not
  arrive active and start asking people again. `paused` is `true` for a question
  the source has paused and for one whose schedule has run out, because Minka's
  schedule has no end to say so with.
  `startsOn` is always written, never left out: Minka would otherwise take the
  day of `createdAt` in the offset the bundle wrote, which is Basecamp's UTC, so
  a question made late in the evening would start a day late. It is the
  `start_date` of the schedule; for an `everyOtherWeek` question it is the day
  the question was last answered on, stepped back to the weekday it is asked on,
  because Minka counts the weeks from the first matching day at or after
  `startsOn` and Basecamp says nothing about which of two weeks it asks in
  (C-046). `anchorsInferred` in the report names each such question with that
  day.
  `asked` comes from the subscribers of the question, which is whom Basecamp
  hands it to and not whoever answered it, and holds only those Minka can ask
  (C-045), in the order of their IDs and each once. A question that comes asking
  nobody at all is named in `nobodyAsked` with how many of its people fell out:
  it will ask nobody again, because the first run settles `asked`. Nothing hangs
  on a question: it carries no comment and no reaction, the format has no word
  for either of one, and none is asked for.
  A question whose schedule Minka has no words for is left out with its
  answers, and the run goes on (C-048); `questionsLeftOut` names it with the
  field that made it impossible: `week_instance`, `week_interval`,
  `month_interval`, `duration`, a `frequency` nobody knows, `days` that are
  empty, outside 0…6 or more than one where the rhythm takes one, an `end_date`
  that has not passed, a `start_date` or an `end_date` of another shape than
  `YYYY-MM-DD`, an `hour` or a `minute` of no day, and `group_on` for an
  `everyOtherWeek` question nobody ever answered. A title the applier refuses ends the run as `api_error`, named by
  ID: empty, longer than 200 characters, or with a character nobody can see. The
  whitespace around a title is taken off, as Minka takes it off.
- **Answers** are what somebody answered a question, per question oldest first,
  by `created_at` and then by ID; one met twice is written once. `groupOn` is
  the day the answer belongs to, taken from `group_on` as it is and never worked
  out from the schedule: Minka stores it and holds it against nothing, so an
  answer may well sit on a day the question never asked on, which is what the
  source says. Where Basecamp names no day, it is the day the answer was
  written, read in the zone of whoever wrote it (C-047), and counted in
  `answersWithoutADay`. The text is the rendered one, as of a comment, and goes
  the way every other text goes.
  An answer in the trash is left out and counted (`answersTrashed`), as a
  comment in the trash is, and one that says nothing is left out and counted
  (`answersWithoutABody`), because the applier refuses such a body. A question
  that has no answer costs no request. Comments, boosts and files hang on the
  answer.
- **To-do lists** are the lists of every To-dos tool, and the archived and
  the trashed ones. Basecamp lists the lists of a tool itself, and the
  archived and the trashed ones of a project in one list across all its To-dos
  tools, those that are switched off included; such a record names only what
  it hangs on, so every list of every tool is known before the first one is
  written, and a list of a tool that is not extracted is left out and counted.
  Minka puts every new list on top of its tool, so the lists that are **in use**
  stand in the file **from the bottom up**: the last line of a tool is the list on
  top. What has left the tool, the archived and the trashed ones, goes first and
  in the order the source lists it, where it cannot push the rest about; their
  order there decides nothing (C-065). No `position` is written for a list, a
  group or a to-do: Minka reads none, and a `todogroups` record that carried
  one would earn a warning. A list comes with its groups and its to-dos
  whatever its status, as a card comes with its steps. `status` is `active`,
  `archived` or `trashed`; any other word leaves the list out, see What a
  bundle cannot carry. The description is the rendered text and goes the way
  every other text goes; a list without one comes with `description` `null`.
  Comments and boosts hang on a list as they hang on a card. **A hill chart is
  not carried** (C-088): Minka has none, so the needle of a list
  (`last_needle_*`, `previous_needle_position`) is never read.
- **Groups** are the named headings of a list. Basecamp makes a group a
  to-do list of its own, and what a record hangs on is what tells the two
  apart. A group goes to the end of its list, so the order of the file is the
  order of the list: the groups in use as Basecamp lists them, then the ones
  it has thrown away. The groups of a list on the shelf or in the trash come in
  the order the list gives them too; the records of the project only stand in
  for one the list does not name, after the ones it does. `status` is `active`
  or `trashed`, because the shelf is the list's alone, so a group the source
  has put on the shelf is left out and counted. **What stands in a list or a
  group on the shelf or in the trash comes in use** (C-087): Basecamp puts
  every group and to-do of a list on the shelf with it and says so with
  `inherits_status`; for the trash that is not seen live yet, see the Status.
  In Minka the list or the group alone holds the shelf and the trash, and
  restoring it restores what stands in it, so a status marked inherited is
  written `active`. A status of its own stays: a trashed one comes `trashed`,
  an archived one is left out. The run trusts `inherits_status`; what Basecamp
  says of a status of its own under a list or a group that has one is not seen
  live. Should it say inherited there, a group or a to-do trashed of its own
  before its list went on the shelf or into the trash comes back in use in
  Minka. A group of a list that is not extracted is left out and counted as
  well. **Nothing hangs on a group:** the format has no word for a comment, a
  reaction or a file of one, such a record would wait in Minka for ever, so
  none is written and none is asked for. **A group carries its colour** in
  `color`, Basecamp's name as it is, never checked, lowercased or mapped:
  Minka knows Basecamp's eleven names and warns about any other (C-086).
  `white` goes along too, since Basecamp says it of every group nobody
  coloured and it is the one word that takes a colour away in Minka on a later
  run. Where the source names no colour, `null` or nothing, the field is left
  out, and Minka leaves the colour it holds alone. A list carries none:
  Basecamp says `null` for every list.
- **To-dos** are those of every list and of every group, and the archived
  and the trashed ones. Basecamp lists what is open apart from what is done,
  and the archived and the trashed ones of a project in one list across all
  its To-dos tools; one that stands in neither a list nor a group of the
  bundle is left out and counted. A list or a group the bundle does not carry
  takes what stood in it with it, and what fell is asked for and counted all
  the same (C-052): `todosLeftOut` counts the lists, the groups and the to-dos
  a bundle does not carry, each kind on its own, and `todoPlacesLeftOut` names
  every list and group that took records with it, with how many groups and
  to-dos those were. Such a request is made for the counting alone, so a place
  Basecamp no longer has takes nothing with it and does not end the run; every
  other failure of it ends the run as everywhere.
  **A to-do can hang on a To-dos tool itself** and stand in no list and no group
  of it. A bundle holds a to-do in a list alone, so such a to-do is left out,
  counted in `todosLeftOut` and named in `todosWithoutAList` with its identifier
  and its project (C-060); the walk of the lists never meets it, so the to-dos
  of every tool that is switched on are asked for under the tool's own
  identifier, and a failure of that request ends the run. A to-do goes to the
  end of its list or its group, so the order of the file is the order of the list:
  what is open as Basecamp lists it, then what is done, then what it has
  thrown away. `groupSourceId` names a group of the to-do's own list and is
  `null` for a to-do standing in the list itself; both it and a `listSourceId`
  that named anything else would be disguised in Minka as "the list is not in
  use here". `status` is `active` or `trashed`, as a group's is, and a to-do
  whose status is inherited from its list or its group comes `active`, as a
  group does. The notes are the rendered text, `assignees` and
  `notifyWhenDone` are the people the source names, in the order of their
  IDs and each once, `completedAt` and `completedBy` come from the
  completion Basecamp records, and `startsOn` and `dueOn` as given, both held
  to the shape of a day. Comments, boosts and files hang on a to-do.
  **`updatedAt` is never older than `completedAt`** (C-051). **Subtasks are
  not carried:** the format has no word for them, so all that is left of
  them is how many there were, counted in `todosWithSubtasks` and
  `subtasksLeftOut` and asked for never.
  **A schedule is not carried either:** a to-do that comes again at the source
  comes here as an ordinary to-do, because Minka has no repeating one, and
  `todosThatRepeat` counts the ones a bundle carries. Basecamp leaves the key of
  a schedule out altogether where nothing repeats, so that the key is there is
  the whole test, and nothing in it is read.
- **Forwards** are the mails somebody forwarded into a project, one record per
  mail of every inbox that is switched on. Basecamp lists the mails of an
  inbox itself and hands out **no** list of the archived and the trashed ones
  of a project: asked for one it answers with a refusal of the request, so a
  bundle holds what an inbox shows and nothing else. The mails of an inbox are
  written oldest first, by `created_at` and then by ID, the inboxes in the
  order of the tools; a walk is no snapshot here either, and a mail a list that
  moved hands out twice is written once.
  `title` is the subject, `createdBy` whoever forwarded it and `createdAt` the
  moment they did. `fromAddress` is the sender, who **stays an address**:
  nobody comes into the bundle for a sender, and `fromName` is always `null`,
  because Basecamp gives the address alone. A subject Minka would refuse is
  made into a title rather than costing the mail or the run, and a sender that
  is no address leaves the mail out (C-056), see What a bundle cannot carry.
  A mail Basecamp names nobody for comes with `createdBy` `null`, which Minka
  gives to the owner of the organization without a word; the run counts them
  (`forwards.withoutAForwarder`) and a `notice` says so.
  A record of Basecamp that says nothing of what became of it is in use, as the
  format reads an absent `status`: Basecamp names one on a mail and on an answer
  alike, and reading silence as a word nobody knows would lose every record of
  an account the day that changed.
  `status` is `active` or `trashed`. **What the source keeps on its shelf
  becomes the trash** (C-058), counted in `forwards.fromTheShelf` and named in
  a `notice`: Minka gives a forward no shelf, its trash holds the mail with
  everything on it, and a bundle that said `archived` would stop the run.
  The body is the mail as the source holds it, **line breaks and all**: Minka
  turns a blank line into a paragraph and a single break into a `<br>`, so the
  extractor rewrites nothing but the files, the mentions, the references and
  the embeds, the way it does in every text. Where the body would reach Minka
  as nothing at all, the run writes one line of its own behind it (C-057), see
  Texts.
  Comments on a mail are `comments` with `parentKind` `forward` and its boosts
  are `reactions` with `targetKind` `forward`. A mail says **how many of
  neither**, so both lists are asked for of every one of them: a count of zero
  there is the field missing and not a mail nobody answered. Its answers it does
  count, and they are asked for where it says it has some **and where it says
  nothing about them at all**, so that a field the source drops does not take
  every answer of an account with it. Its files are
  `attachments` with `ownerKind` `forward`.
- **Answers by mail** are the replies to a forward, `forwardreplies`, a record
  of its own and no comment, oldest first per mail: Minka reads them back by
  their moment and lets the order of the file decide between two of one second.
  They are asked for only where a mail says it has some.
  `createdBy` is somebody of the account, and then `fromAddress` is `null`; for
  an answer from outside it is the other way round, because somebody from
  outside never becomes a person — they would be somebody of the organization.
  **Never both:** Minka reads the person and drops the address without a word.
  Basecamp gives such a writer the kind `Outsider` and their address as their
  name as well, so the name stands in where the field for an address is empty.
  An answer the source names neither somebody nor a usable address for is left
  out and named by ID (`forwards.repliesWithoutASender`), because Minka stops
  the run at one that names neither.
  An answer says what became of it as a mail does: one that says `active`, or
  says nothing, comes, and any other word leaves it out and is counted, see What
  a bundle cannot carry. The body is the rendered text, and where it would
  reach Minka as nothing it carries a line of the run as a mail's does.
  A mail or an answer whose body is longer than the 1,000,000 characters Minka
  holds is counted (`forwards.longerThanMinkaTakes`) and named in a `notice`,
  and the body comes as it is: the source has no such limit, a run that
  shortened it would decide in the one place nobody can repeat, and Minka passes
  such a record over on every run with everything hanging on it. What is counted
  is the body as the run writes it, which is shorter than what Minka stores, so
  the number is what a run can see and no promise. `longestBodyRecords` names
  the longest mail and the longest answer of the run.
  **Nothing hangs on an answer:** it has no comments here, and its boosts are
  counted and never written (`forwards.reactionsOnRepliesLeftOut`), because
  Minka passes a reaction on one over — nor is the list of them asked for. They
  are counted of every answer the source shows, one that is left out included:
  a bundle holds none of them either way. Its
  files are `attachments` with `ownerKind` `forwardreply`; in Minka they belong
  to the mail.
- **Folders** are the folders of every Docs & Files tool that is switched on,
  `folders.jsonl`. Basecamp calls a folder a vault and makes the tool itself the
  vault at the top, so the dock entry is the tool and the folders standing
  directly in it carry `parentSourceId` `null`. Basecamp lists what is in use in
  a vault, folder by folder, and the archived and the trashed records of a
  project in one list per kind across all its Docs & Files tools, those that are
  switched off included. Such a record names the vault it stands in and nothing
  else, so every folder the bundle carries is known before the first entry of a
  tool is written, and an entry naming anything else falls with the place it
  stood in, see What a bundle cannot carry. A walk is no snapshot here either: a
  folder a list that moved hands out twice is written once.
  A folder **stands after the folder it is in** — the applier stops at one whose
  parent it has not seen. For an entry **in use** the order of the file decides:
  it stands **from the bottom up**, because Minka puts every new one on top of its
  kind, so the order the source shows comes out again. For one that has **left its
  place** it decides nothing (C-065): those come first, in the order the source
  lists them, and Minka reads that order nowhere, ordering an archive by the moment
  a record was put away and taking that moment from the record itself. There is no
  `position` in the format, and a record that carried one would earn a warning.
  `status` is `active`, `archived` or `trashed` and is the folder's own;
  `docs.foldersByStatus` splits the folders of the bundle, as
  `docs.documentsByStatus` and `docs.uploadsByStatus` do the other two kinds.
  **A folder takes neither comments nor reactions**, here as in Minka and as in
  Basecamp, so neither is written and neither is asked for: Minka counts one that
  names a folder with what it will never take, and no later run takes it either.
  A folder the source has put away may hand out no list of what stands in it,
  and that is passed over rather than ending the run; a folder that is in use
  has to answer, and a list of one that is not there ends the run. What is in
  use below a folder that would not answer is then beyond the reach of any run
  and nothing can count it, so `docs.foldersThatSaidNothingOfWhatIsInThem`
  counts the folders and a `notice` says so; what the source has put away comes
  from the project's own lists either way, and the folder itself comes.
- **Documents** are the documents of those tools, `documents.jsonl`, with
  `folderSourceId` `null` at the top of the tool. `title` is the title, `body`
  the document as Basecamp renders it, rewritten like every other text, and
  `null` where the document has no words, which Minka takes and
  `docs.documentsWithoutWords` counts. `status` is the document's **own**: one
  in a folder the source archived or threw away carries
  `active` and goes along through the folder in Minka, and a run that wrote the
  folder's word onto it would put it away one by one and never bring it back
  with the folder. Comments on a document are `comments` with `parentKind`
  `document`, its boosts `reactions` with `targetKind` `document`, its files
  `attachments` with `ownerKind` `document`.
- **Uploads** are one stored file each, standing beside a text instead of inside
  one, `uploads.jsonl`. `title` is **what the source calls the upload**:
  Basecamp fills it from the name of the file and anybody can rename it, so the
  title is the one to take. Where the source calls it nothing, **the name of its
  file stands in** (`docs.titlesFromTheFileName`), because Minka derives nothing
  from a file and refuses an upload without a title; where there is neither, the
  title is `(no name)` (`docs.withoutAName`). A title Minka would refuse is made
  into one it takes (C-056) and counted in `docs.namesMadeIntoATitle`: every
  character nobody can see becomes a blank, and a title too long is shortened
  **the way Minka shortens the name of a file** for an upload somebody makes
  there (`ItemTitle::fromFilename`) — the extension stays, the stem before it is
  cut to make room, and nothing ends in an ellipsis, so one file carries the same
  title whether it was uploaded there or brought here.
  `file` is the `sourceId` of the file's record in `attachments.jsonl`, which is `upload-<id>`
  and never the reference a text uses for a file: a stored file has one place in
  Minka, so a text that embedded the same file and an upload that stood for it
  would be one reference used twice, the text would claim it, and the upload
  would stay out. **The bytes of that file are asked for by the identifier of the
  upload and of its project**, and at no address: Basecamp names no stored file
  on such a record, and the address it does give answers with a redirect to a
  store of its own, which no run follows — see Files. `description` is the rich
  text beside the file, or `null`, and
  `status` is its own as a document's is. Only the file the source shows now
  comes; the format keeps `versions` free for the earlier copies and reads it
  nowhere, so a bundle writes none, and the upload says neither how many there
  are nor which. An upload whose file can never be had is left out with
  everything pointing at it (C-063): one Basecamp names no stored file for,
  named in `docs.withoutAStoredFile`, and one whose file the source no longer
  has, weighs more than Minka takes or was refused by Basecamp itself, named in
  `docs.onlyAFileNoRunWillBring`; what fell with either is
  counted in `docs.commentsWithoutTheirUpload` and
  `docs.boostsWithoutTheirUpload`. A file that might still come leaves the upload
  standing, whether a later run may bring it or the failure is one this run could
  not place, see Files that cannot be fetched. **What says so is the record and never the
  shape of the address:** an upload the source gives an address to fetch a file
  from has a file, and nothing looks at what that address is made of, because
  nothing asks for it. Reading it for the key of a stored file, as a file inside a
  text is read, leaves every file of a tool out of a bundle, counted and named and
  gone. Comments, boosts and the files of the
  description use `upload` as their kind.
- **A document or a description longer than the 1,000,000 characters Minka
  holds** is counted (`docs.longerThanMinkaTakes`) and comes as it is, as a mail
  does: Minka passes such a record over on every run with everything hanging on
  it, and a run that shortened it would decide in the one place nobody can
  repeat. A record that lost **a file no run will ever bring** is named by its
  `sourceId` (`docs.onlyAFileNoRunWillBring`), and the two kinds it can be end
  differently, because the one field is optional and the other is not:
  - A **document**, or the description of an upload, whose whole text was that
    file **arrives without words**: the tag comes out, so nothing is left, and
    the record is written empty with its title, its comments and its boosts. The
    tag left in would cost the whole record — Minka keeps nothing of a text that
    is nothing but a file it has not got and asks for the file again on every
    run — and the premise of the case is that the file stays gone. It gets **no
    line of the run's own** as a forwarded mail does (C-057): there the body is a
    field that must not be empty, so an invented sentence was the only way the
    mail came at all, and here it is not; a document is read as something a
    person wrote.
  - An **upload** is nothing without its file and is **left out** with its
    comments and its boosts (C-063), which are neither asked for nor written,
    and the files of its description are not fetched either. `file` is required
    and takes the identifier of a file of the bundle and nothing else; written,
    such an upload would wait in Minka on every run and say nothing, and an
    honest gap in the report is worth more than that. Where the file left a
    record behind — one the source no longer has, one above what Minka takes —
    that record stays as the trace of what became of it, so nothing dangles and
    nothing names it. **Such an upload is in no other number of the run**: its
    title, its description and the files of that description are not read at all,
    so nothing of it is counted among the titles taken from a file name, the
    captions, the owners of a file beyond the first or the longest text of its
    kind. What is counted is what fell with it, and that it fell.

  A file that **might still come** is not one that will never come: the tag
  stays, the upload is written and waits for its file, and a later run writes
  both whole. What tells the two apart is whose word the failure is — `why` of
  the entry in `missingFiles`, and nothing else. Only Basecamp's own refusal
  leaves the upload out; a failure that may pass and one this run could not place
  both leave it standing, and the second fails the run so that nobody takes the
  bundle for whole. A refusal read as a failure we do not understand costs an
  exit code; a failure we do not understand read as a refusal throws away
  whatever it touched, which over one wrong reading of the source is an account.

  A **dry run** cannot know a refusal: nothing the source answers says that a
  file is gone or that a download will be refused, only the download itself does.
  Such a run counts an upload a real run leaves out, which is the way round that
  promises nothing. What it does see is what a guard of the run itself refuses,
  and it says so before anybody spends an hour.
- **Comments** are those of cards, of posts, of answers, of to-do lists, of
  to-dos, of forwarded mail, of documents and of uploads, `parentKind`
  `card`, `post`, `answer`, `todolist`, `todo`, `forward`, `document` or
  `upload`, per parent oldest first, by `created_at` and then by ID: the applier reads
  the order of the comments of a parent from the file. Basecamp hands out the
  rendered text of a comment only. A comment in the trash is left out with
  its boosts and counted (`commentsTrashed`); any other comes, the archived
  comment of an archived card as well.
- **Subscribers** are whom the source says follow a post, a card, a to-do, a
  document or an upload (C-084). They come from
  `buckets/{bucket}/recordings/{id}/subscription.json`, one request per record,
  in a dry run as well. Measured read-only: it answers every one of the five
  kinds and a question alike, with `count`, `subscribed`, `subscribers` and
  `url`, `count` the length of the list, and no pages; a recording or a project
  that is not there or not visible answers 404, and no 403 has been seen. A cap
  above seven people has not been measured, which the check against `count`
  holds. They stand in `subscribers` as person `sourceId`s, in the
  order of their IDs and each once. Minka takes a list that is there as the
  whole truth: exactly those people follow the item once the run is through,
  and everybody else is taken off it — the writer, the people it is handed to,
  its commenters and the people it mentions included. So `[]` is written where
  the source names nobody, and means nobody follows the item.

  Only somebody Minka can put on an item is named: a member of the project who
  is no client, is still there and has an address, the same four reasons a
  question leaves somebody out for. The rest is counted in
  `subscribers.leftOff` under the first reason that holds, and not named per
  record: nothing can be done about them at the source, and whoever reads the
  report needs the size, as for the people a question asks. The filter stands
  here and not only in Minka, so that the bundle says what the import will do.
  Every member of a
  project comes into the bundle where the project is written, so a list never
  names somebody `people.jsonl` does not carry, and nobody comes into the
  bundle for following an item.

  A record in the **trash** is not asked and carries no list: Minka changes
  nothing on an item in the trash. That rests on the status the run has
  already read of the record: Basecamp answers the list of a trashed recording
  like any other. A to-do in the trash with its list or its group counts as
  one in the trash here, though it comes `active` (C-087): it goes back with
  its list or group in Minka and keeps whom it had there. One on the shelf with
  its list is asked like any other. A record in the archive is asked like any
  other, because Minka applies the list before it archives. Where Basecamp
  answers the list as **not found or forbidden**, the record comes without the
  field, and Minka follows the item the way it did before the field existed.
  `subscribers.unread` names each such record with `record`, `sourceId`,
  `projectSourceId` and `why` (`notFound` or `forbidden`, what Basecamp
  answered), a `notice` counts them, and the run goes on. An empty list in
  their place would take everybody off the item, which is the one mistake this
  field can make without anybody seeing it. This decides nothing about the exit
  code: without the list Minka loses nothing it would otherwise have had, and a
  later run brings the list once Basecamp hands it out (C-069 speaks of files
  alone). An answer that is not plainly the whole list ends the run as
  `api_error`, and so does every other failure as it does anywhere: an answer
  without `subscribers` or with `null`, an entry that is `null` or has no ID
  above 0, and a list that holds fewer or more entries than the `count` beside
  it, where there is one. Read as a list, each would name nobody and take
  everybody off the item. Somebody named twice is taken once. The same holds
  for the people a question asks, which come from the same answer.
  `subscribers.lists` counts the records that carry a list. A **dry run** asks
  the list of an upload whose file a real run then finds it can never have and
  leaves out with the upload, so its `subscribers.lists` can be higher than a
  real run's.
- **Reactions** are the boosts of a card (`targetKind` `card`), of a post
  (`post`), of an answer (`answer`), of a to-do list (`todolist`), of a to-do
  (`todo`), of a forwarded mail (`forward`), of a document (`document`), of an
  upload (`upload`), of a comment (`comment`) and of a line (`message`), with
  `content` as it is, a text of some words included: what Minka takes for a reaction is
  the applier's to decide. The boosts of a card or a post stand before those
  of its comments. A boost has one time, which is both `createdAt` and
  `updatedAt`.

Left out besides: the colours of the columns, wormholes between boards, whom
a post told, being on hold as anything more than the flag, and the earlier
copies of the file of an upload.

What Basecamp does not hand out, so that no bundle carries it:

- Whether a message is pinned, and since when. `pinnedAt` is `null` on every
  post.
- A draft of a message. It is in no list; a status that says so would end the
  run.
- A line somebody deleted, and the lines Basecamp writes itself, such as who
  joined a chat.
- Whom a forwarded mail was addressed to, who was copied on it, and when it was
  sent: Basecamp keeps the sender, the subject, the words and the moment
  somebody forwarded it, and nothing else of the mail. The secret address an
  inbox receives at is not read either.

IDs are the numeric IDs of Basecamp as strings; `sourceInstance` is the ID of
the account, taken from the address of a project. Times are written the way
Basecamp writes them, milliseconds included. People, projects, tools and
avatars are ordered by numeric ID, steps, comments and reactions by their
parents in the order of their file and then by position or age, whatever
order Basecamp answers in: two runs over an unchanged account write the same
bytes, apart from `extractedAt`, whether a run fetched its files or took
them over. `attachments.jsonl` begins with the avatars and goes on with the
files in the order the walk meets them, see Files. Only where Basecamp gives
cards no position, in Done and among the archived and the trashed, does its
own order count.

A card, a post, a line, a forwarded mail and an entry of Docs & Files cost no
request of their own: a list shows them in
full. A Docs & Files tool costs three requests per vault it holds — its folders,
its documents and its uploads, the vault at the top of the tool included — and a
project with such a tool six more for the records it has put away, one per kind
and status, whatever the tool. A folder the bundle **cannot** carry costs three
more, for counting what fell with it, and a folder of that folder three again.
Comments are asked for only where a card, a post, a to-do list, a to-do, an
answer, a document or an upload says it has some, and boosts only where one of
those, a comment or a line says so; the answers
of a mail are asked for only where it says it has some. A **mail** says how many comments and
how many boosts it has as little as Basecamp knows, so both of its lists are
asked for whatever they hold: two requests a mail, and one more where it has
answers. The requests
go one after the other, never two at a time: the Basecamp CLI keeps to the
rate Basecamp allows.

### Texts

A bundle never carries the embeds of its source, and writes four tags of its
own in their place: `<import-mention>`, `<import-attachment>`,
`<import-embed>` and `<import-reference>`. In the note of a card, the body of
a post, of a comment, of a line, of a forwarded mail and of an answer by mail,
every `<bc-attachment>` is replaced as a
whole, with the figure Basecamp rendered into it:

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
  The reference carries `caption` when the embed has one, and
  `presentation="gallery"` when the embed says exactly that, in this order,
  escaped as attribute values, and nothing else. A caption of more than 1000
  characters (`maxCaptionLength`) is left off and counted
  (`captionsDropped`), and the file comes all the same. A stored note that
  says neither takes both from the rendered one.
  The listed file is found by the signed ID of the embed, then by the key of
  the stored file, which stands in the address of the embed
  (`…/blobs/<key>/download/…` in `href`, else `url`), then by the ID of the
  attachment the signed ID names. An older text carries the signed ID of its
  day and the list the one of today, so the first alone would lose such
  files. What an embed names counts only when a listed file confirms it: the
  listed file is what says where the file is fetched from. An embedded file
  that is not listed cannot be named: its caption stays as text, for the
  stored note of a card the one of the rendered note, the rest is dropped, it
  is counted (`filesDropped`) and a `notice` says so. It has no reference,
  and so no record and no file.
- A horizontal rule becomes `<hr>`.
- The preview of a link (`application/vnd.basecamp.opengraph-embed`) becomes
  the link, `<a href="…">title</a>`, with the address as the text where the
  preview has no title (`linkPreviews`). The address is the `href` of the
  embed, else its `url`. Only an absolute address over http or https becomes
  a link: of any other, the title stays as plain text, or nothing, and it is
  not counted. A link to a card, a post or a chat of the account becomes a
  reference, see below, and counts as a link all the same.
- A picture at a foreign address, an embed of the kind `image` without a
  signed ID, is no stored file. It becomes a link to its `url` with its
  `caption` as the text, by the rules of the preview of a link, a reference
  included, and never a
  picture: a bundle fetches nothing from an address somebody wrote into a
  text (`picturesLinked`, which counts what became a link). A picture with a
  signed ID, and one of a kind such as `image/png`, is a file as above.
- An embed of any other kind that carries no signed ID stays as it is, with
  all it holds but a frame.

What the source embeds from elsewhere, a video, a track, a player, stands in
a text as an `<iframe>`, mostly in a `<figure>` of its own. It becomes
`<import-embed url="<address>"></import-embed>` in the place of the frame and
of all the frame holds (`embeds`). The tag carries the `src` of the frame and
nothing else: the address the `src` stands for, read out of the HTML once and
escaped as an attribute value once. The whitespace a browser takes away
before and behind an address, blank, tab, line feed, form feed and carriage
return, goes before anything is asked of the address, because Minka takes
none away, and the tag carries the address without it; a `src` of nothing
else is no `src`. Nothing else of the address changes. The extractor tells no
provider from another and asks nobody about the address.

- A `<figure>` that holds a frame is not written, neither its start nor its
  end tag. What else stands in it stays where it stands, as the source wrote
  it, whitespace included. Its `<figcaption>` becomes a paragraph in its
  place, `<p>`, what the caption holds and `</p>`, which is after the tag
  wherever the source puts the caption after the frame; a caption that holds
  nothing but whitespace becomes nothing. Whatever the figure, the caption
  and the frame carry as attributes goes. A block around the figure stays.
- A figure that holds no frame, such as the one around a table, passes byte
  for byte.
- Every frame of a text becomes a tag, in the order of the text, in a figure
  or outside of one.
- Only an address Minka takes becomes a tag, and `embeddable` is the whole
  rule, asked of the address without the whitespace around it: it begins
  with `http://` or `https://` in whatever case, names a host, holds no
  character from U+0000 to U+0020, no U+007F and no backslash, and is at most
  2,000 characters long, counted as code points. No other character is
  refused. The host is what follows the scheme up to the first `/`, `?` or
  `#`, without what stands before the last `@` and without a port; a colon
  no port follows is no port, and a port is one to five digits and at most
  65535. About a port the rule is stricter than Minka, which reads the number
  a port begins with and lets the rest be: no tag is written that could be
  refused, at the price of an embed whose port ends in something else. A
  frame with any other address, relative ones, `//host/…`, `data:`, `about:`
  and `javascript:` included, and a frame without a `src`, is left out with
  all it holds and counted (`embedsLeftOut`), and a `notice` says so. Its
  figure goes the same way, so one that held nothing else leaves nothing, and
  a caption stays as its paragraph.
- A frame that is never closed holds the rest of the text for a reader of
  tokens, as it does for a browser. The rest is read anew as the text it is,
  so such a frame costs no more than itself, and once, however many frames
  follow: nothing closes a frame behind one that is never closed. A text is
  read in a time that grows with its length, whatever it is made of.
- A frame Basecamp rendered into a `<bc-attachment>` that is replaced goes
  with that rendering and is not counted. In an embed of another kind, which
  passes through, a frame becomes a tag or is left out like any other: no
  `<iframe>` reaches a body, and such an embed may never end.
- No output names an address.

The run reads a mention and a reference as something the body says, although
Minka leaves either as the plain words it was given where it finds no target.
A run writes words into both, so a body of one of them alone does say something
there as well.

**A mail whose body would reach Minka as nothing at all carries one line of
the run** (C-057), written behind the body and counted in
`forwards.givenALineOfTheRun`, with a `notice`. That happens where the mail says
nothing, where what it says is a file that will **never** come — one the source
no longer has, one above the gibibyte a run of Minka takes, one whose download
Basecamp refuses — where the body is
nothing but an embed, and where the body
holds nothing but what Minka takes off the edges of a text, a NUL among them,
which Go does not. A file that might still come is none of those, a failure this
run could not place as little as one that may pass: it
leaves no record at all, the run says so and ends non-zero, and a later run
brings the file, so the mail waits with it rather than being told that its file
is lost — which no later run could take back. Minka writes
such a body empty and remembers the mail as finished, or passes it over before it
is remembered and passes it over again on every run afterwards, and either way
the subject, the sender, the answers and the comments are lost with it; the
extractor is the one place that knows a file will never come. The tag of a file
that will never come stays where it stood, and Minka drops it.
**An embed gets the address and no sentence.** It counts for nothing here
although Minka usually takes one, because whether it does is its own reading of
the address; so the address is written beside it as a link, which every reading
keeps, and nothing is said about the mail — a run that called such a mail
wordless would be putting that in writing wherever Minka took the embed after
all. A dry run knows no more than what Basecamp says of a file's size,
so it writes the line only where the size alone gives it away.

A link to a card, a message or a chat of the account, and to a line of a
chat, becomes a reference to that record (C-031):

```
<import-reference kind="cards|posts|tools" ref="<sourceId>" href="<address>">words</import-reference>
```

- `kind` is the kind of record in a bundle: `cards` for a card, `posts` for a
  message, `tools` for a chat. A line of a chat is a reference to its chat.
  `ref` is the source ID of that record, which is the ID of Basecamp in the
  address, without the zeros it may begin with: the ID of a card, of a
  message, and of a chat, which is also the ID of the chat's tool. `href` is
  the address as it stood, escaped as an attribute value once, an anchor
  included. Nothing else of the link comes along.
- The words of the link stand between the tags as plain text, escaped once:
  the text of the link without any tag, a blank for a line break. An embed
  in them gives its words and refers to nothing: a mention the name the
  list of the account has, the one a person of the bundle carries, and the
  name Basecamp rendered only for somebody the list does not know; a preview
  its title, a picture or a file its caption, a rule nothing, a frame
  nothing. Nobody and no file comes into the bundle for it. A link without
  words gives an empty reference.
- Every such link is marked, whatever its words, and whether the record is in
  the bundle or not: the applier resolves what it knows and makes the link
  anew of what it does not, from `href` and the words.
- The address is asked without the whitespace around it. It begins with
  `http://` or `https://` in any case, its host is `3.basecamp.com` or
  `app.basecamp.com` in any case, without a port and without a name before an
  `@`, and its path, without query and fragment, is
  `/<account>/buckets/<n>/card_tables/cards/<id>`,
  `/<account>/buckets/<n>/messages/<id>` or
  `/<account>/buckets/<n>/chats/<id>`, where the account is the one of the
  run and every `<…>` is digits. A chat may be followed by `@<line>`, and each
  of them by `/` and anything, such as `/lines/<line>` or the menu of a line.
  The path is asked with its escapes read and its `.` and `..` segments
  resolved, so a path that climbs out of a page shows what it climbs to. A
  path that begins with `//` shows nothing: Basecamp writes none. An ID of
  more digits than a number holds is none.
- Any other link stays as it stands: a to-do, a document, a file, a card
  table, a message board, a project, another account, another host, a stored
  file, and a link without an address.
- A link ends where its end tag stands, where the next link begins, as it
  does for a browser, where a block such as a paragraph, a list item or a
  figure begins or ends, before the end tag of an element that began before
  the link, and at the end of the text, where the reference is closed. The
  block and the end tag stay as they were written, so an element ends where
  it did; an end tag without a link passes as it stands. An element that
  begins within the words gives its words, and the end of an element that has
  none, such as `</br>`, ends nothing.
- The references are counted by kind (`references`: `cards`, `posts`,
  `tools`).

A line of a chat that is a file somebody dropped has no text. Its body is one
`<import-attachment ref="blob-<key>"></import-attachment>` per file, in the
order of the line, without a caption, and counts as `filesInBodies`. Basecamp
gives such a file no ID, so the key of the stored file names it, which stands
in the address it is downloaded from (`…/blobs/<key>/download/…`), else in
the one it is previewed at (`…/blobs/<key>/previews/…`). A key is made of
letters, digits, `-` and `_`, because it becomes part of the ID of an
attachment; one that has not the shape of a UUID besides keeps its reference
and is a file no run can ask for, see Files. A file
without such a key is dropped and counted (`filesDropped`); a line that keeps
no file is left out with its boosts and counted (`uploadLinesLeftOut`), and
nobody comes into the bundle for it. A plain line and the line of a bot go
the way of every text and, holding no embed and no link to a record, pass
byte for byte.

The kind of an embed is its content type without parameters, in any case. A
signed ID is base64, `--` and a signature, which stands behind the last `--`
and is not checked. Today the base64 holds an envelope Ruby serialised, with
the global ID (`gid://bc3/Person/<id>?expires_in`) readable in it; an older
one holds JSON, `{"_rails": {"message", "exp", "pur"}}`, whose `message` is
the serialised ID as base64 again, in the usual alphabet or the one for
addresses, with or without padding. A global ID counts as a whole: nothing of
a word before it, nothing of a number or a path after it.

Every other token passes through with the bytes it was read from: Minka
sanitises a body, and the extractor adds no second opinion. A text is read
only when it holds `bc-attachment` or `<iframe`, or `<a` followed by
whitespace, `>` or `/`, all in any case; any other text passes as it is.
What goes besides an embed is what belongs to one: whitespace between an
embed and its rendering, an end tag of an embed without its beginning, and
an embed or a frame the text breaks off in. Any other tag a text breaks off
in stays as it was written, at the end, so a text loses no byte to being
read. A text is read token by token, never parsed into a tree,
because a tree moves a figure out of a paragraph; an embed that is never
closed costs no more of the text than its figure, and one that closes itself
nothing.

### Files

Every `<import-attachment>` the extractor writes has a record in
`attachments.jsonl` whose `sourceId` is exactly its `ref`, and its bytes at
`files/<sourceId>/<filename>`: `attachment-<id>` for a file embedded in the
text of a card, a comment or a post, `blob-<key>` for a file dropped into a
chat, `upload-<id>` for the file an upload of Docs & Files stands for, and any of
those with the owner behind it for one owner of a file beyond the first, see below.
The one reference without a record is a file the run could not fetch,
see Files that cannot be fetched (C-030).

**A file comes by one of two routes.** A file of a text or a chat is fetched **at
a storage address** the extractor builds from the key of its stored file, held to
one pattern right before the call. The one file of an **upload** of a Docs & Files
tool is fetched **by the identifier of the upload and of its project**: Basecamp
names no stored file on such a record, and the address it does give —
`…/buckets/{b}/uploads/{id}/download/{name}`, the API's own — answers with a
redirect to a store of its own that is no storage address of the pattern. The
pattern stays exactly as strict as it is and the redirect is never followed: what
it leads to is not a place a run fetches from, and its query carries the name of
the file. `basecamp files download <id> --in <project> --out -` knows the way and
is asked to take it.

- **The record** carries `filename`, `contentType` as Basecamp says it, or
  `null`, `bytes`, `sha256`, `file`, which is exactly
  `<sourceId>/<filename>`, `ownerKind` (`card`, `comment`, `post`, `message`,
  `answer`, `todolist`, `todo`, `forward`, `forwardreply`, `document`,
  `upload`) and
  `ownerSourceId`. Basecamp gives a file
  neither a time nor a creator, so
  `createdAt` and `updatedAt` are those of its owner, and `createdBy` is
  `null`, which the format reads as the author of the owner. Avatars stay as
  they are, with their person as creator.
- **One record and one download per `sourceId`**, however many references of
  **one body** name it: a text may embed the same file twice, and it owns that
  file once.
- **One record per owner, one download per file** (C-066). Where a file turns up
  under a **further owner**, that owner gets a record of its own, with the same
  bytes and a `sourceId` made of the source's own name for the file and the owner
  it belongs to: `attachment-882798998.post-10203006109`. No name of the source
  holds a dot — an identifier of Basecamp is digits, and the key of a stored file
  is held to letters, digits, a hyphen and an underscore before it becomes one —
  so no such record can carry the name of another file. The bytes are
  fetched for the first record alone and the others are linked to it, or copied
  where they cannot be linked, so two records never mean two downloads.
  **The owner and not the order of the walk** names such a record, because Minka
  takes a record over by its identifier: one that moved when another record moved
  would leave the record an earlier run wrote standing and store the same bytes
  again. The one identifier that still moves is the **first** owner's, which is the
  source's plain name for the file: where the record that met a file first is gone
  from the source, the next owner of it becomes the first and its record is called
  what that one was called.
  `filesOfSeveralOwners` counts the **owners beyond the first**, met and not
  written: a file under three owners that no run can fetch counts 2 there and
  leaves no record at all.
  Every number about fetching counts one file of the source once —
  `filesExpected`, `filesBytesExpected`, `filesFetched`, `filesReused`,
  `filesGone`, `fileNamesChanged` and `filesBytes`. So the index holds
  `avatars.count` plus `filesExpected` plus `filesOfSeveralOwners` records, less
  the entries of `missingFiles` that name a file of a text rather than an avatar —
  an avatar that is missing is already out of `avatars.count`. That is the whole of
  `counts.attachments` and the form to check it in.
  **What became of a file becomes of every owner of it at once:** one the source
  no longer has is `gone` on every record of it, one this run could not fetch is
  missing under every one of them with the reason the first got, and nothing of it
  is asked for a second time.
- **Order.** The index begins with the avatars, in the order of the people,
  and goes on with the files in the order the walk meets them: the files of
  a card, those of its comments, the next card; then the posts the same way;
  then the lines; then those of a mail, of its answers and of its comments;
  within a text in the order of the references. A record of a further owner
  stands where that owner's own records stand. A file is
  fetched when its owner is written, and its record is held until the people
  are written at the end of the run.
- **Through the Basecamp CLI.** `basecamp files download <address> --out -
  --agent` streams the bytes of a file to its standard output, and
  `basecamp files download <uploadId> --in <projectId> --out - --agent` those of
  the one file of an upload of Docs & Files, which has no address of that kind.
  `--out -`
  matters: given a directory, the Basecamp CLI chooses the name itself and
  overwrites a file of that name. What the call names stands behind `--`, both
  identifiers being the source's own written as digits, and
  `BASECAMP_NONINTERACTIVE=1` is set.
- **The address** is
  `https://storage.3.basecamp.com/<account>/blobs/<key>/download/<name>`.
  The extractor's own are the scheme, the host, the account of the run and
  the place of every part. Taken from Basecamp are the key, which has to
  have the shape of a UUID, and the name: the last segment of the
  `download_url` Basecamp gives the file, as escaped there, when that
  address names the same account and key, else the name of the file,
  escaped. Neither a host nor a path of an answer is passed on. The Basecamp
  CLI takes a download from a storage address only, not from the
  `download_url` itself, and sends its token to the API it is signed in at,
  whatever host the address names. The preview address of a file is never
  fetched: a bundle carries the original.
  **An upload of Docs & Files has no such address**, and none is built for it:
  Basecamp names no stored file on the record, and the address it does give is
  the API's own, `…/buckets/<bucket>/uploads/<id>/download/<name>`, which answers
  with a redirect to a store of its own. That redirect is never followed — it is
  no storage address of the pattern, and its query carries the name of the file —
  so the file is asked for by the two identifiers instead.
  Right before the call the address is held against one pattern
  (`checkStorageAddress`): https, a host that begins with `storage.` and
  ends in `.basecamp.com`, digits for the account, a key of the shape of a
  UUID, one segment for the name, and no query and no fragment. The name has
  valid escapes and, with them taken away, is neither `.` nor `..` and holds
  neither `/` nor `\` nor a NUL: the Basecamp CLI asks for the path without its
  escapes, so such a name would be a path of its own, as far as into another
  account, asked for with the sign-in. A file whose key or name does not
  pass is missing as `notUnderstood`, never `gone`, and is never asked for: the
  refusal is this run's own and says nothing about the file, so nothing that
  stood on it is written off and the run ends 7. The reason says whether the key
  has no form that can be asked for or the address Basecamp gives cannot be asked
  for safely.
- **Streaming.** The bytes go from the Basecamp CLI into the staged file
  while they are counted and digested (`bundle.Writer.StoreFile`); a file is
  never held in memory, and there is no limit of size and no check of the
  type: the extractor fetches everything and decides nothing. Directories
  are 0700, files 0600.
- **Limits of time.** The Basecamp CLI puts none on a download. A download
  that brings no byte for 60 seconds (`fileIdleTimeout`), or that has not
  ended after an hour (`fileTimeLimit`), is ended together with the Basecamp
  CLI and counts as a failure of the network. A run that is stopped ends the
  Basecamp CLI at once and is `interrupted`; nothing of the bundle stays.
  The reset of the idle time with every piece is what lets a long download
  through.
- **What a download came to.** The exit code decides whether it failed,
  whatever stands on standard output, where the Basecamp CLI prints its
  failure object in place of the file, or behind the part of a file that
  broke off. Exit 0 and as many bytes as Basecamp says the file has
  (`byte_size`, where it says one) is a file. Another number of bytes is a
  failure: fewer came, and the transfer broke off, which may pass; more came, and
  nothing here can say what arrived, which is `notUnderstood`. The size alone
  decides, whatever the bytes look like. Only where Basecamp gives no size does the look count:
  exit 0 with nothing but a failure object (`ok: false`) of a few kilobytes
  is then a failure and no file (`ErrLooksLikeAFailure`). Exit 2 with the
  failure object `not_found` alone on standard output is a file Basecamp no
  longer has:
  its record says `gone: true`, with `file`, `bytes` and `sha256` `null` and
  everything else as Basecamp lists it (`filesGone`, and a `notice`). A
  crash with exit 2 is no such proof, and neither is `not_found` behind a
  part of a file. A Basecamp CLI that is not signed in (exit 3) ends the
  run as `auth_required`, as it does for a request: no further file would
  come either. Every other failure, forbidden included, leaves the file
  out, see Files that cannot be fetched. The download of a file is not asked
  again within a run.
- **Names.** `filename` is a plain name: `/`, `\` and control characters
  become `_`, an empty name, `.` and `..` become `file`, and a name of more
  than 255 bytes, which is never more than 255 characters, is shortened
  before its extension, because a file system takes no more
  (`plainName`). `fileNamesChanged` counts the names that changed. No output
  ever says a name, a caption or an address: they are personal data, and a
  reason names the `sourceId` and a class of failure, never what the
  Basecamp CLI printed.
- **Taking files over.** A run replaces the bundle as a whole, and would
  fetch thousands of files again. So a file is taken over from the bundle at
  `--out` that the run replaces, instead of being downloaded, when the index
  of that bundle has a record of the same `sourceId` that names the same
  `file`, the file is a regular file of the size the record gives, which is
  also the size Basecamp gives, and its digest, computed anew, is the one of
  the record (`bundle.Writer.ReuseFile`). Anything else is fetched. A record
  that says `gone` is not trusted, the file is asked for again; an avatar is
  never taken over, because somebody may have another picture under the
  same ID. The old bundle is only read. The file is linked (a hard link),
  or copied where it cannot be linked or is readable by others, and the
  copy is digested again. A link shares its content with the old file, so
  nothing is ever written into a file that stands: every file goes into a
  directory that is made anew for it, one per attachment, and a directory
  that stands takes nothing. A file is only ever removed by its name.
  Removing the old bundle, giving a run up and tidying up after a killed one
  therefore leave the other bundle whole. `filesReused` and `filesFetched`
  count both ways. A record of a **further owner** of one file is taken over by
  neither route: it shares what the first record of the file got, taken over or
  fetched, so a duplicate is never fetched because its identifier is new.
- **What a run that ends hard loses.** Files are taken over from a bundle
  that is in place, and a bundle takes its place only at the end of a run.
  A run that fails, is stopped or is killed therefore loses the files it
  had fetched, and the next run fetches them again. The first complete run
  over an account is the expensive one; every run after it fetches what is
  new or was missing.

A further kind is a further step of the walk that adds its records to the same
writer, feeds the creators it meets into the same people and attaches the
files of its texts the same way.

## What a bundle says about clients

What a client has seen, they have seen: what a bundle says about them is
therefore read from the source and never guessed (C-055).

- **People** carry `client`, either way and never left out, because Minka reads
  `false` as said and that is the only way back from a client to somebody of the
  company. Basecamp says it of everybody, on the list of the account, among the
  people of a project and on the creator of a record alike. Somebody of the
  legacy kind `personable_type: "Client"` comes with `client: true` even where
  the flag says no: only that keeps somebody the import finds here by their
  address out of the projects, and Minka retires them for it.
- **`company`** stands on a client alone, `{sourceId, name}` or `null`, because
  Minka reads it nowhere else and where somebody who is no client works is
  nothing an import needs. A company the source names without an identifier, or
  with a name Minka refuses — blank, longer than 200 characters, or holding a
  character nobody can see — ends the run as `api_error`, where the record that
  names it comes in and not at the end of a walk of hours. Minka reads a company
  that is missing and one that is `null` the same way and drops the company it
  holds, so writing the record without it would say of somebody that they belong
  to nobody, and a later run would take away what the run before wrote. The
  message names the company's identifier and what is wrong with the name, never
  the name: a field that holds no name holds something.
- **Projects** carry `clientAccess` from `clients_enabled` and `clientCompany`
  from `client_company`, and both stand or neither does: Minka reads a project
  that names a client company and no access as one whose clients have none. A
  project the source says nothing of clients about carries neither, which leaves
  what Minka holds alone.
- **`visibleToClients`** is carried by a tool, a post, a to-do list, a
  check-in question and every folder, document and upload of Docs & Files: the
  records Minka lets decide for themselves. It comes from
  the recording of the record itself — a dock entry says nothing about clients,
  so a tool is read where the tool is read. Where the source says nothing,
  nothing is written: in doubt, private.
- **An entry of Docs & Files carries it at every depth** (C-064), which is the
  one place a record standing inside another says anything about clients. Minka
  reads the word below the top of a tool as well, and there it is the one thing
  that can hide a folder at the top: where a folder, a document or an upload
  below a top folder the source shows is hidden from clients, the import hides
  that top folder with everything in it and names it. An entry that left the
  word out would be imported in the open under a folder clients see.
  So an entry **inside a folder** that the source says nothing about is **named**
  in `docs.silentAboutClientsInsideAFolder` — the kind, the `sourceId` and the
  project, and never a title — and a `notice` says so: nothing is written for it,
  Minka reads it as one that changes nothing above it, and where the folder at the
  top of its tool is shown to clients that word was the one thing which would have
  hidden it. It is named and not merely counted because the one thing to do about
  such an entry is to go and compare it against the source, which nobody can do
  with a number. Saying so is all a run can do — what the source says is the
  source's — but a run that said nothing would let the harm this tool is written
  to prevent arrive through the safest-looking answer there is. At the top of a
  tool the same silence costs nothing and is not named: Minka reads an absent word
  as private there.
- **What follows what it hangs on** — a to-do, a group, a card, a step, an
  answer, a comment, a line of a chat, a forwarded mail, an answer by mail —
  carries no word of its own, because a
  second truth about one record is one truth too many. Where the source offers
  one, it is counted (`visibilityLeftToTheContainer`).
- **A client is no member of a project in Minka**, which leaves the memberships
  of the bundle that are a client's out of the import. The run says how many
  (`clientMemberships`), and a `notice` with them. For the same reason no
  question is ever said to be asked of a client: Minka drops such a person from
  `asked` without a word, so a client among the subscribers of a question is left
  out here and counted (`askedLeftOut.clients`), beside whoever is no member, has
  left or comes without an address. One reason is counted per person: being no
  member of the project first, then being a client, then having left and coming
  without an address, because Minka has a client out of the project before it
  asks anything else about them.
- **Every kind of tool Minka has a counterpart for is extracted**, so no bundle
  is silent about whether clients see one of them. An Email forwards
  tool carries its word like every other tool,
  and a forwarded mail and an answer by mail follow it: an inbox is shown to
  clients as a whole or not at all.

The report counts and never names: how many clients came, how many memberships
of theirs Minka leaves out and how many words about clients fell to a container.
A name and a company are personal data.

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
- `--dry-run` walks Basecamp the same way, downloads no avatar and no file,
  writes nothing and reports the same counts and the same parts, and what a
  run would fetch: `filesExpected`, the files the references name, each once,
  and `filesBytesExpected`, the sum of the sizes Basecamp gives them. It does
  not look at what a run could take over. It needs no `--out`; given one, it
  checks it the way the real run does, so the refusal shows beforehand.
- The report carries `counts` as the manifest has them, `avatars`,
  `skippedTools`, `missingFiles`, `creatorProbesRefused`, `cardsByStatus`,
  `cardsOnHold`, `cardsLeftOut`, `commentsTrashed`, `postsByStatus`,
  `postsLeftOut`, `postsWithEmptyBody`, `messagesByType` (`text`, `richText`,
  `upload`, `integration`), `messagesWithEmptyBody`, `uploadLinesLeftOut`,
  `recordsLeftOut` (`record`, `field`, `value`, `in`, `inSourceId`,
  `leftOut`),
  `parts` as the manifest has them, `mentionsUnresolved`, `filesInBodies`,
  `filesDropped`, `filesExpected`, `filesBytesExpected`, `filesBytes`,
  `filesFetched`, `filesReused`, `filesGone`, `fileNamesChanged`,
  `captionsDropped`, `filesOfSeveralOwners`, `linkPreviews`,
  `picturesLinked`, `embeds`, `embedsLeftOut`, `references`,
  `todoListsByStatus`, `todosByStatus`, `todosLeftOut` (`lists`, `groups`,
  `todos`), `todoPlacesLeftOut` (`record`, `sourceId`, `groups`, `todos`),
  `todosWithoutAList` (`sourceId`, `projectSourceId`),
  `todosNotDone`, `todosWithPeople`, `todosWithSubtasks`, `subtasksLeftOut`,
  `todosThatRepeat`,
  `questionsLeftOut`, `anchorsInferred`, `nobodyAsked`, `askedLeftOut`
  (`notInTheProject`, `clients`,
  `retired`, `withoutAnAddress`), `answersWithoutABody`, `answersWithoutADay`,
  `answersTrashed`, `subscribers` (`lists`, `leftOff` with the four reasons of
  `askedLeftOut`, `unread`), `longestBody`, `longestBodyRecords`, `requestsRetried` and the absolute path of
  the bundle. The lines about check-ins are shown only by a run that met a
  questionnaire, and those about to-dos only by one that met a To-dos tool.
  `filesBytes` is what the files of the bundle weigh, fetched or taken over, each
  file of the source once whatever number of records own it, see Files; it
  and
  `filesFetched`, `filesReused` and `filesGone` are 0 in a dry run.
  `longestBody` is the longest text per kind, `cards`, `comments`, `posts`,
  `messages`, `answers`, `todolists` and `todos`, in characters: whoever sets
  the limits of where a bundle goes has to know it. `longestBodyRecords` names,
  under the same kinds, the record each number was measured on, so that it can
  be looked at at the
  source: `record` is the kind of record (`card`, `comment`, `post`, `line`,
  `answer`), `sourceId` which one, `in` and `inSourceId` where it stands — the
  chat of a line, the project of everything else — and `characters` the length,
  the number `longestBody` holds. Of two texts of the same length the one the
  run met first is named; a kind whose longest text has no characters names no
  record and is missing from `longestBodyRecords`. The numbers stay where they
  are: a reader of `longestBody` reads on unchanged. The text shows a line per
  named record under the line of the numbers. Basecamp's address of a record is
  not in it, because the run does not carry one; the identifier and the place
  are what it names. `requestsRetried` is a number, 0 for a run that met no
  failure; the text shows its line only when it is more, and so it does for
  `creatorProbesRefused`, `embeds`, `embedsLeftOut`, `references` and
  `recordsLeftOut`, whose lines come one per entry under a line that counts the
  records of all of them.
  No output names a title, a person or what stands in a text.
- A run prints nothing while it goes on, in any mode: the output contract
  knows results, warnings and errors only.

## Files that cannot be fetched

One file that is not to be had does not cost a run (C-069). Its attachment
record is left out, and the bundle is written and put in place: it is whole
and can be applied. A person whose avatar is missing comes with
`avatar: null`; a text keeps the reference to a file that is missing, which
the applier passes over until a later bundle answers it. What was written of
such a file is removed. `missingFiles` names each file with
`attachmentSourceId`, `ownerKind`, `ownerSourceId`, `reason` and `why`.
The rule holds for every file of a bundle. What ends the run is an
interruption, a bundle that cannot be written, and a Basecamp CLI that is
not signed in when a file is asked for. A failure of the file system comes
without the path it happened at, which ends in the name of the file. A file
Basecamp no longer has is not missing: it has a record that says so, see
Files.

**`why` says whose word a missing file is**, which is another question than the
`retryable` of a request: that one asks whether the same call is worth making
again, and this one asks who is to do something about the file. Three answers,
and each is acted on differently:

| `why` | what it is | the run |
|---|---|---|
| `failedThisTime` | network, rate limit, a gateway: it may pass by itself | `network`, exit 6, retryable — run the same command again |
| `refusedBySource` | Basecamp's own no: it has no such file, or it hands this one to nobody | exit 0, named in the notice and counted in the summary |
| `notUnderstood` | a guard of this run refused, an answer nobody can read, a Basecamp CLI that says something new | `api_error`, exit 7 — look at this program, not at the file |

**Only `refusedBySource` decides anything about the file** (C-069). A text that
was nothing but such a file is written without it as for a file the source no
longer has, and an upload standing for one is left out (C-063), because a record
that waited for it would wait on every run to the end of time. For the other two
the record is written and waits: a later run may bring the file, and nothing of
the source is written off over a failure this run could not place.

**Where the line runs.** Everything a run refuses **before it asks** — a key that
is no key, an address that cannot be asked for safely — is its own doing and
`notUnderstood`, whatever class that refusal carries; only what comes back from
the download is an answer about the file. A reading of the source that is wrong
by one shape therefore costs one exit code and a later run, not every file of an
account counted and named and gone, and a dry run says so before anybody spends
an hour. The same holds for an avatar: the class of a status that is no 200 is
read, so a 404 is Basecamp's word about the picture and a status nobody knows is
ours. **A 401 is neither**: it says this run is not signed in, so it ends the run
as it does for a file, rather than writing off every picture of an account over
one thing nobody refused. A 403 counts as Basecamp's word only because the host
is Basecamp's own — an address signed by somebody else that had run out would
answer the same.

**The exit code says what to do next, and nothing else** (C-069). A failure
nobody could place decides it wherever one is among the missing files, because
running again does not answer it; else one that may pass does; else every file
was refused by the source and the run ends well, with a `notice` naming them and
the count in the `summary`. What no repeat can bring is a result and not a
failure, left out and counted and named as any record a bundle cannot carry is
(C-050), and an exit code nobody can clear is one whoever watches it stops
reading. The thing to do about such a file is to look at what the run named — the
entries of `missingFiles` and `docs.onlyAFileNoRunWillBring` for the records that
stood on one — and settle it at the source. The report is in the output either
way: as `data` of the error envelope with `--json` and `--quiet` where the run
fails, as the report itself where it does not. A later run replaces the bundle
and brings what it can get then.

An avatar is fetched over https only, redirects included and at most five of
them, with a timeout of 30 seconds and at most 5 MiB, which is enforced while
reading. It has to be a picture of a known type: an error page must not end up
in a bundle. A file of a text or a chat comes through the Basecamp CLI, see
Files. A reason never carries the address or the name of a file, which name
a person or what they wrote.

## A bundle is personal data

A bundle holds names, e-mail addresses and pictures. Its directories are 0700
and its files 0600. It is written outside of any repository and deleted once
it has been applied. Nothing out of a real bundle appears in the repository:
not in a test, a fixture, a document or a commit message. Tests run against
`internal/fakebasecamp`, an invented account, and never call the Basecamp CLI
or the network.
