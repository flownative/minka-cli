# Release concept

How the CLI gets from `main` to a machine. Agreed in outline, built in steps;
what is built moves to `stable/` and gets its entry in the register.

## Principles

- Installing never means piping a script from the network into a shell.
  Homebrew installs and upgrades; by hand, somebody downloads an archive,
  checks its checksum and puts the binary on the `PATH`.
- A release is defined in GitHub: publishing a release with a tag `vX.Y.Z`
  starts the build. Versions stay at 0.x while the API calls itself unstable.
- The skill travels inside the binary, so the description an agent reads
  always fits the commands it can run.
- Nothing writes into a home directory unasked. A stale skill is reported, not
  silently replaced.

## Releasing

1. Work reaches `main` through pull requests; the checks are green.
2. Somebody publishes a release in GitHub; the notes are generated from the
   pull requests.
3. A workflow runs the checks, builds `darwin/arm64`, `darwin/amd64` and
   `linux/amd64` with the version set at build time, and attaches the archives
   and a `checksums.txt` to the release.
4. The workflow writes `Formula/minka.rb` into `flownative/homebrew-flownative`,
   the way Local Beach does.

While the repository is private, the formula builds from source over SSH (`url`
is the Git address, pinned to tag and revision, with Go as a build dependency):
a formula cannot download release files of a private repository without a token
on every machine. Once the repository is public, the formula installs the
archives instead, and the release gains a build provenance attestation with
instructions for verifying it.

A formula, not a cask: a formula sets no quarantine attribute, and Go signs
arm64 binaries ad hoc, so the binary runs without notarisation. Notarisation
matters once people download archives with a browser.

## Installing and first steps

```
brew install flownative/flownative/minka
minka setup
```

- `setup`, `doctor` and the skill are built: see
  [First steps](../stable/first-steps.md) and [The skill](../stable/skill.md).

## Steps

1. Checks on pull requests, the way of working through pull requests.
2. Built: the skill inside the binary with its test against the command tree,
   `setup` and `doctor`.
3. The release workflow, the formula template, a guide for releasing.
4. A README that covers installing, first steps, use, sign-in, agents,
   configuration, troubleshooting, upgrading, verifying a release, development.

Robert merges pull requests.

## Open

- The release workflow writes to the tap with a deploy key of its own: the
  public half is a deploy key with write access on the tap, the private half a
  secret of the `release` environment here. It can write to the tap and
  nowhere else. Somebody with admin rights on both repositories creates it.
- An Apple Developer ID is not there yet; notarising waits for it.
