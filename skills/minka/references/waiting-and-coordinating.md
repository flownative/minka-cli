# Waiting for news and coordinating a board

Read this when you are to stay on a board for longer than one task: as the
session that coordinates your account's work on a board and waits for its next
card.

## Waiting instead of polling

`minka notifications wait --json` blocks until something new arrives in the
bell of your account — a card assigned to you was moved, somebody commented,
mentioned or assigned you — prints it and exits 0. `data` is the list of
notifications as Minka sent them, the oldest first; each has `summary` (one
line: what happened and who did it), `occasion` (such as `card.moved`),
`actor`, `subject` (`type`, `id`, `shortId`, `title`, `appUrl`, `shortUrl`)
and, for a card on a board, `card` with `boardId` and the `column` it stands
in now. `shortUrl` is the card's short address, as on the card itself, and
null for anything else. A card of yours moved into Done arrives as
`card.completed`, and only where you subscribe to it, not as `card.moved`.

Never poll a board in a loop: every poll costs tokens, and a wait costs none
while it waits. Minka holds the request open and answers the moment something
arrives.

In Claude Code, run the wait with `run_in_background: true`. Its exit wakes
you; until then you spend nothing. Read its output, act, and start the next
wait in the background again.

- Each session goes on from where it stopped: the position is kept per account
  and session (`--session`, else `$CLAUDE_CODE_SESSION_ID`). Nothing that
  arrives between two waits is lost. The first wait of a session starts from
  now: it shows nothing that was there before. Read the board for that.
- The position moves on only after the lines are printed. A wait that is
  killed shows them again next time; two waits of one session at once may both
  show the same lines. Run one wait per session.
- `--board <boardId>` and `--column <id|title>` (each repeatable) keep only
  lines about cards on that board or in that column. `--board` alone also keeps
  a line about the board itself: `board.conventionsChanged`, whose `subject`
  is the board (`type` `board`). What they leave out is read all the same and
  not shown again; a notice counts it. The `wait`
  breadcrumb repeats the filters and `--session` of the run; use it.
- `--timeout 30m` exits 12 (`timeout`) when nothing arrived; run it again.
  Without it the wait lasts as long as it takes.
- Exit 6 (`network`) or 7 (`api_error`) with `retryable` true means the
  connection broke or the server failed; the position is safe, start the wait
  again. Exit 7 with `retryable` false means Minka answered in a way a wait
  cannot go on from; tell the person rather than looping. A notice that Minka no longer knew the position
  means the wait started from now: read the board to catch up.

## Reading and settling one line

Waiting reads lines and marks none of them read: the bell in Minka on the web
still counts them. The `id` of a line is what these take:

```bash
minka notifications show <notificationId> --json       # the line as its push named it
minka notifications mark-read <notificationId> --json  # mark it read; data.unreadCount is the bell after it
minka notifications count --json                       # data.unreadCount: the number on the bell
```

- `show` hands over `title` (the push's own sentence, which may say less than
  the `summary` of the wait), `body` (the project), `appUrl`, `read` and
  `unreadCount`. It marks nothing read.
- `mark-read` of a line already read changes nothing.
- Exit 2 (`not_found`) means the line is somebody else's, is about something
  you may no longer see — a project you left or that is archived, something
  in the trash — or is gone.
- The count is the bell's: reactions and unread private messages one by one
  are in it, so it is not the number of lines a wait prints.

## Coordinating a board

Every account coordinates a board with one session at a time: one
coordinating session per agent account and board. Several agents coordinate
the same board side by side, each with an entry of its own that only it sees.
The entry keeps two sessions of the same account from working the board at
once; it decides nothing about who works on what. Assignment does that: a
person releases a card by putting it into a column with the role `ready` and
assigning it.

```bash
minka boards coordinator show <boardId> --json      # data null: no session of yours
minka boards coordinator claim <boardId> --session planning --json  # take it, renew it, take it over
minka boards coordinator release <boardId> --json   # remove your entry
```

- A claim needs `--session <name>`: a name you choose, such as `planning` or
  `"Planung März"`: 1 to 64 characters, no control or format character, sent
  as written. It is not taken from `$CLAUDE_CODE_SESSION_ID`. Keep the same
  name for every renewal; a new name is a new session. In an archived project
  a claim exits 9 with `projectArchived`.
- A claim answers the entry with `columns` (in board order, each with `id`,
  `type`, `title` and `role`) and `conventions` (`text`, `updatedAt`,
  `updatedBy`, `version`, or null). Read both on every claim: they are what
  you work by on this board.
- A claim is never refused for another account. A claim of a new session of
  your own account takes the entry over at once.
- The entry expires 4 hours after its last sign of life. A claim is one, and so
  is every `minka notifications wait` of the same account, so a session that
  keeps waiting keeps its entry. A forgotten entry blocks nobody.
- `data` null with a notice that the board does not show it means you are a
  client of the project, who cannot coordinate.
- `minka boards show <boardId>` names the entry as well, as "Coordinated by"
  and the person: your own on a current Minka.
- Exit 9 with `meta.apiCode` `coordinatorTaken` comes only from a Minka older
  than the one that lets several accounts coordinate a board. Tell the person
  you work for; do not try to remove the other entry.

### The flow of a coordinating session

1. `minka boards coordinator claim <boardId> --session <name> --json`.
2. Read the columns' roles and the conventions from the claim's answer (see
   [working-on-a-board.md](working-on-a-board.md)). Remember the conventions'
   `version`, `updatedAt` and `updatedBy`.
3. Take only your own cards in a column with the role `ready`:
   `minka cards next --board <boardId> --json` names the first in board order,
   or `data` null when nothing is released for you. Begin that one; leave the
   rest.
4. Start `minka notifications wait --board <boardId> --json` in the background.
5. When it wakes you, read `data`, act on each line, and start the next wait.
   On an assignment or a card moved to another column, run step 3 again: only
   a card in a `ready` column that is assigned to you is yours to take. A
   comment that asks something is answered on its card. On
   `board.conventionsChanged`, read `minka boards conventions show <boardId>`,
   compare `updatedAt` and `updatedBy` with what you worked by, and tell the
   person you work for when the writer changed. Every wait keeps the entry.
6. When you stop, `minka boards coordinator release <boardId> --json`.

### Handing over to another session

A new session of the same account takes the entry over by claiming it:
`minka boards coordinator claim <boardId> --session <its name> --json` in the
new session replaces the old one at once. Before that, the old session writes
what the new one needs on the back of the cards in hand (`minka cards notes
set`) and stops its wait.
