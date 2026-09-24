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
  sent, what is fixed about the HTTP client, which check happens before a
  request — before any work on sign-in, client or IDs.
- [Rich text in both directions](stable/rich-text.md): Markdown to HTML and
  back, the allowlist of the server, sources, files and mentions — before any
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
  `work start` are kept and why the machine shares one list, what the client is,
  what the heartbeat does in every case it can meet, how the hook is written —
  before any work on `work`, `heartbeat` or `hooks`.
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
