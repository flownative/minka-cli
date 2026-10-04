# Saying what you are working on

An agent tells Minka which cards it has in hand, and the board shows those
tiles as alive. `work start` and `work stop` remember them, `heartbeat` reports
them, and the mod that `setup` and `hooks install` install calls it from inside
the Claude Code process.

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
  is for the mod, which reports from inside a session and is told its name,
  and otherwise the one in `$CLAUDE_CODE_SESSION_ID`. Claude Code sets that
  variable for the commands its Bash tool runs, and it is the name the mod
  passes, so a typed `work start` and the beat after it name the same list. A
  name is 1 to 128 letters, digits, `.`, `-` and `_`; `--session` refuses
  another as usage, a beat sends nothing under it, and one in the environment
  names nobody.
- **A heartbeat reads nothing from its standard input.** Run as a hook of
  Claude Code, it would be given the `session_id` of the hook there; a typed
  command never sees that input, so a beat that went by it would report a list
  nobody writes into wherever the two differ. Where the variable is missing, a
  typed command speaks for the list of no session, which the mod never
  reports, and `work start` says so in its `notice` when it runs inside Claude
  Code (`CLAUDECODE=1`).
- **A subagent** carries the session of whoever spawned it, so a card it works
  on is its parent's card, and the steps it makes keep it.
- **Without a session** — a person at a terminal, or an agent that knows
  nothing of sessions — a command speaks for the list of no session. It is
  reported under the name of the installation alone, by a heartbeat that names
  no session; no beat of a session ever reports it. Nothing runs such a
  heartbeat but whoever types it, so the cards of that list are alight for three
  minutes after each one, and an agent without the mod that wants its tiles
  alight runs it every minute or two.
- `work list` and the answers of `work start` and `work stop` show the list of
  their session in `cards`, name it in `session` (null for no session), and
  show the cards of every other session of the host in `elsewhere`, each with
  its `session`, so that a card another session reports is not taken for one
  nobody holds.
- **A list is dropped** once it holds no card, keeps no note and has nothing
  sent that the server could still be counting: no set, or one older than the
  three minutes a signal holds. Until then the empty set it owes is reported by
  its next beat, so that the tiles go out at once; after that the server has
  forgotten them by itself.
- **A store of the older shape**, one list per machine whose cards each name
  their session, is read and handed out: every card goes into the list of the
  session it names, with its sign of life, and a card that names none into the
  list of no session. What the machine was last sent, the notes about cards the
  host would not have and how the last signal fared stay with the list of no
  session, which reports under the client the whole machine reported as, so
  that its next signal takes back what is not its own. The mark of a beat of a
  list is the moment its own cards were last seen, not the last beat of the
  machine, so that a session that died long ago does not pass for one at work.
  Nothing is lost, and the next write leaves the new shape alone. A field the
  store no longer knows, such as the `mod` mark of a list, is read past and
  left out of the next write.

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
  sends nothing and says so in `reason`, because the next beat comes within
  thirty seconds and a beat that waited would pile up behind the ones after
  it. What is written beside a signal — how it fared — waits 2 seconds and
  passes the moment over after that.
- `work start` asks the server for the card: a card that cannot be seen is
  refused here, at the one moment somebody can do something about it, instead
  of being passed over in silence for as long as it is remembered. A card in the
  archive of its board is refused the same way, as usage: it is readable and no
  board shows it as alive, so taking it up would end in a beat that lets it go
  again and says so to nobody.
- A card is given up eight hours after its session was last at it (C-040), and
  the next `work list` says so. Every beat of the session that stands for a
  step writes that moment down for its cards, at most once a minute and whether
  or not a signal goes out with it. So a piece of work that lasts longer than a
  day keeps its card by being worked on, while a list nobody reports any more
  is not kept.
- A card in the list of no session goes by the moment it was started. So does
  a card of a session that never runs the heartbeat. Saying `work start` again
  brings such a card back, and that is what the notice advises.
- `cards move` and `cards done` take the card out of every list themselves and
  report the list of their own session at once (C-041), so that a card somebody
  has moved on stops glowing without waiting for the next beat. The list of
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

`heartbeat` runs after every tool call of an agent and every thirty seconds
besides. It therefore (C-038):

- exits 0 whatever happens and says nothing unless asked with `--json`,
  `--quiet` or on a terminal. An expired token, a host that does not answer, a
  person's token: none of it is something the agent could act on from inside a
  tool call, and a beat that fails loudly costs it its attention. Asked, its
  `reason` names the case, with the status and the code of the API where a
  request was turned away. What a request made of it stands in `work list` as
  `failure`, in the same words; a signal that never got that far — nobody
  signed in, the store held by somebody else — leaves nothing behind, and
  `heartbeat --json` is what says so.
- returns at once, without reading the configuration and without a request,
  where nothing was ever remembered on the machine.
- reports at most once a minute, and at once whenever the list has changed, so
  that a card that was given up goes quiet without waiting out the three
  minutes. The throttle is the list's own and is written into the store before
  the request goes out, because every signal is a process of its own.
- bounds its request to five seconds, far below the timeout of every other
  command.

Going out and arriving are two moments, and the list keeps both: `sentAt` for
the last signal that left, `reportedAt` for the last one that arrived. Only
where nothing has ever gone out does a list advise `hooks install`. A signal
that reached the host and was turned away is no reason to go and install
anything, and saying both at once would send somebody after a problem they do
not have.

## The mod reports from inside the process

The mod is the one thing that reports a Claude Code session (C-107): a Claude
Code mod, a module of JavaScript that Claude Code calls on its own events,
which runs `minka heartbeat --session <id> --from-mod` through an argument
list, no shell. It runs inside the process of the session, so when the process
ends, so do its beats, and the board forgets the cards three minutes after the
last one. Nothing else reports for a session: no process of minka's own goes
on after a beat, and a session whose Claude Code loads no mod reports only
what is run by hand. Claude Code loads mods from 2.1.287 on; an older one
takes the plugin without a word and never runs it.

Upgrading from a minka that ran a keepalive leaves two things behind for a
while: the `keepalive-*.lock` files beside the configuration, which nothing
reads any more and which may be deleted, and a keepalive that was running at
the moment, which may light the cards of a session of the mod that has ended
once more, for up to twenty-five minutes plus the three the server counts,
before it stops by itself.

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
- **A tool call that never returns** counts for the timer for two hours from
  its start and no longer, and one that Claude Code abandons, as on an
  interrupt, stops counting at once (`next.signal` of the mods API). A hook
  chain left behind or a hung MCP tool would otherwise keep the session at
  work, and its cards alight, until the process ends: the eight hours of
  C-040 do not end it, because every beat at work renews the card. Two hours
  is longer than any tool call that runs without a step of its own: the Bash
  tool gives up after minutes, and a subagent working for hours in the
  foreground makes tool calls of its own, each of which is work while it runs.
  A call past the bound is forgotten, so calls that never return do not pile
  up.
- **An idle beat** says the process is alive and nothing more (C-105). It
  writes neither the sign of life of a card nor the mark of a beat, and it
  reports a list only for twenty-five minutes after the last step, and not
  after the server forgot it while nobody was at work. A Claude Code left open
  over night therefore does not keep its cards alight until the morning, nor
  past the eight hours of C-040. What says that somebody is at work is the
  moment of a step or a typed command, written into the store at most once a
  minute the way the sign of life is: a beat that is not idle, `work start`,
  `work stop`, and what `cards move` and `cards done` reach through the signal.
  `work list` writes nothing — looking at the list is not being at work.
- **A beat of the mod without a session** reports nothing: the list of no
  session is nobody's to keep alight from inside a process.
- **Subagents** run inside the process of their session. Their tool calls reach
  the mod of that process and beat for that session.
- **The program it runs** is the minka that installed it, by the path it was
  started from. Installed from the link Homebrew keeps on the PATH, that is the
  link, which every upgrade keeps pointing at the new binary, and the path a
  firewall knows minka by. Installed from any other path, such as `bin/minka` of
  a checkout, the beats go out from that program, and a firewall that lets only
  the Homebrew one through holds them back, silently like every failure of a
  beat.
- `--from-mod` and `--idle` are minka's own business: hidden from the help and
  from `minka commands`.

## Installing the mod

`setup` installs the mod after the sign-in and the skill, and `hooks install`
installs it on its own (C-108). Both write the mod beside the configuration, in
`claude-mod/`, as a marketplace named `minka-cli` holding one plugin,
`minka-heartbeat`, and run `claude plugin marketplace add <dir>` and `claude
plugin install minka-heartbeat@minka-cli --scope user`, for every project.
Claude Code loads a plugin of a marketplace in a directory from that directory,
so a later install that writes other files reaches every session started after
it, without an update in Claude Code; running it again is harmless, and Claude
Code answers the second time that it has both.

- **Only a Claude Code that loads mods is told of it.** Before the two
  commands, `claude --version` is asked. Its standard output alone counts, and
  in it the first word of three numbers, such as `2.1.289` of `2.1.289 (Claude
  Code)`, with a pre-release such as `-rc1` behind it passed over; a line of a
  warning before it, or anything on standard error, does not hide it. `v2.1.289`
  is no such word. A version below 2.1.287, or one that cannot be read, is not
  told of the mod: it would take the plugin, exit
  0 and never run it, and the install would report a mod that reports nothing
  and take out the hook that still did. The state is `written`, the `reason`
  says to update Claude Code, and the old hook stays. `hooks status` asks the
  same and says it in `mod.reason`, whatever the settings tell Claude Code.
- **Where `claude` cannot be run,** or refuses, the files are written all the
  same and the state is `written`. `hooks install` exits 1 and its `hint`
  carries the two command lines, each argument quoted where a shell would read
  it otherwise, or the advice to update Claude Code. `setup` says so and goes
  on: the sign-in is done and worth keeping, and it exits 0 with the state and
  the `reason` in `data.mod`. `claude` missing from the PATH is the common case
  of this, and setup names it as such. A mod that cannot be written at all ends
  `hooks install` with exit 1, and setup goes on with the state `not
  installed` and the `reason`.
- **`setup --no-mod`** writes nothing of the mod, runs no `claude` and leaves
  the settings of Claude Code alone; `data.mod.state` is `skipped`. An old hook
  it finds is named in `data.oldHook` and in the `notice`, as `hooks status`
  names it.
- **`hooks install --mod`** is what `hooks install` does anyway, and is taken
  without a word, hidden from the help, so that a script written for the time
  the mod had to be asked for keeps working. `--file` and `--command` are gone
  with the hook they wrote, and are refused as usage.
- **What setup says** stands in `data.mod`: `path`, `plugin`, `program`,
  `state` (`installed`, `current`, `written`, `not installed` or `skipped`) and
  `reason` where Claude Code was not told; and in `data.oldHook`, null or what
  became of the old hook, as `hooks install` says it.

What the install writes, besides `claude-mod/`, Claude Code writes itself: the
marketplace in `extraKnownMarketplaces` and the plugin in `enabledPlugins` of
its `settings.json`, and its own files under `plugins/` of its configuration.
It is Claude Code's own writer, run on the one command whose job it is.
Undoing it is Claude Code's as well:

```
claude plugin uninstall minka-heartbeat@minka-cli
claude plugin marketplace remove minka-cli
```

after which `claude-mod/` may be deleted.

Claude Code reads its settings from `$CLAUDE_CONFIG_DIR` where that is set,
and `~/.claude` otherwise; `setup`, `hooks install` and `hooks status` go by
the same rule.

`hooks status` says in `mod` what lies in the directory — `current` where it is
what this minka writes, `stale` where another minka wrote it, `not installed`
where nothing does —, in `enabled` whether the settings tell Claude Code to
load it, and in `settings` which settings it read. Settings that cannot be read
tell it nothing: `enabled` is false.

## The old hook is taken out

An earlier minka wrote a hook into the settings of Claude Code: under
`hooks.PostToolUse`, a group of the matcher `*` holding one hook of the type
`command` that ran `<path of minka> heartbeat`. The mod does all it did, so the
hook is a leftover (C-109).

- **`hooks status`** names it in `oldHook` — `path`, `command`, `removed`
  false — with a `notice` and the breadcrumb to `hooks install`, and changes
  nothing.
- **`hooks install` and `setup`** take it out once a Claude Code that loads
  mods has been told of the mod, and say so in `oldHook` with `removed` true.
  Not before, and not where its version cannot load the mod: a session left
  with neither would report nothing. And after `claude plugin install`,
  because that writes the same file, and what it wrote is kept.
- **What is the hook of minka** is exactly what minka wrote: a hook of the type
  `command` whose command line is `minka`, or a program called `minka` by its
  absolute path, spaces and all, quoted or not, followed by ` heartbeat` and
  nothing else. A command line that does anything more is somebody else's,
  even where it runs minka: one that runs another program first (`echo`,
  `nice`), sets a variable, changes the directory or names a relative path.
  Every such hook in the file goes, where it was written twice. A key that
  stands twice in one object on the way to the hook — `hooks`, `PostToolUse`,
  the `matcher` and `hooks` of a group, the `type` and `command` of a hook —
  leaves the file alone with a `notice`: which of the two counts is the
  reader's business, and a cut by a guess could take what somebody else wrote.
- **What goes with it** is what only held it: the group around it where that
  group is the one minka wrote — the matcher `*` and the hooks, nothing else —
  and holds nothing more, the list of `PostToolUse` where it is left empty, and
  `hooks` where it is left empty. So a file whose only hook was minka's reads
  as it would have without it. A group with anything else in it keeps an empty
  list of hooks.
- **The cut of minka changes nothing else, byte for byte:** the other hooks
  under the same event and others, every other key, their order, the
  indentation, the line endings and the spaces stay as the file had them when
  minka read it. The entry is cut out of the text with the comma that joined
  it, not read and written anew, and a cut that would leave no JSON is refused.
  The file is written whole or not at all, by a temporary file renamed over it,
  with the permissions it had; a file behind a symbolic link is written where
  the link points, and stays a link. What `claude plugin install` did to the
  file a moment before is Claude Code's: it writes the whole file anew, in an
  order and an indentation of its own and without a byte order mark, so the
  file after an install is not the file somebody wrote, wherever the hook
  stood.
- **A file that is no JSON** — one with comments in it, say — is no file to
  change. Claude Code refuses such a file itself, so `claude plugin install`
  fails on it and the cut is never reached; where it is reached all the same,
  the file is left alone, and the `notice` of `hooks install`, `setup` and
  `hooks status` says that whether it carries the hook cannot be told. Where a
  hook was found and writing the file failed, `oldHook.reason` says why and the
  `notice` says to take the entry out of `hooks.PostToolUse` by hand.
- **Only the settings Claude Code reads for every project** are looked into: a
  hook of minka in the settings of one project is taken out by hand.
