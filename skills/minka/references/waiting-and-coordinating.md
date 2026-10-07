# Waiting for news and coordinating a board

Read this when you are to stay on a board for longer than one task: as the
session that hands out the work of a board, or as one that waits for its next
card.

## Waiting instead of polling

`minka notifications wait --json` blocks until something new arrives in the
bell of your account — a card assigned to you was moved, somebody commented,
mentioned or assigned you — prints it and exits 0. `data` is the list of
notifications as Minka sent them, the oldest first; each has `summary` (one
line: what happened and who did it), `occasion` (such as `card.moved`),
`actor`, `subject` (`type`, `id`, `shortId`, `title`, `appUrl`) and, for a card
on a board, `card` with `boardId` and the `column` it stands in now. A card of
yours moved into Done arrives as `card.completed`, and only where you
subscribe to it, not as `card.moved`.

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
  lines about cards on that board or in that column. What they leave out is
  read all the same and not shown again; a notice counts it. The `wait`
  breadcrumb repeats the filters and `--session` of the run; use it.
- `--timeout 30m` exits 12 (`timeout`) when nothing arrived; run it again.
  Without it the wait lasts as long as it takes.
- Exit 6 (`network`) or 7 (`api_error`) with `retryable` true means the
  connection broke or the server failed; the position is safe, start the wait
  again. Exit 7 with `retryable` false means Minka answered in a way a wait
  cannot go on from; tell the person rather than looping. A notice that Minka no longer knew the position
  means the wait started from now: read the board to catch up.

## Coordinating a board

A board has at most one coordinator: one session of one person that decides
who works on what. The board shows it with the session and when it was last
active.

```bash
minka boards coordinator show <boardId> --json      # data null: nobody
minka boards coordinator claim <boardId> --session planning --json  # take it, or renew it
minka boards coordinator release <boardId> --json   # give it up
```

- A claim needs `--session <name>`: a name you choose for people to read on
  the board, such as `planning`: 1 to 64 ASCII letters, digits, `.`, `-` and
  `_`, no spaces. In an archived project it exits 9 with `projectArchived`.
  It is not taken from `$CLAUDE_CODE_SESSION_ID`. Keep the same name for every
  renewal; a new name is a new session.
- The entry expires 4 hours after its last sign of life. A claim is one, and so
  is every `minka notifications wait` of the same person, so a coordinator that
  keeps waiting keeps the board.
- Exit 9 with `meta.apiCode` `coordinatorTaken`: another person coordinates the
  board. `data` is the holder (`person`, `session`, `lastSeenAt`,
  `expiresAt`), and so is the hint. Do not take it over; tell the person who
  asked you, or wait for the entry to expire.
- `minka boards show <boardId>` names the coordinator as well.

### The flow of a coordinating session

1. `minka boards coordinator claim <boardId> --session <name> --json`.
2. Read the board: `minka boards show <boardId> --json`. Take up only cards
   assigned to you (`minka cards mine --json`); hand out the rest by assigning
   them, never by working on them yourself.
3. Start `minka notifications wait --board <boardId> --json` in the background.
4. When it wakes you, read `data`, act on each line — a card moved to the
   column you hand out from, a comment that asks something — and start the
   next wait. Every wait keeps the board.
5. When you stop, `minka boards coordinator release <boardId> --json`.

### Handing over to another session

A new session of the same person takes the board over by claiming it:
`minka boards coordinator claim <boardId> --session <its name> --json` in the
new session replaces
the old one at once. Before that, the old session writes what the new one
needs on the back of the cards in hand (`minka cards notes set`) and stops its
wait. Another person cannot take it over; the holder releases it first, or
anybody who may edit the board removes a stale entry with
`minka boards coordinator release <boardId>`.
