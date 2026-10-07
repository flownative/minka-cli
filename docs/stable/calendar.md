# Calendars and events

A calendar of a project holds **events**: one-off, all-day or repeating.
`events tools` lists the calendars of a project, `events list` and `events
show` read, `events create`, `edit` and `delete` write
([C-098](../decisions.md#c-098)), and `events comments` reads and writes the
comments under an event ([C-095](../decisions.md#c-095)). Beside its events a
calendar shows what of its project is due, unless its team chose otherwise
with `events settings`, and `events mine` reads My calendar, across projects
([C-106](../decisions.md#c-106)). `events tool <toolId>` reads one calendar by
its ID alone: the fields of every tool read so ([C-120](../decisions.md#c-120))
and `content`, what it shows beside its events.

Everything here was read in Minka's code — `CalendarApiController`,
`ReadsRepetitionSchedule`, its resources, `CalendarEventQueryService`,
`EventOccurrences`, `EventRepetition`, `EventTime`, `JoinLink`, `TimeZone`,
the event handlers, `EventParticipants`, `DbalCalendarEventRepository`,
`ApiEventsTest`, `EventCommentsApiController` and `ApiEventCommentsTest`, and
for what a calendar shows and My calendar `CalendarDatedItemQueryService`,
`MyCalendarQueryService`, `CalendarDatedItem`, `OpenCard`, the readers of
one's own cards, to-dos and steps,
`ChooseCalendarContentHandler`, `DbalCalendarSettingsRepository` and
`ApiMyCalendarTest`. What has been seen live is in the
[Status](../work-in-progress/status.md).

## What is not there

Taking an event **out of the trash**, **showing an event to clients** or
hiding it, renaming or removing a calendar, and changing or leaving out a
**single occurrence** of a series are web only.

## What a read hands over

`GET projects/{projectId}/calendars` lists the calendars in use as `id`,
`title` and `appUrl`; one no longer in use is not listed and is still read and
written at its own address. `GET calendars/{toolId}` answers a calendar with
its `projectId`, `project` and `content`, which `events create` reads to look
up a person given by name.

`GET events/{eventId}` answers the event: `id`, `projectId`, `toolId`,
`project`, `tool`, `title`, `allDay`, the time, `schedule`, `timeZone`,
`description`, `descriptionSource`, `attachments`, `joinUrl`, `highlighted`,
`participants` (in the order they were put on, those of one write among
themselves in no promised order), `commentCount`, `createdAt`,
`creator` and `appUrl`. There is **no `updatedAt`**.

- **The time.** A timed event carries `startsAt` and `endsAt`, moments in UTC
  to the microsecond, and `firstDay` and `lastDay` null. An all-day event
  carries `firstDay` and `lastDay`, both included and the same day twice for
  one day, and the moments null. `allDay` says which.
- **`schedule`** is null for an event that does not repeat; otherwise it has
  the fields of a to-do series ([Repeating to-dos](todo-series.md)). A series
  is one event: its time is that of its first occurrence, and
  `schedule.startsOn` is always the day that occurrence starts on.
- **`timeZone`** is the IANA zone a timed series keeps its clock time in, and
  null for an all-day series and for an event that does not repeat. 10:00 in
  `Europe/Berlin` stays 10:00 there across a change of summer time, so the
  moments of the occurrences in UTC move by an hour.

## The span

`GET calendars/{toolId}/events?from=…&to=…` answers `{from, to, timeZone,
content, events, datedItems}`. **Both days are required** and included, at most 366 days counting
both ends, and nothing is paged. `events list` refuses a call without both
`--from` and `--to` as a usage error before anything goes out; the values
themselves are Minka's to judge (`eventsSpanNotADay`, `eventsSpanOutOfOrder`,
`eventsSpanTooLong`).

- **Whose days.** `from` and `to` are days of the person of the token: the zone
  of their preferences, else the installation's, which the top-level
  `timeZone` names. A query cannot name another zone. The installation may keep
  a name of the backward table, such as `CET`; the span is read in it all the
  same.
- **Every event stands in the span once**, a series too, with `occurrences`:
  the times it takes place at in the span, each shaped like the time of an
  event, in the order they start, and never empty. An event without an
  occurrence in the span is left out. The four time fields of the line are the
  event's own, for a series its first occurrence, which may lie long before the
  span. An agenda is laid out from the occurrences, not from the lines.
- An all-day occurrence counts when one of its days lies in the span; a timed
  one when a part of it lies between the start of `from` and the end of `to`,
  so one that started the evening before reaches in.
- **Order:** by the first occurrence in the span, an all-day one counted from
  midnight UTC of its first day, and then by when the event was added.
- A line carries neither `description` nor `project`, `tool`, `createdAt` and
  `creator`.

**The CLI works out no occurrence and no day.** What it shows is what Minka
answered; the text view only turns the moments into the local time of the
machine.

## What is due beside the events

`content` is `eventsAndDatedItems`, which a calendar has until somebody
chooses, or `eventsOnly`. With the first, `datedItems` holds the open to-dos
and cards **of the calendar's project** due in the span, whoever they belong
to; with the second it is `[]`. Each item is `id`, `kind` (`todo` or `card`),
`projectId`, `toolId` (the To-dos tool or the board), `cardId` (`null`),
`title`, `dueOn`, `placeTitle` (the list of a to-do, the board of a card) and
`appUrl`; it carries no `project`.

- **Left out:** a done to-do; a card that stands in Done or was completed in
  another column; what is in the trash or in the archive; a to-do in a list
  that is archived or in the trash, or in a group in the trash; a card in a
  column in the trash; what a client is not shown, and everything in a tool
  hidden from them or in the trash. A step never stands in a calendar's span.
- An **archived project** still carries its dated items: the span is read
  there as before.
- **Order:** by `dueOn`, then by `title` with only A to Z lowercased and
  compared byte by byte, then by `id`: `10` before `9`, `apple` before
  `Zebra`, `bx` before `By`, and `Äb` before `äa`, since an umlaut keeps its
  case. My calendar orders alike.
- A dated item is **no event**: it has a day and no time, no occurrences, and
  is read with `todos show` or `cards show`. The text view lists them under a
  heading of their own, after the events, with their kind and their place.

`events settings <toolId> --content eventsAndDatedItems|eventsOnly` sends
`PATCH calendars/{toolId}` with `{"content": …}` and answers the calendar.
`--content` is required and goes out as typed: Minka's two words, as `--rhythm`
takes Minka's words, and Minka is the one to refuse another
(`calendarContentUnknown`). The choice is the calendar's, for everybody who
reads it, and **the team's alone**; choosing what stands is 200. A calendar no
longer in use takes it. The order of the refusals: the 404 of a calendar not
there or not visible, before the body; then the media type and the JSON, and
`content` no string (400); `{}` or `content: null` is `nothingToChange`, which
the CLI cannot send; another value `calendarContentUnknown`; then
`projectArchived`, for a client of the project too; and only then the 404 of a
client, the same body as for a calendar not there. A Minka without the address
answers with a page of its web: exit 2, and the message says that the host
cannot choose, not that the calendar is missing.

## My calendar

`events mine --from … --to …` reads `GET me/events?from=…&to=…`, which answers
`{from, to, timeZone, events, datedItems}` — no `content`: it belongs to no
calendar, and what a calendar chose to show does not touch it. The span is
read, bounded and refused exactly as that of a calendar, in the reader's days,
and the CLI refuses a call without both days as a usage error.

- It reaches every project the reader is a **member of and that is active**;
  an archived project, and one they left, stand nowhere in it.
- **`events`** are those the reader **takes part in**, wherever they were
  added, under the same rules of visibility and occurrence as a calendar's
  span, each line with `toolId` and `project` (`id`, `name`) besides. A
  calendar no longer in use still counts. Order: by the first occurrence in
  the span, then by when the event was added, across calendars.
- **`datedItems`** are the open to-dos, cards and steps **assigned to the
  reader** and due in the span, each shaped as in a calendar's span with
  `project` (`id`, `name`) besides. A step has `kind` `step`, `toolId` its
  board, `cardId` its card, `placeTitle` the title of its card and `appUrl` the
  page of its card. A to-do of which only a subtask is the reader's stays out,
  and a subtask is never an entry. A card is left out where a calendar's span
  leaves it out, in Done or completed in another column, and its steps with
  it; so are a step that is done, a card in the archive or the trash, and what
  a client is not shown. Order: as in a calendar's span.
- A Minka without the address answers with a page of its web: exit 2, and the
  message says that the host has no My calendar.

## Writing

**The time** is given in one of two ways, on `create` and on `edit` alike:
`--starts-at` and `--ends-at` together, each ISO 8601 with seconds and an
**offset** (`+02:00`, `+0200` or `Z`, at most fourteen hours, a fraction of up
to six digits), the start in the year 1 or later once told in UTC, the end
not before the start, asked in this order, so a start before the year 1 is
`eventTimeNotReadable` whatever the end; or `--first-day` and optionally
`--last-day`. Minka refuses moments and days at once (`eventTimeAmbiguous`)
and one moment alone (`eventTimeMissing`); the CLI refuses both as a usage
error before anything goes out, and a `create` without any time as well. An
`edit` that names any of the four flags names the whole time anew, and may
turn a timed event into an all-day one and back. The CLI passes a moment on
as it was typed and assumes no zone for one without an offset: Minka refuses
it with `eventTimeNotReadable`.

**The other fields.** `--title` is required on `create`; an empty one is
`eventTitleMissing`. `--description` is Markdown converted to HTML here, or
HTML with `--description-format html`, and may be empty. `--join-url` is an
address starting with `http://` or `https://`, at most 2000 characters;
`--join-url ""` takes it off. `--highlighted` sets the highlight and
`--highlighted=false` takes it off.

**People.** `--participant` takes an ID, `me`, an e-mail address or a name,
looked up among the members of the project, which `create` reads the calendar
and `edit` the event for, and only where a name or an address is given. On an
edit it is the **whole set**: whoever takes part and is not named again is
taken off, those named again keep their place and newcomers follow, among
themselves in no order Minka promises; `--no-participants` sends `[]`. Only
a member of the project the event is shown to can take part: what the team
adds is shown to no client, so a client named is `personCannotSeeIt`, and
somebody outside the project `personNotAProjectMember`.

**No guard.** A change takes no `expectedUpdatedAt` and the event carries no
`updatedAt`: the last write wins. A change that asks for what stands already
answers 200 and writes nothing. A description with files, mentions or
references is not replaced by one without them unless `--drop-attachments` is
given; `events edit --description` reads the event first to know.

## Repeating

`--repeat` sends a schedule, `{}` where no detail is given: on `create` that
repeats weekly on the weekday of the first day, on `edit` it keeps the stored
schedule from the first day. The details are the flags of `todos repeat`
without `--lead-days` and `--starts-on`, and follow the same rules: a detail
`--rhythm` does not use is a usage error, one Minka ignored for the rhythm it
kept a `notice`. A detail implies `--repeat`. The first day of the event is
the first day of the series, whatever the details say, and a schedule that
does not fall on it is `eventRepetitionDoesNotFit`. A daily series falls on
Monday to Friday unless `--weekdays` says otherwise.

`--time-zone` names the IANA zone a timed series keeps its clock time in. Left
out, it is the zone the series has, else the writer's own. Minka takes every
name PHP lists, the backward table included: `CET`, `GMT`, `Europe/Kiev` and
`US/Pacific` are kept as given. A name is held to that list exactly, capitals
and all, so `europe/Berlin`, a name that is none such as `Mars/Base`, an offset
such as `+02:00`, and a writer whose own zone is none of the list are
`eventRepetitionTimeZoneUnknown`. Minka reads it only
together with a schedule and ignores it for an all-day event, so the CLI
refuses `--time-zone` without `--repeat` or a detail, and beside `--first-day`,
as a usage error, and says in a `notice` when the event Minka answered with is
all-day.

On `edit`, what becomes of the repetition follows what is given:

| Given | The event |
|---|---|
| neither `--repeat` nor a detail | repeats as before, or stays an event that does not repeat |
| `--repeat`, a detail, or both | repeats by this schedule from its first day on; a detail not given is taken from the stored schedule while rhythm and first day stay, else from the first day |
| `--no-repeat` | `schedule: null`: stops repeating and stays where its first occurrence was |

`--no-repeat` beside `--repeat` or a detail is a usage error. A change without
a schedule cannot move the first day of a series or turn it between timed and
all-day: `eventRepetitionKeepsItsFirstDay`. Moving the clock time on the same
day is fine. To move a series, the new time goes with `--repeat`, and with the
details that hold the new first day where the stored ones do not.

`events delete` puts the whole series into the trash.

## Clients

A client reads only a calendar and the events shown to clients, changes such
an event, and puts only an event of their own into the trash; everything else
answers the 404 of an event that is not there. What a client adds is shown to
clients.

## The order of the refusals

What Minka says, in this order: exit 2 for a calendar or an event not there or
not visible, before the body is read. Then the media type and the JSON, then
the **type of every field** (400): `title`, `description`, `joinUrl`,
`highlighted`, `participantIds`, `timeZone`, the four fields of the time, then
`schedule` and its fields. On an edit then `nothingToChange`, which the CLI
catches as a usage error before anything goes out. Then each value (422):
the title, the time, the link, a participant that is no identifier
(`personNotAProjectMember`), the fields of the schedule, the zone, `--ends-on`
against the first day, the schedule itself, the schedule against the first
day — or, on an edit without one, `eventRepetitionKeepsItsFirstDay` — and the
description. Then `projectArchived`, also for a change that would change
nothing, and only then the people, one at a time in the order given
(`personNotAProjectMember`, `personCannotSeeIt`). Reading stays open in an
archived project.

## Comments

The comments under an event have addresses of their own —
`events/{eventId}/comments…` — in the shape of the comments under a to-do, with
the same codes and reactions, answered by `EventCommentsApiController` on the
base the controllers of to-dos, posts and Docs & Files share. `events comments`
names the event as the first argument and is built from the same commands as
`todos comments`. A series has its comments **on the event as a whole**: an
occurrence has no identifier of its own, and none of its own comments.

**`events show` carries no comments**, only `commentCount`. `events comments
list` reads them, and `events comments edit` reads the event and its comments
first, for the files and mentions a comment of one's own may carry; the guard
stands aside in an archived project ([C-082](../decisions.md#c-082)).

The event is looked up as for any other request: an event in the trash, one of
a project the person is no member of, and for a client one not shown to
clients or in a calendar hidden from them answer 404 at all six addresses. A
calendar no longer in use takes comments. Minka reads the
body before it looks for the event on a write, a change and a reaction, so a
body that is no JSON (400) or of another type (415) answers before the 404 —
unreachable from here. Then: the event (404), a comment that is no identifier
or not under this event (404), the text (`commentMissing`, `commentTooLong`,
`commentHasControlCharacters`, `commentNotReadable`) or the reaction
(`reactionMissing`, `reactionTooLong`, `reactionHasControlCharacters`), then
`projectArchived`, and last the 404 of a comment that is not one's own. In an
archived project every write is `projectArchived`, somebody else's comment
included; reading stays open.

## Codes

`eventsSpanNotADay`, `eventsSpanOutOfOrder`, `eventsSpanTooLong`,
`eventTitleMissing`, `eventTitleTooLong`, `eventTitleHasControlCharacters`,
`eventTimeMissing`, `eventTimeAmbiguous`, `eventTimeNotReadable`,
`eventEndsBeforeItStarts`, `eventJoinUrlNotAnAddress`,
`eventDescriptionTooLong`, `eventDescriptionNotReadable`,
`eventRepetitionScheduleInvalid`, `eventRepetitionDatesNotADate`,
`eventRepetitionDatesOutOfOrder`, `eventRepetitionTimeZoneUnknown`,
`eventRepetitionDoesNotFit`, `eventRepetitionKeepsItsFirstDay`,
`calendarContentUnknown`, `personNotAProjectMember`, `personCannotSeeIt`,
`projectArchived` and `nothingToChange` (422), and under the comments those of
a comment and a reaction, all exit 9, each with a hint that says what can be
done.
