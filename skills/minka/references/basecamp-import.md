# Importing from Basecamp

```bash
minka import basecamp [--project <basecampProjectId>... | --active-only] (--out <dir> | --dry-run) [--calendars] [--upload-versions] [--keep-files] [--agent <basecampPersonId>=<basecampPersonId>...] --json
```

- `import basecamp --out <dir>` extracts projects of a Basecamp
  account into an import bundle, the directory Minka's applier reads foreign
  history from. It talks to Basecamp only, and only reads there; it needs no
  Minka host and no token.
- It runs the Basecamp CLI, which has to be installed and signed in. Without
  it the command exits 1 (`config`), without a sign-in 3; the `hint` names
  `basecamp auth login`. A project that Basecamp does not show exits 2.
- Exit 4 with "masks e-mail addresses" means the Basecamp identity that is
  signed in may not see the addresses of other people. Nothing was written and
  trying again does not help: tell the person that the run needs an identity
  of the account's own company, or an admin.
- Without `--project` the run takes every project the Basecamp identity can
  see, the active and the archived ones; an archived project comes with
  `status` `archived`. `--active-only` leaves the archived ones out. Projects
  in the trash are never taken. The projects stand in the bundle in the order
  of their IDs. An identity that sees no project (no active one with
  `--active-only`) exits 2.
- `--project` takes the numeric ID of a Basecamp project, names exactly the
  projects to take whatever their status, and is repeated for several.
  Anything that is no number exits 1 before any request, and so does
  `--project` together with `--active-only`.
- `data.invalidAddresses` lists, by ID only, the people who come without an
  e-mail address, and so without a login, because the one Basecamp holds is
  none; `notice` says how many. The run still exits 0 and running it again
  changes nothing: only somebody with access to the account can repair the
  address there. Never look the address up to quote it.
- Every address a bundle carries — a person's `email`, the sender of a
  forwarded mail, the sender of an answer by mail — is read the way Minka reads
  it, no wider: one that would stop the applier is left out here instead. One
  pair of quotation marks around the whole of an address comes off before it is
  read, which is the one thing a run repairs rather than passes on;
  `data.addressesUnquoted` counts the records it happened to — one mailbox
  written that way in many records counts in each of them — a `notice` says how
  many, and no output names an address.
- `data.creatorProbesRefused` counts the tools asked for who created a project
  that Basecamp refuses this identity. They are passed over and a `notice`
  says how many: the run exits 0, and a project may come without a creator
  because of it. An identity that may see nothing at all still exits 4.
- So far a bundle holds the structure, the boards, the message boards, the
  chats, the check-ins, the to-dos, the forwarded mail and the Docs & Files:
  the people the
  projects refer to
  (members and whoever a record names; `data.peopleLeftOut` counts the rest of
  the account's list, which holds more for an admin than for anybody else, the
  people written do not differ) with their avatars, the projects with their
  members, the card tables, chats, message boards, questionnaires, to-do
  sets, inboxes and Docs & Files tools that are switched on, of each card table
  the columns, the
  cards with
  the archived and the trashed ones, their steps, comments and boosts, of each
  message board its messages as posts, the archived and the trashed ones
  included, with their categories, comments and boosts, of each chat its lines
  as messages with their boosts, of each questionnaire its questions with
  their answers, comments and boosts, of each to-do set its lists with
  their groups and to-dos, of each inbox its forwarded mail with the answers
  by mail, the comments and the boosts of each, and of each Docs & Files tool
  its folders, documents and uploads with the comments, the boosts and the files
  of each. `data.skippedTools` counts what
  was left out, per
  kind and whether it was switched on; `data.counts` is what the manifest
  says.
- A person comes with `retired: true` unless Basecamp calls them a user:
  somebody who was deleted and an outsider are authors and nothing more,
  whichever identity ran the import. A chatbot or integration, a deleted one
  included, comes with `kind: "bot"` and without an address or `client`, a
  deleted one with `retired: true`; `data.bots` counts them.
  `data.botsAlsoNamedElsewhere` names deleted people who wrote as an
  integration and as a person, and a `notice` says so: Minka never turns a bot
  back into a human, so look at them before the bundle is applied.
- What the source says about clients comes with the bundle: `client` on every
  person, `company` on a client alone, `clientAccess` and `clientCompany` on a
  project, and `visibleToClients` on a tool, a post, a to-do list, a check-in
  question and every folder, document and upload of Docs & Files. That word is
  always the record's own and never the word of its tool:
  a post of a board the clients see can be one the source keeps from them. Where
  the source says nothing, nothing is written and Minka leaves what it holds
  alone. A record that follows what it hangs on — a to-do, a group, a card, an
  answer, a comment — carries none, and `data.visibilityLeftToTheContainer`
  counts the words the source offered there. `data.peopleAsClients` counts the
  clients and `data.clientMemberships` the memberships Minka leaves out, because
  a client is no member of a project there; for the same reason no question is
  said to be asked of a client, which `data.askedLeftOut.clients` counts. A
  company the source names without an identifier ends the run where the record
  that names it comes in: written away as none it
  would tell Minka to drop the company it holds. An Email forwards tool carries
  its word, and a mail and an answer follow it. **An entry of Docs & Files is
  the one exception to the rule above:** a folder, a document and an upload
  carry the word at **every** depth, and not only at the top of their tool,
  because Minka reads it below the top as well — something hidden from clients
  below a folder the source shows makes Minka hide that folder with everything
  in it. An entry inside a folder that the source says nothing about is named in
  `data.docs.silentAboutClientsInsideAFolder` — the kind, the `sourceId` and the
  project, never a title — with a `notice`: nothing is written for it, so it can
  hide nothing above it, and where the folder at the top of its tool is shown to
  clients that word was the one thing which would have. It is named rather than
  counted because the only thing to do about it is to compare it against the
  source. No output ever names a person or a company.
- `--calendars` brings the calendar of each project (Basecamp's Schedule) as a
  tool of `kind` `calendar` and its entries as events. Only use it for a Minka
  whose import applies events: any other stops at the first calendar, which is
  why a run without it skips the calendar like a kind Minka has nothing for and
  says nothing of calendars. A timed event carries its two moments, an all-day
  one its first and last day; an entry that repeats is one event with its
  schedule. `data.calendars.calendars[]` counts per calendar what was written,
  the trashed ones, those that repeat and those that come with their series,
  names in `repetitionNotMapped` each part of a schedule that did not fit (the
  event then comes once), and in `allDayInInstants` and `timedInDays` the
  entries left out because the source says their days in moments or the other
  way round, and in `timesMinkaRefuses` those left out because they end before
  they start or start before the year 1. `data.calendars.commentsUnderSingleDays` counts the comments
  written under one day of a series, which a bundle has no place for. An entry
  on the shelf or drafted is in `data.recordsLeftOut`. A run never asks for a
  single day of a series: Basecamp makes a record of the day where somebody
  does.
- `data.cardsByStatus` splits the cards, `data.cardsOnHold` counts those set
  aside within their column, `data.cardsLeftOut` the archived and trashed
  cards whose column is in no extracted board, `data.commentsTrashed` the
  comments left out because they are in the trash.
- `data.postsByStatus` splits the posts, `data.postsLeftOut` counts the
  archived and trashed posts of a message board that is not extracted,
  `data.postsWithEmptyBody` the posts that are a title alone. Basecamp hands
  out no drafts and tells of no pin, so every post comes with `pinnedAt`
  `null`.
- `data.messagesByType` splits the messages by what a line was in its chat:
  `text`, `richText`, `upload`, `integration`; `data.messagesWithEmptyBody`
  counts the lines somebody wrote that say nothing, which are left out. A
  line that was files alone becomes a body of references to them;
  `data.uploadLinesLeftOut` counts the lines of which Basecamp names no
  file, which are left out and which a `notice` counts as well. Lines
  somebody deleted and the lines Basecamp writes itself are not in its API.
- A to-do set becomes a tool of kind `todos`. Its lists stand in the bundle from
  the bottom up where they are in use, because Minka puts every new list on top,
  the ones the source has put away standing before them as it lists them, where
  their order decides nothing, and the groups and the
  to-dos of a list in the order of the list. `data.todoListsByStatus` splits the
  lists and `data.todosByStatus` the to-dos. A
  list knows the shelf and the trash, a group and a to-do the trash alone: one the
  source has put on the shelf is left out and counted. `data.todosLeftOut` counts
  per kind (`lists`, `groups`, `todos`) what the bundle holds none of, whatever
  the reason, and `data.todoPlacesLeftOut` names every list and group that took
  records with it, with how many groups and to-dos fell with it; a `notice` says
  how many such places there were. A to-do can hang on a to-do set itself and
  stand in no list of it, which a bundle has no place for: it is left out,
  `data.todosWithoutAList` names it with its `sourceId` and its project, and a
  `notice` says so — move it into a list at the source and a later run carries it.
  Nothing hangs on a group,
  which the format has no word for; a list and a to-do carry comments, boosts and
  the files of their text. A to-do is never said to be older than the tick the
  source put on it, or Minka would never write the tick. Subtasks are not carried:
  `data.todosWithSubtasks` and `data.subtasksLeftOut` count what fell away, and a
  `notice` says so. A to-do that comes again at the source comes as an ordinary
  one, because Minka has no repeating to-do: the schedule stays behind and
  `data.todosThatRepeat` counts the ones a bundle carries, with a `notice`. Two differences a later run
  cannot repair, each with a
  `notice`: it leaves the people of a to-do as they stand in Minka
  (`data.todosWithPeople`) and reopens nothing it once ticked off
  (`data.todosNotDone`).
- An inbox becomes a tool of kind `forwards`, and its mails `forwards` with
  their `forwardreplies`. `title` is the subject, `fromAddress` the sender, who
  stays an address and never becomes a person, `fromName` always `null`, and the
  body the mail as the source holds it, line breaks and all: Minka turns those
  into paragraphs. `data.forwards.byStatus` splits the mails; what the source
  keeps on its shelf comes into the trash, where it can be brought back, and
  `data.forwards.fromTheShelf` counts it. A subject nobody wrote becomes
  `(no subject)` and one longer than 200 characters or over several lines is
  folded and cut short (`data.forwards.withoutASubject`,
  `data.forwards.subjectsChanged`). A mail whose sender is no e-mail address
  Minka takes is left out and named by ID in
  `data.forwards.withoutAnAddress`; a sender inside a pair of quotation marks
  comes out without them and is counted in `data.addressesUnquoted`. Where what
  a mail says would reach Minka as nothing at all, the run writes a line of its
  own into the body
  (`data.forwards.givenALineOfTheRun`), because such a mail would be lost on
  every run. A file that might still come is not that: the mail waits
  for it, and a later run brings both, while one Basecamp itself refuses is a
  file that never comes. A mail nobody is named for goes to the
  owner of the organization
  (`data.forwards.withoutAForwarder`). An answer by mail is a record of its own
  and no comment: it belongs to somebody of the account or, for one from outside
  it, to the address alone and never to both
  (`data.forwards.repliesFromOutside`); one naming neither is left out and named
  (`data.forwards.repliesWithoutASender`), and a boost on an answer is counted
  and not carried (`data.forwards.reactionsOnRepliesLeftOut`), whether the answer
  itself comes or not. Comments and
  boosts hang on the mail itself. A body longer than the 1,000,000 characters
  Minka holds is counted in `data.forwards.longerThanMinkaTakes` and comes as it
  is: Minka passes such a mail over on every run, with everything hanging on it.
  No output ever names a subject or an address.
- A Docs & Files tool becomes a tool of kind `files`, and what stands in it
  `folders`, `documents` and `uploads`. A folder stands in the bundle after the
  folder it is in. For an entry in use the order of the file decides, bottom up,
  because Minka puts every new one on top of its kind; for one that has left its
  place it decides nothing, Minka ordering an archive by the moment a record was
  put away, so those stand first as the source lists them. There is no position in
  the format. `data.docs.foldersByStatus`, `documentsByStatus` and
  `uploadsByStatus` split them. The status of an entry is its **own** and never
  the status of its folder: what stands in a folder the source archived or threw
  away carries `active` and goes along through the folder in Minka. A document
  carries its title, its text, its files, its comments and its boosts; a folder
  takes neither comments nor boosts, here as in Basecamp, so neither is asked
  for. An upload is one stored file beside a text, and its title is **what the
  source calls it**, which Basecamp fills from the name of the file and anybody
  can rename;
  the name of the file stands in where the source calls it nothing
  (`data.docs.titlesFromTheFileName`) and `(no name)` where there is neither
  (`data.docs.withoutAName`). A title Minka would refuse is folded to fit
  (`data.docs.namesMadeIntoATitle`), a name too long the way Minka shortens one:
  the extension stays and the stem is cut. With `--upload-versions` the files an
  upload showed before come as its `versions`, oldest first and the file it shows now last, each
  earlier one a file of its own (`data.docs.versions.earlierFiles`); a version
  Basecamp lists without its file is left out and named
  (`data.docs.versions.withoutTheirFile`), and an upload whose list of versions
  Basecamp would not hand out, or that does not say which file is current, comes
  with the file it shows alone (`data.docs.versions.historiesUnread`). Such an
  earlier file is fetched at its version's address with the token of the Basecamp
  CLI, which goes to Basecamp's API and nowhere else. Without the flag no
  version is asked for and the report has no `data.docs.versions`. An upload Basecamp names no stored file for — one in a
  cloud — is left out and named by ID (`data.docs.withoutAStoredFile`), with its
  comments and its boosts (`data.docs.commentsWithoutTheirUpload`,
  `boostsWithoutTheirUpload`), because no run will ever have bytes for it. What
  says so is the record and never the shape of the address: the bytes of an
  upload's file are asked for **by the identifier of the upload and its
  project**, the address the source gives being one no run follows. A
  document or a description longer than the 1,000,000 characters Minka holds is
  counted (`data.docs.longerThanMinkaTakes`) and comes as it is. A record that
  lost a file no run will bring is named (`data.docs.onlyAFileNoRunWillBring`),
  and the kinds end differently because a text is optional and a file is not: a
  document whose whole text was that file arrives **without words**, its title,
  comments and boosts with it; an upload has no purpose without its file and is
  **left out** with its comments and its boosts — its file gone at the source,
  above what Minka takes, in a cloud, or refused by Basecamp itself. A file that
  might still come is not that: `why` of its entry in `data.missingFiles` says
  `failedThisTime` or `notUnderstood`, the upload comes and waits for it, and a
  later run brings both whole. A dry run downloads nothing and cannot know a
  refusal, so it counts an upload a run leaves out. `data.docs.leftOut` counts per kind
  what the bundle holds none of, and `data.docs.placesLeftOut` names every
  folder and every tool that took entries with it. A folder the source has put
  away that would not say what stands in it is passed over and counted
  (`data.docs.foldersThatSaidNothingOfWhatIsInThem`): what is in use below it is
  in no bundle and nothing can count it. No output ever names a file,
  a folder or a document.
- A record the source describes with a word a bundle has none for is left out
  and the run goes on: a chat line of an unknown kind or with a status other
  than `active`, a column of a kind a bundle has no role for, a card, a
  post, a comment, a question or an answer with a status a bundle does not
  know, a draft among them, a group or a to-do the source has put on the
  shelf, a forwarded mail of a status that is none of the three, an answer
  by mail that says anything but `active`, and a folder, a document or an
  upload of Docs & Files whose status is none of the three;
  `data.recordsLeftOut` calls the mail and the answer `forward` and `reply`. `data.recordsLeftOut` names each with the kind of
  record, the field, the word the source said there, where it stood and how
  many records it cost, and a `notice` counts the records. A record whose times
  are missing or are no times still ends the run: it would be put in the wrong
  place. A time is a moment written one way, `2024-02-05T17:00:00.000Z` or
  `2024-02-05T17:00:00+01:00`: a relative word such as `now`, a time without a
  zone, and a moment of the right shape that no calendar has end the run, named
  with the record and the field, and so does a day that is not `YYYY-MM-DD`
  where a card is due, where a to-do starts or is due, or where an answer
  belongs. The message says what stood there only where it has the shape of a
  time and is short: a value of another shape is not repeated, because that field
  then holds whatever the source put in it, and a long one is not repeated
  because a message is no place to carry it.
- A title or a name Minka would refuse — empty, holding a control character such
  as a line break, longer than 200 characters (a step 500) — never ends the run:
  a control character becomes a blank, what is too long is cut and ends in an
  ellipsis, and nothing becomes `(no title)` or `(no name)`; a system column
  without a name keeps Minka's. `data.titlesMadeToFit` names each record by kind
  and `sourceId` with what was done (`none`, `folded`, `cut`), never the title,
  and a `notice` counts them. A to-do that starts after the day it is due comes
  without its start, named in `data.todosStartingAfterTheyAreDue`. A comment or
  a chat line loses the control characters Minka refuses
  (`data.textsCleaned`), a project description over 2000 characters is cut at
  a word with an ellipsis (`data.descriptionsCut`). A kind of post without an
  icon comes without an `emoji`, by its name alone.
- A question of a check-in carries the rhythm, the weekdays counted from Monday,
  the time of day, the day the rhythm is counted from and whom it is asked of,
  which are the subscribers of the question and not whoever answered it. Only a
  member of the project who is no client, is still there and has an address can
  be asked; `data.askedLeftOut` counts the rest by reason, one reason each and
  in the order just named. `data.nobodyAsked` names every question that comes
  asking nobody at all, which will ask nobody again. A question whose schedule
  Minka cannot express is left out with its answers, and `data.questionsLeftOut`
  names it with the field that made it impossible; one whose schedule has run
  out comes paused; one asked every other week is anchored on the day it was
  last answered, which `data.anchorsInferred` names, and left out where nobody
  ever answered it.
  Comments, boosts and files hang on an answer, never on a question.
  `data.answersWithoutABody` and `data.answersTrashed` count the answers left
  out, `data.answersWithoutADay` those Basecamp names no day for, which take the
  day they were written in the zone of whoever wrote them.
- A post, a card, a to-do, a document and an upload carry `subscribers`: whom
  the source says follow the item, by `sourceId`, in the order of their IDs.
  Minka takes the list as the whole truth and takes everybody else off the item,
  the writer included, so `[]` means nobody follows it. Only a member of the
  project who is no client, is still there and has an address is named;
  `data.subscribers.leftOff` counts the rest by reason, one reason each, and
  `data.subscribers.lists` the records that carry a list. An item in the trash
  carries none and is not asked. Where Basecamp answers the list as not found or
  forbidden the record carries no `subscribers` at all, Minka follows it as it
  did before lists came, and `data.subscribers.unread` names it (`record`,
  `sourceId`, `projectSourceId`, `why`: `notFound` or `forbidden`); a `notice`
  counts them, and the run still exits 0. An answer that is not plainly the
  whole list ends the run: no list or a `null` one, an entry that is `null` or
  has no ID, or a list whose length differs from the `count` beside it. So does
  any other failure.
- The messages are written in parts, `messages-0001.jsonl` and on, each with
  lines of one chat; a part ends once it holds 10,000 lines, where the second
  changes, so it may hold somewhat more. `data.parts.messages` and the
  manifest name them in their order. Only a bundle whose lines came to a
  single part carries `messages.jsonl` instead, and `data.parts` is missing.
- In the texts a mention and a file become a reference and the preview of a
  link the link: `data.filesInBodies` counts the files referred to,
  `data.filesDropped` the files Basecamp lists nothing for, of which only the
  caption stays and which a `notice` counts as well,
  `data.mentionsUnresolved` the mentions of somebody the account does not
  list any more, which stay a name, `data.linkPreviews` the previews that
  became links, `data.picturesLinked` the pictures at a foreign address that
  became a link to it; one whose address is none of the web leaves its title
  as text and is not counted. A video or a player embedded in a text, an
  `<iframe>`, becomes `<import-embed url="…"></import-embed>` in its place:
  `data.embeds` counts them, `data.embedsLeftOut` the frames that are left
  out because Minka would not take their address, which a `notice` counts as
  well. A line that was a video alone is a message; one whose only frame is
  left out says nothing. A link to a card, a message, a chat or an entry of
  a calendar of the account, or to a line of a chat or a day of an entry,
  becomes `<import-reference kind="…" ref="…" href="…">words</import-reference>`,
  a day naming its event, whether the record is in the bundle or not and
  with or without `--calendars`, with the address as it stood and the words
  of the link as plain text: `data.references` counts them by `cards`,
  `posts`, `tools` and `events`. `data.longestBody` is the longest text per kind
  (`cards`, `comments`, `posts`, `messages`, `answers`, `todolists`, `todos`)
  in characters, and
  `data.longestBodyRecords` names under the same kinds which record it was
  measured on (`record`, `sourceId`, `in`, `inSourceId`, `characters`), so that
  a number can be looked at at the source; a kind whose longest text has no
  characters is missing there. None of them fails a run.
- Every reference to a file has a record in `attachments.jsonl` with the same
  ID (`attachment-<id>` for a file of a text, `blob-<key>` for one dropped
  into a chat) and its bytes under `files/<id>/`, whatever their size and
  type. `data.filesExpected` counts the files the references name, each
  once, and `data.filesBytesExpected` what Basecamp says they weigh: that is
  all `--dry-run` knows of them, so run it first to see what a run would
  fetch. After a real run `data.filesFetched` were downloaded,
  `data.filesReused` taken over from the bundle the run replaced, which a
  run does by itself when the file there still has the size and the digest
  of its record, `data.filesBytes` is what they weigh together, and
  `data.filesGone` counts the files Basecamp lists and no longer has: they
  have a record with `gone: true` and a `notice` counts them. Every run asks
  for such a file again and brings it when it is back; nothing has to be
  done about one that stays gone. `data.fileNamesChanged` counts the names
  that had to be made plain, `data.captionsDropped` the captions over 1000
  characters that are left off their reference,
  `data.filesOfSeveralOwners` the owners a file turned up under after the one it
  was fetched for: every owner of a file gets a record of its own, named
  `<the file>.<kind>-<owner>`, with the same bytes and one download, because a
  file stands in one place in Minka and a record naming another's file would lose
  it where the two stand in different projects. So a bundle can hold the same
  bytes twice. It counts owners met and not records written, and
  `counts.attachments` is `data.avatars.count` plus `data.filesExpected` plus
  `data.filesOfSeveralOwners`, less the entries of `data.missingFiles` whose
  `ownerKind` is not `person`. None of them fails a run. No output says the name of a file, a
  caption or an address; do not quote them either.
- A request that fails for a reason that may pass (network, rate limit, a
  5xx, 408 or 429 of Basecamp or of a gateway in front of it) is asked again
  by the command itself, up to four times with pauses of 2, 10 and 30
  seconds, and at most 100 times in one run. `data.requestsRetried` counts
  those further attempts, 0 when none was needed; it fails no run. A run that
  still ends on such a failure, or because Basecamp kept failing, is
  `retryable` (exit 6 for the network, else 7): nothing was written, run it
  again later.
- `--dry-run` walks Basecamp the same way, downloads and writes nothing, and
  answers with the same counts and with what a run would fetch. `--out` is
  not needed then.
- The bundle is built next to `--out` and takes its place at the end: a failed
  run leaves what was there. An earlier bundle of Basecamp is replaced as a
  whole; a directory that holds anything else is refused with exit 1, by
  `--dry-run --out <dir>` as well.
- One run at a time works on an `--out`: a second one exits 1 (`config`) and
  names the lock the first holds. A run that was killed holds none. `--out`
  must not be a symbolic link.
- A file that cannot be fetched does not end the run: the bundle is written
  without it and can be applied, and `data.missingFiles` names each file with
  its owner, the `reason` and **`why`**, which says whose word the loss is and
  is the field to read:
  - `failedThisTime` — it may pass. Exit 6 and `retryable`, the report as `data`
    of the error envelope. Do not treat such a run as lost: run the same command
    again and it fetches what is missing and takes the rest over.
  - `refusedBySource` — Basecamp itself has no such file or hands it to nobody.
    Where **every** missing file says this, the run ends 0, with a notice naming
    them and the count in the `summary`. No run will bring those, the records
    that stood on one are already written without it or left out, and running
    again changes nothing: look at what is named and settle it at the source.
  - `notUnderstood` — a guard of the CLI refused, or an answer nobody could
    read. Exit 7, not retryable, wherever one of them is among the files.
    **Nothing is written off**: the records that stood on such a file keep
    waiting, and a later run brings it once the reason is gone. A few of them are
    a shape nobody has met; many at once mean this CLI or the Basecamp CLI it
    reads through has changed, which is the thing to look at — not the files.
  A download that brings nothing for a minute is given up and counts
  as missing, `failedThisTime`. A
  Basecamp CLI that lost its sign-in while files were fetched ends the run
  with exit 3 and nothing written, and an avatar answered with 401 ends it the
  same way — a sign-in that is gone is no word about a picture, and every person
  of the account would lose theirs. A run that ends that way, or is
  stopped, fetches its files again: only a bundle in place hands files
  over, unless the run had `--keep-files`.
- `--keep-files` keeps every file a run brings in `.<name>.kept` beside
  `--out` from the first file on, so a run that fails, is stopped or killed
  hands them to the next run with the flag, which takes over what is still
  whole (`data.filesReused`) and walks Basecamp anew. Use it for a long run.
  It needs a build whose version names its commit (`make build` in a clean
  checkout, or a release); `dev` or `-dirty` exits 1 (`usage`). Kept files of
  another build or another Basecamp account exit 1 (`usage`) with the path:
  run that build, or remove the directory. A bundle in place removes them.
  Wherever kept files lie after a run, `data.keptFiles` names them (also in
  the `data` of an error envelope) with a `notice`; a run without the flag
  leaves them alone and says so.
- `--agent <agent ID>=<human ID>` (temporary, for moving one account over)
  writes that Basecamp user as the agent of the other, without an address,
  and brings the human along. A bad value, an agent of themselves or of two
  humans, or a human who is also named as an agent exits 1 (`usage`) before
  anything is read; an agent or human who is no user of the account, or a
  client, exits 1 (`usage`) once the people list is read.
  `data.agents` counts them.
- A bundle holds personal data: names, e-mail addresses, pictures, and so do
  kept files. Write it outside of any repository, do not quote from it, and
  delete it once it has been applied, and kept files once no run is to take
  them over.
