# Search

`search <term>...` asks `GET /api/v1/search` the way Minka's search on the web
finds things (E-304, E-357). Without `--tool` or `--folder` it searches
everything the person may read: cards, comments, posts, to-do lists, to-do
groups, to-dos, folders, documents, uploads, events, decisions, the lines of
the chats of their projects and of their own private conversations. Minka never finds
drafts, comments under a draft, the trash, archived projects, the template
library or a line taken back. A client is not refused: Minka finds only what
the clients are shown.

Everything here was read in Minka's code — `SearchApiController`,
`SearchFilters`, `SearchTool`, `FullTextSearchQueryService`,
`SearchHitResource`, `ApiSearchTest` and `ApiSearchEverywhereTest` — and the
search within a tool or a folder was seen live as well.

## The term

The words of the command line are one term, joined by a space, and go out as
they are: Minka reads phrases and operators in it and cuts it at 200
characters, which a notice says where the term is longer. A term that starts
with a dash, Minka's NOT, goes behind `--`, with every flag before it.

## Narrowing

`--kind` narrows to the hits of tools, with Minka's words: `board`, `posts`,
`todos`, `files`, `calendar`, `decisions`, `chat`, `conversation`, repeated
or separated by commas. A comment counts toward the tool of what it stands under. Each word
goes out once, in the order given ([C-156](../decisions.md#c-156)).

`--project` narrows to one project and leaves the private conversations out,
which belong to none. A readable project that is archived, or the template
library, is a search without hits.

`--tool` searches the documents and uploads of a Docs & Files tool, `--folder`
those of a folder and everything beneath it, both those of a folder within a
tool. Beside them Minka finds documents and uploads alone, never folders or
comments, so `--kind` has to name `files` there
([C-156](../decisions.md#c-156)). `--project` stands beside neither: a tool or
a folder lies in one project already, and Minka answers another project with
an empty page ([C-157](../decisions.md#c-157)).

## Checked here, before any request

A term at all; each word of `--kind`, case and all, an empty one between
commas included (Minka answers `searchKindsUnknown`, exit 9); `--kind` without
`files` beside `--tool` or `--folder` (Minka would answer with nothing); a
`--project` beside them; the IDs; `--sort` as `best` or `recent` (Minka takes
any other word as `best` without saying so); `--limit` of at least 1; and
`--offset` as a whole number of at most six digits and no more than 500
([C-132](../decisions.md#c-132), [C-158](../decisions.md#c-158)). Each is exit
1. What remains for Minka is the scope itself: a project, a tool that is no
Files tool, a folder or tool that is not there or cannot be read, and a folder
outside the tool named are the plain 404, exit 2, with a hint that names the
way to a project or to a tool and a folder.

## Pages

Minka serves 25 hits a page and takes no size. `--limit` (25 unless given) is
reached with as few pages as it takes: a page is asked for only while hits are
lacking, so a limit within one page costs one request and no hit costs one of
its own. The offset counts hits, so a run that stops inside a page names the
first hit it left behind as `next` (the offset of that page and the hits taken
from it); a run that took a whole page passes Minka's `next` on. `data` is
`{hits, next}`, each hit the JSON Minka sent, `next` a string or `null`; the
breadcrumb `next` and a notice, as on the other paged commands, give the whole
command with `--offset`, keeping `--kind`, `--project`, `--tool`, `--folder`,
`--sort` and `--limit`. The term stands first, so a flag an agent appends,
`--json` above all, stays a flag; only a term starting with a dash goes last
behind `--`.

Minka serves no page beyond offset 500, the 525th hit. The run asks for none
and hands out no `next` beyond it, whatever Minka names there
([C-158](../decisions.md#c-158)): where the run would go on beyond 500, or
read a full page that ends behind it, `next` is `null` and a notice says that
minka reads no further than offset 500, that there may be more, and to narrow
the search with `--kind`, `--project`, `--tool`, `--folder` or a sharper term.

Pages are read one after another, so a hit that moves while somebody writes
can be met twice or not at all across the border. A failure on a later page
fails the run, and the hits read before it are not shown. An answer the run
cannot go on from is `unexpectedResponse`, exit 7: a hit that is no object,
`null` included, or a `next` that is no number of hits, an empty one included,
which would start over. A page without hits ends the run whatever `next` it
names.

## A hit

`kind` is `card`, `comment`, `post`, `todoList`, `todo`, `todoGroup`,
`folder`, `document`, `upload`, `event`, `decision`, `chat` or
`conversation`. `id` is the
hit itself: the comment, the message. `targetKind` and `targetId` name what the
addresses lead to: the item a comment stands under, the chat or conversation
of a line, else the item itself. `title` is the item's, for a comment the one
of its item, for a line of a chat the chat's and for a private conversation
the other person's name. `excerpt` is plain text. `section` names the heading
a document's match stands under, and `appUrl` then ends in its anchor; a
comment's ends in `#comment-<id>`, a line's in `#chat-message-<id>`. `project`
is the name of the project, `projectId`, `toolId` and `project` are `null` for
a private conversation. `shortId` and `shortUrl` are the card's, or the one of
the card a comment stands under, where its board shows short identifiers.
`updatedAt` is a comment's or message's moment of writing. `archived` marks an
item in the archive or below an archived folder or list, a comment by its
item's own archive. `apiUrl` is absolute, `null` for a to-do group, and for a
line the messages of its chat.

The text view names each hit on one line: its ID, its kind and title, a card's
short identifier as `#MNK-1`, a document's section as `› Heading`, the project
and the mark `archived`; a comment as `comment on card …`, a line of a chat as
`chat line in …`, of a private conversation as `message with …`. The excerpt
and `appUrl` follow on lines of their own ([C-160](../decisions.md#c-160)). The
breadcrumb `show` reads what the first hit with a command leads to: `cards
show`, `posts show`, `todolists show`, `todos show`, `docs folders show`,
`docs show`, `docs uploads show`, `events show` or `decisions show`, for a
comment the item it stands under. A to-do group, and a line of a chat or a conversation, have
none: the API shows no group on its own, and a transcript is read from its
newest end, so no command opens it at the line.

## An older Minka

A Minka that searches only within a Docs & Files tool or folder answers a
search without `--tool` or `--folder` with `searchScopeMissing`. The CLI makes
that exit 2 (`not_found`), saying the host searches only within a tool or
folder and to give `--tool` or `--folder` ([C-159](../decisions.md#c-159)).
The CLI stops at offset 500 there too, although such a host serves deeper
pages; the notice names the CLI's limit, not the host's. Beside a tool or
folder it passes over `--kind` and `--project` as unknown
parameters; since `--kind` must name `files` there and `--project` is refused
there, that changes no answer. Its hits carry neither `targetKind`,
`targetId`, `shortId`, `shortUrl` nor `apiUrl`; a hit without a target leads to
itself, so the breadcrumb still reads the document or upload.
