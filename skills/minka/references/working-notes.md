# Working notes

The back of a card is the working memory of whoever works on it, agents above
all: the plan, the intermediate state, the handover. Another session, on
another machine, takes the card over by reading it. The front — the
description and the comments — is the assignment and the conversation,
written for people. What still holds once the card is done, a decision with
its reason, a rule, a how-to, belongs in the repository before the card goes
to Done; the back stays on the card when it is done.

The back holds one text, the notes, and files beside it, the attachments. The
notes are light Markdown, stored and printed as they are, never converted.
Writing either notifies nobody and leaves no line in the timeline.

```bash
minka cards notes attachments list <cardId> --json
minka cards notes attachments create <cardId> (<file> | --blob <blobId>) [--name <name>] [--content-type <type>] --json
minka cards notes attachments show <cardId> <attachment> --json
minka cards notes attachments download <cardId> <attachment> [-o <path>|-] [--force] --json
minka cards notes attachments delete <cardId> <attachment> --json
```

How to keep the notes:

- **In the language of the card's front.** Whatever language the description
  and the comments are written in, the back is written in it as well.
- **The state, not a chronicle.** Rewrite what changed and drop what no longer
  holds; never append a log. What happened is in the comments, in the version
  history and in the pull requests.
- **About 40 to 60 lines.** A guide, not a limit: whoever takes the card over
  reads all of it. Say where the work stands, not how it got there.
- **In this outline**, as headings:
  `Status` · `Plan` (the packages, each with its state) · `Settled` (what was
  decided, by whom, when) · `Where things are` (branch, worktree, migration,
  numbers of decisions, pull requests) · `Next step` · `Pitfalls`.
- **Name every attachment** in the notes with one line of what it holds, such
  as `- research.md — what the three tent suppliers offer, prices of
  2026-10-07`, so that whoever reads the back knows what to open without
  opening everything.
- **Write at the milestones**, not as you go: the plan stands, a package is
  merged, a handover, blocked on a question you cannot answer yourself. In
  between, work.
- **Read the back before you begin,** and never write without having read.
- **`show` first, then `set` with `--expect <updatedAt>`.** The guard is what
  makes a write that would wipe out somebody else's notes fail instead. With
  `--json`, `show` hands you the `set` to run with `--expect` filled in, in its
  `breadcrumbs`; a `set` that went through hands back the same with the time it
  has just written, so a second write in the same session needs no second read.
- **`--force` is how one session writes over another's notes unasked.** It is
  not the way past a refusal: a refusal means somebody wrote while you were
  writing, and their text is in `data`. Merge yours into their `body` and send
  it again with their `updatedAt`. Take `--force` only where the person you work
  for has asked for it — whether anybody else writes these notes is not
  something you can know.
- The agent the card is assigned to writes its back. On a card that is not
  yours, leave the back alone and say on the front what you would have
  written.

The back is the team's alone. A client of the project gets exit 2
(`not_found`) for the notes and the attachments of a card whose front it sees,
with a hint that says so.

What the two commands do:

- `cards notes show <cardId>` gives `{cardId, body, updatedAt, updatedBy,
  appUrl}`. Notes nobody has written have `body` `""` and `updatedAt` and
  `updatedBy` null. The card itself does not say whether it has notes.
- `cards notes set <cardId> -` replaces the notes as a whole with stdin; `""`
  clears them. It needs `--expect <updatedAt>` — the `updatedAt` you read, or
  `none` for notes nobody had written yet — or `--force`, and the two exclude
  each other (exit 1). Notes somebody changed since the time `--expect` names
  are left as they stand: exit 9, `meta.apiCode` `notesChanged`, and `data`
  holds the notes as they stand now. Sending the text that stands already is no
  conflict.
- At most 100000 characters, counted as Minka stores them; a longer text exits
  9 with `cardNotesTooLong` before it is sent. Control characters other than
  line breaks and tabs exit 9 with `cardNotesHasControlCharacters`, and
  invisible format characters — a direction override, a tag character, a
  zero-width space, a byte order mark, runs of joiners or variation selectors
  — with `cardNotesHasInvisibleCharacters`; a single zero-width joiner or
  variation selector, as in emoji, is fine. One byte order mark at the start
  of stdin, as editors write it, is dropped before the notes are sent.
- `--expect` takes the time in any form Minka reads it: up to six digits of a
  second, under `Z` or an offset. It has to be the stored time exactly, so
  send back the `updatedAt` you read and nothing built by hand.
- In text output `notes show` prints the body alone; who changed it last and
  the `--expect` to write over it go to standard error, so that the notes can
  be piped into a file and sent back unchanged.
- A card in its board's archive can be read, not written: exit 9 with
  `cardArchived`.

What the attachments are and what their commands do:

- An attachment is a file on the back, named by its file name, which is one per
  back as written, case and all: `plan.md` and `Plan.md` are two. Putting a
  file there under a name the back has already makes it that
  attachment's next `version`: it keeps its `id` and its `appUrl`, the address
  to link to from a comment. Minka keeps the earlier versions but neither lists
  nor serves them. An attachment carries `id`, `cardId`, `filename`, `version`,
  `contentType`, `byteSize`, `uploadedAt`, `uploadedBy`, `content` (the address
  of its bytes) and `appUrl`. `cards notes show` does not list them.
- `cards notes attachments create <cardId> <file>` does both of Minka's
  requests: it stores the file in the card's project and puts it on the back.
  It reads the card and its back first, so a card in the archive (exit 9,
  `cardArchived`) or a back you may not see (exit 2) costs no transfer. Where
  the file was stored and putting it on the back was refused, `data` is the
  stored upload. Where the card was the reason, the hint names `--blob
  <blobId>`, which tries again without sending it anew; where the upload was
  (`uploadNotAttachable`), something took it in the meantime, and running the
  command again stores the file anew. The reads before the transfer go in
  Minka's order: the back you may not see before the archive. `--blob` takes only an upload you stored in the
  card's project that nothing holds yet; anything else exits 9 with
  `uploadNotAttachable`.
- `<attachment>` is the `id`, the position in `list` or the file name. The
  name as written wins; otherwise case is ignored where that leaves one file,
  and two that differ in case alone exit 8 (`ambiguous`). A name that is a
  number counts as the position.
- `cards notes attachments list <cardId>` lists them by name.
- `cards notes attachments show <cardId> <attachment>` prints the newest version
  as text, byte for byte, alone on standard output, where that is a terminal
  or `--styled` is given; piped without `--styled`, and with `--json`, it is
  `data.text` of the envelope, beside the attachment as `data.file`. To put the
  file on disk, take `download`, not a redirect of `show`. A file that is no
  text — a picture, audio, video, or bytes that are no UTF-8 or carry a zero
  byte, judged on the first bytes already — exits 1 and prints nothing: fetch
  it with `download`. So does, outside the envelope, a text with control
  characters other than tab, line feed and carriage return, which a terminal
  would act on; `--json` hands it over escaped.
- `cards notes attachments download <cardId> <attachment>` writes the file into
  the current directory under its name, never over a file that is there, which
  is refused before the bytes are fetched (`--force` replaces it, `-o <path>` names another, `-o -` writes the bytes
  to standard output alone), readable by its owner alone.
- `cards notes attachments delete <cardId> <attachment>` takes it off with every
  version, and Minka discards their uploads with it. Take its line out of the
  notes as well.
- A card in its board's archive: its attachments can be read, not written
  (exit 9, `cardArchived`). A Minka older than attachments on the back (not in
  a release yet) answers exit 2 for every one of these commands.
