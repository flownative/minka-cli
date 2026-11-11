# Decisions

A Decisions tool holds **decisions**: the sentence that holds, why, who
decided, when and in which event, and a status of the tool. `decisions …`
reads and writes them; `decisions comments …` the comments under one.

## Commands

```bash
minka decisions tools <projectId> --json
minka decisions tool <toolId> --json
minka decisions statuses <toolId> --json
minka decisions list <toolId> [--holds | --status <id|role|all>] [--label <label>] [--applies-to-card <card> | --applies-to-event <eventId>] --json
minka decisions show <decisionId> --json
minka decisions create --tool <toolId> --title "The sentence" [--reason "Why."] [--body - < more.md] [--status <id|role>] [--decided-on YYYY-MM-DD] [--decided-by <id|me|name>]... [--event <eventId>] [--votes "5 : 1"] [--applies-to-card <card> | --applies-to-event <eventId>] [--valid-until YYYY-MM-DD] [--review-on YYYY-MM-DD] [--supersedes <decisionId> | --revokes <decisionId>] [--visible-to-clients] --json
minka decisions create --tool <toolId> --from decisions.json --json
minka decisions edit <decisionId> [the flags of create but --visible-to-clients] [--drop-attachments] [--no-reason] [--no-decided-on] [--no-decided-by] [--no-event] [--no-votes] [--no-applies-to] [--no-valid-until] [--no-review-on] [--no-earlier] --json
minka decisions status <decisionId> <id|role> --json
minka decisions supersede <decisionId> --title "The new sentence" [the flags of create] --json
minka decisions revoke <decisionId> --title "What holds instead" [the flags of create] --json
minka decisions delete <decisionId> --json
minka decisions label <decisionId> <label>... --json
minka decisions unlabel <decisionId> <label>... --json
minka decisions versions list <decisionId> --json
minka decisions versions show <decisionId> <versionId> --json
minka decisions history add <decisionId> <address> [--description "What it is"] --json
minka decisions consequences add <decisionId> <address> --json
minka decisions entries move <decisionId> <entryId> --top|--bottom|--after <entryId>|--before <entryId> --json
minka decisions entries delete <decisionId> <entryId> --json
minka decisions comments list <decisionId> --json
minka decisions comments create <decisionId> - --json < comment.md
minka decisions comments edit <decisionId> <commentId> - --json < comment.md
minka decisions comments delete <decisionId> <commentId> --json
minka decisions comments react <decisionId> <commentId> 👍 --json
minka decisions comments unreact <decisionId> <commentId> 👍 --json
```

## Looking up what holds

- Before you work in a project, read what holds: `decisions tools
  <projectId>` names the tools, `decisions list <toolId> --holds` the
  decisions in force, each with its ID, sentence and reason. Nothing is paged.
  `decisions show <decisionId>` gives the whole decision.
- Every status is listed unless `--status` names one. A status is named by
  its ID or by its **role, spelt exactly**: `draft`, `proposed`, `holds`,
  `done`, `superseded`, `revoked`, `rejected`. `Holds` is no role (exit 9
  `decisionStatusUnknown`). A status the team added has no role; `decisions
  statuses <toolId>` lists all with their IDs. A client is never offered
  `draft` and never meets a draft.
- `--label` takes a label's ID or the name of a label of the project.
  `--applies-to-card` (an ID or a short identifier such as `MNK-1A`) and
  `--applies-to-event` narrow to the decisions that apply to that card or
  event.

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
- `--from <file|->` writes several: a JSON list of objects with the fields
  Minka takes (`title`, `reason`, `body` as Markdown, `decidedOn`, `status`,
  `decidedBy` as `[{"personId": …}, {"name": …}]`, `decidedInEventId`,
  `votes`, `supersedes` or `revokes`, `appliesTo` as `{"kind": "card"|"event",
  "id": …}`, `validUntil`, `reviewOn`, `visibleToClients`). A field Minka
  does not take exits 1 before anything is sent. The first refusal stops the
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
- A decision in the archive is read, not changed: an edit or a status exits
  2, a label or an entry exits 9 `decisionArchived`. Archiving is done on the
  web.

## History and consequences

- `decisions history add <decisionId> <address>` records what led to it: the
  `appUrl` of a record, a comment's `…#comment-<id>`, `#MNK-1A`, or any web
  address with `--description`. `decisions consequences add` records what
  follows from it: a to-do or a card. Naming the same again answers with the
  entry there. `decisions show` lists both, a consequence ticked when done.
- `decisions entries move` and `decisions entries delete` take the entry's
  ID from `decisions show --json`.
