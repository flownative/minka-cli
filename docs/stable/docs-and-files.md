# Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body, an **upload** a file standing on its own with a
description. Both take comments, and the comments take reactions.

`docs tools`, `docs list`, `docs show` and `docs uploads show` read; `docs
create` and `docs edit` write a document; `docs comments …` write under either
kind of entry.

## What is not there, and why nothing pretends otherwise

The API offers no delete, trash, archive, unarchive or move of a document, no
creating, renaming or deleting of a folder, no reading of a tool's archive and
no reading of a draft — a draft is 404 for everybody, its author included.
Reactions reach the comments alone, although the web offers them on an entry
as well. Putting a file into a tool is two requests with their own failure
modes and waits for a card of its own; changing the title or the description
of an upload waits with it. The CLI therefore writes only into folders that
already exist, and the help of `docs uploads show` says where the rest is
done.

## The body, and the refusal that never comes

`RichText::fromUntrustedValue` asks whether a value carries a tag of an element
the server serves **before** it cleans anything. A body without one — raw
Markdown, plain text — is escaped, its line breaks turned into `<br>`, and kept
inside one `<div>`. The answer is the same 201 or 200 as any other, and
`# Heading` then stands in the document as the characters `# Heading`. The same
fallback catches HTML whose cleaning would drop text, and the same rule holds
for the description of an upload.

So the CLI converts Markdown itself, as it does for a card's description and a
comment, and **nothing will ever tell it that it forgot**. Where a caller sends
HTML with `--body-format html` or `--format html`, a text that carries no
element the server serves draws a `notice` naming what becomes of it
([C-068](../decisions.md#c-068)). It is not refused: the server accepts it, and
a CLI that refused what the server takes would be wrong about it.

## The title of a document, which is not a value one clears

`"title": null` and an absent title keep what stands. `"title": ""` is 422
`documentTitleMissing`. That is the opposite of `--due ""` and
`--description ""`, which clear a value, so `docs edit --title ""` reaches the
server and comes back as exit 9, and the help says that a title can only be
replaced. `--body ""` empties the body, as `"body": null` does.

## The guard, and the second it cannot see

A document and an upload keep their `updatedAt` in `TIMESTAMP(0)` — whole
seconds — and `expectedUpdatedAt: null` means **no expectation, last write
wins**. The working notes of a card keep theirs in `TIMESTAMP(6)`, and there
`null` means **no notes have been written yet**. One flag over both would
invert the meaning on one of them, so there are three mappings and no shared
helper ([C-067](../decisions.md#c-067)):

- `docs edit --expect <updatedAt>` sends the string **verbatim as it was read**.
  It always ends `.000000Z`. The shape is held here first, so a value with no
  zone exits 1 as a usage error rather than travelling to be answered with 400
  `malformedBody`.
- `docs edit --force` leaves the field **out of the body**, and never sends
  `null`.
- `--expect none` is refused as a usage error: it is the word of `cards notes
  set`, and it would mean the opposite here.

One of the two flags is required, as for the notes, and the CLI does not read
the document to fill `--expect` in: a guard protects only what the writer read
before it began to write ([C-042](../decisions.md#c-042)). Where `--body` is
given the document **is** read, for the one thing the new body has to be held
against — whether the old one carries files or mentions that Markdown cannot
hold — and that read still fills no guard.

The round trip is safe: the `updatedAt` a read hands over is the column the
write is held against. What it cannot do is notice a change made **in the same
second as the read**: that change carries the moment that was read and passes
for it. Minka decided against microseconds for now, so every guarded write says
so in a `notice`, the help says it, and the SKILL.md says it where agents read
— after a write that matters, read the document back.

A refused write is 409 `documentChanged` with the document under `"document"`,
which becomes the `data` of the error, is printed for a person, and the `hint`
names the `--expect` to send next. An upload refuses with `uploadChanged` and
`"upload"`, which no command of this CLI can reach yet.

## Whom a new document reaches

`--audience` is required, and the CLI holds that word alone: `everybody`,
`chosen` or `nobody`, and anything else is a usage error before a request goes
out. Which people go with which audience is Minka's rule and stays there —
`notifyIds` under any audience but `chosen` is `notifyIdsWithoutChosenAudience`,
and `chosen` with nobody named is `notifyIdsMissing`. Both come back as exit 9
with a hint naming the flag to change. One rule in one place: a second copy here
would be a second thing to keep in step with a server that already answers it
([C-070](../decisions.md#c-070)).

`--notify` takes the ID of a person, or `me`. A name is not looked up: which
project a tool hangs in is not known before the document exists, and a name can
only be matched against the members of a project.

A **client** of the project is a member, and being a member is not enough to be
told: `--notify` naming a client is refused with `personCannotSeeIt` wherever
the document is not shown to clients, which is everywhere but inside a folder
that is (E-199). Nothing is written then, and nothing about the request can be
mended — the person is real, belongs to the project and may simply not hear of
it, so the answer is to name somebody else. The CLI cannot tell a client apart
and does not try: it does not know the project before the document exists, and
which folder a project shows its clients is decided in Minka on the web.

## Two kinds of entry, one flag

A document and an upload have addresses of their own, and the identifier of one
is 404 at the address of the other. The comment commands therefore take
`--document <documentId>` or `--upload <uploadId>`, exactly one of the two, the
way `uploads create` names its circle ([C-010](../decisions.md#c-010)).
Nothing is guessed and nothing is probed.

## The archive of a folder

An archived folder is in no list, and the entries below it go quiet with it:
readable, and closed to every write. Its inside **can** be listed by naming it
as `--folder`, and then the answer and every line of it says `archived: true`.
A write to an entry inside it answers `documentArchived` or `uploadArchived` —
never `folderArchived`, which is the answer to writing something **new** into
it.

## The refusals

`docs create` says what is wrong with the request before it meets the project:
the title, the body, the folder, the audience and the people named under it are
read first, so an archived project with an unknown folder answers
`folderUnknown`. Everywhere else `projectArchived` comes first, because there
the statement refuses before anything else can.

The people `--notify` names are met one at a time, in the order they were
given, and each is asked two things in turn: whether they belong to the project
at all, and whether the document is shown to them. So of one person
`personNotAProjectMember` comes before `personCannotSeeIt`, and of a list the
first name that is refused decides which of the two the answer is.

| Answer | Command | Exit |
|---|---|---|
| 404 | every read and write, for what is unknown, foreign, hidden from a client, in the trash, or a draft | 2 |
| 415 `unsupportedMediaType` | unreachable: every write of this CLI sends JSON | — |
| 400 `malformedBody` | unreachable: a moment `--expect` lets through is one the server reads | — |
| 409 `documentChanged` | `docs edit --expect` against a document written over since | 9 |
| 422 `documentTitleMissing`, `documentTitleTooLong`, `documentTitleHasControlCharacters` | `docs create`, `docs edit` | 9 |
| 422 `documentBodyTooLong`, `documentBodyNotReadable` | `docs create`, `docs edit` | 9 |
| 422 `documentAudienceMissing` | unreachable: the audience is held here | — |
| 422 `notifyIdsMissing`, `notifyIdsWithoutChosenAudience` | `docs create --audience` and `--notify` that do not go together | 9 |
| 422 `personNotAProjectMember` | `docs create --notify` naming somebody outside the project | 9 |
| 422 `personCannotSeeIt` | `docs create --notify` naming a client of the project, for a document the clients of it are not shown | 9 |
| 422 `folderUnknown`, `folderArchived` | `docs create --folder` | 9 |
| 422 `documentArchived`, `uploadArchived` | every write on an archived entry, or under one, comments included | 9 |
| 422 `projectArchived` | every write; on `docs edit` and the comments before the codes above, on `docs create` after the title, the body and the folder | 9 |
| 404 `variantNotAvailable` | `uploads download --variant` of the file of an upload, as for any file | 2 |
| 422 the comment codes | `docs comments …`, as under a card | 9 |

## What a list promises

Nothing here is paged and nothing is silently cut, so a short list is the whole
list. `entries` reaches one folder deep: going down means calling again with a
folder. The order is the web's own position, neither alphabetical nor by date,
and the CLI passes it through. `lead` is the first 240 characters of the body
without markup, cut where the count runs out and not at a word.
