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
- `work start` asks the server for the card: a card that cannot be seen is
  refused here, at the one moment somebody can do something about it, instead
  of being passed over in silence for as long as it is remembered.
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
