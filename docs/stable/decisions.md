# Decisions

A Decisions tool holds **decisions**: the sentence that holds, its reason, a
body, a status of the tool, who decided, when and in which event, the votes,
what it applies to, the days Minka acts on, the earlier decision it
supersedes or revokes, its history and consequences, labels and comments.
`decisions tools`, `tool`, `statuses`, `list`, `show` and `versions …` read;
`decisions create`, `edit`, `status`, `supersede`, `revoke`, `delete`,
`label`, `unlabel`, `history add`, `consequences add` and `entries …` write;
`decisions comments …` reads and writes the comments under one.

Everything here was read in Minka's code — `DecisionsApiController`, its
resources, the decision handlers, `LabelsApiController` and `SearchTool` —
and in `ApiDecisionsTest`, `ApiDecisionEntriesTest` and
`ApiDecisionCommentsTest`; none of it has been seen live.

## What is not there

Archiving a decision and taking it back out, taking one out of the trash,
writing the statuses of a tool, changing an entry of the history or the
consequences, and writing several decisions in one request have no address in
the API. A decision has no short identifier yet.

## Reading

`GET projects/{projectId}/decisions` lists the tools in use, `GET
decisions-tools/{toolId}` one by its ID alone (C-120), `GET
decisions-tools/{toolId}/statuses` its statuses in their order. A status
carries `id`, `name`, `customName` and `role`; a status the team added has no
role, and a decision from before its tool had statuses carries a status
without an ID, which holds. A client is offered no draft.

`decisions list <toolId>` reads `GET decisions-tools/{toolId}/decisions`, the
day decided on last first and one without a day by the day it was written,
not paged. The text names each decision by its ID, sentence, status, day and
labels, with its reason below, so that what holds is looked up at a glance.
Every status is listed unless `--status` names one; `--holds` is `--status
holds`. A status is named by its ID, which goes out in lowercase, or by its
role, which goes out as typed, because Minka compares a role exactly
([C-165](../decisions.md#c-165)). `--label` takes an ID or the name of a label
of the project, looked up before the list is read
([C-171](../decisions.md#c-171)); `--applies-to-card` and `--applies-to-event`
narrow to one subject.

`decisions show` gives the decision with `body` and `bodySource`, its files,
labels, comments (oldest first), `decidedBy` (each `{person, name}`),
`decidedIn`, `votes`, `supersedes` or `revokes` and `supersededBy` and
`revokedBy`, `history` and `consequences` with `consequencesDone`,
`appliesTo`, `validUntil` and `reviewOn`. The text converts the body to
Markdown. A decision of the archive is read and says so. A client meets only
a decision shown to clients, never a draft, and no version.

`decisions versions list` and `show` read the versions, oldest first: the
sentence, reason, body, day, who decided, event and votes. A change of the
status or of the earlier decision makes none. They are the team's alone.

## Writing

`decisions create --tool <toolId> --title <sentence>` writes a decision down,
201; without `--status` it holds. Every field goes out only where its flag is
given. The body is Markdown, converted here, or HTML with `--body-format
html`. `--decided-by` is repeatable: an ID or `me` names a member, anything
else is a name given freely ([C-167](../decisions.md#c-167)). `--event` is
`decidedInEventId`; Minka takes its day where `--decided-on` is not given and
the event does not repeat. What the decision applies to is named by
`--applies-to-card`, a card by its ID or short identifier, or
`--applies-to-event` ([C-166](../decisions.md#c-166)). `--visible-to-clients`
goes with a new decision only.

`--from <file|->` writes several: a JSON list of objects in Minka's names of
the fields of a new decision, a body as Markdown unless `--body-format html`,
taken as written: a body of `-` is no call for stdin.
A field Minka does not take is exit 1 before any request; IDs go out in
lowercase, a role as written. The decisions go one request each in the order
given, and the first refusal ends the run with `data.created`, the decisions
as Minka answered, and `data.notCreated`, the titles of the rest
([C-168](../decisions.md#c-168)).

`decisions edit` is a PATCH of what is given; what is left out stays, and a
request that changes nothing is 200. Each field is cleared by its `--no-…`
flag, which sends null, and `--no-earlier` sends null for both relations
([C-170](../decisions.md#c-170)). `--decided-by` is the whole list anew. A new
body that would drop files or mentions is refused unless `--drop-attachments`
is given, as for a post (C-082). `decisions status <decisionId> <status>`
sends the status alone.

`decisions supersede <decisionId>` and `revoke` read the earlier decision for
its tool and write the new one there with `supersedes` or `revokes`. Minka
moves the earlier decision into the status of role superseded or revoked when
it is newly named or the new one comes into force, and only while the new one
holds; it never moves it back. Every answer whose earlier decision stands in
another status draws a `notice` saying so ([C-169](../decisions.md#c-169)).

`decisions delete` puts a decision into the trash, 204; a client only one of
their own. `decisions label` and `unlabel` work as for a card, with the labels
of the decision's tool and project (see [Labels](labels.md)).

`decisions history add <decisionId> <address>` and `consequences add` append
an entry: an address of this Minka, a `#` card name, or for the history a web
address with `--description`; a consequence is a to-do or a card. Naming one
there already answers with that entry. `decisions entries move` puts an entry
first, last, after or before another of its part; `--bottom` reads the
decision for the last entry of the part. `decisions entries delete` takes one
off.

## The order of the refusals

On a new decision: 415 and 400, the tool (404), then the title, the reason,
the body, the day, the status, who decided, the event and the votes, then the
earlier decision named twice or malformed, the shape of `appliesTo`,
`validUntil` and `reviewOn`, then the archived project, then a subject and an
earlier decision that are unknown. A change follows the same order after the
decision (404), except that it meets an unknown earlier decision before an
unknown subject; a change of a decision of the archive is 404. An entry, a
label and a move of an entry meet the archived project, then
`decisionArchived`.
