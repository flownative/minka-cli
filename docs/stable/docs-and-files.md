# Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body, an **upload** a file standing on its own with a
description. Both take comments, and the comments take reactions.

`docs tools`, `docs list`, `docs show` and `docs uploads show` read; `docs
create` and `docs edit` write a document; `docs uploads create` puts a file into
a tool and `docs uploads edit` changes an upload; `docs versions list` and
`show` read the versions of a document, `docs uploads versions list` the files
of an upload, and `docs uploads versions create` gives an upload another file;
`docs comments …` write under either kind of entry.

## What is not there, and why nothing pretends otherwise

The API offers no delete, trash, archive, unarchive or move of a document or
an upload, no creating, renaming or deleting of a folder, no reading of a
tool's archive and no reading of a draft — a draft is 404 for everybody, its
author included. Reactions reach the comments alone, although the web offers
them on an entry as well. A change never replaces the file of an upload: Minka
ignores a `blobId` sent with it, so `docs uploads edit` has no flag for it
([C-085](../decisions.md#c-085)); another file goes in as a new version. Making
an earlier version of a document the current one, giving an upload an earlier
file back by its version and the marked differences between two versions have
no address either. The CLI writes only into folders that already exist.

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

## A file goes into a tool in two requests

Minka takes a file into a tool in two steps: `POST projects/{id}/uploads`
stores the bytes in the project, as `uploads create` does, and `POST
files-tools/{id}/uploads` with the `blobId` of that answer puts the stored file
on top of the tool or of a folder. `docs uploads create <file> --tool <toolId>`
takes both in one run ([C-085](../decisions.md#c-085)):

1. It reads the tool, `GET files-tools/{id}`, for the project it stands in,
   and stops with exit 2, before the file travels, where the tool is at no
   address for the person of the token. A file stored in another project than
   the tool's is refused only after it travelled.
2. It stores the file in the project of the tool. A refusal ends the run and
   nothing is put anywhere.
3. It puts the stored file into the tool. Where Minka refuses that, the file
   stays stored and nothing carries it: the error's `data` is the stored file
   and the hint names `--blob <blobId>`, which takes this step alone.

`--blob <blobId>` in place of the file is the second step alone, for such a
file or one `uploads create --project` stored; `--project`, `--name` and
`--content-type` are refused beside it. A file stands in one place: in a text
or in an upload, never in two.

`--project` is hidden and still taken ([C-092](../decisions.md#c-092)): the
project it names has to be the tool's, or the run stops with exit 2 before the
file travels, and a run it does not stop says in a `notice` that it is not
needed, also where the second step is refused.

A file nothing carries is kept for **at least** seven days. Minka's sweeper runs
daily and when the service starts, so seven days are a floor and not a
deadline, and everything that speaks of them says so.

### The title and the description of a new upload

A title left out, or `null`, names the upload after the file; `""` is 422
`uploadTitleMissing`. Minka reads the title only after the file travelled, so
the CLI refuses an empty or blank `--title` itself, before anything goes out,
and sends no title where none was given. A description left out, `null` and
`""` all mean an empty one; it is Markdown converted here, as a body is.

Nobody is told about a new upload but the people mentioned in its description,
and there is no audience and no `notifyIds`. The writer is subscribed to it.

### The file Minka does not know

`fileUnknown` covers a file swept after standing in nothing, one never stored,
one of another project, one hidden from a client and one somebody else stored.
Nothing tells these apart. With `--blob` the hint names the one remedy worth
trying: store the file again by giving it in place of `--blob`. Where the same
run stored the file a moment before, it cannot have been swept, and the hint
says that storing it again is not the remedy. `fileInUse` is a file of one's
own that stands in an upload or a text already; of somebody else's Minka says
`fileUnknown`.

## Versions

`docs versions list <documentId>` and `docs uploads versions list <uploadId>`
read what Minka keeps of every write: the versions of a document, and the
files an upload showed. Both come oldest first, unpaged, the newest marked
`current` — for an upload the file it shows now. `docs versions show
<documentId> <versionId>` reads one version of a document: its title, its body
as `docs show` renders a body, and the files that body names. A version is
reached only through its document, so the command takes both IDs. Minka
hands out a version's body only as it stores it, where a file carries no name
and no address, so a file in it reads as `[attachment]` and shows only in the
list of files. `author` is
`null` for a version written before Minka kept who wrote it.

**The versions are the team's alone.** A client of the project meets 404 at
every one of these addresses, also where the document or upload itself is shown
to it, and at an earlier file of an upload it did not store itself. The hint of a 404 says so. A
document or upload in the archive, below an archived folder or in an archived
project is read with its versions; one in the trash is not found.

### Another file for an upload

`docs uploads versions create <uploadId> (<file> | --blob <blobId>)` gives an
upload another file, as the web's "Replace with a new version" does
([C-099](../decisions.md#c-099)). The title and the description stay, and the
file it showed stays as an earlier version. It takes the two requests of a new
upload, with the same hand-over:

1. Given a file, it reads the upload for its project and then its versions,
   and stops with exit 2 before the file travels where either is at no
   address: the second read is what tells a client, who may read the upload
   and give it nothing. An upload the first read says is `archived` — itself
   or through a folder above it — stops the run with exit 9 before the file
   travels, where Minka would refuse it with `uploadArchived` only after the
   transfer and leave the stored file standing in nothing.
2. It stores the file in the project of the upload. A refusal ends the run.
3. It sends the stored file to `POST files-uploads/{id}/versions`. Where Minka
   refuses that, the error's `data` is the stored file and the hint names
   `--blob <blobId>`, which takes this step alone.

`--blob` naming the file the upload shows already is answered 200 and writes
nothing; the command says so in its summary and a `notice`, and adds no
version. An earlier file of the same upload stands in it, so it is
`fileInUse`: an upload gets an earlier file back only by downloading it and
giving it as a file, which stores its bytes anew.

`--expect <updatedAt>` is optional, and `--force` does not exist here. Without
an expectation Minka expects the upload as the request found it, so a title or
description changed in the meantime is never written over, and a file given in
the meantime stays as a version: nothing is lost. With one, a change since the
read is refused as by `docs uploads edit`, with the upload under `data` and a
hint that names both the `--expect` to send next and, where this run stored the
file, `--blob`. The shape of `--expect` is held as for every entry, and
`--expect none` is refused.

Minka refuses in this order: a body it cannot read; the upload (404, a client
included); `expectedUpdatedAt` and `blobId` that are no moment or no
identifier (400); a file the project does not know (`projectArchived` in an
archived project, else `fileUnknown`); then what the write meets —
`projectArchived`, `uploadArchived` for the upload or a folder above it, the
file it shows already (200, whatever the expectation), `uploadChanged`, and
last whether the file may be taken: somebody else's is `fileUnknown`, one's
own that stands anywhere `fileInUse`.

## The title of a document, which is not a value one clears

`"title": null` and an absent title keep what stands. `"title": ""` is 422
`documentTitleMissing`. That is the opposite of `--due ""` and
`--description ""`, which clear a value, so `docs edit --title ""` reaches the
server and comes back as exit 9, and the help says that a title can only be
replaced. `--body ""` empties the body, as `"body": null` does.

An upload is the same: `docs uploads edit --title ""` reaches Minka and comes
back as exit 9 `uploadTitleMissing`, `"title": null` keeps the title, and
`--description ""` empties the description, as `"description": null` does.
A change that names nothing new answers 200 and writes nothing.

## The guard, and the second it cannot see

A document and an upload keep their `updatedAt` in `TIMESTAMP(0)` — whole
seconds — and `expectedUpdatedAt: null` means **no expectation, last write
wins**. The working notes of a card keep theirs in `TIMESTAMP(6)`, and there
`null` means **no notes have been written yet**. One flag over both would
invert the meaning on one of them, so there are three mappings and no shared
helper ([C-067](../decisions.md#c-067)):

- `docs edit --expect <updatedAt>` and `docs uploads edit --expect` send the
  string **verbatim as it was read**.
  It always ends `.000000Z`. The shape is held here first, so a value with no
  zone exits 1 as a usage error rather than travelling to be answered with 400
  `malformedBody`.
- `--force` leaves the field **out of the body**, and never sends `null`.
- `--expect none` is refused as a usage error: it is the word of `cards notes
  set`, and it would mean the opposite here.

One of the two flags is required, as for the notes, and the CLI does not read
the document to fill `--expect` in: a guard protects only what the writer read
before it began to write ([C-042](../decisions.md#c-042)). Where `--body` is
given the document **is** read, for the one thing the new body has to be held
against — whether the old one carries files or mentions that Markdown cannot
hold — and that read still fills no guard. That guard stands aside for a
document of the archive, one below an archived folder and one in an archived
project, where Minka refuses any body ([C-082](../decisions.md#c-082)).
`docs uploads edit --description` reads the upload for the same reason and
stands aside in the same places.

`docs comments edit` reads the entry first, for the files and mentions a
comment of one's own may carry. A comment of somebody else, one under an entry
of the archive and one in an archived project Minka refuses whatever is sent,
so there the request goes out. Under an entry that lies in the archive only
through a folder above it, a Minka that asks the folders refuses every write on
a comment as well — a change, a removal, a reaction and taking one back — with
`documentArchived` or `uploadArchived`; **a Minka whose statement asks the entry
alone changes and removes one's own comment there**, v0.41.0 among them. So
there the guard stands: the entry says `archived` for both cases, and the
command reads the folder to tell them apart; where both the entry and its
folder are archived the guard stays as well. Against a Minka that refuses, this
costs more than a request: such an edit reads the entry, the person of the
token, the folder and the archived projects — the last two only for the guard —
and exits 1, and only a second run with `--drop-attachments` reaches Minka's
422. The price is paid for the files a Minka that writes there would lose.

The round trip is safe: the `updatedAt` a read hands over is the column the
write is held against. What it cannot do is notice a change made **in the same
second as the read**: that change carries the moment that was read and passes
for it. Minka decided against microseconds for now, so every guarded write says
so in a `notice`, the help says it, and the SKILL.md says it where agents read
— after a write that matters, read the document back.

A refused write is 409 `documentChanged` with the document under `"document"`,
which becomes the `data` of the error, is printed for a person, and the `hint`
names the `--expect` to send next. An upload refuses the same way with
`uploadChanged` and the upload under `"upload"`.

## Whom a new document reaches

`--audience` is required, and the CLI holds that word alone: `everybody`,
`chosen` or `nobody`, and anything else is a usage error before a request goes
out. Which people go with which audience is Minka's rule and stays there —
`notifyIds` under any audience but `chosen` is `notifyIdsWithoutChosenAudience`,
and `chosen` with nobody named is `notifyIdsMissing`. Both come back as exit 9
with a hint naming the flag to change. One rule in one place: a second copy here
would be a second thing to keep in step with a server that already answers it
([C-070](../decisions.md#c-070)).

`--notify` takes the ID of a person, `me`, an e-mail address or a name. A name
or an address is looked up among the members of the project the tool stands in:
the tool is read for its project, and the members of the project once, and
only where one is given. An ID and `me` go out without either read, and Minka
says what it thinks of them ([C-070](../decisions.md#c-070)).

A **client** of the project is a member, and being a member is not enough to be
told. An agent whose principal is a client counts as a client. `--notify`
naming either is refused with `personCannotSeeIt` wherever the document is not
shown to clients (E-199): it is shown only where the tool is shown to clients
**and** the topmost folder above the document is. A folder further down has no
say, and a new document at the top of the tool is shown to no client. Nothing
is written then, and nothing about the request can be mended — the person is
real, belongs to the project and may simply not hear of it, so the answer is to
name somebody else. The CLI does not try to tell a client apart: which tools
and folders a project shows its clients is decided in Minka on the web.

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

Minka puts each person on the document's list as it meets them and passes over
whoever is on it already, the writer from the start. So **a person named twice
is told once**, and the CLI sends `--notify` as given without merging
repetitions. Somebody outside the project is `personNotAProjectMember` however
often they are named.

| Answer | Command | Exit |
|---|---|---|
| 404 | every read and write, for what is unknown, foreign, hidden from a client, in the trash, or a draft | 2 |
| 404 | `docs uploads create <file>` reading the tool, before the file travels; `docs create --notify` and `posts create --notify` reading it for a name | 2 |
| — | `docs uploads create <file> --project` naming another project than the tool's, before the file travels | 2 |
| 415 `unsupportedMediaType` | unreachable: every write of this CLI sends JSON | — |
| 400 `malformedBody` | unreachable: a moment `--expect` lets through is one the server reads, and a `--blob` that is no ID exits 1 | — |
| 409 `documentChanged` | `docs edit --expect` against a document written over since | 9 |
| 409 `uploadChanged` | `docs uploads edit --expect` against an upload written over since | 9 |
| 422 `uploadTitleMissing`, `uploadTitleTooLong`, `uploadTitleHasControlCharacters` | `docs uploads edit`; of `docs uploads create`, all but an empty title, which exits 1 before anything goes out | 9 |
| 422 `uploadDescriptionTooLong`, `uploadDescriptionNotReadable` | `docs uploads create`, `docs uploads edit` | 9 |
| 422 `fileUnknown` | `docs uploads create`, for a file swept, never stored, of another project or somebody else's — one answer for all | 9 |
| 422 `fileInUse` | `docs uploads create --blob` naming a file of one's own that stands in an upload or a text; `docs uploads versions create --blob` naming one that stands in an upload, a text or an earlier version, of the same upload included | 9 |
| 404 | `docs versions list`, `docs versions show`, `docs uploads versions list` and `docs uploads versions create`, also for a client of the project shown the entry; a version of another document | 2 |
| 409 `uploadChanged` | `docs uploads versions create --expect` against an upload written over since | 9 |
| 422 `fileUnknown` | `docs uploads versions create`, as for `docs uploads create` | 9 |
| 422 `uploadArchived` | `docs uploads versions create --blob` for an upload of the archive or below an archived folder; with a file the command exits 9 itself before the file travels | 9 |
| 422 `contentTypeMismatch`, 413 `tooLarge` | `docs uploads create <file>`, storing it, as `uploads create` | 9 |
| 422 `documentTitleMissing`, `documentTitleTooLong`, `documentTitleHasControlCharacters` | `docs create`, `docs edit` | 9 |
| 422 `documentBodyTooLong`, `documentBodyNotReadable` | `docs create`, `docs edit` | 9 |
| 422 `documentAudienceMissing` | unreachable: the audience is held here | — |
| 422 `notifyIdsMissing`, `notifyIdsWithoutChosenAudience` | `docs create --audience` and `--notify` that do not go together | 9 |
| 422 `personNotAProjectMember` | `docs create --notify` naming somebody outside the project | 9 |
| 422 `personCannotSeeIt` | `docs create --notify` naming a client of the project or an agent of one, for a document the clients of it are not shown | 9 |
| 422 `folderUnknown`, `folderArchived` | `docs create --folder`, `docs uploads create --folder` | 9 |
| 422 `documentArchived`, `uploadArchived` | every write on an archived entry, or under one, comments and their reactions included, and a folder above the entry counts — but v0.41.0 changes and removes one's own comment below an archived folder | 9 |
| 422 `projectArchived` | every write; on `docs edit`, `docs uploads edit` and the comments before the codes above but after the title, the description and the moment of `--expect`; on `docs create` after the title, the body, the folder, the audience and `notifyIdsMissing` / `notifyIdsWithoutChosenAudience`, and before `personNotAProjectMember` and `personCannotSeeIt`; on `docs uploads create` with a file when storing it, after the whole transfer, and with `--blob` before `fileUnknown` and again before `fileInUse` | 9 |
| 404 `variantNotAvailable` | `uploads download --variant` of the file of an upload, as for any file | 2 |
| 422 the comment codes | `docs comments …`, as under a card | 9 |

## What a list promises

Nothing here is paged and nothing is silently cut, so a short list is the whole
list. `entries` reaches one folder deep: going down means calling again with a
folder. The order is the web's own position, neither alphabetical nor by date,
and the CLI passes it through. `lead` is the first 240 characters of the body
without markup, cut where the count runs out and not at a word.
