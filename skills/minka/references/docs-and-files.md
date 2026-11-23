# Docs & Files

A Docs & Files tool holds folders, documents and uploads. A **document** is a
page with a title and a body; an **upload** is a file standing on its own,
with a description. Both take comments, and the comments take reactions. All
three go into the trash and the archive, come back and move.

## Commands

```bash
minka docs tools <projectId> --json
minka docs tool <toolId> --json
minka docs list <toolId> [--folder <folderId>] --json
minka docs list <toolId> --archived --json
minka docs show <documentId> --json
minka search <term>... (--tool <toolId> | --folder <folderId> | both) [--sort best|recent] [--limit N] [--offset <next>] --json
minka docs create --tool <toolId> --title "Title" --audience everybody|chosen|nobody [--notify <person>]... [--folder <folderId>] [--body - < body.md] --json
minka docs edit <documentId> [--title "New"] [--body - < body.md] (--expect <updatedAt> | --force) --json
minka docs edit <documentId> (--in-navigation | --not-in-navigation) --json
minka docs book <folderId> --json
minka docs export <folderId> --out <dir> [--format markdown|html] [--read-all] [--tool <toolId>] [--force] --json
minka docs order <toolId> automatic|manual [--folder <folderId>] --json
minka docs versions list <documentId> --json
minka docs versions show <documentId> <versionId> --json
minka docs versions restore <documentId> <versionId> --json
minka docs delete <documentId> --json
minka docs restore <documentId> --json
minka docs archive <documentId> --json
minka docs unarchive <documentId> --json
minka docs move <documentId> (--folder <folderId> | --top-level) [--tool <toolId>] [--after <entryId> | --before <entryId>] --json
minka docs folders show <folderId> --json
minka docs folders create --tool <toolId> --title "Title" [--folder <folderId>] [--visible-to-clients] --json
minka docs folders edit <folderId> [--title "New"] [--book | --no-book] --json
minka docs folders delete <folderId> --json
minka docs folders restore <folderId> --json
minka docs folders archive <folderId> --json
minka docs folders unarchive <folderId> --json
minka docs folders move <folderId> (--folder <folderId> | --top-level) [--tool <toolId>] [--after <entryId> | --before <entryId>] --json
minka docs uploads show <uploadId> --json
minka docs uploads create <file> --tool <toolId> [--folder <folderId>] [--title "Title"] [--description - < about.md] --json
minka docs uploads create --blob <blobId> --tool <toolId> [--folder <folderId>] [--title "Title"] --json
minka docs uploads edit <uploadId> [--title "New"] [--description - < about.md] (--expect <updatedAt> | --force) --json
minka docs uploads versions list <uploadId> --json
minka docs uploads versions create <uploadId> (<file> | --blob <blobId>) [--expect <updatedAt>] --json
minka docs uploads delete <uploadId> --json
minka docs uploads restore <uploadId> --json
minka docs uploads archive <uploadId> --json
minka docs uploads unarchive <uploadId> --json
minka docs uploads move <uploadId> (--folder <folderId> | --top-level) [--tool <toolId>] [--after <entryId> | --before <entryId>] --json
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
  comments. `--body` prints only the body as Markdown, as `docs edit --body`
  takes it back. A decisions block shows as its marker, its rows read out in
  a section after the body, which never go back as text; inserting one is in
  [texts.md](texts.md#decisions-blocks). `Locked: yes` (`locked` in JSON) is a
  document that takes no edit until it is unlocked on the web. `docs uploads show <uploadId>` gives an upload the same way, with
  its `description` and `descriptionSource`. A
  **draft is at no address**, its author included: exit 2.
- The bytes of an upload are fetched with `uploads download <blobId>`, where
  `<blobId>` is `file.id` of the upload — not the upload's own ID, which names
  the entry.

## Searching a tool or a folder

`minka search <term> --tool <toolId>` finds the documents and uploads of a
tool by what they say, `--folder <folderId>` those of a folder and everything
beneath it. Use it rather than walking `docs list` and reading every document
when you look for a passage of a long text, such as a chapter of a handbook.
`minka docs tools <projectId> --json` gives the IDs of the tools of a project,
`minka docs list <toolId> --json` the folders. A document hit names the
heading of its match in `section`, and its `appUrl` opens the document there;
in the body of `docs show`, the section is the heading named in
`section.heading`. Everything else about the search, across Minka as well:
[search.md](search.md).

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
- A body can hold callouts, Mermaid diagrams, link tiles and a block of
  variables, each in a Markdown convention of its own: see "The shapes of a
  document" in the reference on texts.
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
  an archived project still has its versions read.
- `docs versions restore <documentId> <versionId>` writes what an earlier
  version said, title and body, as the **newest** version and answers with the
  document. Nothing is removed and nobody is told. A version that says what the
  document says already exits 0 and adds no version. The team's alone, as
  reading the versions is; a document in the archive exits 9
  `documentArchived`.
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

## Keeping a tool in order

Each kind has the same five writes in its own group — `docs …` for a
document, `docs uploads …` for an upload, `docs folders …` for a folder — so
an ID is never read as the wrong kind:

- **`delete`** puts it into the trash of the project, from the tool or from its
  archive; a folder takes everything in it along. It then reads like one that
  never was (exit 2). `data` is `{id, deleted: true}`.
- **`restore`** brings back what went into the trash **by itself**, to where it
  stood, and answers with it. What went in with its folder comes back with the
  folder, and one whose folder lies in the trash or the archive comes back once
  the folder does: exit 2. `trash list <projectId>` shows `restorable` and
  `blockedBy` for each line, and a draft (listed to its author) comes back on
  the web only.
- **`archive`** puts it into the archive of its tool; `unarchive` brings it
  back. Already archived is exit 9 `documentArchived`, `uploadArchived` or
  `folderArchived`; not archived is `documentNotArchived`, `uploadNotArchived`
  or `folderNotArchived`. **One archived together with a folder above it comes
  back only with that folder**: `unarchive` exits 9 `folderArchived`, and
  `docs list <toolId> --archived` marks it `archivedWithAFolderAbove`.
- **`move`** takes `--folder <folderId>` or `--top-level`, exactly one; it lands
  first among its kind in a place ordered automatically and last in one
  arranged by hand (see "Books and the order of a place"). Nothing in the
  archive moves and nothing goes into an archived folder (`folderArchived`),
  and a folder cannot go into itself or one below it (`folderIntoItself`).
  Where it stands already, nothing is written and the answer is the same.
- **`move --tool <toolId>`** moves it into another Docs & Files tool, also of
  another project: `--folder` then names a folder of **that** tool
  (`folderUnknown` otherwise), `--top-level` its top. It keeps its ID and goes
  with everything inside it, its versions and comments, and lands as above.
  There it is **hidden from clients** until someone shows it on the web,
  whoever is not a member of that project is taken off it, and **labels that
  belong to neither that project nor that tool come off**; a `notice` says so. Only the
  team of both projects may: a tool out of reach is exit 9 `toolUnknown`, a
  client of the entry's project exit 2. Across projects, one that shares a file
  with something that stays is exit 9 `entrySharesFiles`: it moves within its
  project only, and Minka does not say which file. `--after`/`--before` do not
  go with `--tool` (exit 1). A Minka that does not move between tools yet
  answers every folder of another tool with `folderUnknown`, and with
  `--top-level` moves the entry to the top of its own tool: exit 7, the answer
  in `data`; move it back with `--folder <its old folder>`.
- What stands **below an archived folder** is not put into the trash on its
  own: `delete` exits 9 `folderArchived`. Bring the folder back, or put the
  folder itself away.
- **Who may:** the team may do all of it. A client of the project puts into
  the trash and the archive **only what they made**, may make, rename and move,
  and **brings nothing back** — no `restore`, no `unarchive`, no version: each
  answers exit 2, as for something that is not there.
- `docs folders show <folderId>` reads a folder by its ID alone, without its
  tool: `data` as `docs list --folder` reads it, with `projectId`, `project`,
  `parent` (the folder it stands in, `null` at the top) and `updatedAt` (its
  last rename). An older Minka has no such address: exit 2, and the hint names
  `docs list <toolId> --folder`.
- `docs folders create --tool <toolId> --title …` makes a folder at the top or
  inside `--folder`; `data` is the folder as `docs list --folder` reads it, its
  ID under `folder.id`. `--visible-to-clients` counts only at the top of the
  tool and only from the team; a `notice` says so where `--folder` is given,
  and no answer says whether it counted. `docs folders edit <folderId> --title`
  renames one; `--title ""` exits 9 `folderTitleMissing`.
- `docs list <toolId> --archived` lists the archive of the tool from any
  folder, the one archived last first, as `{id, projectId, kind, title,
  archivedAt, archivedBy, archivedWithAFolderAbove, appUrl}`. `kind` is
  `folder`, `document` or `upload`. Not paged; not with `--folder`.
- A write in an archived project exits 9 `projectArchived`, a `move` to where
  the entry stands already and one into an archived folder included.

## Books and the order of a place

- **Each place orders itself.** The top of a tool and every folder are either
  `automatic` — folders, then documents, then uploads, each newest first — or
  `manual`, the three kinds mixed in an order set by hand. `docs list` says
  `order`, and `entries` (`[{kind, id}]`) is the order shown; arranged by hand,
  the text lists one mixed list instead of three.
- `docs order <toolId> manual [--folder <folderId>]` arranges a place by hand,
  keeping the order it showed; `automatic` brings the automatic order back
  unchanged. A folder does not inherit the order of the one above it. The
  team's alone: a client exits 2.
- `docs move <id> --folder <folderId> --after <entryId>` (or `--before`)
  places a document, upload or folder next to an entry of that place; name the
  place where it stands to rearrange it there. `--after` and `--before`
  exclude each other. A place ordered automatically refuses it: exit 9
  `folderOrderAutomatic`. The entry itself as its own neighbour is exit 1. A
  neighbour that does not stand there counts as none, and the entry then goes
  first. Without a neighbour, a move into a place
  arranged by hand appends at the end, and so does a new entry made there.
- **A book** is a folder marked with `docs folders edit <folderId> --book`
  (`--no-book` takes the mark away and leaves every order as it is). Marking
  arranges the book and every folder in it by hand. A folder below a book, or
  one holding a book (the trash counts), cannot become one, and a book, or a
  folder holding one, does not move into a book: exit 9 `bookWithinABook`. The
  team's alone. `--book` and the other switches here take no `=false`.
- A Minka that does not know books answers the mark and the navigation with
  200 and writes neither: the command then exits 7 (`api_error`) with the
  answer as `data`, and says whether the rest of the change went through.
- `docs book <folderId>` reads the book a folder belongs to — itself or the
  one above it — as one tree: `{id, title, tool, order, entries, appUrl}`,
  each entry a `chapter` (`{id, title, order, entries, appUrl}`) or a `page`
  (`{id, title, inNavigation, appUrl}`). No bodies, no uploads, nothing in the
  archive. A folder in no book exits 2.
- `docs edit <documentId> --not-in-navigation` leaves a page out of the
  navigation of its book (still reached by link, reference and search; previous
  and next pass it over), `--in-navigation` lists it again. Alone it writes
  only the flag — no title or body, no version, `updatedAt` as it was, never
  409 — so it needs neither `--expect` nor `--force`; `--expect` given is sent
  as it is. A client
  exits 2 for the whole call. A document in no book takes it as well and a
  `notice` says it counts once the document is in one. `docs show` names the
  `book` and `inNavigation`.

## Reading a book as files

To read a whole book — a handbook, a runbook — or any folder, or to grep
through it, export it once and read the files:

```bash
minka docs export <folderId> --out handbook --json
```

- The folder can be a book, a chapter of one (then only what lies below), or
  any other folder. The folder and its whole tree are read in two requests,
  and every page in one more, four at a time.
- `--tool <toolId>` is needed only against an older Minka without the tree of
  a folder: there a folder in no book exits 2 without it, and a book is read
  without its uploads.
- `handbook/README.md` is the table of contents: every folder, page and upload
  in Minka's order, as relative links, a page left out of the navigation
  marked `(not in navigation)`, an upload `(file)`. Every directory has its
  own `README.md`. Start there.
- A chapter is a directory, a page `<name>.md`: the name is the title in lower
  case, umlauts spelled out, other characters a dash (`Über Größe` →
  `ueber-groesse.md`); two pages of one title are `tools.md` and
  `tools-2.md` in the book's order. Each page starts with a frontmatter:
  `title`, `id` (what `docs show` and `docs edit` take), `appUrl`,
  `inNavigation`, `updatedAt`.
- Every heading has the anchor Minka's page gives it, on the line above:
  `<a id="h-zugang-pruefen"></a>`. A link to a section of another page of the
  book is a relative link to that anchor (`../setup/tools.md#h-hammer`), so
  following it lands on the heading; a reference to anything outside the
  export keeps its Minka address.
- Pictures and files of a page are downloaded beside it, into
  `<name>.assets/`, and linked from the page. An upload is downloaded into the
  directory of its folder, under its title and with its file's extension.
- A decisions block is its marker with the rows right below it; a page with one is read again on every run, since its rows
  change without it.
- `--format html` writes the stored HTML in minimal pages instead, the anchor
  as the heading's `id`, `index.html` as the index.
- **Refresh by running the same command again.** It reads again only the
  pages Minka changed since (and those that refer to something outside the
  export), renders every page anew, so a link follows a page renamed or
  moved, writes only files whose content changed, removes the files of pages
  gone from Minka (and their emptied directories), and keeps a page's name
  while its title gives it. `data.written`, `data.removed`, `data.unchanged`
  and `data.documentsRead` say what happened. A client of the project is told
  no versions, so every page is read again.
- Every edit makes a version; a maintenance rewrite of the stored texts on
  the host does not, and a page kept from before stays as it was read. After
  one, add `--read-all`: every page is read again, and the manifest's rules
  hold as ever.
- An export read from a folder's tree is refused (exit 1) when the host
  answers as an older Minka without trees, rather than losing its uploads;
  try again, and only if the host really is older, `--force`.
- The directory keeps `.minka-export.json`, the list of what the export
  wrote. A file it did not write is never written over or removed: a run
  that would is refused before it writes anything (exit 1, the paths in
  `data.paths`). A directory that holds files but no manifest, or a file of
  the export somebody changed since, is refused too; `--force` exports anyway
  and writes over those changed files, but still never touches a file it did
  not write. Export each folder into a directory of its own. A run that
  failed halfway is carried on by running it again; where that run is
  refused for files that "may have been changed since", keep what is yours
  of them elsewhere, then `--force` writes over them. A second run into the same directory at the
  same time is refused (exit 1).
- The export is for reading. To change a page, use `docs show` and `docs
  edit` with its `id`; the frontmatter and the anchors are not part of the
  page in Minka.

## What does not exist

No reading of a draft, by anybody; no replacing the file of an upload by an
earlier one of its versions, other than downloading it and giving it anew;
removing anything for good. All of it is done in Minka on the web.

## Codes of this part

`documentTitleMissing`, `documentTitleTooLong`,
`documentTitleHasControlCharacters`, `documentBodyTooLong`,
`documentBodyNotReadable`, `decisionBlockInvalid`, `documentLocked`,
`documentAudienceMissing`, `personNotAProjectMember`,
`personCannotSeeIt` (a person to replace, not input to fix), `notifyIdsMissing`,
`notifyIdsWithoutChosenAudience`,
`folderUnknown`, `folderArchived`, `documentArchived`, `uploadArchived`,
`documentNotArchived`, `uploadNotArchived`, `folderNotArchived`,
`folderIntoItself`, `toolUnknown`, `entrySharesFiles`, `folderTitleMissing`,
`folderTitleTooLong`, `folderTitleHasControlCharacters`, `bookWithinABook`,
`folderOrderAutomatic`, `documentChanged` (409, with the document in `data`),
`uploadTitleMissing`, `uploadTitleTooLong`, `uploadTitleHasControlCharacters`,
`uploadDescriptionTooLong`, `uploadDescriptionNotReadable`, `uploadChanged`
(409, with the upload in `data`), `fileUnknown`, `fileInUse`, the codes of
storing a file (`contentTypeMismatch`, `tooLarge`), and the comment codes a
card knows. An entry in the archive, or one in a folder in the archive, can be
read and takes no write: exit 9 with `documentArchived` or `uploadArchived`.
