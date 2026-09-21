# Importing from Basecamp

`minka import basecamp` is an extractor: it walks a Basecamp account and writes
a bundle in Minka's import format. The format is Minka's
(`Documentation/stable/02-architecture/import-format.md` there) and the only
thing the two sides share; Minka's applier reads the bundle, the CLI never
writes history into Minka itself.

```
minka import basecamp --project <id> [--project <id> …] --out <dir> [--dry-run]
```

The command talks to Basecamp only, and only reads there. It needs no Minka
host and no token.

## Through the Basecamp CLI

The extractor does not speak the Basecamp API itself. It runs
`basecamp api get <path> --agent` for every request (C-023): the Basecamp CLI
brings the sign-in, the refreshing of the token, rate limiting and retries,
and no Basecamp token ever passes through minka. The Basecamp CLI has to be
installed and signed in.

- The path stands behind `--` and `BASECAMP_NONINTERACTIVE=1` is set: nothing
  in a path is read as a flag, and a question would hang a run nobody watches.
- The Basecamp CLI answers one page per call and does not say whether another
  follows. Lists are read with `?page=1, 2, …` until a page comes back empty; a
  page that repeats the one before it ends the run instead of looping.
- Only paths of the account reach the Basecamp CLI. Where an answer names an
  absolute address, as a dock entry does, it has to lie under the address of
  the account and is turned into a path; anything else is refused, because the
  Basecamp CLI signs every request it makes.
- A project is named by its numeric ID of at most 18 digits. Anything else is
  a usage error before a request goes out and before a directory is made.
- The address of a project has to have the shape
  `https://<host>/<account>/projects/<id>.json`, and all projects of a run the
  same account: the account names the bundle, and its address is what every
  other address is held against.
- A run that is stopped is `interrupted`: when the context is done, or when
  SIGINT or SIGTERM ended the Basecamp CLI, which the signal may reach first.
  A Basecamp CLI that any other signal ended, such as the kill of a system out
  of memory, is an `api_error` that names the signal.

Its failures keep their class:

| Basecamp CLI | `code` | Exit |
|---|---|---|
| not installed | `config` | 1 |
| not found (2) | `not_found` | 2 |
| not signed in (3) | `auth_required` | 3 |
| forbidden (4) | `forbidden` | 4 |
| rate limit (5) | `api_error`, retryable | 7 |
| network (6) | `network`, retryable | 6 |
| anything else | `api_error` | 7 |

The exit code alone is no class: a program that crashes exits 2 as well. A
class counts only when the failure object `{ok: false, …}` stands on stdout
and its `code`, where it has one, says the same. Everything else is an
`api_error` that carries what the Basecamp CLI printed on stderr, cut to 500
bytes. Stdout is never quoted, only measured: it may hold the data of a
request that failed halfway, names and addresses included.

A bundle that cannot be written is `config`; an `--out` that holds something
else is `usage`.

What Basecamp says and a bundle cannot carry ends the run as `api_error`
instead of being passed on: a project status other than `active`, `archived`
or `trashed`, a project or a tool without `created_at` or `updated_at`.

## What the Basecamp identity needs

The run sees what the identity sees that the Basecamp CLI is signed in as. The
same holds for an unattended job, which needs such an identity of its own.

- It may see the projects named. One it may not see is `not_found`.
- It may see the e-mail addresses of other people. Basecamp masks them with
  `•` (U+2022) for an identity that may not, which is anybody who is neither of
  the account's own company nor an admin; only its own address comes through.
  A bundle with masks in it would make people nobody can sign in as.

The people list of the account is read first and checked as a whole, before a
project is asked for, a file is fetched or anything is written, in a dry run
as well, and whoever of it comes into the bundle later:

- One masked address, anywhere in it, ends the run as `forbidden`, exit 4, and
  the hint names what the identity lacks. The same holds for the creator of a
  record who is not in the list.
- An address looks like one when it is something without whitespace, one `@`,
  and a domain of at least two labels made of letters of any script, digits
  and hyphens, none of them empty. Whitespace around it is taken away first.
- An address that is none without a mask is a flaw in the data of the source,
  which only its owner can repair there, and must not cost the import of an
  account. The person comes with `email: null`, which makes a person without a
  login; the address is not repaired. The report names them under
  `invalidAddresses`, by `sourceId` only and in the order of the numbers, a
  `notice` says how many, and the run exits 0: the bundle is whole and a
  repeat would change nothing. The same holds for a creator.
- When more than half of the addresses of the list are none
  (`sourceProblemShare`), that is no flaw in the data any more but an identity
  or a source that hides addresses, with another glyph than `•` for instance,
  and the account would come through as people without logins. The run ends
  as `api_error` with a hint to look at what the identity may see.

A message counts the addresses concerned out of those given. No output ever
names an address or whose it is, and `invalidAddresses` holds IDs, never names.

## What a bundle holds so far

The structure: `manifest.json`, `people.jsonl`, `projects.jsonl`,
`tools.jsonl`, `attachments.jsonl` and `files/`.

- **People** are the ones a record of the bundle refers to, and nobody else:
  the members of the projects named and the creators of what is extracted. The
  people list of an account is what somebody is looked up in, not what is
  written: most of an account has no place where only some projects move to.
  `peopleLeftOut` in the report counts them; `counts.people` is who was
  written. Referring to a person is the one way into the bundle, so every
  reference leads to a person who is written. People are written at the end
  of the run, when it is known whom the records refer to.
  Who is in the account's list comes with `retired: false`, and so does a
  member of a project. Somebody who only turns up as the creator of a record
  has left the account and comes as the record shows them, with
  `retired: true`. An empty e-mail address becomes `null`, and so does one
  that is none, see above.
- **Avatars** are attachments of their person, `avatar-<personId>`, downloaded
  without a sign-in and only for people of the bundle. Basecamp serves a
  picture it made from initials at the same kind of address, with the same
  headers and the same type as one somebody uploaded, so they cannot be told
  apart and every avatar is taken; the report says so
  (`avatars.initialsDistinguishable: false`). The picture is stored as it
  comes, without converting it. `file` is relative to `files/`.
- **Projects** come with their members. Basecamp names no creator, so
  `createdBy` is `null`. The description is plain text there and HTML in the
  bundle: escaped, a paragraph per block, a `<br>` per line break.
- **Tools** are the dock entries that are switched on and that Minka has a
  counterpart for: `kanban_board` → `board`, `chat` → `chat`, `message_board`
  → `posts`, several of a kind where a project has several. Creator and
  timestamps come from the tool itself. Every other entry is counted as
  skipped, per kind and whether it was switched on, and shown in the report.

IDs are the numeric IDs of Basecamp as strings; `sourceInstance` is the ID of
the account, taken from the address of a project. Every file is ordered by
numeric ID, whatever order Basecamp answers in: two runs over an unchanged
account write the same bytes, apart from `extractedAt`.

A further kind is a further step of the walk that adds its records to the same
writer, feeds the creators it meets into the same people and attaches its
files the same way as an avatar is attached.

## A run

- The bundle is built in a directory next to `--out`,
  `.<name>.staging-<random>`, and takes its place with a rename at the end;
  the bundle of the run before is moved aside first and removed last (C-024).
  A run that fails or is interrupted, by Ctrl-C or by SIGTERM, leaves what was
  there: the earlier bundle, or nothing, not even the directories it made on
  the way to `--out`.
- When the new bundle cannot take its place, the old one is put back; when
  that fails too, the error names where it lies. An old bundle that cannot be
  removed afterwards is a `notice` with its path, not a failure.
- A run that is killed cannot tidy up. The next run into the same `--out` does:
  it removes what was being built, which is personal data nobody will use, and
  puts a bundle back that was moved aside when no new one took its place. It
  touches only real directories named exactly `.<name>.staging-<digits>`, with
  or without `.previous`: the name of `--out` never works as a pattern, and a
  link is left alone.
- One run at a time works on an `--out`. A run holds an advisory lock of the
  kernel (`flock`) on `.<name>.lock` next to it, from before the tidying up
  until the bundle is in place, and removes the file at the end. A second run
  is refused as `config` with the path of the lock. The kernel drops the lock
  with the process, so a run that was killed locks nobody out; it leaves the
  empty file behind, which the next run takes over and removes. minka is
  released for macOS and Linux; on a system without `flock` it builds and
  does not lock.
- An `--out` that is a symbolic link is refused as `config`: the bundle would
  take the place of the link, and the one behind it would stay where nobody
  looks for it.
- A bundle of an earlier run in `--out` is replaced as a whole. It counts as
  one only when its `manifest.json` says `format` 1 and the same `source`: a
  directory with anything else in it is refused, a web application with a
  manifest of its own and a bundle of another source included, and the refusal
  names the first entries that are in the way. The check
  happens before Basecamp is asked and again before the replacing.
- `--dry-run` walks Basecamp the same way, downloads no avatar, writes nothing
  and reports the same counts. It needs no `--out`; given one, it checks it
  the way the real run does, so the refusal shows beforehand.
- The report carries `counts` as the manifest has them, `avatars`,
  `skippedTools`, `missingFiles` and the absolute path of the bundle.

## Files that cannot be fetched

One file that is not to be had does not cost a run (C-025). Its attachment
record is left out, whatever pointed to it points to nothing (`avatar: null`),
and the bundle is written and put in place: it is whole and can be applied.
`missingFiles` names each file with `attachmentSourceId`, `ownerKind`,
`ownerSourceId`, `reason` and `retryable`. The rule holds for every file of a
bundle, not for avatars only. Only an interruption ends the run.

Such a run fails, so that a caller notices: `network`, exit 6 and retryable
when at least one file failed for a reason that may pass by itself, else
`api_error`, exit 7. The report is in the output all the same: as `data` of
the error envelope with `--json` and `--quiet`, as text before the error
otherwise. A later run replaces the bundle and brings what it can get then.

A file is fetched over https only, redirects included and at most five of
them, with a timeout of 30 seconds and at most 5 MiB, which is enforced while
reading. It has to be a picture of a known type: an error page must not end up
in a bundle. A reason never carries the address of the file, which names a
person.

## A bundle is personal data

A bundle holds names, e-mail addresses and pictures. Its directories are 0700
and its files 0600. It is written outside of any repository and deleted once
it has been applied. Nothing out of a real bundle appears in the repository:
not in a test, a fixture, a document or a commit message. Tests run against
`internal/fakebasecamp`, an invented account, and never call the Basecamp CLI
or the network.
