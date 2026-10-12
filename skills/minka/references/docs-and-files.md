# Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body; an **upload** is a file standing on its own,
with a description. Both take comments, and the comments take reactions.

## Commands

```bash
minka docs tools <projectId> --json
minka docs tool <toolId> --json
minka docs list <toolId> [--folder <folderId>] --json
minka docs show <documentId> --json
minka docs create --tool <toolId> --title "Title" --audience everybody|chosen|nobody [--notify <person>]... [--folder <folderId>] [--body - < body.md] --json
minka docs edit <documentId> [--title "New"] [--body - < body.md] (--expect <updatedAt> | --force) --json
minka docs versions list <documentId> --json
minka docs versions show <documentId> <versionId> --json
minka docs uploads show <uploadId> --json
minka docs uploads create <file> --tool <toolId> [--folder <folderId>] [--title "Title"] [--description - < about.md] --json
minka docs uploads create --blob <blobId> --tool <toolId> [--folder <folderId>] [--title "Title"] --json
minka docs uploads edit <uploadId> [--title "New"] [--description - < about.md] (--expect <updatedAt> | --force) --json
minka docs uploads versions list <uploadId> --json
minka docs uploads versions create <uploadId> (<file> | --blob <blobId>) [--expect <updatedAt>] --json
minka docs comments create (--document <documentId> | --upload <uploadId>) - --json < comment.md
minka docs comments edit (--document <documentId> | --upload <uploadId>) <commentId> - [--format html] --json < comment.md
minka docs comments delete (--document <documentId> | --upload <uploadId>) <commentId> --json
minka docs comments react (--document <documentId> | --upload <uploadId>) <commentId> 👍 --json
minka docs comments unreact (--document <documentId> | --upload <uploadId>) <commentId> 👍 --json
```

## Reading

- `docs tools <projectId>` lists the tools of a project; a project can have
  more than one. `docs tool <toolId>` gives one tool by its ID alone as `{id,
  projectId, project {id, name}, title, appUrl}`.
- `docs list <toolId>` lists what stands at the top, `--folder <folderId>`
  what stands in a folder. It reaches **one folder deep**: to go further, call
  it again with a folder of the answer. Nothing is paged and nothing is cut,
  so a short list is the whole list.
- The order is the one the web shows: not alphabetical, not by date. Every
  document carries a `lead`, the first 240 characters of its body without
  markup, cut where the count runs out and not at a word.
- Drafts and entries in the archive are not listed, and neither is a folder in
  the archive. The inside of an archived folder **can** be listed by naming it,
  and then the answer and every line of it says `archived: true`.
- `docs show <documentId>` gives the document with its body, its files and its
  comments. `docs uploads show <uploadId>` gives an upload the same way, with
  its `description` and `descriptionSource`. A
  **draft is at no address**, its author included: exit 2.
- The bytes of an upload are fetched with `uploads download <blobId>`, where
  `<blobId>` is `file.id` of the upload — not the upload's own ID, which names
  the entry.

## Writing a document

- `docs create --tool <toolId> --title "..." --audience ...` writes a
  **published** document; there are no drafts here. `--folder` puts it in a
  folder, and without `--body` it is a title and nothing else.
- `--audience` is required: `everybody`, `chosen` with one `--notify <person>`
  per person, or `nobody`. A word that is none of the three exits 1 before a
  request goes out; which people go with which audience is Minka's rule and the
  CLI keeps none of its own. `--notify` under any audience but `chosen` is exit 9
  `notifyIdsWithoutChosenAudience`, and `chosen` naming nobody is exit 9
  `notifyIdsMissing`. Both are **your input to change** — name people, or choose
  the audience that takes them — and the `hint` says which flag. `--notify`
  takes an ID, `me`, an e-mail address or a name; a name or an address is
  looked up among the members of the project the tool stands in.
- **`personCannotSeeIt` is not your input to fix.** The person you named exists,
  is a member of the project, and simply may not be told: they work with the
  project as a client, or are an agent working for a client. Such a person is
  told only about a document in a folder shown to clients, in a tool shown to
  clients, and only the topmost folder above the document counts. Nothing was
  written, and **sending the same request again will be refused again, however
  often you try**. The only thing that gets the document written is to **name
  somebody else**; which tools and folders clients are shown is decided in
  Minka on the web. The people are met one at a time in the order you gave them,
  and of one person `personNotAProjectMember` is answered before this, so a list
  holding both is answered for whichever name comes first.
- A person named more than once is told once.
- `docs edit <documentId>` changes the title, the body or both. `--body ""`
  empties the body. **`--title ""` does not empty the title:** a document needs
  one, so it exits 9 with `documentTitleMissing`. A title can only be replaced.

## Putting a file into a tool

- `docs uploads create <file> --tool <toolId>` puts a file of this machine on
  top of the tool, or of `--folder <folderId>`. Minka takes it in two requests
  — the file is stored in the project of the tool, then put into the tool — and
  the command does both. It reads the tool for its project first, so a tool you
  cannot open exits 2 **before the file travels**. `data` is the upload, as
  `docs uploads show` gives it. You need no `--project`: an old script that
  still gives one keeps working where it names the tool's project, gets a
  `notice` that it is no longer needed, and exits 2 where it names another.
- A step that fails ends the run, and nothing after it goes out. Where the file
  was stored and the second step was refused, the error's `data` is the stored
  file and the `hint` names `--blob <blobId>`: **try again with `--blob`
  instead of the file**, and it does not travel a second time.
- `docs uploads create --blob <blobId> --tool <toolId>` puts in a file stored
  earlier — by `uploads create --project` or by a run that failed at the
  second step. `--project`, `--name` and `--content-type` belong to storing a
  file and exit 1 beside `--blob`.
- Without `--title` the upload is named after the file. **`--title ""` exits 1
  before anything goes out**, because Minka would refuse it only after the file
  travelled.
- `--description` is Markdown converted here, as the body of a document is.
  Somebody `@`-mentioned in it is told; nobody else is, and there is no
  `--audience` or `--notify` for an upload. You are subscribed to your own
  upload.
- **`fileUnknown` tells no cause apart.** The file may have been swept after
  standing in nothing, never stored, be of another project, or be somebody
  else's. With `--blob` the one remedy worth trying is to store it again:
  give the file itself in place of `--blob`. Where the same run stored it a
  moment ago it cannot have been swept, and the `hint` says so instead.
- **`fileInUse`**: the file of yours stands in an upload or a text already. A
  file goes into one place; give the file itself to store it anew.

## Changing an upload

- `docs uploads edit <uploadId>` changes the title, the description or both,
  guarded as `docs edit` is: `--expect <updatedAt>` or `--force`, a conflict
  exits 9 with `uploadChanged` and the upload in `data`, and the guard is blind
  for one second.
- `--description ""` empties the description. **`--title ""` does not empty the
  title**: exit 9 `uploadTitleMissing`.
- **`docs uploads edit` never changes the file.** Minka ignores a file sent
  with a change, so the command has no flag for it. Another file goes in with
  `docs uploads versions create`, below.
- The description is held against the files and mentions it carries, as a
  document's body is: a Markdown description that would drop them exits 1
  unless `--drop-attachments` is given. Send the `descriptionSource` back with
  `--description-format html` to keep them. People newly mentioned are told.

## Versions

- `docs versions list <documentId>` lists every version of a document, oldest
  first, with its `number`, its `author` and `writtenAt`; the newest is
  `current`. `author` is `null` for a version written before Minka kept who
  wrote it. `docs versions show <documentId> <versionId>` gives one version
  with its `title`, its `body` (Markdown in text, the stored HTML in JSON) and
  the files the body names. **A file in a version's body shows only in that
  list**: the stored body names it by no more than a handle, so it reads as
  `[attachment]` there.
- `docs uploads versions list <uploadId>` lists every file an upload showed,
  oldest first; `current` is the one it shows now. An earlier file is fetched
  with `uploads download <blobId>`, `<blobId>` being `file.id` of its line.
- **The versions are the team's alone.** A client of the project gets exit 2,
  also for a document or upload it is shown, and so does an earlier file it
  did not store itself.
- Nothing is paged. A document in the archive, below an archived folder or in
  an archived project still has its versions read. Making an earlier version
  of a document the current one is done in Minka on the web.
- `docs uploads versions create <uploadId> <file>` **replaces the file of an
  upload**: title and description stay, and the file it showed stays as an
  earlier version. It takes the two requests of `docs uploads create`: it reads
  the upload and its versions first, so an upload you cannot reach — or reach
  only as a client — exits 2 **before the file travels**, and one in the
  archive or below an archived folder exits 9 there. Then it stores the file in
  the upload's project and gives it to the upload. A refused second step hands
  over the stored file in `data`, and `--blob <blobId>` takes that step alone.
  **On a 409 `uploadChanged` `data` is the upload as it stands instead**, and
  the stored file is named only in the `hint`, with its `--blob`.
- **An earlier file of the same upload is `fileInUse`.** To give an upload an
  earlier file back, download it and give it as a file, which stores its bytes
  anew. `--blob` naming the file the upload shows already writes nothing: exit
  0 with a `notice` saying so.
- `--expect <updatedAt>` is optional here: without it Minka still never writes
  over a title or description changed in the meantime, and nothing is lost,
  because the file the upload showed stays as a version. With it a change since
  your read exits 9 `uploadChanged` with the upload in `data`, blind for one
  second as every guard on an entry.

## The body, and the trap under it

**Minka refuses no body.** A body that carries no element it serves — raw
Markdown, plain text — is not an error: it is escaped and kept as the
characters it consists of, and the answer is the same 201 or 200 as any other.
`# Heading` then stands in the document as `# Heading`, and only the web ever
shows it. The same holds for the description of an upload.

So `--body` is **Markdown by default and converted here**, exactly as a card's
description is. Send HTML yourself only with `--body-format html`, and then the
command's `notice` is the only warning you will ever get.

## Writing over a document somebody else may have written

- `docs edit` needs `--expect <updatedAt>` — the `updatedAt` you read — or
  `--force`, and the two exclude each other. A document written over since is
  then left as it stands: exit 9, `meta.apiCode` `documentChanged`, and `data`
  holds the document as it stands now. Merge your text into its `body` and send
  it again with its `updatedAt`.
- **`--expect none` is not a value here.** It belongs to the notes of a card,
  where it means "no notes yet". On a document `expectedUpdatedAt: null` would
  mean "no expectation at all", which is the opposite, so the CLI refuses the
  word and sends no such field.
- **The guard is blind for one second.** Minka keeps the moment of a document
  and of an upload to the whole second. A change somebody else made **in the
  same second as your read** carries the moment you read and passes the guard.
  So after a write that matters, **read it back** with `docs show` or `docs
  uploads show` and check that it says what you wrote. There is no flag for this and no
  answer that mentions it.
- Send the `updatedAt` back **exactly as you read it**. It always ends
  `.000000Z`. A value that is no moment of that shape — one without a zone
  among them — never leaves the CLI: it exits 1 (`usage`) before a request
  goes out.

## Comments

- `docs comments create|edit|delete|react|unreact` take
  `--document <documentId>` **or** `--upload <uploadId>`, exactly one of the
  two. The two kinds have addresses of their own, and the identifier of one is
  nothing at the address of the other, so the flag says which it is.
- The text is Markdown by default, as for a card. Only the author can change or
  remove a comment; somebody else's answers exit 2.
- A document or an upload inside an archived folder shows `archived: true`,
  and its comments take no write: a new one, a change, a removal, a reaction
  and taking one back exit 9 with `documentArchived` or `uploadArchived`. An
  older Minka (v0.41.0) still changes and removes **your own comment** there,
  so `docs comments edit` refuses a text that drops files or mentions unless
  `--drop-attachments` is given, and a newer one then answers exit 9.
- Reactions live on the comments alone. A document and an upload themselves
  take none through the API, although the web offers it.

## What does not exist

There is no delete, trash, archive, unarchive or move of a document; no
creating, renaming or deleting a folder, so the CLI writes only into folders
that already exist; no reading of a tool's archive; no reading of a draft, by
anybody; no replacing the file of an upload, nor deleting, archiving or moving
one. All of it is done in Minka on the web.

## Codes of this part

`documentTitleMissing`, `documentTitleTooLong`,
`documentTitleHasControlCharacters`, `documentBodyTooLong`,
`documentBodyNotReadable`, `documentAudienceMissing`, `personNotAProjectMember`,
`personCannotSeeIt` (a person to replace, not input to fix), `notifyIdsMissing`,
`notifyIdsWithoutChosenAudience`,
`folderUnknown`, `folderArchived`, `documentArchived`, `uploadArchived`,
`documentChanged` (409, with the document in `data`), `uploadTitleMissing`,
`uploadTitleTooLong`, `uploadTitleHasControlCharacters`,
`uploadDescriptionTooLong`, `uploadDescriptionNotReadable`, `uploadChanged`
(409, with the upload in `data`), `fileUnknown`, `fileInUse`, the codes of
storing a file (`contentTypeMismatch`, `tooLarge`), and the comment codes a
card knows. An entry in the archive, or one in a folder in the archive, can be
read and takes no write: exit 9 with `documentArchived` or `uploadArchived`.
