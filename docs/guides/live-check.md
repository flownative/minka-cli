# Live check

A new API message counts as checked only once it ran against a running
instance. Until then the status says it is checked against the fake only.

## Where writing is allowed

| Instance | Account | Writing allowed |
|---|---|---|
| `dev.minka.flownative.com` | agent account "Claude", acts for Robert | anywhere in the project "Flownative HQ", and privately to Shaun |
| `minka.localbeach.net` | smoke account Bruno | only the board "API-3 Abnahme" and the chat "Chat1 Abnahme" in "Test-Projekt" |

The chat "Sommerfest" on the local instance is Robert's demo and stays
untouched. Only what the run created itself gets deleted.

The default host is the test instance; the local one needs
`--host https://minka.localbeach.net`.

Anything beyond that — writing to another person, creating something in a
project of others — is a decision of Robert or Shaun, not of the coordinator.

## Procedure

1. `make build`, so that `bin/minka` has the state under test.
2. Create data of your own: a card, a message, a comment. Never continue on
   somebody else's.
3. Play through the reported case, the refusals too: second call, foreign ID,
   value too long, missing permission.
4. Look at the text view, not only at `--quiet`. An error in the rendering
   shows up only there.
5. Tidy up: delete your own cards, remove your own messages. An upload that no
   stored text or upload points to disappears by itself on a sweep after at
   least seven days; an upload of a Docs & Files tool stays until it is removed
   on the web.
6. What the run showed goes into the status; what it uncovered in the CLI gets
   fixed at once.

## The keepalive

Checking the keepalive live means watching a tile through a tool call that runs
for longer than the three minutes a signal counts for: `work start`, one beat,
then something that takes five minutes, and the tile alight all through it.
Afterwards `minka heartbeat --json` says in `keepalive` that one is reporting,
and `work stop` empties the list, whereupon the tile goes out at once.

That nothing is left behind is seen on the process, not on the file: the lock
beside the configuration stays where it is and says nothing about who holds it.
So `pgrep -fl 'minka heartbeat --keepalive'` finds nothing a minute after the
list was emptied, and `ps -o pid,ppid,sess,tty` on one that is running shows
`ppid 1`, a session of its own and no terminal. What no run can show in an
afternoon is a machine that really sleeps; that one is seen by closing the lid
with a card in hand and looking at the board an hour later.

## Images

A test image should be visible. A 1×1 pixel does arrive correctly, but nobody
can tell from it whether the display is right.
