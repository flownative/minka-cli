# Status

The first look of every new session. What holds is under `stable/`; here
stands where the work currently is.

## Where we are

The CLI serves what `/api/v1` offers: projects, boards, cards including the
trash and the archive of a board, columns with their area for cards on hold,
assignees, due dates, steps, subscribers, comments, chats, private
conversations, uploads, mentions, reactions and one's own cards across all
projects. `minka commands` lists them all, built from the command tree itself.

The columns of a board are written with `columns create`, `edit` (title and
colour, `--no-color`, a system column's title given back with `--title ""`),
`move` and `delete`, which takes the cards of the column into the trash with
it. The rule the commands follow is [Columns](../stable/columns.md) (C-102,
C-086). Minka serves the column routes from v0.52.0+1 on. What Minka's #331
changed is on its `main` and in no release yet: `"color": null` taking a
colour away, a client's 404 on every column write,
`neighbourColumnOnAnotherBoard`, and a no-op move refused in an archived
project. Until a release carries it, a released Minka answers `--no-color`
alone with `nothingToChange` and keeps the colour beside `--title`, which the
CLI reports as such; it tells a client of a system column, the archive or a
bad title before the 404, answers a client's no-op with 200, and puts a
column next to a neighbour on another board first. The commands are checked against Minka's code and tests and the
fake only, see below.

The comments on a card are read with `cards show`, `comments list` and
`comments unread`, which shows only what this machine has not shown yet. A
comment can wait for them: `comments create --if-latest <commentId|none>`
writes only while that comment is still the newest, and the setting
`comments.requireRead` (`minka config set`) refuses a comment while the card
has comments not yet shown; both exit 11 (`unread`) with those comments in
`data`. The rule the commands follow is [Comments on a
card](../stable/comments.md) (C-101). The guards are cards only. Minka's side
is what it already serves; the commands are checked against its code and the
fake, not live.

The archive of a board is read with `boards archive`, and `cards archive` and
`cards unarchive` move a card in and out of it. `--all` walks every page in one
run and works around the weakness Minka carries in its own debts: a cursor that
counts only while the card it names lies in the archive. The rule the commands
follow is [The archive of a board](../stable/archive.md) (C-054). None of it has
been seen live: the whole feature is checked against the fake, see below.

A file goes both ways: `uploads create` puts one in, `uploads download`
fetches its bytes back. Every text says which files hang in it, so reading a
card, a comment or a message gives the identifier a download takes, without
anything being taken out of the text. `uploads download` sends no `?download`,
which changes the headers and not the bytes (C-121).

`messages create --reply-to <messageId>` answers an earlier message of the same
chat, and `messages list` shows which message a reply answers. `chats show`
reads one chat of a project by its ID alone, and `todolists tool`, `posts
tool`, `docs tool`, `checkins tool` and `events tool` one tool of their kind
(C-120). `auth logout --revoke` revokes the token of the entry on the
server before it takes the entry out, and keeps the entry when that fails
(C-119). Minka serves all of it from v0.53.0 on (replies from v0.26.0 on); the
commands are checked against Minka's code and tests and the fake, not live.

The Docs & Files of a project are read with `docs tools`, `docs list`,
`docs show` and `docs uploads show`; `docs create` and `docs edit` write a
document, `docs uploads create` puts a file into a tool — both requests Minka
takes it in, in one run, or the second alone with `--blob` — and `docs uploads
edit` changes the title and the description of an upload; `docs comments …`
write under a document or an upload. The tool names its project, so a file
finds its project through the tool, and `--notify` of `docs create` and `posts
create` and `--ask` of `checkins create` take names and e-mail addresses
looked up there; `--project` of `docs uploads create` is hidden and checked
(C-092). The rule the commands follow is
[Docs & Files](../stable/docs-and-files.md) (C-067, C-068, C-085, C-092).
Everything the API does not offer — deleting, archiving and moving an entry,
folders, the archive of a tool, drafts, making an earlier version current — is
named there and nowhere pretended. None of it has been seen live: the whole
feature is checked against the fake, see below.

The versions of a document are read with `docs versions list` and `show`, the
files of an upload with `docs uploads versions list`, and `docs uploads
versions create` gives an upload another file in the two steps of `docs
uploads create`, keeping the old one as a version (C-099). Minka's side is on
its `main` and in no release yet, so production gets it with the next Minka
release; the commands are checked against Minka's code and tests and the fake,
and live only in what addresses that name nothing show, see below.

The to-do lists of a project are read with `todolists tools`, `todolists list`
and `todolists show`, and written with `todolists create`, `edit`, `archive`,
`unarchive` and `delete`; the groups of a list with `todogroups create`, `edit`,
`move` and `delete`, a group with its colour (`--color`, C-086). The rule the
commands follow is [To-do lists and groups](../stable/todo-lists.md) (C-071,
C-072). None of it has been seen live: the whole feature is checked against
the fake, see below.

The templates of the organization are listed with `todolists templates`, and a
list is made from one with `todolists start --tool <toolId> --template
<templateId> [--title …]`, whose answer names in `leftOff` the people it did
not carry (C-110). A template is changed with the commands of a list; a day or
a tick there is refused, and the hints of `todos` say so. Minka's side is #344,
on its `main` and in no release yet, so production gets it with the next Minka
release; the commands are checked against Minka's code and tests and the fake,
and live only in what reads and addresses that name nothing show, see below.

The to-dos are read with `todos show`, `todos completed` for the done ones of a
list and `todos mine` for one's own across all projects, and written with
`todos create`, `edit`, `complete`, `reopen`, `move` and `delete`; their
subtasks with `todos subtasks list`, `create`, `edit`, `complete`, `reopen`,
`move` and `delete`. The rule the commands follow is
[To-dos and subtasks](../stable/todos.md) (C-073 to C-076).

The comments under a to-do and under a list are read and written with `todos
comments` and `todolists comments` (`list`, `create`, `edit`, `delete`, `react`,
`unreact`), built from the same commands as `posts comments` (C-095). The API
is in production from Minka v0.29.0 on; the commands are checked against Minka's
code and tests and the fake only.

None of the to-dos has been seen live: the whole feature is checked against
the fake, see below.

A to-do with a due day is made to repeat with `todos repeat`, and its series is
read with `todos series list` and `todos series show`, and written with `todos
series edit`, `stop`, `resume` and `show-now`; `todos show` and `todolists show`
name the series of an occurrence. The rule the commands follow is
[Repeating to-dos](../stable/todo-series.md) (C-089). The API came with Minka
v0.41.0: a read-only `todos series list` has answered on `minka.flownative.com`,
and everything else, every write above all, is checked against the fake only,
see below. The notes of a series — `todos series edit --notes` with the guard
over files, and `notesSource` and `attachments` in every answer — are on
Minka's `main` and in no release yet, so production gets them with the next
Minka release; they are checked against Minka's code and tests and the fake.

The check-ins are read with `checkins tools`, `checkins list`, `checkins show`
and `checkins reminders`, and written with `checkins create`, `edit`, `pause`
and `resume`; their answers with `checkins answers list`, `show`, `create` and
`edit`. The rule the commands follow is [Check-ins](../stable/checkins.md)
(C-077 to C-080). Putting a question into the trash, stopping to be asked,
removing an answer and the comments under an answer have no address in the
API. None of it has been seen live: the whole feature is checked against the
fake, see below.

The posts are read with `posts tools`, `posts categories`, `posts list` (and
`--archived` for the archive of a tool) and `posts show`, and written with
`posts create`, `edit`, `pin`, `unpin`, `archive`, `unarchive` and `delete`;
their comments with `posts comments list`, `create`, `edit`, `delete`, `react`
and `unreact`. The rule the commands follow is [Posts](../stable/posts.md)
(C-081). The guard over files stands aside wherever Minka refuses a text
anyway, for every text (C-082). Drafts, taking a post out of the trash,
reacting to a post itself and showing one to clients have no address in the
API. None of it has been seen live: the whole feature is checked against the
fake, see below.

The calendars of a project are listed with `events tools`, their events read
with `events list` (a span of days, with the occurrences of every series in
it) and `events show`, and written with `events create`, `edit` and `delete`;
their comments with `events comments list`, `create`, `edit`, `delete`,
`react` and `unreact`. The rule the commands follow is [Calendars and
events](../stable/calendar.md) (C-098, C-095). Taking an event out of the
trash and showing one to clients have no address in the API. Minka's side is on its
`main` and in no release yet, so production gets it with the next Minka
release; the commands are checked against Minka's code and tests and the fake,
and live only in what a read of addresses that name nothing shows, see below.

Beside its events a calendar's span carries what of its project is due,
`datedItems`, unless its team chose `eventsOnly` with `events settings`; `events
mine` reads My calendar, the events one takes part in and what is assigned to
one and due, steps included, across one's active projects (C-106). The text
views put what is due under a heading of its own. Minka's side is on its `main`
(#319, and #327, by which a calendar's span, My calendar and `cards mine` leave
out the same cards and order a day's items alike) and in no release yet, so
production gets it with the next Minka release; until then a released Minka
still lists a card completed outside Done under `cards mine`. `events mine` is
seen live, `events settings` and the dated items of a calendar only against
the fake, see below.

One's own reminders ("Remind me") are read with `reminders list` and written
with `reminders set <kind> <item> --preset … | --at …` and `reminders delete
<kind> <item> [--outcome done|dismissed]`, of an item, a comment or a line of
a project chat. The rule the commands follow is
[Reminders](../stable/reminders.md) (C-113). Minka's side (#358, and #359,
which reads a moment in RFC 3339 only; E-252) is on its `main` and in no
release yet, so production gets it with the next Minka
release; the commands are checked against Minka's code and tests and the fake,
and live only in a read, see below.

A project is renamed and described with `projects edit`, which sends only
`--name` and `--description` given, the description as plain text. The rule
the command follows is [Projects](../stable/projects.md) (C-093). Minka's side
(`PATCH /api/v1/projects/{projectId}`) is on its `main` and in no release yet,
so the command is checked against Minka's code and tests and the fake only,
see below.

A reference to a card, a post, a chat, a chat message, a to-do, a to-do list,
a document, an upload or a question in any text is read as its address,
`<href>`, which comes back as the reference when the text is edited, or as
`(a card you cannot open)` and its like where the reader may not see the
target (C-091);
the guard over files counts references, `messages edit` included, `--json`
keeps the served HTML with title and short identifier, and the skill tells
agents to name a card as `#MNK-1` or by its URL. Minka's side
is on its `main` and in no release yet, so production sends no `<minka-ref>`
with its additions: the rendering is checked against Minka's code and tests
only, and the fake stores rich text as sent and makes no references of its
own.

`minka import basecamp` extracts the structure, the boards, the message
boards, the chats, the check-ins, the to-dos, the forwarded mail and the
Docs & Files of Basecamp
projects into an
import bundle for Minka's applier, every project the identity sees, the
archived ones included, unless `--project` names some or `--active-only`
leaves the archived ones out (C-088): people with avatars, projects with members,
the boards, chats, message boards, questionnaires, to-do sets, inboxes and
Docs & Files tools as tools, of
each board the columns, cards, steps with their people and due day, comments
and boosts, of each message
board the posts of every status with their categories, comments and boosts, of
each chat its lines as messages with their boosts, written in parts of one
chat and some 10,000 lines each, of each questionnaire its questions with the
schedule Minka has words for and their answers with what hangs on them, of
each to-do set its lists, the ones in use from the bottom up because Minka puts
every new list on top, with their groups and their to-dos in the order of the
list, a to-do that repeats with its schedule (C-090), and what
hangs on a list and a to-do, of each inbox its forwarded mail with the
answers by mail, the comments and the boosts of each, and of each Docs & Files
tool its folders, documents and uploads with the comments, the boosts and the
files of each. With `--upload-versions` it brings an upload's earlier files
as its versions, fetched at each version's address with the Basecamp CLI's token
on Basecamp's API alone (C-104); without it no token passes through minka. With
`--calendars` (C-096) it brings each project's calendar as
well, its entries as events with their comments, boosts, files and followers,
a series as one event with its schedule; without it a calendar is skipped,
because a Minka whose import does not apply events stops at one. No request of
a run reaches an address of a single day of a series or of one entry, which
Basecamp answers by writing (C-097). With `--keep-files` (C-115) a run keeps
every file it brings beside `--out`, so that one that fails, is stopped or is
killed hands them to the next run with the flag, which walks Basecamp anew and
fetches only what is missing; a bundle in place removes them. With `--agent
<agent ID>=<human ID>` (C-116, temporary, for moving Flownative's own account
over) a user comes as the agent of another, without an address, and the human
comes along.
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
Every title and name a bundle carries is made into one Minka takes and named
in `titlesMadeToFit` rather than ending the run, a to-do that starts after it
is due comes without its start, and an event that ends before it starts is left
out (C-111): a walk of hours ends on nothing somebody typed. A comment or a
chat line loses the control characters Minka refuses, a project description
over 2000 characters is cut (C-112); a category of posts without an icon
comes without an emoji (C-114).
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
weighs more than Minka takes or for a download Basecamp itself refuses. Whose
word a missing file is says which of the three it is, and `why` of its entry
carries it: a refusal of the source leaves the upload out, while a failure that
may pass and one this run could not place both leave it written and waiting,
because neither says anything about the file. That word decides the exit code as
well, and nothing else does (C-069): a run that could not place a failure ends
7, one that lost a file which may still come ends 6, and one whose missing files
Basecamp refused every one of ends 0 and names them, the count in the `summary`.
A guard of the run refusing an address is the first of the three and never the
last: a reading of the source that is wrong by one shape costs an exit code and
a later run, and never an account written off as lost.
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
the source names without an identifier ends the run where the record that names
it comes in: written away as none, it would tell Minka to drop the company it
holds. A name Minka would refuse is made to fit (C-111).
Every kind of tool Minka has a counterpart for is extracted, so no report names
a gap about clients; an Email forwards tool carries its word like
every other tool, and a mail and an answer by mail follow it.
Mentions in the texts are references, the preview of a link is the link, a
video or a player embedded in a text is an `<import-embed>` with its address,
a link to a card, a message, a chat or an entry of a calendar of the account,
a day of an entry naming the entry, is an `<import-reference>`, made from the
address alone and with or without `--calendars`, and the files of the texts
and the files dropped into a chat come with a record and their bytes,
streamed from the Basecamp CLI or taken over from the bundle the run
replaces. It reads through the Basecamp CLI and is tested against the
invented account in `internal/fakebasecamp`.
A post, a card, a to-do, a document and an upload say who follows them at the
source (`subscribers`, C-084), which Minka takes as the whole truth. The list is
read per record from Basecamp's subscription of the recording and names only the
members Minka can put on the item; `subscribers.leftOff` counts the rest by
reason. A record in the trash is not asked, and one whose list Basecamp answers
as not found or forbidden comes without the field, named in
`subscribers.unread`, so Minka follows it as before instead of taking everybody
off it. It costs one request per record.
A group of a to-do list carries its colour as Basecamp names it, `white`
included, which is how a colour taken away at the source is taken away in Minka
(C-086). Every group of the account seen so far said `white`; the other ten
names come from Minka's code, not from a coloured group seen live.

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
Chatbots and integrations come as people of `kind: "bot"`, without an
address or `client`, a deleted chatbot among them with `retired: true`
(C-094); the run report counts them in `bots` and names in
`botsAlsoNamedElsewhere` the deleted people who wrote as both.
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
Owed with it: a dry run that meets the lists of who follows an item. How
`subscription.json` answers was measured read-only, see below; a list longer
than seven people has not been seen.

A step carries the people on it and the day it is due: `steps list` and
`cards show` name them, and `steps edit` changes them, the people as a whole
set; `steps create` takes neither (C-100). The rule the commands follow is
[Steps](../stable/steps.md). Minka's side is on its `main` and in no release
yet, so the commands are checked against Minka's code and tests and the fake
only, see below; a Minka without it answers `steps edit` with
`stepCompletedMissing`, which the hint names. The extractor writes `dueOn` and
`assignees` on every step. The shape of a Basecamp step was read in the Basecamp
CLI's response cache of the account, not asked for: no run has carried the two
fields yet.

`steps edit --title` changes the text of a step in the same request as its
people and its day, and `steps move` puts it elsewhere on its card with
exactly one of `--top`, `--bottom`, `--after` and `--before` (C-123). Minka's
side is #390, on its `main` and in no release yet, so production gets it with
the next Minka release; until then a released Minka answers `--title` alone
with `nothingToChange`, passes over it beside other flags, which the CLI
reports as exit 7, and answers every move with 404, which the hint names. The
commands are checked against Minka's code and tests and the fake only, see
below.

The working notes on the back of a card are read with `cards notes show` and
written with `cards notes set`, which needs `--expect` or `--force` (C-042).
Seen live against the local instance, which runs Minka's main with the notes
not yet released: reading notes nobody wrote, the first write with `--expect
none`, a refusal as `notesChanged` for a stale time and for `none` over
existing notes with the notes as they stand, a write with the right time,
clearing with `--force`, and a 404 for an unknown card. The card resource
does not say whether a card has notes, so `cards show` only points to them.

An agent says which cards it has in hand: `work start` and `work stop` keep
one list per session, `heartbeat` reports the list of its own session to `PUT
/me/work` as a client of that session's own (C-103), and the Claude Code mod
calls it from inside the process of a session after every step and every
thirty seconds, `--idle` while no tool call runs (C-105, C-107), so a session
that ended goes dark three minutes after its last beat. `minka setup` installs
the mod after the sign-in (`--no-mod` leaves it out, a missing `claude` is
named and passed over), and `hooks install` installs it on its own (C-108),
both only into a Claude Code of 2.1.287 or later, the first that loads mods;
`claude --version` decides. Both take the settings hook an earlier minka wrote
out of `~/.claude/settings.json`, that entry and nothing else (C-109).
No process of minka's own reports between beats any more. The session comes
from `--session` or `$CLAUDE_CODE_SESSION_ID`; a store of one list per machine
is handed out to its sessions when it is read. A typed command waits up to 30
seconds for the lock on the list, the claim of a beat not at all.
The list, the refusals and the throttle are seen live, and the report with
them: on `minka.flownative.com`, where this CLI is signed in as an agent, `work
start`, `heartbeat` and `work stop` get through, and the settings hook reported
out of a session of its own. Robert's machine has the mod enabled. What nobody
has seen yet is `setup` installing it, or the old hook taken out of a real
settings file; see below.

Two things about the settings file are measured, not tested. `claude plugin
install` writes the whole `settings.json` anew: it reorders the keys,
reindents and strips a byte order mark. So byte for byte holds only for the cut
minka makes afterwards, not for the file somebody wrote. And Claude Code
refuses a settings file with comments in it, so such a file makes the install
fail before the cut is reached. Claude Code 2.1.211 and 2.1.121 take `claude
plugin marketplace add` and `install` with exit 0, and `claude plugin
validate` refuses the mod there ("hooks: Invalid input: expected record");
2.1.270 to 2.1.284 validate it. The install goes by 2.1.287 as the first
version that loads a mod; that a version between 2.1.270 and 2.1.286 really
runs it is not measured.

An agent waits for what is new with `notifications wait` instead of polling a
board, and one session of a person coordinates a board with `boards
coordinator claim`, `release` and `show` (C-117, C-118); `notifications
count`, `show` and `mark-read` read the number on the bell and one line, and
settle it (C-122). The rule is
[Waiting and coordinating](../stable/waiting-and-coordinating.md). Minka's
side, the list of the bell, `card.moved` and the coordinator, is MNK-B3, on
Minka's `main` since #381 and in no release yet; the routes of one line and
the count are E-241 and older. The commands are checked against that code and
its tests and the fake, not live, see below.

Everything else is tested against the fake API and checked live — the chain
upload → attachment in the text → display → the bytes back, as well as private
conversations in both directions. What is checked against the fake only is
listed below.

Coordination of the API: the Minka session `minka-ca`. It reports changes,
which are checked against the Minka code; deviations go back to it. Minka
v0.41.0 runs on `minka.flownative.com`, the instance this CLI talks to, since
2026-09-30; v0.37.0 ran on both instances on 2026-09-28, and which version
`dev.minka.flownative.com` runs now has not been asked again.

Versions are released by publishing a release in GitHub and installed through
the Homebrew tap; `setup`, `doctor` and the skill inside the binary cover the
first steps.

Dogfooding waits for the internal alpha. The smoke token `minka-cli-smoke`
(account Bruno, local instance) gets revoked then.

## Checked against the fake only

- Waiting and coordinating (`notifications wait`, `count`, `show`,
  `mark-read`, `boards coordinator claim`, `release`, `show`, the coordinator
  in `boards show`): the fake is held against Minka's `main` at b2011b254:
  `NotificationWaitApiController`, `NotificationsApiController`,
  `NotificationResource`, `UnreadCountResource`,
  `NotificationQueryService::findAfter`, `findOneFor`, `countNewFor` and
  `selectNotificationsOfAProject`, `Notifications::markRead`,
  `BoardApiController::claimCoordinatorAction` and `releaseCoordinatorAction`,
  `BoardCoordinators`, `CoordinatorResource`, `NotificationLineResource`,
  `NotificationOutboxListener`, `NotificationRecipients`,
  `ApiNotificationWaitTest`, `ApiNotificationsTest` and
  `ApiBoardCoordinatorTest`. The fake mirrors that a row of an unknown
  occasion uses up a page and moves the cursor, that a row the person may no
  longer see and a reaction do neither, that a line about a card in a column
  in the trash stays listed and names that column, that a sign of life renews
  only a board the person still sees as one of the team, that a move names its
  column from the trash as well and says the bell's own sentence where it
  names none or the column is gone, and the lookup of one line with its 404s. **The fake does not mirror**: of what the bell hides,
  more than a project the person left or that is archived and a card in the
  trash or the archive — a tool in the trash, the subjects other than a card,
  what a client is not shown, a draft post — so lines about those are shown;
  the unread private messages in the count and a private mention that its
  conversation stands for; that Minka forgets read lines; a post sent by name
  as time-sensitive; the appUrl of a line about anything but a card;
  subjects other than a card as Minka names them, the translated summaries
  and titles but those of a move and a completion, the poll interval of a
  held request, and a board in the archive of a project in use, which the
  fake has not, so the 404 Minka gives a claim on one is not held by a test.
  Nothing of it has been seen live.
- Reminders (`reminders list`, `set`, `delete`): Minka's #358 and #359 read
  in `ReminderApiController`, `Rfc3339Moment`, `ReminderKind`,
  `ReminderPreset`, `ReminderQueryService`, `DbalReminderRepository`,
  `Rfc3339MomentTest` and `ApiRemindersTest`; the moments of
  `Rfc3339MomentTest` are held against the fake's reader and the CLI's check
  alike.
  Live on `minka.localbeach.net` (Bruno, 2026-10-05) `reminders list`, JSON
  and text: 200, an empty list; and three writes against the nil UUID, which
  names nothing: `PUT me/reminders/card/00000000-0000-0000-0000-000000000000`
  with `{"preset":"tomorrow"}`, 404; `DELETE` of the same address with
  `?outcome=later`, 422 `reminderOutcomeUnknown`, the outcome before the
  reminder; and without an outcome, 404. No reminder was set. **The fake does not mirror**:
  forwards (it has none, so `forward` finds nothing); a client's view of a
  single comment or chat line beyond the item and the chat; for a client, the
  switch of a document or an upload at the top of a Docs & Files tool
  (Minka's `minka_item_shown_to_clients`), so such an entry is shown to no
  client in the fake, while one below a folder follows the topmost folder;
  of the plain text Minka keeps beside an answer (`RichText::asPlainText`),
  the fake has the blocks on lines of their own, line breaks, the tags gone
  and the entities decoded, but not the source of `script`, `style` and their
  like dropped, an embed read as its address, nor the leniency of an HTML5
  parser with broken markup; the bell and its read mark; and a document or an upload in the
  trash. That an archived card in an archived project and one's own draft
  take a reminder, and somebody else's draft not, is what Minka's
  `ApiRemindersTest` says
  (`aReminderMayPointAtAnArchivedCardInAnArchivedProject`,
  `aReminderMayPointAtOnesOwnDraftButNotAtAnothersDraft`).

- Lists from templates (`todolists templates`, `todolists start`, and
  `templateTakesNoDates`/`templateTakesNoTicks` under `todos`): Minka's #344
  read in its controller, its copy statement and `ApiTemplatesTest`. Live on
  `minka.localbeach.net` (Bruno, 2026-10-04) only reads — `todolists
  templates` answered the library with no template in it, the library is in no
  `projects list`, its tool is named by `todolists tools <libraryProjectId>` —
  and two writes against identifiers that name nothing: `POST
  todos-tools/8cfabb68-c3cc-466d-a7a5-38c6fc25c985/lists`, a random tool,
  with a random template, and `POST
  todos-tools/00000000-0000-0000-0000-000000000000/lists`, the nil tool, with
  `"title": ""`, both 404, the tool before the template and the title.
  No list was made: the library is empty, and a POST into a real tool is not
  allowed there. **The fake does not mirror**: bots (401 before anything),
  more than one organization and its library, the team as anything but the
  members of the library, files of a template copied into the project (the
  fake copies the HTML as it is), the people left off counted before the
  write while the copy follows it, a template archived or trashed between the
  checks and the write, and the library hidden anywhere but in `projects list`
  and `todos mine` (search, activity, recent visits). The fake orders
  `leftOff` by the lowercased name bytewise, as Postgres on musl does, so Ä
  comes after Z; a glibc collation would put Ä right after A. Pre-existing and
  untouched: the fake's `PATCH todos/{id}/subtasks/{id}` looks the subtask up
  after the body, Minka's before it.

- The calendars of `import basecamp --calendars`: no run has met a real
  calendar yet, and Minka's applier of events is not on its `main`, so no bundle
  with one has been applied. The fake's entries are shaped after bodies read
  read-only from the account: an all-day entry in plain days with its last day
  included, a timed one in moments of UTC, no `recurrence_schedule` key on an
  entry that does not repeat, and the one series the account holds, every other
  week on Fridays at 18:00 in `Europe/Berlin` for 1800 seconds. **Unmeasured**,
  and in the fake by reasoning or by the format alone: `?status=trashed` and
  `?status=archived` of `entries.json` and an entry that is drafted; paging of
  `entries.json` (no calendar held more than three); `join_url`,
  `highlighted` and `description_attachments` on an entry (named as a to-do's
  are); `include_due_assignments: false`; every `frequency` but
  `every_other_week`, and what `days`, `week_instance` and the intervals hold
  for each; whether `end_date` is the last day a series may start on; an
  all-day series and its `duration`; a creator whose zone is not the one the
  hour holds in; whether the list names the first day of a series that has run
  for years; and whether `projects/recordings.json?type=Comment` names the
  comments under single days at all. The links to an entry are shaped after
  those the account's texts hold, read from the Basecamp CLI's cache: eight
  distinct ones, six to an entry and two to a day of eight digits, every one
  at `3.basecamp.com` with the account and the bucket, never with `.json` or
  anything below; that Minka
  turns them into a reference to the event is checked against its code and
  tests only. Owed before the merge into a real run: a
  read-only dry run of one project with a calendar, with Robert's word.

- The order of the refusals follows Minka's `main`, which is in no release
  yet: the address is looked up before the fields of the body and before the
  archive, and a comment, a chat message, a step or the subtask of a move
  under it as well, so whoever cannot see what an address names, or names one
  that is not there, gets 404 whatever the body says. Only where the
  controller reads the body first — writing a comment on a card, creating or
  changing a post, a document or an upload, and a comment written, changed or
  reacted to under one of them — is a body that cannot be read (400, 415)
  refused before the address. An argument
  of the web's forms, in the query or in a body Flow parses, is 400 before
  everything. A release before that answers a refusal of
  the body first, which no command depends on: the CLI keys on codes, not on
  their order. Read in the controllers, not seen live.

- A tool by its identifier alone (`GET files-tools/{id}`, `posts-tools/{id}`,
  `todos-tools/{id}`, `checkins/{id}` and `chats/{id}`, each with `projectId`
  and `project`) and `projectId` on the lines of the entries of a tool, of the
  posts and the archive of a Posts tool, of the lists and the archive of a
  To-dos tool and of the archive of a board: read in the controllers and
  resources of v0.41.0+1, not seen live. The fake answers 404 for a tool of
  another kind, of a project one is no member of, and, for a Docs & Files or a
  Posts tool, one a client is not shown. It has no client switch on a To-dos
  tool, a Check-ins tool or a chat, which Minka has, and models no Docs & Files
  or Posts tool in the trash and no project in the trash. The CLI reads the
  tools of Docs & Files, Posts and Check-ins.

- Everything of `columns create`, `edit`, `move` and `delete`: no request has
  gone to a real Minka. The routes are in v0.52.0+1 and later; #331 is on
  Minka's `main` only, and the fake mirrors `main`, with
  `KeepsColumnColorOnNull` for the null colour of a released Minka. Read
  in `BoardApiController::addColumnAction`, `updateColumnAction`,
  `moveColumnAction` and `removeColumnAction`, the column handlers,
  `DbalBoardColumnRepository`, `BoardColumnColor`, `Gap` and `ApiColumnsTest`
  on Minka's `main`, `--no-color`, the client's 404 on every column write,
  `neighbourColumnOnAnotherBoard` and the refused no-op move in an archived
  project included, and mirrored by the fake in the order of
  [Columns](../stable/columns.md). What
  the fake does not mirror: a board in the trash, which it has no word for;
  bringing a column back, which only the web does; Minka's positions, for
  which the fake keeps the order of a list, the same order; a working column of
  the fixture set up without a colour, where Minka gives every new one a
  colour; and the race in which a neighbour read on the board leaves it
  before the move, which Minka would answer by putting the column first.
- `steps edit` and the people and the day of a step: no request has gone to a
  real Minka. Read in `BoardApiController::updateStepAction`,
  `EditStepHandler`, `DbalCardStepRepository` and `ApiStepAssigneesTest` on
  Minka's `main`, and mirrored by the fake in the order of
  [Steps](../stable/steps.md). The fake has no board hidden from clients, so a
  client who does not see the board, refused by Minka, is put on a step there;
  it writes the day, the people and the tick of one request together, where
  Minka writes the tick in a second transaction; and it shows no overdue for a
  step, which Minka's API does not either.
- `steps edit --title` and `steps move`: no request has gone to a real Minka.
  Read in `BoardApiController::updateStepAction` and `moveStepAction`,
  `StepTitle`, `RenameStepHandler`, `MoveStepHandler`, `Gap` and
  `ApiCardStepsTest` on Minka's `main` (da3fb0250), and mirrored by the fake in
  the order of [Steps](../stable/steps.md). The fake puts a moved step right
  behind or in front of its neighbour in a list, where Minka works out a
  position between two keys; it writes the text in the same go as the rest,
  and stands in for a Minka without either by a switch, whose 404 for the
  move is the API's own.
- `projects edit`: no request of it has gone to a real Minka, and none can
  until a release carries the rename. Read in `ProjectsApiController::
  updateProjectAction`, `DbalProjectRepository::rename` and `RenameProjectTest`
  on Minka's `main`, and mirrored by the fake: the same 404 for a client as
  for an outsider before anything about the body, then the body, then
  `projectArchived`. A Minka
  without the rename answers the address with a 404 page of text/html instead
  of its JSON on `minka.flownative.com` and `dev.minka.flownative.com`,
  measured read only with a GET; a PATCH meets the same missing route (v0.41
  has no route for the address under any method). The CLI tells such a 404 apart from Minka's own
  and says the host's Minka is older than this address. The fake keeps no
  project in the trash, which Minka answers with 404 as well.

- The status of what stands in a list or a group on the shelf or in the trash:
  seen live for an archived list, whose groups and to-dos come `archived` with
  `inherits_status` in its `groups.json` and `todos.json` and among the
  archived records of the project. Not seen live, and mirrored by the fake all
  the same: a trashed list or group, and a group or to-do with a status of its
  own under a parent that has one, which the fake reports as the parent's and
  inherited. A live check is open.
- IDs in any case: that Minka writes an ID in lowercase before it reads it
  (`AbstractApiController::readId`), in the address, the body and the query,
  is read in its code on `main` (00bbcf39), not released and not seen live.
  The fake does the same: every `…Id` of a route, the identifier fields of a
  body alone or in a list, and the four queries that carry one — `before` of a
  board's archive and of the answers, `personId` of the answers, `folderId` of
  the entries, `after` of one's own to-dos. What Minka does not read so stays
  as it is in the fake too: the name of a draft of a chat message, refused in
  capitals, and the cursors of the posts and of the done to-dos. The CLI sends
  every ID in lowercase all the same, which every Minka reads.
- `replyTo` of a chat message: the fake refuses one that is no identifier, or
  names no standing message of the chat, with `chatMessageReplyToUnknown`, the
  first before an archived project and the second after it; it neither keeps
  nor shows which message was answered. No command sends one. Read in the
  Minka code: the controller refuses the first, and the repository asks the
  second only after the counter of the chat, which an archived project does
  not move.
- An address whose identifier is none: Minka answers 404 before it tells
  anybody that a project is archived — for assignees, subscribers, steps and
  subtasks, and for the removal of a comment under a card, a document or an
  upload and of a chat message — and a removal of a step or a subtask named so
  is 404 rather than 204; a subscriber named so was `personNotAProjectMember`
  before. A step
  of another card is 404 on a change and 204 on a removal, which takes nothing
  off the other card. Read in the code, mirrored by the fake, not seen live,
  and unreachable from the CLI, which checks every ID as a UUID before a
  request.

- Of the notes of a card: `cardNotesHasControlCharacters`, and `cardArchived`
  for a card in its board's archive, which the fake gives after the body and
  before the same text counts as no change.
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
  tell apart — which is the one case the guard cannot see live. The fake asks
  `richtext.CarriesMarkup` for the fallback, the same rule the CLI asks, so
  the edges of the list of served elements are shared knowledge and not a
  second measurement.
  Of the three refusals over whom a document reaches — `notifyIdsMissing`,
  `notifyIdsWithoutChosenAudience` and `personCannotSeeIt` — **no live instance
  has been seen to answer any one of them**, although the v0.37.0 that carries
  them runs on both instances. All three are read in
  `FilesApiController::addDocumentAction`, `PeopleAPostTells::enrolEachOf` and
  the functions `minka_item_shown_to_clients` and `minka_tool_shown_to_clients`,
  and mirrored in the fake from that reading alone. Every one of the three is
  reached by a command, so all three are held by a run of `docs create` against
  the fake, hint and order included. For `personCannotSeeIt` the fake carries a
  person who works with the project as a client and a folder whose contents its
  clients are shown — that client signs in nowhere, so the fake never has to
  answer as one. The fake also carries whether the tool is shown to clients,
  that the topmost folder decides, that an agent of a client counts as a
  client, and that a person named twice is told once. The last is read from
  `PeopleAPostTells::enrolEachOf` at v0.41.0, which passes over whoever is on
  the list already (MNK-5P; before, the second naming was refused as no
  member), and has not been seen live.
- Putting a file into a tool and changing an upload were read in
  `FilesApiController::addUploadAction` and `updateUploadAction`,
  `BlobQueryService::findStoredBlob` and `isOwnFileInUse`, `AddUploadHandler`,
  `EditUploadHandler` and `DbalFilesRepository::editUpload` at Minka 345c063e,
  and mirrored in the fake from that reading alone: the tool, then the file,
  then the title, the description and the folder, and only from the write
  itself `projectArchived`, `fileInUse` for a file of one's own and
  `fileUnknown` for everything else; a change that keeps the file whatever is
  sent and empties the description on `null` as on `""`. What the fake does not
  mirror: it does not look through its texts for a stored file, so a test says
  with `StandsInAText` that one carries it, and a sweep is a test setting
  `Swept`; the moment a sweep comes is Minka's and nothing here measures it.
  Who is told about a mention in the description, and the subscription of the
  writer, the fake does not keep at all. Which files a client is shown the fake
  has no word for.
- The versions of documents and uploads: no request of them has reached a
  version on a real Minka. Read in `FilesApiController::documentVersionsAction`,
  `documentVersionAction`, `uploadVersionsAction` and `addUploadVersionAction`,
  `EditUploadHandler`, `DbalFilesRepository::editUpload`,
  `whyAnUploadCannotTake`, `FilesQueryService::findVersionsOf` and
  `findFilesOf` and `ApiFilesVersionsTest` on Minka's `main` (2cef0959), and
  mirrored by the fake. Seen live on `minka.localbeach.net` against a fresh
  random UUID only: the three reads and the POST answer Minka's JSON 404, and
  the POST reads its body first — 400 `malformedBody` for a body that is no
  JSON and 415 for `text/plain` before the 404, and a body with a `blobId` and
  an `expectedUpdatedAt` that are none answers the 404, so the upload comes
  before the fields. `minka.flownative.com` answers the address with a 404 of
  text/html: it has no such route yet. The local Test-Projekt has no Docs &
  Files tool, so no version has been read live. What the fake does not mirror:
  a document or upload set up without versions gets them made up when they are
  first asked for (a document's first without an author, an upload's by
  whoever stored its file), where Minka keeps them from its own records; which
  files a version names is the files its body names, where Minka asks the files kept
  for the version and those of the document; a write the controller reads in
  the same moment as somebody else's, which the fake cannot have; and an entry
  in the trash, which the fake has no word for. A client is refused as in
  Minka, at the versions and at an earlier file of an upload unless it stored
  that file itself.
- Everything of `todolists` and `todogroups`: no request of it has gone to a
  real Minka. The fake mirrors what was read in `TodosApiController`, the
  `TodoQueryService`, the handlers and `DbalTodoRepository`: the order the
  controller refuses in — the tool, the list or the group before the body, the
  title before the description, a neighbour that is no identifier before the
  archive — the project before the
  archive of the list, the group of an archived list at no address, the
  archive ordered by the moment of archiving, a list coming back to its place,
  a trashed group taking its to-dos out of every count, and a tool no longer in
  use that the project does not name and that is read at its own address. What
  it does not mirror: which lists and groups a **client** is shown beyond a
  list hidden from the clients (`HiddenFromClients`), which the fake has no
  word for on a tool, a group or a to-do; `todoListDescriptionNotReadable`,
  which needs markup nested deeper than a cleaner reads; and the race in which
  a group write meets a list archived after the group was found, which is the
  one way to `todoListArchived` on a group and which no command can steer
  into.
- Everything of `todos repeat` and `todos series` but one read: the API came
  with Minka v0.41.0, a read-only `todos series list` has answered on
  `minka.flownative.com`, and no write of it has gone to a real Minka.
  The fake mirrors what was read in `TodoSeriesApiController`,
  `RepetitionScheduleFields`, `RepetitionSchedule`, `TodoSeriesQueryService`,
  the series handlers, `DbalTodoSeriesRepository` and `ApiTodoSeriesTest`: the
  order the controller refuses in — the type of every field before any value —
  an edit that changes nothing writing nothing and meeting no conflict, people
  compared as the series shows them, the details a request leaves out taken from
  the stored schedule or the first day, the resource with its state and days,
  a list hidden from a client at no address, the guard, and the occurrence
  `show-now` makes at the end of the list with the series' people and creator.
  The date math is a port of `RepetitionSchedule`, held against the cases of
  Minka's `RepetitionScheduleTest`. What it does not mirror: the round that
  makes occurrences as days pass — the fake's clock stands still and nothing
  but `show-now` makes one; the installation's time zone, for which the fake
  takes UTC; who follows an occurrence; and the race in which a series is made
  or shown twice in one moment. Of the notes it mirrors the order — the type
  after the title's, the value after the title and before the people, the
  archive and the guard — notes kept and compared as a to-do's, `notesSource`
  and `attachments`; not the cleaning against the project, which drops a
  foreign or forged handle and a stranger's mention (the fake keeps them in
  the stored notes, and only its source and its files leave out a file of no
  circle of the project, as Minka's source does), the references Minka
  resolves on
  saving, the files hanging on the series rather than in its text, and
  `todoNotesNotReadable`, which needs markup nested deeper than a cleaner
  reads.
- Everything of `todos`: no request of it has gone to a real Minka. The fake
  mirrors what was read in `TodosApiController`, the `TodoQueryService`, the
  `AssignedTodoQueryService`, the to-do and subtask handlers,
  `DbalTodoRepository`, `DbalTodoSubtaskRepository` and `ApiTodosTest`: the
  order the controller refuses in, the list or the to-do before the body and
  the subtask of a move as well, a day alone as the due day, the people as a
  whole set with who stays keeping their place, the moves by place and
  neighbour, a done to-do ticked off again answered like one ticked off now,
  the cursor of the done to-dos behind a moment and not a to-do, and one's own
  to-dos in the order of the page. What it does not mirror: the time zone of
  the person asking, in which Minka decides `isOverdue` and writes the moment
  into the cursor — the fake takes UTC for both, and the cursor stays opaque to
  the CLI; which to-dos a **client** is shown; `todoNotesNotReadable`; and somebody who left the
  project and is not named again in a new set, whom Minka leaves on the to-do
  unseen and the fake takes off. Ticking off gives every to-do of the fake a moment a
  second after the one before, so that the done to-dos have an order to read
  while its clock stands still.
- Everything of `checkins`: no request of it has gone to a real Minka. The fake
  mirrors what was read in `CheckinsApiController`, its resources, the
  `CheckinQueryService`, the question and answer handlers,
  `DbalCheckinQuestionRepository`, `DbalCheckinAnswerRepository`,
  `CheckinSchedule`, `TimeOfDay` and `ApiCheckinsTest`: the order the
  controller refuses in, the schedule kept with its weekdays sorted and taken
  once and its time with two digits, `asked` naming only the members who may
  see the question in the order of their names, an edit without people leaving
  them as they are and one with people taking off only whom Minka shows as
  asked (MNK-5R, read at v0.41.0 in `CheckinsApiController::updateQuestionAction`
  and `DbalCheckinQuestionRepository::askOnly`, not seen live), a client the
  question is not shown to refused as no member when named (read at v0.41.0 in
  `PeopleACheckinAsks::askOnly`, not seen live), pausing and resuming twice,
  an archived project refusing both whatever the state, the page of answers
  without an end and cut at 100, a boundary in the trash still marking where a
  page ends, the day of an answer without `askedFor` as the latest asking by the
  schedule, and the reminders as the latest day per question, open until an
  answer on that day or later, at most 100. What it does not mirror: the time
  zone of the person asked and of the writer, in which Minka decides the start
  day, the latest asking and so the day of an answer — the fake takes UTC;
  `answerNotReadable`; and the cleaning of a body against the files of its
  circle. An agent that stepped down is listed in `asked` wherever it is still a
  member, which Minka leaves it only in the projects archived then. That a question set up through the API is shown to no
  client was read, not measured: the insert names no `visible_to_clients`, and
  the column defaults to false.
- Everything of `posts`: no request of it has gone to a real Minka. The fake
  mirrors what was read in `PostsApiController`, its resources,
  `PostQueryService`, `PostsCursor`, the post handlers, `PeopleAPostTells`,
  `DbalPostRepository`, `DbalItemSubscriptionRepository` and `ApiPostsTest`:
  the order the controller refuses in — the tool before the title of a new
  post, the request before the project, the project before the kind and the
  kind before the people, the trash before the project when a post is
  archived — the page with the pinned first and `next` null on the last page,
  behind the values of a post and not the post, the silent cut above 100, the
  archive ordered by its moment, a pin that keeps its place and survives the
  archive, the trash from the list and from the archive, a new post shown to no
  client, and a person named twice told once. That last one is the code path
  of documents and has not been seen live either. What it does
  not mirror: the time zone of the reader, in which Minka writes the moments of
  a cursor — the fake takes UTC and the cursor stays opaque to the CLI; the
  plain text a `lead` is cut from, which the fake takes as the body without its
  tags; the cleaning of a body against the files of its project; and the trash
  of a project, which lists cards alone in the fake; and what a **client** is
  shown — the fake's clients sign in nowhere, so a post or a tool hidden from a
  client is never read as one. `postAudienceMissing` and a `notifyIds` entry
  that is no identifier are mirrored and unreachable. A comment on a post
  written again with the same text moves its `editedAt` in Minka
  (`DbalCommentRepository::change`) and in the fake, under a document, an
  upload, a to-do, a list and a card as well.
- The calendars and events: no write of them has gone to a real Minka. On
  `minka.localbeach.net` (Minka's `main`), `events tools` answered `[]` for the
  three projects Bruno sees — none has a calendar, and the live check allows
  no write that would add one — and `events tools`, `events list` (with a
  span and with one that is no span), `events show` and `events edit` of
  `0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6` answered the byte-identical 404, so
  the address is looked up before the query and the body there too. That ID
  is no fresh random UUID but one shaped like the fixtures of the tests; it
  names nothing on the local instance. The one write request was that
  `events edit`: `PATCH
  /api/v1/events/0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6` with
  `{"title":"x"}`. Everything else is checked against the fake, which
  mirrors `CalendarApiController`, `ReadsRepetitionSchedule`,
  `CalendarEventQueryService`, `EventOccurrences`, `EventRepetition`,
  `EventParticipants`, `DbalCalendarEventRepository` and `ApiEventsTest`: the
  calendar or event before the body, the type of every field before any value,
  the values in the controller's order, `projectArchived` after them — also for
  a change that changes nothing — and before the people, the clock time a
  series keeps in its zone across summer time, with an hour that comes twice
  taken the first time and one that is skipped moved forward as PHP's
  `setTime()` places them (measured with PHP 8.5), the reader's days of a
  span, the order by the first occurrence, and a client who sees and writes
  only what is shown to clients. What it does not mirror: an hour that comes
  twice where the clocks go back from 03:00 to 02:00, as in
  `Africa/Casablanca` on the day Ramadan time starts, which PHP takes the
  second time for 02:00 to 02:45 and the fake the first; PHP's list of time
  zones, which the fake holds as a snapshot taken with PHP 8.5
  (`internal/fakeapi/phpzones.json`), so a zone a later PHP adds or drops
  differs until it is taken again; PHP's check of a URL, which the
  fake approaches by a host it demands; the cleaning of a description
  against its project and `eventDescriptionNotReadable`; whom a write tells
  and that the writer follows the event; and a calendar in the trash.
- What a calendar shows and My calendar. Seen live on `minka.localbeach.net`
  (Minka's `main` with #319) on 2026-10-03, as Bruno: `events mine` with a card
  of the board "API-3 Abnahme" due on 2026-10-21 and assigned to Bruno, and a
  step on it due on 2026-10-22 and his, both created for the run, answered the
  two dated items in the shape the fake gives, the step with `cardId`, its
  card as `placeTitle` and the card's page as `appUrl`; the text view put them
  under their heading; `eventsSpanNotADay`, `eventsSpanOutOfOrder` and
  `eventsSpanTooLong` came as for a calendar, 366 days and days with spaces
  around them were taken; once the card was in Done both were gone. The card
  was then put into the trash. `events settings` went out only against
  identifiers that name nothing: `PATCH
  /api/v1/calendars/f111132a-4d49-4646-ac46-9dd0dea16f6d` (a fresh random
  UUID) with `{"content":"eventsOnly"}` and with `{"content":"everything"}`,
  and `PATCH /api/v1/calendars/00000000-0000-0000-0000-000000000000` with
  `{"content":"eventsOnly"}`, each the byte-identical 404, so the calendar is
  looked up before the value. No project of Bruno's has a calendar, so the
  dated items of a calendar's span, `content`, `calendarContentUnknown` and
  the client's 404 after the archive are checked against Minka's code and
  `ApiMyCalendarTest` and the fake only. The rules of #327 are mirrored and
  tested, against its code and `ApiMyCalendarTest`, not live: a card
  completed outside Done and its steps stand nowhere, and a day's items go by
  title with only A to Z lowercased, then by `id`, in both. What the fake
  does not mirror: a card hidden from clients, since the fake shows a client
  every card; and a tool or a calendar in the trash.
- The comments under an event: no request of them has gone to a real Minka,
  and production has none of them yet. The fake mirrors
  `EventCommentsApiController` and `ApiEventCommentsTest` through the
  handlers of the comments under a to-do: the body before the event, the
  event and the comment under it before the text or the reaction,
  `projectArchived` before the author, 404 at all six addresses for an event
  in the trash and for a client on one not shown to clients, and comments
  under an event of a calendar out of use. What it does not mirror: the
  cleaning of a comment against the files of its project and the people a
  mention tells.
- The comments under a to-do and under a list: no request of them has gone to
  a real Minka. The fake mirrors `TodoCommentsApiController`,
  `AbstractCommentsApiController`, `DbalCommentRepository`, `CommentableItem`
  and `ApiTodoCommentsTest`, through the same handlers as the comments under a
  post or a document: the body before the item, the item and the comment under
  it before the text or the reaction, `projectArchived` and `todoListArchived`
  before the author, `todoListArchived` for every write under an archived list
  and under its to-dos. What it does not mirror: the cleaning of a comment
  against the files of its project and the people a mention tells.
- The `subscribers` of posts, cards, to-dos, documents and uploads: no
  extraction has read one live. What `recordings/{id}/subscription.json`
  answers was **measured** read-only by the coordination, some 40
  requests under the default identity: 200 for a card, a message, a to-do, a
  document and an upload, in the shape a question's answer has as well —
  `count`, `subscribed`, `subscribers` and `url`, every entry with an `id` and
  `personable_type` `User`, and `count` equal to the length of the list in every
  answer, seven people at most. Nothing in the body hints at pages, and the
  OpenAPI description the Basecamp CLI 0.11.0 carries marks no pagination for
  it where it marks it for the operations that page. A cap above seven is not
  measured; the run ends where the list holds fewer people than `count`, so a
  capped list stops the run instead of unsubscribing anybody. A recording that
  is not there, a wrong project and one the identity cannot see all answered
  404 (`not_found`); no 403 was seen, so `forbidden` is held against the fake
  alone. A record in the **trash still answers 200 with its list**: that such a
  record is not asked rests on the status the run already read, never on an
  answer of the source. What Minka makes of the field was read in
  `ImportedSubscribers`, `SetImportedItemSubscribersHandler` and
  `BundleRecord::strings` at Minka's main 6780365e.
- That a group in the trash carries `color` where Basecamp lists it among the
  recordings of a project: only the fake says so. Measured read-only against
  the live account was `color` in `groups.json` and in the recordings of
  active and archived groups.
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
- Every write on an archived card answers `cardArchived` in the fake, the
  ways of Minka's `ApiArchivedCardTest` and the notes, in the order
  `BoardApiController` asks: a comment or a step under the card first, then
  the body, then `projectArchived`, `cardArchived` and last the author of a
  comment.
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
- Of `import basecamp` without `--project`: the lists of the active and the
  archived projects answer live in the shape the fake gives, one project
  object each with its `status`, and `?status=trashed` answers `[]` for the
  account; a walk of their pages and a whole run over every project are not
  seen live yet.
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
  a chat, bots and deleted people as writers whom an admin's list holds as
  well, boosts on lines, a line that says nothing left out, posts with and
  without a category, archived posts, a second message
  board with categories no list holds, previews of links and pictures at a
  foreign address as links, and five parts of one chat each.
- Of the bots of `import basecamp` (C-094): all of it. The rule rests on what
  Minka's bots concept measured on 2026-09-26 — a chatbot's lines carry a
  creator of `personable_type: "Integration"` with `email_address: null` and
  the chatbot's own ID, and a deleted chatbot leaves a `Tombstone` — and on
  nothing seen by a run of ours. Open for a read-only live check: whether a
  deleted chatbot's `Tombstone` keeps an address, and which; whether any other
  kind than `Integration` (a `Chatbot`, say) or a `User` writes
  `Chat::Lines::Integration`; whether a chatbot shows up anywhere but as the
  creator of a chat line — the account list for an admin, the people of a
  project, a boost, a comment, a mention — and in what shape; whether a
  deleted user's `Tombstone` can ever be the creator of such a line; and
  whether an integration's person carries `client` or a `company`.
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
- Of the schedules of to-dos in `import basecamp` (C-090): the mapping rests on
  the twelve schedules measured read-only on 2026-09-30 and on Minka's format,
  and no run has carried one yet. The fake sends them in the measured shape:
  all ten keys, `days` a list, `hour` and `minute` 0, the key missing on a
  to-do without one. What the account was seen to hold: `every_week` with one
  weekday, `every_month` with `week_instance` 1 or 2 and one weekday,
  `every_day_of_month` with one day, `custom_month` with `week_instance` 1 and
  `month_interval` 2 or 6, `every_year` with empty `days`; `start_date` equal
  to `due_on`; `week_interval`, `end_date` and `duration` always `null`. Never
  seen, and read by the extractor from the format alone: how Basecamp says
  "the last" weekday of a month (so a `week_instance` outside 1 to 4 costs the
  series), `every_month` without `week_instance`, a `week_interval` or an
  `end_date` that is set, a time of day other than 0, and what `month_interval`
  says on a rhythm other than `custom_month`. The recurring to-do on a to-do
  set itself (10298510137) stays left out with its schedule until somebody
  moves it into a list.
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
  **A file Basecamp refuses** is the one shape of this whole tool that a
  full extraction of the account has met, exactly once over 10,274 files: the
  download of one upload's file was refused, and no run will ever bring it.
  An upload whose file no run will ever bring is left out and named in
  `docs.onlyAFileNoRunWillBring`, with its comments and its boosts counted as
  fallen, and that holds for all four ways of having no file: one in a cloud,
  one the source no longer has, one above what Minka takes, and one Basecamp
  refuses. Where the file left a record behind, that record stays as the trace of
  what became of it. What the run does with such a file rests on the invented
  account, where every one of the four is played by a stand-in. The residual of
  the cloud branch falls under the same rule: **an upload that lives in a cloud
  and does carry a `download_url`** is taken as having a file and the fetch
  fails, and where Basecamp refuses it the upload is left out and named.
  Nothing has been seen of such a record either way.
  A **dry run** counts such an upload all the same, because nothing the source
  answers says that a file is gone or that a download will be refused: it says
  one upload too many rather than promising a file it never asked for. What a dry
  run does see is an address it cannot build, which is the early warning it
  exists for.
  **What a walk of the account is to expect of the exit code**: that one file
  ends the run **0**, with a notice naming it, the record that fell with it and
  the count in the `summary`. A file the source refused is a result and not a
  failure; a file that may still come ends the run 6, and a failure the run could
  not place ends it 7 (C-069). Read `docs.onlyAFileNoRunWillBring` and the
  entries of `missingFiles`, and settle at the source the ones that say
  `refusedBySource`.
- The earlier files of an upload (`versions`, C-104) are checked against Minka's
  `UploadVersionAtTheSource`, `UploadsApplier` and `ApplyBundleDocsAndFilesTest`
  at Minka 82633dfde (E-229) and against the fake. **Measured live**, read-only,
  with Robert's profile: the upload record carries nothing that says it has
  versions, so the list is asked for per upload, 0.61 seconds a call; the list
  answers `?page=2` with the whole list again, so it is read as one answer; one
  upload with two versions gave, by the two requests this run makes — the token
  of the Basecamp CLI on `3.basecampapi.com` with no redirect followed, 302 to
  `storage.basecamp.com/<bucket>/<key>?X-Amz-…`, then that place without a
  token — 60,003 bytes for the earlier version and 64,316 for the current one,
  each exactly its `byte_size`, with two different digests, the current one's
  equal to what `basecamp files download --in` brings. Those steps were measured
  by a probe outside Go; **the Go code itself has not reached Basecamp**: a test
  binary of it was held back by the firewall, so the first real run is its first
  live use. Not measured: whether a long list of versions is ever cut, an upload
  whose list names a version without its file, the list of an archived or a
  trashed upload, a refusal of the signed store, and a version given by somebody
  other than whoever made the upload, which the scout's five uploads with
  versions did not hold.
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
  Whose word a missing file is, which decides both the exit code and whether
  anything is written off, is held by a table over every class a failure can
  carry, the empty one included, and by the fixtures of the fake. Live, only
  three of the classes have been seen: the refusal of a download, a network that
  failed, and a gateway in front of Basecamp. What no live run has produced is a
  failure the run cannot place — the shape that matters most, because it is what
  a Basecamp CLI that changes after an upgrade would send. For an avatar the
  class of a status that is no 200 is read in a test of its own and nothing else:
  live, no avatar has answered with anything but 200. That a 401 there ends the
  run rather than writing the picture off is held against the fake, by a run whose
  avatar answers without a sign-in; no sign-in has ever run out while avatars were
  being fetched live.
- Of `--agent` of `import basecamp` (C-116): all of it, against the fake.
  Checked by reading against Minka's merged import (`main` at `fd6e6ffd0`,
  E-263): it takes `kind: "agent"` and `principal` as a reference, refuses an
  agent with an `email` and one whose `principal` no earlier line brought, is
  itself, is no human or is a client, and leaves an agent out of a project its
  human is not in, counting it. It turns a person an earlier run brought as a
  human into the agent and stops, naming them, over a login of their own,
  another organization, agents or bots of their own, the owner or admin role,
  or being the last member of a project their human is not in. Nothing the
  bundle writes goes against it; no bundle with an agent has been applied.
- Of `--keep-files` of `import basecamp` (C-115): all of it. No live run has
  used it; it is held against the fake and against the file system of the
  tests, a kill by a run that runs no handler. That a run killed in the middle
  of a file or of a line of the index leaves nothing that is taken over is
  reasoned from the order of the writes and tested with the leftovers such a
  kill makes, not with a real kill.
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
  with 200 and `leftOut` while everything else of the set is noted, and every
  write on it is 422 `cardArchived`, a move before a column without the area on
  hold. All of it read in the Minka code.
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
- That every session reports its own list under a client of its own is tested
  against the fake alone, and so are `--session`, a beat going by the
  environment whatever its standard input names, and a store of one list per
  machine handed out to its sessions. Claude Code's documentation names no
  `CLAUDE_CODE_SESSION_ID`; should the variable go, a typed `work start` falls
  back to the list of no session, which the mod does not report.
- The mod (C-105) is measured in throwaway runs of Claude Code 2.1.288, with the
  mod loaded by `--plugin-dir` and a wrapper in place of minka that logged what
  it was given: `$.session.id()` is the `CLAUDE_CODE_SESSION_ID` the Bash tool
  sees, what `$.process.run` starts carries neither that variable nor
  `CLAUDECODE`, the timer beats during a Bash call of seventy seconds, the
  tool calls of a subagent reach the mod of the parent with `agentId` set and
  its Bash sees the parent's session, a subagent's turn ends without a start,
  and a run that is not awaited still runs. `hooks install` ran against a
  Claude Code configuration of its own (`CLAUDE_CONFIG_DIR`): both `claude
  plugin` commands answer exit 0 the second time too, the settings carry
  `enabledPlugins` and the marketplace as a `directory` source, and a file
  changed in the directory after the install was loaded by the next session,
  so the plugin loads in place. `claude plugin validate --strict` and the mod's
  own tests (`claude plugin test`) run in `make check` wherever `claude` is on
  the PATH, and are skipped where it is not, which is CI. The two-hour bound on
  a tool call that never returns is tested with the mock clock, and that an
  abandoned call stops counting at once (`next.signal`) with a mod above the
  heartbeat that gives up on the call, which aborts the signal beneath it; a
  mod above that returns without waiting at all does not. A call that never
  returns has never been seen live. Not seen: the mod in
  an installed, signed-in Claude Code over a working day, a tile that goes dark
  three minutes after a session was killed, what `/clear` does to the session
  a beat names, and `session.start` after `/resume`, which the documentation
  says does not come.
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
  carries the session of whoever spawned it, so a session that works through
  subagents keeps the sign of life of its cards.
  Taking the old hook out is tested against settings files of the test's own:
  other hooks under the same event and others, unusual spacing and order kept
  byte for byte, a group of somebody else's, the hook written twice, a link, a
  file with a comment in it, and the mode kept. No test runs `claude`; the two
  `claude plugin` commands go through an injection point.
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
- That a beat waits for nobody at the store is held for the claim of the
  heartbeat. The 2 seconds that the note of how a signal fared waits are held by
  no test: a test can tell a wait from none, not a short one from a long one,
  without waiting it out.
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

- Below an archived folder, Minka's `main` refuses changing and removing a
  comment of a document or an upload with `documentArchived` or
  `uploadArchived` (`DbalCommentRepository::underAnItemInUse` asks
  `FolderChain`, E-195), as it refuses a new comment and a reaction on one.
  No release carries it yet: v0.41.0, which `minka.flownative.com` runs, and
  v0.42.0 change and remove one's own comment there. The fake follows `main`,
  checked in the order of `FilesApiController` and
  `AbstractCommentsApiController` (the body before the entry, the comment
  before the fields, the archive before the author), not seen live. The
  CLI keeps reading the folder before a comment edit that would drop files
  (C-082), because against v0.41.0 the guard is all that holds them; once the
  instance runs a Minka with the refusal, that read can go.
- `editedAt` of a post is set by going into the trash and coming back out of
  it, so a post restored on the web says it was edited although nobody changed
  it (`DbalPostRepository::throwAway`, `restoreFromTrash`). Reported to Minka
  by the coordination.

- Of the calendars, four places where Minka's format and its code part, for
  the coordination: the format takes every zone name PHP's `DateTimeZone`
  takes, `Europe/Kiev` and `US/Pacific` among them, while Minka's `TimeZone`
  takes only `DateTimeZone::listIdentifiers()`, which holds neither; the run
  writes what the format says, which the coordination confirmed. Minka's
  `TimeZone.php:24` accepts only `listIdentifiers()` (`ALL`), so its applier
  of events has to widen it. The format asks for who follows an entry in the
  trash, which C-084 does not do for any record. It asks for comments and
  boosts of every entry, where the run asks only where `comments_count` and
  `boosts_count` say there are some, as for every other kind. And the applier
  of events (`ImportKind` `events`) is not on `main`, so `ToolsApplier` still
  refuses `calendar`.

- A post category's emoji is part of its name since Minka #365 (7c038982d)
  (E-144), which no release carries yet. The CLI follows it (C-114): it shows
  the name alone and reads no `emoji`, which Minka keeps on the categories,
  always empty, for one release. Against a Minka without it, `posts
  categories` shows a kind without its emoji. **A bundle made by this CLI is
  refused by a Minka without #365**, which demanded a non-blank `emoji`
  on every category; so the release of the CLI carrying C-114 must not go out
  before a Minka release carrying #365.

## Open in the API

- Paging Done beyond 25 cards does not exist; needed once dogfooding needs it.
- Flow answers a wrong HTTP method with HTML instead of JSON.
- Column colours come later, read-only.

## Next steps

- A read-only dry run and then a run with `--upload-versions` over a project
  holding an upload with versions, with Robert's word: the first time the Go code of C-104 reaches
  Basecamp, and the run that says whether `bin/minka` gets past the firewall
  for the two requests it makes itself.
- A read-only dry run of one project with a calendar, `import basecamp
  --project <id> --calendars --dry-run`, with Robert's word, before the
  calendars go into a real run.
- `events settings` and the dated items of a calendar's span against a real
  calendar, once the live check names one a run may write to.

- Check the bots of `import basecamp` read-only against the live account, as
  listed under Checked against the fake only, and a bundle with them through
  Minka's applier.
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
- See two sessions live: a card of a session that ended goes dark while another
  session works on, and `work list` shows the other's card under `elsewhere`.
- Check the mod live, as the [live check](../guides/live-check.md) says: a tile
  that stays alight through a tool call of five minutes, and one that goes dark
  within three minutes of quitting Claude Code. And on Robert's machine after
  the release: `minka hooks install` takes the old hook out of his
  `~/.claude/settings.json` and leaves the rest of it as it was.
- Of the ninety-odd addresses the API offers, a good part have no command.
- Check the events live once a release carries them and the
  [live check](../guides/live-check.md) names a place to write them: a timed
  series across a change of summer time listed in a span, a series moved with
  and without `--repeat`, stopped with `--no-repeat`, a zone such as `CET`
  taken and one such as `europe/Berlin` refused, a client named and refused,
  an event put into the trash, and a comment under a series written, reacted
  to and refused in an archived project.
- Check the posts live, as the [live check](../guides/live-check.md) says: a
  post sent out to `chosen` with `--notify me` and the same person twice, a
  client named and refused, a page of two with `next` and the page behind it,
  a pin twice and an unpin twice, an edit taking the kind off, a post archived,
  written to in the archive, brought back pinned, trashed from the archive and
  archived from the trash, and a comment under it written, reacted to and
  refused in the archive.
- Check the check-ins live, as the [live check](../guides/live-check.md) says:
  a question set up with `--ask me` and a client refused as no member,
  `asked` in the order of the names, `startsOn` in a time zone that is not UTC
  near midnight, an edit to every other week and its notice, pause and resume
  twice, an answer without `--asked-for` and the day it lands on, one for a
  reminder's day and one for a day not asked, somebody else's answer refused as
  404, a page of answers over two pages, and a reminder closing once answered.
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
  how the twelve schedules read in `repetition`, `todosWithASeries` and
  `repetitionsNotMapped`.
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
- Check the to-do lists and groups live, as the [live check](../guides/live-check.md)
  says: a list started, changed, archived, written to in the archive, brought
  back and trashed, a group added, renamed, moved next to a group of another
  list and trashed, and a group of an archived list answering 404.
- Check the to-dos live, as the [live check](../guides/live-check.md) says: a
  to-do added into a group with days and people, a start alone coming back as
  the due day with its notice, the people replaced as a whole set and taken off
  with an empty list, a tick twice, a move into another group and into another
  list, a neighbour of another group refused, every write on a to-do of an
  archived list refused, the done to-dos read over two pages, `todos mine` with
  a subtask of one's own, and the subtasks added, named by position, moved and
  taken off twice. Look at `isOverdue` and at the cursor near midnight in a time
  zone that is not UTC, which the fake cannot show.
- Check the uploads of a Docs & Files tool live, as the
  [live check](../guides/live-check.md) says: a file put in with both steps, the
  tool read for its project, one with the hidden `--project` of the tool and its
  notice and one naming another project refused before the file travels, one
  with `--blob` after `uploads create --project`, a title left out, one
  refused as empty before the file travels, a folder of the archive refused
  after the file was stored and the retry with `--blob`, `fileInUse` for the
  same blob twice, `fileUnknown` for a blob of another project, a description
  with a mention, its change guarded and conflicting, `--description ""`, and a
  change of an upload of the archive. The uploads stay behind: there is no
  delete in the API, so a live run writes them into a project kept for tests.
- See the three refusals over whom a document reaches answered by a running
  instance: `notifyIdsMissing`, `notifyIdsWithoutChosenAudience` and
  `personCannotSeeIt`. All three are reachable with `docs create` now that the
  CLI judges the combination no longer, and the first two need nothing of a
  project but a Docs & Files tool. `personCannotSeeIt` needs a project that
  works with a client and a folder its clients are shown, which is the one of
  the three a live check has to be set up for.
- React to messages from `minka-ca`: check against the code, fake first,
  then the CLI, test and sabotage proof, check live, documentation in the
  same commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
