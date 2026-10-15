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
6. Once the formula is written, Minka is told the new version, see [Telling
   Minka the version](#telling-minka-the-version).

The jobs that write to the tap and to Minka run in the environment `release`,
which holds the secrets of this repository: a deploy key that can write to the
tap and nowhere else, and a token that can set Minka's latest CLI version and
nothing else. The environment only deploys from tags `v*`; that, not the
workflow, is what keeps a branch with a workflow of its own from reaching the
secrets. Somebody who may push a tag `v*` can still reach them, with a release
from a commit whose workflow they changed: a required reviewer would close
that, and GitHub offers one for private repositories on the Enterprise plan
only. The tap carries other formulae as well, so who may write to this
repository is worth that thought.

## Telling Minka the version

Minka names the latest release of the CLI in the header `Minka-CLI-Latest`,
from which minka tells its users about an update (C-129). The job `announce`
sets that version once the formula points at it, so the notice never names a
version `brew upgrade` cannot install yet, and only for a stable release, the
same as the tap (C-131):

```
PUT https://minka.flownative.com/api/v1/operations/cli-version
Authorization: Bearer <MINKA_CLI_VERSION_TOKEN>

{"latest": "1.2.3"}
```

The token is the secret `MINKA_CLI_VERSION_TOKEN` in the environment
`release`, beside the deploy key of the tap. It is made on Minka with
`./flow cliversion:createtoken github-release`, which shows it once; it starts
with `minka_rel_` and may set the CLI versions and nothing else.
`./flow cliversion:listtokens` lists the tokens with their ids,
`./flow cliversion:revoketoken <id>` revokes one for good; a new token then
replaces the secret.

Minka answers 200 with `{"latest", "minimum"}`. A 401 means the token is wrong
or revoked: make a new one and replace the secret. A 422 (`versionInvalid`)
comes only for a tag Minka does not accept as x.y.z, with a leading zero or
more than six digits in a part, such as `v01.2.3`, which the tag check lets
through; the job then only warns.

Any 2xx answer is success. Transient failures and a refused connection are
retried a few times, a refusal by Minka is not; each try ends after 30
seconds. Whatever else happens — the secret is missing,
Minka refuses, or no answer comes (`HTTP 000`) — the job ends with a warning
on the run that names the HTTP status, and the release counts as done: the
archives and the formula are out. Until the version is set, the update notice
names the previous release. The warning names the catch-up:
`./flow cliversion:set --latest x.y.z` on Minka (`--minimum x.y.z` sets the
oldest fitting version as well, `./flow cliversion:show` shows both), or
running the job `announce` of that release again in GitHub, which GitHub allows
within 30 days of the run.

A failed `tap` means `announce` does not run at all. Re-running the failed jobs
of the run runs `tap` and then `announce`.

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
