# Saying what you are working on

An agent tells Minka which cards it has in hand, and the board shows those
tiles as alive. `work start` and `work stop` remember them, `heartbeat` reports
them, `hooks install` makes the hook that calls it.

## What the server holds

`PUT /me/work` takes the whole set every time, and it is the set of one
**client** of one agent: what arrives last is what holds, a card left out is
given up in the same breath, and an empty set says that the client has
stopped. A signal counts for three minutes. Nothing can be read back, and a
card the agent may not see is passed over without a word.

A card in the archive of its board is not taken, because nobody works on one.
The answer is then 200 and names it in `leftOut` as `{cardId, reason}`, with the
rest of the set noted all the same; an answer that leaves nothing out is the 204
it always was.

Only an agent may say so. A person's token is refused with `agentsOnly`.

## The list belongs to the machine

The cards lie in `work.json` beside the configuration file, never in a project
and never in the directory a command was started in. Every session on the
machine writes into that one list, and every signal reports its union
(C-036): the server keeps one set per client, so two sessions with lists of
their own would take each other's cards away with every signal.

- The list is read and written under an advisory lock of the kernel on
  `work.lock`, and saved by writing a temporary file and renaming it. Without
  the lock, two sessions starting a card at the same moment would read the same
  list and the later write would drop the other's card.
- A command somebody typed waits its turn for the lock: `work start`, `work
  stop`, `work list`, and what `cards move` and `cards done` take out of the
  list. Every holder reads and writes one small file and lets go, so a line of
  sessions starting at once is through in well under a second. The wait ends
  after 30 seconds, which only a holder that hangs or a disk that does not
  answer reaches. A `work` command then exits 1 (`config`), changes nothing,
  and its `hint` says to try again; `cards move` and `cards done` pass it over
  like every other trouble with the list. Giving up sooner would cost a card
  that somebody started on a machine that is merely slow.
- A beat waits for nobody: where another minka holds the store, `heartbeat`
  sends nothing and says so in `reason`, because the next beat comes after the
  next tool call and a hook that waited would hold that tool call up. What is
  written beside a signal — how it fared — and every tick of the keepalive wait
  2 seconds and pass the moment over after that; the keepalive's own lock,
  which stands for a running process, is not waited for at all.
- `work start` asks the server for the card: a card that cannot be seen is
  refused here, at the one moment somebody can do something about it, instead
  of being passed over in silence for as long as it is remembered. A card in the
  archive of its board is refused the same way, as usage: it is readable and no
  board shows it as alive, so taking it up would end in a beat that lets it go
  again and says so to nobody.
- A card is given up eight hours after the session that took it up was last at
  it (C-040), and the next `work list` says so. A card remembers that session
  by the name it finds in `CLAUDE_CODE_SESSION_ID`, and every heartbeat of that
  session writes the moment down again, at most once a minute and whether or
  not a signal goes out with it. So a piece of work that lasts longer than a
  day keeps its card by being worked on, while the card of a session that died
  runs out however busy the machine around it is — which the sign of life would
  not manage if it were the signal, because a signal carries the whole list of
  the machine. What binds a card to a session is that name and nothing else:
  every heartbeat that finds the same name in the variable keeps the card, one
  out of a subagent as well, wherever a subagent is given the name of the
  session that spawned it.
- A card taken up where nothing says who is asking, a person at a terminal for
  instance, carries no session and goes by the moment it was started. So does a
  card of a session that never runs the heartbeat. Saying `work start` again
  brings such a card back, and that is what the notice advises.
- `cards move` and `cards done` take the card out of the list themselves and
  report at once (C-041), so that a card somebody has moved on stops glowing
  without waiting for the next tool call. What counts as moving on is landing
  in another column, being put on hold or reaching Done; a card that only
  changes place where it already is is one nobody has let go of. Neither a store another
  session holds at that moment nor a signal that does not get through makes the
  move untrue, so both are passed over in silence; the `notice` says that the
  card left the list and whether the board has heard of it yet, and says
  nothing where the card was never in the list.
- A card the host names in `leftOut` leaves the list, so that one card it will
  not have cannot put out the tiles of everything else the machine is at work
  on (C-053). The list is shorter than what the last signal carried then, which
  is a list that has changed, so the next beat reports the corrected set at once
  and `sentAt` and `sentCards` go on saying what went out. A word this CLI does
  not know takes no card out of anything: the note is written and the card
  stays, so the list is what it was and the set the host has spoken about is not
  sent again before the minute is out. What was let go stands in `work list` as
  `letGo` — the card as the list knew it, the word of the host and the moment —
  and in its `notice` in words, because after the card is gone from the list
  nothing else says why. `work start` and `work stop` name only a note about the
  card they were given, since a note stands for as long as the window of C-040,
  in which the notes are swept with the cards.
- `work stop` asks nothing of the network and finds a card under either of the
  names it was remembered with. It needs the host all the same, so on a machine
  that is signed in nowhere it ends like every other command, with exit 3.
  Without a card it stops the one card that is remembered; where several are,
  it says which and changes nothing, because another session may have put one
  of them there.

## The client is a made-up name

`clientId` is random, made once and kept in the configuration as `clientId`
(C-037). It is never the host name and says nothing about the machine or the
person: it travels into Minka's database and its logs.

Minka reads only a `clientId` left out or `null` as naming no client; an empty
one is refused with 400 `malformedBody`, as one that is too long or carries
anything but letters, digits, `.`, `-` and `_`. The CLI never sends an empty
one: a configuration without a name, or with an empty one, gets a new name
before the signal goes out.

## The signal disturbs nothing

`heartbeat` runs after every tool call of an agent. It therefore
(C-038):

- exits 0 whatever happens and says nothing unless asked with `--json`,
  `--quiet` or on a terminal. An expired token, a host that does not answer, a
  person's token: none of it is something the agent could act on from inside a
  tool call, and a hook that fails loudly costs it its attention. Asked, its
  `reason` names the case, with the status and the code of the API where a
  request was turned away. What a request made of it stands in `work list` as
  `failure`, in the same words; a signal that never got that far — nobody
  signed in, the store held by somebody else — leaves nothing behind, and
  `heartbeat --json` is what says so.

Going out and arriving are two moments, and the list keeps both: `sentAt` for
the last signal that left, `reportedAt` for the last one that arrived. Only
where nothing has ever gone out does a list advise `hooks install`. A signal
that reached the host and was turned away is no reason to go and install
anything, and saying both at once would send somebody after a problem they do
not have.
- returns at once, without reading the configuration and without a request,
  where nothing was ever remembered on the machine.
- reports at most once a minute, and at once whenever the list has changed, so
  that a card that was given up goes quiet without waiting out the three
  minutes. The throttle is written into the store before the request goes out,
  because every signal is a process of its own.
- bounds its request to five seconds, far below the timeout of every other
  command.

## One process reports while nobody else can

One tool call that lasts makes no signal at all, and the server forgets a card
three minutes after the last one: the tile would go dark while the work goes on.
So a beat that is not itself the keepalive and finds a card in hand starts
`minka heartbeat --keepalive` (C-043): a process of its own that reports the
list until nobody is at work on it any more.

What that is worth is twenty-five minutes from the last beat of the session.
While the one call runs nothing is at the store, so the mark of a beat stands
still with it and the keepalive stops that long after it. A test run or a build
of a quarter of an hour keeps its tiles, and so does a question that waits that
long for an answer; a call of an hour outlives them.

A subagent is no such case: its tool calls fire the hook, and under the name of
the session that spawned it, so a session working through subagents reports by
itself.

- **One per host.** The lock is an advisory lock of the kernel beside the
  store, named after the host it serves, and it is held for as long as the
  process reports. A beat that finds it taken starts nothing, and a keepalive
  that was killed leaves nothing behind, the kernel giving the lock back with
  the process. One lock for the whole machine would let the cards of a second
  host go dark without a word, which is worse than a second process.
- **It is at no work of its own,** and two things say so, both of them: the
  child is given the environment of the beat that started it less
  `CLAUDE_CODE_SESSION_ID`, and a beat in that mode writes neither the sign of
  life of a card nor the mark of a beat, whatever the environment carries. The
  filter alone would be one stray variable away from a card glowing through a
  working day while nothing ever stopped the reporting.
- **It is started detached:** all three streams at the null device, a session
  of its own, nobody waiting for it, and a working directory that holds no
  project. Claude Code reads the output of a hook through a pipe, and a child
  that held the write end would keep the tool call waiting for as long as the
  keepalive lives — the opposite of the point. The own session is also what
  keeps the Ctrl-C meant for the agent away from it.
- **It wakes every thirty seconds** and lets the throttle of the signal be the
  reporting rate. A ticker of exactly a minute falls a hair short of the
  throttle every second or third time, and the gap grows past the three minutes
  the server counts a signal for: the tile would flicker.
- **Every tick sweeps,** like every other writer of the store. When a session
  dies the keepalive is often the last process left, and without the sweep it
  would report a card past the eight hours of C-040.
- **It ends in one of three ways, and they differ.** An empty list is reported
  and only then is it over, so the tiles go out at once. Twenty-five minutes in
  which nothing but the keepalive was at the store, and a wall clock that
  jumped further than a tick can account for, end it without a signal: the
  cards may still be in hand, and the server forgets them by itself. The jump
  is measured on the wall clock, because the monotonic clock stands still while
  a machine sleeps and would show no jump at all.
- **What says that somebody is at work** is the moment of a beat or a typed
  command, written into the store at most once a minute the way the sign of
  life is: a beat that is not the keepalive, `work start`, `work stop`, and
  what `cards move` and `cards done` reach through the signal. `work list`
  writes nothing — looking at the list is not being at work, and a command that
  reads does not become a writer for this. The deadline hangs on the invocation
  and not on a request going out, because once the keepalive reports every
  minute the throttle swallows most beats, and a deadline measured on requests
  would expire in a busy session.
- `heartbeat --json` says in `keepalive` what became of it: `started`, `already
  reporting`, or `not started` with the reason. Without that word nobody could
  tell a tile that stays dark from a keepalive that never ran. The text view and
  the exit code of a hook are what they were: silent, exit 0. The keepalive says
  in `ended` why it stopped, which nothing reads while it runs detached, and
  nothing it meets is a failure either — a host it cannot find, a lock it cannot
  take, a store nobody hands over: it stops and says so, and exits 0 like every
  other heartbeat (C-038).
- `--keepalive` is minka's own business: hidden from the help and from `minka
  commands`, and nothing but a beat has reason to call it.

## The hook belongs to whoever wrote the settings

`hooks install` adds a `PostToolUse` hook to `~/.claude/settings.json`, or to
the file named with `--file`. Everything else in that file is kept, a file that
carries the hook already is left alone, and a file that is not shaped the way
Claude Code writes it is refused rather than repaired, with the entry to add by
hand in the hint (C-039). A `hooks` that stands there as `null` is read as no
hooks and written into all the same, so that the status and the install never
say different things about one file. The entry names the minka that writes it
by its full path, so the hook does not depend on what is on the PATH of whoever
starts the agent; where that program is not there any more, the status says so
instead of calling the hook current.

The default is the settings of the whole machine, and that is where the hook
belongs: what is reported is the machine — one list, one client, the union of
every session on it — so a hook for one project alone would report that
project's sessions and leave the rest of the machine quiet. A file for one
project is therefore `.claude/settings.local.json` and never
`.claude/settings.json`, which is meant to be committed: the entry names a path
of this machine, which no other checkout has. Where the file lies in a working
tree of Git, `hooks install` says so and points at `--command minka`, which
writes a bare command instead of a path.
