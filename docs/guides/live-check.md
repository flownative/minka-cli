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

A request with a write method counts as a write by its address, not by its
outcome. Outside the places above it is allowed only against an identifier
that names nothing, a fresh random UUID or the nil UUID, to see a 404 or the
order of the lookups. Never against a real identifier, never a POST into a
real container, and the status names every such request.

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

## The mod

Checking the mod live means watching a tile through a tool call that runs for
longer than the three minutes a signal counts for: in a Claude Code session
with the mod, `work start`, then something that takes five minutes, and the
tile alight all through it. `work stop` empties the list, whereupon the tile
goes out at once. Then quit Claude Code with a card in hand: the tile goes out
within three minutes.

A run of `claude` for this takes a `CLAUDE_CONFIG_DIR` of its own, so that
the install and what it writes into the settings stay out of anybody's real
configuration.

## Images

A test image should be visible. A 1×1 pixel does arrive correctly, but nobody
can tell from it whether the display is right.
