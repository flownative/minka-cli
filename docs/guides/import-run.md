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

Without `--project` the run takes every project the identity sees, the
archived ones included, which is the whole account and many times the time
below; `--active-only` leaves the archived ones out. A dry run over
everything is the step before the first full run: it costs almost the whole
walk, only the downloads are left out, and says how much a run would fetch. A
single failure anywhere ends a full run with nothing written. A `not_found`
names what is missing in its message; the hint to list the projects comes only
where a project is missing, a tool that answers 404 gets another one.

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
run the same. Files come at about 50 a minute and 10 GiB an hour: one project
with 1,500 files of 5.4 GiB took 33 minutes, both projects with 2,700 files
of 6 GiB take about an hour; a run that follows takes unchanged files over
from the bundle it replaces. Start a run in the background and let it report
when it ends. The ten-minute limit of a background command does not end it.

A run that ends hard leaves the earlier bundle in place and loses the files it
fetched. A single failure of a gateway ends no run any more, the request is
asked again.

A further run goes into a clone of the newest bundle rather than into an empty
directory: `cp -Rc <bundle> <new bundle>` is done in a second and costs no
space, the files being shared until one of them is written, and the run takes
them over instead of fetching them again. Both projects with their 2,700 files
come down to twenty minutes that way. The bundle that was cloned stays as it
is, which is what a bundle somebody else is trying out needs.

## The bundles and the coordination

| Path | Holds |
|---|---|
| `~/minka-import/basecamp-3923521` | the structure and the boards, as Minka's applier of boards ran it |
| `~/minka-import/basecamp-3923521-chats` | the posts and the chats as well, released to the coordination |
| `~/minka-import/basecamp-3923521-random` | one project whole, with its files and embeds, released to the coordination |
| `~/minka-import/basecamp-3923521-full` | both projects whole, with their files and embeds, applied on minka-2 and hard-linked into it: never written again, a new run goes into a new directory |
| `~/minka-import/basecamp-3923521-references` | both projects whole, with the links back into the source, released to the coordination |

The format coordination copies from a bundle for its trial runs. Tell it
before a directory is written again, and again when it is ready, with counts.
Into a directory that is being tried out elsewhere, write a new run under
another name first and move it once it is checked.

A bundle goes to the applier whole. What an earlier run of the applier passed
over and a later one brings lands at the end where Minka keeps an order of its
own, a line of a chat at the end of the chat. So the history of an account is
applied from the complete bundle, files and embeds included, into an
organisation that holds nothing of it yet.

## Checking a bundle

Every line parses; identifiers are unique per kind; every reference resolves
— `createdBy`, members, assignees, `completedBy`, the target of a reaction,
the parent of a comment, the category of a post, every `ref` of a mention;
`retired` agrees with the kind of person Basecamp gives; a part holds lines of
one chat, oldest first, and no second is cut in two; `bytes` and `sha256`
agree with the files; the counts of the manifest are the counts of the report.

Minka brings `Build/verify-bundle.sh <bundle>`, which checks the files of a
bundle against its index and prints identifiers and counts only. It only
reads, so it may be run although it lies in Minka's repository.

**Of the files, ask who owns them and not only whether they are found.** Neither
`verify-bundle.sh` nor a closure check over the references asks whether the record
that names a file is the record the index gives that file to, and a bundle where
the two differ reads as whole while a file is quietly lost (C-066). In the
bundle's own directory, and writing nothing into it:

```bash
probe=$(mktemp -d)
jq -r '.sourceId + " " + .ownerKind + " " + .ownerSourceId' attachments.jsonl |
  sort > "$probe/owners"

# Once per kind that carries a text, with the field that holds it: cards
# (description), comments, posts, messages or messages-*.jsonl, answers,
# forwards, forwardreplies, documents (body), todolists, uploads (description),
# todos (notes). The kind is the singular, as ownerKind writes it.
jq -r --arg kind post --arg field body \
  '.sourceId as $id | (.[$field] // "")
   | [scan("<import-attachment ref=\"([^\"]+)\"")[]]
   | .[] + " " + $kind + " " + $id' posts.jsonl | sort -u >> "$probe/named"

sort -u "$probe/named" | comm -23 - "$probe/owners"
```

Nothing there, or a body naming a file it does not own. Then how many records
carry a file another record carries too, which is `filesOfSeveralOwners` less
whatever the run could not fetch:

```bash
withBytes=$(jq -r 'select(.sha256) | .sha256' attachments.jsonl | wc -l)
files=$(jq -r 'select(.sha256) | .sha256' attachments.jsonl | sort -u | wc -l)
echo $((withBytes - files))
find files -type f -links +1 | wc -l    # names that share their bytes on disk
rm -r "$probe"
```

`verify-bundle.sh` counts the records that have a file and not the files
themselves, so it says nothing about how many of them are one file. The second
number is 0 while the first is not only where the bytes were copied instead of
linked, which happens where the file system will not have two names for one file.

## Afterwards

What the run showed goes into the status, what it uncovered gets fixed at
once. Run outputs, probes and copies in the scratchpad are deleted; a bundle
is deleted once it has been applied.
