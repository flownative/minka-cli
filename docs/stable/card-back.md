# The back of a card

The **back** of a card is the working memory of whoever works on it, agents
above all: the plan, the intermediate state, the handover. It holds one text,
the **notes**, and files beside it, the **attachments**. The front — the
description and the comments — is the assignment and the conversation, for
people. `cards notes show` and `set` read and write the notes,
`cards notes attachments list`, `create`, `show`, `download` and `delete` the
files ([C-124](../decisions.md#c-124)). How the notes themselves travel is in
[Rich text in both directions](rich-text.md) (C-042).

Everything here was read in Minka's code — `CardNotesApiController`, `CardBackFilesApiController`,
`CardBackFileResource`, `AttachToCardBackHandler`,
`RemoveFromCardBackHandler`, `DbalCardBackFileRepository`,
`CardBackFileQueryService`, `BlobDelivery` — and in `ApiCardBackFilesTest` and
`CardBackPermissionsTest`, E-189, E-275 and E-276.

## The rule for the notes

Minka's E-276 says how a back is kept, and the CLI says it where the commands
are: in the help of `cards notes` and in the skill. The notes are written in
the language of the card's front; they state where the work stands and are
rewritten, not appended to; they are written at the milestones; about 40 to 60
lines are a guide, not a limit; they follow the outline Status · Plan
(packages with their state) · Settled (by whom, when) · Where things are
(branch, worktree, migration, numbers, pull requests) · Next step · Pitfalls;
they stay when the card is done. What still holds after the card is done goes
into the repository before the card reaches Done. Every attachment is named in
the notes with one line of what it holds.

Who writes a back is a rule of how agents work on a board, not of the tool:
the skill says that the agent the card is assigned to writes it, and that any
other agent leaves it alone and says on the front what it would have written
(C-145).

## What Minka refuses in the notes

Minka refuses, in this order, control characters other than line break and tab
(`cardNotesHasControlCharacters`), invisible format characters
(`cardNotesHasInvisibleCharacters`) and more than 100,000 characters
(`cardNotesTooLong`). Invisible are the Unicode format characters but a single
zero-width joiner or non-joiner, supplement variation selectors, and runs of
two or more joiners or of two or more variation selectors. The CLI gives the
invisible ones a hint that names them and checks only the length before
sending, which a text with both refusals therefore hears first; the byte
order mark editors put in front of a file is dropped from stdin before
(C-152).

## Only the team

The whole back is the team's (E-189, E-275): a client of the
project gets 404 for the notes, every address of the attachments and the
bytes, also of a card whose front it sees. Every command of the back says so
in the hint of its exit 2, because "not found" for a card one has just read is
otherwise a riddle.

## What an attachment is

An attachment is an upload of the card's project on the back, carried as its
newest version: `id`, `cardId`, `filename`, `version` (1 for the first),
`contentType`, `byteSize`, `uploadedAt`, `uploadedBy`, `content` (the address
of its bytes) and `appUrl` (its address on the card page). A file is one name
on its back, as Minka compares it, case and all: an upload under a name the
back has already becomes the next version under the same `id`, and the earlier
versions are kept until the file is removed but neither listed nor served. A
version names no upload, so an earlier one cannot be had through the API.

`GET cards/{cardId}/attachments` lists them by name, folded to lowercase
first. A card in the archive of its board is read like one on its board; one
in the trash is 404. Neither the card nor its notes say whether a back has
files, so `cards notes show` points to them only in its breadcrumbs.

An attachment is named by its `id`, its position in the list from 1 or its
file name; a number is always the position. Since Minka tells names apart by
case, the name as written is taken first; only where none is written so is
case ignored, and then it has to leave one file, or the command exits 8
(`ambiguous`). A step, whose texts Minka does not keep apart, is matched
without regard to case alone. An ID goes to Minka as it is, anything else is
looked up in the list first, as a step is ([C-123](../decisions.md#c-123)).

## Putting a file on the back

Minka takes it in two requests, as a Docs & Files upload
([C-085](../decisions.md#c-085)): `POST projects/{projectId}/uploads` stores
the bytes, `POST cards/{cardId}/attachments` with `{"upload": "<id>"}` puts
them on the back, 201 with the attachment. `cards notes attachments create`
does both. Before the bytes travel it reads the card, for its project and
whether it lies in the archive, and then the back, which a client may not
read, in the order Minka asks them: a back one may not see exits 2 whatever
the card's state, and a card in the archive exits 9 with `cardArchived`.
Neither costs a transfer or leaves a stored file standing in nothing for days.
A step that fails ends the run.

Where the file was stored and the second step was refused, the error carries
the stored upload as `data`. Where the card was the reason, the hint names
`--blob <blobId>`, which sends the second step alone. Where Minka refused the
upload itself with `uploadNotAttachable`, something took it between the two
steps, and naming it again cannot help: the hint says that running the
command again stores the file anew.

Minka checks in this order:

1. The card: not there, in the trash, or the person no member or a client of
   the project — 404.
2. The body: `upload` that is no string — 400 `malformedBody`; missing,
   `null` or no UUID — 422 `uploadMissing`.
3. One write that takes the card and the upload together. Where it took
   nothing, the card is asked first: an archived project 422
   `projectArchived`, a card in the archive 422 `cardArchived`, a card whose
   column or board is out of use 422 `cardTakesNoChanges`. Only then the
   upload: one of somebody else, of another project, swept, or held by
   anything — a text, an upload or a version of Docs & Files, another back —
   is 422 `uploadNotAttachable`, one code for all of them.

## Reading an attachment

`GET …/attachments/{fileId}/content` hands out the bytes of the newest version
as stored: a picture, audio or video in its own type, everything else,
Markdown included, as `application/octet-stream`. A picture shown where it
stands may be answered with a redirect to the store, which the client
follows as for `uploads download`, the token left behind. The CLI sends no
`?download`, which changes only the headers ([C-121](../decisions.md#c-121)).
Minka re-encodes a picture when it is stored, so the bytes of a picture that
come back are not the ones sent; `byteSize` says what comes.

`cards notes attachments show` prints a text and nothing else: alone on
standard output and byte for byte, the word about it on standard error. That
holds in text output, on a terminal or with `--styled`; piped without it, and
with `--json`, the text is `data.text` of the envelope beside `data.file`, so
a file on disk is what `download` is for. Since the served type says nothing
about a text, the bytes decide: UTF-8 without a zero byte is a text. A stored
type of a picture, audio or video, in any case, is refused before the bytes
are fetched; anything else is judged on its first 4096 bytes before the rest
is read, and whole once it has come. In text output a control character other
than tab, line feed and carriage return is refused as well, because a
terminal acts on it; the envelope writes it out escaped. Every such refusal
exits 1, prints nothing and names `download`. Bytes that are not as many as
the attachment says are refused as well, and nothing is printed.

`cards notes attachments download` writes any attachment to disk the way
`uploads download` does: under its name, never over a file that is there
without `--force`, `-o -` to standard output alone, readable by its owner
alone. A target that would be refused is refused before the bytes are
fetched.

## Taking a file off

`DELETE …/attachments/{fileId}` takes the file with every version, and Minka
discards their uploads in the same change (204): nothing of it can be fetched
afterwards. The file is looked up first, so one that is not there is 404 even
on an archived card; then an archived project is 422 `projectArchived` and a
card in the archive 422 `cardArchived`.

## What the fake does not mirror

The fake keeps a file's versions and finds the bytes of the newest by ID; it
does not carry the uploads of a back into another project with a moved card,
has no column or board out of use and so never answers `cardTakesNoChanges`,
re-encodes no picture, and serves no ranges.
