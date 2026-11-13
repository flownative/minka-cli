# Meetings

Every event the reader sees has a **meeting**: the agenda of the decisions it
is to take, what it made of each, and whether it is closed. `events meeting
show` reads it, `events meeting agenda add`, `move` and `remove` keep the
agenda, `events meeting outcome` records what the meeting made of an item, and
`events meeting close` closes it for good. The commands live under `events`,
because a meeting has no address but under its event.

Everything here was read in Minka's code — `MeetingApiController`,
`AbstractApiController`, `ReadsDecisionFields`, `MeetingResource`,
`MeetingAgendaItemResource`, `MeetingOutcomeResource`, `DecisionLinkResource`,
`EventOccurrenceResource`, the handlers `PutDecisionsOnMeetingAgendaHandler`,
`RecordMeetingOutcomeHandler` and `CloseMeetingHandler`,
`DbalMeetingAgendaRepository`, `DbalDecisionRepository`,
`MeetingAgendaQueryService`, `MeetingOutcome`, `DecisionStatusRole` — and in
`ApiMeetingTest`, `ApiEventsTest` and `ApiMyCalendarTest`. What has been seen
live is in the [Status](../work-in-progress/status.md).

## What is not there

A meeting is never reopened, an outcome never taken back to none, and there
is no list of meetings: whether an event has an agenda is learnt by reading
its meeting, for a series day by day. A decision does not name the agendas it
stands on, and an event read by itself says nothing of its meeting.

## The day of a series

A series meets on each of its days, and each day has an agenda of its own. So
every address takes `day` as `YYYY-MM-DD`. `--day` goes **into the query,
never into a body** — Minka might read a body's `day` as the same argument —
and is sent whenever it is given ([C-183](../decisions.md#c-183)). It is held
to a day that exists before any request: exit 1 (`usage`).

- The day is the one **the schedule gives**, in the zone of the series: for an
  occurrence that was moved it stays the day it was moved from, and it is
  never the first day of the series. `events list` and `events mine` hand it
  out as `day` on every entry of `occurrences`, null for an event that does
  not repeat. The text view of both names it behind an occurrence that does
  not start on it: `Thu 2026-10-29 … (day 2026-10-27 of the series)`.
- A series without a readable `day` is 422 `meetingDayMissing`, and the hint
  says where the days are found. A day the schedule does not give is 404.
- A **cancelled** day is left out of `occurrences`, yet its meeting reads with
  `cancelled: true` and every flag false; every write to it is 404, and the
  hint of `agenda add` says that a cancelled day takes no agenda. A day that
  was cancelled or moved and that the schedule gives no more counts for
  nothing.
- An event that **does not repeat** reads no `day`, whatever it holds, so one
  sent along does no harm. Its meeting carries `day: null`.

## What a read hands over

`data` of every command but `agenda remove` is the meeting as Minka answers
it, unchanged: `eventId`, `projectId`, `toolId` (the calendar), `title` (the
event's), `day`, `cancelled`, `closed`, `closedAt`, `closedBy`, `mayClose`,
`items` and `appUrl`. An event without an agenda has `items: []`, is open and
has `mayClose: false`.

An item carries `id` — its own, not the decision's — `number`, `current`,
`decision` as `{id, title, status, archived, appUrl, shortId}`, `outcome` and
the flags `mayMove`, `mayTakeOff` and `mayRecordOutcome`. `number` counts the
items **the reader is shown** from 1 without a gap, and `current` marks the
first of them without an outcome. A client of the project is shown only the
decisions shown to them. `outcome` is null until recorded, else `{outcome,
status, withChanges, votes, votesFor, votesAgainst, abstentions, recordedBy,
recordedAt}`: the word, the status it put the decision in, and the votes
counted in this meeting, whatever the decision says since.

The flags are Minka's answer to what the reader may do, and the text view and
the breadcrumbs take it from them and from nothing else: a closed meeting
moves and takes off nothing and records only a correction; the item of an
archived decision can only be taken off; an archived project and a cancelled
day allow nothing.

The text view shows the state (open, closed with who and when, cancelled),
the day of a series, and the agenda numbered, the current item marked with
`>`, each decision with its short identifier, status and address, the item's
ID, its outcome with the votes and who recorded it, and what the reader may
do with it.

## Naming an item

An item is given by its ID, by its number as the agenda shows it, or by the
decision it carries — the decision's ID or its short identifier, in any case
([C-184](../decisions.md#c-184)). `move`, `remove` and `outcome` read the
meeting first and find the item there, as the step commands find a step: a
number always means a place, a number or a name on no item is exit 1
(`usage`) with the agenda in the hint, and an ID on no item goes out for
Minka to answer, which is 404. `--after` and `--before` take the same.

## Putting decisions on

`events meeting agenda add <eventId> <decision>…` sends all decisions in one
request, `{"decisionIds": […]}`, in the order given; they go to the end of
the agenda, all or none, and Minka answers 201 with the meeting. Minka takes
IDs only, so a short identifier is looked up first, as the `decisions`
commands do; a name that is neither is exit 1 before any request.

A decision qualifies where it belongs to any Decisions tool of the event's
project and is still to be taken: a status of role `proposed` or `deferred`,
or one the team added. It stands on one agenda once and may stand on several
meetings. Putting it on changes nothing on the decision.

## Moving and taking off

`events meeting agenda move` takes exactly one of `--top`, `--bottom`,
`--after <item>` and `--before <item>`, as `cards steps move` does, and sends
`previousItemId` or `nextItemId` in a JSON object, which is `{}` for the top.
The end has no word: `--bottom` sends the last other item **as read** as the
previous one, none where the item is the only one, and where that item is
gone by the move, the hint of `agendaPlaceUnknown` says so. It is the end of
the agenda as the reader is shown it: behind items hidden from them, as from
a client, others may follow. A neighbour is another item the reader is
shown, and its ID goes out in lowercase, the only form Minka reads there.

`events meeting agenda remove` deletes the item; the decision stays, with the
status an outcome gave it. Minka answers 204 without a body, so `data` is
what the command knows: `{eventId, day, itemId, decisionId}`. Unlike a step,
an item that is gone is **404**, a second removal too, and the command does
not paper over it.

## The outcome

`events meeting outcome <eventId> <item> <outcome>` puts
`…/agenda/{item}/outcome`. The outcome is one of four words, which the
command line spells with hyphens, takes in any case as it takes the role of a
status, and sends as Minka's: `accepted`, `accepted-with-changes`
(`acceptedWithChanges`), `rejected` and `deferred`. Any other word is exit 1
before any request.

An outcome **changes the decision**: it puts it into the status of its tool
that has the role `holds` (accepted, with changes or not), `rejected` or
`deferred`. A tool without any status is given the set a new one starts
with first, and one with a `proposed` and no `deferred` status that status;
any other role that is missing is `meetingOutcomeHasNoStatus`. The first
three outcomes make this meeting the day and the event the decision was taken
on — for an event that does not repeat its first day, or the day its start
falls on in UTC — and `deferred` takes them back only where they name this
very meeting. A decision that comes into force this way moves the earlier
decision it supersedes or revokes, as any other way into force does, and a
change of the sentence or of the votes keeps a version of the decision.
`accepted-with-changes` carries `--sentence`, the decision's new sentence,
which is required there and refused with any other outcome, both before any
request. Recording tells nobody.

The outcome is **the last one written**: there is no guard, a second write
corrects the first, and a correction does not bring an earlier sentence back.
A closed meeting still takes the correction of an outcome it recorded.

**The votes.** `--votes` is a note of one line, `--votes-for`,
`--votes-against` and `--abstentions` whole numbers from 0 to 99999, held to
that range before any request. The command **sends exactly the vote fields
given** and reads none to carry over ([C-185](../decisions.md#c-185)): a
field left out keeps its value, on the item and on the decision, and
`--no-votes` sends the note as null, which takes it off, as `decisions edit
--no-votes` does. A count has no flag that takes it off.

## Closing

`events meeting close <eventId> --audience everybody|chosen|nobody` closes a
meeting. It **cannot be undone**. `--audience` is required
([C-186](../decisions.md#c-186)), although Minka reads a missing one as
everybody: closing tells people, and nobody is told by a flag left out.
`--notify` names the people for `chosen`, as for a decision: IDs go out as
given, a name or an address is looked up among the members of the event's
project, for which the meeting is read. Which people go with which audience
is left to Minka. The summary names the audience sent and nobody by name,
since Minka's answer does not say whom it told.

Everybody is the humans of the project: no agent hears of a closed meeting
unless it is chosen. Whoever sees the event and may change every item on the
agenda, hidden ones too, may close it; an empty agenda is closed by nobody. `mayClose` says so
beforehand. Minka answers both with the 404 of an event that is not there,
so the hint names all three and does not pretend to know which.

## An older Minka

A Minka without meetings answers every address with a page of its web
instead of the API's own 404. Every command then exits 2 (`not_found`) and
says the host does not know meetings, as `cards next` does against a Minka
without its address. Its occurrences carry no `day`, and the text view names
none.

## The order of the refusals

Every write is checked in this order, and each step comes only where the one
before it passed; a read asks only the first three:

1. The event: 404 for one that is unknown, foreign, hidden from a client or in
   the trash, and for an identifier that is none.
2. A series without a readable `day`: 422 `meetingDayMissing`.
3. A day the schedule does not give and that is not cancelled: 404.
4. An archived project: 422 `projectArchived`.
5. A cancelled day: 404.
6. The item named, for a move, an outcome and a removal: 404 for an
   identifier that is none, an item the reader is not shown, and one they may
   not change; the item of an archived decision can only be taken off.
7. A closed meeting: 422 `meetingClosed`; for an outcome only where the item
   has none yet.
8. The body: 415, 400 `malformedBody`.
9. The fields, then what the write meets.

So a closed meeting is told before anything of the body is read. After that:

- **Putting on:** 400 for `decisionIds` that is missing, null or no list of
  strings; `decisionsMissing` for an empty list; `decisionUnknown` for an
  identifier that is none. Then every decision in turn: `decisionUnknown` for
  one that is unknown, in the trash or the archive, of another project or
  hidden, else `decisionNotForAMeetingAgenda` for one that is not open. Only
  then `decisionAlreadyOnMeetingAgenda`, also for an ID named twice.
- **Moving:** for `previousItemId` and then for `nextItemId`, each judged
  before the other is read: 400 for one that is no string;
  `agendaPlaceUnknown` for one that is no identifier in lowercase, the item
  itself or no item the reader is shown.
- **The outcome:** 400 for an `outcome` that is no string,
  `meetingOutcomeUnknown`; 400 for a `sentence` that is no string,
  `sentenceWithoutChanges`; the sentence as the title of a decision
  (`decisionTitleMissing`, `decisionTitleTooLong`,
  `decisionTitleHasControlCharacters`); the votes (400 for a note that is no
  string, `decisionVotesTooLong`, `decisionVotesHasControlCharacters`,
  `decisionVoteCountMalformed`); last `meetingOutcomeHasNoStatus`.
- **Closing:** 400 for an `audience` that is no string,
  `meetingAudienceUnknown`; 400 for `notifyIds` that is no list of strings;
  `personNotAProjectMember` for an identifier that is none; `notifyIdsMissing`;
  `notifyIdsWithoutChosenAudience`; then 404 for an empty agenda and for items
  the writer may not change; last `personNotAProjectMember` and
  `personCannotSeeIt` for the people chosen, which leave the meeting open.

| Answer | Command | Exit |
|---|---|---|
| 404 | every command, for an event or a day that is not there; every write to a cancelled day; `move`, `remove` and `outcome` for an item that is gone or not to be changed; `close` for an empty agenda and for items the writer may not change, which the hint says | 2 |
| 404 page | every command against a Minka without meetings | 2 |
| 422 `meetingDayMissing` | every command on a series without `--day`; the hint names `occurrences[].day` of `events list` | 9 |
| 422 `projectArchived` | every write | 9 |
| 422 `meetingClosed` | `agenda add`, `move`, `remove`, `close`, and `outcome` on an item without one | 9 |
| 422 `decisionUnknown`, `decisionNotForAMeetingAgenda`, `decisionAlreadyOnMeetingAgenda` | `agenda add`; none is put on | 9 |
| 422 `decisionsMissing` | unreachable: `agenda add` needs a decision | 9 |
| 422 `agendaPlaceUnknown` | `agenda move` with the item as its own neighbour, with `--after` or `--before` given an ID that is on no item, which goes out as it is, or behind a last item that is gone | 9 |
| 422 `meetingOutcomeUnknown`, `sentenceWithoutChanges`, `decisionTitleMissing`, `decisionVoteCountMalformed` | unreachable: `outcome` checks the word, the sentence and the counts itself | 9 |
| 422 `decisionTitleTooLong`, `decisionTitleHasControlCharacters`, `decisionVotesTooLong`, `decisionVotesHasControlCharacters` | `outcome` with a sentence or a note Minka does not keep | 9 |
| 422 `meetingOutcomeHasNoStatus` | `outcome` where the decision's tool has no status of the role | 9 |
| 422 `meetingAudienceUnknown` | unreachable: `close` checks the word | 9 |
| 422 `notifyIdsMissing`, `notifyIdsWithoutChosenAudience`, `personNotAProjectMember`, `personCannotSeeIt` | `close`; the meeting stays open | 9 |
