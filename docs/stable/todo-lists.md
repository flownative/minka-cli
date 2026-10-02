# To-do lists and groups

A To-dos tool holds lists, the newest on top. A **list** has a title, a
description and **groups**, named headings with an order of their own. A to-do
stands in its list directly or in one group of it.

`todolists tools`, `todolists list` and `todolists show` read; `todolists
create`, `edit`, `archive`, `unarchive` and `delete` write a list; `todogroups
create`, `edit`, `move` and `delete` write a group; `todolists comments …`
reads and writes the comments under a list, as [To-dos and subtasks](todos.md)
says for both. The to-dos themselves and their subtasks have commands of their
own, under `todos`, which is why the lists and the groups are not called that
([C-071](../decisions.md#c-071)); they follow [To-dos and subtasks](todos.md).

## What a read hands over

`GET todos-tools/{toolId}/lists` and `…/archive` answer the same line per list:
`id`, `title`, the whole stored `description` (not a lead), `archived`,
`openCount`, `completedCount` and `appUrl`. Nothing is paged and nothing is cut,
so a short list is the whole list. `todolists list --archived` asks the second
address: the two are separate lists, and a list stands in exactly one of them.
The order of the page is the position — a new list goes on top — and the archive
is ordered by the moment of archiving, the last first. A list brought back takes
the place it had.

`GET todo-lists/{listId}` answers a list in use or in the archive with its
`todos` — the **open** to-dos standing in no group — and its `groups` in their
order, each with its `listId` and its open `todos`. A to-do of a group in the
trash is in neither and in no count. **Done to-dos are counted and never
listed**: Minka reads them a page at a time at an address of their own, which
`todos completed` asks.

`GET projects/{projectId}/todos` names the tools that are **in use**. A tool
that is no longer in use is not named there and is still read, and written, at
its own address.

A group has **no address to read it at**. Every write on a group answers the
group (`id`, `listId`, `title`, `color`, `todos`), and `todolists show` is where
its ID and its place come from.

## The colour of a group

A group carries one of the colours of a board column, or none: `color` is
always in the answer, `null` where there is none, and the text of `todolists
show` names a colour in the heading of its group only where there is one.
`todogroups create` and `todogroups edit` take it as `--color`; `edit` needs
`--title`, `--color` or both, and Minka writes what the body names together or
not at all, so a refused colour leaves a new title unwritten as well.

The names are Minka's: `white`, `red`, `orange`, `yellow`, `green`, `blue`,
`aqua`, `purple`, `gray`, `pink` and `brown`, as its API controller and its
importer read them, the same as Basecamp's. **`white` is no colour**: a group
given white is stored and answered with `null`, which is why `--color white`
is how a colour is taken away and there is no `--color none`. The API would
take `"color": null` as well, which the CLI never sends.

The CLI sends what is typed and refuses nothing itself (C-086): a name in
capitals, an unknown one and the empty one given as `--color ""` go to Minka,
which answers 422 `todoGroupColorUnknown`, and the hint names the eleven. The
list or the group is looked up first, so a bad colour aimed at one that is not
there is 404.

## The description, and the refusal that never comes

The description of a list goes through the same cleaning as the body of a
document: a value without an element the server serves is escaped into one
`<div>` and kept as its characters, with a 201 or 200 like any other. So
`--description` is Markdown converted here, and `--description-format html`
draws the `notice` of [C-068](../decisions.md#c-068) where the text carries no
served markup. An empty description empties it; a description left out stays.

`"title": ""` is `todoListTitleMissing`, as for a document: `todolists edit
--title ""` reaches the server and comes back as exit 9. A body that names
neither field is `nothingToChange`, which the CLI never sends: an edit without a
flag is a usage error here.

A description that carries files or mentions is not replaced by one that does
not, unless `--drop-attachments` is given, as for a card. For a list of the
archive and in an archived project that guard stands aside, and Minka's
`todoListArchived` or `projectArchived` is the answer
([C-082](../decisions.md#c-082)).

## The archive of a list

A list in the archive is read at its own address and takes **no write** but the
two that end its stay: `todolists unarchive` and `todolists delete`, which
trashes it from the archive as well. Everything else — a new title, a new
description, a new group, archiving it once more — answers 422
`todoListArchived`, and the hint names the `unarchive` to run with the list's
own ID.

**A group of an archived list is at no address.** Minka looks a group up only
in a list in use, so renaming, moving or trashing one answers 404, exactly as
for a group that never existed, until its list is back. The CLI cannot tell the
two apart and does not try; the hint of every 404 of `todogroups` names the
archive beside the trash.

## Moving a group

`POST todo-groups/{groupId}/moves` takes `previousGroupId` and `nextGroupId`.
Minka puts the group behind the first where it is named, else in front of the
second, and **at the top of the list where neither is named**. So a body
without a neighbour is a move to the top, and the CLI asks for that in words:
exactly one of `--top`, `--after <groupId>` and `--before <groupId>`, and none
or two are a usage error before a request goes out
([C-072](../decisions.md#c-072)). Given both neighbours, Minka checks both and
places the group by the previous one alone, so a second placement would say
something that is not done. There is no `--bottom`: the address offers
none, and finding the last group would need the list, which the ID of a group
does not name.

A neighbour has to be **another** group of the **same** list, out of the trash.
Anything else, the group itself included, is 422 `todoPlaceUnknown` from Minka,
and nothing moves; in an archived project it is `projectArchived` instead. A
neighbour that is no identifier at all is `todoPlaceUnknown` before the
archive is asked, which the CLI never sends: `--after` and `--before` take an
ID.

## The refusals

The writes check in the controller's order: whether the tool, the list or the
group is there first, then the body and the title, then the colour of a group
or the description of a list, and only after the statement refused why it
did — the project, then the archive of the list.

| Answer | Command | Exit |
|---|---|---|
| 404 | every read and write, for what is unknown, foreign, in the trash, hidden from a client, and every `todogroups` command on a group of an archived list; before every 422 | 2 |
| 422 `todoListTitleMissing`, `todoListTitleTooLong`, `todoListTitleHasControlCharacters` | `todolists create`, `todolists edit` | 9 |
| 422 `todoListDescriptionTooLong`, `todoListDescriptionNotReadable` | `todolists create`, `todolists edit` | 9 |
| 422 `todoGroupTitleMissing`, `todoGroupTitleTooLong`, `todoGroupTitleHasControlCharacters` | `todogroups create`, `todogroups edit` | 9 |
| 422 `todoGroupColorUnknown` | `todogroups create`, `todogroups edit`, after the title | 9 |
| 422 `todoListArchived` | every write on an archived list but `unarchive` and `delete`, `todogroups create` on one included | 9 |
| 422 `todoListNotArchived` | `todolists unarchive` on a list in use | 9 |
| 422 `todoPlaceUnknown` | `todogroups move` next to a group of another list or one in the trash | 9 |
| 422 `projectArchived` | every write, before `todoListArchived`, `todoListNotArchived` and `todoPlaceUnknown` | 9 |
| 422 `nothingToChange` | unreachable: an edit without a change is a usage error here | — |
| 415 `unsupportedMediaType`, 400 `malformedBody` | unreachable: every write of this CLI sends JSON with the fields as strings | — |
