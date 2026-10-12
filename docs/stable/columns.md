# Columns

A board has three **system columns** — Triage, Not now and Done — and any
number of **working columns** between Triage and Not now, in an order of
their own. `columns create`, `edit`, `move` and `delete` write them; `columns
on-hold` switches the area for cards on hold, which [the skill](../../skills/minka/references/cards.md)
describes with the cards. A column is read with its board: the API has no
address that reads one alone and none that names the board of a column.

## How a column is named

`<column>` is the ID of a column. With `--board` it is also a type (`triage`,
`not_now`, `done`) or a title, looked up on that board as `cards move --column`
looks it up; a title that is a type or another column's title is ambiguous
(exit 8). Without `--board` a column that is not an ID is a usage error before
any request. `columns move` always needs `--board` (C-102).

## What a column carries

The column resource is `id`, `type`, `title`, `customTitle`, `color`,
`cardCount`, `cards`, `onHoldEnabled`, `onHoldCardCount` and `onHoldCards`;
every write but `delete` answers it, a new column with 201 and no `Location`.
`title` is what a person reads, `customTitle` the stored title or `null` where
a system column goes by the English name of its type.

`color` is one of `white`, `red`, `orange`, `yellow`, `green`, `blue`, `aqua`,
`purple`, `gray`, `pink` and `brown` for a working column, always `green` for
Done and `null` for Triage and Not now. White is how a column without a colour
looks: `"color": null` takes a working column's colour away, and Minka answers
`white` for it, the same as for `"color": "white"`. A missing `color` is no
change.

## Writing

- **`create`** sends `{"title"}` alone: Minka puts the column behind the working
  columns and gives it the colour the board holds least of, the earliest of the
  palette among equals and never white. Neither a place nor a colour is taken
  in the same request, and the CLI sends no second one behind it, for the reason
  of [C-100](../decisions.md#c-100): a second request refused after the first
  stood would leave half of one command written.
- **`edit`** sends `--title` and `--color` as given, and `--no-color` as
  `"color": null`; at least one of them, and `--color` and `--no-color` exclude
  each other, or it exits 1 before any request. The summary calls white "white,
  which is no colour". Triage, Not now and Done take a title of their own; a
  title that is empty after trimming, `""` or `null`, gives them back the name
  of their type. A working column refuses it. The colour is written first, so
  a system column refusing a colour leaves the title as it was. The colour goes
  to Minka as typed and is refused there (C-086); the hint names the eleven.
  Triage, Not now and Done refuse `--no-color` like any colour.
- **A Minka that does not take a colour away yet** reads `"color": null` as no
  colour named: `--no-color` alone is `nothingToChange`, whose hint then names
  this cause, and beside `--title` the title is written and the colour stays,
  with a 200. The CLI therefore holds the colour of the answer against
  `white` after `--no-color` and otherwise exits 7 (`api_error`), saying the
  column kept its colour and whether the title was written, with the answer
  in `data`. `--color white` gives the same look on any Minka.
- **`move`** places a working column behind `--after`, in front of `--before`,
  or with `--first` at the front of the working columns; exactly one of the
  three. Minka refuses a neighbour on another board with
  `neighbourColumnOnAnotherBoard`, but passes over Triage, Not now, Done, the
  column itself and a column out of sight, and places the column as though
  `--first` had been given, with the same 200. The CLI therefore reads the
  board and refuses any neighbour that is no other working column of it as a
  usage error before the move (C-102); Minka's refusal is unreachable from it
  and still has a hint. A column that stands where it is asked to go already
  is no write while the project is in use.
- **`delete`** puts a working column into the trash of its project, 204, and a
  second call is 404. Its cards are not touched: they stay in it and travel
  with it, so none of the cards standing in it is at its address while it is in
  the trash, a card of
  it in the trash names `blockedBy: column` and is not restorable, and a card of
  it in the archive of the board stays readable and comes back into the first
  working column. `trash list` names the column with `kind: column` and
  `restorable: false`: only the web brings a column back.

A title is trimmed and counted in characters: 200 at most, one line without
control characters.

## Who may, and the order of the refusals

Only the team writes. Minka finds a column by its identifier alone among the
boards the token sees, and a client of the project then gets the 404 of a
column that does not exist, on every column write and before anything else is
judged: no body, field, system column, archive or write that would change
nothing is told to a client. The same holds for `columns on-hold` and for a
new column on a board the client sees. For the team, a write that fails is
answered in this order: `projectArchived`, then the code of a system column
where one applies (`systemColumnHasNoColor`, `systemColumnCannotMove`,
`systemColumnCannotBeRemoved`), and then the 404 of an address that names
nothing. A request that writes nothing — the same title again, a move to where
the column stands — answers 200 while the project is in use and
`projectArchived` in an archived one; the same colour again is a write.

Per route, in order:

| Route | Order |
|---|---|
| `POST /boards/{boardId}/columns` | board (404) · client (404) · body (415, 400) · `title` a string (400) · title (`columnTitleTooLong`, `columnTitleHasControlCharacters`, `columnTitleMissing`) · write |
| `PATCH /columns/{columnId}` | column (404) · client (404) · body · `title` and `color` strings or null (400) · `nothingToChange` when neither is named · title · `columnColorUnknown` · colour written · title written |
| `POST /columns/{columnId}/moves` | column (404) · client (404) · body · neighbours strings (400) · a neighbour that is no identifier (404) · `neighbourColumnOnAnotherBoard`, the previous neighbour first · write |
| `DELETE /columns/{columnId}` | column (404) · client (404) · write |
| `PUT /columns/{columnId}/on-hold` | column (404) · client (404) · body · `onHoldEnabled` · write |

A write that fails answers in the order of the paragraph above, so a
neighbour on another board is told before the archive and before
`systemColumnCannotMove`, and a null colour on Triage, Not now or Done in an
archived project is `projectArchived`. The CLI keys on the codes, not on their
order, and checks every ID as a UUID before a request, so the 404 of a
neighbour that is no identifier is unreachable from it.

`nothingToChange` cannot come from the CLI on a Minka that takes a colour
away with null: `edit` without a flag is a usage error here. An older Minka
answers `--no-color` alone with it, see `edit` above.
