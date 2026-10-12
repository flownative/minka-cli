# Calendars and events

A calendar holds **events**: one-off, all-day or repeating. `events …` reads
and writes them; `events tools <projectId>` lists the calendars in use, and
`events tool <toolId>` gives one calendar by its ID alone, with `content`
(what it shows beside its events) next to the fields of every tool. There
is no way back out of the trash through the API.

## Commands

```bash
minka events tools <projectId> --json
minka events tool <toolId> --json
minka events list <toolId> --from YYYY-MM-DD --to YYYY-MM-DD --json
minka events mine --from YYYY-MM-DD --to YYYY-MM-DD --json
minka events settings <toolId> --content eventsAndDatedItems|eventsOnly --json
minka events show <eventId> --json
minka events create --tool <toolId> --title "Title" (--starts-at <moment> --ends-at <moment> | --first-day YYYY-MM-DD [--last-day YYYY-MM-DD]) [--repeat] [--rhythm <rhythm> ...] [--time-zone <zone>] [--participant <person>]... [--description - < event.md] [--join-url <url>] [--highlighted] --json
minka events edit <eventId> [--title "New"] [<time>] [--repeat [<details>] | --no-repeat] [--participant <person>... | --no-participants] [--description -] [--join-url <url>] [--highlighted=false] --json
minka events delete <eventId> --json
minka events comments list <eventId> --json
minka events comments create <eventId> - --json < comment.md
minka events comments edit <eventId> <commentId> - [--format html] --json < comment.md
minka events comments delete <eventId> <commentId> --json
minka events comments react <eventId> <commentId> 👍 --json
minka events comments unreact <eventId> <commentId> 👍 --json
```

## Reading

- `events list <toolId> --from <day> --to <day>` lists every event with an
  occurrence in the span, both days included and **both required**, at most
  366 days, not paged. `data` is `{from, to, timeZone, content, events,
  datedItems}`. The days are **yours**: the zone of your preferences in Minka,
  else the installation's, named by `timeZone`.
- **`datedItems` are no events.** While the calendar's `content` is
  `eventsAndDatedItems` (the default), they are the open to-dos and cards of
  the project due in the span, **whoever they belong to**, the earliest first,
  then by title with only A to Z lowercased (`10` before `9`), then by `id`:
  `kind` (`todo`, `card`), `toolId` (To-dos tool or board), `cardId`, `title`,
  `dueOn`, `placeTitle` (list or board) and `appUrl`. With `eventsOnly` the
  list is empty. They have a day and no time; read one with `todos show` or
  `cards show`. The text view puts them under a heading of their own.
- `events mine --from <day> --to <day>` is **My calendar**, across every
  project of yours that is not archived: `events` you **take part in** (each
  line with `toolId` and `project` besides), and `datedItems` assigned **to
  you**, steps included (`kind` `step`, `cardId` its card, `placeTitle` the
  card, `appUrl` opens the card), in the order of `events list`. A to-do of
  which only a subtask is yours is not there, nor is a card in Done or
  completed in another column, nor its steps. The span is refused as for
  `events list`; a calendar's `content` does not matter here.
- **Every event stands in the span once, a series too.** Lay out an agenda from
  `occurrences`, the times it takes place at in the span, never empty and in
  the order they start. The event's own `startsAt`/`endsAt`/`firstDay`/
  `lastDay` are its first occurrence, which for a series may lie years before
  the span. A line has no description.
- `events show <eventId>` gives the event with `description`,
  `descriptionSource`, its files, `schedule`, `timeZone`, `joinUrl`,
  `highlighted` and `participants`.
- A timed event carries `startsAt` and `endsAt` in UTC and `firstDay`/`lastDay`
  null; an all-day one the reverse, `allDay` says which. `schedule` is null for
  an event that does not repeat; its fields are those of a to-do series.

## Writing

- **The time** is `--starts-at` and `--ends-at` together, ISO 8601 **with an
  offset** (`2026-10-05T09:00:00+02:00`, `…Z`), or `--first-day` and optionally
  `--last-day`. A moment without an offset is exit 9 `eventTimeNotReadable`.
  An edit that names any of the four names the whole time anew.
- **Repeating:** on `create`, `--repeat` alone repeats weekly on the weekday
  of the first day; on `edit`, it keeps the stored schedule; `--rhythm`, `--weekdays`, `--interval`, `--day`, `--weekday`,
  `--instance`, `--month`, `--ends-on` work as for `todos repeat`. The details
  must fall on the first day of the event (`eventRepetitionDoesNotFit`). A
  timed series keeps its clock time in `--time-zone` (an IANA name such as
  `Europe/Berlin`, with its capitals; `CET` and `Europe/Kiev` are taken,
  `+02:00` is refused), else in the zone it has, else in yours.
- `events edit` sends only what was given. **Without `--repeat` or a detail, a
  series keeps its first day:** moving it is exit 9
  `eventRepetitionKeepsItsFirstDay`; give the new time **with** `--repeat` (and
  the details that hold the new day). `--no-repeat` stops the repetition.
- `--participant` names members, the **whole set** on an edit;
  `--no-participants` takes everybody off. An event the team adds is shown to
  no client: naming a client is exit 9 `personCannotSeeIt`.
- **No guard: the last write wins.** There is no `--expect` here.
- `--join-url ""` takes the link off, `--highlighted=false` the highlight. A
  description with files or mentions is not replaced by one without them
  unless `--drop-attachments` is given.
- `events delete` puts an event into the trash, a series with all its days; a
  client only an event of their own.
- `events settings <toolId> --content eventsAndDatedItems|eventsOnly` chooses,
  for everybody, whether the calendar's span carries what is due. Minka's
  words, as written: anything else is exit 9 `calendarContentUnknown`. Only
  the team chooses: a client is exit 2, after the value and the archive.

## Comments

- `events comments create|edit|delete|react|unreact <eventId> …` work as under
  a card, and `events comments list <eventId>` reads them, the oldest first.
  **`events show` carries only `commentCount`**: the comments, with their
  `bodySource`, come from `events comments list`.
- A series has its comments **on the event as a whole**, never on one
  occurrence. A calendar no longer in use takes comments; an event in the
  trash is exit 2; in an archived project every write is exit 9
  `projectArchived`, reading stays open.
- Only the author changes or removes a comment; somebody else's is exit 2.
  `events comments edit` reads the event and its comments first, for the
  files and mentions a comment of yours may carry.

## The order of the refusals

Exit 2 for a calendar or event not there **before** anything else; then the
type of every field (400); then title, time, `--join-url`, a participant that
is no ID, the schedule, the zone, `--ends-on`, the schedule against the first
day, the description; then `projectArchived`; then the people
(`personNotAProjectMember`, `personCannotSeeIt`).

## Codes of this part

`eventsSpanNotADay`, `eventsSpanOutOfOrder`, `eventsSpanTooLong`,
`eventTitleMissing`, `eventTitleTooLong`, `eventTitleHasControlCharacters`,
`eventTimeMissing`, `eventTimeAmbiguous`, `eventTimeNotReadable`,
`eventEndsBeforeItStarts`, `eventJoinUrlNotAnAddress`,
`eventDescriptionTooLong`, `eventDescriptionNotReadable`,
`eventRepetitionScheduleInvalid`, `eventRepetitionDatesNotADate`,
`eventRepetitionDatesOutOfOrder`, `eventRepetitionTimeZoneUnknown`,
`eventRepetitionDoesNotFit`, `eventRepetitionKeepsItsFirstDay`,
`calendarContentUnknown`, `personNotAProjectMember`, `personCannotSeeIt`,
`projectArchived`, all exit 9.
