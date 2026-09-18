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
- "agent for X" in `people list` has been seen live, an agent that stepped
  down has not — it is in no list any more.

## Open in the API

- Renaming and reordering steps does not exist.
- Paging Done beyond 25 cards does not exist; needed once dogfooding needs it.
- Flow answers a wrong HTTP method with HTML instead of JSON.
- Column colours come later, read-only.
- Archiving a project and moving it to the trash (E-129) is in place. A project
  in the trash answers 404 everywhere. An archived one stays readable at its
  addresses, but is missing from `projects list` and `me cards`; writing
  answers 404 `notFound`, for subscribers 422 `personNotAProjectMember`. The
  hint of the CLI for 404 does not name this case.
- Promised by the coordination, not merged yet: a write by a member into an
  archived project answers 422 `projectArchived` throughout, uploads included;
  `chats mark-read` stays allowed; the project resource gains `status`
  (`active|archived`). With the merge message: check against the code, adapt
  the fake, add a hint for `projectArchived`, and show `status` in the output.
  Until then the hint text stays as it is.

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
