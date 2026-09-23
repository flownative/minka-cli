# Status

The first look of every new session. What holds is under `stable/`; here
stands where the work currently is.

## Where we are

The CLI serves what `/api/v1` offers: projects, boards, cards including the
trash, columns with their area for cards on hold, assignees, due dates,
steps, subscribers, comments, chats, private conversations, uploads,
mentions, reactions and one's own cards across all projects. `minka
commands` lists them all, built from the command tree itself.

A file goes both ways: `uploads create` puts one in, `uploads download`
fetches its bytes back. Every text says which files hang in it, so reading a
card, a comment or a message gives the identifier a download takes, without
anything being taken out of the text.

`minka import basecamp` extracts the structure, the boards, the message
boards and the chats of Basecamp projects into an import bundle for Minka's
applier: people with avatars, projects with members, the boards, chats and
message boards as tools, of each board the columns, cards, steps, comments
and boosts, of each message board the posts of every status with their
categories, comments and boosts, and of each chat its lines as messages with
their boosts, written in parts of one chat and some 10,000 lines each.
Mentions in the texts are references, the preview of a link is the link, a
video or a player embedded in a text is an `<import-embed>` with its address,
a link to a card, a message or a chat of the account is an
`<import-reference>`, and the files of the texts and the files dropped
into a chat come with a record and their bytes, streamed from the
Basecamp CLI or taken over from the bundle the run replaces. It reads
through the Basecamp CLI and is tested against the invented account in
`internal/fakebasecamp`.
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

Everything else is tested against the fake API and checked live — the chain
upload → attachment in the text → display as well as private conversations in
both directions. What is checked against the fake only is listed below.

Coordination of the API: the Minka session `minka-14`. It reports changes,
which are checked against the Minka code; deviations go back to it.

Versions are released by publishing a release in GitHub and installed through
the Homebrew tap; `setup`, `doctor` and the skill inside the binary cover the
first steps.

Dogfooding waits for the internal alpha. The smoke token `minka-cli-smoke`
(account Bruno, local instance) gets revoked then.

## Checked against the fake only

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
  there shows short identifiers, so only the 404 was seen.
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
  file dropped into a chat without a usable key, and what ends a run: a line
  with a status other than `active`, a category without a name and a person
  without their times. Seen live, in a bundle that was checked reference by
  reference, its people against the kind Basecamp gives them: chats listed
  newest first in pages that grow, the four kinds of line, files dropped into
  a chat, bots and deleted people as writers who come retired although an
  admin's list holds them, boosts on lines, a line that says nothing left
  out, posts with and without a category, archived posts, a second message
  board with categories no list holds, previews of links and pictures at a
  foreign address as links, and five parts of one chat each.
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
- The fake API knows no archived card, so what Minka answers a move of one is
  not mirrored: that an archived card is named before a column without the
  area on hold is read in the Minka code alone.
- "agent for X" in `people list` has been seen live, an agent that stepped
  down has not — it is in no list any more.
- Of `uploads download`, everything: fetching a file has not been run live
  yet. The fake decides from the longer side of a picture whether there is a
  smaller rendering to show, as the server does, but what it renders is a
  shorter run of bytes and no picture. Not mirrored: the entity tag and the
  304 and 416 that go with it, the ranges a recording is played in, and that a
  recording keeps its own media type where every other download leaves as an
  octet stream. The CLI sends neither a range nor `If-None-Match`, so none of
  it is reached from here.
- That the bytes may come from a store of their own is shown for the client
  alone, against servers of a test: the token stays behind, an address that is
  not https is not followed, and five hops are the end. No command of the CLI
  has ever fetched a file over a redirect — the fake can answer one, but no
  test sets `StoreURL`, and a test that did would need a store speaking https.
  It waits.

## Open in the API

- A file may weigh 100 MiB, `uploads create` still refuses one over 50 MiB
  before it travels and `security.md` says so. To be set right together.
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

- React to messages from `minka-14`: check against the code, fake first,
  then the CLI, test and sabotage proof, check live, documentation in the
  same commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
