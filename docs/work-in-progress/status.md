# Status

The first look of every new session. What holds is under `stable/`; here
stands where the work currently is.

## Where we are

The CLI serves what `/api/v1` offers: projects, boards, cards including the
trash and the archive of a board, columns with their area for cards on hold,
assignees, due dates, steps, subscribers, comments, chats, private
conversations, uploads, mentions, reactions and one's own cards across all
projects. `minka commands` lists them all, built from the command tree itself.

The archive of a board is read with `boards archive`, and `cards archive` and
`cards unarchive` move a card in and out of it. `--all` walks every page in one
run and works around the weakness Minka carries in its own debts: a cursor that
counts only while the card it names lies in the archive. The rule the commands
follow is [The archive of a board](../stable/archive.md) (C-054). None of it has
been seen live: the whole feature is checked against the fake, see below.

A file goes both ways: `uploads create` puts one in, `uploads download`
fetches its bytes back. Every text says which files hang in it, so reading a
card, a comment or a message gives the identifier a download takes, without
anything being taken out of the text.

`minka import basecamp` extracts the structure, the boards, the message
boards, the chats, the check-ins and the to-dos of Basecamp projects into an
import bundle for Minka's applier: people with avatars, projects with members,
the boards, chats, message boards, questionnaires and to-do sets as tools, of
each board the columns, cards, steps, comments and boosts, of each message
board the posts of every status with their categories, comments and boosts, of
each chat its lines as messages with their boosts, written in parts of one
chat and some 10,000 lines each, of each questionnaire its questions with the
schedule Minka has words for and their answers with what hangs on them, and of
each to-do set its lists from the bottom up, because Minka puts every new list
on top, with their groups and their to-dos in the order of the list and what
hangs on a list and a to-do.
A record the source describes with a word a bundle has none for is left out and
counted, and the run goes on (C-050): a chat line of an unknown kind — a bundle
has a word for `Chat::Lines::Text`, `Chat::Lines::RichText`,
`Chat::Lines::Upload` and `Chat::Lines::Integration` — or with a status other
than `active`, a column of a kind a bundle has no role for, a card, a post,
a comment, a question or an answer with a status a bundle does not know, a draft
among them, and a group or a to-do the source has put on the shelf, which a
bundle gives none. What stood in a list or a group a bundle cannot carry falls
with it, is asked for to be counted and is named with the place it fell with
(C-052), so that no run says a bundle holds more of the source than it does.
`recordsLeftOut` names each with the kind of record, the word and where it
stood, so one exotic record does not cost the account. What still ends
a run is what would be written wrongly: times that are missing or are none, a
second Triage, Not now or Done on one board, the status of a project the run was
asked for. Where a chat line is brought only by a later run and another line
carries the same second, the order of the two can turn round, which is a known
limit and written down with the rule.
Mentions in the texts are references, the preview of a link is the link, a
video or a player embedded in a text is an `<import-embed>` with its address,
a link to a card, a message or a chat of the account is an
`<import-reference>`, and the files of the texts and the files dropped
into a chat come with a record and their bytes, streamed from the
Basecamp CLI or taken over from the bundle the run replaces. It reads
through the Basecamp CLI and is tested against the invented account in
`internal/fakebasecamp`.
The check-ins are checked against the fake alone: no extraction has met a
questionnaire live yet. The frequencies Basecamp sends are `every_day`,
`on_certain_days`, `every_week`, `every_other_week` and `every_month`, and the
fake holds one question in each of the five; `every_month` on one weekday is the
first such day of the month, as Minka's `monthly` is. A frequency nobody knows
leaves the question out and is reported with its field, so such a run says what
is missing instead of writing a schedule nobody chose.
It has been run live, as a dry run and into a bundle outside the repository,
under an identity that sees addresses, the posts and the chats included: all
references of that bundle resolve and its parts are the ones its manifest
names. The files, the embeds, the references and the mentions have been run
live for both projects: every file fetched, every embed written, every
reference and every mention as the format asks, see below.
A run needs a Basecamp identity that may see the e-mail addresses of others,
which is somebody of the account's own company or an admin. Under one that
may not, masked addresses were seen live ending the run as `forbidden`. Only
the people the chosen projects refer to come into a bundle.

A dry run over the whole account walks through: all thirty-two projects, and of
the words a bundle has no place for that account holds exactly one, the kind of
line a run used to end on. What such a run is worth is the size it names before
anything is fetched, and one thing it named that nobody expected: a comment
seven times longer than what Minka takes, which the applier passes over rather
than ending on. Nobody could look at that comment, because the number named no
record; `longestBodyRecords` now says, beside every length, which record it was
measured on — the kind, the identifier and the place it stands in — so the next
such number can be traced without walking the account again. Basecamp's own
address of the record is not in it: no run carries one. A run into a bundle over
the whole account is still owed, and its size is Robert's and Shaun's to allow.

The working notes on the back of a card are read with `cards notes show` and
written with `cards notes set`, which needs `--expect` or `--force` (C-042).
Seen live against the local instance, which runs Minka's main with the notes
not yet released: reading notes nobody wrote, the first write with `--expect
none`, a refusal as `notesChanged` for a stale time and for `none` over
existing notes with the notes as they stand, a write with the right time,
clearing with `--force`, and a 404 for an unknown card. The card resource
does not say whether a card has notes, so `cards show` only points to them.

An agent says which cards it has in hand: `work start` and `work stop` keep
one list per machine, `heartbeat` reports it to `PUT /me/work`, and `hooks
install` writes the hook of Claude Code that calls it after every tool call.
A beat that finds a card in hand and no keepalive of its host starts one
(C-043), so a tool call that runs for longer than the three minutes a signal
counts for keeps its tiles alight — up to twenty-five minutes after the last
beat of the session, and no further. That process is at no work itself and stops
on an empty list, on twenty-five minutes in which nothing else was at the store,
or on a clock that jumped.
The list, the refusals, the throttle and the hook are seen live, and the
report with them: on `minka.flownative.com`, where this CLI is signed in as an
agent, `work start`, `heartbeat` and `work stop` get through, and the hook
reports out of a session of its own. What nobody has seen yet is a tile alive
on the board, see below.

Everything else is tested against the fake API and checked live — the chain
upload → attachment in the text → display → the bytes back, as well as private
conversations in both directions. What is checked against the fake only is
listed below.

Coordination of the API: the Minka session `minka-ca`. It reports changes,
which are checked against the Minka code; deviations go back to it.

Versions are released by publishing a release in GitHub and installed through
the Homebrew tap; `setup`, `doctor` and the skill inside the binary cover the
first steps.

Dogfooding waits for the internal alpha. The smoke token `minka-cli-smoke`
(account Bruno, local instance) gets revoked then.

## Checked against the fake only

- Of the notes of a card: `cardNotesHasControlCharacters`, and `cardArchived`
  for a card in its board's archive, which no write of the fake refuses with.
  Notes over 100000 characters are refused before sending, counted as the
  server counts them since it was brought into line. The fake refuses in the
  controller's order, the text before the guard, and reads a guard's time in
  the forms the server reads and no wider. All of it read in the Minka code.

- That a conversation with somebody from a foreign organisation gives 404: the
  fake API knows no organisations. Read in the Minka code.
- That a foreign attachment silently disappears on saving: the fake does not
  remove it.
- Unicode NFC for the content of a reaction: the server normalises, the fake
  does not.
- That a write into an archived project answers 422 `projectArchived`: no
  project that may be written to live is archived. Read in the Minka code and
  its tests. Live, the local instance shows `status` and takes the filter; the
  test instance runs an older Minka, where `projects list --archived` refuses
  as it should.
- A `shortId` that is not null, and a card found by it: live, every card
  carries the field and the address answers, but no board that can be read
  there shows short identifiers, so only the 404 was seen, for `cards show`
  and for `work start` alike; and no card that `work stop` could find under
  one has ever lain in the list.
- The order of a chat where only an import shows it: numbers out of order down
  a list, and imported lines that are never unread. Nor have unread messages
  of other people been seen, the smoke account being the only one. Live,
  the local instance shows the shared counter (edits and reactions take
  numbers), the order by creation, `--before` also for a number that was taken
  back or that no message carries, the read mark and its hint taking the
  highest number of a page, one's own messages never unread, and the refusals.
- Of `import basecamp`: a Basecamp CLI that is not signed in, forbidden by
  Basecamp or rate limited, a creator who left the account, a dock entry
  asked for the creator of a project that the identity may not see, and that an
  address which is none comes out as `null` with its person named in
  `invalidAddresses`: the account holds such an address, but on nobody the
  projects extracted refer to, and it holds nobody retired. A run that is
  killed was not seen live either. Seen live: a missing Basecamp CLI, masked
  addresses ending the run, a missing project as `not_found`, people of the
  account being left out, and files that could not be fetched, with the bundle
  written without them and the run ending as `network`, and a 525 of a
  gateway in front of Basecamp ending a run. That a request which failed for
  a reason that may pass is asked again is tested against the fake and a
  stand-in of the Basecamp CLI only. The classes are the exit codes and codes
  the Basecamp CLI shows; of its source only the failure object and the
  wording of an API failure were read.
- That a time of a record which is no moment ends the run: no real account has
  ever sent one, so every such string is an invented one. Held against the
  expression the applier reads a moment with, string by string, and for every
  kind of record and every field a moment reaches a bundle through, the to-do
  lists, groups and to-dos included.
  At two places the run is stricter than that expression, which takes them and
  no calendar has them: an hour of `24`, as in `…T24:00:00Z`, and a leap second,
  as in `…:60Z`. A run that ends on one of them is doing what it is meant to,
  and nobody need go looking for the cause.
- Of the boards of `import basecamp`: a mention in a stored note that has no
  rendering, a mention of somebody who left the account, a boost on a card, a
  trashed comment, a card whose column is in no extracted board, more steps
  than a card shows itself, and a second system column. Seen live, in a bundle
  that was checked reference by reference: cards on hold, archived and trashed
  cards with their comments, completed cards ranked as Basecamp lists them,
  mentions resolved from a rendering, horizontal rules, and files whose signed
  ID is an older one than the list has, found by the key of the stored file.
  The fake mirrors both forms of a signed ID as they were seen.
- Of the posts and chats of `import basecamp`: an outsider or a placeholder
  as the writer of something, a part that runs past its size because a second
  does, a trashed post, a post of a message board that is not extracted, a
  file dropped into a chat without a usable key, a line left out and counted for
  its kind and one for its status, a post of a status nobody knows, a column of a
  kind nobody knows with the cards in it, a card, a comment, a question and an
  answer of a status nobody knows, and what ends a run: a category without a name
  and a person without their times. A fifth kind of line does exist: a dry run
  over all 32 projects of the account met `Chat::Lines::Play` and ended on it,
  which is what the rule is for; no run has gone through a record that is left
  out for its word yet, and no live account has shown a draft, a second Triage or
  a column of a kind nobody knows either. Seen live,
  in a bundle that was checked reference by
  reference, its people against the kind Basecamp gives them: chats listed
  newest first in pages that grow, the four kinds of line, files dropped into
  a chat, bots and deleted people as writers who come retired although an
  admin's list holds them, boosts on lines, a line that says nothing left
  out, posts with and without a category, archived posts, a second message
  board with categories no list holds, previews of links and pictures at a
  foreign address as links, and five parts of one chat each.
- Of the to-dos of `import basecamp`: all of it. No extraction has met a To-dos
  tool live, so everything about them rests on the invented account and on
  reading: that a dock entry of a to-do set is called `todoset`, which is read
  in the code of a client that speaks to the real Basecamp and not from an answer
  of it; that a list, a group and a to-do carry `title`, `description` with
  `description_attachments`, `starts_on`, `due_on`, `assignees`,
  `completion_subscribers`, `completion` and `subtasks_count` under those names;
  that the lists of a tool come from its `todolists.json` topmost first and the
  groups of a list from its `groups.json` in the order of the list; that what is
  done is listed apart, under `todos.json?completed=true`; that the archived and
  the trashed lists, groups and to-dos of a project come as recordings of the
  types `Todolist` and `Todo`, a group being a `Todolist` whose parent is a list;
  and that the groups and the to-dos of a list on the shelf or in the trash can
  be asked for at all, which is what a run does for a list of any status. A to-do
  whose completion is newer than the moment the source says it was touched
  (C-051) is a shape the invented account holds and no real one has shown.
- Of the files of `import basecamp`: a download that breaks off, a file
  whose size differs from what Basecamp says, a file that is gone, a name that
  has to be changed, a caption too long, and one file under two owners.
  Seen live, in a bundle of both projects that was checked file by file,
  size and digest computed anew, and that Minka's `Build/verify-bundle.sh`
  passes: some 2,700 files of 6.1 GiB fetched in about an hour, none missing,
  the largest of 273 MiB, files dropped into a chat, files in posts and comments,
  pictures, PDFs, videos and sound; and a run into a clone of that bundle,
  which took 2,687 of its 2,688 files over and fetched the one that was new,
  twenty minutes instead of an hour. That the Basecamp CLI asks for the path of
  an address without its escapes was read in its SDK, not seen.
- Of the embeds of `import basecamp`: a frame that gets no tag, a caption in
  a figure, a frame outside a figure and broken markup. Seen live, in the
  bundle of both projects: some 500 embeds written, none left out, no raw frame
  and no embed of Basecamp left in a text. The rule of which address becomes
  a tag is read from Minka's `RichText::isEmbeddableUrl` and kept stricter
  where PHP reads a port more loosely.
- Of the references of `import basecamp`: a link to a chat itself, one of
  another account and one carrying a query. Seen live, in the bundle of both
  projects, checked one by one against Minka's format: 93 references to 46
  cards, 33 messages and 14 chats, each carrying `kind`, `ref` and the address
  as it stood, plain text between the tags and nothing else; a line of a chat
  points at the chat it stands in, `tools` stands for no board and no message
  board, and no plain link to a card, a message or a chat of the account was
  left behind. Of the 93, 39 point into projects the bundle does not carry:
  Minka's applier makes those links again and counts them, so they are marked
  all the same.
- Of the mentions of `import basecamp`, now seen live in the same bundle: 784
  of them, each with one `ref` that names somebody the bundle carries, and
  between the tags the plain name of the account's list. Eight name somebody
  who is no member of the project the text lives in, whom Minka's applier
  turns back into plain words.
- A known limit of the files: a run that ends hard loses the files it
  fetched, because files are taken over from a bundle in place only. The
  first complete run over an account is the expensive one.
- Of the cards on hold, two things. That a column of another board answers 404
  needs an instance with a second board that may be written to, `columns
  on-hold` finding a column wherever it stands. That the server puts a card
  whose neighbour stands in the other area at the top of its own area is
  reached by no call of the CLI, which refuses such a neighbour itself.
  Everything else of the area is seen live: switching it on and off, by
  the ID of the column and by its title with `--board`, a second switch that
  changes nothing, a card put on hold and taken off again, three cards handed
  back to the end of the column in the order they stood on hold, a placement
  among the cards on hold, a card that comes back from the trash off hold, the
  refusals of a working column without the area, of the system columns and of a
  column that is not there, each with its code, and the area with its count in
  the text view of the board and of the card.
- The fake API knows a card in the archive of its board: it says `archived`, is
  read at its own address, stands in no list, goes into the trash from the
  archive and comes back onto the board, and it is left out of a set at work
  with 200 and `leftOut` while everything else of the set is noted. What it does
  not mirror is the refusal of a write on such a card: every write answers as an
  address that has nothing instead of 422 `cardArchived`, and the order in which
  a move refuses — an archived card before a column without the area on hold —
  is read in the Minka code alone.
- Of the archive of a board, all of it. No page, no walk and neither write has
  been seen live; everything rests on the Minka code —
  `BoardApiController::archivedCardsAction`,
  `ToolQueryService::findArchivedCardsOf` and `isArchivedCardOf`, the two
  handlers, and the weakness of the cursor as Minka's own debts write it down.
  The fake mirrors that code: the order of the refusals with the 404 for the
  board before both 422s, `archivedCardsLimitNotANumber` for a limit that is no
  whole number of at least one, the silent cut of a page above 100, the order
  `archivedAt` descending with a card whose moment nobody noted last and the id
  deciding a tie, `cardArchived` and `cardNotArchived` behind `projectArchived`,
  and a card in the trash as an address that has nothing. It gives the boundary
  card's disappearance in both forms, which no live run can be made to produce on
  purpose, and as many cards at a time as a walk can lose:
  `ForgetsTheArchiveBoundariesOf` for the 422 and
  `RacesOnTheArchiveBoundariesOf` for the empty 200 of the race, where the same
  card named twice is that window met twice. What the fake
  cannot show at all is a column deleted from a board, so that a card comes back
  into the first working column or into Triage instead of the column it left: the
  fake deletes no columns, and that branch is read in
  `UnarchiveCardHandler` alone.
- That a card is left out of a set at work is read live nowhere yet: it was read
  in Minka's code, and the answer the fake gives is the one that code builds.
  What a word for it that this CLI does not know does to the list is seen
  against the fake alone, which gives such a word when it is asked to.
- The trash of the fake holds cards alone. That a post, a to-do, a question, a
  folder or a document names its tool in the field `board`, and that `blockedBy`
  can name a to-do list, a group, a question or a folder in the trash or in an
  archive, is read in Minka's code and held against the text view on its own.
- Of saying what one is working on, what is left: a tile that lights up and one
  that goes quiet. The signal itself got through on `minka.flownative.com`,
  which knows this CLI as an agent, and the hook has reported out of a real
  session; on the local instance the only account that may write is a person,
  whom the address turns away with 403 `agentsOnly`. The sign of life is tested
  against the fake alone: that the heartbeat of the session which took a card
  up keeps it in the list beyond the eight hours, and that a card of a session
  which says nothing more runs out while the machine around it works on. The
  order of the refusals is tested against the fake and read in Minka's code,
  its tests and E-184. What the fake does not mirror: it never forgets a signal
  after the three minutes Minka counts it for, because nothing the CLI does
  depends on that; and an `Accept` without JSON gets 406 there before the
  token is looked at, which no request of this CLI sends.
  Everything else is seen live: the refusal of a person's token, silent and
  exit 0 as a hook needs it, with its reason afterwards in `work list` as
  `failure`; `work start` by ID and its 404 for a card nobody can see;
  `work stop` by ID, without an argument for the one card and refused for
  several, `--all`, and a card that is not in the list; the auth refusal of
  `list`, `start` and `stop` on a machine signed in nowhere; ten processes at
  once losing no card, and ten stopping beside signals without one going
  astray; the throttle across processes, which reports at once when the list
  changed, keeps quiet for the minute after and reports again beyond it; a
  signal bounded to five seconds where a host does not answer, against the
  thirty of every other command; nothing remembered returning without a
  request and without making a directory; the eight hours taking out, for every
  reader, a card nobody said anything more about; and the text view of all of
  it. Measured outside a test, though not on a board: a tool call of a subagent
  carries the session of whoever spawned it and runs the hook, so a session
  that works through subagents keeps the sign of life of its cards.
  Of the keepalive, nothing at all is seen live yet, and against the fake not
  everything either: what a tick decides is tested as a plain function with a
  moment handed to it, and the lock, the mark of a beat, the sweep of a tick, the
  mode that is at no work, the host the child is told, the store that cannot be
  read and what a beat says became of the keepalive are tested with no process
  forked. Seen by hand outside the tests, on a store of its own and against a
  host that answers nothing: the output of a beat read through a pipe returns at
  once although the child is running, that child has `ppid 1`, a session of its
  own and all three streams at the null device, it reported once a minute, and it
  left neither mark behind. What no test reaches stands under the limits below.
  `hooks install` and `hooks status` are seen on real files: a fresh one, one
  carrying other hooks, one carrying ours, a `hooks` that stands there as
  `null`, and the shapes that are refused unrepaired with the entry to add by
  hand.
- "agent for X" in `people list` has been seen live, an agent that stepped
  down has not — it is in no list any more.
- What Minka takes for one file is 100 MiB. The CLI refuses a byte over that
  itself, which is tested; what a server answers above the limit is played by a
  stand-in that gives the bare status, so neither the `tooLarge` of the fake nor
  the hint the CLI has for it is reached by any test. No file of that order has
  ever been sent live either, neither before the limit was raised nor after;
  the number is read in Minka's `uploads.maxBytes` and in the controller that
  takes an upload of the API.
- Of `uploads download`, what is left: a file of a project one is not in, for
  which the local instance offers no second account; the entity tag with the
  304 and the 416 that go with it; the ranges an audio file is played in; and
  an answer that weighs something other than the file says. The CLI sends
  neither a range nor `If-None-Match`, so none of that is reached from here.
  Not mirrored by the fake either: that a picture and an audio file keep their
  own media type where every other download leaves as an octet stream, and
  that the server writes a picture anew when it takes it in — the fake keeps
  the bytes that arrived, so the `original` of a picture weighs there what it
  weighed here, and live it does not. Everything else of the command is seen
  live: the chain from upload through the description and `cards show` back to
  the bytes, a picture of 2400 × 1350 whose pixels come back unchanged, its
  display rendering at 1600 × 900, a picture that has none, `--variant`
  refused for a rendering the file has not got and for one that is no
  rendering, `-o <name>`, `-o -` with the bytes alone on standard output and
  the word about them on standard error, `--json` and `--quiet` refused beside
  `-o -`, a file that is already there and `--force` over one, every written
  file 0600 whatever stood there before, an identifier that names nothing as
  2 `notFound` and one that is no UUID as 1 without a request, `attachments`
  of a comment and of a chat message with the card naming none of its
  comments' files, and the text view of all of it.
- No redirect to a store happens on the local instance: it keeps its files in
  its own file system, and only a Google Cloud Storage target signs an address
  to send a client to, then only for a picture that is not asked for as a
  download and whose name matches its media type. So the following of a
  redirect is shown for the client alone, against servers of a test: the token
  stays behind, an address that is not https is not followed, and five hops
  are the end. No command of the CLI has ever fetched a file over a redirect.
  It waits for an instance whose files lie in such a store.

## Known limits of the tests

- `TestADownloadThatKeepsBringingBytesIsNotGivenUp` demands a byte of the
  stand-in every 300 milliseconds and goes red when the machine is loaded
  enough that one of them is late. The reserve is a trade: the stand-in ticks
  every 20 milliseconds, and a wider reserve makes every run of the test
  longer, because the run has to outlast twice the idle allowance to show
  anything. It is the one test that makes the pace of another program its
  precondition, the named exception to [C-035](../decisions.md#c-035).
- Of the keepalive, three things are left untested on purpose, and the split is
  the point of [C-035](../decisions.md#c-035) rather than a new exception to it:
  the detachment itself, the waiting of the loop, and a machine that really
  sleeps. The spawning is reached through an injection point on `app` that a
  stand-in fills, so no test forks a detached process; the decision of a tick is
  a moment handed to a plain function, not a machine put to sleep; and what the
  loop does on its first turn — take the lock, read the store, report, and the
  mode it does it in — is reached by a run that is told to stop, so only the
  waiting and what follows it stand outside. Of the endings, the empty list
  reported before the exit is the one no test reaches.
- What a tick decides under a machine that really slept is held by no test: a
  reading of the monotonic clock differs from the wall clock only where a machine
  was really away or a clock was really set, and in a test every way of moving a
  moment either shifts both readings together or strips the monotonic one. So the
  `Round(0)` in `tick.decide`, which is what makes the sleeping machine a jump,
  can be dropped with every test staying green, and the sleeping machine of the
  live check is what says whether it is there.
- `TestARunThatIsStoppedIsInterruptedNotFailed` needs SIGINT to reach the
  stand-in, which sends it to itself. A shell sets SIGINT to ignored for a
  command it starts in the background, and a program inherits that, so the
  test goes red whenever the test binary runs as a background job — in a
  session of an agent, say. In the foreground it is green, under load as
  well, and so it is in CI.

## Open elsewhere

- The live check knows two instances by name, the test instance and the local
  one. `minka.flownative.com`, where the signal of an agent was seen, is in
  neither row, so the guide does not say what may be written there. Whose
  decision that is is Robert's and Shaun's, not the coordinator's.

## Open in the API

- Posts have `attachments` too, and the CLI has no commands for posts at all.
- Renaming and reordering steps does not exist.
- Paging Done beyond 25 cards does not exist; needed once dogfooding needs it.
- Flow answers a wrong HTTP method with HTML instead of JSON.
- Column colours come later, read-only.

## Next steps

- The bundle with the posts and the chats waits for Minka's appliers of
  posts and of chat lines; what those runs find comes back through the
  coordination.
- Install through Homebrew once and play `minka setup` through by hand on a
  terminal: the release workflow has built a version and written the formula,
  but nobody has installed from it yet, and `setup` is covered by tests only.
- Once the repository is public: the formula installs the archives, a release
  gains a build provenance attestation with instructions for verifying it, and
  the macOS binaries get notarised, which waits for an Apple Developer ID.
- See a tile live: one that lights up and one that goes quiet at once.
- Check the keepalive live, as the [live check](../guides/live-check.md) says: a
  tile that stays alight through a tool call of five minutes, the word in
  `heartbeat --json`, the tile going out with `work stop`, no keepalive left
  behind afterwards — and, at some point, a machine that really slept.
- Commands for the to-dos of a project: the addresses are released, the
  contract stands in Minka's `authorization.md` and E-190.
- Commands for the check-ins of a project: released as well, the contract in
  `authorization.md` and E-191.
- Of the ninety-odd addresses the API offers, about half have no command. Four
  cards say which: the lists and groups of to-dos, the to-dos and their
  subtasks, the check-ins, and the archive of cards and posts.
- Extract the to-dos of a project live: whether `todoset` is the dock entry, the
  fields of a list, a group and a to-do, the two lists of what is open and what is
  done, the recordings of the archived and the trashed ones, whether a list on
  the shelf or in the trash answers for its groups and its to-dos, and what a
  place the account no longer has answers when it is asked for what fell with it.
  Then check the bundle record by record, as the boards and the chats were
  checked.
- Write the bundle of the whole account, once Robert or Shaun has allowed its
  size: a dry run over it goes through and names what it would fetch. What it
  leaves out of the words a bundle has no place for is the one kind of line and
  nothing else, so the next surprise is the first one nobody has met yet — read
  `recordsLeftOut` of that run as well, and whoever gives a bundle a word for
  something has to have seen what Basecamp sends under it.
- What the check-ins still owe is only the rare shapes: the dry run over the
  whole account met no question without a `start_date`, no answer without a
  `group_on`, no frequency outside the five and none whose schedule Minka cannot
  express, so those four wait for an account that holds them rather than for a
  run.
- React to messages from `minka-ca`: check against the code, fake first,
  then the CLI, test and sabotage proof, check live, documentation in the
  same commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
