# Posts

A Posts tool — the message board — holds **posts**: a title, a body, a kind
it may be filed under, and comments. `posts …` reads and writes them;
`posts comments …` the comments under one.

## Commands

```bash
minka posts tools <projectId> --json
minka posts tool <toolId> --json
minka posts categories <projectId> --json
minka posts list <toolId> [--limit <n>] [--before <next>] --json
minka posts list <toolId> --archived --json
minka posts show <postId> --json
minka posts create --tool <toolId> --title "Title" --body - --audience everybody|chosen|nobody [--notify <person>]... [--category <categoryId>] --json < post.md
minka posts edit <postId> [--title "New"] [--body - < post.md] [--category <categoryId> | --no-category] --json
minka posts pin <postId> --json
minka posts unpin <postId> --json
minka posts archive <postId> --json
minka posts unarchive <postId> --json
minka posts delete <postId> --json
minka posts comments list <postId> --json
minka posts comments create <postId> - --json < comment.md
minka posts comments edit <postId> <commentId> - [--format html] --json < comment.md
minka posts comments delete <postId> <commentId> --json
minka posts comments react <postId> <commentId> 👍 --json
minka posts comments unreact <postId> <commentId> 👍 --json
```

## Reading

- `posts tools <projectId>` lists the Posts tools in use, `posts tool
  <toolId>` one tool by its ID alone; `posts categories
  <projectId>` the kinds a post can be filed under, which every tool of the
  project shares. An emoji is part of a kind's name ("📢 Announcement"), and a
  post's `category` is that name; the `emoji` beside it is deprecated and
  always empty — do not read it.
- `posts list <toolId>` lists the posts that went out: **pinned ones first**,
  the one pinned last on top, then the newest. It is a page, `{posts, next}`:
  `next` is null on the last page, and otherwise goes to `--before` exactly as
  it came. `--limit` is 50 unless given; Minka cuts a page above 100, which
  `next` then says. No draft is listed, your own included, and nothing of the
  archive or the trash.
- `posts list <toolId> --archived` lists the archive, the one archived last
  first, with `archivedAt` and `archivedBy`. It is not paged.
- `posts show <postId>` gives the post with `body`, `bodySource`, its files,
  `category`, `pinned`, `archived` and its comments. **A draft and a post in the
  trash are at no address**, their author included: exit 2.

## Writing

- `posts create --tool <toolId> --title "..." --body - --audience ...` sends a
  post out; there are no drafts here. The body is Markdown, converted here, and
  **a post says something**: a body without a word and without a file is exit 9
  `postBodyMissing`. `--category <categoryId>` files it under a kind.
- `--audience` and `--notify` work as for a document: `everybody`, `chosen`
  with one `--notify <person>` per person, or `nobody`; the hint of
  `notifyIdsMissing` and `notifyIdsWithoutChosenAudience` says which flag to
  change. A person named more than once is told once.
- **A post written here is shown to no client of the project.** Naming a client,
  or an agent working for one, is always exit 9 `personCannotSeeIt`: nothing
  was written, the same request is refused again, and the only way on is to
  name somebody else. `everybody` refuses nobody; it puts the clients on the
  post's list, but they are not notified and cannot open it.
- `posts edit <postId>` changes the title, the body, the kind or several;
  what is not given stays, and the change tells nobody. Anybody in the project
  may change any post. `--title ""` is exit 9 `postTitleMissing` and `--body ""`
  exit 9 `postBodyMissing`: neither can be emptied. `--no-category` takes the
  kind off. A post with files or mentions is not replaced by a body without
  them unless `--drop-attachments` is given.
- `posts pin` puts a post on top of its list; there is no limit, and pinning a
  pinned post (or unpinning one that is not) succeeds and moves nothing.
- `posts archive` takes a post off the list into the archive: readable, and
  every write on it — comments included — is exit 9 `postArchived` until
  `posts unarchive`. A pinned post keeps its pin there and is on top again when
  it comes back. Archiving a post in the trash is exit 9 `postInTrash`.
- `posts delete` puts a post into the trash, from the list or from the archive;
  anybody in the project may. It comes back on the web only.
- `posts comments create|edit|delete|react|unreact <postId> …` work as under a
  card. Only the author changes or removes a comment; somebody else's is exit 2.

## The order of the refusals

For a new post: exit 2 for a tool that is not there **before** anything else,
then the title, the body, the audience and the people it names, then
`projectArchived`, then `postCategoryUnknown`, then the people one at a time
(`personNotAProjectMember`, `personCannotSeeIt`). For an edit: exit 2, then the
title and the body, then `projectArchived`, `postArchived` and
`postCategoryUnknown`. For pin, unpin, a comment and archiving:
`projectArchived` before `postArchived` — except that archiving a post in the
trash says `postInTrash` first. `posts edit --body` and `posts comments edit`
read the post first; `posts comments edit` reads you (`me`) as well where the
comment carries files or mentions, and both read the archived projects where
their guard over files is about to refuse. That guard never comes before
`projectArchived` or `postArchived`.

## Codes of this part

`postTitleMissing`, `postTitleTooLong`, `postTitleHasControlCharacters`,
`postBodyMissing`, `postBodyTooLong`, `postBodyNotReadable`,
`postCategoryUnknown`, `notifyIdsMissing`, `notifyIdsWithoutChosenAudience`,
`personNotAProjectMember`, `personCannotSeeIt` (a person to replace, not input
to fix), `postArchived`, `postNotArchived`, `postInTrash`,
`postsLimitNotANumber`, `postsBeforeUnknown`, `projectArchived` and the comment
and reaction codes a card knows, all exit 9.
