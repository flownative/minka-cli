# Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body, an **upload** a file standing on its own with a
description. Both take comments, and the comments take reactions.

`docs tools`, `docs list`, `docs show` and `docs uploads show` read; `docs
create` and `docs edit` write a document; `docs uploads create` puts a file into
a tool and `docs uploads edit` changes an upload; `docs versions list` and
`show` read the versions of a document, `docs uploads versions list` the files
of an upload, and `docs uploads versions create` gives an upload another file;
`docs comments …` write under either kind of entry. `docs tool <toolId>` reads
one tool by its ID alone, with the project it stands in, as `todolists tool`
reads a To-dos tool ([C-120](../decisions.md#c-120)).

A tool is kept in order with `delete`, `restore`, `archive`, `unarchive` and
`move` in the group of each kind — `docs …`, `docs uploads …`, `docs folders
…` — beside `docs folders create` and `edit`, `docs versions restore` and
`docs list --archived` ([C-130](../decisions.md#c-130)), see below.

`search` finds documents and uploads within a tool or a folder
([C-132](../decisions.md#c-132)), see below.

`docs book` reads a book as one tree, `docs folders edit --book` and
`--no-book` mark one, `docs edit --in-navigation` and `--not-in-navigation`
place a page in its navigation, `docs order` orders a place automatically or
by hand, and `move --after` and `--before` place an entry in an order set by
hand ([C-133](../decisions.md#c-133), E-302, E-307), see below.

`docs export` writes a book, a chapter or a folder into a directory as files
and brings it up to date on the next run ([C-135](../decisions.md#c-135)),
see below.

## What is not there, and why nothing pretends otherwise

The API offers no reading of a draft — a draft is 404 for everybody, its
author included, at every address of an entry, and the trash lists a trashed
one to its author only, with `restorable: false`. Reactions reach the comments
alone, although the web offers them on an entry as well. A change never
replaces the file of an upload: Minka ignores a `blobId` sent with it, so `docs
uploads edit` has no flag for it ([C-085](../decisions.md#c-085)); another file
goes in as a new version. Giving an upload an earlier file back by its
version, the marked differences between two versions and removing anything for
good have no address either.

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

A body holds the shapes of a document beside what any rich text holds:
callouts, Mermaid diagrams, link tiles, blocks of variables and their
placeholders. How each is written and shown in Markdown is in
[The shapes of a document](rich-text.md#the-shapes-of-a-document).

## A file goes into a tool in two requests

Minka takes a file into a tool in two steps: `POST projects/{id}/uploads`
stores the bytes in the project, as `uploads create` does, and `POST
files-tools/{id}/uploads` with the `blobId` of that answer puts the stored file
into the tool or a folder, first among the uploads where the order is
automatic, last where it is set by hand. `docs uploads create <file> --tool <toolId>`
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
so in a `notice`, the help says it, and the skill says it where agents read
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
`folderArchived` is the answer to writing something **new** into it, and to
taking an entry out of it on its own. `docs list --archived` names the folder;
what stands in it without being archived itself is not listed there.

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
| 422 `folderUnknown`, `folderArchived` | `docs create --folder`, `docs uploads create --folder`, `docs folders create --folder`, every `move --folder` | 9 |
| 404 | `restore` of an entry not in the trash by itself, below a folder out of use, or a draft; `restore`, `unarchive` and `docs versions restore` for a client; `delete` and `archive` of somebody else's for a client | 2 |
| 422 `folderArchived` | `delete` below an archived folder; `unarchive` of an entry archived with a folder above; a folder's `archive`, `edit` and `move` where it or a folder above is archived | 9 |
| 422 `documentNotArchived`, `uploadNotArchived`, `folderNotArchived` | `unarchive` of an entry not in the archive | 9 |
| 422 `folderIntoItself` | `docs folders move` into the folder itself or one below it | 9 |
| 422 `folderIdMissing` | unreachable: every `move` names its place | — |
| 422 `folderTitleMissing`, `folderTitleTooLong`, `folderTitleHasControlCharacters` | `docs folders create`, `docs folders edit` | 9 |
| 400 `malformedBody` | unreachable for `docs folders create`: `visibleToClients` goes out as a boolean | — |
| 422 `documentArchived`, `uploadArchived` | every write on an archived entry, or under one, comments and their reactions included, `archive`, `move` and `docs versions restore`, and a folder above the entry counts — but v0.41.0 changes and removes one's own comment below an archived folder; `delete` takes an archived entry, `unarchive` brings it back | 9 |
| 422 `projectArchived` | every write; on `docs edit`, `docs uploads edit` and the comments before the codes above but after the title, the description and the moment of `--expect`; on `docs create` after the title, the body, an unknown folder (but before an archived one), the audience and `notifyIdsMissing` / `notifyIdsWithoutChosenAudience`, and before `personNotAProjectMember` and `personCannotSeeIt`; on `docs uploads create` with a file when storing it, after the whole transfer, and with `--blob` before `fileUnknown` and again before `fileInUse` | 9 |
| 404 `variantNotAvailable` | `uploads download --variant` of the file of an upload, as for any file | 2 |
| 422 the comment codes | `docs comments …`, as under a card | 9 |

## Keeping a tool in order

Minka runs the web's handler for each of these writes, so its statement
decides who may: the team may do everything; a client of the project puts into
the trash and the archive only what they made, may make, rename and move, and
brings nothing back — no restore, no unarchive, no version (E-298). Each
refusal of a client is the 404 of something that is not there, and the hint of
every such 404 says so beside what else it can mean.

The three kinds take the same five writes, each in the group of its kind
([C-130](../decisions.md#c-130)): an ID of one kind is 404 at the address of
another, as for the comments, and a group per kind is what keeps a command from
reading it as the wrong one.

- `delete` takes the entry into the trash of its project, from the tool or from
  its archive, a folder with everything in it; `data` is `{id, deleted: true}`,
  as for a card. Below an archived folder it is 422 `folderArchived`: what
  stands there is not taken out on its own.
- `restore` brings back only what went into the trash by itself, below folders
  in use. What went in with its folder keeps its own state and comes back with
  the folder; one whose folder lies in the trash or the archive is 404, and the
  trash names that under `blockedBy`. `trash list` says `restorable` for each
  line; its breadcrumbs name the restore of each kind it holds one of.
- `archive` and `unarchive` answer with the entry, whose `archived` says where
  it stands. One archived together with a folder above it comes back only with
  that folder: 422 `folderArchived`, and the archive of the tool marks it
  `archivedWithAFolderAbove`.
- `move` takes `--folder <folderId>` or `--top-level`, exactly one, and the
  CLI refuses neither and both before a request goes out. The top level travels
  as `"folderId": null`; a body without the field is `folderIdMissing`, which
  this CLI never sends. The entry lands first among its kind in a place
  ordered automatically and last in one arranged by hand. A move to where the
  entry stands, without a neighbour, writes nothing and answers 200, so the
  answer cannot tell a move from none. `--after` and `--before` are below.
- `docs folders create` answers 201 with the folder in the shape of `docs
  list --folder`, its ID under `folder.id`. `visibleToClients` counts only at
  the top of the tool, in a project open to clients, from the team, and a
  client's own folder at the top is shown to clients anyway. No answer says
  whether it counted, so `--visible-to-clients` beside `--folder` draws a
  `notice` that it does not, and goes out as given. `docs folders edit` needs
  `--title`, `--book` or `--no-book`: a call without any writes nothing on
  Minka's side, so the CLI refuses it as a call that changes nothing. It sends
  only what is given.
- `docs versions restore <documentId> <versionId>` writes what the version
  said, title and body, as the newest version, and tells nobody. A version that
  says what the document says already answers 200 and adds none; the CLI cannot
  tell that answer from a write.
- `docs list <toolId> --archived` reads `GET files-tools/{id}/archive`: what
  went into the archive itself, at any depth, the one archived last first,
  unpaged; what lies below a folder in the trash is not in it. It does not go
  with `--folder`.

The hints name the write that helps, and one code can mean another folder to
another write: `folderArchived` is the folder above the entry to `delete` and
`unarchive`, the entry or a folder above it to a folder's `archive`, `edit` and
`move`, and the target to a document's or upload's `move` and to `create`.

Minka refuses in this order. Every write first finds the entry at its address
(404, a draft and an outsider included; for `restore` an entry in the trash by
itself, for the team), and reads the body where it takes one. Then:

- `delete`, `archive` and `unarchive` learn why the statement wrote nothing
  only afterwards: `projectArchived`; then, for `unarchive`, a client (404),
  the entry not archived (`…NotArchived`) and a folder above archived
  (`folderArchived`); for `archive` the entry archived itself or through a
  folder above (`documentArchived`, `uploadArchived`, `folderArchived`); for
  `delete` a folder above archived (`folderArchived`); then 404, which for
  `delete` and `archive` is a client and somebody else's. `restore` asks
  `projectArchived`, then a folder above out of use (404).
- `move`: an archived entry (`projectArchived`, then its own code), the field
  (`folderIdMissing`, a body Minka cannot read), a target it does not know
  (`folderUnknown`), then `projectArchived`, and only then an archived target
  (`folderArchived`), `folderIntoItself` and the place it stands already (200),
  so an archived project refuses a move to where the entry stands as well.
- `docs folders create`: the tool (404), the title, the folder above
  (`folderUnknown`; an archived one is `projectArchived` in an archived
  project and `folderArchived` otherwise), `visibleToClients` that is no
  boolean (400), then `projectArchived`. An archived folder given to `docs
  create` and `docs uploads create` is answered the same way. `docs folders edit`: the folder (404), the
  title, then `projectArchived` and `folderArchived`.
- `docs versions restore`: the document (404), a client (404), a version ID
  that is none or one the document has not got (404), then `projectArchived`
  and `documentArchived`.

## Books and the order of a place

Each place — the top of a tool and every folder — orders itself, and a folder
does not take the order of the one above it (E-302). `automatic` shows
folders, then documents, then uploads, each newest first; `manual` mixes the
three in an order set by hand. Every answer in the shape of `docs list`
carries `order`, `entries` (`[{kind, id}]` in the order shown), `isBook` and
`book`, the book the place belongs to, itself or one above; `folders[]` carry
`isBook` and `documents[]` `inNavigation`. Arranged by hand, the text view
lists one mixed list in the order of `entries`, each line naming its kind,
because three lists by kind would lose the order; ordered automatically it
keeps the three lists. A Minka that sends no `order` is read as automatic, one
that sends no `inNavigation` as listing the document, and the text says
nothing about either.

- `docs order <toolId> automatic|manual [--folder <folderId>]` is `PUT
  files-tools/{id}/order`. Manual keeps the order shown a moment before;
  automatic drops the order set by hand and the automatic one comes back
  unchanged. A place that orders so already answers 200 the same. Any other
  word is exit 1 before a request, so `orderUnknown` is unreachable. A new
  entry in a place arranged by hand goes to its end, and so does one moved in
  without a neighbour.
- `move --after <entryId>` and `--before <entryId>` send `previousEntryId`
  and `nextEntryId`; they exclude each other, and each is checked as an ID
  before a request, so `entryPlaceUnknown` is unreachable. Minka places the
  entry right behind the previous neighbour, or where that does not stand in
  the place, right before the next one, and where neither does, first. The
  answer of a document or an upload does not say where it landed, so the
  summary says what was asked. A place ordered automatically refuses a
  neighbour with `folderOrderAutomatic`.
- `docs folders edit --book` and `--no-book` send `isBook`; the two exclude
  each other. Marking arranges the book and every folder in it by hand, in the
  order they showed, and a folder made in a book or moved into one is arranged
  by hand as well; unmarking leaves every order as it is. A folder below a
  book, or one holding a book at any depth, the trash included, cannot become
  one, and a book or a folder holding one does not move into a book:
  `bookWithinABook`. Setting the mark a folder has already answers 200.
- `docs book <folderId>` is `GET files-folders/{id}/book`, for any folder of
  the book, and passes Minka's tree through as `data`: chapters
  `{id, title, order, entries, appUrl, kind}`, pages
  `{id, title, inNavigation, appUrl, kind}`, folders in use and published
  documents in use, no uploads, not paged. An entry of any other kind, which
  a later Minka may send, is left out of the text and the counts, and out of
  `docs export`; `--json` carries it as sent. A book in the archive is read; a
  folder in no book, and a book a client is not shown, is 404. The text view
  indents the tree, each line shaped as one of `docs list`, a chapter ending
  in a slash, and marks a chapter ordered
  automatically and a page out of the navigation.
- `docs edit --in-navigation` and `--not-in-navigation` send `inNavigation`;
  the two exclude each other. A document in no book takes it too, and a
  `notice` says it counts once the document is in one. A PATCH that carries
  only the navigation, or only the mark, writes only the flag: no title or
  body, no version, `updatedAt` as it was, and never 409 (from the Minka
  release that carries #494). So the guard of `docs edit` stands over the
  title and the body alone: the navigation alone asks for neither `--expect`
  nor `--force`, and sends `--expect` where it is given. A 409 over the title
  or body leaves the navigation as it was.
- A Minka that does not know books takes `isBook` and `inNavigation` with
  200 and writes neither, so the commands hold the answer against the
  request: a folder whose `isBook` is not what was asked, or a document
  without `inNavigation`, is exit 7 (`api_error`) with the answer as `data`,
  and the message says whether the rest of the change went through.
- `--book`, `--no-book`, `--in-navigation` and `--not-in-navigation` take no
  value: given as `=false`, which would read as not given at all, they are
  exit 1. A neighbour that is the entry itself is exit 1 as well: Minka takes
  the entry out of the order before it looks for the neighbour and would put
  it first without a word.
- The mark, the navigation and the order are the team's: a client gets 404,
  for `docs edit` with `inNavigation` also where the call changes the title or
  the body. The hints of these 404s say so.

Minka refuses the mark of a folder in this order: the folder (404), `title`
that is no string (400), `isBook` that is no boolean (400 `malformedBody`;
an explicit null 422 `isBookNotABoolean`), a client (404), `projectArchived`,
`folderArchived`, `bookWithinABook`, and only then the title. The navigation
of a document: the document (404), the title, the body, `expectedUpdatedAt`,
`inNavigation` that is no boolean (400 `malformedBody`; null 422
`inNavigationNotABoolean`), a client (404), and for an archived document
`projectArchived` and `documentArchived`. `PUT …/order`: the
body (400), the tool (404), `orderUnknown`, the folder (`folderUnknown`; an
archived one `projectArchived`, then `folderArchived`), `projectArchived`, a
client (404). A move with neighbours, after the target: `entryPlaceUnknown`,
`projectArchived`, `folderOrderAutomatic`, then `folderIntoItself` and, for a
folder that changes its place, `bookWithinABook`.

| Answer | Command | Exit |
|---|---|---|
| 404 | `docs book` for a folder in no book; `docs order`, `docs folders edit --book`/`--no-book` and `docs edit --in-navigation`/`--not-in-navigation` for a client | 2 |
| 422 `bookWithinABook` | `docs folders edit --book`, `docs folders move` | 9 |
| 422 `folderOrderAutomatic` | every `move --after`/`--before` into a place ordered automatically | 9 |
| 422 `folderUnknown`, `folderArchived` | `docs order --folder` | 9 |
| 422 `orderUnknown`, `entryPlaceUnknown`, `isBookNotABoolean`, `inNavigationNotABoolean`; 400 `malformedBody` | unreachable: the CLI checks the word and the IDs and sends booleans | — |
| 200 without the mark or the navigation | `docs folders edit --book`/`--no-book`, `docs edit --in-navigation`/`--not-in-navigation` against a Minka without books | 7 |

## Exporting a book or a folder

`docs export <folderId> --out <dir> [--format markdown|html] [--tool <toolId>]
[--force]` writes what stands in a folder into a directory, for an agent or a
reader who wants the files at hand ([C-135](../decisions.md#c-135)).

**Reading.** The folder is first asked for as a book (`GET
files-folders/{id}/book`), one request for the whole tree. A chapter is
exported with what lies below it; a reference to a page of the book outside
the chapter then keeps its address in Minka. A folder in no book answers 404,
and is walked through its tool with `--tool`, one `GET
files-tools/{toolId}/entries?folderId=` per folder, because Minka reads a
folder's entries only through its tool and has no address for a folder by
itself. Without `--tool` that is exit 2 with a hint that names it. Then every
page is read with `GET documents/{id}`, four at a time, and the files are
downloaded the same way with `GET uploads/{blobId}/original`. Nothing else
goes out. A page gone between the tree and its body (404) is left out, named
in `gone` and in a notice.

**What a book holds.** A book's tree carries no uploads and nothing in the
archive, so a book is exported as its readers read it: chapters and pages. A
walked folder downloads its uploads into the directory of their folder; the
listing of a folder leaves out the archived ones as well.

**The tree.** A folder is a directory, a page a file, an upload its file, and
every directory holds an index, `README.md` (`index.html` in HTML): the
name GitHub, GitLab and every file browser open first, and the one an agent
looks for. The index lists what stands in the directory and below it, in
Minka's order, as relative links; a page out of the navigation of its book is
marked `(not in navigation)`, an upload `(file)`. Its frontmatter names
`title`, `id`, `appUrl`, `kind` (`book`, `chapter` or `folder`), `order` and,
in a book, `book` and `bookId`.

**Names.** A name is the title as the slug of a heading's anchor without the
`h-` — lower case, umlauts spelled out, accents dropped, every run of other
characters one dash, cut at 80 bytes — so it holds only letters, digits and
dashes: no name is a path, starts with a dot, or differs from another in case
alone, and a reader can guess it from the title. A title without a letter or
digit is `untitled`. Within one directory the pages, folders and uploads share
the names, each taken once as a case-insensitive file system compares them;
`readme`, `index` and the device names of Windows are taken from the start.
The first entry in Minka's order gets the title, the next `-2`, `-3`. A name
an earlier run gave stays with its entry while its title still gives it and
nobody took it first, so a new page of the same title put in front takes the
next free name instead of moving the others. An upload keeps the extension of
its file, in lower case.

**A page.** In Markdown (the default) a page is a frontmatter (`title`, `id`,
`appUrl`, `inNavigation`, `updatedAt`), the title as `#`, and the body as
`docs show` writes it, with the shapes of a document as
[Rich text](rich-text.md#the-shapes-of-a-document) writes them. In HTML it
is the stored HTML in a minimal page: the same fields as `meta` elements and
`link rel="canonical"`, the title as `h1`, no script and no style.

- **Headings** carry the anchors Minka's page gives them (E-304), counted over
  every heading of the body in document order, as `search` names them. In
  Markdown the anchor is `<a id="h-…"></a>` on the line above the heading:
  GitHub (which prefixes and resolves it), VS Code, pandoc and the
  static site generators honour an `id` in raw HTML, while `{#h-…}` is read by
  a few of them only and the heading's own slug differs from Minka's. It is
  the form this repository's documentation uses for its own anchors. In HTML
  the anchor is the `id` of the heading.
- **Files hanging in a page** are downloaded beside it, into
  `<page>.assets/`, named after the file and counted on where two meet; a
  picture becomes an image, any other file a link, both with the caption or
  the file's name as words. The handle of a file is its ID, two dashes and a
  signature, and the ID is looked up in the page's `attachments`; a file the
  page no longer lists stays as `docs show` shows it. One directory per page
  keeps a page and its files together, and no name of an entry holds a dot, so
  no entry is ever called so.
- **References and links.** A reference to a page or an upload of the export,
  and a link to the page of a page, an upload or a folder of the export or to
  its `/go/item/…` on the host signed in to, becomes a relative link to its
  file — at the section a reference names in `anchor`, or a link in its
  `#h-…`. Anything else keeps its address in Minka: as `docs show` writes it in
  Markdown, as a link with the title in HTML, and a target the reader cannot
  open as Minka's words for it.

**Again into the same directory.** The directory keeps `.minka-export.json`:
the host, the folder, the format, every file the export wrote with the
SHA-256 of its bytes, the ID it stands for and, for a downloaded file, the
file of Minka; and the directories it made. A run holds a lock while it
works, so a second run into the same directory is refused at once. The lock
lies beside the config file, in `export-locks/`, named after the directory
as it really is, links resolved, so that two names of one directory meet the
same lock; it stays there, as every lock file of minka does, because taking
it away would let two runs hold a lock each. Nothing of a run that wrote
nothing is left in the directory. A run reads the tree and every
page again — the tree carries no `updatedAt`, and a page's file also changes
with the titles and names of the pages it links to — and writes a file only
where its bytes differ. A downloaded file whose file of Minka is the same and
whose bytes are those written is not fetched again. What the manifest names
and this run no longer writes is removed, and a directory of the export left
empty with it; one still holding something stays and is tried again next
time. A directory of the export in the place of a file of the plan — a
chapter whose name an upload without an extension takes — goes first, where
it holds nothing but files of the export that go. A run in the other format
replaces the files of the first.

A new file gets the mode any program gives one, `0666` less the umask, and a
new directory `0777` less the umask; a file written over keeps the mode it
has.

**What is never touched.** Everything is read and every path checked before
the first file is written, and a refusal writes nothing (exit 1):

- a path the run would write, or a directory it would make, where something
  stands that the manifest does not name — a file of somebody else is never
  written over, `--force` or not; the paths are in `data.paths`;
- a file of the manifest whose bytes are no longer those written, to be
  written over or removed — somebody changed it; `--force` writes over it or
  removes it;
- a directory that holds anything and no manifest — `--force` exports beside
  what is there, which stays untouched;
- a manifest of another folder or another host — each export has a directory
  of its own, since a run removes what its folder no longer holds;
- a manifest that names a path that is not a clean relative path inside the
  directory, or that this minka cannot read — nothing it names is touched.

A link is never followed: a file or directory of the manifest behind a link,
or no longer a file, is left alone and given up, and a file is written by
renaming a temporary file into its place, which replaces a link rather than
writing through it.

Once the checks are through, the manifest names every file of the plan
before the first is written: a file it knew keeps the hash it was written
with, a new one has none yet. A run that fails halfway — a download, the
disk — or is killed, leaves nothing the manifest does not name, and the next
run carries on, without `--force` where no such file landed. A file
without a hash is the export's while it is not there, or, a page or an
index, while its bytes are those the plan writes. Anything else there may be
the broken run's or a person's: the run is refused as for a file changed
since, and `--force` writes over it or removes it. Such a file stays in the
manifest while it lies there, and a directory in a file's way that holds one
stays. A file whose bytes are those the plan writes is no change, whatever
hash the manifest names. Where a manifest was there before, the next run
also removes the temporary files a broken run left in the directories of the
export. A run that fails removes nothing. A download that
breaks off is the network's failure (exit 6); one that cannot be written is
`config`.

**Output.** `data` names `out`, `folderId`, `title`, `isBook`, `book`,
`format`, the counts of `folders`, `documents`, `uploads` and `assets`, the
paths `written` and `removed`, the count `unchanged`, `gone` and `manifest`;
the summary says what was exported where and the three counts, the text adds
every path written (`+`) or removed (`-`) and the index to start at.
Breadcrumbs: the same export again, `docs book`, `docs show`. A file that
cannot be written is `config`, exit 1; a refusal, the lock held included,
`usage`, exit 1.

## What a list promises

Nothing here is paged and nothing is silently cut, so a short list is the whole
list. `entries` reaches one folder deep: going down means calling again with a
folder. The order is the web's own, neither alphabetical nor by date, set by
hand or automatic, and the CLI passes it through. `lead` is the first 240 characters of the body
without markup, cut where the count runs out and not at a word.

## Searching a tool or a folder

`search <term>...` asks `GET /api/v1/search` within a Files tool (`--tool`), a
folder and everything beneath it (`--folder`), or both, the way the web's search
finds entries (E-304). The words of the command line are one term, joined by a
space, and go out as they are: Minka reads phrases and operators in it and cuts
it at 200 characters, which a notice says where the term is longer. Every hit
carries the heading its match stands under and an `appUrl` that ends in that
heading's anchor; the CLI shows the address and builds nothing from the anchor.
An entry in the archive, or below an archived folder, is found as on the web
and carries `archived: true`, which the text view marks as `docs list` does.

Checked here, before any request ([C-132](../decisions.md#c-132)): a term at
all, `--tool` or `--folder` (Minka would answer `searchScopeMissing` with exit 9
instead of 1), the IDs, `--sort` as `best` or `recent` (Minka takes any other
word as `best` without saying so), `--limit` of at least 1, and `--offset` as a
whole number of at most six digits. What remains for Minka is the scope itself:
a tool that is no Files tool, a folder or tool that is not there or cannot be
read, and a folder outside the tool named are the plain 404, exit 2.

Minka serves 25 hits a page and takes no size. `--limit` (25 unless given) is
reached with as few pages as it takes: a page is asked for only while hits are
lacking, so a limit within one page costs one request and no hit costs one of
its own. The offset counts hits, so a run that stops inside a page names the
first hit it left behind as `next` (the offset of that page and the hits taken
from it); a run that took a whole page passes Minka's `next` on. `data` is
`{hits, next}`, each hit the JSON Minka sent, `next` a string or `null`; the
breadcrumb `next` and a notice, as on the other paged commands, give the whole
command with `--offset`. The term stands first, so a flag an agent appends,
`--json` above all, stays a flag. Only a term starting with a dash, Minka's
NOT, goes last behind `--`, where it is not read as a flag; every flag then has
to stand before the `--`. Minka takes no offset of more than six digits, so no
hit behind 999,999 is reached. Pages are read one after another, so a hit that
moves while somebody writes can be met twice or not at all across the border. A
failure on a later page fails the run, and the hits read before it are not
shown. An answer the run cannot go on from is `unexpectedResponse`, exit 7: a
hit that is no object, `null` included, or a `next` that is no number of hits,
an empty one included, which would start over. A page without hits ends the run
whatever `next` it names.
