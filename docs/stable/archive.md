# The archive of a board

A board puts a card away instead of throwing it out: the card leaves its column,
stays readable at its own address and takes no more writes. `cards archive` and
`cards unarchive` move it, `boards archive` lists what a board has put away.

The archive is the one list of this CLI a card in it appears in. `boards show`
shows what stands in the columns and `cards mine` leaves it out, which is why a
card missing from either is not necessarily done.

## The page, and why its size is bounded here

`GET boards/{b}/archive` takes `before` and `limit`, answers a plain list of six
fields per card — `id`, `shortId`, `title`, `archivedAt`, `archivedBy`, `appUrl`
— newest put away first, and carries **no other sign for the end of the list**:
a caller knows the end by getting fewer cards than it asked for.

A `limit` above 100 is **cut down to 100 without a word**. The two together are
the danger of this feature: whoever asks for 500 gets a full page that reads as a
short one and takes a hundred cards for the whole archive, while the command
reports success. Therefore, by [C-054](../decisions.md#c-054):

- `--limit` outside 1 to 100 is a usage error of this CLI, refused before a
  request goes out, with the bound named. `api.Client.ArchivedCards` refuses the
  same bound again, so nothing inside the CLI can reach the silent cut either.
- A walk ends only on an answer that carried no card, never on a page of them.
  That is stricter than the address asks for — a page shorter than the one asked
  for is already the end — and it is meant so: a run pays one request at its end
  whether the last page was full or short, and it does not go wrong where the
  largest page the server serves changes under it.
- An empty answer behind a full page is no end either: it can as well be the
  cursor's card going between the server's check and its read. The page behind
  the card before the cursor settles which of the two it was. Where no cursor was
  sent, or the page before was already shorter than asked for, there is nothing
  to doubt.

## The weakness of the cursor, and what may be promised

`before` counts only while the card it names lies in the archive. Somebody who
takes that card back onto the board or into the trash between two pages makes the
next page answer 422 `archivedCardsBeforeUnknown` — or, in a narrow race between
the server's check and its read, an empty 200 that reads like the end. Minka
carries that in its own debts; it is not this CLI's to fix.

A walk therefore keeps the ids of the **whole** page it last read. On
`archivedCardsBeforeUnknown` it falls back through them, from the last card of
that page towards its first, instead of starting again at the first page. An
empty answer behind a full page is met with the same fallback rather than with
the cursor it just used: where the list has really ended, the page behind the
card before the cursor is **shorter than asked for and holds nothing the run has
not read**, and where the list went on, it holds what follows — so one request
settles it either way, where the same cursor asked again can only be refused for
a card that is gone or come back as empty as before.

Both halves of that are needed. A **full** page proves nothing however well known
its cards are: a run that has begun again read far beyond the page it is checking
against, so known cards there say nothing about what lies behind them.

That holds as long as a card of the last page is left to ask behind. Where none
is, the same cursor is asked once more and a second empty answer closes the list
**unchecked**: a card that left the archive in that moment reads exactly like the
end of the list, and cards behind it were not read. A page of one card is in that
place from the first request; a larger one gets there where the fallback has used
up the cards of the page it last read. The run exits 0 — it did what could be
done — and says in a `notice` that its end could not be checked.

A walk deduplicates by id over the whole run and counts what it swallowed. Only
when no card of the last page is left does it begin again where it started, once,
and say so in a `notice`. A walk that still cannot go on ends as `validation` and
hands over what it read as `data`, with its notices.

Nothing of a walk rests on the answers being what the address promises. A page
that ends on a card the walk has already asked behind carries it nowhere, and
asking on would be the same request for ever, so such an answer ends the run as
`api_error` with what it read.

What a reader may be promised: **every card that lay in the archive for the whole
run appears exactly once** — under the one reservation above, that the run could
check its end. What may not be promised: **completeness against a moment.** A
card put away during the run lands on the first page, which the run has passed,
and can be missed; a card taken out of the archive vanishes from the run without
a trace, and the API says nothing about either. A run of more than one page says
so in a `notice`, not in small print, and a run that could not check its end says
that as well: the general sentence does not cover it, because that is a card
which lay in the archive throughout.

## The refusals

| Answer | Command | Exit |
|---|---|---|
| 404 for the board, before either 422 | `boards archive` | 2 |
| 422 `archivedCardsLimitNotANumber` | unreachable: the bound is held here | — |
| 422 `archivedCardsBeforeUnknown` | `boards archive --before` | 9 |
| 422 `cardArchived` | `cards archive` on a card of an archive | 9 |
| 422 `cardNotArchived` | `cards unarchive` on a card of a board | 9 |
| 422 `projectArchived` | both writes, before either code above | 9 |
| 404 | both writes, for a card in the trash | 2 |
| 200 with a page that carries a run no further | `boards archive --all` | 7 |

A card in the **trash** answers 404 on both writes, where a post of an archive
answers 422. The hint therefore names the trash and `cards restore`, and does not
borrow the post's wording.

## What the answer does not say

Bringing a card back puts it at the top of the column it was taken from, or in
the first working column where that column is gone from the board, or in Triage
on a board that has none. The answer names the column. It does **not** say that
the card came back **off hold**, which it always does, so `cards unarchive` says
it in a `notice`.

A card of an archive is left out of the set an agent says it is working on, with
the word `cardArchived`, so the list of its session loses it with the next signal
by [C-053](../decisions.md#c-053). `cards archive` does not take it out itself:
one mechanism for that is enough.
