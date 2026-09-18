# Releasing a version

The rules behind this are in [Releases](../stable/releases.md).

## Before

- Everything that belongs to the version is merged, and the checks on `main`
  are green.
- Decide the number: a fix raises the patch, anything new the minor. While the
  version is 0.x, a change that breaks the output contract raises the minor
  too and gets a line in the release notes that says so.

## Releasing

1. In GitHub: Releases → Draft a new release.
2. Choose a tag: type `vX.Y.Z`, create it on publish, target `main`.
3. Generate release notes, and rewrite them into what somebody who uses the
   CLI wants to know.
4. For a version to try out, tick "Set as a pre-release" and use a tag such as
   `v0.2.0-rc.1`: the archives are built, Homebrew is left alone.
5. Publish.

## After

1. Watch the workflow "Release" in Actions. The archives and `checksums.txt`
   appear on the release; the tap gets a commit "minka X.Y.Z".
2. Try it:

   ```bash
   brew update
   brew upgrade minka      # or: brew install flownative/flownative/minka
   minka version
   minka skill install
   minka doctor
   ```

## When something goes wrong

- The workflow failed before the archives were attached: fix the cause on
  `main`, delete the release and its tag, release the same number again.
  Nobody has that version yet.
- The archives are attached but the tap was not written: run the failed job
  again from Actions, not the whole workflow — the build would fail at archives
  that are already out. Writing the formula twice changes nothing, and an
  older version than the tap offers is refused.
- A released version is broken: release the next patch. A version that
  Homebrew has offered is never replaced.

## Installing by hand

```bash
gh release download vX.Y.Z --repo flownative/minka-cli --pattern '*darwin_arm64*' --pattern checksums.txt
shasum -a 256 --check --ignore-missing checksums.txt
tar -xzf minka_X.Y.Z_darwin_arm64.tar.gz
mv minka_X.Y.Z_darwin_arm64/minka ~/bin/
```
