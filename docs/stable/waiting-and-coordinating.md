# Waiting and coordinating

An agent that stays on a board waits for what is new instead of polling it,
and one session of one person coordinates a board. `notifications wait` does
the first, `boards coordinator claim|release|show` the second (C-117,
C-118). `notifications count`, `show` and `mark-read` read the number on the
bell and one line of it, and settle that line (C-122).

## What the server offers

`GET /me/notifications?after=<cursor>&wait=<seconds>&limit=<n>` lists the
lines of the bell of the token's person behind a position, the oldest first,
at most `limit` (default 50, at most 200), as `{notifications: […], cursor}`,
with no envelope, as every address of the API answers. A position is a
string the CLI keeps as it came; every line carries its own, and `cursor` is
the position of the last row read, which may lie behind the last line, or,
where no row was read, the one asked from. Without
`after`, or with an empty one, the answer carries no line and `cursor` is
where the bell stands now. Where nothing is behind the position, the request
is held for up to `wait` seconds (0 to 300), at most for Minka's setting
`notifications.waitAtMost`, 50 seconds by default, and answered as soon as a
line arrives; a longer wait is answered empty after that, its cursor behind
whatever rows it read meanwhile. The refusals come in
this order: a `wait` that is no whole number from 0 to 300, an empty one
included, as 422 `notificationsWaitInvalid`, a `limit` outside 1 to 200 as
`notificationsLimitInvalid`, then a position Minka did not hand out — no whole
number, one with a sign or a leading zero, or one beyond where the bell stands
— as `cursorUnknown`. A request with a `wait` of more than 0 is a sign of life
of every board the person coordinates, still holds and still sees as one of
the team — a member of its project, archived or not, who is no client —
before its position is read; one of 0 is not. Reading the list marks nothing read, and
reactions are not listed. The lines are those the bell shows. A row the person
may no longer see — no member of the project any more, the project archived,
the tool or the item in the trash, the item archived, hidden from a client —
is left out before `limit` counts and does not move the cursor, and so is a
reaction. A card in a column in the trash is none of these: the column keeps
its cards, and the bell does not ask about the column, so the line is listed
and `card` names that column. A row that
makes no line, of an occasion this Minka does not know, counts toward `limit`
and moves the cursor past it: an answer may be shorter than `limit`, even
empty, although more follow.

A card that changes column gives each of its assignees but the mover a line,
occasion `card.moved`, its summary naming the card by its short identifier,
else its title in quotes, and the column it went into, all as they are called
when the line is read. A column deleted through the API lies in the trash and
is named still. Where the row names no column, or the column is gone for
good, which happens only when its board is purged and then takes the line
with it, the summary is the bell's own sentence, "Moved to another column: "
and the title. A move into
Done is told as `card.completed` alone, to the subscribers of the card, its
summary "Completed: " and the title.

`GET /me/notifications/count` answers `{unreadCount}`, the number on the bell:
the unread rows the person may still see, those of an occasion that makes no
line and reactions included, and the unread messages of private
conversations one by one. `GET /notifications/{id}` answers one line as its
push named it — `{id, title, body, appUrl, createdAt, read, threadId,
timeSensitive, unreadCount}` — and `POST /notifications/{id}/read` marks it
read, once, and answers `{unreadCount}` after it. `title` is the bell's own
sentence, not the `summary` of the list: a move says "Moved to another
column: " and the title. Both look the line up through the bell's own query,
so a line of somebody else, a malformed or an unknown ID, a line the person
may no longer see, a reaction and a row that makes no line are all the same
404, and nothing is marked. The `id` of a line of the list is the ID of the
same row, so it is what both take. A read line may be forgotten by Minka
after a while and is then 404 as well.

`GET /boards/{id}` carries `coordinator`: `{person: {id, name, kind}, session,
since, lastSeenAt, expiresAt}`, or null where nobody coordinates the board or
the entry has expired. For a client of the project the field is left out, not
null. `PUT /boards/{id}/coordinator` with `{"session": …}` (1 to 64
characters, none of them a control or format character, taken as written and
not trimmed, else 422 `coordinatorSessionInvalid`) claims or renews it and answers 200 with the
coordinator, bare; another session of the same person takes it over, and
`since` starts anew; while another person holds it, the answer is 409
`coordinatorTaken` with the holder as `coordinator` beside the error; and only
then is a claim in an archived project refused with `projectArchived`.
`DELETE` takes it off for the holder and for anybody who may edit the board,
in an archived project as well, and answers 204 also where there is nothing
to take off. A client of the project and somebody outside it get 404 on both,
and so does a claim on a board in the archive of a project in use.
An entry expires 4 hours after its last sign of life, a claim or a wait.

## Waiting

`notifications wait` blocks until something new arrives, prints it and exits
0. `data` is the list of lines as Minka sent them (C-002); the text view
prints one line each, what it is about and then Minka's `summary`.

- **Where a wait starts.** Each session keeps the position it has read up to,
  so the next wait goes on from there and nothing that arrived between two
  waits is lost. The first wait of a session asks where the bell stands, keeps
  that and waits from there: what was in the bell before is not shown, and an
  agent reads the board for it.
- **Which session.** `--session`, else `$CLAUDE_CODE_SESSION_ID`, else the one
  position of no session, as for `work` (C-103). A person at a terminal has no
  session, and refusing them would buy nothing: a position of no session is
  one more mark, shared by whatever waits without naming one.
- **Where the position lies.** In `notifications-read.json` beside the
  configuration, per host, per reader — the token a command signs in with,
  named by a digest as for the read marks of comments — and per session. A
  mark nobody wrote for 30 days counts as none when a wait starts, and goes
  with the next write, because every Claude Code session has a name of its
  own; a session that comes back after that starts from then, with a notice.
  A wait that read its mark moves it on however long it waited since, and
  writes it again where the write of another session pruned it meanwhile, so a
  wait of days never loses the mark it started from. A mark that is not there
  is no later position of another wait of the session: that would be recent.
- **A file that cannot be read.** As with the read marks of comments, marks
  that cannot be read cost the marks and not the command: the wait starts from
  now, a notice names the file, and the next write replaces it, so every
  session of the machine starts from now, except a wait that was running then:
  it writes its mark again when it moves it on. A file that cannot be written ends
  a wait before it printed anything with exit 1, naming the file.
- **When it moves on.** Only after the lines are printed. A wait that is
  killed in between prints them again next time; a mark that cannot be written
  after printing leaves the lines printed, says so on stderr and exits 0. At
  worst a line is printed twice, never lost.
- **Two waits of one session.** The mark moves on only from the position the
  process read, under a lock of the kernel. A position is opaque, so of two
  nobody here can tell the later one; a process that finds the mark moved by
  another leaves it there, since that one printed everything up to it. Both
  may print the same lines; neither loses one. A process that finds the mark
  moved stops moving it for the rest of its run.
- **Filters.** `--board <id>` and `--column <id|title>`, each repeatable, keep
  the lines about cards there — by `card`, which is where the card stands now,
  a title matched in any case. A line about no card is left out under either.
  What they leave out is read all the same: the mark moves past it and the
  wait goes on, and a notice counts it. A filter is no part of the position,
  so a later wait of the same session without it does not see those lines.
- **How long.** Without `--timeout` as long as it takes, asking with
  `wait=50`, as long as Minka holds a request by default: asking for more
  gains nothing. With `--timeout` in asks of at most 50 seconds, the last for
  what is left, rounded up to a second; nothing by then exits 12 (`timeout`),
  retryable. Every ask is a sign of life of the boards the person coordinates.
  The HTTP client gives a held request its wait and 30 seconds more; the
  timeout of every other request would cut it off after 30.
- **Several pages.** A page that comes back full is followed at once by the
  next, without waiting, so a run prints everything that piled up. A page
  that a row without a line made short ends the run like the last page: its
  cursor stands behind that row, and the next wait, which finds the rest
  there, prints it at once. Nothing is lost; a run need not print all that
  piled up.
- **An ask that comes back early.** An ask that comes back empty in less than
  half the wait it asked for and in less than 5 seconds — a server or a proxy
  that does not hold it — is
  followed by a pause of 5 seconds, doubled with every such ask up to a
  minute, up to a quarter more at random, never past the end of `--timeout`.
  Half, not a fixed margin: the last ask of a timeout may ask for one second,
  and one answered at once is as early as any. And never 5 seconds or more:
  an installation configured to hold a request for less than it is asked
  holds all the same, and its next ask costs nothing worth a pause. An
  ask that was held, or brought lines, starts the pause anew. Without it such
  a server is asked thousands of times a second.
- **Odd answers.** A page that names no position goes on from its last line.
  An answer that names none and has no line, lines that do not lead past the
  position asked from, and a full page that leads back to a position already
  read from end the wait with exit 7, not retryable, before anything is
  printed: printing would print the same lines in every wait after it.
- **A position Minka forgot.** `cursorUnknown` starts the wait from now again,
  keeps that position and goes on waiting; the notice, given once however
  often it happens, says that what came in between is not shown. A position
  refused right after Minka handed it out ends the wait with exit 7, not
  retryable: starting anew would be refused the same way, for ever. A
  position refused on a following page of a full one ends the wait like any
  refusal, the mark where it was; the next wait asks from there.
- **The next wait.** The `wait` breadcrumb carries the filters, an explicit
  `--session` and `--timeout` of the run, so that following it neither
  changes the position nor shows what the run left out.
- **One line each.** The text view puts every notification on one line, line
  breaks in a title or a summary turned into blanks.
- **A failure.** A network failure or a 5xx ends the wait as any read: exit 6
  or 7, retryable, the mark where it was. There is no retry inside the wait:
  one rule holds for every read, and a wait that hung on through a dead
  connection would look the same as one that waits for news. The caller runs
  it again and loses nothing.

## One line, and the number on the bell

Waiting marks nothing read: the bell on the web still counts what a wait
printed. `notifications show <notificationId>` hands over the line as Minka
sent it (C-002) and marks nothing; its text view says the title, the project,
whether it is read, the moment, the ID and the address on the web.
`notifications mark-read <notificationId>` marks it read and hands over
`{unreadCount}` as Minka sent it; a line already read stays as it was.
`notifications count` hands over `{unreadCount}`. The ID is the `id` of a line
a wait printed, taken in any case and sent in lowercase. A 404 exits 2 with a
hint that names the reasons Minka has for it. Neither writes into a project:
a line is one's own, and one of an archived project is not found at all.

## Coordinating

`boards coordinator claim <boardId> --session <name>` claims or renews the
board for the session of that name, sent as written. `--session` is required
and never taken from `$CLAUDE_CODE_SESSION_ID`: the board shows the name to
people, and an identifier nobody can read tells them nothing. A claim without
it, or with a name Minka would refuse — empty, longer than 64 characters, or
with a control or format character — is refused as usage before a request.
Blanks and umlauts are part of a name, at either end as well, as Minka takes
it untrimmed: `--session "Planung März"`. The session of a wait is another
thing: it names a position on this machine and never leaves it.
`coordinatorTaken` exits 9: `data` is the holder as
Minka sent it, and the hint names them, when they were last active, until
when the entry holds and the release that clears a stale one. A claim in an
archived project exits 9 with `projectArchived`.

`boards coordinator release <boardId>` takes the entry off, `boards
coordinator show <boardId>` hands over `coordinator` of the board, null
included. Where the board leaves the field out, as for a client, `data` is
null as well, and a notice and the summary say that the board does not show
who coordinates it: null with that notice never means nobody. `boards show`
names the coordinator in its text view and says nothing where the field is
missing; `--json` carries the board as it came.

## The skill

The main part of the skill says in a few lines when to wait instead of
polling; the flow — claim, pull only assigned cards, wait in the background,
hand over — is in `references/waiting-and-coordinating.md` beside it, which
travels and is installed with it ([The skill](skill.md)). How an agent works
on a board — which column it takes work from, how it asks a person, how it
hands over — is in `references/working-on-a-board.md` beside that one, which
points to it for the mechanics.

