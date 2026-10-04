# Releases

How the CLI gets from `main` to a machine.

## Principles

- Installing never means piping a script from the network into a shell.
  Homebrew installs and upgrades; by hand, somebody downloads an archive,
  checks its checksum and puts the binary on the `PATH` (C-020).
- A release is defined in GitHub: publishing a release with a tag `vX.Y.Z`
  builds it. There is no release from a laptop.
- Versions follow semantic versioning and stay at 0.x while the API calls
  itself unstable. The tag carries a `v`, the version inside the binary and in
  the formula does not.

## What publishing a release does

`.github/workflows/release.yml` runs when a release is published.

1. It refuses a tag that is not a version, and a tag that does not stand on
   `main`.
2. It runs the checks of a pull request once more: tidy dependencies,
   `make check`, known vulnerabilities.
3. `make dist` builds `darwin/arm64`, `darwin/amd64` and `linux/amd64` without
   cgo, with the version set at build time, as
   `minka_<version>_<os>_<arch>.tar.gz` with a `checksums.txt` (SHA-256), and
   the workflow makes sure the binary reports that version.
4. The archives are attached to the release. Archives that are out are never
   replaced: attaching a second time fails.
5. Unless the release is a prerelease — by its checkbox or by a `-` in its tag
   — `Formula/minka.rb` in `flownative/homebrew-flownative` is written from
   `.github/formula/minka.rb.tpl` and pushed. A version older than the one the
   tap offers is refused.

The job that writes to the tap runs in the environment `release`, which holds
the one secret of this repository: a deploy key that can write to the tap and
nowhere else. The environment only deploys from tags `v*`; that, not the
workflow, is what keeps a branch with a workflow of its own from reaching the
key. Somebody who may push a tag `v*` can still reach it, with a release from a
commit whose workflow they changed: a required reviewer would close that, and
GitHub offers one for private repositories on the Enterprise plan only. The
tap carries other formulae as well, so who may write to this repository is
worth that thought.

## The formula

While the repository is private the formula builds from source: its `url` is
the Git address over SSH, pinned to the tag and its commit, with Go as a build
dependency. A formula cannot download release files of a private repository
without a token on every machine; over SSH, everybody who may read the
repository can install (C-021). Once the repository is public, the template
changes to the archives and their checksums.

It is a formula, not a cask: a formula sets no quarantine attribute, and Go
signs arm64 binaries ad hoc, so the binary runs without notarisation.

The formula installs shell completions and tells, as caveats, to run
`minka setup` and, after an upgrade, `minka skill install` and `minka hooks
install`.
