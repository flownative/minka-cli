# Saying which cards you are working on

```bash
minka heartbeat [--session <id>] --json
minka hooks install --json
minka hooks status --json
```

The board can show a card as alive while somebody works on it. Only an agent
can say so — with a person's token every signal is refused — and it works like
this:

- **`minka work start <card>` alone shows nothing.** It only remembers the
  card in the list of your session. What tells Minka is `minka heartbeat`, and
  until one has run, no board has heard a word. `work start` says so in its
  `notice` while nothing has ever reported that list.
- **Every session has a list of its own,** and its heartbeat reports that list
  and no other, as a client of its own. When your session ends, its cards go
  dark with it instead of being reported by the other sessions on the machine.
  The session is the one Claude Code names in `$CLAUDE_CODE_SESSION_ID` to the
  commands you run, and the mod reports under the same name; subagents share
  the session that spawned them, so their steps count for it. `--session <id>`
  names it explicitly, for the mod; you do not need it. Without either, a
  command speaks for the list of no session, which the mod never reports, and
  `work start` says so in its `notice` inside Claude Code.
- **The mod does the reporting.** `minka setup` installs it, and `minka hooks
  install` installs it again: code that runs inside the Claude Code process,
  for every project, and runs `minka heartbeat` at the start of every turn,
  after every tool call and every thirty seconds. So a tool call that runs for
  an hour keeps its tile alight, and a session's cards go dark three minutes
  after its process ended. Install it once, then `work start` is all a session
  does. It needs `claude` on the PATH, version 2.1.287 or later, the first that
  loads mods; without it, `hooks install` exits 1 with the two `claude plugin`
  commands to run in `hint`, and with an older one it exits 1 and says to
  update Claude Code. `setup` says either and goes on. It takes effect in
  sessions started afterwards. `minka hooks status --json` says in
  `data.mod.state` (`current`, `stale`, `not installed`) and `data.mod.enabled`
  what became of it, and in `data.mod.reason` when Claude Code is too old.
  `--from-mod` and `--idle` of `minka heartbeat` are the mod's own business —
  never pass them yourself. Removing the mod is `claude plugin uninstall
  minka-heartbeat@minka-cli` and `claude plugin marketplace remove minka-cli`.
- An earlier minka wrote a `PostToolUse` hook into `~/.claude/settings.json`
  that ran `minka heartbeat`. `minka hooks status --json` names it in
  `data.oldHook` as a leftover, and `minka hooks install` takes that entry out
  once Claude Code has the mod, leaving everything else in the file as it was;
  `data.oldHook.removed` says that it did. With a Claude Code too old for mods
  the hook stays.
- Without the mod — a Claude Code older than 2.1.287, or an agent that is not
  Claude Code — nothing reports for you: run `minka heartbeat` yourself every
  minute or two while you work on a card, since the board forgets it three
  minutes after the last signal.
- `minka work stop <card>` takes a card out again, and the next signal gives it
  up at once. Do it when you are done with a card; otherwise it stays alive
  until it runs out. It asks nothing of the network and the card may be named
  by its short identifier, but it needs a host like every other command, so
  without a sign-in it exits 3. A card your session does not hold is taken out
  of the list of whichever session on this machine holds it.
- `minka cards move` and `minka cards done` take the card out of every list by
  themselves and tell the board at once, where the card lands in another
  column, is put on hold or reaches Done; a card that only changes place where
  it already is stays in the list. Only a card your own session held is
  reported at once; one another session held goes with that session's next
  signal. The `notice` says where it happened, and
  whether the board has heard of it yet. So take a card up in that order: move
  it into the working column first, then `minka work start`.
- `minka work stop` without a card stops the one card your session holds. With
  several it exits 1 and changes nothing: name the card, or take all of your
  session's cards with `--all`.
- `minka work list --json` shows your session's list: `data.session`, then
  `data.cards` with `id`, `shortId`, `title` and `startedAt`, `data.elsewhere`
  with the cards of the other sessions on this machine, each with its
  `session`, `data.sentAt` when a signal of your session last went out,
  `data.reportedAt` when one last arrived, and `data.failure` what the host
  made of the last one that did not. The `notice` advises `minka hooks install`
  only while nothing has ever gone out; a signal that was turned away is no
  reason to install anything.
- **A card in the archive of its board is not taken at work.** `minka work
  start` on one exits 1 and says so, because no board shows a card in an archive
  as alive: take it out of the archive first.
- A card that is archived while somebody has it in hand is left out by Minka: it
  answers the signal with 200 and `leftOut`, notes the other cards of the set,
  and the card is taken out of the list of this machine, so that one card nobody
  can work on does not put out the tiles of the rest. The shorter list is
  reported with the next beat. It stands afterwards in `data.letGo` of `minka
  work list` and of `minka heartbeat --json`, with `id`, `shortId`, `title`,
  `reason` and `at`, and in the `notice` of `work list` in words, until it has
  stood there eight hours. So a card that is missing from `work list` and named
  in `letGo` was archived, not forgotten.
- A `reason` this CLI does not know takes no card out of the list: the card
  stays, the word is reported as it came, and the `notice` says so. Act on it
  yourself.
- `minka heartbeat` always exits 0 and prints nothing unless you ask with
  `--json`, which names the session it reported for in `data.session`. Not
  signed in, no network, a token that was revoked: none of it stops your work,
  and none of it is reported as a failure. If the board shows nothing, run
  `minka heartbeat --json` and read `data.sent` and `data.reason`, which names
  the case with the status and the code of the API, such as "(HTTP 403,
  agentsOnly)" for a token that belongs to a person and not to an agent. `minka
  work list --json` carries the same words in `failure` for the last signal that
  reached the host.
- Never write the store yourself.
- Minka forgets a card three minutes after the last signal, so a session that
  ends without `work stop` goes quiet by itself, three minutes after the last
  beat of its mod. Other sessions never extend that. A session that sits idle,
  with no tool call running, keeps its cards alight for twenty-five minutes
  after its last step, not for as long as Claude Code stays open.
- The list gives a card up eight hours after your session was last at it. Your
  heartbeat says so for the cards you started, so a card you keep working on is
  kept however long the work lasts. A card started outside a session — by a
  person at a terminal — goes by the moment it was started instead. `work list`
  carries a `notice` when a card was given up, and `minka work start` brings it
  back.
- A card you may not see exits 2 at `work start`; the server itself would pass
  it over without a word. There is no way to read back what Minka holds.
