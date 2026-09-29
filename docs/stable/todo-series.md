# Repeating to-dos

A **series** makes a to-do again and again on the days of its schedule. It is
made from a to-do with a due day, which becomes its first occurrence, and it
stands in the list of that to-do ([To-dos and subtasks](todos.md)). Every
further occurrence is an ordinary to-do at the end of the list, due on its day.
A series is read through its list, so whoever may not read the list finds no
series of it, and it is never deleted, only stopped. `todos repeat` makes one,
`todos series …` reads and writes it ([C-089](../decisions.md#c-089)).

## What a read hands over

`GET todo-series/{seriesId}` answers the series with where it stands
(`projectId`, `toolId`, `listId`, `groupId` and `project`, `tool`, `list`,
`group` as references), `title` and `notes` the occurrences start from,
`schedule`, `leadDays`, `state`, `endedAt`, `nextOccursOn`, `nextShowsOn`,
`canShowNow`, `assignees`, `notified`, `createdAt`, `creator`, `updatedAt` and
`appUrl`. `GET todo-lists/{listId}/series` answers every series of a list in
that form, stopped ones too, the one made first first.

- `state` is `running`, `stopped`, or `finished`: not stopped, but with no
  further day before its end.
- `nextOccursOn` is the day of the upcoming occurrence, null while stopped or
  finished. A series that fell behind owes only the latest day not after
  today. While the series holds an occurrence not yet due, it is the day
  **after** that one.
- `nextShowsOn` is the day that occurrence appears in the list: its lead
  ahead, and not before the day of the occurrence made last. It is null where
  the occurrence comes with the next round or is there already, and while the
  list is archived.
- `canShowNow` says whether `show-now` would make an occurrence; false while
  the list is archived.
- `group` and `groupId` are null where the series makes its to-dos loose in the
  list, also once its group is in the trash.
- `assignees` and `notified` name only members who see the list; somebody
  hidden meanwhile stays stored and is back once visible.

**The CLI works out no day of a series.** "Today" is the installation's day,
not the person's, and the rules above are Minka's reckoning; what the CLI shows
is what Minka answered.

A to-do says what it stands for: `todos show` carries `series` as `{id,
occursOn}`, the day of the series it is, or null, and a line of `todolists
show` carries `seriesId`. The text of `todos show` names the series, that of
`todolists show` marks the line `repeats`.

## The schedule

| Field | Rhythms | Flag |
|---|---|---|
| `rhythm` | all | `--rhythm` |
| `weekdays` | `daily`, `weekly` | `--weekdays`, repeatable, a comma list |
| `interval` | `weekly`, `monthlyOnDay`, `monthlyOnWeekday` | `--interval` |
| `day` | `monthlyOnDay`, `yearly` | `--day` |
| `weekday` | `monthlyOnWeekday` | `--weekday` |
| `instance` | `monthlyOnWeekday` | `--instance`, 1 to 4 or `last` |
| `month` | `yearly` | `--month` |
| `startsOn` | all, on an edit only | `--starts-on` |
| `endsOn` | all | `--ends-on`, `""` takes the end off |

A weekday is given as a number from 1 (Monday) to 7 or as a name such as `mon`,
read as for check-ins; a number goes out as it is, so that Minka says what it
makes of one outside the week. `--instance last` goes out as Minka's `-1`.

Minka checks every field sent for its type and range and then ignores a field
the rhythm does not use. So that such a field is not dropped without a word,
the CLI refuses it as a usage error where `--rhythm` is given in the same call,
and says in a `notice` which flags Minka ignored where the rhythm is only known
from the answer.

A monthly day a month lacks falls on its last day; a yearly day has to exist in
its month in a leap year, and 29 February falls on the 28th in other years.

## Making a to-do repeat

`POST todo-lists/{listId}/series` takes `todoId`, `schedule` and `leadDays`.
The address names the list, so `todos repeat` reads the to-do first for its
`listId`. The series starts on the to-do's due day, whatever the schedule says,
and takes its title, notes, group and people. Without `--rhythm` it repeats
weekly on the weekday of that day; the details not given follow from that day.
`--lead-days` is left to Minka's seven where it is not given. `data` is the
series.

## Editing, and the guard

`PATCH todo-series/{seriesId}` changes the occurrences made from now on. **The
CLI sends only what was given**, never a detail of the stored schedule: Minka
keeps day, weekday, instance and month while rhythm and first day stay, keeps
weekdays and interval while the rhythm stays, and takes the rest from the first
day. A client that echoed the stored details would pin them where Minka would
have let them follow a new first day. So `todos series edit` reads the series
only to look up a person given by name, address or `me`, and never sends what
it read.

`--assignee` and `--notify-when-done` are the whole set, sent as a list; `[]`
takes everybody off, and a set left out stays untouched. Minka takes a person
named twice once. The notes of a series cannot be changed.

Like a document, a series is guarded: `--expect <updatedAt>` sends
`expectedUpdatedAt`, and a series written since — edited, stopped, resumed, or
an occurrence made — is refused with 409 `todoSeriesChanged`, exit 9, with the
series as it stands as `data` and printed, and a hint naming its `updatedAt`.
`--force` writes over whatever stands, and one of the two is required;
`--expect none` belongs to the notes of a card. `updatedAt` is kept to the
microsecond, so the guard sees every write.

## Stop, resume, show now

- `stop` ends the series; the occurrences it made stay. A finished series can
  be stopped too.
- `resume` continues a stopped series with its first day **after today**: the
  days that passed while it was stopped are not made up. A series that is not
  stopped, a finished one included, is `todoSeriesNotStopped`.
- `show-now` makes the upcoming occurrence at once. Minka answers 201 with the
  series and names the new to-do only in `Location`, so `data` is `{series,
  todoId}`: the series unchanged, and the ID read from `Location`, null with a
  `notice` where the header names no to-do. A series holds at most one
  occurrence not yet due.

## The order of the refusals

What Minka says, in this order: what is wrong with the request itself,
**field by field in the controller's order**, each field's shape (400) and
value (422) before the next field is read — so a 422 of an earlier field comes
before a 400 of a later one: on an edit `title: ""` is 422 `todoTitleMissing`
before `assigneeIds: null` is 400, and on `repeat` `todoId` is read first and
an out-of-range `leadDays` is 422 before `schedule: 5` is 400. The fields of an
edit are read as title, `assigneeIds`, `notifiedIds`, `schedule`, `leadDays`,
`expectedUpdatedAt`; those of `repeat` as `todoId`, `leadDays`, `schedule`. In a coming
change the Minka side moves every 400 ahead of every lookup and of every 422.
Then exit 2 for a list or series not there or not readable, the same
for a stranger and for a client the list is hidden from. Then, for `repeat`,
the to-do (`todoToRepeatUnknown`, `todoRepeatsAlready`,
`todoSeriesNeedsDueDay`); for `repeat` and an edit, the days in order and a
schedule that cannot hold (`todoSeriesDatesOutOfOrder`,
`todoSeriesScheduleInvalid`). Then `projectArchived`, then `todoListArchived`.
After those, on an edit `todoSeriesChanged` and then the people
(`personNotAProjectMember`); on `stop` and `show-now` `todoSeriesStopped`, on
`resume` `todoSeriesNotStopped`, and on `show-now` then `todoSeriesFinished` or
`todoSeriesOccurrenceAhead`. Reading stays open in an archived project and
list.

## Codes

`todoToRepeatUnknown`, `todoSeriesNeedsDueDay`, `todoRepeatsAlready`,
`todoSeriesScheduleInvalid`, `todoSeriesDatesNotADate`,
`todoSeriesDatesOutOfOrder`, `todoSeriesLeadDaysInvalid`, `todoTitleMissing`,
`todoTitleTooLong`, `todoTitleHasControlCharacters`, `personNotAProjectMember`,
`nothingToChange`, `todoSeriesStopped`, `todoSeriesNotStopped`,
`todoSeriesOccurrenceAhead`, `todoSeriesFinished`, `projectArchived`,
`todoListArchived` (422) and `todoSeriesChanged` (409), all exit 9, each with a
hint that says what can be done. `nothingToChange` is caught by the CLI as a
usage error before anything goes out.
