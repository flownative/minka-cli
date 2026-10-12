# To-dos

## Commands

```bash
minka todolists tools <projectId> --json
minka todolists tool <toolId> --json
minka todolists list <toolId> [--archived] --json
minka todolists show <listId> --json
minka todolists create --tool <toolId> --title "Title" [--description - < body.md] --json
minka todolists templates --json
minka todolists start --tool <toolId> --template <templateId> [--title "Title"] --json
minka todolists edit <listId> [--title "New"] [--description - < body.md] --json
minka todolists archive <listId> --json
minka todolists unarchive <listId> --json
minka todolists delete <listId> --json
minka todolists comments list <listId> --json
minka todolists comments create <listId> - --json < comment.md
minka todolists comments edit <listId> <commentId> - [--format html] --json < comment.md
minka todolists comments delete <listId> <commentId> --json
minka todolists comments react <listId> <commentId> 👍 --json
minka todolists comments unreact <listId> <commentId> 👍 --json
minka todogroups create --list <listId> --title "Title" [--color <colour>] --json
minka todogroups edit <groupId> [--title "New"] [--color <colour>] --json
minka todogroups move <groupId> (--top | --after <groupId> | --before <groupId>) --json
minka todogroups delete <groupId> --json
minka todos mine [--limit <1-100>] [--after <todoId>] --json
minka todos show <todoId> --json
minka todos completed <listId> [--limit <n>] [--before <next>] --json
minka todos create --list <listId> --title "Title" [--group <groupId>] [--notes - < notes.md] [--starts-on <day>] [--due <day>] [--assignee <person>]... [--notify-when-done <person>]... --json
minka todos edit <todoId> [--title "New"] [--notes - < notes.md] [--starts-on <day>|""] [--due <day>|""] [--assignee <person>]... [--no-assignees] [--notify-when-done <person>]... [--no-notify-when-done] --json
minka todos complete <todoId> --json
minka todos reopen <todoId> --json
minka todos move <todoId> (--top | --after <todoId> | --before <todoId>) [--list <listId>] [--group <groupId> | --no-group] --json
minka todos delete <todoId> --json
minka todos subtasks list <todoId> --json
minka todos subtasks create <todoId> "Text" [--assignee <person>] [--due <day>] --json
minka todos subtasks edit <todoId> <subtask> [--title "New"] [--assignee <person>|""] [--due <day>|""] --json
minka todos subtasks complete <todoId> <subtask> --json
minka todos subtasks reopen <todoId> <subtask> --json
minka todos subtasks move <todoId> <subtask> (--top | --after <subtask> | --before <subtask>) --json
minka todos subtasks delete <todoId> <subtask> --json
minka todos repeat <todoId> [--rhythm <rhythm>] [--weekdays <days>] [--interval <n>] [--day <n>] [--weekday <day>] [--instance <1-4|last>] [--month <n>] [--ends-on <day>] [--lead-days <n>] --json
minka todos series list <listId> --json
minka todos series show <seriesId> --json
minka todos series edit <seriesId> (--expect <updatedAt> | --force) [--title "New"] [--notes - < notes.md] [schedule flags] [--starts-on <day>] [--ends-on <day>|""] [--lead-days <n>] [--assignee <person>]... [--no-assignees] [--notify-when-done <person>]... [--no-notify-when-done] --json
minka todos series stop <seriesId> --json
minka todos series resume <seriesId> --json
minka todos series show-now <seriesId> --json
minka todos comments list <todoId> --json
minka todos comments create <todoId> - --json < comment.md
minka todos comments edit <todoId> <commentId> - [--format html] --json < comment.md
minka todos comments delete <todoId> <commentId> --json
minka todos comments react <todoId> <commentId> 👍 --json
minka todos comments unreact <todoId> <commentId> 👍 --json
```

## To-do lists and groups

A To-dos tool holds **lists**, the newest on top. A list has a title, an
optional description and **groups**: named headings in an order of their own.
A to-do stands in its list directly or in one of its groups. The to-dos
themselves and their subtasks are `todos …`, see the next part.

### Reading

- `todolists tools <projectId>` lists the To-dos tools of a project in use; a
  project can have more than one.
- `todolists tool <toolId>` gives one To-dos tool by its ID alone as `{id,
  projectId, project {id, name}, title, appUrl}`, one the project no longer
  lists included. `posts tool`, `docs tool`, `checkins tool` and `events
  tool` do the same for their kind of tool; a client gets exit 2 for a tool
  not shown to clients.
- `todolists list <toolId>` lists the lists on the page of the tool, in its
  order; `--archived` lists **the archive instead**, the list archived last
  first. A list is on one of the two, never on both, so one missing from the
  first is looked for in the second. Nothing is paged and nothing is cut. A line
  carries the whole description and how many to-dos are open and done.
- `todolists show <listId>` gives the list with its description, its files,
  the open to-dos standing in no group (`todos`), the groups in their order with
  the colour (`color`, `null` for none) and the open to-dos of each, and the
  comments. **Done to-dos are counted
  (`completedCount`) and not listed.** A list in the archive is shown too and
  says `archived: true`.
- A group has no address to read it at: its ID and its place come from
  `todolists show`.

### Writing

- `todolists create --tool <toolId> --title "..."` starts a list **on top** of
  the tool. `--description` is Markdown by default and converted here, as for a
  card; with `--description-format html` a text carrying no element Minka serves
  is kept as its characters, and the `notice` is the only warning you get.
- `todolists edit <listId>` changes the title, the description or both; what
  you do not give stays. `--description ""` empties the description. **`--title
  ""` does not empty the title**: exit 9 `todoListTitleMissing`. A description
  with files or mentions is not replaced unless `--drop-attachments` is given;
  to keep them, edit `descriptionSource` and send it with
  `--description-format html`.
- `todolists archive` and `todolists unarchive` move a list in and out of the
  archive of its tool; it comes back to the place it had. `todolists delete`
  puts a list into the trash from either place, its groups and to-dos with it.
- `todolists comments create|edit|delete|react|unreact <listId> …` work as
  under a card, and `todolists comments list <listId>` reads them, the oldest
  first. Only the author changes or removes a comment; somebody else's is
  exit 2.
- `todogroups create --list <listId> --title "..."` adds a group **at the end**
  of the list, with the colour `--color` names or none. `todogroups edit
  <groupId>` changes the title, the colour or both, and needs at least one of
  `--title` and `--color`; both are written together or not at all. A colour
  is one of `white`, `red`, `orange`, `yellow`, `green`, `blue`, `aqua`,
  `purple`, `gray`, `pink` and `brown`, spelt exactly so. **`--color white`
  takes the colour away**: a group given white reads `color: null`. Any other
  value, `""` included, is exit 9 `todoGroupColorUnknown`, and nothing of the
  edit is written. `todogroups delete` trashes a group **with its to-dos**,
  which then leave the list and its counts.
- `todogroups move <groupId>` needs **exactly one** of `--top`, `--after
  <groupId>` and `--before <groupId>`; the neighbour has to be another group of
  the same list, and the group itself or any other is exit 9
  `todoPlaceUnknown`. A group never leaves its list.

### Lists from templates

The organization's **templates** are lists of a hidden template library, the
team's alone: a client, and anybody outside the team, is in none.

- `todolists templates` lists the templates in use, in their order, as lines of
  `todolists list`, and names the library's `projectId` and `toolId`. A person
  in no library is exit 2.
- `todolists start --tool <toolId> --template <templateId>` makes a list **on
  top** of a To-dos tool of a project: the template's description, groups with
  their colours, to-dos with notes and subtasks. It starts fresh: nothing done,
  **no days at all**, no comments, no series. The title is the template's unless
  `--title` is given (`--title ""` is exit 9 `todoListTitleMissing`). There is
  no flag for a description, days or people: Minka takes none here.
- **Read `leftOff`.** People on the template's to-dos and subtasks who are no
  members of the project are not carried; `data.leftOff` names them (`[]` for
  nobody), and so do the `summary` and the text. Somebody no longer on the team
  is left off without being named.
- **Every call makes a new list**: a second run is a second list, so read the
  tool first where a run may already have happened.
- A template is read and changed as the list it is (`todolists show|edit`,
  `todogroups …`, `todos …`) with the library's IDs; a new template is
  `todolists create --tool <library toolId>`, the archived ones are `todolists
  list <library toolId> --archived`. In a template a day is exit 9
  `templateTakesNoDates` (taking one off is fine) and ticking off a to-do or a
  subtask `templateTakesNoTicks`.
- Refusals of `start`: exit 2 for a tool that is unknown, not yours or **no
  longer in use**; then exit 9 `templateUnknown` (unknown, in the trash, a list
  of a project, or you are in no library), `templateIntoLibrary` (`--tool` is
  the library's own), `templateArchived`, a title code, `projectArchived`.
  Without `--title` an archived template is `templateArchived` before
  everything after it; with `--title` the title is checked first and
  `projectArchived` comes before `templateArchived`.

### The archive of a list

A list in the archive is read and takes **no write** — no change, no new
group, no comment, no reaction — until it is back: exit 9 `todoListArchived`,
and the `hint` names the `todolists unarchive` to run. Its **groups are at no
address at all**: every `todogroups` command on one exits 2, as if it did not
exist. Archiving a list twice is `todoListArchived`, bringing back one in use
`todoListNotArchived`. In an archived project `projectArchived` comes before
those three and before `todoPlaceUnknown`, but after what is wrong with the
request itself: a title or a description Minka refuses is answered as such,
and a list or a group that is not there is exit 2.

### What does not exist here

Moving or copying a list into another tool or project, restoring from the
trash are not offered by this CLI; making one from a template is, above.

### Codes of this part

`todoListTitleMissing`, `todoListTitleTooLong`,
`todoListTitleHasControlCharacters`, `todoListDescriptionTooLong`,
`todoListDescriptionNotReadable`, `todoGroupTitleMissing`,
`todoGroupTitleTooLong`, `todoGroupTitleHasControlCharacters`,
`todoGroupColorUnknown`, `todoListArchived`, `todoListNotArchived`,
`todoPlaceUnknown` (a neighbour that is no other group of the same list),
`templateUnknown`, `templateArchived`, `templateIntoLibrary`,
`templateTakesNoDates`, `templateTakesNoTicks`, all exit 9; `projectArchived`
before any of the archive codes.

## To-dos and subtasks

A **to-do** has a title, notes, a start and a due day, **assignees**, people
**told when it is done**, and **subtasks**: lines of work under it, each with
one optional assignee and one optional due day.

### Reading

- `todos show <todoId>` gives the to-do with `list`, `group`, `notes`,
  `startsOn`, `dueOn`, `isOverdue`, `assignees`, `notified`, `subtasks`,
  `listArchived` and the comments. A to-do of an archived list is read too.
- `todos completed <listId>` reads the **done** to-dos of a list, the one done
  last first, a page at a time: `data` is `{todos, next}`. Hand `next` to
  `--before` exactly as it came (it is no ID); it is `null` on the last page.
- `todos mine` lists your **open** to-dos across all projects: those you are
  assigned to and those you hold an open subtask of (`isAssigned: false`, and
  `subtasks` holds only your own open ones). Done ones, those of an archived
  list and of an archived project are not among them. A bare array, a page at
  a time: **Minka marks the end by nothing but a page shorter than asked for**,
  so a full page says in the `notice` where the next one starts
  (`--after <the last todoId>`). `--limit` is 1 to 100, 50 unless given.

### Writing

- `todos create --list <listId> --title "..."` adds a to-do **at the end** of
  the list, or of the group given with `--group`. `--notes` is Markdown by
  default, as for a card.
- Days are `YYYY-MM-DD`. **A start is kept only on a day before the due day**:
  a start alone, or one on the due day, becomes the due day. Taking the due day
  off (`--due ""`) while a start is left makes the start the due day; give
  `--starts-on ""` as well to take both off. A due day moved onto the start
  takes the start away. The `notice` says when Minka kept a day otherwise than
  you meant it; the exit code does not.
- `--assignee` and `--notify-when-done` take an ID, `me`, an e-mail address or a
  name, repeatable; Minka takes a person named twice once. **On `todos edit`
  they are the whole set**: whoever is not named again is taken off. `--no-assignees` and `--no-notify-when-done` take
  everybody off. Somebody who is no member of the project is exit 9
  `personNotAProjectMember`, and nothing is written.
- `todos edit` changes only what you give. `--title ""` is exit 9
  `todoTitleMissing`; `--notes ""` empties the notes. Notes with files or
  mentions are not replaced unless `--drop-attachments` is given.
- `todos complete` and `todos reopen` succeed twice as well. They leave the
  subtasks as they are.
- `todos move <todoId>` needs **exactly one** of `--top`, `--after <todoId>`
  and `--before <todoId>`. `--list` takes it into another list of the **same**
  To-dos tool. Without `--group` or `--no-group` a to-do **keeps its group
  within its own list**, and stands in no group in another list. The to-do
  named as neighbour has to stand in the list **and the group** the to-do goes
  to, so to go next to a to-do of another group, name that group with
  `--group`. Anything else is exit 9 `todoPlaceUnknown`; another tool or
  project is done on the web.
- `todos delete` puts a to-do into the trash with its subtasks.
- `todos subtasks …` name a subtask as `cards steps` do: by ID, by position from 1
  or by its text. `create` adds one at the end; `edit` changes the text,
  `--assignee` (one person, `""` for nobody) or `--due` (`""` takes it off);
  `complete` and `reopen` succeed twice; `move` needs exactly one of `--top`,
  `--after` and `--before`; `delete` succeeds for one that is gone already.
  A subtask has no trash.
- `todos comments create|edit|delete|react|unreact <todoId> …` work as under a
  card, and `todos comments list <todoId>` reads them, the oldest first. A done
  to-do takes comments as an open one does. Only the author changes or removes
  a comment; somebody else's is exit 2. `todos comments edit` and `todolists
  comments edit` read the to-do or the list first, for the files and mentions
  a comment of yours may carry.

### The archive, and the order of the refusals

A to-do of a list in the archive is read and takes **no write** at all — no
change, no move, no subtask, no comment or reaction, not even a tick: exit 9
`todoListArchived`, and the `hint` names `todolists unarchive`. No to-do
moves into an archived list either. What Minka says, in this order: exit 2 for
a list or a to-do that is not there, whatever the request says, then what is
wrong with the request itself (a title, a line of a subtask, the days), then
`projectArchived`, then `todoListArchived`, then the place
(`todoPlaceUnknown`) or the people (`personNotAProjectMember`). Somebody else's
comment is exit 2 only after all of these: under an archived list it is
`todoListArchived` as well.

### Codes of this part

`todoTitleMissing`, `todoTitleTooLong`, `todoTitleHasControlCharacters`,
`todoNotesTooLong`, `todoNotesNotReadable`, `todoDatesNotADate`,
`todoDatesOutOfOrder` (a start after the due day), `subtaskMissing`,
`subtaskTooLong`, `subtaskHasControlCharacters`, `subtaskDueNotADate`,
`personNotAProjectMember`, `todoPlaceUnknown`, `todoListArchived`,
`todosLimitNotANumber`, `completedTodosBeforeUnknown` (a `--before` that is
no `next`), `todosAfterUnknown` (an `--after` of none of your projects), all
exit 9.

## Repeating to-dos

A **series** makes a to-do again and again. It is made **from a to-do with a
due day**, which becomes its first occurrence; every further occurrence is an
ordinary to-do at the end of the list, due on its day, with `series: {id,
occursOn}` in `todos show` and `seriesId` on its line in `todolists show`. A
series is **never deleted, only stopped**.

### Reading

- `todos series list <listId>` gives every series of a list, stopped ones too.
- `todos series show <seriesId>` gives `schedule`, `leadDays`, `state`
  (`running`, `stopped`, or `finished`: past its end), `nextOccursOn` (the day
  of the next occurrence), `nextShowsOn` (the day it appears in the list, null
  where it comes with the next round or is there already), `canShowNow`,
  `assignees`, `notified`, `notes`, `notesSource`, `attachments` (the files of
  the notes) and `updatedAt`. **Take the days from Minka**; the CLI never
  works them out, and "today" is the installation's day.

### Writing

- `todos repeat <todoId>` makes the series; it starts on the to-do's due day.
  Without `--rhythm` it is weekly on that weekday. The rhythms and the details
  each takes: `daily` (`--weekdays`), `weekly` (`--weekdays`, `--interval`),
  `monthlyOnDay` (`--day`, `--interval`), `monthlyOnWeekday` (`--weekday`,
  `--instance` 1-4 or `last`, `--interval`), `yearly` (`--month`, `--day`).
  Weekdays are `1`-`7` or names such as `mon`. What is not given follows from
  the first day. A detail the rhythm does not use is a usage error when you
  give `--rhythm`, and a `notice` otherwise (Minka ignores it).
- `todos series edit` sends **only the flags you give**, never the stored
  details, and needs `--expect <updatedAt>` (from `todos series show`) or
  `--force`. While rhythm and first day stay, `--interval 2` alone keeps the
  day; a new `--rhythm` or `--starts-on` takes the details not given from the
  first day. `--ends-on ""` takes the end off. `--assignee` and
  `--notify-when-done` are the **whole set**. It changes the occurrences made
  from now on. Written over since you read it: exit 9 `todoSeriesChanged`,
  and `data` is the series as it stands. A change that asks for what stands
  already writes nothing and is no conflict.
- `todos series edit --notes` takes Markdown, `-` for stdin, `""` to empty
  them, as `todos edit` does; `todos repeat` takes no notes, the series starts
  with the to-do's. **To keep the files and mentions, send `notesSource` back**
  with `--notes-format html`: notes that would drop them are refused (exit 1)
  unless `--drop-attachments` is given. Minka drops a file or person from
  outside the project without a word and tells nobody the notes mention, so
  read the series back.
- `todos series stop` stops it; the to-dos it made stay. `todos series resume`
  continues a stopped one with its first day **after today**; missed days are
  not made up.
- `todos series show-now` makes the upcoming occurrence at once: `data` is
  `{series, todoId}`, where `todoId` is the new to-do. A series holds at most
  one occurrence not yet due.

### Codes of this part

`todoToRepeatUnknown`, `todoSeriesNeedsDueDay`, `todoRepeatsAlready`,
`todoSeriesScheduleInvalid`, `todoSeriesDatesNotADate`,
`todoSeriesDatesOutOfOrder`, `todoSeriesLeadDaysInvalid`, `todoNotesTooLong`,
`todoNotesNotReadable`, `personNotAProjectMember`, `todoSeriesStopped` (stop
or show-now of a stopped series), `todoSeriesNotStopped` (resume of one that
is not stopped, a finished one included), `todoSeriesOccurrenceAhead`,
`todoSeriesFinished`, `todoListArchived`, `projectArchived` and
`todoSeriesChanged` (409), all exit 9. A series of a list you may not read is
exit 2.
