# Decisions

A Decisions tool holds **decisions**: the sentence that holds, its reason, a
body, a status of the tool and who set it, who decided, when and in which
event, the votes, what it applies to, the days Minka acts on, the earlier
decision it supersedes or revokes, its history and consequences, labels,
comments and a short identifier such as ENT-14.
`decisions tools`, `tool`, `statuses`, `list`, `show` and `versions …` read;
`decisions create`, `edit`, `status`, `supersede`, `revoke`, `delete`,
`label`, `unlabel`, `history add`, `consequences add` and `entries …` write;
`decisions comments …` reads and writes the comments under one.

Everything here was read in Minka's code — `DecisionsApiController`, its resources, the decision handlers
(`AddDecisionsHandler` and `DecisionLines` for several at once),
`DbalDecisionRepository`, `DecisionStatusSetBy`, `LabelsApiController` and
`SearchTool` — and in `ApiDecisionsTest`, `ApiDecisionEntriesTest` and
`ApiDecisionCommentsTest`; none of it has been seen live. Several decisions at
once and `agendaNumber` are on Minka's `main` (119d5ca71) and in no release
yet.

## What is not there

Archiving a decision and taking it back out, taking one out of the trash,
writing the statuses of a tool, changing an entry of the history or the
consequences, labels on one new decision (`POST …/decisions` reads no
`labelIds`), and several decisions with fields of their own in one request
have no address in the API, nor have the prefix and the next number of a
tool, which the team sets on the web.

## Naming a decision

Every argument and flag that names a decision — `show`, `edit`, `status`,
`supersede`, `revoke`, `delete`, `label`, `unlabel`, `versions …`, `history
add`, `consequences add`, `entries …`, `comments …`, `--supersedes`,
`--revokes` and `supersedes`/`revokes` of `--from` — takes its ID or its short
identifier, `shortId`. A short identifier is looked up in capitals with `GET
decisions/by-short-id/{identifier}`, which also knows a former prefix of the
tool, and the command goes on with the ID; `show`, `supersede`/`revoke` and
`entries move --bottom` take the decision that answer gives. A draft for a client, an unknown name
and a name of the wrong shape are the same 404, which the CLI words as no
decision you can see going by that name. A value of neither shape is exit 1
before any request; an answer without an ID is exit 7 before anything is
written ([C-172](../decisions.md#c-172)). The short identifiers of earlier
decisions in a `--from` list are looked up before the first decision is
written ([C-175](../decisions.md#c-175)), so a short identifier cannot name
an earlier decision written in the same run: it has none before it is
written.

`shortId` is null for a decision its tool has not numbered yet, and so is
`shortUrl`, its `/go/decision/…` address. Both stand on the list, on a
decision, on `supersedes`, `revokes`, `supersededBy` and `revokedBy` (`shortId`
only) and on an entry naming a card or a decision. A `#ENT-14` and a
`shortUrl` given to `history add` are read by Minka, like `#MNK-1A`, and so
they are in a text; the CLI passes them on as they stand. A decision is no
consequence: `consequences add` refuses both with `entryTargetUnknown`.
`show`, `edit`, `status` and `entries move --bottom` read a decision named
by its short identifier once and take what they need from that read.

The text views lead with the short identifier: a line of the list with it in
front of the ID, `show` with a `Short ID:` line below the bare sentence,
confirmations with it in front of the sentence. Every address a text view
gives is `shortUrl`, or `appUrl` for a decision without one
([C-174](../decisions.md#c-174)).

## Reading

`GET projects/{projectId}/decisions` lists the tools in use, `GET
decisions-tools/{toolId}` one by its ID alone (C-120), `GET
decisions-tools/{toolId}/statuses` its statuses in their order. A status
carries `id`, `name`, `customName` and `role`; a status the team added has no
role, and a decision from before its tool had statuses carries a status
without an ID, which holds. A client is offered no draft.

`decisions list <toolId>` reads `GET decisions-tools/{toolId}/decisions`, the
day decided on last first and one without a day by the day it was written,
not paged. The text names each decision by its short identifier and ID,
sentence, status, day and labels, with its reason below, so that what holds is looked up at a glance;
a status an agent set by hand says "set by an agent", one Minka moved "set by
Minka".
Every status is listed unless `--status` names one; `--holds` is `--status
holds`. A status is named by its ID or by its role, in any case; it goes
out in lowercase, the form Minka keeps both in and compares them in
([C-165](../decisions.md#c-165)). `--label` takes an ID or the name of a label
of the project, looked up before the list is read
([C-171](../decisions.md#c-171)); `--applies-to-card` and `--applies-to-event`
narrow to one subject, `--set-by-agent` (`?setBy=agent`) to the decisions
whose status an agent set by hand. Minka refuses any other value of `setBy`
with `decisionSetByUnknown`, which the CLI never sends. Decisions written down
at once stand in the order of their agenda, one written down alone after them
on the same day and moment.

`agendaNumber` stands on the list and on a decision: its place among the
decisions written down with it, counted from 1, null for one written down
alone. The list gives it as `agenda item 2`, `show` as an `Agenda item:`
line.

`statusSetBy` stands on the list and on a decision: `person` (null once
purged), `agent`, true where an agent set the status by hand, `minka`, true
where Minka moved it and `person` is whom its line names, and `at`; null where
nobody is known. The writer sets it on adding, whoever changes the status
afterwards, and the writer of a later decision whose coming into force moved
the earlier one. A save that keeps the status keeps who set it. `show` gives it
as `Status set: <when> by <who>`, an agent marked as such, Minka as "Minka, on
behalf of" the person.

`decisions show` gives the decision with `body` and `bodySource`, its files,
labels, comments (oldest first), `statusSetBy`, `decidedBy` (each `{person,
name}`), `decidedIn`, `votes`, `supersedes` or `revokes` and `supersededBy` and
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

`--label`, repeatable, takes an ID or the name of a label of the project.
Every one is looked up among the project's labels before anything is
written: one the project lacks is exit 2, two of one scope exit 9 with
`meta.apiCode` `labelsShareAScope`, as Minka refuses them, and a label named
twice goes once ([C-178](../decisions.md#c-178)). Since Minka
takes no labels with one new decision, a single decision is written first and
takes each label right after, one `POST decisions/{id}/labels` each; a label
that then fails ends the run with `data.created`, `data.labelled` and
`data.notLabelled`, the decision staying written. Several decisions carry
their labels in `labelIds` of the one request.

## Several at once

`--title` given more than once, or `--titles-from <file|->` with one sentence
per line, writes several decisions with `POST
decisions-tools/{toolId}/decisions/batch` from `titles`, all or none, in their
order; Minka numbers them one after the other and gives each its
`agendaNumber`. A line that is empty or only spaces is left out, every other
one goes as typed, list markers such as `1.` or `TOP 3:` included: stripping
them is a convenience of Minka's web form for pasted text, which neither the
API nor the CLI does ([C-177](../decisions.md#c-177)). `--title` and
`--titles-from` exclude each other; `--titles-from -` beside `--body -` is
exit 1, since both would read stdin. One sentence, of a file too, is the single create and
takes every flag. Two or more share `--decided-on`, `--decided-by`,
`--event`, `--status`, `--applies-to-card`/`--applies-to-event`,
`--audience`/`--notify`, `--visible-to-clients` and `--label`; any other flag
beside them is exit 1 before any request ([C-176](../decisions.md#c-176)).
Success hands over `data` as the list of the decisions in their order.

Refused sentences are 422 `decisionLinesRefused`, exit 9, nothing written:
`data.lines` carries each refused one with Minka's `index` (from 0 in the
order sent), `code` (`decisionTitleMissing`, `decisionTitleTooLong`,
`decisionTitleHasControlCharacters`) and `message`, and the CLI's `title` and
`from`, where it was given: `--title 2` or `line 5 of agenda.txt`, a line
counted in the file with the empty ones. `decisionsMissing` and
`decisionsTooMany` (beyond 100, left to Minka) and `labelsShareAScope` get a
hint; a client sending labels is 404.

`--from <file|->` writes several: a JSON list of objects in Minka's names of
the fields of a new decision, a body as Markdown unless `--body-format html`,
taken as written: a body of `-` is no call for stdin.
A field Minka does not take is exit 1 before any request; IDs and the
status go out in lowercase. `--from` is for decisions with fields of their
own and is not all or none: the decisions go one request each in the order
given, without an `agendaNumber`, and the first refusal ends the run with
`data.created`, the decisions as Minka answered, and `data.notCreated`, the
titles of the rest ([C-168](../decisions.md#c-168)). Sentences that share
their fields go all or none with `--title` or `--titles-from`.

## Whom a decision reaches

`--audience` and `--notify` of `create`, `edit`, `status`, `supersede` and
`revoke` go out as `audience` and `notifyIds`, and only where they are given:
Minka takes `everybody` where `audience` is absent, `chosen` with the people
of `notifyIds`, who then follow the decision, or `nobody`. A person of
`--notify` is an ID, `me`, an e-mail address or a name, looked up among the
members of the project, sent as IDs. Which people go with which audience is
Minka's rule and left to it: `decisionAudienceUnknown`, `notifyIdsMissing`,
`notifyIdsWithoutChosenAudience`, `personNotAProjectMember` (also for a value
that is no ID), `personCannotSeeIt` (a client chosen who is not shown the
decision) and 400 `malformedBody` (`notifyIds` no list of strings) each get a
hint. The CLI checks only the word of `--audience`, exit 1
([C-173](../decisions.md#c-173)).

The audience takes effect only where a decision comes into a status of role
proposed or holds: a new one proposed or in force, or a change that moves it
there from another status. A draft, another role and a status the team added
tell nobody; a move Minka makes tells nothing. A save that keeps the status
ignores both fields, and Minka checks them all the same, right after
`reviewOn`; whether the people chosen may be told is asked only where the
decision is announced. So `edit` refuses `--audience` and `--notify` without
`--status`, exit 1 before any request. In a `--from` list `audience` and
`notifyIds` are fields like the others, the IDs going out in lowercase.

## Changing

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
an entry: an address of this Minka, a `#` card name, or for the history a
decision by its `#` name or its `shortUrl` and a web address with
`--description`; a consequence is a to-do or a card. Naming one
there already answers with that entry. `decisions entries move` puts an entry
first, last, after or before another of its part; `--bottom` reads the
decision for the last entry of the part. `decisions entries delete` takes one
off.

## The order of the refusals

On a new decision: 415 and 400, the tool (404), then the title, the reason,
the body, the day, the status, who decided, the event and the votes, then the
earlier decision named twice or malformed, the shape of `appliesTo`,
`validUntil` and `reviewOn`, then the audience (its word, the shape of
`notifyIds`, a value that is no ID, then the people missing or without
`chosen`), then the archived project, then a subject and an earlier decision
that are unknown, then the people chosen. Several at once: 415 and 400, the
tool (404), `titles` (400 no list of strings, `decisionsMissing`,
`decisionsTooMany`, then `decisionLinesRefused` with every refused sentence),
the day, the status, who decided, the event, the shape of `appliesTo`, the
audience, then `labelIds` (400, a client 404, `labelUnknown`), then the
archived project, an unknown subject, `labelsShareAScope` and the people
chosen. A change follows the same order
after the decision (404), except that it meets the people chosen right after
the archived project, before an unknown earlier decision, which it meets
before an unknown subject. A change of a decision of the archive is 422
`decisionArchived` once the project is known to take writes, before the
people chosen; exit 9, with a hint that it is read and comes out of the
archive on the web only. The
people chosen are met in the order given, the writer and anybody met before
passed over, and only where the decision is announced; the first who cannot
be told decides: `personCannotSeeIt` for a client the decision is not shown
to, `personNotAProjectMember` for anybody else. An entry, a
label and a move of an entry meet the archived project, then
`decisionArchived`.
