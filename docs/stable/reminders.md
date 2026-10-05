# Reminders

A reminder is Minka's "Remind me": a note of one person to themselves that an
item, a comment or a line of a project chat is to come back at a moment. It is
private. Whoever holds the token sets, reads and takes away only their own, an
agent its own as well. These are not the reminders of check-ins, which
`checkins reminders` lists ([Check-ins](checkins.md)).

| Command | Request |
|---|---|
| `reminders list` | `GET /me/reminders` |
| `reminders set <kind> <item> (--preset <preset> \| --at <moment>)` | `PUT /me/reminders/{kind}/{id}` |
| `reminders delete <kind> <item> [--outcome done\|dismissed]` | `DELETE /me/reminders/{kind}/{id}` |

## The kind and the item

Minka addresses a reminder by what its identifier names and by the identifier
itself (C-113):

| Kind | `<item>` is the ID of |
|---|---|
| `card`, `post`, `todo`, `todoList`, `document`, `upload`, `question`, `answer`, `event`, `forward` | the item |
| `comment` | a comment, on whichever of these it stands |
| `chatMessage` | a line of a project chat |

Minka compares the kind in its own spelling and answers any other word with
the same 404 as a thing that is not there. The CLI therefore takes the kind in
any case, sends it in Minka's spelling and refuses a word that is no kind
before it sends anything, exit 1. `<item>` is an ID, written in lowercase
before it goes out; for `card` it may also be a short identifier such as
`MNK-AG`, which is looked up first, as everywhere a card is named.

A line of a private conversation takes no reminder: Minka answers 404.

## The moment

`set` takes exactly one of the two; neither and both are refused before
anything is sent, exit 1.

- `--preset` is a time of the web menu: `in30Minutes`, `in1Hour`, `in3Hours`,
  `tomorrow` (9:00) and `nextMonday` (9:00; on a Monday the Monday a week on).
  The two mornings are counted by Minka in the time zone of the person's
  preferences, else the installation's, never in the zone of the machine the
  CLI runs on.
- `--at` is a moment in RFC 3339, the one form Minka reads: a capital `T`,
  the seconds, a fraction of any length if wanted, and `Z` or an offset of
  `±HH:MM` up to 14 hours, on a day that exists — `2026-10-06T09:00:00+02:00`,
  `2026-10-06T07:00:00Z`. The CLI holds it against the same form before it
  sends anything and refuses anything else with exit 1 (C-113): no zone name,
  no `UTC`, no lowercase `z`, no `+0200`. It holds the same form as Minka, so
  it refuses nothing Minka takes, and what passes goes out as written. The
  moment has to lie ahead of Minka's clock, not of this machine's; a moment
  equal to that clock is gone already.

`set` on something that has a reminder already moves it, due or not. The
answer is the reminder with `isDue` false, and Minka counts its line in the
bell as read.

## The list

The due reminders come first, the most recently due first, then those ahead,
the soonest first; two of the same moment stand by the ID of what they are
of. `isDue` is true once the moment has come, until the reminder is taken
away or set anew. Minka resolves every reminder as the page of its item on
each reading, so one whose item the person no longer sees is left out and
comes back once they see it again.

Minka reads at most 500 due reminders and 500 ahead, and leaves out the ones
out of sight only afterwards. A side can therefore be cut and list fewer than
500, which nothing in the answer tells. Where a side lists 500, the CLI says in
the notice that there may be more; below that it cannot know.

## Taking a reminder away

`delete` takes the reminder away whether or not the person still sees what it
is of, so a reminder that has left the list is still taken away by its kind
and ID. The kind has to be one of the words above, but Minka does not compare
it with the reminder: the ID decides.

`--outcome done` is the web's tick, `dismissed` its X. Both take the reminder
away alike; Minka records which it was and shows it nowhere. Without
`--outcome` the CLI sends none, and Minka takes `dismissed`; `data.outcome`
says so. The outcome is not checked here: Minka refuses one it does not know
before it looks for the reminder.

## No write into a project

A reminder is the person's own and is set and taken away in an archived
project as well: Minka asks for no status of the project, and an archived
card, post, list, document or upload can be reminded of. The three
commands are on the list of commands that write into no project, so the test
of archived projects does not expect `projectArchived` of them.

## The order of the refusals

After 406, 401 and a `__` argument, which every address has:

- **`set`**: the item first — an ID that is none, a kind other than the one of
  the ID, a thing that is not there or not seen (not a member, hidden from a
  client, in the trash or in a trashed tool or project) are all 404 —, then the
  body: 415, then 400 `malformedBody` for `preset` or `remindAt` not a string
  and for both, then `reminderPresetUnknown`, `reminderTimeMissing`,
  `reminderTimeNotAMoment` and `reminderTimeGone`.
- **`delete`**: the kind and the ID (404), then the outcome (422
  `reminderOutcomeUnknown`, even where there is no reminder), then the
  reminder (404 where the person has none of that ID).

| Answer | Command | Exit |
|---|---|---|
| 404 | `set` for anything not seen or of another kind; `delete` where there is no reminder | 2 |
| 422 `reminderPresetUnknown`, `reminderTimeGone` | `set` | 9 |
| 422 `reminderOutcomeUnknown` | `delete --outcome` | 9 |
| 422 `reminderTimeMissing`, `reminderTimeNotAMoment`, 400 `malformedBody`, 415 | unreachable: the CLI sends exactly one of the two as JSON, and a moment only in RFC 3339 | — |

A 404 on `set` and on `delete` mean different things, and their hints say
which: on `set` that the kind may not fit the ID or the thing is out of sight,
on `delete` that there is no reminder of that ID and where to list them.
