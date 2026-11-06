# Projects, boards and cards

## Commands

```bash
minka projects edit <projectId> [--name "New name"] [--description - < about.txt] --json
minka boards archive <boardId> [--before <cardId>] [--limit <n>] [--all] --json
minka cards archive <cardId> --json
minka cards unarchive <cardId> --json
minka cards delete <cardId> --json
minka cards restore <cardId> --json
minka columns create --board <boardId> --title "Review" --json
minka columns edit <column> [--board <boardId>] [--title "New"] [--color <colour> | --no-color] [--role <role> | --no-role] --json
minka columns move <column> --board <boardId> (--first | --after <column> | --before <column>) --json
minka columns delete <column> [--board <boardId>] --json
minka columns on-hold <column> [--board <boardId>] [--off] --json
minka trash list <projectId> --json
minka cards subscribe <cardId> <person> --json
minka cards unsubscribe <cardId> <person> --json
```

## Your own cards

- `cards mine` lists the cards the account of the token is assigned to, across
  every project it is a member of, as a bare array of the same cards
  `cards show` gives. Each carries `project {id, name}` and `board {id, title}`
  besides `projectId` and `boardId`.
- `--board <boardId>` keeps the cards of one board, `--column <column>` those
  of one column, each by itself; an ID that matches nothing lists none. The
  column is its ID, or its type or title together with `--board`, looked up on
  that board: `minka cards mine --board <boardId> --column "Als nächstes"`
  lists your cards in the column of that title. `cards next --board
  <boardId>` names the card Minka releases to you next: in the columns with
  the role `ready` from left to right, each from the bottom, the first of
  yours that is not on hold; `data` null when nothing is released for you,
  always so in an archived project. A board in the trash is exit 2.
  The bottom card is usually the one that has waited longest. Exit 2 with a
  message that the host does not name the next card means the Minka is too
  old; then read the board yourself.
  Exit 7 with a hint to list without the filters means this Minka cannot say
  where its cards stand: then read a card with `cards show` before taking it.
- Cards in Done, cards completed in another column, cards in the trash, cards
  in the archive of their board, cards of an archived project and cards of a
  project that was left are not among them, so the list is what is still to
  do. A card that is not in the list is therefore not necessarily done: it may
  have been archived.
- The order is the order of the page: projects by name, then by `dueOn` with
  the overdue ones first and the ones without a date last.
- It is the cheapest way to answer "what am I on?": one request instead of a
  board per project.
- `cards delete` puts a card in the trash and `cards restore <cardId>` takes it
  back out, on top of the column it hung in. Only a card that is in the trash
  can be restored: a second restore, a card that was never trashed and a card
  whose board or project is in the trash all exit 2.
- A card in the archive of its board goes into the trash from there, and a
  restore puts it back on the board, not into the archive: the restored card
  says `archived: false`.
- `trash list <projectId>` says what was taken out of a project, the newest
  first and at most 200 (a `notice` says when there are more). Every entry is
  `{kind, id, title, toolType, board, column, trashedAt, trashedBy,
  restorable, blockedBy, appUrl}`.
- `kind` is `tool`, `column`, `card`, `post`, `todoList`, `todo`, `todoGroup`,
  `checkinQuestion`, `checkinAnswer`, `forward`, `folder`, `document` or
  `upload`, and Minka may name further kinds.
- `board` is a board only for a `card` and a `column`. For every other kind the
  same field carries the tool the thing hung in, which the text view marks as a
  tool; `column` is there for a card alone.
- `restorable` says whether a command brings the entry back: `cards restore`
  for a card, and `docs restore`, `docs uploads restore` and `docs folders
  restore` for a `document`, an `upload` and a `folder` (see
  [docs-and-files.md](docs-and-files.md)). `blockedBy` names what has to come
  back first: `board`, `column`, `tool`, `todoList`, `todoGroup`, `question`,
  `folder` or `archivedFolder` — the last one lies in an archive, not in the
  trash. A draft and everything else come back in the web only.

## Projects

- `projects list` gives the projects in use as `{id, name, description,
  status, appUrl}`; `status` is `active` or `archived`. `--archived` lists the
  archived ones instead.
- An archived project stays readable under every address: boards, cards,
  chats, people, trash. Every write into it exits 9 with `projectArchived`;
  only `chats mark-read` still works. Somebody who is no member gets exit 2 as
  ever and does not learn that it is archived.
- `messages create` is refused like every other write, also with a
  `--client-message-id` that was sent before the project was archived.
- A host whose Minka does not know archived projects yet makes `projects list
  --archived` exit 7 rather than list projects in use as archived.
- `projects edit <projectId>` renames a project (`--name`) or changes its
  description (`--description`, `-` for stdin, `""` to clear it); at least one
  of the two, or exit 1 before any request. What is not given stays, even if
  somebody changed it meanwhile. The description is plain text: no Markdown is
  converted, no mention or reference is made. The answer is the project as
  `projects list` gives it. An empty name exits 9 with `projectNameMissing`;
  `projectNameTooLong` (200 characters), `projectNameHasControlCharacters` and
  `projectDescriptionTooLong` (2000) are the other refusals. Only the team may
  edit: a client of the project gets exit 2 like an outsider. A host whose
  Minka is too old for the command also exits 2, with a hint that says so.
  Rename a project only when you were asked to.

## Boards

- A card carries `shortId`, such as `MNK-1A`, or null where its board shows no
  short identifiers. It is what people call a card by. The text views show it
  beside the title; it is no part of the title. It is no lasting address: it
  changes when the board changes its prefix, and the `id` never does.
- A card carries `shortUrl` beside `appUrl`: its short address, such as
  `https://<host>/go/card/MNK-1A`, null where `shortId` is. A former prefix
  keeps leading to the card, so the address stays valid. A Minka from before
  it leaves the field out. The text views give a card's `shortUrl` where it
  has one and its `appUrl` otherwise.
- Wherever a command takes `<cardId>`, the short identifier works as well, in
  any case: `minka cards show mnk-1a`, `minka comments create MNK-1A -`,
  `minka cards move MNK-1A --column Doing --after MNK-2B`. The command looks
  the card up first and goes on with its `id`; answers and `breadcrumbs` name
  the `id`, which stays when a board changes its prefix.
- Exit 2 for a short identifier means that no card you can see goes by it: it
  does not exist, its board shows no short identifiers, the card is in the
  trash or in a project you are not in, or two of your organizations use the
  prefix. `cards restore` therefore needs the `id`, which `trash list` gives.
- Column types: `triage` (landing zone for new cards), `column` (working
  columns), `not_now`, `done`.
- A new card lands in Triage unless `--column` names Triage or a working
  column. Not now and Done take no new cards; create, then move.
- Moving into Done completes a card (`completedAt`, `completedBy`); moving out
  reopens it. Without placement a moved card goes to the top. Done is ordered
  by completion and refuses `--bottom`, `--after` and `--before`.
- A column title that equals a type or another column's title is ambiguous
  (exit 8); the hint lists the candidate IDs.
- Done lists only the 25 cards completed last; `cardCount` counts all of
  them. Read an older card by its ID.

## Columns

- A column carries `color`: one of `white`, `red`, `orange`, `yellow`,
  `green`, `blue`, `aqua`, `purple`, `gray`, `pink` and `brown` for a working
  column, always `green` for Done, null for Triage and Not now.
- `columns create --board <boardId> --title <title>` adds a working column
  behind the others, in the colour the board holds least of. It takes neither
  a place nor a colour: move and colour it afterwards.
- `columns edit <column> [--title] [--color | --no-color]` changes what is
  given, at least one of them. Triage, Not now and Done take a title of their
  own too, and `--title ""` gives them back the name of their type; a working
  column exits 9 with `columnTitleMissing` on it. `--no-color` takes the colour
  away: the column reads `white` then, which is how a column without a colour
  looks. Any other word for `--color` exits 9 with `columnColorUnknown`, and a
  colour or `--no-color` for Triage, Not now or Done exits 9 with
  `systemColumnHasNoColor` and leaves the title as it was too.
  `columnTitleTooLong` (200 characters) and `columnTitleHasControlCharacters`
  are the other refusals.
- A column carries `role`: `inbox` for Triage and `done` for Done by type,
  null for Not now, and for a working column `ready`, `in_progress`,
  `waiting_for_decision` or null, as the team chose. `columns edit --role
  <role>` gives a working column one, `--no-role` takes it away; anybody on
  the team may. Any other word, `inbox` and `done` included, exits 9 with
  `columnRoleUnknown`, and `--role` or `--no-role` for Triage, Not now or Done
  with `systemColumnHasNoRole`; both refusals write nothing, the title sent
  along included. A Minka without roles answers `--role` alone with exit 9
  and `nothingToChange`, and `--role` with `--title` with exit 7.
- A Minka that does not take a colour away yet answers `--no-color` alone
  with exit 9 and `nothingToChange`, and `--no-color` with `--title` with exit
  7: the title was written, the colour stayed, and `data` is the column.
  `--color white` gives the same look on any Minka.
- `columns move <column> --board <boardId>` with exactly one of `--first`,
  `--after <column>` and `--before <column>` places a working column among the
  working columns. The neighbour has to be another working column of the same
  board, or the command exits 1 before anything moves: Minka would put the
  column first without a word next to a system column or itself. Triage, Not
  now and Done exit 9 with `systemColumnCannotMove`.
- `columns delete <column>` puts a working column into the trash of its
  project, and its cards go with it: the cards standing in it cannot be read,
  moved or restored until the column is back, and only the web brings it back.
  A card of the column in the archive of the board stays readable. `trash list`
  names it with `kind` `column`. Triage, Not now and Done exit 9 with
  `systemColumnCannotBeRemoved`; deleting again exits 2.
- Without `--board`, `<column>` has to be its ID; with it, a type or a title
  works as well. Only the team changes columns: a client of the project gets
  exit 2 like an outsider, whatever the write. An archived project exits 9
  with `projectArchived`, a write that would change nothing included.

## Cards on hold

- A working column can have an area for the cards that are waiting. Triage,
  Not now and Done cannot. A card on hold stays in its column, apart from the
  rest and in an order of its own.
- A column carries `onHoldEnabled`, `onHoldCardCount` and `onHoldCards`, in
  the order they stand on hold. `cards` and `cardCount` leave the cards on
  hold out. A card carries `onHold`.
- `columns on-hold <column>` switches the area on, `--off` switches it off and
  hands its cards back to the end of the column, in the order they stood on
  hold. Without `--board`, `<column>` has to be its ID. A system column exits
  9 with `meta.apiCode` `systemColumnHasNoOnHoldArea`, on and off alike.
- `cards move <cardId> --column <column> --on-hold` puts a card on hold; the
  placement flags then place it among the cards on hold. A column of the board
  that has no such area exits 9 with `meta.apiCode` `columnHasNoOnHoldArea`,
  Triage, Not now and Done among them: on a move that code is the answer, not
  `systemColumnHasNoOnHoldArea`. A move without `--on-hold` takes a card off
  hold, also within its own column.
- A card cannot be created on hold, by design: it comes into being in a
  column and goes on hold by a move. A card restored
  from the trash comes back off hold.

## The archive of a board

- Every card carries `archived`. It is true for a card in the archive of its
  board: the card stands in no column, it can be read, and every write on it
  exits 9 with `meta.apiCode` `cardArchived`.
- Such a card still carries a `column`, and that is the column it was taken
  from, which may have been deleted from the board since. The text view of
  `cards show` therefore prints `Archived: yes, was in <column>` and no
  `Column:` line, and the summary names the archive of the board instead of a
  column.
- A card in the archive is in no list of the board: `boards show` shows what
  stands in the columns, and `cards mine` leaves it out. `boards archive
  <boardId>` is the one list it is in; read the card itself by its `id`.
- `cards archive <cardId>` puts a card away, `cards unarchive <cardId>` brings
  it back. Putting away a card that lies in the archive exits 9 with
  `cardArchived`, bringing back one that stands on its board exits 9 with
  `cardNotArchived`, and an archived project exits 9 with `projectArchived`
  before either. A card in the **trash** exits 2 (`not_found`) on both, and
  comes back with `cards restore` instead — the trash is not the archive.
- A card brought back lands at the top of the column it was taken from, or in
  the first working column where that column is gone from the board, or in
  Triage on a board that has none; the answer names the column. It comes back
  **off hold**, whatever it was when it was put away, which nothing in the
  answer says: the command says it in a `notice`.
- The board leaves a card of an archive out of the set an agent says it is
  working on, so a card in hand drops out of `work list` with the next signal.
  `cards archive` does not take it out by itself.

### Reading the archive

- `boards archive <boardId> [--before <cardId>] [--limit <n>] [--all]` lists the
  cards a board has put away, the one put away last first. Each carries `id`,
  `projectId`, `shortId`, `title`, `archivedAt`, `archivedBy`, `appUrl` and
  `shortUrl` — and nothing else: the card itself is read with
  `cards show <cardId>`.
- A page holds 50 cards by default and **at most 100**. A larger `--limit` is
  refused here with exit 1 (`usage`), and that refusal is a safeguard, not a
  restriction: the server cuts a larger page down to 100 without saying so, and
  the only sign its answer carries for the end of the archive is a page shorter
  than asked for. Asking for 500 would therefore hand you 100 cards that read
  like the whole archive. Do not work around it by asking the API directly.
- `--before <cardId>` gives the page behind that card. Take it from the
  `breadcrumbs` of a page rather than working it out; `--before` needs the `id`,
  not the short identifier. A card that no longer lies in the archive of that
  board exits 9 with `meta.apiCode` `archivedCardsBeforeUnknown`: somebody took
  it back onto the board or into the trash. That is no end of the list — read
  the archive again from the beginning.
- `--all` walks every page to the end in one run and works around the weakness
  above by itself: it keeps the ids of the whole last page and falls back through
  them when the one it named is gone, it settles an empty answer behind a full
  page by reading behind the card before the cursor rather than taking it for the
  end, and it lists every card once. A page that carries the run no further ends
  it with exit 7 (`api_error`) and what it read.
- **What a run of `--all` promises:** every card that lay in the archive for the
  whole run appears exactly once, under one reservation: the run checks the end of
  the list by asking behind another card of the last page, and where no card of
  that page is left to ask behind, an answer that carried nothing is all it has
  for the end. A card that left the archive in that moment reads the same, and
  cards behind it were not read. Such a run exits 0 and says so in its `notice`;
  do not report its result as the whole archive. A page of one card is in that
  place from the first request, a larger page where the fallback has used up the
  cards of the page it last read. **What it does not promise:** the archive at
  one moment. A card put away during the run lands on the first page, which the
  run has already passed, and can be missed; a card taken out of the archive
  during the run vanishes from the result with no trace, and the API says
  nothing about either. A run of more than one page says this in its `notice`,
  together with how many pages it read, how many cards came twice and whether it
  had to begin again. Do not report the result of a multi-page run as a count of
  what the archive holds right now.
- A run that ends on `archivedCardsBeforeUnknown` still hands over what it read:
  the error envelope carries that as `data` with the notices, and the exit code
  9 says that something is missing from it.

## People and assignments

- A card has any number of assignees, listed in `assignees` as `{id, name}`
  in the order they were assigned.
- `<person>` is an ID, `me`, an e-mail address or a name (case-insensitive,
  it has to match one person; two people with that name give exit 8).
- Only members of the card's project can be assigned; `minka people list
  <projectId>` shows them. A UUID of somebody else gives exit 9 with
  `meta.apiCode` `personNotAProjectMember`.
- Assigning somebody who is assigned already succeeds, and the summary says
  so. Taking off somebody who is not assigned succeeds as well, and people who
  left the project can still be taken off.
- `cards create --assignee <person>` (repeatable) looks the people up first
  and creates the card together with them in one request. Somebody outside
  the project leaves no card behind (exit 9, `personNotAProjectMember`).
- `cards create --label <label>` (repeatable) gives the card labels of its
  board or project, by ID or name; see [Labels](labels.md).
- New cards go to the top of their column.
- A card read by itself carries `subscribers` as `{id, name}`, sorted by
  name: the people it notifies about comments, its due date and completion.
  Minka subscribes people by itself when they create a card, are assigned or
  comment. `cards subscribe <cardId> <person>` adds somebody, `cards
  unsubscribe` takes somebody off and keeps Minka from adding them again.
  Both succeed when nothing changes; only project members can be named
  (`personNotAProjectMember`, exit 9).

## Steps

- A card read by itself carries `steps` as `{id, title, completed, assignees,
  dueOn}` in the order it shows them; `assignees` in the order they were put
  on, somebody who left the project included, `dueOn` as `YYYY-MM-DD` or null. The board carries only `stepCount` and
  `completedStepCount` per card; read the card for the steps themselves.
- `<step>` is a step ID, its position starting at 1, or its text
  (case-insensitive; a number always means a position; a text on two steps
  gives exit 8 with the positions).
- `cards steps create` adds a step at the end. A step is one line of at most 500
  characters: `stepMissing`, `stepTooLong` and `stepHasControlCharacters` exit 9.
  It comes without people and without a due day; give them with `cards steps edit`.
  `data` is the new step, with `--card` too, so `data.id` is the step's ID for
  a following `cards steps edit` or `cards steps move`.
- `cards steps edit <cardId> <step>` changes the text, the people and the due day in
  one request; what is not given stays. `--title` follows the rules of
  `cards steps create`; `--title ""` exits 9 with `stepMissing`. `--assignee` (repeatable) is the WHOLE set: whoever
  is on the step and not named again is taken off; `--no-assignees` takes
  everybody off. `--due YYYY-MM-DD`, `--due ""` takes the day off. A person is
  an ID, `me`, an e-mail address or a name. Only a member of the project who
  sees the board can be on a step: anybody else refuses the whole write with
  `personNotAProjectMember` (exit 9), and nothing is written, the text
  neither. A day that is no
  day is `stepDueDateNotADate`, a card in the archive `cardArchived` (exit 9).
  A Minka without the people and the day of a step answers
  `stepCompletedMissing`, and the hint says so.
- Completing a completed step, reopening an open one and deleting a step that
  is gone all succeed. Deleting does not ask.
- `cards steps move <cardId> <step>` needs exactly one of `--top`, `--bottom`,
  `--after <step>` and `--before <step>`; the neighbour is named like `<step>`.
  Moving a step where it stands succeeds. The step itself as its neighbour is
  `stepPlaceUnknown` (exit 9). `--bottom` goes behind the last step as read;
  if that step is gone by the move, `stepPlaceUnknown` comes with a hint that
  says so: list the steps and move again. A Minka without the move answers 404
  (exit 2) for every step; the hint says so only for a step found on the card.
- A Minka without renaming answers `--title` alone with `nothingToChange`
  (exit 9) and passes over it beside other flags: exit 7, the rest written,
  the card in `data`; the hint says so.

### A card as a step

- A step is words or another card. `cards steps create <cardId> --card
  <child>` attaches a card, named by its ID, its short identifier (`MNK-1A`
  or `#MNK-1A`) or its address on this host; the text and `--card` exclude
  each other. The child can be on any board of any project you see, also in
  the archive, and a step of several cards. It is a step of the same card at
  most once: attaching it again succeeds (the summary says it was a step
  already) without a second step. The card itself is `stepCardIsThisCard`, a
  card you cannot see `stepCardUnknown` (exit 9).
- A card step has no title, tick, due day or people of its own: its card
  carries them. Its `title` is the card's, `completed` says the card is in
  Done, `assignees` is `[]`, `dueOn` null, and `card` is `{kind, id, shortId,
  title, projectId, boardId, column, columnColor, assignees, appUrl}` (no
  `shortUrl`; build it as `https://<host>/go/card/<shortId>`). A step in words
  has `card: null`. `cards steps edit`, `complete` and `reopen` on a card step
  are refused with `stepIsACard` (exit 9): change the card itself, and move it
  to Done to finish the step. A card step moves like any step.
- A child you cannot see reads as `"card": {"kind": "card"}` with an empty
  title, shown as `(a card you cannot open)`; name it by its position or ID.
- `cards show` carries `stepOf`, the cards it is a step of that you see, and
  prints them as `Step of: #MNK-1 Title, …` below the title.
- `cards steps delete` on a card step removes only the link; the child card
  stays.
- `cards steps make-card <cardId> <step>` makes a step in words into a new
  card in Triage of the same board, with the words as its title and the
  step's people and due day; `data` is the new card. The step keeps its place
  and becomes a card step. Words over 200 characters are cut with `…` and kept
  whole in the description. A card step is `stepIsACard`; words or people
  refused for a new card are refused here alike, and nothing is written.
- A Minka without card steps answers `--card` and `make-card` with exit 2 and
  a message that the host does not know card steps yet.

## Due dates

- A card carries `dueOn` as `YYYY-MM-DD`, or null. It is a day, with no time
  and no zone.
- Set it with `cards create --due YYYY-MM-DD` or `cards edit --due YYYY-MM-DD`;
  remove it with `cards edit --due ""`. Relative words such as `tomorrow` are
  not understood: compute the day yourself.
- A text that is no existing day (`2026-02-30`, `28.09.2026`, `2026-9-8`)
  exits 9 with `meta.apiCode` `cardDueDateNotADate`.
