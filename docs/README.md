# Documentation of the Minka CLI

The map of the whole documentation: one line per document, with a sentence on
when you need it. There is deliberately only **this one** index.

**Rules:** `stable/` describes the practice in force · `guides/` are proven
procedures · `work-in-progress/` changes often. The principle above everything:
the documentation says what holds; how it came about is in git.

## Rules in force

- [Output, errors and exit codes](stable/output-contract.md): The three output
  modes, the envelope, the table of exit codes, how commands are cut — read
  before any work on a command.
- [Token, hosts and requests](stable/security.md): How the token is kept and
  sent, how a host name is read whoever wrote it, what is fixed about the HTTP
  client, which check happens before a request — before any work on sign-in,
  client or IDs.
- [Rich text in both directions](stable/rich-text.md): Markdown to HTML and
  back, the allowlist of the server, sources, files, mentions and references
  to other records — before any
  work on descriptions, comments or messages.
- [First steps](stable/first-steps.md): What `setup` asks and in which order,
  why it needs a terminal, what `doctor` checks and why it always exits 0 —
  before any work on `setup`, `doctor` or the sign-in.
- [The skill](stable/skill.md): How the skill travels inside the binary, how it
  gets installed, what happens after an upgrade, the test against the command
  tree — before any work on a command's name or on `skill`.
- [Releases](stable/releases.md): What publishing a release in GitHub sets off,
  why Homebrew and no install script, why the formula builds from source —
  before any work on the workflows, the Makefile's `dist` or the formula.
- [Importing from Basecamp](stable/import.md): What the extractor reads
  through the Basecamp CLI and why, what a bundle holds so far, how a run
  protects what is in `--out`, the privacy rule for bundles — before any work
  on `import` or on a further kind of record.
- [Saying what you are working on](stable/at-work.md): Where the cards of
  `work start` are kept and why the machine shares one list, when a card is
  given up and what keeps it, what the client is, what the heartbeat does in
  every case it can meet, what the keepalive reports while one long tool call
  runs, how the hook is written — before any work on `work`, `heartbeat` or
  `hooks`.
- [Projects](stable/projects.md): What a project carries, the edit that sends
  only what was given and needs no guard, the description that is plain text,
  the client who gets the same 404 as an outsider, the order of the refusals —
  before any work on `projects`.
- [The archive of a board](stable/archive.md): Why the page size is bounded in
  the CLI, what a walk of several pages promises and what it does not, how a
  cursor that has left the archive is worked around, the refusals of the two
  writes — before any work on `boards archive`, `cards archive` or `cards
  unarchive`.
- [Docs & Files](stable/docs-and-files.md): What a document and an upload are,
  the two requests a file goes into a tool with, the body that is never
  refused, the guard that cannot see one second, whom a new document reaches,
  the flag that names the kind of entry, the archive of a folder, the refusals
  — before any work on `docs`.
- [To-do lists and groups](stable/todo-lists.md): What a read of a list hands
  over and what it only counts, the colour of a group and why white is none,
  the archive a list goes quiet in and the group that is then at no address,
  why a group move names its place, the refusals, the comments under a list —
  before any work on `todolists` or `todogroups`.
- [To-dos and subtasks](stable/todos.md): What a to-do carries, the days Minka
  keeps otherwise than they were sent, the people given as a whole set, where a
  to-do may move, the two pages and the one that has no end, the comments under
  a to-do and a list, the order of the refusals — before any work on `todos`.
- [Repeating to-dos](stable/todo-series.md): What a series is and hands
  over, the days only Minka works out, the flags of a schedule and the detail a
  rhythm ignores, the edit that sends only what was given and its guard, the
  new to-do named by `Location`, the order of the refusals — before any work on
  `todos repeat` or `todos series`.
- [Check-ins](stable/checkins.md): What a question carries and whom `asked`
  leaves out, the schedule and the day every other week counts from, the edit
  that names nobody, the day an answer belongs to, the page of answers without
  an end and the reminders without a page, the order of the refusals — before
  any work on `checkins`.
- [Posts](stable/posts.md): What a page of posts hands over and where it
  ends, the post that is shown to no client, whom a new post reaches, pinning,
  the archive and the trash, the guard that stands aside where Minka refuses,
  the order of the refusals — before any work on `posts`.
- [Testing](stable/testing.md): Fake API, sabotage proofs, what a test pins
  down, what it may demand of time — read before your first test.

## Guides

- [Releasing a version](guides/release.md): Choosing the number, publishing in
  GitHub, trying the result, what to do when a step fails — before every
  release.
- [Live check](guides/live-check.md): Where writing is allowed, in which order
  to check, what gets tidied up afterwards — before every run against a real
  instance.
- [Extracting from Basecamp live](guides/import-run.md): Which identity and
  which binary a run takes, how long it lasts, where the bundles lie, how a
  bundle is checked without showing what is in it — before every run of
  `import basecamp` against the real account.

## Decisions

- [Decision register](decisions.md): Per decision, what holds, why and where
  it is implemented (C-001…). For looking things up, not for reading in one
  go. Decisions about the API itself are in Minka's register (E-…).

## Work in progress

- [Status](work-in-progress/status.md): What the CLI can do, what is checked
  against the fake only, what is open in the API — the first look of every new
  session.

---

With a code change, the documentation update belongs in the same commit. What
is outdated gets deleted, not left standing; when a document is created or
moved, this index is kept up to date.
