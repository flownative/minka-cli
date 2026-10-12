# Check-ins

A **question** of a Check-ins tool asks chosen members of the project on a
schedule; each person asked gets a reminder and answers it. `checkins …` reads
and writes the questions, `checkins answers …` the answers, and `checkins
reminders` lists what you were asked and have not answered.

## Commands

```bash
minka checkins tools <projectId> --json
minka checkins tool <toolId> --json
minka checkins list <toolId> --json
minka checkins show <questionId> --json
minka checkins create --tool <toolId> --title "Question?" --rhythm daily|weekly|everyOtherWeek|monthly --weekday <day>... --time HH:MM --ask <person>... --json
minka checkins edit <questionId> [--title "New"] [--rhythm <rhythm>] [--weekday <day>]... [--time HH:MM] [--ask <person>]... --json
minka checkins pause <questionId> --json
minka checkins resume <questionId> --json
minka checkins reminders --json
minka checkins answers list <questionId> [--person <person>] [--on <day>] [--limit <1-100>] [--before <answerId>] --json
minka checkins answers show <answerId> --json
minka checkins answers create <questionId> - [--asked-for <day>] [--format html] --json < answer.md
minka checkins answers edit <answerId> - [--format html] --json < answer.md
```

## Reading

- `checkins tools <projectId>` lists the Check-ins tools in use, `checkins
  tool <toolId>` one tool by its ID alone;
  `checkins list <toolId>` their questions, the oldest first; `checkins show
  <questionId>` one question with `schedule` (`rhythm`, `weekdays`, `timeOfDay`,
  `startsOn`), `paused` and `asked`.
- `asked` names only the people asked **who are still members and may see the
  question**, in the order of their names. Whoever it does not show stays
  asked unseen, and no edit takes them off.
- `checkins answers list <questionId>` lists the answers, the newest day
  first: `groupOn` is the day an answer belongs to. `--person` (ID, `me`,
  address or name) and `--on <day>` narrow them. A bare array, a page at a
  time: **Minka marks the end by nothing but a page shorter than asked for**,
  so a full page says in the `notice` where the next one starts (`--before <the
  last answerId>`). `--limit` is 1 to 100, 50 unless given.
- `checkins answers show <answerId>` gives the answer with `bodySource`, its
  files and its comments.
- `checkins reminders` lists your open reminders across all projects, the one
  asked last first: per question only the latest day it asked you for, open
  until an answer of yours belongs to that day or a later one. Paused
  questions and archived projects are not among them. **At most 100, and no
  page beyond**: a list that long says in the `notice` there may be more; those
  are the ones asked earlier, and answering some brings them into view.

## Writing a question

- `checkins create --tool <toolId> --title "..." --rhythm <rhythm> --weekday
  <day> --time HH:MM --ask <person>`. `--rhythm` is Minka's word: `daily` (on
  each weekday given), `weekly`, `everyOtherWeek` or `monthly` (the first of the
  weekday in a month); every rhythm but `daily` takes exactly one weekday.
  `--weekday` is `1` (Monday) to `7` or a name (`mon`, `monday`), repeatable or
  as `mon,wed,fri`. `--time` holds in the time zone of each person asked. The
  question starts on your today.
- `--ask` on `checkins create` takes an ID, `me`, an e-mail address or a name;
  a name or an address is looked up among the members of the project the tool
  stands in. Only members of the project who may see the question can be asked:
  anybody else, a client of the project or an agent working for one included,
  is exit 9 `personNotAProjectMember`.
- `checkins edit` changes only what you give; the start day never changes.
  **`--weekday` and `--ask` are the whole set.** The weekdays not given are the
  question's own, so `--rhythm weekly` on a question asked Monday to Friday
  needs one `--weekday`. **Every other week is counted from the first day of
  its weekday on or after `startsOn`**, so a new weekday can move the question
  into the other set of weeks; an edit of the rhythm or the weekday of such a
  question names the first three of its dates, counted from `startsOn`, in the
  `notice`; for an older question those lie in the past.
- An edit without `--ask` leaves the people asked as they are. `--ask` takes
  off only people shown in `asked` and not named again: a member who left the
  project and a client the question is not shown to stay asked, so the people
  read from `checkins show` and named back take nobody off.
- Only a member who may see the question can be asked. A question set up here
  is shown to no client, so naming a client of the project, or an agent
  working for one, is exit 9 `personNotAProjectMember` and nothing is written.
  Should somebody named still be missing from `asked`, the `notice` names them
  as not asked.
- `checkins pause` and `checkins resume` succeed twice as well. The days a
  question was paused are not asked afterwards.

## Answering

- `checkins answers create <questionId> -` answers as the account of the
  token; the text is Markdown with every line break kept, as for a comment.
  Anybody in the project may answer, as often as they like, the same day too.
- `--asked-for <day>` ties the answer to a reminder: give `askedFor` of
  `checkins reminders`. A day the question was not put to you for is exit 9
  `answerDayNotAsked`. Without it the answer belongs to the day of the latest
  asking by the question's schedule, in your time zone, whether the question
  was paused then or asks you at all (or today, before the first asking);
  `groupOn` says which, and it never changes.
- `checkins answers edit <answerId> -` changes your own answer; somebody
  else's is exit 2, as if it were not there, also where it carries files. The
  same text again succeeds. Your own answer with files or mentions is not
  replaced unless `--drop-attachments` is given.

## The order of the refusals

What Minka says, in this order. First exit 2 for a tool, a question or an
answer that is not there, somebody else's answer included, whatever the
request says. For a new question then the wording, the weekdays, the time and
the rhythm, then `questionNobodyAsked`, then `projectArchived`, then
`personNotAProjectMember`. For an edit the wording, the schedule, then
`questionNobodyAsked`, `projectArchived` and `personNotAProjectMember`. For an
answer `answerDayNotADate`, then the text (`answerTooLong`,
`answerNotReadable`, `answerMissing`), then `answerDayNotAsked`, then
`projectArchived`. Pausing or resuming a question of an archived project is
`projectArchived` whatever state it is in.

## Codes of this part

`questionTitleMissing`, `questionTitleTooLong`,
`questionTitleHasControlCharacters`, `questionWeekdaysMissing`,
`questionTimeInvalid`, `questionScheduleInvalid`, `questionNobodyAsked`,
`personNotAProjectMember`, `answerDayNotADate`, `answerDayNotAsked`,
`answerMissing`, `answerTooLong`, `answerNotReadable`, `answersBeforeUnknown`
(a `--before` that is no answer to the question), `projectArchived`, all exit 9.
