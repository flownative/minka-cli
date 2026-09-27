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

The Docs & Files of a project are read with `docs tools`, `docs list`,
`docs show` and `docs uploads show`; `docs create` and `docs edit` write a
document, and `docs comments …` write under a document or an upload. The rule
the commands follow is [Docs & Files](../stable/docs-and-files.md) (C-067,
C-068). Putting a file into a tool and changing an upload wait for a card of
their own, and everything the API does not offer — deleting, archiving and
moving an entry, folders, the archive of a tool, drafts — is named there and
nowhere pretended. None of it has been seen live: the whole feature is checked
against the fake, see below.

`minka import basecamp` extracts the structure, the boards, the message
boards, the chats, the check-ins, the to-dos, the forwarded mail and the
Docs & Files of Basecamp
projects into an
import bundle for Minka's applier: people with avatars, projects with members,
the boards, chats, message boards, questionnaires, to-do sets, inboxes and
Docs & Files tools as tools, of
each board the columns, cards, steps, comments and boosts, of each message
board the posts of every status with their categories, comments and boosts, of
each chat its lines as messages with their boosts, written in parts of one
chat and some 10,000 lines each, of each questionnaire its questions with the
schedule Minka has words for and their answers with what hangs on them, of
each to-do set its lists, the ones in use from the bottom up because Minka puts
every new list on top, with their groups and their to-dos in the order of the
list and what
hangs on a list and a to-do, of each inbox its forwarded mail with the
answers by mail, the comments and the boosts of each, and of each Docs & Files
tool its folders, documents and uploads with the comments, the boosts and the
files of each.
The Docs & Files tools come last of all, so that the bytes of a file standing in
a card and in a document alike are fetched for the card, and the inboxes before
them. Every owner of a file has a record of its own in `attachments.jsonl`, named
after the file and its owner, with the same bytes and one download (C-066): a file
stands in one place in Minka, so a record naming another's file is shown none of
it where the two stand in different projects.
`filesOfSeveralOwners` counts the owners beyond the first, met and not written:
what it names costs a duplicate and loses nothing.
Three things of such a mail are the extractor's own
doing, each
written down as a rule: a subject Minka would refuse becomes a title rather than
costing the mail or the run and a sender that is no address leaves the mail out
(C-056), a body that would reach Minka as nothing at all carries one line of the
run (C-057), and what the source keeps on its shelf comes into the trash, where
it can be brought back (C-058). Somebody who wrote to a project from outside the
account stays an address and becomes nobody.
Of the Docs & Files, three things are the extractor's own doing. A folder
stands in the bundle after the folder it is in; for an entry in use the order of
the file decides, bottom up, because Minka puts every new one on top of its kind,
and for one that has left its place it decides nothing (C-065), so those stand
first as the source lists them. The status of an entry is its own and never that of
its folder, so what stands in a folder the source put away comes in as it is
and goes along through the folder there. And the title of an upload is what the
source calls it, the name of its file standing in and folded to fit where the
source calls it nothing and the name is one Minka would refuse (C-056),
because Minka derives nothing from a file and refuses an upload without a
title.
Every address a bundle carries — a person's, a mail's sender, an answer's —
is read the way Minka reads it and no wider (C-059), so that none of them
stops the applier after hours of walking, and one pair of quotation marks
around the whole of an address comes off before it is read: the one place a
run repairs what the source says instead of passing it on, counted per record
in `addressesUnquoted`.
That rule is held against Minka's own reader over 193,531 inputs — the seams of
the rule, a combinatorial sweep, byte mutations of addresses that pass, random
strings — compared on what Minka makes of what a bundle would carry and not of
the input: no case the CLI takes and Minka refuses, and the narrow differences
are the three C-059 names.
A record the source describes with a word a bundle has none for is left out and
counted, and the run goes on (C-050): a chat line of an unknown kind — a bundle
has a word for `Chat::Lines::Text`, `Chat::Lines::RichText`,
`Chat::Lines::Upload` and `Chat::Lines::Integration` — or with a status other
than `active`, a column of a kind a bundle has no role for, a card, a post,
a comment, a question or an answer with a status a bundle does not know, a draft
among them, a group or a to-do the source has put on the shelf, which a
bundle gives none, a forwarded mail of a status that is none of the three,
an answer by mail that says anything but `active`, and a folder, a document or
an upload of Docs & Files whose status is none of the three. An upload whose
file can never be had is left out with its comments and its boosts
(C-063): an upload has no purpose without its file, and no run will ever have
bytes for what lies in a cloud, for what the source no longer has, for what
weighs more than Minka takes or for a download Basecamp refuses for good. What
tells such a refusal from a failure that may pass is the one word the reason
carries about itself, `retryable`: a file that may still come leaves the upload
written and waiting, and the run after this one brings both. That word decides
the exit code too, and nothing else does (C-025): a run whose missing files will
all never come ends 0 and names them, and one that lost a file which may still
come fails, because the exit code is there to say whether running again helps.
What stood in a list, a group, a folder or a whole Docs & Files tool a bundle
cannot carry falls
with it, is asked for to be counted and is named with the place it fell with
(C-052), so that no run says a bundle holds more of the source than it does. A
to-do that hangs on a To-dos tool itself stands in no list a bundle can name, so
it is left out, counted and named with its project (C-060), and the to-dos of
every tool that is switched on are asked for under the tool's own identifier for
that reason.
`recordsLeftOut` names each with the kind of record, the word and where it
stood, so one exotic record does not cost the account. What still ends
a run is what would be written wrongly: times that are missing or are none, a
second Triage, Not now or Done on one board, the status of a project the run was
asked for. Where a chat line is brought only by a later run and another line
carries the same second, the order of the two can turn round, which is a known
limit and written down with the rule.
What the source says about clients comes with a bundle (C-055): `client` and the
company on a person, `clientAccess` and the client company on a project, and
`visibleToClients` on a tool, a post, a to-do list, a check-in question and
every folder, document and upload of Docs & Files, read
from each record and never from the tool it stands in. A record that follows what
it hangs on carries no word of its own and is counted where the source offers
one. **An entry of Docs & Files is the one exception** (C-064): it carries the
word at every depth, because Minka reads it below the top of a tool as well and
something hidden there hides the folder at the top with everything in it.
A client is no member of a project in Minka, so the run counts the
memberships that fall away and says no question is asked of a client. A company
the source names without an identifier, or with a name Minka refuses, ends the
run where the record that names it comes in: written away as none, it would tell
Minka to drop the company it holds.
Every kind of tool Minka has a counterpart for is extracted, so no report names
a gap about clients; an Email forwards tool carries its word like
every other tool, and a mail and an answer by mail follow it.
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

- Everything of `docs`: no request of it has gone to a real Minka. The fake
  mirrors the contract read in Minka's own code — the order the controller
  refuses in, the plain-text fallback of a body without served markup, the
  moment kept to the whole second, `documentArchived` for an entry under an
  archived folder, the inside of an archived folder listed as archived, and
  the draft that is at no address. Two things it deliberately does not
  mirror: a second write within the same second, because every write of the
  fake stands a second after the one before so that a guard has something to
  tell apart — which is the one case the guard cannot see live — and
  `PATCH files-uploads/{id}`, which no command reaches. The fake asks
  `richtext.CarriesMarkup` for the fallback, the same rule the CLI asks, so
  the edges of the list of served elements are shared knowledge and not a
  second measurement.
- Everything a bundle says about clients: no extraction has met a client live.
  What the fields are and how Minka reads them was read in its appliers
  (`PeopleApplier`, `ProjectsApplier`, `ToolsApplier`, `WhatClientsSee`), and
  what Basecamp answers was measured read-only against the live account before
  the code was written, not live through a run of the CLI.
- That being a client comes before the reasons which attach to the person,
  having left and coming without an address, and after the one which attaches to
  the project: the person three of the four reasons hold for is an invented one.
  The account says `client: true` of a kind of person who is not retired and of
  no other, so nothing live has ever put the client case against the rest. Held
  against the order Minka takes a client out of a project in.
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
  `invalidAddresses`, and that a person's address inside a pair of quotation
  marks comes into the bundle without them: the account holds such an address,
  but on nobody the projects extracted refer to, and it holds nobody retired.
  A run that is killed was not seen live either. Seen live: a missing Basecamp
  CLI, masked addresses ending the run, a missing project as `not_found`, people
  of the account being left out, and files that could not be fetched, with the
  bundle written without them and the run ending as `network`, and a 525 of a
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
  Two shapes of the real account were measured all the same, read-only and
  **without a run of the CLI**, and the extractor rests on that measurement: a
  to-do whose `parent` says `Todoset` instead of `Todolist`, of which the whole
  account holds six in use, in three projects and across four to-do sets, none of
  them archived or trashed, the parent being the tool's own identifier, and which
  Basecamp hands out at `buckets/{b}/todolists/{todoset}/todos.json` although no
  `todolists.json` of the tool names that identifier; and `repetition_schedule`,
  which twelve to-dos of the account carry and the other 3,244 leave out
  altogether rather than sending an empty one, with the same ten fields every
  time. How Basecamp answers that path is measured as well: a to-do set that holds
  no to-do of its own answers 200 with an empty array and no error, one that holds
  lists with to-dos in them answers the same, and `?completed=true` does too,
  which is why a failure of that request ends the run where a place the bundle
  does not carry is passed over when it is gone. The path hands out the to-dos
  hanging on the tool alone and never those of its lists, so the run's reading of
  `parent.type` is a second hold on the same thing and not what carries it.
  An **active** to-do hanging on the to-do set of a tool that is switched off
  stays invisible, as an active list of such a tool already is: the walk never
  learns that tool's identifier, so the same gap holds for both. One on the shelf
  or in the trash is named all the same, because the records of a project name
  every to-do set of it, switched off or not. Where **every** To-dos tool of a
  project is switched off, the project is passed over whole and not even those
  are counted.
- Of the forwarded mail of `import basecamp`: no extraction has met an inbox
  live, and the shape of the source is measured all the same, read-only and
  without a run. Measured: that a dock entry of an inbox is
  called `inbox`, that its mail is listed at `buckets/{b}/inboxes/{i}/forwards.json`
  and the answers of one at `buckets/{b}/inbox_forwards/{id}/replies.json`, that
  a mail carries `subject`, `from` — the address alone, never a name —,
  `content`, `content_attachments`, `creator`, `created_at`, `status`,
  `replies_count` and `visible_to_clients`, and says how many comments and how
  many boosts it has as
  little as it says whom it was addressed to; that an answer carries `content`,
  `content_attachments`, `creator`, `boosts_count` and `status` as well, and
  that such a creator can be
  an `Outsider`; that an inbox lists its mail newest first, by `created_at`
  descending; and that `projects/recordings.json?type=Inbox::Forward` refuses
  the request while the status of the list is ignored, so **no mail of any
  status but `active` can be got out of Basecamp at all** and the shelf becoming
  the trash may never fire. Measured too, and the one measurement that says the
  run is needed: over 28 active projects, 9 inboxes that are switched on and 54
  forwards, four senders are addresses Minka refuses. Two of the four are the
  whole address written inside a pair of quotation marks, which the run takes
  off — about 4% of the mail of the projects swept — and two have a local
  part over 64 bytes and stay out whatever is done. So a mail left out for its
  sender is no invented case but one the first real run will meet, and writing
  such a sender would have ended the applier after hours of walking. The same
  sweep says what the account holds in the other direction, which is what it is
  worth here — that the rule takes nothing Minka refuses is held above, over a
  corpus built for it: the only over-strictness the account holds is a bracketed
  literal address of a machine and a top label of one letter, neither of them
  anybody's mail, and of the two only the bracketed one is still left out.
  What the run does with all that rests on the invented account, with three
  things held against Minka's own reading rather than against a
  test here: every address and every title the run would write was put through
  PHP's `filter_var`, the plain local part of `EmailAddress` and the three
  refusals of `IsName`, and not one of them is refused; and the bodies of a
  bundle were put through `MailLineBreaks` with the tag of the file that will
  never come taken out first, as the applier takes it out, and none of them
  comes out empty.
  Invented for want of anything to measure: a mail or an answer whose `status`
  says anything but `active`, and one that says nothing of its status, which
  Basecamp always names; a sender that is
  no address, of the shape the account holds and of the others, and one inside a
  pair of quotation marks, which is a shape the account holds; a mail
  without a subject and one whose subject is longer than a
  title holds; a mail whose body is nothing but a file the source no longer has,
  one whose file this run could not fetch, one above the ceiling of a run of
  Minka — for which the ceiling is lowered, a gibibyte fitting in no fixture —,
  one that says nothing, one that is nothing but a player, Basecamp having
  cleaned every body of the account down to text, links and files, and one that
  holds nothing but a NUL; a mail nobody
  is named for; somebody from outside whose address Minka would refuse and one
  the source names by their name alone; a mail that says nothing of how many
  answers it has; and a
  boost on a mail, of which the account holds none, so the list a run asks for of
  every mail has never answered with one.
  One number of the report will be larger live than the fake suggests: every
  mail of the account carries a word about clients, and every one of them is
  counted in `visibilityLeftToTheContainer`, where the fake offers one. Of the
  answers only one list was read, and what it says about clients was not among
  what was measured.
- Of the Docs & Files of `import basecamp`. What the bundle has to carry was
  read in Minka's own code — `FoldersApplier`, `DocumentsApplier`,
  `UploadsApplier`, `EntriesOfDocsAndFiles` and `WhatClientsSee` — and `E-195`,
  and read there rather than in Minka's prose, which said the opposite of the
  client rule until the code was read against it.
  **Measured live**, read-only, by dry runs over three projects that went
  through and by reading two upload records raw: that a dock entry of a Docs &
  Files tool is called `vault` and **is itself the vault at the top**, so what
  stands directly in the tool names the dock entry as its parent; that a vault
  answers for what stands in it at `buckets/{b}/vaults/{v}/vaults.json`,
  `documents.json` and `uploads.json`; that `projects/recordings.json` takes the
  types `Vault`, `Document` and `Upload`, which the Basecamp CLI offers for
  neither and its own list of types does not name; and that an upload carries
  `id, status, title, description, description_attachments, filename,
  content_type, byte_size, download_url, app_download_url, parent, position,
  visible_to_clients, inherits_status, comments_count, boosts_count, creator,
  created_at, updated_at, bucket, url, app_url, bookmark_url, subscription_url,
  height, width` — the whole key list, which the fake now answers with. Two of
  those uploads were ordinary stored files of up to 3.3 MB, and both carried
  `visible_to_clients` **below the top of their tool**, which is the field the
  whole client rule hangs on.
  Measured over the account besides: 18 archived folders over 3 buckets, 151
  active and 115 archived documents, and
  **nothing in the trash of any of the three kinds**; every one of the 28
  projects has a `vault` dock entry, 8 of them switched off, so "no Docs & Files"
  is a tool that is switched off and never a missing entry; a folder's own list
  hands out what is in use alone, so every record the source has put away comes
  from `projects/recordings.json`; an **archived folder answers not-found** for
  its three lists, which is the branch a run passes over and the normal case,
  not an exception; and Basecamp grows its page from 15 to 30, 50 and 100, which
  a walk that asks until a page comes back empty does not care about.
  **What that measurement caught**, and what it changed: the shape of an
  upload's `download_url` had been taken from the wrong record. It is
  `…/buckets/{b}/uploads/{id}/download/{name}` — the API's own address, no
  `blobs` segment and no key of a stored file anywhere — **one shape without
  exception over every upload of the account that was read**, and the
  `…/blobs/{key}/download/{name}` shape read off it belongs to a file inside a
  text, which carries a `key` of its own. So every upload took the branch for one
  that names no stored file, and a bundle of the account would have arrived with
  **no uploads at all** — the card's own measurement of the account puts that at
  some 278 files and 0.4 GB — counted and named and gone.
  What says an upload has a file is now the record's own word, an address to
  fetch one from, and the bytes are asked for **by the identifier of the upload
  and of its project** (`basecamp files download <id> --in <project> --out -`):
  the record names no stored file and its address answers with a redirect to a
  store of its own, which `checkStorageAddress` refuses and which no run follows
  — the query of that redirect carries the name of the file, so nothing of it is
  ever logged. The title is the source's own `title`, the name of the file
  standing in only where the source calls the upload nothing.
  **That route is proven live and by more than a run's own word.** Two
  extractions into bundles outside the repository ended 0, every upload's bytes
  arrived with the size and the digest they were expected to have, computed anew
  rather than taken from the run, and one file was fetched again by hand and came
  back byte for byte the same; the second bundle holds 214 files and 496 MB, with
  none missing, none gone and no request asked twice. `--agent` is taken by
  `files download` although its own help does not list it, which is written down
  where the call is built so that nobody removes it as a mistake. Every reference
  of both bundles resolves, Minka's own `verify-bundle.sh` ends 0 on both, and the
  status of every entry is its own with not one inheriting its folder's. The two
  archived folders of those projects really did answer not-found for their
  contents and their archived documents came out of the recordings list, so the
  path the walk was built for is the path the source takes.
  **The client rule is a live measurement and no longer a rule we believe.** A
  third extraction went over the one project of the account that can answer the
  question — four of its projects work with clients and only that one carries a
  `visibleToClients: true` at all, entries below the top of a tool, and a tree
  with both words in it. Exit 0, 437 seconds, 202 files, nothing left out. Of its
  20 records of the three kinds — 1 folder, 9 documents, 10 uploads — **every one
  carries the field and not one lacks it**, at both depths, the four that stand
  inside a folder included: 4 `true` and 12 `false` at the top of a tool and 4
  `false` below it, each compared one by one against the source's own
  `visible_to_clients` with **no difference at all**. Nothing the source shows is
  written as hidden and nothing it hides is written as shown.
  And the shape the whole card exists for is one the account holds: both Docs &
  Files tools of that project are shown to clients at the source, and one of them
  holds sixteen entries of which **every single one is hidden** — the folder and
  its four uploads among them. So a tool the source shows, holding entries the
  source hides, is no fixture: the run took the record's own word every time and
  the tool's never once.
  Measured with it: `clientAccess` and the client company on the project, exactly
  two people with `client: true` and exactly those two carrying a company, all 211
  digests matching, every reference resolving, and `verify-bundle.sh` green.
  The naming of an entry inside a folder that says nothing about clients found
  **nothing to say** on that project, every record below the top carrying the
  field at the source: a safeguard that has met the real world and come back
  empty, which is what it is for.
  **Minka has applied both.** On a worktree instance, exit 0, everything created —
  one folder with two documents and four uploads, and four folders including the
  two archived with sixteen documents and one upload — and on a second and a third
  run everything is passed over, so the import is idempotent against what a bundle
  of ours says. The blob storage grew by 23 and 359 files. Both bundles are
  deleted, as the rule for a bundle holding personal data asks.
  **Still taken on trust of Basecamp** and held by nothing but the invented
  account: that a vault lists what stands in it topmost first, which is what the
  order of a bundle turns on; that an upload in a cloud names no file at all,
  which **no record of the account has ever exercised** — the branch stands, and
  nothing has disproved or proved it; that a folder takes neither comments nor
  boosts; and the flags of the call that streams an upload's file, which are read
  off the Basecamp CLI's own documentation and not off a run.
  **What no live run has met yet**, and so still rests on the invented account:
  a **folder inside a folder** — none of the three projects has one, so a parent
  standing before its child is only trivially exercised live; an **entry in use
  inside a folder the source has put away**, the client project holding nothing
  archived or trashed of the three kinds at all; and a hidden entry below a
  **shown folder**, the only folder there being hidden itself, so the case of a
  container that is shown holding something hidden was met one level up, at the
  tool, and not at a folder.
  **Which commit the live measurements rest on.** The three bundles were made
  from this branch while it was being worked on, the third from a dirty tree whose
  manifest names the extractor `v0.15.0-41-g69630d5-dirty`; that binary did not
  carry the naming of an entry that says nothing about clients. So what is
  measured above is the behaviour of the branch and not of its last commit, and
  one confirming extraction over that one project on the final commit is owed
  before the account is walked.
  **Which fixtures still encode a guess rather than a reading**, which is the
  list to read before the next run: a folder, a document or an upload whose
  status is none of the three; **anything in the trash** of the three kinds, the
  account holding none; an upload in a cloud, with a comment and a boost on it;
  a title longer than one Minka takes and an upload with neither a title nor a
  name — the titles of both live bundles were put through Minka's rule again and
  none would be refused, and neither project exercised the folding at all,
  `namesMadeIntoATitle` and `titlesFromTheFileName` both coming out zero;
  a document whose whole text is a file the source no longer has, and an
  upload whose own file is one; a document and an upload whose file Basecamp
  refuses the download of, which is a shape the account holds although no
  fixture is a reading of it: the refusal is played by a stand-in, and what a
  run makes of it is held by tests alone; a description with a file embedded in it, every
  upload of the account that was read carrying an empty
  `description_attachments`; a
  document and a description longer than Minka holds; an entry the source
  lists twice; a folder the bundle cannot carry with a folder, a document and
  an upload standing in it; a Docs & Files tool that is switched off with an
  archived folder and an archived document behind it; a folder in use behind
  such a tool, which is the only way to reach the count of an entry no place
  ever claimed; and a vault the source lists **without a parent**, which is how
  it lists the vault at the top of a tool and which no record it has put away is
  in, no whole tool of the account being put away; and **a record that says
  nothing about clients**, which is the shape the naming of a silent entry and
  three of the fixtures stand on — the one project that could answer it carries
  the field on every record at both depths, and no other project's Docs & Files
  records were read, so an absent `visible_to_clients` has never been seen and
  the fake invents it. A document or a description longer than Minka holds is
  made by a test rather than standing in the fake, which is the same want of a
  reading by another road. What the fake answers with otherwise is the shape that
  was read: the key list of an upload, the address it
  gives, the not-found of an archived folder, and a folder's own list holding what
  is in use alone.
  **A file Basecamp refuses for good** is the one shape of this whole tool that a
  full extraction of the account has met, exactly once over 10,274 files: the
  download of one upload's file answered with a refusal that will not pass.
  An upload whose file no run will ever bring is left out and named in
  `docs.onlyAFileNoRunWillBring`, with its comments and its boosts counted as
  fallen, and that holds for all four ways of having no file: one in a cloud,
  one the source no longer has, one above what Minka takes, and one refused for
  good. Where the file left a record behind, that record stays as the trace of
  what became of it. What the run does with such a file rests on the invented
  account, where every one of the four is played by a stand-in; what says a
  refusal is one is `retryable` of the reason, and no other reading of a failure
  is anywhere in the way. The residual of the cloud branch falls under the same
  rule: **an upload that lives in a cloud and does carry a `download_url`** is
  taken as having a file and the fetch fails, and one that will not pass leaves
  the upload out and named. Nothing has been seen of such a record either way.
  A **dry run** counts such an upload all the same, because nothing the source
  answers says that a file is gone or that a download will be refused: it says
  one upload too many rather than promising a file it never asked for.
  **What a walk of the account is to expect of the exit code**: that one file
  ends the run **0**, with a notice naming it and the record that fell with it.
  A file no repeat can bring is a result and not a failure, and only a missing
  file that may still come fails the run (C-025). Read
  `docs.onlyAFileNoRunWillBring` and the entries of `missingFiles` that say
  `retryable: false`, and settle those at the source.
  Not counted and not asked for: how many earlier copies of its file an upload
  has. `versions` is a field Minka reads nowhere, and the upload says nothing
  of them, so a run that wanted the number would ask once per upload. That
  **eight** files of the account have older versions is a number of the card's
  own read-only measurement of the account and of no run of the CLI; nothing
  here measured it and nothing here can.
- The titles of the Docs & Files are held against Minka's own reading rather
  than against a test here, as the addresses and the titles of the mail were:
  every folder name, document title, upload title and upload file name of the
  account — 1,040 values over 753 records — was put through `IsName`, and **not
  one is refused**, the longest of any kind being 99 characters against the bound
  of 200 and the shortest 2, so nothing sits near either edge. The title of a
  folder or a document ends a run where Minka would refuse it, which is right and
  which nothing in the account will meet; a title a **file** carries is folded
  and ends no run (C-056). The trashed half of all three kinds is not unasked but
  empty at the source.
- Of the files of `import basecamp`: a download that breaks off, a file
  whose size differs from what Basecamp says, a file that is gone, a name that
  has to be changed, a caption too long, and one file under several owners — its
  record per owner, the bytes shared by a hard link, a gone and an unfetchable
  file under all of them, and a later run taking a duplicate over. The one
  cross-owner reference a real bundle has shown was measured before the records
  were written: a post of project 38147508 naming a file of another post, which
  the run counted and did nothing about.
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
  `ForgetsTheArchiveBoundariesOf` for the 422, which takes the card out of the
  archive with the refusal because one status decides the cursor and the listing
  alike, and `RacesOnTheArchiveBoundariesOf` for the empty 200 of the race, where
  the card stays and the same card named twice is that window met twice. What the fake
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
- Of the eight kinds of record whose word about clients falls to what they hang
  on, the fake produces two: a group and a to-do. A card, a comment, a chat line,
  a column, a step and an answer go through the same one place
  (`Extractor.record`), so the rule is held for them by that and by nothing of
  their own.
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
- Minka's `import-format.md` says of one identifier under two owners that the
  extractor writes one record with the owner of the first place it found it,
  which is what C-066 replaces. The format is Minka's and only its side can
  change that sentence, so it is a question for the coordination.
  Three things about the loss were read in Minka's code rather than believed.
  `ownedBy` is asked only of a body that embeds no file at all, so it is not what
  loses a file of ours, and a body that names the file resolves the tag by the
  reference alone. What loses it is the **circle**: `Attachments::attachTo` asks
  `DbalAttachmentRepository::attachableBlobs`, which reads the blob rows of the
  circle alone (`project_id = :circle`), so a blob of one project is never
  attached to a place in another and the renderer drops an attachment that hangs
  in no text. `onlyWhatThisCircleHas` does **not** do it: it runs on the text
  while the handles of the run are still held back as plain tokens, by design.
  So the loss is a record standing in **another project** than the first owner,
  and within one project a bundle of ours was losing nothing.
  Nor does such a record **wait**: `blobOf` answers a known reference without
  asking about the circle, so the tag becomes markup, the body does not read as
  empty, and the record is written and shows nothing. Waiting is what a file that
  never comes at all causes.

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
- Extract the forwarded mail of a project live, and check the bundle record by
  record as the boards and the chats were checked. What is still open there is
  what no reading of the source answers: what a body looks like that the run has
  to write a line of its own into, how the mails whose sender Minka refuses read
  in the report, and what `addressesUnquoted` comes out at. The sweep of the
  account counted the senders of mails alone and found two, while the number
  counts a record at a time and takes in the senders of answers and the people
  as well, so a larger number is no sign of anything wrong. Whether a mail on the
  shelf exists at all is a question for Basecamp and not for a run: nothing the
  API offers hands one out.
- **One confirming extraction of the Docs & Files on the final commit**, over the
  one project that works with clients and carries both words, before the account
  is walked for MNK-53. Three bundles are checked record by record already, the
  client rule among them, but they were made from the branch while it was being
  worked on and the third from a dirty tree, so what is measured is the branch's
  behaviour and not this commit's. Seven minutes turns the one into the other.
  Open besides, and none of it settled by that run: whether the account holds an
  upload in a cloud at all and what such a record looks like; whether a vault
  really lists what stands in it topmost first, which the order of a bundle turns
  on; and the three shapes no project walked so far holds — a folder inside a
  folder, an entry in use inside a folder that was put away, and a hidden entry
  below a folder that is shown.
- Extract the to-dos of a project live: whether `todoset` is the dock entry, the
  fields of a list, a group and a to-do, the two lists of what is open and what is
  done, the recordings of the archived and the trashed ones, whether a list on
  the shelf or in the trash answers for its groups and its to-dos, and what a
  place the account no longer has answers when it is asked for what fell with it.
  Two shapes were measured at the account and have still never come through a
  run: how the six to-dos that stand in no list read in `todosWithoutAList`, and
  the twelve schedules in `todosThatRepeat`.
  Then check the bundle record by record, as the boards and the chats were
  checked.
- Write the bundle of the whole account, once Robert or Shaun has allowed its
  size: a dry run over it goes through and names what it would fetch. What it
  leaves out of the words a bundle has no place for is the one kind of line and
  nothing else, so the next surprise is the first one nobody has met yet — read
  `recordsLeftOut` of that run as well, and whoever gives a bundle a word for
  something has to have seen what Basecamp sends under it.
  Read `filesOfSeveralOwners` of that run too, and check the bundle the way the
  [run guide](../guides/import-run.md) now asks: who owns a file and not only
  whether it is found. One project showed one such file; 8,321 files over 32
  projects will show more, and how many bytes the duplicates cost is a number
  nothing here can guess.
- What the check-ins still owe is only the rare shapes: the dry run over the
  whole account met no question without a `start_date`, no answer without a
  `group_on`, no frequency outside the five and none whose schedule Minka cannot
  express, so those four wait for an account that holds them rather than for a
  run.
- React to messages from `minka-ca`: check against the code, fake first,
  then the CLI, test and sabotage proof, check live, documentation in the
  same commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
