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
- [Testing](stable/testing.md): Fake API, sabotage proofs, what a test pins
  down — read before your first test.

## Guides

- [Live check](guides/live-check.md): Where writing is allowed, in which order
  to check, what gets tidied up afterwards — before every run against a real
  instance.

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
