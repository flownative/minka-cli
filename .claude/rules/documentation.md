---
paths:
  - "Documentation/**"
---

# Structure and lifecycle of the documentation

The complete index is `Documentation/README.md` — one line per document, saying
when you need it. Do not create sub-indexes.

The principle above everything: **the status quo, not how it came about.** The
documentation says what holds; how it got there is in git.

## Language

The documentation and the skills of this project are written in English.

## Types

- **`stable/`** — Established knowledge, always in step with the code.
- **`guides/`** — Proven step-by-step procedures.
- **`work-in-progress/`** — Active work, changes often.

There is no archive: what nobody needs any more is deleted. Measurements and
retrospectives belong in the commit message.

## Decisions

Two readers, two places: the rule stands normatively under `stable/` and names
the ID, the reason stands in one sentence in the register
(`Documentation/decisions.md`).

- IDs (`C-001`) are stable and never reused.
- An entry has at most eight lines: *what holds*, *why* (one sentence),
  *applies to*, *implemented as* (a component, not a commit hash).
- When a decision changes within the same scope, the entry is overwritten; when
  it is overturned, the new one gets a new ID and the old one the line
  "superseded by C-xxx".
- Decisions about the **API** belong to Minka, not here. Here stands only what
  the CLI decides for itself.

## Tone

Present tense, normative. No "formerly", "since", "by now", no date and no
commit hash in running text. A live check is not a document but a sentence in
the status — and if it changed something, a line in the commit message.

## Lifecycle

The documentation update belongs in the same commit as the code change. Delete
what is outdated instead of leaving it. When creating or moving a document,
keep the index up to date.
