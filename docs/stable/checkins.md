# Check-ins

A Check-ins tool holds **questions**. A question asks chosen members of its
project on a **schedule**; each of them is reminded for a day and answers it.
An **answer** belongs to one day, and anybody in the project may write one.
`checkins tools`, `checkins list` and `checkins show` read the questions;
`checkins create`, `edit`, `pause` and `resume` write them; `checkins answers
list`, `show`, `create` and `edit` read and write the answers; `checkins
reminders` lists what the person of the token was asked and has not answered
([C-077](../decisions.md#c-077)).

## What a read hands over

`GET projects/{projectId}/checkins` lists the tools in use, as `id`, `title`
and `appUrl`. A tool no longer in use is not among them and is read and
written at its own address as before.

`GET checkins/{toolId}/questions` lists the questions out of the trash, the
oldest first, and `GET questions/{questionId}` answers one, both in the same
shape: `projectId`, `toolId`, `project` and `tool` as references, `title`,
`schedule` with `rhythm`, `weekdays`, `timeOfDay` and `startsOn`, `paused`,
`asked` and `appUrl`. Nothing is paged.

**`asked` is not the set of the people asked.** It names only those who are
still members of the project and may see the question, in the order of their
names. A person who left the project, and a client asked before the question
was hidden from clients, stay asked unseen, and no edit takes them off. A
client the question is not shown to cannot be named to be asked (see
[The people](#writing-a-question)). An agent that stepped down
stays a member of the projects that were archived then, and is listed there.
The API cannot show a question to clients: that is set on the web.

## The schedule

`rhythm` is one of four words: `daily` asks on each weekday given, `weekly`
on one weekday, `everyOtherWeek` on one weekday every second week, and
`monthly` on the first of one weekday in a month (a day within its first
seven). `weekdays` are ISO numbers, Monday 1 to Sunday 7, and every rhythm but
`daily` takes exactly one. `timeOfDay` is `HH:MM` on the 24-hour clock and
holds in the time zone of **each person asked**: the schedule names no zone.
`startsOn` is the writer's today in the writer's zone when the question was
set up, and **no edit moves it**. Every other week is counted from the first
day on or after it that is the question's weekday, so **the weekday decides
which weeks are asked**: with `startsOn` 2026-09-01, a Tuesday, Monday asks on
2026-09-07 and 2026-09-21, Wednesday on 2026-09-02 and 2026-09-16. A new
weekday can therefore move a question into the other set of weeks.

Minka keeps what it is sent in a form of its own, with the same success as any
write: the weekdays each once and in the order of the week, and the time with
two digits each (`9:00` is `09:00`, `09:00:00` is `09:00`). Neither loses
anything, so the CLI says nothing about them; it says what the answer does not
show ([C-080](../decisions.md#c-080)).

`checkins create` and `checkins edit` take `--rhythm` as Minka's word,
`--weekday` as a number or a name (`mon`, `monday`), repeatable or as a list
(`mon,wed,fri`), and `--time`. The rhythm and the numbers go out as given,
the number 8 as well, and Minka says what it does not take; only a word that
names no weekday is a usage error, because it cannot be sent as a number
([C-078](../decisions.md#c-078)).

## Writing a question

`POST checkins/{toolId}/questions` sets up a question from `title`, `rhythm`,
`weekdays`, `timeOfDay` and `askedIds` and answers it with 201 and `Location`.
It asks nobody at once: the reminders follow the schedule. Whoever sets it up
is put on it, asked or not, and so is everybody asked.

`PATCH questions/{questionId}` changes any of the five; what the body leaves
out stays. The weekdays left out are the question's own, so a daily question
asked on several days refuses `rhythm: weekly` without a weekday, with
`questionScheduleInvalid`. `nothingToChange` is unreachable: an edit without a
flag is a usage error here.

**The people.** An edit that leaves `askedIds` out leaves the people asked as
they are, also on a question that shows nobody. `askedIds` is the whole set over
the people the question shows in `asked`: whoever of them is not named again is
asked no longer (and stays subscribed), and a person named twice is asked once.
Whoever `asked` does not show — a member who left the project, a client the
question is not shown to — stays asked whatever the set names, so the people
read from `asked` and sent back take nobody off. `[]` is `questionNobodyAsked`,
and so is a body that leaves the people out on a new question.

`checkins create --ask` takes an ID or `me`: the ID of a tool names no project,
so no name or address can be looked up, as for the people a new document
reaches ([C-070](../decisions.md#c-070)). `checkins edit --ask` takes an ID,
`me`, an e-mail address or a name and reads the question for its project where
one of the last three is given. There is no `--no-ask`: a question asks
somebody. A person named twice goes out twice.

Only a member who may see the question can be asked. A question set up through
the API is shown to no client, so a client of the project, or an agent working
for one, named with `--ask` is refused with `personNotAProjectMember`, on a new
question and on an edit alike, and nothing is written. Should somebody named
still be missing from `asked` in the answer, a `notice` names them as named but
not asked. Where an
edit gives the rhythm or the weekdays and the question is asked every other
week afterwards, a `notice` names the first day of its weeks and the two after
it — also where it was asked every other week before — and says that another
weekday can move it into the other weeks ([C-080](../decisions.md#c-080)). The
text output names that first day as well.

`POST questions/{questionId}/pause` and `…/resume` read no body and answer the
question; pausing a paused one and resuming a running one succeed and change
nothing. The days a question was paused are not asked afterwards.

## Answers

`GET questions/{questionId}/answers` lists the answers out of the trash, the
newest day first and within a day the one written last first: `id`,
`questionId`, `groupOn`, `body`, `commentCount`, `createdAt`, `editedAt`
(null until its writer changed it), `author` and `appUrl`. `personId` narrows
to one person — one who is no identifier is a 404 — and `on` to one day.
`limit` is 50 unless given, Minka cuts one above 100 down to 100 without a
word, and `before` names the last answer of the page before: one that is no
answer to the question is `answersBeforeUnknown`, while one of the question in
the trash still marks where a page ends.

It is **a bare array without a sign of its end**, the danger of
[C-054](../decisions.md#c-054) once more, with the same answer as one's own
to-dos ([C-079](../decisions.md#c-079)): `--limit` outside 1 to 100 is a usage
error, `api.Client.CheckinAnswers` refuses it again, the size always goes out,
and a full page names the next one in the `notice`, `--before` the last answer,
with `--person` and `--on` kept. `--before` has to be an ID. `--person` takes
what `edit --ask` takes and reads the question for its project where it is no
ID.

`GET answers/{answerId}` answers one with `question`, `groupOn`, `body`,
`bodySource`, `attachments`, `author` and its `comments`. An answer whose
question is in the trash is at no address.

`POST questions/{questionId}/answers` answers from `body` and `askedFor` with
201 and `Location`. Anybody in the project may answer, as often as they like,
the same day too, a paused question as well. **`askedFor`** names the day of a
reminder, and the answer belongs to it; a day the question was not put to the
writer for is `answerDayNotAsked`, never quietly replaced. Without it the answer
belongs to the day of the latest asking by the question's schedule, in the
writer's zone — whether the question was paused then or asks the writer at
all — or to the writer's today before the first asking. `groupOn` says
which, and no write changes it. The body is cleaned like the body of a post:
Markdown is converted here with every line break kept, as for a comment, and
`--format html` draws the notice of [C-068](../decisions.md#c-068).

`PATCH answers/{answerId}` changes the `body` of one's own answer. Somebody
else's is 404, as if it were not there; the same body again is 200 and changes
nothing. `checkins answers edit` reads the answer first, so that a text with
files or mentions is not replaced by one without unless `--drop-attachments` is
given. Where it carries them and somebody else wrote it, the command says so
with exit 2 before that guard and writes nothing, as Minka would answer: Minka
asks who wrote the answer before the text and before the archived project. In
an archived project one's own answer goes out and `projectArchived` is the
answer, not the guard ([C-082](../decisions.md#c-082)).

## Reminders

`GET me/checkin-reminders` lists the open reminders of the person of the token
across all projects: `question`, `project`, `tool`, `askedFor`, `askedAt` and
`appUrl`, the one asked last first. Per question only the latest day it asked
for counts, and it is open while no answer of theirs belongs to that day or a
later one: a day skipped and the next one answered leaves nothing open. A
paused question, one in the trash, one of a tool in the trash, one that no
longer asks the person and one of an archived project are not among them.

Minka hands out **at most 100 and nothing beyond**, and takes no parameter. A
list of 100 therefore says in the `notice` that there may be more: those left
out are the ones asked earlier, and answering some of the list brings them
into view ([C-079](../decisions.md#c-079)).

## Archived and trashed

A question, an answer, a tool or a project in the trash is at no address: 404.
An archived project is read like any other, and every write into it is
`projectArchived` for a member — pausing a paused question and resuming a
running one as well, and the same body of an answer again.

## The order of the refusals

Minka checks in this order, and each step comes only where the one before it
passed:

- **A new question:** a field of the wrong type (400, unreachable from here),
  an identifier of a person that is none (`personNotAProjectMember`), the
  wording, then the schedule — no weekday (`questionWeekdaysMissing`), the time
  (`questionTimeInvalid`), the rhythm and the weekdays
  (`questionScheduleInvalid`) — then the tool (404), then nobody asked, then
  `projectArchived`, then a person who is no member.
- **An edit:** the fields, `nothingToChange`, the wording, then the question
  (404), then the schedule held against the question's own, then nobody asked,
  `projectArchived` and a person who is no member.
- **Pause and resume:** the question (404), then `projectArchived`.
- **A page of answers:** the question (404), then `on`
  (`answerDayNotADate`), `limit` (`answersLimitNotANumber`, unreachable from
  here), a `personId` that is no identifier (404), then `before`.
- **An answer:** `askedFor` that is no day (`answerDayNotADate`), then the
  question (404), then the body (`answerTooLong`, `answerNotReadable`,
  `answerMissing`), then `answerDayNotAsked`, then `projectArchived`.
- **An edit of an answer:** `nothingToChange` (unreachable), the answer (404,
  and 404 for somebody else's), the body, then `projectArchived`.

The CLI changes this order where it reads before it writes: a question or an
answer that is not there is then exit 2 before anything else. It reads the
question for `checkins edit` where `--ask` names a person by name, address or
`me`, and for `checkins answers list` where `--person` does; it reads the answer
for every `checkins answers edit`. So `checkins edit <unknown> --title "" --ask
me` is exit 2, where the same with an ID for the person is
`questionTitleMissing`.

| Answer | Command | Exit |
|---|---|---|
| 404 | every command, for a tool, question or answer that is unknown, foreign or in the trash; `answers edit` for somebody else's answer | 2 |
| 422 `questionTitleMissing`, `questionTitleTooLong`, `questionTitleHasControlCharacters` | `checkins create` (before the tool), `checkins edit` (before the question) | 9 |
| 422 `questionWeekdaysMissing`, `questionTimeInvalid`, `questionScheduleInvalid` | `checkins create` (before the tool), `checkins edit` (after the question) | 9 |
| 422 `questionNobodyAsked` | `checkins create` without `--ask`; unreachable on `checkins edit`, which sends no empty set | 9 |
| 422 `personNotAProjectMember` | `checkins create`, `checkins edit` naming somebody outside the project, or a client of it or an agent of one where the question is not shown to clients | 9 |
| 422 `answerDayNotADate` | `checkins answers create --asked-for` (before the question), `checkins answers list --on` (after it) | 9 |
| 422 `answerDayNotAsked` | `checkins answers create --asked-for`, before `projectArchived` | 9 |
| 422 `answerMissing`, `answerTooLong`, `answerNotReadable` | `checkins answers create`, `checkins answers edit` | 9 |
| 422 `answersBeforeUnknown` | `checkins answers list --before` | 9 |
| 422 `projectArchived` | every write, after the body and the question | 9 |
| 422 `nothingToChange`, `answersLimitNotANumber` | unreachable: the CLI sends no empty edit and no page outside 1 to 100 | — |

## What does not exist here

Putting a question into the trash, stopping to be asked oneself, removing an
answer, writing a comment under an answer and showing a question to clients are
done on the web: the API has no address for them.
