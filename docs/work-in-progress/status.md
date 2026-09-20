# Status

The first look of every new session. What holds is under `stable/`; here
stands where the work currently is.

## Where we are

The CLI serves what `/api/v1` offers: projects, boards, cards including the
trash, columns, assignees, due dates, steps, subscribers, comments, chats,
private conversations, uploads, mentions, reactions and one's own cards across
all projects. `minka commands` lists them all, built from the command tree
itself.

`minka import basecamp` extracts the structure and the boards of Basecamp
projects into an import bundle for Minka's applier: people with avatars,
projects with members, the boards, chats and message boards as tools, and of
each board the columns, cards, steps, comments and boosts, with mentions and
files in the texts as references. It reads through the Basecamp CLI and is
tested against the invented account in `internal/fakebasecamp`. It has been
run live, as a dry run and into a bundle outside the repository, under an
identity that sees addresses.
A run needs a Basecamp identity that may see the e-mail addresses of others,
which is somebody of the account's own company or an admin. Under one that
may not, masked addresses were seen live ending the run as `forbidden`. Only
the people the chosen projects refer to come into a bundle.

Everything else is tested against the fake API and checked live — the chain
upload → attachment in the text → display as well as private conversations in
both directions. What is checked against the fake only is listed below.

Coordination of the API: the Minka session `minka-27`. It reports changes,
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
  Basecamp or rate limited, a creator who left the account, and that an
  address which is none comes out as `null` with its person named in
  `invalidAddresses`: the account holds such an address, but on nobody the
  projects extracted refer to, and it holds nobody retired. A run that is
  killed was not seen live either. Seen live: a missing Basecamp CLI, masked
  addresses ending the run, a missing project as `not_found`, people of the
  account being left out, and files that could not be fetched, with the bundle
  written without them and the run ending as `network`. The classes are the
  exit codes and codes the Basecamp CLI shows; its source was not read.
- Of the boards of `import basecamp`: a mention in a stored note that has no
  rendering, a mention of somebody who left the account, a boost on a card, a
  trashed comment, a card whose column is in no extracted board, more steps
  than a card shows itself, and a second system column. Seen live, in a bundle
  that was checked reference by reference: cards on hold, archived and trashed
  cards with their comments, completed cards ranked as Basecamp lists them,
  mentions resolved from a rendering, horizontal rules, and files whose signed
  ID is an older one than the list has, found by the key of the stored file.
  The fake mirrors both forms of a signed ID as they were seen.
- "agent for X" in `people list` has been seen live, an agent that stepped
  down has not — it is in no list any more.

## Open in the API

- Renaming and reordering steps does not exist.
- Paging Done beyond 25 cards does not exist; needed once dogfooding needs it.
- Flow answers a wrong HTTP method with HTML instead of JSON.
- Column colours come later, read-only.

## Next steps

- The bundle with the boards waits for Minka's applier of boards; what that
  run finds comes back through the coordination.
- The extractor goes on kind by kind, each a further step of the same walk:
  chats and posts (categories, posts, messages, comments, reactions), then
  the files of all texts: an attachment record and a download for every
  `<import-attachment>` a body carries.
- Install through Homebrew once and play `minka setup` through by hand on a
  terminal: the release workflow has built a version and written the formula,
  but nobody has installed from it yet, and `setup` is covered by tests only.
- Once the repository is public: the formula installs the archives, a release
  gains a build provenance attestation with instructions for verifying it, and
  the macOS binaries get notarised, which waits for an Apple Developer ID.

- React to messages from `minka-27`: check against the code, fake first, then
  the CLI, test and sabotage proof, check live, documentation in the same
  commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
