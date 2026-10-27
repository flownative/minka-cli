# Labels

## Commands

```bash
minka labels list (--project <projectId> | --board <boardId> | --tool <toolId>) --json
minka labels create <name> (--project <projectId> | --board <boardId> | --tool <toolId>) [--color <colour>] [--icon <icon>] --json
minka labels edit <label> (--project … | --board … | --tool …) [--name <name>] [--color <colour>] [--icon <icon> | --no-icon] --json
minka labels rename-scope <scope> <newScope> (--project … | --board … | --tool …) --json
minka labels move <label> (--project … | --board … | --tool …) (--first | --after <label> | --before <label>) --json
minka labels delete <label> (--project … | --board … | --tool …) --json
minka cards create --board <boardId> --title "Title" --label <label> [--label <label>...] --json
minka cards label <cardId> <label> [<label>...] --json
minka cards unlabel <cardId> <label> [<label>...] --json
minka docs label <documentId> <label> [<label>...] --json
minka docs unlabel <documentId> <label> [<label>...] --json
minka docs uploads label <uploadId> <label> [<label>...] --json
minka docs uploads unlabel <uploadId> <label> [<label>...] --json
```

## Where labels are kept

- A label belongs to a project, or to one of its boards or Docs & Files tools
  (`--tool` is the ID of a Docs & Files tool). A card takes the labels of its
  board and of its project; a document or an upload those of its tool and of
  its project.
- `labels list` under a board or a tool shows its own labels first, then the
  project's. `position` counts from 1 within each place.
- `labels edit`, `move` and `delete` reach a label only under its own place: a
  label of the project is changed with `--project`, one of a board with
  `--board`. A label given by name under the wrong place is pointed to the
  right one; one given by ID there is a plain exit 2.
- Only the team of a project reads and writes labels; a client of the project
  gets exit 2 like somebody outside it. An archived project still lists its
  labels and refuses every write with `projectArchived`.

## Names and colours

- A name holds letters, digits, spaces, `-`, `+` and `*`, at most 50
  characters, and at most one `::` between a scope and a value:
  `Priority::High`. Spaces around `::` and runs of spaces are tidied away.
- Names are unique across the whole project and all its boards and tools,
  case aside (`labelNameTaken`).
- `<label>` is an ID or a name; a text shaped like a UUID is always an ID. A name is matched case and spacing aside:
  `"priority :: high"` finds `Priority::High`.
- Colours: `red`, `orange`, `yellow`, `lime`, `green`, `teal`, `blue`,
  `indigo`, `purple`, `pink`, `brown`, `gray`, spelt exactly so. Minka refuses
  another value with exit 9 and `labelColorUnknown`. Without `--color` Minka
  picks one that stands out from the labels in reach.
- `--icon` names a Lucide icon shown in front of the label's name, such as
  `check`, `bug` or `flag` (names at https://lucide.dev/icons); aliases are
  accepted and answered as given. Minka refuses an unknown name, `""`
  included, with exit 9 and `labelIconUnknown`. `labels edit --no-icon` takes
  the icon off; an icon alone is a change. Every label in JSON carries `icon`,
  `null` for none.
- Deleting a label takes it off every item; there is no trash for labels.
- `labels rename-scope Priority Urgency --board <boardId>` renames every
  `Priority::…` label of that place (not the project's) to `Urgency::…`, all
  or none; `data` is the place's own labels afterwards. Renaming into a scope
  already in use joins the two scopes. Only a resulting name that exists
  already in the project refuses it (exit 9, `labelNameTaken`). An item that
  carried one label of each keeps both until it takes the next label of that
  scope. A scope no label of the place has exits 2.

## Labels on items

- A scoped label is exclusive within its scope: putting `Priority::Low` on a
  card that carries `Priority::High` takes `Priority::High` off. Plain labels
  never replace one another. A rename into a scope takes nothing off, so an
  item can then carry two labels of one scope.
- `cards create --label <label>` (repeatable) creates the card with its labels
  or not at all. A name is looked up among the board's and the project's
  labels first: one found nowhere exits 2, and nothing is written. Two labels of one scope exit 9 with
  `labelsShareAScope`: a new card takes one label of each scope.
- Several labels go in one request each, in the order given. The first refusal
  stops the run; `data.applied` and `data.notApplied` of the error say how far
  it got. On success `data` is every label the item carries afterwards.
- A label the item carries already, or taking off one it does not carry, is no
  refusal. A label Minka does not find among those of the item's board or tool
  and project exits 9 with `labelUnknown`. An item in the archive exits 9 with
  `cardArchived`, `documentArchived` or `uploadArchived`.
- `cards show`, `boards show`, `cards mine`, `docs show`, `docs uploads show`
  and `docs list` show the labels: a `Labels:` line on a single item, the names
  in brackets in a list; in JSON every card, document and upload carries
  `labels`.

## Finding by label

- Documents and uploads of a Docs & Files tool are filtered by a label through
  the search: `minka search --tool <toolId> "priority::high" --json`.
  `scope::value` acts as a label filter where such a scope is in reach;
  `scope::` finds any label of the scope.
- There is no filter of a board by label in the API: read the board with
  `minka boards show <boardId> --json` and look at `labels` of each card.
