# Saying what you are working on

An agent tells Minka which cards it has in hand, and the board shows those
tiles as alive. `work start` and `work stop` remember them, `heartbeat` reports
them, `hooks install` makes the hook that calls it, and `hooks install --mod`
the mod that calls it from inside the Claude Code process.

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

## Every session has a list of its own

The cards lie in `work.json` beside the configuration file, never in a project
and never in the directory a command was started in: one list per host and
session. A signal carries the list of one session and nothing else, under a
client of that session's own (C-103). The server keeps a card alight for as
long as any client names it, so a session that reported the cards of another
would keep them glowing after that one died — for hours, as long as anything
else on the machine was at work.

- **Which session a command speaks for** is the one `--session` names, which
  is for a program that reports from inside a session and is told its name,
  and otherwise the one in `$CLAUDE_CODE_SESSION_ID`. Claude Code sets that
  variable for the commands its Bash tool runs, and the hook inherits it, so a
  typed `work start` and the beat after it name the same list. A name is 1 to
  128 letters, digits, `.`, `-` and `_`; `--session` refuses another as usage,
  a beat sends nothing under it, and one in the environment names nobody.
- **The input of a hook is not read,** although Claude Code documents its
  `session_id` as the session of a hook and does not document the variable. A
  typed command never sees that input, so a beat that went by it would report a
  list nobody writes into wherever the two differ or the variable is missing:
  the card would never be reported. Where the variable is missing, both sides
  speak for the list of no session, and `work start` says so in its `notice`
  when it runs inside Claude Code (`CLAUDECODE=1`).
- **A subagent** carries the session of whoever spawned it, in the variable as
  in the input of its hooks, so a card it works on is its parent's card, and
  its beats keep it.
- **Without a session** — a person at a terminal, or an agent that knows
  nothing of sessions — a command speaks for the list of no session. It is
  reported under the name of the installation alone, by a heartbeat that names
  no session and by the keepalive that such a beat starts; no beat of a session
  ever reports it. A per-terminal name would be none better: nothing that runs
  in a terminal runs the hook, so nothing would report under it either.
- `work list` and the answers of `work start` and `work stop` show the list of
  their session in `cards`, name it in `session` (null for no session), and
  show the cards of every other session of the host in `elsewhere`, each with
  its `session`, so that a card another session reports is not taken for one
  nobody holds.
- **A list is dropped** once it holds no card, keeps no note and has nothing
  sent that the server could still be counting: no set, or one older than the
  three minutes a signal holds. Until then the empty set it owes is reported by
  its next beat or by the keepalive, so that the tiles go out at once; after
  that the server has forgotten them by itself.
- **A store of the older shape**, one list per machine whose cards each name
  their session, is read and handed out: every card goes into the list of the
  session it names, with its sign of life, and a card that names none into the
  list of no session. What the machine was last sent, the notes about cards the
  host would not have and how the last signal fared stay with the list of no
  session, which reports under the client the whole machine reported as, so
  that its next signal takes back what is not its own. The mark of a beat of a
  list is the moment its own cards were last seen, not the last beat of the
  machine, so that a session that died long ago does not pass for one at work.
  Nothing is lost, and the next write leaves the new shape alone.

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
- A card is given up eight hours after its session was last at it (C-040), and
  the next `work list` says so. Every heartbeat of the session writes that
  moment down for its cards, at most once a minute and whether or not a signal
  goes out with it. So a piece of work that lasts longer than a day keeps its
  card by being worked on, while a list nobody reports any more is not kept.
- A card in the list of no session goes by the moment it was started. So does
  a card of a session that never runs the heartbeat. Saying `work start` again
  brings such a card back, and that is what the notice advises.
- `cards move` and `cards done` take the card out of every list themselves and
  report the list of their own session at once (C-041), so that a card somebody
  has moved on stops glowing without waiting for the next tool call. The list of
  another session is not reported from there: a signal carries the whole list it
  reports, so this session would be keeping that one's other cards alight. That
  session's next beat reports it, or the server forgets the card three minutes
  after that session's last signal. What counts as moving on is landing in
  another column, being put on hold or reaching Done; a card that only changes
  place where it already is is one nobody has let go of. Neither a store another
  session holds at that moment nor a signal that does not get through makes the
  move untrue, so both are passed over in silence; the `notice` says that the
  card left the list and whether the board has heard of it yet, and says nothing
  where the card was never in the list.
- A card the host names in `leftOut` leaves the list, so that one card it will
  not have cannot put out the tiles of everything else the session is at work
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
  Without a card it stops the one card of its session; where the session holds
  several, it says which and changes nothing. A card named that the session
  does not hold is taken out of the list of every other session that holds it,
  because whoever stops a card has let go of it, and the list of a session that
  died is cleared nowhere else.

## The client is a made-up name

`clientId` is random, made once and kept in the configuration as `clientId`
(C-037). It is never the host name and says nothing about the machine or the
person: it travels into Minka's database and its logs. A session reports as
that name, a dot and sixteen hex digits of a SHA-256 of the session's name,
shortened in front where it would not fit the 64 letters the server takes: the
same client with every signal of the session, another for every session, and
the session's own name never reaches the server. The list of no session
reports as the plain name.

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
  minutes. The throttle is the list's own and is written into the store before
  the request goes out, because every signal is a process of its own.
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

- **One per host, deciding on every session by its own beat.** The lock is an
  advisory lock of the kernel beside the store, named after the host it serves,
  and it is held for as long as the process reports. A beat that finds it taken
  starts nothing, and a keepalive that was killed leaves nothing behind, the
  kernel giving the lock back with the process. One lock for the whole machine
  would let the cards of a second host go dark without a word, which is worse
  than a second process. Every tick it reports the list of every session whose
  own last beat is younger than twenty-five minutes, under that session's
  client, and nothing of a session whose beat is older: a session that is busy
  says nothing about one that died. One process per session would decide the
  same and only multiply the processes.
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
  and only then let go of, so the tiles go out at once. Twenty-five minutes in
  which the session was not at the store, and a wall clock that jumped further
  than a tick can account for, end the reporting without a signal: the cards
  may still be in hand, and the server forgets them by itself. The keepalive
  stops once no session is left to report, and at once on a jump.
- **It does not light again what the server forgot.** A list whose last signal
  is older than the three minutes the server counts it for, and whose session
  has not been at the store since, is passed over: a keepalive that starts after
  a machine woke up, or after the last one stopped, would otherwise light the
  tiles of a session that may be gone. The session's next beat lights them
  again where it is not.
- **What it cannot see** is a session that ended while its last beat is young:
  nothing tells the keepalive that a session is gone, so the cards of a session
  that crashed or exited without `work stop` stay alight up to twenty-five
  minutes after its last beat, then three minutes more on the server. Another
  session's work no longer extends that, and a session that runs the mod is no
  such case: the keepalive leaves it alone, see below. A machine that slept or
  lost its power is no such case: the jump of the clock ends the keepalive, and
  the server forgets the cards three minutes after the last signal. The jump
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

## The mod reports from inside the process

The keepalive cannot tell a session that ended from one that is busy, because
it runs outside the process of the session. The mod runs inside it (C-105): a
Claude Code mod, a module of JavaScript that Claude Code calls on its own
events, which runs `minka heartbeat --session <id> --from-mod` through an
argument list, no shell. When the process ends, so do its beats.

- **When it beats:** at the start of the session, at the start of every turn,
  at the end of every tool call, and every thirty seconds. The timer of a mod
  runs while a tool call runs, so a build of an hour keeps its tiles for the
  hour. A beat is skipped where the one before has not returned yet, and one
  more follows it where a step came in meanwhile, so beats never pile up behind
  a minka that is slow; the one that follows is work where any step it stands
  for was.
- **Which session:** the one `$.session.id()` names, asked on every beat, and
  that is the session `$CLAUDE_CODE_SESSION_ID` names to the Bash tool, so a
  typed `work start` and the beats of the mod meet in one list. The mod has to
  pass it as `--session`: what the mod starts does not carry the variable.
- **What counts as work** is a step: the start of a turn and the end of a tool
  call. A beat of the timer is work while a tool call runs — of the main
  conversation or of any subagent, which may still work in the background
  after the turn that started it has ended — and `--idle` otherwise. Turns are
  not followed: a subagent's turn ends without having started, and a turn whose
  end never came would count as work for as long as the process runs.
- **An idle beat** says the process is alive and nothing more. Like the keepalive, it writes neither the sign of life
  of a card nor the mark of a beat, and it reports a list only for as long as
  the keepalive would report it: twenty-five minutes after the last step, and
  not after the server forgot it while nobody was at work. A Claude Code left
  open over night therefore does not keep its cards alight until the morning,
  nor past the eight hours of C-040.
- **A list a beat of the mod reaches is marked** (`mod` in the store) for as long
  as the list lives. The keepalive passes such a list over, and refuses a signal
  for it even where the list was marked after the tick read the store; a beat
  of the hook or of the mod starts no keepalive for it. So a session with the
  mod goes dark at most three minutes after its last beat: its last signal is no
  later than that beat, and nothing else reports it. The keepalive stops once
  only such lists are left. Nothing takes the mark back: where the mod is
  uninstalled while a list lives, that list is reported by its hook alone, and a
  tool call that runs for longer than three minutes lets its tiles go dark until
  the next beat. A list dropped and taken up anew starts without it.
- **A beat of the mod without a session** reports nothing: the list of no
  session is nobody's to keep alight from inside a process.
- **Subagents** run inside the process of their session. Their tool calls reach
  the mod of that process and beat for that session, as their hook did.
- **The program it runs** is the minka that installed it, by the path it was
  started from. Installed from the link Homebrew keeps on the PATH, that is the
  link, which every upgrade keeps pointing at the new binary, and the path a
  firewall knows minka by. Installed from any other path, such as `bin/minka` of
  a checkout, the beats go out from that program, and a firewall that lets only
  the Homebrew one through holds them back, silently like every failure of a
  beat.

`hooks install --mod` writes the mod beside the configuration, in
`claude-mod/`, as a marketplace named `minka-cli` holding one plugin,
`minka-heartbeat`, and runs `claude plugin marketplace add <dir>` and `claude
plugin install minka-heartbeat@minka-cli --scope user`. Claude Code loads a
plugin of a marketplace in a directory from that directory, so a later install
that writes other files reaches every session started after it, without an
update in Claude Code; running it again is harmless, and Claude Code answers
the second time that it has both. Where `claude` cannot be run, the files are
written all the same, the command exits 1 and the `hint` carries the two
command lines, each argument quoted where a shell would read it otherwise. The
mod takes neither `--file` nor `--command`: it is installed for every project
and runs this minka.

What the install writes, besides `claude-mod/`, Claude Code writes itself: the
marketplace in `extraKnownMarketplaces` and the plugin in `enabledPlugins` of
its `settings.json`, and its own files under `plugins/` of its configuration.
It is Claude Code's own writer on a flag that asks for it, so the rule that
minka leaves a settings file as it finds it beyond its own entry (C-039) holds.
Undoing it is Claude Code's as well:

```
claude plugin uninstall minka-heartbeat@minka-cli
claude plugin marketplace remove minka-cli
```

after which `claude-mod/` may be deleted.

The hook may stay beside the mod. Both beat after a tool call then, and the
signal still goes out at most once a minute, so nothing is reported twice; what
the hook costs is one process per tool call. `hooks install --mod` says so in
its `notice` where the hook is there. Removing the hook is left to whoever wrote
the settings (C-039). A session without the mod is reported by the hook and the
keepalive as before.

Claude Code reads its settings from `$CLAUDE_CONFIG_DIR` where that is set,
and `~/.claude` otherwise; `hooks install` and `hooks status` without `--file`
go by the same rule, for the hook and the mod alike.

`hooks status` says in `mod` what lies in the directory — `current` where it is
what this minka writes, `stale` where another minka wrote it, `not installed`
where nothing does — and in `enabled` whether the settings tell Claude Code to
load it.

## The hook belongs to whoever wrote the settings

`hooks install` adds a `PostToolUse` hook to `~/.claude/settings.json` — the
`settings.json` in `$CLAUDE_CONFIG_DIR` where that is set — or to the file named
with `--file`. Everything else in that file is kept, a file that
carries the hook already is left alone, and a file that is not shaped the way
Claude Code writes it is refused rather than repaired, with the entry to add by
hand in the hint (C-039). A `hooks` that stands there as `null` is read as no
hooks and written into all the same, so that the status and the install never
say different things about one file. The entry names the minka that writes it
by its full path, so the hook does not depend on what is on the PATH of whoever
starts the agent; where that program is not there any more, the status says so
instead of calling the hook current.

The default is the settings of the whole machine, and that is where the hook
belongs: every session reports its own cards through it, so a hook for one
project alone would report that project's sessions and leave the rest of the
machine quiet. A file for one project is therefore
`.claude/settings.local.json` and never `.claude/settings.json`, which is meant
to be committed: the entry names a path of this machine, which no other
checkout has. Where the file lies in a working tree of Git, `hooks install`
says so and points at `--command minka`, which writes a bare command instead of
a path.

The hook needs nothing for the sessions: it inherits the session of Claude Code
in `$CLAUDE_CODE_SESSION_ID`, so the entry is the same for every session. `hooks
status` says whether the hook is there and nothing about cards; which session
holds what is `work list`.
