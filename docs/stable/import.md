# Importing from Basecamp

`minka import basecamp` is an extractor: it walks a Basecamp account and writes
a bundle in Minka's import format. The format is Minka's
(`Documentation/stable/02-architecture/import-format.md` there) and the only
thing the two sides share; Minka's applier reads the bundle, the CLI never
writes history into Minka itself.

```
minka import basecamp --project <id> [--project <id> …] --out <dir> [--dry-run]
```

The command talks to Basecamp only, and only reads there. It needs no Minka
host and no token.

## Through the Basecamp CLI

The extractor does not speak the Basecamp API itself. It runs
`basecamp api get <path> --agent` for every request, and
`basecamp files download` for every file of a text or a chat, see Files
(C-023): the Basecamp CLI brings the sign-in, the refreshing of the token,
rate limiting and retries, and no Basecamp token ever passes through minka.
The Basecamp CLI has to be installed and signed in.

- The path stands behind `--` and `BASECAMP_NONINTERACTIVE=1` is set: nothing
  in a path is read as a flag, and a question would hang a run nobody watches.
- The Basecamp CLI answers one page per call and does not say whether another
  follows. Lists are read with `?page=1, 2, …` until a page comes back empty; a
  page that repeats the one before it ends the run instead of looping. A list
  that grows by exactly a page between two requests looks the same and ends
  the run as well, and the message says both. A page is read into its records
  when it arrives and let go of; of the page before it a digest is kept.
- Only paths of the account reach the Basecamp CLI. Where an answer names an
  absolute address, as a dock entry does, it has to lie under the address of
  the account and is turned into a path; anything else is refused, because the
  Basecamp CLI signs every request it makes.
- A project is named by its numeric ID of at most 18 digits. Anything else is
  a usage error before a request goes out and before a directory is made.
- The address of a project has to have the shape
  `https://<host>/<account>/projects/<id>.json`, and all projects of a run the
  same account: the account names the bundle, and its address is what every
  other address is held against.
- A run that is stopped is `interrupted`: when the context is done, or when
  SIGINT or SIGTERM ended the Basecamp CLI, which the signal may reach first.
  A Basecamp CLI that any other signal ended, such as the kill of a system out
  of memory, is an `api_error` that names the signal.
- A request that fails for a reason that may pass is asked again before the
  run gives up (C-029): at most four times (`MaxAttempts`), after pauses of 2,
  10 and 30 seconds (`RetryPauses`). Such a reason is a failure of the
  network, the rate limit, and a failure of the API that may pass, see below.
  Every other failure ends the run at the first answer: not found, not signed
  in, forbidden, a request Basecamp refuses, an answer that is no JSON, a
  Basecamp CLI that crashed or that a signal ended. A run that is stopped
  during a pause ends at once as `interrupted`, without a further request.
  When the attempts are used up the run ends with the last failure, which is
  `retryable`. A run asks again at most 100 times (`MaxRetriesOfARun`): a
  source that fails now and then stays far below that, and one where every
  request needs its pauses would stretch a run by hours. The next failure
  that may pass then ends the run with its class, `retryable`, and a message
  that says Basecamp kept failing. Every request of a run goes through the
  one source that asks again (`Retrying`), the dry run included; the download
  of a file does not, see Files that cannot be fetched. `requestsRetried` in
  the report counts the requests that were asked again, each further attempt
  counted.

Its failures keep their class:

| Basecamp CLI | `code` | Exit |
|---|---|---|
| not installed | `config` | 1 |
| not found (2) | `not_found` | 2 |
| not signed in (3) | `auth_required` | 3 |
| forbidden (4) | `forbidden` | 4 |
| rate limit (5) | `api_error`, retryable | 7 |
| network (6) | `network`, retryable | 6 |
| API failure (7) that may pass | `api_error`, retryable | 7 |
| anything else | `api_error` | 7 |

The failure object of the Basecamp CLI is `{ok, error, code, retryable, hint,
meta: {request_id}}` and names no HTTP status. A failure of the API (exit 7,
`api_error`) may pass when the Basecamp CLI says `retryable: true`, which it
does for a gateway error (502, 503, 504), or when its message is, as a whole,
one of those the Basecamp CLI makes up around a status, `Request failed (HTTP
<status>)`, `Server error (<status>)` or `Gateway error (<status>)`, and the
status is 5xx, 408 or 429. Its own word is not enough: it calls a status it
does not know final, such as the 525 of a gateway that lost its connection to
Basecamp, and it does not try such a request again itself. A message Basecamp
sent instead names no status, and a text that merely quotes one proves
nothing; both stay final.

The message of a failure is what the Basecamp CLI says, as of every failure
of the API. Only a message of one of the fixed forms is known to hold a
status and nothing else. One that comes with `retryable: true` may be text
the server sent, which the Basecamp CLI cuts to 500 bytes, and minka passes
it on as it does for a failure that stays.

The exit code alone is no class: a program that crashes exits 2 as well. A
class counts only when the failure object `{ok: false, …}` stands on stdout
and its `code`, where it has one, says the same. Everything else is an
`api_error` that carries what the Basecamp CLI printed on stderr, cut to 500
bytes. Stdout is never quoted, only measured: it may hold the data of a
request that failed halfway, names and addresses included.

A bundle that cannot be written is `config`; an `--out` that holds something
else is `usage`.

What Basecamp says and a bundle cannot carry ends the run as `api_error`
instead of being passed on: a status of a project, a card, a post or a
comment other than `active`, `archived` or `trashed`, which a draft would be,
a line of a chat that is not `active`, because a message of a bundle has no
status, a record without `created_at` or `updated_at`, a time that is none
where records are ordered by it, a kind of column a bundle has no role for, a
kind of line a bundle does not know, a second Triage, Not now or Done on one
board, a boost that does not say whose it is. A card, a post and a comment
are held to this where the walk first meets them, before anything that hangs
on them is asked for, and the lines of a chat before the first of them is
written. Such a message names the record by its ID and what is missing. Of
what Basecamp said it quotes the status or the kind of column or line a
bundle does not know, and never a title, a name, a text or an address.

## What the Basecamp identity needs

The run sees what the identity sees that the Basecamp CLI is signed in as. The
same holds for an unattended job, which needs such an identity of its own.

- It may see the projects named. One it may not see is `not_found`.
- It may see the e-mail addresses of other people. Basecamp masks them with
  `•` (U+2022) for an identity that may not, which is anybody who is neither of
  the account's own company nor an admin; only its own address comes through.
  A bundle with masks in it would make people nobody can sign in as.

The people list of the account is read first and checked as a whole, before a
project is asked for, a file is fetched or anything is written, in a dry run
as well, and whoever of it comes into the bundle later:

- One masked address, anywhere in it, ends the run as `forbidden`, exit 4, and
  the hint names what the identity lacks. The same holds for the creator of a
  record who is not in the list.
- An address looks like one when it is something without whitespace, one `@`,
  and a domain of at least two labels made of letters of any script, digits
  and hyphens, none of them empty. Whitespace around it is taken away first.
- An address that is none without a mask is a flaw in the data of the source,
  which only its owner can repair there, and must not cost the import of an
  account. The person comes with `email: null`, which makes a person without a
  login; the address is not repaired. The report names them under
  `invalidAddresses`, by `sourceId` only and in the order of the numbers, a
  `notice` says how many, and the run exits 0: the bundle is whole and a
  repeat would change nothing. The same holds for a creator.
- When more than half of the addresses of the list are none
  (`sourceProblemShare`), that is no flaw in the data any more but an identity
  or a source that hides addresses, with another glyph than `•` for instance,
  and the account would come through as people without logins. The run ends
  as `api_error` with a hint to look at what the identity may see.

A message counts the addresses concerned out of those given. No output ever
names an address or whose it is, and `invalidAddresses` holds IDs, never names.

## What a bundle holds so far

The structure, the boards, the message boards, the chats and the check-ins:
`manifest.json`, `people.jsonl`, `projects.jsonl`, `tools.jsonl`,
`categories.jsonl`, `columns.jsonl`, `cards.jsonl`, `steps.jsonl`,
`posts.jsonl`, `questions.jsonl`, `answers.jsonl`, `messages.jsonl` or its
parts, `comments.jsonl`, `reactions.jsonl`, `attachments.jsonl` and `files/`
with the avatars and the files of texts, chats and answers. A file of a kind
the projects have nothing of is missing.

The walk goes kind by kind across the projects of a run (C-028): the projects
with their tools and boards, then the message boards of all projects, then
the chats, then the questionnaires. So comments and reactions stand in the
order of what they hang on: those of cards, those of posts, those of lines,
those of answers.

- **People** are the ones a record of the bundle refers to, and nobody else:
  the members of the projects named and whoever a record names: its creator,
  an assignee, who completed a card or a step, who gave a boost, who is
  mentioned in a text. The people list of an account is what somebody is
  looked up in, not what is written: most of an account has no place where
  only some projects move to.
  `peopleLeftOut` in the report counts them; `counts.people` is who was
  written. The first number depends on what the list holds for the identity
  that asks, which is more for an admin than for anybody else; the people
  written do not. Referring to a person is the one way into the bundle, so every
  reference leads to a person who is written. People are written at the end
  of the run, when it is known whom the records refer to.
  What the account's list says about somebody wins over what a record says;
  somebody the list does not hold comes as the record shows them. An empty
  e-mail address becomes `null`, and so does one that is none, see above.
  `retired` follows the kind of person Basecamp names (`personable_type`),
  not the list: a `User`, and a `Client`, comes with `retired: false`, and
  every other kind with `retired: true`, a bot (`Integration`), what is left
  of somebody who was deleted (`Tombstone`), a `DummyUser`, an `Outsider` and
  a kind nobody knows yet, wherever somebody comes from. The list says
  nothing here, because it depends on who asks: for an admin it holds every
  kind of person, for anybody else users alone, and both give the same
  people. Only of somebody whose kind Basecamp does not say does the list
  decide: in the list or a member of a project is not retired, named by a
  record alone is. A bot and somebody deleted cannot be asked for one by one.
  A bot comes with `email: null` as Basecamp gives it, somebody deleted and
  an outsider with the address Basecamp keeps, which is held to the same
  rules as every address.
  A person carries `createdAt` and `updatedAt` like any record: somebody of
  whom Basecamp says neither ends the run as `api_error`, named by ID, where
  a record first refers to them.
- **Avatars** are attachments of their person, `avatar-<personId>`, downloaded
  without a sign-in and only for people of the bundle. Basecamp serves a
  picture it made from initials at the same kind of address, with the same
  headers and the same type as one somebody uploaded, so they cannot be told
  apart and every avatar is taken; the report says so
  (`avatars.initialsDistinguishable: false`). The picture is stored as it
  comes, without converting it. `file` is relative to `files/`.
- **Projects** come with their members. Basecamp names no creator of a
  project, but it makes the first tools of a project in the same moment and
  in the name of whoever created it. So `createdBy` is the creator of a tool
  that was made within five seconds after the project
  (`projectCreationWindow`), compared as moments; a tool somebody added later
  never names the creator, and without such a tool `createdBy` is `null`. The
  tools the run extracts are looked at first, in the order of their IDs,
  which is the order they were made in, so that the oldest of them decides
  where several were made with the project and name different people. Only
  when none of them answers are up to three others asked for
  (`maxCreatorProbes`), those that are switched off included, in the same
  order, and only until one answers; a refusal counts like an answer and uses
  one of the three up, so a tool behind it is never reached. Such a tool that
  is not there, that lies outside the account, or that the identity may not
  see, is passed over: one entry somebody may not open says nothing about the
  project, and the run takes what the rest gives. A refusal is counted
  (`creatorProbesRefused`) and a `notice` says so, because the project may
  come without a creator for it. An identity that may see nothing at all is
  refused where the people of the account, the project and its members are
  asked for, which ends the run as it always did. The creator comes into the
  bundle like anybody a record names.
  The description is plain text there and HTML in the bundle: escaped, a
  paragraph per block, a `<br>` per line break.
- **Tools** are the dock entries that are switched on and that Minka has a
  counterpart for: `kanban_board` → `board`, `chat` → `chat`, `message_board`
  → `posts`, `questionnaire` → `checkins`, several of a kind where a project
  has several. Creator and
  timestamps come from the tool itself. Every other entry is counted as
  skipped, per kind and whether it was switched on, and shown in the report.
- **Columns** are the lists of a card table: `Kanban::Triage` → `triage`,
  `Kanban::NotNowColumn` → `notnow`, `Kanban::Column` → `work`,
  `Kanban::DoneColumn` → `done`. They are written in the order of a board:
  Triage, Not now, the work columns by their position, Done. Basecamp gives
  the three columns every table has no position, so `position` is the number
  of the column in that order. The colour of a column is left out.
- **Cards** are those of every column, and the archived and trashed ones.
  Basecamp lists the cards of a column itself, and the archived and the
  trashed ones of a project in one list across all its card tables, those
  that are switched off included. Such a card names its column and no table,
  so it goes to the board its column is of; a card whose column is in no
  extracted board, because the board is switched off or the column is gone,
  is left out and counted (`cardsLeftOut`).
  A walk has no snapshot: a card that is moved between two requests can be
  missed or met twice, and one met twice is written once.
  The cards of a board are written oldest first, by `created_at` and then by
  ID, across all columns and whatever their status: the applier hands out the
  short identifiers in that order. `position` counts from 1 through the
  column without a gap and is what the applier orders the column by: the
  cards by their position in Basecamp, where a card without one (in Done)
  keeps its place in the list, then the cards on hold, then the archived and
  the trashed ones as Basecamp lists them.
  A card that was set aside within its column belongs to that column, never
  to the list Basecamp keeps such cards in, and carries `onHold: true`; the
  key is left out otherwise. `completedAt` and `completedBy` come from the
  completion Basecamp records, `dueOn` as given, `assignees` in the order of
  their IDs. The note is the stored one (`content`), and `null` when empty.
- **Steps** come with the card, which shows them itself; only when it has
  more than it shows is their list asked for. Whom a step is assigned to and
  when it is due is left out, because a step in Minka has neither.
- **Categories** are the kinds of post of a project, each once: those the
  list of the project holds, and those only a post that is written shows.
  Basecamp lists the categories of the first message board of a project
  alone; a further board has its own, which are in no list and cannot be
  asked for, so such a category is what a post shows of it, and as old as the
  oldest post that carries it. `emoji` is the icon, `createdBy` is `null`.
  They are written in the order of their names, whatever the case of a
  letter, then of their IDs, and `position` is the number in that order,
  because Basecamp gives none. A category without a name ends the run as
  `api_error`, named by its ID, because the applier would stop at it; an
  empty icon comes as it is. The list has no pages and answers every page
  with all of it, so it is read once and without a page, and only for a
  project with a message board that is extracted.
- **Posts** are the messages of every message board, and the archived and
  trashed ones. Basecamp lists the messages of a board itself, and the
  archived and the trashed ones of a project in one list across all its
  boards, those that are switched off included; a message of a board that is
  not extracted is left out and counted (`postsLeftOut`). The posts of a
  board are written oldest first, by `created_at` and then by ID, whatever
  their status, the boards in the order of the tools. `title` is the subject
  of the message, because what Basecamp calls its title begins with the emoji
  of the category. `categorySourceId` is `null` for a message without a
  category. The text is the rendered one, as of a comment; an empty one comes
  as `body: ""` and is counted (`postsWithEmptyBody`): whether a post may say
  nothing is the applier's to decide.
- **Messages** are the lines of every chat: what somebody wrote, plain or
  formatted, what a bot wrote, and a file somebody dropped, see Texts.
  Basecamp lists a chat newest first, cannot be asked to turn it round and
  does not say how long it is, so a chat is read to its end and held as a
  whole, one chat at a time, and written oldest first, by `created_at` and
  then by ID (C-027). A walk has no snapshot here either: a line that arrives
  while the walk goes on moves every page, so a line can be met twice, and is
  written once, or be missed where one is deleted meanwhile. A line somebody
  wrote that says nothing is left out and counted (`messagesWithEmptyBody`):
  nobody comes into the bundle for it, its boosts are not asked for and no
  part ends at it. Nothing is what a reader sees nothing of: whitespace,
  characters without a width, `<br>` and empty `<div>` and `<p>`. A
  reference to a file or a person, a link, a rule and an embed say
  something: a line that was a video alone is a line, and one whose only
  frame is left out, see Texts, says nothing. A line comes as long as it is:
  nothing is shortened.
- **Parts.** The messages are written in parts: `messages-0001.jsonl` and on,
  numbered through the bundle in the order of the chats, which is the order
  of the projects and their tools. A part holds lines of one chat. It ends
  once it holds 10,000 lines, but only where the second changes that the
  lines were created in, so it may hold somewhat more: the applier keeps the
  lines of one second in the order of their file by counting them, anew in
  every part. The second is that of the moment, whatever zone and fraction a
  time is written with. A chat without a line has no part and leaves no gap
  in the numbers. The manifest names the parts in their order under
  `parts.messages`, and `counts.messages` counts all lines. Only a bundle
  whose lines came to a single part carries the one file `messages.jsonl`
  instead, and its manifest has no `parts`. A bundle never carries both. The
  writer of a bundle does this for any kind that is said to come in parts: it
  counts, and whoever adds the records says where a part may end.
- **Questions** are the questions of every questionnaire, oldest first, by
  `created_at` and then by ID. A questionnaire lists the questions it asks; a
  walk is no snapshot, so a question met twice is written once.
  The schedule of Basecamp becomes the one Minka has. The weekdays are Minka's:
  Basecamp counts Sunday as 0 and Saturday as 6, Minka Monday as 1 and Sunday as
  7, so every day but Sunday keeps its number and only Sunday moves; a day named
  twice is one day, and they come in order. `rhythm` is `weekly` where one
  weekday is chosen and `daily` where several are, because Minka's `daily` means
  "on each of the chosen weekdays" and the two are the same thing for one day.
  The frequencies Basecamp sends are `every_day`, `on_certain_days`,
  `every_week`, `every_other_week` and `every_month`. The first three all carry
  the days they are asked on, so they are the same thing and come to `weekly` or
  `daily` by the number of days; `every_other_week` becomes `everyOtherWeek` and
  `every_month` becomes `monthly`, both of which take exactly one weekday.
  `timeOfDay` is `HH:MM`.
  `status` is the one the source has, so a question it has shelved does not
  arrive active and start asking people again. `paused` is `true` for a question
  the source has paused and for one whose schedule has run out, because Minka's
  schedule has no end to say so with.
  `startsOn` is always written, never left out: Minka would otherwise take the
  day of `createdAt` in the offset the bundle wrote, which is Basecamp's UTC, so
  a question made late in the evening would start a day late. It is the
  `start_date` of the schedule; for an `everyOtherWeek` question it is the day
  the question was last answered on, stepped back to the weekday it is asked on,
  because Minka counts the weeks from the first matching day at or after
  `startsOn` and Basecamp says nothing about which of two weeks it asks in
  (C-046). `anchorsInferred` in the report names each such question with that
  day.
  `asked` comes from the subscribers of the question, which is whom Basecamp
  hands it to and not whoever answered it, and holds only those Minka can ask
  (C-045). Nothing hangs on a question: it carries no comment and no reaction,
  the format has no word for either of one, and none is asked for.
  A question whose schedule Minka has no words for is left out with its
  answers and counted in `questionsLeftOut` with the field that made it
  impossible: `week_instance`, `week_interval`, `month_interval`, `duration`, a
  `frequency` nobody knows, `days` that are empty, outside 0…6 or more than one
  where the rhythm takes one, an `end_date` that has not passed, a `start_date`
  or an `end_date` of another shape than `YYYY-MM-DD`, an `hour` or a `minute`
  of no day, and `group_on` for an `everyOtherWeek` question nobody ever
  answered. A title the applier refuses ends the run as `api_error`, named by
  ID: empty, longer than 200 characters, or with a character nobody can see. The
  whitespace around a title is taken off, as Minka takes it off.
- **Answers** are what somebody answered a question, per question oldest first,
  by `created_at` and then by ID; one met twice is written once. `groupOn` is
  the day the answer belongs to, taken from `group_on` as it is and never worked
  out from the schedule: Minka stores it and holds it against nothing, so an
  answer may well sit on a day the question never asked on, which is what the
  source says. Where Basecamp names no day, it is the day the answer was
  written, read in the zone of whoever wrote it (C-047), and counted in
  `answersWithoutADay`. The text is the rendered one, as of a comment, and goes
  the way every other text goes.
  An answer in the trash is left out and counted (`answersTrashed`), as a
  comment in the trash is, and one that says nothing is left out and counted
  (`answersWithoutABody`), because the applier refuses such a body. A question
  that has no answer costs no request. Comments, boosts and files hang on the
  answer.
- **Comments** are those of cards, of posts and of answers, `parentKind`
  `card`, `post` or `answer`,
  per parent oldest first, by `created_at` and then by ID: the applier reads
  the order of the comments of a parent from the file. Basecamp hands out the
  rendered text of a comment only. A comment in the trash is left out with
  its boosts and counted (`commentsTrashed`); any other comes, the archived
  comment of an archived card as well.
- **Reactions** are the boosts of a card (`targetKind` `card`), of a post
  (`post`), of an answer (`answer`), of a comment (`comment`) and of a line
  (`message`), with `content`
  as it is, a text of some words included: what Minka takes for a reaction is
  the applier's to decide. The boosts of a card or a post stand before those
  of its comments. A boost has one time, which is both `createdAt` and
  `updatedAt`.

Left out besides: the colours, wormholes between boards, who follows a card
or a post and whom a post told, and being on hold as anything more than the
flag.

What Basecamp does not hand out, so that no bundle carries it:

- Whether a message is pinned, and since when. `pinnedAt` is `null` on every
  post.
- A draft of a message. It is in no list; a status that says so would end the
  run.
- A line somebody deleted, and the lines Basecamp writes itself, such as who
  joined a chat.

IDs are the numeric IDs of Basecamp as strings; `sourceInstance` is the ID of
the account, taken from the address of a project. Times are written the way
Basecamp writes them, milliseconds included. People, projects, tools and
avatars are ordered by numeric ID, steps, comments and reactions by their
parents in the order of their file and then by position or age, whatever
order Basecamp answers in: two runs over an unchanged account write the same
bytes, apart from `extractedAt`, whether a run fetched its files or took
them over. `attachments.jsonl` begins with the avatars and goes on with the
files in the order the walk meets them, see Files. Only where Basecamp gives
cards no position, in Done and among the archived and the trashed, does its
own order count.

A card, a post and a line cost no request of their own: a list shows them in
full. Comments are asked for only where a card or a post says it has some,
boosts only where a card, a post, a comment or a line says so. The requests
go one after the other, never two at a time: the Basecamp CLI keeps to the
rate Basecamp allows.

### Texts

A bundle never carries the embeds of its source, and writes four tags of its
own in their place: `<import-mention>`, `<import-attachment>`,
`<import-embed>` and `<import-reference>`. In the note of a card, the body of
a post, of a comment and of a line, every `<bc-attachment>` is replaced as a
whole, with the figure Basecamp rendered into it:

- A mention becomes `<import-mention ref="<personId>">Name</import-mention>`,
  with the name the list of the account has. The person is looked for in the
  signed ID of the embed first, then in the avatar Basecamp rendered, then in
  what the editor wrote next to it (C-026). Only somebody the list of the
  account knows becomes a reference, and comes into the bundle like anybody a
  record names. Any other mention stays the name that stood in the text and
  is counted (`mentionsUnresolved`); for the stored note of a card, which
  shows no name, the name comes from the rendered one.
- A file becomes `<import-attachment ref="attachment-<id>"></import-attachment>`
  with the ID of the file Basecamp lists next to the text (`filesInBodies`).
  The reference carries `caption` when the embed has one, and
  `presentation="gallery"` when the embed says exactly that, in this order,
  escaped as attribute values, and nothing else. A caption of more than 1000
  characters (`maxCaptionLength`) is left off and counted
  (`captionsDropped`), and the file comes all the same. A stored note that
  says neither takes both from the rendered one.
  The listed file is found by the signed ID of the embed, then by the key of
  the stored file, which stands in the address of the embed
  (`…/blobs/<key>/download/…` in `href`, else `url`), then by the ID of the
  attachment the signed ID names. An older text carries the signed ID of its
  day and the list the one of today, so the first alone would lose such
  files. What an embed names counts only when a listed file confirms it: the
  listed file is what says where the file is fetched from. An embedded file
  that is not listed cannot be named: its caption stays as text, for the
  stored note of a card the one of the rendered note, the rest is dropped, it
  is counted (`filesDropped`) and a `notice` says so. It has no reference,
  and so no record and no file.
- A horizontal rule becomes `<hr>`.
- The preview of a link (`application/vnd.basecamp.opengraph-embed`) becomes
  the link, `<a href="…">title</a>`, with the address as the text where the
  preview has no title (`linkPreviews`). The address is the `href` of the
  embed, else its `url`. Only an absolute address over http or https becomes
  a link: of any other, the title stays as plain text, or nothing, and it is
  not counted. A link to a card, a post or a chat of the account becomes a
  reference, see below, and counts as a link all the same.
- A picture at a foreign address, an embed of the kind `image` without a
  signed ID, is no stored file. It becomes a link to its `url` with its
  `caption` as the text, by the rules of the preview of a link, a reference
  included, and never a
  picture: a bundle fetches nothing from an address somebody wrote into a
  text (`picturesLinked`, which counts what became a link). A picture with a
  signed ID, and one of a kind such as `image/png`, is a file as above.
- An embed of any other kind that carries no signed ID stays as it is, with
  all it holds but a frame.

What the source embeds from elsewhere, a video, a track, a player, stands in
a text as an `<iframe>`, mostly in a `<figure>` of its own. It becomes
`<import-embed url="<address>"></import-embed>` in the place of the frame and
of all the frame holds (`embeds`). The tag carries the `src` of the frame and
nothing else: the address the `src` stands for, read out of the HTML once and
escaped as an attribute value once. The whitespace a browser takes away
before and behind an address, blank, tab, line feed, form feed and carriage
return, goes before anything is asked of the address, because Minka takes
none away, and the tag carries the address without it; a `src` of nothing
else is no `src`. Nothing else of the address changes. The extractor tells no
provider from another and asks nobody about the address.

- A `<figure>` that holds a frame is not written, neither its start nor its
  end tag. What else stands in it stays where it stands, as the source wrote
  it, whitespace included. Its `<figcaption>` becomes a paragraph in its
  place, `<p>`, what the caption holds and `</p>`, which is after the tag
  wherever the source puts the caption after the frame; a caption that holds
  nothing but whitespace becomes nothing. Whatever the figure, the caption
  and the frame carry as attributes goes. A block around the figure stays.
- A figure that holds no frame, such as the one around a table, passes byte
  for byte.
- Every frame of a text becomes a tag, in the order of the text, in a figure
  or outside of one.
- Only an address Minka takes becomes a tag, and `embeddable` is the whole
  rule, asked of the address without the whitespace around it: it begins
  with `http://` or `https://` in whatever case, names a host, holds no
  character from U+0000 to U+0020, no U+007F and no backslash, and is at most
  2,000 characters long, counted as code points. No other character is
  refused. The host is what follows the scheme up to the first `/`, `?` or
  `#`, without what stands before the last `@` and without a port; a colon
  no port follows is no port, and a port is one to five digits and at most
  65535. About a port the rule is stricter than Minka, which reads the number
  a port begins with and lets the rest be: no tag is written that could be
  refused, at the price of an embed whose port ends in something else. A
  frame with any other address, relative ones, `//host/…`, `data:`, `about:`
  and `javascript:` included, and a frame without a `src`, is left out with
  all it holds and counted (`embedsLeftOut`), and a `notice` says so. Its
  figure goes the same way, so one that held nothing else leaves nothing, and
  a caption stays as its paragraph.
- A frame that is never closed holds the rest of the text for a reader of
  tokens, as it does for a browser. The rest is read anew as the text it is,
  so such a frame costs no more than itself, and once, however many frames
  follow: nothing closes a frame behind one that is never closed. A text is
  read in a time that grows with its length, whatever it is made of.
- A frame Basecamp rendered into a `<bc-attachment>` that is replaced goes
  with that rendering and is not counted. In an embed of another kind, which
  passes through, a frame becomes a tag or is left out like any other: no
  `<iframe>` reaches a body, and such an embed may never end.
- No output names an address.

A link to a card, a message or a chat of the account, and to a line of a
chat, becomes a reference to that record (C-031):

```
<import-reference kind="cards|posts|tools" ref="<sourceId>" href="<address>">words</import-reference>
```

- `kind` is the kind of record in a bundle: `cards` for a card, `posts` for a
  message, `tools` for a chat. A line of a chat is a reference to its chat.
  `ref` is the source ID of that record, which is the ID of Basecamp in the
  address, without the zeros it may begin with: the ID of a card, of a
  message, and of a chat, which is also the ID of the chat's tool. `href` is
  the address as it stood, escaped as an attribute value once, an anchor
  included. Nothing else of the link comes along.
- The words of the link stand between the tags as plain text, escaped once:
  the text of the link without any tag, a blank for a line break. An embed
  in them gives its words and refers to nothing: a mention the name the
  list of the account has, the one a person of the bundle carries, and the
  name Basecamp rendered only for somebody the list does not know; a preview
  its title, a picture or a file its caption, a rule nothing, a frame
  nothing. Nobody and no file comes into the bundle for it. A link without
  words gives an empty reference.
- Every such link is marked, whatever its words, and whether the record is in
  the bundle or not: the applier resolves what it knows and makes the link
  anew of what it does not, from `href` and the words.
- The address is asked without the whitespace around it. It begins with
  `http://` or `https://` in any case, its host is `3.basecamp.com` or
  `app.basecamp.com` in any case, without a port and without a name before an
  `@`, and its path, without query and fragment, is
  `/<account>/buckets/<n>/card_tables/cards/<id>`,
  `/<account>/buckets/<n>/messages/<id>` or
  `/<account>/buckets/<n>/chats/<id>`, where the account is the one of the
  run and every `<…>` is digits. A chat may be followed by `@<line>`, and each
  of them by `/` and anything, such as `/lines/<line>` or the menu of a line.
  The path is asked with its escapes read and its `.` and `..` segments
  resolved, so a path that climbs out of a page shows what it climbs to. A
  path that begins with `//` shows nothing: Basecamp writes none. An ID of
  more digits than a number holds is none.
- Any other link stays as it stands: a to-do, a document, a file, a card
  table, a message board, a project, another account, another host, a stored
  file, and a link without an address.
- A link ends where its end tag stands, where the next link begins, as it
  does for a browser, where a block such as a paragraph, a list item or a
  figure begins or ends, before the end tag of an element that began before
  the link, and at the end of the text, where the reference is closed. The
  block and the end tag stay as they were written, so an element ends where
  it did; an end tag without a link passes as it stands. An element that
  begins within the words gives its words, and the end of an element that has
  none, such as `</br>`, ends nothing.
- The references are counted by kind (`references`: `cards`, `posts`,
  `tools`).

A line of a chat that is a file somebody dropped has no text. Its body is one
`<import-attachment ref="blob-<key>"></import-attachment>` per file, in the
order of the line, without a caption, and counts as `filesInBodies`. Basecamp
gives such a file no ID, so the key of the stored file names it, which stands
in the address it is downloaded from (`…/blobs/<key>/download/…`), else in
the one it is previewed at (`…/blobs/<key>/previews/…`). A key is made of
letters, digits, `-` and `_`, because it becomes part of the ID of an
attachment; one that has not the shape of a UUID besides keeps its reference
and is a file no run can ask for, see Files. A file
without such a key is dropped and counted (`filesDropped`); a line that keeps
no file is left out with its boosts and counted (`uploadLinesLeftOut`), and
nobody comes into the bundle for it. A plain line and the line of a bot go
the way of every text and, holding no embed and no link to a record, pass
byte for byte.

The kind of an embed is its content type without parameters, in any case. A
signed ID is base64, `--` and a signature, which stands behind the last `--`
and is not checked. Today the base64 holds an envelope Ruby serialised, with
the global ID (`gid://bc3/Person/<id>?expires_in`) readable in it; an older
one holds JSON, `{"_rails": {"message", "exp", "pur"}}`, whose `message` is
the serialised ID as base64 again, in the usual alphabet or the one for
addresses, with or without padding. A global ID counts as a whole: nothing of
a word before it, nothing of a number or a path after it.

Every other token passes through with the bytes it was read from: Minka
sanitises a body, and the extractor adds no second opinion. A text is read
only when it holds `bc-attachment` or `<iframe`, or `<a` followed by
whitespace, `>` or `/`, all in any case; any other text passes as it is.
What goes besides an embed is what belongs to one: whitespace between an
embed and its rendering, an end tag of an embed without its beginning, and
an embed or a frame the text breaks off in. Any other tag a text breaks off
in stays as it was written, at the end, so a text loses no byte to being
read. A text is read token by token, never parsed into a tree,
because a tree moves a figure out of a paragraph; an embed that is never
closed costs no more of the text than its figure, and one that closes itself
nothing.

### Files

Every `<import-attachment>` the extractor writes has a record in
`attachments.jsonl` whose `sourceId` is exactly its `ref`, and its bytes at
`files/<sourceId>/<filename>`: `attachment-<id>` for a file embedded in the
text of a card, a comment or a post, `blob-<key>` for a file dropped into a
chat. The one reference without a record is a file the run could not fetch,
see Files that cannot be fetched (C-030).

- **The record** carries `filename`, `contentType` as Basecamp says it, or
  `null`, `bytes`, `sha256`, `file`, which is exactly
  `<sourceId>/<filename>`, `ownerKind` (`card`, `comment`, `post`, `message`,
  `answer`) and `ownerSourceId`. Basecamp gives a file neither a time nor a creator, so
  `createdAt` and `updatedAt` are those of its owner, and `createdBy` is
  `null`, which the format reads as the author of the owner. Avatars stay as
  they are, with their person as creator.
- **One record and one download per `sourceId`**, however many references
  name it: a text may embed the same file twice. Where a file turns up under
  a second owner, the first keeps the record and the case is counted
  (`filesOfSeveralOwners`).
- **Order.** The index begins with the avatars, in the order of the people,
  and goes on with the files in the order the walk meets them: the files of
  a card, those of its comments, the next card; then the posts the same way;
  then the lines; within a text in the order of the references. A file is
  fetched when its owner is written, and its record is held until the people
  are written at the end of the run.
- **Through the Basecamp CLI.** `basecamp files download <address> --out -
  --agent` streams the bytes of a file to its standard output. `--out -`
  matters: given a directory, the Basecamp CLI chooses the name itself and
  overwrites a file of that name. The address stands behind `--`, and
  `BASECAMP_NONINTERACTIVE=1` is set.
- **The address** is
  `https://storage.3.basecamp.com/<account>/blobs/<key>/download/<name>`.
  The extractor's own are the scheme, the host, the account of the run and
  the place of every part. Taken from Basecamp are the key, which has to
  have the shape of a UUID, and the name: the last segment of the
  `download_url` Basecamp gives the file, as escaped there, when that
  address names the same account and key, else the name of the file,
  escaped. Neither a host nor a path of an answer is passed on. The Basecamp
  CLI takes a download from a storage address only, not from the
  `download_url` itself, and sends its token to the API it is signed in at,
  whatever host the address names. The preview address of a file is never
  fetched: a bundle carries the original.
  Right before the call the address is held against one pattern
  (`checkStorageAddress`): https, a host that begins with `storage.` and
  ends in `.basecamp.com`, digits for the account, a key of the shape of a
  UUID, one segment for the name, and no query and no fragment. The name has
  valid escapes and, with them taken away, is neither `.` nor `..` and holds
  neither `/` nor `\` nor a NUL: the Basecamp CLI asks for the path without its
  escapes, so such a name would be a path of its own, as far as into another
  account, asked for with the sign-in. A file whose key or name does not
  pass is missing, not retryable, never `gone`, and is never asked for; the
  reason says whether the key has no form that can be asked for or the
  address Basecamp gives cannot be asked for safely.
- **Streaming.** The bytes go from the Basecamp CLI into the staged file
  while they are counted and digested (`bundle.Writer.StoreFile`); a file is
  never held in memory, and there is no limit of size and no check of the
  type: the extractor fetches everything and decides nothing. Directories
  are 0700, files 0600.
- **Limits of time.** The Basecamp CLI puts none on a download. A download
  that brings no byte for 60 seconds (`fileIdleTimeout`), or that has not
  ended after an hour (`fileTimeLimit`), is ended together with the Basecamp
  CLI and counts as a failure of the network. A run that is stopped ends the
  Basecamp CLI at once and is `interrupted`; nothing of the bundle stays.
  The reset of the idle time with every piece is what lets a long download
  through.
- **What a download came to.** The exit code decides whether it failed,
  whatever stands on standard output, where the Basecamp CLI prints its
  failure object in place of the file, or behind the part of a file that
  broke off. Exit 0 and as many bytes as Basecamp says the file has
  (`byte_size`, where it says one) is a file. Another number of bytes is a
  failure, retryable when fewer came; the size alone decides, whatever the
  bytes look like. Only where Basecamp gives no size does the look count:
  exit 0 with nothing but a failure object (`ok: false`) of a few kilobytes
  is then a failure and no file (`ErrLooksLikeAFailure`). Exit 2 with the
  failure object `not_found` alone on standard output is a file Basecamp no
  longer has:
  its record says `gone: true`, with `file`, `bytes` and `sha256` `null` and
  everything else as Basecamp lists it (`filesGone`, and a `notice`). A
  crash with exit 2 is no such proof, and neither is `not_found` behind a
  part of a file. A Basecamp CLI that is not signed in (exit 3) ends the
  run as `auth_required`, as it does for a request: no further file would
  come either. Every other failure, forbidden included, leaves the file
  out, see Files that cannot be fetched. The download of a file is not asked
  again within a run.
- **Names.** `filename` is a plain name: `/`, `\` and control characters
  become `_`, an empty name, `.` and `..` become `file`, and a name of more
  than 255 bytes, which is never more than 255 characters, is shortened
  before its extension, because a file system takes no more
  (`plainName`). `fileNamesChanged` counts the names that changed. No output
  ever says a name, a caption or an address: they are personal data, and a
  reason names the `sourceId` and a class of failure, never what the
  Basecamp CLI printed.
- **Taking files over.** A run replaces the bundle as a whole, and would
  fetch thousands of files again. So a file is taken over from the bundle at
  `--out` that the run replaces, instead of being downloaded, when the index
  of that bundle has a record of the same `sourceId` that names the same
  `file`, the file is a regular file of the size the record gives, which is
  also the size Basecamp gives, and its digest, computed anew, is the one of
  the record (`bundle.Writer.ReuseFile`). Anything else is fetched. A record
  that says `gone` is not trusted, the file is asked for again; an avatar is
  never taken over, because somebody may have another picture under the
  same ID. The old bundle is only read. The file is linked (a hard link),
  or copied where it cannot be linked or is readable by others, and the
  copy is digested again. A link shares its content with the old file, so
  nothing is ever written into a file that stands: every file goes into a
  directory that is made anew for it, one per attachment, and a directory
  that stands takes nothing. A file is only ever removed by its name.
  Removing the old bundle, giving a run up and tidying up after a killed one
  therefore leave the other bundle whole. `filesReused` and `filesFetched`
  count both ways.
- **What a run that ends hard loses.** Files are taken over from a bundle
  that is in place, and a bundle takes its place only at the end of a run.
  A run that fails, is stopped or is killed therefore loses the files it
  had fetched, and the next run fetches them again. The first complete run
  over an account is the expensive one; every run after it fetches what is
  new or was missing.

A further kind is a further step of the walk that adds its records to the same
writer, feeds the creators it meets into the same people and attaches the
files of its texts the same way.

## A run

- The bundle is built in a directory next to `--out`,
  `.<name>.staging-<random>`, its parts included, and takes its place with a
  rename at the end; the bundle of the run before is moved aside first and
  removed last (C-024).
  A run that fails or is interrupted, by Ctrl-C or by SIGTERM, leaves what was
  there: the earlier bundle, or nothing, not even the directories it made on
  the way to `--out`.
- When the new bundle cannot take its place, the old one is put back; when
  that fails too, the error names where it lies. An old bundle that cannot be
  removed afterwards is a `notice` with its path, not a failure.
- A run that is killed cannot tidy up. The next run into the same `--out` does:
  it removes what was being built, which is personal data nobody will use, and
  puts a bundle back that was moved aside when no new one took its place. It
  touches only real directories named exactly `.<name>.staging-<digits>`, with
  or without `.previous`: the name of `--out` never works as a pattern, and a
  link is left alone.
- One run at a time works on an `--out`. A run holds an advisory lock of the
  kernel (`flock`) on `.<name>.lock` next to it, from before the tidying up
  until the bundle is in place, and removes the file at the end. A second run
  is refused as `config` with the path of the lock. The kernel drops the lock
  with the process, so a run that was killed locks nobody out; it leaves the
  empty file behind, which the next run takes over and removes. minka is
  released for macOS and Linux; on a system without `flock` it builds and
  does not lock.
- An `--out` that is a symbolic link is refused as `config`: the bundle would
  take the place of the link, and the one behind it would stay where nobody
  looks for it.
- A bundle of an earlier run in `--out` is replaced as a whole. It counts as
  one only when its `manifest.json` says `format` 1 and the same `source`: a
  directory with anything else in it is refused, a web application with a
  manifest of its own and a bundle of another source included, and the refusal
  names the first entries that are in the way. The check
  happens before Basecamp is asked and again before the replacing.
- `--dry-run` walks Basecamp the same way, downloads no avatar and no file,
  writes nothing and reports the same counts and the same parts, and what a
  run would fetch: `filesExpected`, the files the references name, each once,
  and `filesBytesExpected`, the sum of the sizes Basecamp gives them. It does
  not look at what a run could take over. It needs no `--out`; given one, it
  checks it the way the real run does, so the refusal shows beforehand.
- The report carries `counts` as the manifest has them, `avatars`,
  `skippedTools`, `missingFiles`, `creatorProbesRefused`, `cardsByStatus`,
  `cardsOnHold`, `cardsLeftOut`, `commentsTrashed`, `postsByStatus`,
  `postsLeftOut`, `postsWithEmptyBody`, `messagesByType` (`text`, `richText`,
  `upload`, `integration`), `messagesWithEmptyBody`, `uploadLinesLeftOut`,
  `parts` as the manifest has them, `mentionsUnresolved`, `filesInBodies`,
  `filesDropped`, `filesExpected`, `filesBytesExpected`, `filesBytes`,
  `filesFetched`, `filesReused`, `filesGone`, `fileNamesChanged`,
  `captionsDropped`, `filesOfSeveralOwners`, `linkPreviews`,
  `picturesLinked`, `embeds`, `embedsLeftOut`, `references`,
  `questionsLeftOut`, `anchorsInferred`, `askedLeftOut` (`notInTheProject`,
  `retired`, `withoutAnAddress`), `answersWithoutABody`, `answersWithoutADay`,
  `answersTrashed`, `longestBody`, `requestsRetried` and the absolute path of
  the bundle. The lines about check-ins are shown only by a run that met a
  questionnaire. `filesBytes` is
  what the files of the bundle weigh, fetched or taken over; it and
  `filesFetched`, `filesReused` and `filesGone` are 0 in a dry run.
  `longestBody` is the longest text per kind, `cards`, `comments`, `posts`,
  `messages` and `answers`, in characters: whoever sets the limits of where a bundle
  goes has to know it. `requestsRetried` is a number, 0 for a run that met no
  failure; the text shows its line only when it is more, and so it does for
  `creatorProbesRefused`, `embeds`, `embedsLeftOut` and `references`.
  No output names a title, a person or what stands in a text.
- A run prints nothing while it goes on, in any mode: the output contract
  knows results, warnings and errors only.

## Files that cannot be fetched

One file that is not to be had does not cost a run (C-025). Its attachment
record is left out, and the bundle is written and put in place: it is whole
and can be applied. A person whose avatar is missing comes with
`avatar: null`; a text keeps the reference to a file that is missing, which
the applier passes over until a later bundle answers it. What was written of
such a file is removed. `missingFiles` names each file with
`attachmentSourceId`, `ownerKind`, `ownerSourceId`, `reason` and `retryable`.
The rule holds for every file of a bundle. What ends the run is an
interruption, a bundle that cannot be written, and a Basecamp CLI that is
not signed in when a file is asked for. A failure of the file system comes
without the path it happened at, which ends in the name of the file. A file
Basecamp no longer has is not missing: it has a record that says so, see
Files.

Such a run fails, so that a caller notices: `network`, exit 6 and retryable
when at least one file failed for a reason that may pass by itself, else
`api_error`, exit 7. The report is in the output all the same: as `data` of
the error envelope with `--json` and `--quiet`, as text before the error
otherwise. A later run replaces the bundle and brings what it can get then.

An avatar is fetched over https only, redirects included and at most five of
them, with a timeout of 30 seconds and at most 5 MiB, which is enforced while
reading. It has to be a picture of a known type: an error page must not end up
in a bundle. A file of a text or a chat comes through the Basecamp CLI, see
Files. A reason never carries the address or the name of a file, which name
a person or what they wrote.

## A bundle is personal data

A bundle holds names, e-mail addresses and pictures. Its directories are 0700
and its files 0600. It is written outside of any repository and deleted once
it has been applied. Nothing out of a real bundle appears in the repository:
not in a test, a fixture, a document or a commit message. Tests run against
`internal/fakebasecamp`, an invented account, and never call the Basecamp CLI
or the network.
