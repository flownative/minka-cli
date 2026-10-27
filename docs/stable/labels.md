# Labels

A **label** belongs to a project, or to one of its boards or Docs & Files
tools. A card carries labels of its board and of its project, a document or an
upload labels of its tool and of its project. `labels list`, `create`, `edit`,
`rename-scope`, `move` and `delete` keep the labels of a place;
`cards create --label` gives a new card its labels; `cards label` and
`unlabel`, `docs label` and `unlabel`, `docs uploads label` and `unlabel` put
them on items and take them off. [The skill](../../skills/minka/references/labels.md)
describes them for agents.

## The place

Every `labels` command takes exactly one of `--project`, `--board` and
`--tool` (a Docs & Files tool, which is Minka's Files tool), each checked as an
ID before any request; none or two exit 1. The API reaches a label only under
its own place: a board's label under `/projects/…` is 404, and the other way
round. `labels list` under a board or a tool answers its own labels first and
then the project's, each group counted from 1 in `position`.

## What a label carries

`id`, `name`, `scope`, `value`, `color`, `icon`, `projectId`, `toolId` and
`position`, the same in a list, in the answer of a write and on an item.
`name` is the tidied name in the case it was written in; `scope` is the part
before `::` and `null` for a plain label, `value` the part after it or the
whole name. `toolId` is `null` for a label of the project. `icon` is the name
of a Lucide icon shown in front of the name, `null` for none; a stored name the
Lucide set of the server no longer has reads as `null`.

A name holds letters, combining marks, decimal digits, spaces, `-`, `+` and
`*`, at most 50 characters, with at most one `::`. Minka tidies it first:
composed, runs of white space as one space, no space around `::`, trimmed. It
compares names case-folded across the whole project and all its tools, so a
name taken anywhere in the project is `labelNameTaken`. The CLI sends a name
as typed and leaves judging it to Minka.

The twelve colours are `red`, `orange`, `yellow`, `lime`, `green`, `teal`,
`blue`, `indigo`, `purple`, `pink`, `brown` and `gray`, spelt exactly so.
`--color` goes to Minka as typed and Minka is the one to refuse it (C-086):
`labelColorUnknown`, exit 9, with a hint naming the twelve. Without `--color`,
`create` sends no colour and Minka suggests one.

## Icons

An icon is the name of a Lucide icon, such as `check`, `bug` or `flag`; the
names are at https://lucide.dev/icons. Lucide's aliases are icons too
(`check-circle` beside `circle-check`), and Minka answers with the name as it
was given. `--icon` goes to Minka as typed, `""` included, and Minka is the one
to refuse it: `labelIconUnknown`, exit 9, with a hint pointing to Lucide's
names (C-143). The text view of `labels list` shows the icon in its own
column, `-` for none; the other text views show labels by name only, and JSON
carries `icon` everywhere.

## Naming a label

`<label>`, and the neighbour of `move`, is an ID or a name; a text in the
shape of a UUID is always taken as an ID. An ID is sent as it is, lowercased,
and nothing is read: under the wrong place it is Minka's plain 404. A name is
looked up in `GET …/labels` of the place, compared the way Minka compares
names, among the place's own labels only. A name that belongs to the project
under a board exits 2 and points to `--project`, or, as the neighbour of a
move, says that a neighbour is a label of the same place; a name found nowhere
exits 2 and points to `labels list` (C-138).

## Writing a place's labels

- **`create`** sends `{"name"}`, and `"color"` and `"icon"` where given; 201
  with the label at the end of its place, without an icon where none is given.
- **`edit`** sends `--name`, `--color` and `--icon` as given and `--no-icon` as
  `"icon": null`, which takes the icon off; at least one, or it exits 1 before
  any request, and an icon alone is a change. `--icon` and `--no-icon`
  together exit 1 before any request. A rename that changes only the case is
  allowed. A rename into a scope takes nothing off the items.
- **`rename-scope <scope> <newScope>`** sends `{"scope", "newScope"}` as
  typed to `POST P/labels/scope-renames`, with nothing read first (C-142).
  Every label of `scope` among the place's own labels takes `newScope`, in
  the case it is written in; scopes compare case aside, so a rename into the
  same scope changes only its case. Colour, position and the items that carry
  the labels stay; the project's labels are not renamed under a board or a
  tool. All or none: a resulting name over 50 characters is
  `labelNameTooLong`, an empty scope `labelScopeMissing`, one with `::` or
  another character a name refuses `labelScopeMalformed`, a scope no label of
  the place has 404 (exit 2). The answer is the place's own labels afterwards,
  without the project's, and the text view lists them.

  A rename into a scope already in use **joins** it: `cull::select` renamed to
  `auswahl` stands beside an existing `auswahl::keep`. Only a resulting full
  name that exists already anywhere in the project, compared case-folded,
  refuses the rename with `labelNameTaken`, and then nothing is renamed. An
  item that carried a label of each scope keeps both until it takes the next
  label of that scope, which replaces them.
- **`move`** takes exactly one of `--first`, `--after` and `--before`. It sends
  `{}` for `--first`, `{"previousLabelId"}` for `--after` and
  `{"nextLabelId"}` for `--before`, never both: Minka places by the previous
  label alone where both are sent. A neighbour that is the label itself exits 1
  before anything is written; one of another place is Minka's `labelPlaceUnknown`. The
  answer is the moved label with its new position.
- **`delete`** deletes the label and takes it off every item; 204, there is no
  trash, and a second call is 404.

## Labels on items

A label given as an ID goes as `{"labelId"}`, anything else as `{"name"}`, and
`unlabel` uses `DELETE …/labels/{labelId}` or `DELETE …/labels?name=`; the CLI
never sends both, and refuses an empty name with exit 1 (C-139). Minka looks a
name up among the item's own board or tool and its project. A card is also
named by its short identifier.

Several labels go in one request each, in the order given, and the first
refusal stops the run: the error names the label, carries `data.applied` and
`data.notApplied`, and nothing behind it is sent (C-140). On success `data` is
every label the item carries afterwards — the answer of the last `POST`, and
for `unlabel` a read of the item. Where that read fails after every label came
off, the run fails with the read's code, and `data.applied` names every label
and `data.notApplied` none.

A scoped label is exclusive in its scope: putting one on takes off any other
label of the same scope, case-folded, in the same write; plain labels never
displace. A label the item carries already answers 200 instead of 201, and a
label taken off that it does not carry is a 204 as well.

## Labels on a new card

`cards create --label <label>` (repeatable) creates the card with its labels in
one request, or not at all. The API takes only IDs, as `labelIds`. A label in
the shape of a UUID is sent lowercased; a name is looked up in
`GET /boards/{id}/labels`, the board's own labels and the project's, compared
the way Minka compares names. The list is read once, and only when a name is
given; without `--label` no `labelIds` is sent and nothing is read. A name
found nowhere exits 2 and an empty one exits 1, each before anything is
written. A label named twice is sent once (C-141).

Two labels of one scope are left to Minka: `labelsShareAScope`, exit 9, with a
hint that a card takes one label of each scope. A label ID of another board, a
Docs & Files tool or another project is Minka's `labelUnknown`. A client of the
project who sends labels gets 404.

## Where labels show

Minka hands `labels` out on every card answer — `cards show`, the card of a
write, `cards mine` — on the cards of `boards show`, on documents and uploads
and on their lines in `docs list` and the folder answers. It leaves them out of
search hits, comments, the archive of a board, books and trees, the Files
archive and the trash. JSON passes them through. The text views give a
`Labels:` line on a single card, document or upload, and the names in brackets
behind the title in the lists of `boards show`, `cards mine` and `docs list`.

Finding by label is the search of a tool: `minka search --tool <toolId>
"scope::value"`. The API has no filter of a board by label.

## Who may, and the order of the refusals

Only the team of a project reads and writes labels; a client of the project
gets 404 at every label address, the same as somebody outside it. Labels on
items are shown to whoever sees the item.

| Route | Order |
|---|---|
| `POST /boards/{id}/cards` with `labelIds` | board (404) · body · title · `dueOn` · `assigneeIds` (400, malformed ID `personNotAProjectMember`) · `labelIds` not a list of strings (400) · a client (404, only where labels are sent) · `labelUnknown`, compared by exact ID · description · column (`projectArchived`, 404) · `personNotAProjectMember` · `labelsShareAScope`, a label named twice counting once |
| `GET P/labels` | place (404, client 404) · list; an archived project reads |
| `POST P/labels` | place · body (415) · `name` (400, `labelNameMissing`, `labelNameTooLong`, `labelNameMalformed`) · `color` (400, `labelColorUnknown`) · `icon` (400, `labelIconUnknown`) · `labelNameTaken` · `projectArchived` |
| `PATCH P/labels/{id}` | place · label of this place (404) · body (415, 400 for any field) · `nothingToChange` · name · colour · `labelIconUnknown` · `labelNameTaken` · `projectArchived` |
| `POST P/labels/scope-renames` | place · body · `scope`, then `newScope` (`labelScopeMissing`, `labelNameTooLong`, `labelScopeMalformed`) · `projectArchived`, once · no label of the scope (404) · label by label in the place's order: `labelNameTooLong` · `labelNameTaken` |
| `POST P/labels/{id}/moves` | place · label · body · `labelPlaceUnknown`, previous first · `projectArchived`, a move that changes nothing included |
| `DELETE P/labels/{id}` | place · label · `projectArchived` |
| `POST I/labels` | item (404: draft, trash, client) · body · `labelAmbiguous` · `labelMissing` · `labelUnknown` · `projectArchived` · `cardArchived`, `documentArchived`, `uploadArchived` |
| `DELETE I/labels[/{id}][?name=]` | item · `labelAmbiguous` · malformed ID (404) · `labelMissing` · `labelUnknown` · `projectArchived` · archived item |

P is `/projects/{id}`, `/boards/{id}` or `/files-tools/{id}`; I is
`/cards/{id}`, `/documents/{id}` or `/files-uploads/{id}`. A malformed name on
an item is `labelUnknown`, never `labelNameMalformed`.
