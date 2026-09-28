# Posts

A Posts tool — the message board of a project — holds **posts**: a title, a
body, a kind it may be filed under, and comments. `posts tools`, `posts
categories`, `posts list` and `posts show` read; `posts create`, `edit`, `pin`,
`unpin`, `archive`, `unarchive` and `delete` write; `posts comments …` reads
and writes the comments under a post ([C-081](../decisions.md#c-081)).

Everything here was read in Minka's code — `PostsApiController`, its
resources, `PostQueryService`, `PostsCursor`, the post handlers,
`PeopleAPostTells`, `DbalPostRepository`, `DbalItemSubscriptionRepository`,
`minka_item_shown_to_clients` and `ApiPostsTest` — and none of it has been
seen live.

## What is not there

A **draft** is at no address, its author included: it is written, read and
sent out on the web. Taking a post **out of the trash**, reacting to a **post
itself** (the web offers it; the API only reacts to comments), the list of
**whom a post tells** and changing it, **showing a post to clients**, and
writing the **list of kinds** are web only as well. The trash of a project is
listed with `trash list`, and a post there says so.

## What a read hands over

`GET projects/{projectId}/posts` lists the tools in use as `id`, `title` and
`appUrl`; one no longer in use is not listed and is still read and written at
its own address. `GET projects/{projectId}/post-categories` lists the kinds of
the project as `id`, `emoji` and `name`, in the order the members put them.

`GET posts-tools/{toolId}/posts` is a page `{posts, next}`. A line carries
`id`, `title`, `lead` (the first 400 characters of the body as plain text,
trimmed), `category` (emoji and name, or null), `categoryId`, `pinned`,
`commentCount`, `createdAt`, `creator` and `appUrl`. The order is **pinned
first, the one pinned last on top**, then the newest, the identifier breaking a
tie. No draft stands in it, and nothing of the archive or the trash.

`next` is the place after the last post of the page, and **null on the last
page**: Minka reads one post more than the page holds to know. The place is the
values of that post — when it was pinned, when it was written, its identifier —
and not the post, so pinning, archiving or removing it between two pages moves
nothing. `limit` is 50 unless given, and one above 100 is cut to 100 without a
word; `next` then says there is more. So `--limit` goes out as given, as for
the done to-dos ([C-076](../decisions.md#c-076)): a page that says where it
ends cannot pass for the whole list. `limit` that is not digits or below 1 is
`postsLimitNotANumber`; `before` that is not a place of that shape is
`postsBeforeUnknown`. Both come after the tool.

`GET posts-tools/{toolId}/archive` lists the archive: `id`, `title`,
`category`, `archivedAt` and `archivedBy` (null for a post put away before
Minka noted either), `appUrl`, the one archived last first. It is not paged,
and `posts list --archived` refuses `--limit` and `--before`, which would do
nothing there.

`GET posts/{postId}` gives `id`, `projectId`, `toolId`, `project`, `tool`,
`title`, `body`, `bodySource`, `attachments`, `category`, `categoryId`,
`archived`, `pinned`, `commentCount`, `createdAt`, `editedAt`, `creator`, `comments` (oldest first) and `appUrl`. A
post of the archive is read here and says `archived: true`. **The post carries
no word about clients and no audience.**

`editedAt` is null while the post stands as it was made, and otherwise the
moment it was last written to. That is not only a change of its text: going
into the trash and coming back out of it write the same moment
(`DbalPostRepository::throwAway` and `restoreFromTrash`), so a post restored on
the web says it was edited although nobody changed it. Pinning and archiving
leave it as it is.

## Sending a post out

`POST posts-tools/{toolId}/posts` takes `title`, `body` (HTML), `categoryId`
and `audience`, and answers 201 with the post and no `Location`.

The **body** is cleaned against the files of the project like any text, and a
body without an element Minka serves is kept as the characters it consists of
([C-068](../decisions.md#c-068)), so Markdown is converted here and
`--body-format html` draws the notice. Unlike a document, **a post says
something**: a body with no word and no file — an empty one, one of blanks —
is `postBodyMissing`. A mention or a file alone is enough.

`categoryId` null or empty files it under no kind. One that is no identifier
is `postCategoryUnknown`, which the CLI never sends ([C-013](../decisions.md#c-013)).

**Whom it reaches** works as for a document, and the CLI keeps no rule of it
([C-070](../decisions.md#c-070)): `--audience` is required and one of three
words; `chosen` without a `--notify` is `notifyIdsMissing`, a `--notify` under
another audience `notifyIdsWithoutChosenAudience`, both with the hint that
names the flag. The writer is always told. `everybody` tells every member of
the project.

**A post is shown to no client.** Whether a client sees a post is the post's
own switch together with its tool's, and neither the API nor the web sets the
post's: only an import does. So a client of the project, or an agent working
for one, named with `--notify` is always `personCannotSeeIt`, nothing is
written, and repeating it changes nothing — the hint says to name somebody
else and names no place to change it, because there is none. `everybody`
refuses nobody: it puts the clients on the post's list, and they are not
notified of it (`NotificationRecipients` keeps out whom a place is not shown
to) and cannot open it.

The people are met one at a time in the order given, each asked first whether
they belong to the project and then whether the post is shown to them, so the
first name refused decides between `personNotAProjectMember` and
`personCannotSeeIt`. **A person named twice is refused as no member**: the
list writes nobody twice and reads the second naming as somebody it could not
put there. That is the same code path as for a document
(`DbalItemSubscriptionRepository::enrol` with `ON CONFLICT DO NOTHING`, called
by `PeopleAPostTells::enrolEachOf`), so the defect Minka carries for documents
holds for posts unchanged. The CLI sends `--notify` as given and the hint of
`personNotAProjectMember` names the case.

## Changing a post

`PATCH posts/{postId}` takes `title`, `body` and `categoryId`; what the body
leaves out stays, and **the change tells nobody**. Anybody in the project may
change any post that went out. There is no guard: two writers are two writes,
the last one wins. The same text again is answered 200 and changes nothing,
and `editedAt` stays.

`"title": ""` is `postTitleMissing` and `"body": ""` — and `"body": null` —
`postBodyMissing`: neither can be emptied, so `--title ""` and `--body ""` go
out and come back as exit 9. `"categoryId": null` or `""` takes the kind off;
`posts edit --no-category` sends `""`, and `--category` and `--no-category`
exclude each other. An edit without a flag is a usage error.

`posts edit --body` reads the post first, for the one thing the new body has to
be held against: whether the old one carries files or mentions that Markdown
cannot hold. The guard stands aside where Minka refuses the body whatever it
is: on a post of the archive, which the post says, and in an archived project,
which it does not say. That one is asked only where the guard is about to
refuse, with one read of the archived projects, and then the request goes out
and Minka's `projectArchived` is the answer ([C-082](../decisions.md#c-082)).

## Pinning

`POST posts/{postId}/pin` and `…/unpin` answer with the post. A pinned post
stands above every other in the list, the one pinned last on top, and there is
**no limit** to how many. Pinning a pinned post and unpinning one that is not
succeed and change nothing — the pinned one keeps its moment and its place.
Anybody in the project may pin.

## The archive and the trash

**The archive** takes a post off the list and out of every count. It is still
read with its comments, and every write on it — changing it, pinning,
unpinning, commenting, reacting, removing a comment — is `postArchived`; only
going into the trash is open. Its pin stays, so a pinned post comes back on top.
`POST posts/{postId}/archive` refuses one there already with `postArchived`,
`…/unarchive` one that is not with `postNotArchived`.

**The trash** is not the archive. A post in the trash is at no address, except
that archiving it says `postInTrash`; bringing it back from the archive is 404.
`DELETE posts/{postId}` puts a post into the trash from the list **or from the
archive**, and the archive notes go with it; it answers 204, and a second time
404. Anybody in the project may. It comes back on the web, among the posts.

**An archived project** is read like any other and refuses every write with
`projectArchived`.

## Comments

The comments under a post have addresses of their own —
`posts/{postId}/comments…` — in the shape of the comments of a card, and take
the same codes and reactions. `posts comments` names the post as the first
argument, as `comments` names a card, since one kind of item hangs there.
`api.Commented` carries the post as it carries a document and an upload.

`posts comments edit` reads the post first, for the files and mentions a
comment of one's own may carry, and where the comment carries them it reads the
person of the token to know whose it is. The guard asks only where the write
could land: a comment of somebody else, any comment under a post of the
archive and any comment in an archived project are refused by Minka whatever is
sent, so there the request goes out and Minka's answer comes
([C-082](../decisions.md#c-082)).

Minka writes a comment and its `editedAt` again whatever the text
(`DbalCommentRepository::change`), so the same text sent again answers 200 and
says it was edited now.

## Silent rewrites

Minka keeps some things otherwise than they were sent, with the same success:

- a body without served markup, kept as characters — a `notice`
  ([C-068](../decisions.md#c-068));
- a page above 100, cut — `next` says there is more;
- the title trimmed of blanks — only its form, no notice
  ([C-080](../decisions.md#c-080));
- a file of another project in the body, dropped by the cleaning — no notice,
  as for every text of this CLI;
- `everybody` putting clients on the list of a post they are not notified of
  and cannot open — said in the help, since nothing in the answer shows the
  list;
- the same text of a comment again, which moves its `editedAt`.

## The order of the refusals

Minka checks in this order, and each step comes only where the one before it
passed. A body that is no JSON object (400) comes first wherever one is read,
and is unreachable from here.

- **A new post:** the tool (404), the title, the body (`postBodyTooLong`,
  `postBodyNotReadable`, `postBodyMissing`), a `categoryId` that is no
  identifier, the audience (`postAudienceMissing`, unreachable), a `notifyIds`
  entry that is no identifier (`personNotAProjectMember`, unreachable), then
  `notifyIdsMissing` / `notifyIdsWithoutChosenAudience`, then
  `projectArchived`, then `postCategoryUnknown` for a kind the project does not
  offer, then the people one at a time.
- **An edit:** the post (404), the title, the body, a `categoryId` that is no
  identifier, then `projectArchived`, `postArchived`, `postCategoryUnknown`.
- **Pin and unpin:** the post (404), `projectArchived`, `postArchived`.
- **Archive:** a post in the trash (`postInTrash`) or none (404), then
  `projectArchived`, then `postArchived`.
- **Unarchive:** the post (404, also for one in the trash), `projectArchived`,
  `postNotArchived`.
- **Delete:** the post (404), `projectArchived`.
- **A page:** the tool (404), `postsLimitNotANumber`, `postsBeforeUnknown`.
- **A comment:** the post (404), a comment that is no identifier (404), the
  text, then `projectArchived` and `postArchived`, then 404 for a comment that
  is not one's own. A reaction: the post, the comment identifier, the reaction,
  the comment (404), then `projectArchived` and `postArchived`.

The CLI changes this order where it reads or refuses before it writes. An ID
that is no UUID — of a post, a tool, a kind, a comment, a person — is exit 1
without a request ([C-013](../decisions.md#c-013)). `--notify me` reads the
person of the token. `posts edit --body` and `posts comments edit` read the
post, which is 404 first as Minka's own order has it; `posts comments edit`
reads the person of the token as well where the comment carries files or
mentions, and both read the archived projects where their guard is about to
refuse. After the reads the guard comes before the title and the body on an
edit, and before the text of a comment; it never comes before
`projectArchived` or `postArchived`.

| Answer | Command | Exit |
|---|---|---|
| 404 | every command, for a tool or post that is unknown, foreign, a draft, in the trash or hidden from a client; a comment that is not one's own | 2 |
| 422 `postTitleMissing`, `postTitleTooLong`, `postTitleHasControlCharacters` | `posts create`, `posts edit` | 9 |
| 422 `postBodyMissing`, `postBodyTooLong`, `postBodyNotReadable` | `posts create`, `posts edit` | 9 |
| 422 `postCategoryUnknown` | `--category` naming a kind the project does not offer | 9 |
| 422 `notifyIdsMissing`, `notifyIdsWithoutChosenAudience` | `posts create --audience` and `--notify` that do not go together | 9 |
| 422 `personNotAProjectMember` | `posts create --notify` naming somebody outside the project, or somebody other than the writer twice | 9 |
| 422 `personCannotSeeIt` | `posts create --notify` naming a client or an agent of one | 9 |
| 422 `postArchived` | every write but delete on a post of the archive, comments included | 9 |
| 422 `postNotArchived` | `posts unarchive` of a post among the posts | 9 |
| 422 `postInTrash` | `posts archive` of a post in the trash | 9 |
| 422 `postsLimitNotANumber`, `postsBeforeUnknown` | `posts list --limit`, `--before` | 9 |
| 422 `projectArchived` | every write | 9 |
| 422 the comment and reaction codes | `posts comments …`, as under a card | 9 |
| 422 `postAudienceMissing`, 400 `malformedBody`, 415 | unreachable: the audience is held here and every write sends JSON | — |
