# Reminders

A **reminder** is Minka's "Remind me": a note to yourself that an item, a
comment or a line of a project chat comes back at a moment. It is yours alone;
nobody else sees it, and your principal does not see yours. Not to be confused
with `checkins reminders`, the check-ins you were asked.

```bash
minka reminders list --json
minka reminders set <kind> <item> (--preset in30Minutes|in1Hour|in3Hours|tomorrow|nextMonday | --at <moment>) --json
minka reminders delete <kind> <item> [--outcome done|dismissed] --json
```

- `reminders set <kind> <item> --preset tomorrow` sets one, or moves the one
  that is there, due or not. `<kind>` says what `<item>` names: `card`,
  `post`, `todo`, `todoList`, `document`, `upload`, `question`, `answer`,
  `event` or `forward` for the item by its ID, `comment` for a comment by its
  own ID, `chatMessage` for a line of a project chat by its own ID. A card
  may be named by its short identifier: `reminders set card MNK-1A --preset
  tomorrow`. The kind is taken in any case; a word that is no kind is exit 1
  before anything is sent.
- **The kind has to fit the ID**: a comment's ID as `card`, or a card's as
  `post`, is exit 2, as though nothing were there. A line of a private
  conversation takes no reminder, exit 2 as well.
- Exactly one of `--preset` and `--at`. `--preset` is `in30Minutes`,
  `in1Hour`, `in3Hours`, `tomorrow` (9:00) or `nextMonday` (9:00; on a Monday
  the Monday a week on), **counted in the time zone of your Minka
  preferences**, not in this machine's. `--at` is a moment in **RFC 3339**,
  `2026-10-06T09:00:00+02:00` or `2026-10-06T07:00:00Z`: a capital `T`, the
  seconds, and `Z` or `±HH:MM`. Anything else (`+0200`, `z`, `UTC`, a zone
  name, no offset) is exit 1 before anything is sent. It has to lie ahead.
- `reminders list` gives yours, the due ones first (`isDue`), the most
  recently due first, then those ahead, the soonest first; `appUrl` opens the
  item, at the comment or line. One whose item you no longer see is left out
  and comes back when you see it again. **At most 500 due and 500 ahead**, cut
  before what is out of sight is left out: where a side lists 500, the
  `notice` says there may be more.
- `reminders delete <kind> <item>` takes one away, due or not, and also one of
  something you no longer see: the ID decides, the kind is not compared.
  `--outcome done` ticks it off, without it (or `dismissed`) it is waved away;
  both take it away alike. No reminder of that ID is exit 2; `reminders list`
  shows yours.
- A reminder is not a write into the project: it works in an archived
  project as well.

## Codes of this part

`reminderPresetUnknown`, `reminderTimeGone` (not ahead of Minka's clock),
`reminderOutcomeUnknown` (refused before the reminder is looked for), all exit
9; `notFound`, exit 2.
