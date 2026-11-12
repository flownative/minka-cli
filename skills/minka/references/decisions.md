# Decisions

A Decisions tool holds **decisions**: the sentence that holds, why, who
decided, when and in which event, a status of the tool and who set it, and a
short identifier such as `ENT-14`. `decisions …` reads and writes them;
`decisions comments …` the comments under one. Wherever a command below says
`<decision>`, give the ID or the short identifier.

## Commands

```bash
minka decisions tools <projectId> --json
minka decisions tool <toolId> --json
minka decisions statuses <toolId> --json
minka decisions list <toolId> [--holds | --status <id|role|all>] [--label <label>] [--applies-to-card <card> | --applies-to-event <eventId>] [--set-by-agent] --json
minka decisions show <decision> --json
minka decisions create --tool <toolId> --title "The sentence" [--reason "Why."] [--body - < more.md] [--status <id|role>] [--decided-on YYYY-MM-DD] [--decided-by <id|me|name>]... [--event <eventId>] [--votes "5 : 1"] [--applies-to-card <card> | --applies-to-event <eventId>] [--valid-until YYYY-MM-DD] [--review-on YYYY-MM-DD] [--supersedes <decision> | --revokes <decision>] [--audience everybody|chosen|nobody] [--notify <person>]... [--visible-to-clients] [--label <label>]... --json
minka decisions create --tool <toolId> (--title "First" --title "Second"... | --titles-from <file|->) [--decided-on …] [--decided-by …]... [--event …] [--status …] [--applies-to-card … | --applies-to-event …] [--audience … [--notify …]...] [--visible-to-clients] [--label <label>]... --json
minka decisions create --tool <toolId> --from decisions.json --json
minka decisions edit <decision> [--title "…"] [--reason "…"] [--body -] [--body-format markdown|html] [--status <id|role>] [--decided-on YYYY-MM-DD] [--decided-by <id|me|name>]... [--event <eventId>] [--votes "…"] [--applies-to-card <card> | --applies-to-event <eventId>] [--valid-until YYYY-MM-DD] [--review-on YYYY-MM-DD] [--supersedes <decision> | --revokes <decision>] [--audience …] [--notify <person>]... [--drop-attachments] [--no-reason] [--no-decided-on] [--no-decided-by] [--no-event] [--no-votes] [--no-applies-to] [--no-valid-until] [--no-review-on] [--no-earlier] --json
minka decisions status <decision> <id|role> [--audience everybody|chosen|nobody] [--notify <person>]... --json
minka decisions supersede <decision> --title "The new sentence" [the flags of create but --label and --titles-from] --json
minka decisions revoke <decision> --title "What holds instead" [the flags of create but --label and --titles-from] --json
minka decisions delete <decision> --json
minka decisions label <decision> <label>... --json
minka decisions unlabel <decision> <label>... --json
minka decisions versions list <decision> --json
minka decisions versions show <decision> <versionId> --json
minka decisions history add <decision> <address> [--description "What it is"] --json
minka decisions consequences add <decision> <address> --json
minka decisions entries move <decision> <entryId> --top|--bottom|--after <entryId>|--before <entryId> --json
minka decisions entries delete <decision> <entryId> --json
minka decisions comments list <decision> --json
minka decisions comments create <decision> - --json < comment.md
minka decisions comments edit <decision> <commentId> - --json < comment.md
minka decisions comments delete <decision> <commentId> --json
minka decisions comments react <decision> <commentId> 👍 --json
minka decisions comments unreact <decision> <commentId> 👍 --json
```

## Looking up what holds

- Before you work in a project, read what holds: `decisions tools
  <projectId>` names the tools, `decisions list <toolId> --holds` the
  decisions in force, each with its ID, sentence and reason. Nothing is paged.
  `decisions show <decisionId>` gives the whole decision.
- Every status is listed unless `--status` names one. A status is named by
  its ID or by its **role**, in any case: `draft`, `proposed`, `holds`,
  `done`, `superseded`, `revoked`, `rejected`. "In force" is no role (exit 9
  `decisionStatusUnknown`). A status the team added has no role; `decisions
  statuses <toolId>` lists all with their IDs. A client is never offered
  `draft` and never meets a draft.
- `--label` takes a label's ID or the name of a label of the project.
  `--applies-to-card` (an ID or a short identifier such as `MNK-1A`) and
  `--applies-to-event` narrow to the decisions that apply to that card or
  event. `--set-by-agent` narrows to the decisions whose status an agent set,
  an earlier decision an agent superseded or revoked among them: check those
  before you rely on them.
- `statusSetBy` says who set the status and when: `person`, `agent` (true
  where an agent set it by hand), `minka` (true where Minka moved it, and
  `person` is whom its line names), `at`; null where nobody is known. The text
  of `list` says "set by an agent" or "set by Minka", `show` names who.

## Naming a decision

- Every command and flag that takes a decision takes its ID or its short
  identifier (`ENT-14`, any case, also under a former prefix of its tool).
  `shortId` is null for a decision its tool has not numbered yet: use the ID
  then.
- In a comment, a description, a message or a history entry, point at a
  decision as `#ENT-14` or by its `shortUrl`
  (`https://<host>/go/decision/ENT-14`); Minka links either. A decision is no
  consequence: `consequences add` refuses it.
- Outside Minka — a commit, a pull request — name a decision by its
  `shortUrl`, or by its `appUrl` where `shortUrl` is null. The text views give
  that same address.

## Writing one down

- `decisions create --tool <toolId> --title "..."` writes a decision down;
  without `--status` it holds. Write the title as the sentence that holds
  and `--reason` as one sentence of why. The body is Markdown, for what more
  there is to say.
- `--decided-by` is repeatable, in the order the names are to stand: an ID or
  `me` names a member of the project, anything else is a name given freely
  ("Board") and links nobody. `--event <eventId>` is the event it was decided
  in; without `--decided-on` the decision takes its day.
- `--valid-until` is the day Minka sets a decision in force to done,
  `--review-on` the day it reminds those who follow it. `--applies-to-card` or
  `--applies-to-event`: once the event is over or the card done, Minka sets a
  decision in force to done.
- `--audience` says whom a decision reaches once it is proposed or in force:
  `everybody` (what Minka takes without the flag), `chosen` with one
  `--notify <person>` each (ID, `me`, e-mail or name; they then follow it), or
  `nobody`. It takes effect only where the decision comes into proposed or
  in force: on `create`, `supersede`, `revoke`, and on `status` or `edit
  --status` that moves it there. A draft tells nobody, and `edit` refuses
  `--audience` without `--status` (exit 1). Minka refuses `--notify` without
  `chosen` (`notifyIdsWithoutChosenAudience`), `chosen` without a person
  (`notifyIdsMissing`), a non-member (`personNotAProjectMember`) and a client
  who is not shown the decision (`personCannotSeeIt`).
- `--label <label>`, repeatable, puts a label of the project on: its ID or
  its name. An unknown one exits 2 and two of one scope exit 9
  (`labelsShareAScope`), before anything
  is written. One decision takes its labels right after it is written; should
  one fail, the error carries `data.created`, `data.labelled` and
  `data.notLabelled` — the decision is written, put the rest on with
  `decisions label`.
- Several decisions that share their fields, such as the votes of one
  meeting: `--title` more than once, or `--titles-from <file|->` with one
  sentence per line (empty lines are left out, everything else goes as
  typed: the CLI strips no list markers as the web form does, so leave them
  off; `--titles-from -` beside `--body -` exits 1). They go in one request, all or none, in
  their order, numbered one after the other, each with `agendaNumber` 1, 2, 3.
  They share `--decided-on`, `--decided-by`, `--event`, `--status`,
  `--applies-to-card`/`--applies-to-event`, `--audience`/`--notify`,
  `--visible-to-clients` and `--label`; any other flag beside them exits 1. At
  most 100. A refused sentence refuses all: exit 9, `meta.apiCode`
  `decisionLinesRefused`, and `data.lines` names each by `index` (from 0),
  `title`, `from` (`--title 2`, `line 5 of agenda.txt`) and `code`
  (`decisionTitleMissing`, `decisionTitleTooLong`,
  `decisionTitleHasControlCharacters`). Correct those and send all again;
  nothing was written. On success `data` is the list of the decisions.
- `--from <file|->` writes several with fields of their own, and is not all
  or none: a JSON list of objects with the fields
  Minka takes (`title`, `reason`, `body` as Markdown, `decidedOn`, `status`,
  `decidedBy` as `[{"personId": …}, {"name": …}]`, `decidedInEventId`,
  `votes`, `supersedes` or `revokes` (an ID or a short identifier),
  `appliesTo` as `{"kind": "card"|"event", "id": …}`, `validUntil`,
  `reviewOn`, `audience`, `notifyIds` (IDs), `visibleToClients`). A field Minka
  does not take exits 1 before anything is sent. A short identifier is looked
  up before the first write, so it cannot name a decision of the same list:
  write that one first, then supersede it in a second run. The first refusal stops the
  run: the error carries `data.created` (as Minka answered) and
  `data.notCreated` (the titles of the rest) — do not write those again.

## Changing, superseding, revoking

- `decisions edit` sends only what is given; each field is cleared by its
  `--no-…` flag, the earlier decision by `--no-earlier`. `--decided-by` gives
  the whole list anew. `decisions status <decisionId> holds` changes the
  status alone.
- When what holds changes, do not edit the old decision: write the new one
  with `decisions supersede <decisionId> --title "..."` (it takes the old
  one's place) or `decisions revoke <decisionId> --title "..."` (it lifts
  it). **The earlier decision moves into superseded or revoked only while the
  new one holds**; with another status it stays where it is, and `notice`
  says so. Nothing ever moves it back.
- A decision in the archive is read, not changed: an edit, a status, a label
  or an entry exits 9 `decisionArchived`. Archiving is done on the
  web.

## History and consequences

- `decisions history add <decision> <address>` records what led to it: the
  `appUrl` of a record, a comment's `…#comment-<id>`, `#MNK-1A`, an earlier
  decision as `#ENT-14` or by its `shortUrl`, or any web address with
  `--description`. `decisions consequences add` records what follows from it:
  a to-do or a card, never a decision. Naming the same again answers with the
  entry there. `decisions show` lists both, a consequence ticked when done.
- `decisions entries move` and `decisions entries delete` take the entry's
  ID from `decisions show --json`.
