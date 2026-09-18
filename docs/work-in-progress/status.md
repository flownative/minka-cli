# Status

The first look of every new session. What holds is under `stable/`; here
stands where the work currently is.

## Where we are

The CLI serves what `/api/v1` offers: projects, boards, cards including the
trash, columns, assignees, due dates, steps, subscribers, comments, chats,
private conversations, uploads, mentions, reactions and one's own cards across
all projects. `minka commands` lists them all, built from the command tree
itself.

Everything is tested against the fake API and checked live — the chain
upload → attachment in the text → display as well as private conversations in
both directions. What is checked against the fake only is listed below.

Coordination of the API: the Minka session `minka-92`. It reports changes,
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
- "agent for X" in `people list` has been seen live, an agent that stepped
  down has not — it is in no list any more.

## Open in the API

- Renaming and reordering steps does not exist.
- Paging Done beyond 25 cards does not exist; needed once dogfooding needs it.
- Flow answers a wrong HTTP method with HTML instead of JSON.
- Column colours come later, read-only.
- In work at Minka: cards gain `shortId`, a string such as `MINKA-1K`, or null
  where the board has no prefix (E-138). `data` carries it without a change;
  the text of `cards show`, `cards mine` and `boards show` should show it. It
  is no address: the API finds a card by its UUID only.

## Next steps

- Install through Homebrew once and play `minka setup` through by hand on a
  terminal: the release workflow has built a version and written the formula,
  but nobody has installed from it yet, and `setup` is covered by tests only.
- Once the repository is public: the formula installs the archives, a release
  gains a build provenance attestation with instructions for verifying it, and
  the macOS binaries get notarised, which waits for an Apple Developer ID.

- React to messages from `minka-92`: check against the code, fake first, then
  the CLI, test and sabotage proof, check live, documentation in the same
  commit.
- Prepare the internal alpha: change the token, proofread the SKILL.md.
