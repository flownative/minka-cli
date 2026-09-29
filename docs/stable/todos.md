# To-dos and subtasks

A **to-do** stands in a list of a To-dos tool, directly or in one group of it
([To-do lists and groups](todo-lists.md)). It has a title, notes, a start and a
due day, **assignees**, the people **told when it is done**, and **subtasks**:
lines of work that are no items of their own, each with one optional assignee
and one optional due day. `todos …` reads and writes the to-dos, `todos subtasks
…` the subtasks ([C-071](../decisions.md#c-071)).

## What a read hands over

`GET todos/{todoId}` answers the to-do with where it stands (`projectId`,
`toolId`, `listId`, `groupId` and `project`, `tool`, `list`, `group` as
references; `group` and `groupId` are null for one standing in no group),
`completed`, `completedAt` and `completedByName` (null while open), `notes` and
`notesSource`, `attachments`, `startsOn`, `dueOn` (both `YYYY-MM-DD` or null),
`isOverdue`, `assignees` and `notified` in the order they were put on,
`subtasks` in their order (`id`, `title`, `completed`, `assignee` or null,
`dueOn`, `isOverdue`), `listArchived`, `commentCount`, `createdAt`, `creator`,
`comments` and `appUrl`, and `series`: `{id, occursOn}` for an occurrence of a
series, null otherwise ([Repeating to-dos](todo-series.md)). Somebody who left
the project stays on a to-do and is named nowhere in it; the assignee of a subtask who left reads as null. A to-do
of an archived list is read like any other and says `listArchived: true`; one in
the trash, in a group in the trash or in a list in the trash is at no address.

`isOverdue` is a due day before today in the time zone of the person asking, and
never for something done.

## The done to-dos of a list

`GET todo-lists/{listId}/completed` reads the done to-dos of a list in use or in
the archive, the one done last first, as the lines `todolists show` gives, with
`completedAt` and `groupId`. It answers `{todos, next}`: `next` is where the
next page starts, an opaque string, and null on the last page — a page that holds
the rest exactly included. It carries the moment the last to-do was ticked off
and its identifier, not the to-do, so opening it again or ticking it off anew
between two pages moves nothing.

`todos completed` hands `--before` over as it is given. `--limit` is 50 unless
given and always goes out. Minka refuses one below 1 with
`todosLimitNotANumber` and cuts one above 100 down to 100 without a word, which
does no harm here: `next` then says there is more. So the CLI leaves the size to
Minka ([C-076](../decisions.md#c-076)).

## One's own to-dos

`GET me/todos` lists the **open** to-dos of the person of the token across all
projects: those they are assigned to and those they hold an open subtask of.
Done ones, those in the trash or in a group or list in the trash, those of a list
in the archive, of a tool in the trash and of a project not in use are not among
them. The order is the one of the page of one's tasks: project name, project,
due day with those without one last, title, identifier. Each names `project`,
`tool` and `list`, says `isAssigned` (false where only a subtask is the
person's), all `assignees`, and in `subtasks` only the person's own open ones.

It is **a bare array without a sign of its end**: a page shorter than asked for
is the last, and a limit above 100 is cut down in silence. That is the danger of
[C-054](../decisions.md#c-054), and the answer is the same: `--limit` outside 1
to 100 is a usage error here, and `api.Client.MyTodos` refuses it again. The
size always goes out, and a full page says in the `notice` where the next one
starts: `--after` the last to-do of it. `--after` has to be an ID; Minka compares
it as that to-do stands now, done or not, and refuses one of none of the
person's projects with `todosAfterUnknown` ([C-076](../decisions.md#c-076)).

## Writing a to-do

`POST todo-lists/{listId}/todos` adds a to-do **at the end** of the list, or of
the group named by `groupId`. `PATCH todos/{todoId}` changes any of `title`,
`notes`, `startsOn`, `dueOn`, `assigneeIds` and `notifiedIds` in one write; what
the body leaves out stays. `nothingToChange` is unreachable: an edit without a
flag is a usage error here.

**The notes** go through the cleaning of every text of the editor, as the
description of a list does: `--notes` is Markdown converted here, and
`--notes-format html` draws the notice of [C-068](../decisions.md#c-068) where
the text carries no served markup. Notes that carry files or mentions are not
replaced by ones that do not, unless `--drop-attachments` is given — except for
a to-do of a list in the archive and in an archived project, where Minka
refuses any notes and its answer comes ([C-082](../decisions.md#c-082)).

**The days.** A start is kept only on a day before the due day. One day alone
is the due day, whichever field it was written in, and a start on the due day is
no start. A start after the due day is `todoDatesOutOfOrder`. On an edit, a day
left out stays, and a day sent as null or `""` is taken off, so taking the due
day off under a start makes the start the due day, and a due day moved onto the
start takes the start away. Minka answers all of it with the same success as
any write. The CLI compares what came back with the start that was meant — the
one sent, or on an edit that sends none the one the to-do carried, for which an
edit with a due day and no start reads the to-do first — and says in a `notice`
where that start was not kept. Where an edit took the due day off and sent no
start, the notice says instead that the due day was not taken off
([C-075](../decisions.md#c-075)). Whether a day is a day is Minka's to say: the
CLI sends what it is given.

**The people.** `assigneeIds` and `notifiedIds` are **the whole set** the
to-do is to carry: whoever stays keeps their place, whoever is new comes behind
in the order named, and whoever is not named again is taken off. So `todos edit
--assignee` names the whole set, `--no-assignees` and `--no-notify-when-done`
send an empty list — never null, which Minka reads as no change — and there is
no command that adds or takes off one person ([C-073](../decisions.md#c-073)). A
person is an ID, `me`, an e-mail address or a name; the members of the project
are read only where a name, an address or `me` asks for them. A person named
twice goes out twice, and Minka takes them once, in the place first named.
Somebody who is no member refuses the whole write with
`personNotAProjectMember`, and nothing is written.

`POST todos/{todoId}/complete` and `…/reopen` answer the to-do, twice as well.
Neither touches the subtasks: a to-do is done with open subtasks, and ticking off
every subtask does not tick off the to-do. `DELETE todos/{todoId}` puts the
to-do into the trash (204).

## Moving a to-do

`POST todos/{todoId}/moves` takes `listId`, `groupId`, `previousTodoId` and
`nextTodoId`. The list left out is the to-do's own. The group left out is its
own within its own list and none in another; `groupId: null` is none. Minka puts
the to-do behind the previous one where it is named, else in front of the next
one, and **at the top of its place** where neither is named. Its place is the
list and the group: the to-dos standing in no group are one place, each group
another.

So `todos move` follows [C-072](../decisions.md#c-072): exactly one of `--top`,
`--after <todoId>` and `--before <todoId>`, and none or two are a usage error.
`--list` names a list, `--group` a group, `--no-group` sends `groupId: null`,
and `--group` with `--no-group` is a usage error ([C-074](../decisions.md#c-074)).

A neighbour has to stand, out of the trash, in the list **and the group** the
to-do goes to, and may not be the to-do itself. A list of another tool, a group
of another list, a list or group that is not there and a neighbour of another
place are all `todoPlaceUnknown`, and nothing moves. Taking a to-do into
another tool or project is Minka's web alone.

## Subtasks

`POST todos/{todoId}/subtasks` adds a subtask at the end from `title`,
`assigneeId` and `dueOn` and answers the to-do (201). `PATCH
todos/{todoId}/subtasks/{subtaskId}` changes any of `title`, `assigneeId`,
`dueOn` and `completed`; null takes the assignee or the day off, and asking for
the state a subtask is in already succeeds. `POST …/moves` takes
`previousSubtaskId` and `nextSubtaskId` with the rule of a group: neither is the
top, and a neighbour that is no other subtask of the to-do is
`todoPlaceUnknown`. `DELETE …` takes a subtask off for good, with 204 for one
that is gone already.

`todos subtasks` names a subtask as `steps` names a step: by ID, by position
from 1 or by its text, which needs the to-do, so every subtask command but
`create` reads it first. An ID that is on no subtask goes out as it is.

## The archive of the list, and the order of the refusals

**A to-do of an archived list takes no write at all** — no change, no tick, no
move, no subtask, not even taking off a subtask that is not there — and none
moves into an archived list: 422 `todoListArchived`. The hint names `todolists
unarchive`.

Minka checks in this order, and each step comes only where the one before it
passed:

1. What is wrong with the body. For a to-do: a field of the wrong type (400,
   unreachable from here), an identifier of a person or of a place that is
   none (`personNotAProjectMember`, `todoPlaceUnknown`, which the CLI never
   sends), then `nothingToChange`, the title, and the days of a **new** to-do.
   For a subtask: `nothingToChange`, the line, then an identifier of a person
   that is none, then the day. A neighbour of a move that is no identifier comes
   before everything else.
2. The list or the to-do, and the subtask: 404.
3. The days of an **edit**, which are read against the to-do found, then the
   notes.
4. After the statement refused, why it did: `projectArchived`, then
   `todoListArchived` for the list of the to-do or the list it was to go to, then
   `todoPlaceUnknown` for a group, a list or a neighbour, and
   `personNotAProjectMember` for a person who is no member.

The CLI changes this order where it reads before it writes: a list or a to-do
that is not there is then exit 2 before anything else. It reads the list for
`todos create`, and the to-do for `todos subtasks create`, where a person is
named by name, address or `me`; the to-do for every other subtask command; and
the to-do for `todos edit` where it changes the notes, names a person that way,
or gives a due day without a start. So `todos create --list <unknown> --title
" " --assignee me` is exit 2, where the same with an ID for the person is
`todoTitleMissing`.

| Answer | Command | Exit |
|---|---|---|
| 404 | every read and write, for a list or to-do that is unknown, foreign, in the trash or in a group, list or tool in the trash; `subtasks edit`, `complete`, `reopen` and `move` for a subtask that is not on the to-do; before every 422 where the CLI reads first | 2 |
| 422 `todoTitleMissing`, `todoTitleTooLong`, `todoTitleHasControlCharacters` | `todos create`, `todos edit`; before an unknown list or to-do unless the CLI reads first | 9 |
| 422 `todoNotesTooLong`, `todoNotesNotReadable` | `todos create`, `todos edit` | 9 |
| 422 `todoDatesNotADate`, `todoDatesOutOfOrder` | `todos create` (before the list), `todos edit` (after the to-do) | 9 |
| 422 `subtaskMissing`, `subtaskTooLong`, `subtaskHasControlCharacters`, `subtaskDueNotADate` | `todos subtasks create`, `todos subtasks edit` | 9 |
| 422 `personNotAProjectMember` | `todos create`, `todos edit`, `todos subtasks create`, `todos subtasks edit` | 9 |
| 422 `todoPlaceUnknown` | `todos create --group`, `todos move`, `todos subtasks move` | 9 |
| 422 `todoListArchived` | every write on a to-do of an archived list, `todos create` into one and `todos move` into one | 9 |
| 422 `projectArchived` | every write, before `todoListArchived`, `todoPlaceUnknown` and `personNotAProjectMember`; also a tick on a to-do that is done already and a subtask taken off that is not there | 9 |
| 422 `todosLimitNotANumber`, `completedTodosBeforeUnknown` | `todos completed` | 9 |
| 422 `todosAfterUnknown` | `todos mine --after` | 9 |
| 422 `nothingToChange` | unreachable: an edit without a change is a usage error here | — |
