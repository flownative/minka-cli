# Extracting from Basecamp live

A run of `minka import basecamp` against the real account. What the extractor
does and guarantees is in [Importing from Basecamp](../stable/import.md); here
is how a run is made on this machine and what belongs around it.

## What a run may do

It reads Basecamp and writes a bundle outside of every repository. Nothing is
ever written to Basecamp, and nothing of a bundle — a name, an address, a
title, a line, a file name — goes into the repository, a commit message, a
message to another session or the terminal. A bundle is checked with scripts
that print counts, kinds and identifiers only.

## The identity

The Basecamp CLI's default identity on this machine is no admin and not of the
account's own company, so Basecamp masks the e-mail addresses of others for it
and a run ends as `forbidden` right after the people are read. That identity
is good for looking at the shape of Basecamp's answers.

A run that writes a bundle goes under the profile Robert has set up for it:

```bash
BASECAMP_PROFILE=flownative-robert ./bin/minka import basecamp \
  --project 6162023 --project 6165736 \
  --out ~/minka-import/<bundle> --json > <scratchpad>/run.json
```

Robert has allowed that profile for extraction runs, reading only. It is his
own sign-in: nothing else is done under it. Under it the people list of the
account holds every kind of person, bots and deleted people included.

## The binary

Build with `make build` and run `bin/minka`. The firewall of this machine
holds back the network of a freshly built binary at any other path; the
symptom is every avatar timing out after 30 seconds, not an error. The
requests and the files go through the Basecamp CLI and are not affected, the
avatars are fetched by minka itself. While a run is going, nothing else writes
into `bin/`: an implementer or a reviewer at work is told so.

## How long, and how to wait

Two projects without their files are some 2,500 requests and 25 minutes, a dry
run the same. With the files, about 2,700 of them and 6 GiB, an hour or more
is to be expected; a run that follows takes unchanged files over from the
bundle it replaces. Start a run in the background and let it report when it
ends. The ten-minute limit of a background command does not end it.

A run that ends hard leaves the earlier bundle in place and loses the files it
fetched. A single failure of a gateway ends no run any more, the request is
asked again.

## The bundles and the coordination

| Path | Holds |
|---|---|
| `~/minka-import/basecamp-3923521` | the structure and the boards, as Minka's applier of boards ran it |
| `~/minka-import/basecamp-3923521-chats` | the posts and the chats as well, released to the coordination |

The format coordination copies from a bundle for its trial runs. Tell it
before a directory is written again, and again when it is ready, with counts.
Into a directory that is being tried out elsewhere, write a new run under
another name first and move it once it is checked.

## Checking a bundle

Every line parses; identifiers are unique per kind; every reference resolves
— `createdBy`, members, assignees, `completedBy`, the target of a reaction,
the parent of a comment, the category of a post, every `ref` of a mention;
`retired` agrees with the kind of person Basecamp gives; a part holds lines of
one chat, oldest first, and no second is cut in two; `bytes` and `sha256`
agree with the files; the counts of the manifest are the counts of the report.

Minka's fixture `ImportWithFiles` brings a `verify.sh`. Its output names file
paths, which is personal data with a real bundle: pass on the result of each
check and nothing else. Its check of unanswered references is wired to the
fixture and fails for any other bundle.

## Afterwards

What the run showed goes into the status, what it uncovered gets fixed at
once. Run outputs, probes and copies in the scratchpad are deleted; a bundle
is deleted once it has been applied.
