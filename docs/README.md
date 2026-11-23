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
- [Token, hosts and requests](stable/security.md): How the token is kept,
  sent and revoked on signing out, how a host name is read whoever wrote it,
  what is fixed about the HTTP client, which check happens before a request —
  before any work on sign-in, client or IDs.
- [Profiles](stable/profiles.md): What a profile is and how it is named, which
  one a command uses and why, `$MINKA_TOKEN` beside a profile, the default,
  the file that older versions still read and what becomes of one they wrote,
  the cards of `work start` per account and the heartbeat over all of them —
  before any work on sign-in, `profiles`, the configuration file or the store.
- [Rich text in both directions](stable/rich-text.md): Markdown to HTML and
  back, the allowlist of the server, the shapes of a document, decisions
  blocks and their rows, sources, files, mentions and references to other
  records — before any
  work on descriptions, comments, messages or documents.
- [First steps](stable/first-steps.md): What `setup` asks and in which order,
  why it needs a terminal, what `doctor` checks and why it always exits 0 —
  before any work on `setup`, `doctor` or the sign-in.
- [The skill](stable/skill.md): How the skill travels inside the binary, what
  stands in its core and what in its references, how it gets installed, what
  happens after an upgrade, the test against the command tree — before any
  work on a command's name, on the skill's text or on `skill`.
- [Releases](stable/releases.md): What publishing a release in GitHub sets off,
  why Homebrew and no install script, why the formula builds from source —
  before any work on the workflows, the Makefile's `dist` or the formula.
- [Importing from Basecamp](stable/import.md): What the extractor reads
  through the Basecamp CLI and why, what a bundle holds so far, how a run
  protects what is in `--out`, which files a run keeps for the next one, the
  privacy rule for bundles — before any work
  on `import` or on a further kind of record.
- [Saying what you are working on](stable/at-work.md): Where the cards of
  `work start` are kept and why the machine shares one list, when a card is
  given up and what keeps it, what the client is, what the heartbeat does in
  every case it can meet, what the mod in the Claude Code process does, how
  `setup` and `hooks install` install it, and how the old hook is taken out —
  before any work on `work`, `heartbeat`, `hooks`, `setup` or `mod/`.
- [Waiting and coordinating](stable/waiting-and-coordinating.md): What the
  bell's list, one line of it, its count and the own coordinator entry of a
  board offer, which rows a page leaves out and which it counts, where a wait starts and
  which session it goes by, where the position lies and when it moves on, the
  filters, the timeout and exit 12, a position Minka forgot, the entry per
  account and board and what an older Minka's 409 does, the filters of `cards
  mine` — before any work on `notifications`, `boards coordinator` or `cards
  mine`.
- [Projects](stable/projects.md): What a project carries, the edit that sends
  only what was given and needs no guard, the description that is plain text,
  the client who gets the same 404 as an outsider, the order of the refusals,
  starting a project with its category, adding and taking out members, the
  tools of every type with their positions, moves and trash — before any work
  on `projects`, `people add`, `people remove` or `tools`.
- [Columns](stable/columns.md): How a column is named, what it carries and
  its colour, the place and colour a new column gets, why a move reads the
  board first, what happens to the cards of a deleted column, who may write
  and the order of the refusals — before any work on `columns`.
- [The archive of a board](stable/archive.md): Why the page size is bounded in
  the CLI, what a walk of several pages promises and what it does not, how a
  cursor that has left the archive is worked around, the refusals of the two
  writes — before any work on `boards archive`, `cards archive` or `cards
  unarchive`.
- [Docs & Files](stable/docs-and-files.md): What a document and an upload are,
  the two requests a file goes into a tool with, the body that is never
  refused, the guard that cannot see one second, whom a new document reaches,
  the flag that names the kind of entry, the archive of a folder, how a tool
  is kept in order — the trash, the archive, moves, also into another tool,
  folders, a version made the newest — and who may, the refusals, books and
  the order set by hand, the export of a book into files and what a run never
  touches — before any work on `docs`.
- [Search](stable/search.md): What a search finds across Minka and what never,
  the kinds and the project it is narrowed to, the search within a tool or a
  folder, what is checked before a request, its pages and the stop at offset
  500, what a hit names and leads to, what an older Minka answers — before any
  work on `search`.
- [Labels](stable/labels.md): Where a label is kept and reached, its name
  rules and colours, how a label is named by ID or name, the move by one
  neighbour, putting several on an item and the scope that replaces, where
  labels show, the order of the refusals — before any work on `labels` or the
  `label` and `unlabel` commands of cards, documents and uploads.
- [Comments on a card](stable/comments.md): What a read of the comments
  hands over and which one is the newest, the guards that keep a comment from
  being written past an unread answer, what counts as read and where the marks
  lie, the address of a comment under any item, the settings of `config` —
  before any work on `comments` or `config`.
- [The back of a card](stable/card-back.md): The rule for the notes, the
  back that is the team's alone, what an attachment is and how a file goes on
  the back in two requests, the text `show` prints and what it refuses, the
  order of the refusals — before any work on `cards notes`.
- [Steps](stable/steps.md): What a step of a card carries, the people given as
  a whole set and who may be among them, the day, why `cards steps create` takes
  neither, the text that changes, the move and its end, the step that is
  another card or a to-do, what an older Minka answers, the order of the
  refusals — before any work on `cards steps`.
- [To-do lists and groups](stable/todo-lists.md): What a read of a list hands
  over and what it only counts, the colour of a group and why white is none,
  the archive a list goes quiet in and the group that is then at no address,
  why a group move names its place, lists from templates and whom they leave
  off, the refusals, the comments under a list —
  before any work on `todolists` or `todogroups`.
- [To-dos and subtasks](stable/todos.md): What a to-do carries, the days Minka
  keeps otherwise than they were sent, the people given as a whole set, where a
  to-do may move, the two pages and the one that has no end, the comments under
  a to-do and a list, the order of the refusals — before any work on `todos`.
- [Repeating to-dos](stable/todo-series.md): What a series is and hands
  over, the days only Minka works out, the flags of a schedule and the detail a
  rhythm ignores, the edit that sends only what was given and its guard, the
  notes and the guard over their files, the new to-do named by `Location`,
  the order of the refusals — before any work on `todos repeat` or `todos
  series`.
- [Check-ins](stable/checkins.md): What a question carries and whom `asked`
  leaves out, the schedule and the day every other week counts from, the edit
  that names nobody, the day an answer belongs to, the page of answers without
  an end and the reminders without a page, the order of the refusals — before
  any work on `checkins`.
- [Posts](stable/posts.md): What a page of posts hands over and where it
  ends, the post that is shown to no client, whom a new post reaches, pinning,
  the archive and the trash, the guard that stands aside where Minka refuses,
  the order of the refusals — before any work on `posts`.
- [Decisions](stable/decisions.md): What a decision carries, a decision
  named by its short identifier, who set its status, whom it reaches and
  when, a status named by its ID or its role in any case, the flag per kind of
  subject, who decided, several written from a file and where such a run stops, the fields cleared
  by `--no-…`, the earlier decision that moves only while the new one holds,
  history and consequences, the order of the refusals — before any work on
  `decisions`.
- [Calendars and events](stable/calendar.md): What a span hands over and whose
  days it reads, the series that stands in it once with its occurrences, the
  time given as moments or days, the zone a series keeps its clock time in,
  what an edit does to the repetition, the write without a guard, the order of
  the refusals — before any work on `events`.
- [Meetings](stable/meetings.md): What the meeting of an event hands over and
  what its flags allow, the day that names the meeting of a series and where
  it is found, the three ways to name an agenda item, what an outcome does to
  the decision and to the votes, why closing asks for its audience, what an
  older Minka answers, the order of the refusals — before any work on `events
  meeting`.
- [Reminders](stable/reminders.md): What a reminder of one's own is, the
  kind beside the ID and why it is checked here, the preset counted in
  Minka's zone and the moment sent as written, the list cut before what is out
  of sight is left out, the outcome, why it is no write into a project, the
  order of the refusals — before any work on `reminders`.
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
