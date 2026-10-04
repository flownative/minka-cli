# Token, hosts and requests

A token is a key to an account. The CLI treats it as one: it is kept only in
the configuration file, goes only to its host, and must not show up anywhere
else — not in the repository, not in a commit, not in a test, not in a log
line.

## Sign-in

- The token never comes through a flag: it is read from the terminal with
  hidden input, or from stdin with `--with-token`. A flag would end up in the
  history and in `ps`.
- For `--with-token`, stdin must not be a terminal; the token is a single line
  and limited to 4096 bytes.
- Before saving, the CLI checks it against `/me`, so that no useless token
  lands in the file.

## Configuration file

`$MINKA_CONFIG`, else `$XDG_CONFIG_HOME/minka/config.json`, else
`~/.config/minka/config.json`.

- File 0600 in a directory 0700, written atomically with `fsync` and `rename`;
  a symlink stays a symlink.
- A file that others can read is refused instead of silently used.
- It holds several hosts with one token each, and a default host.

## A token goes to its own host only

`MINKA_TOKEN` counts only together with `MINKA_HOST` and only for that host.
Whoever addresses another host with `--host` gets that host's token from the
configuration file, or a refusal — never the token of the first host (C-004).

## The Basecamp token of an import

`import basecamp` leaves Basecamp's sign-in to the Basecamp CLI, and no
Basecamp token passes through minka (C-023), with one exception, and only with
`--upload-versions` (C-104): an earlier file of an upload, which no call of the
Basecamp CLI brings.

- The token comes from `basecamp auth token --agent`, asked anew for every
  file. It is held in memory for the one request and never written: not to a
  file, the bundle, a log, a report or a message. A failure of that call is
  reported in minka's own words and quotes nothing the Basecamp CLI printed,
  since that may be the token.
- It is sent to `3.basecampapi.com` alone, over https, to the address of one
  version, held to that shape right before the request. That request follows
  no redirect.
- The place Basecamp redirects to has to be `storage.basecamp.com` over https,
  no other port, with a bucket and a key; it is asked without the token and
  follows no redirect either.
- Without `--upload-versions` nothing of this happens.

## Host names

`https://` is added, `/api/v1` and a default port are removed. `http://` is
accepted for loopback only. Other paths are refused, and so is a value that
names no machine. What may stand in a host is what a name and an address are
written with: letters in any alphabet, digits, `-`, `.` and `_`, or, in
brackets, an IP address, with the interface of a link-local one behind a
percent — and a port behind it all. The line is drawn there and no closer: an
underscore, an empty label and a dot at the end go through, because they name a
machine somebody may really be reaching. A registered name may carry the
sub-delimiters of a URL by the letter of the standard, so
`https://minka.example.com;t=x` parses as one — and is refused, because nothing
answers at it while it would become a key of the configuration and of the list
of cards.

This holds for the default host of the configuration as well, whoever wrote it
there, and a value that cannot be read at all is no host: it is refused where
it is read, and no session comes about under it (C-044). The host is a key of
the list of cards, so every process has to make the same string of it, and a
value no process can make a host of is nothing to carry further: carried
further, it stands in a status, in a report and in the hint of a 401. The
refusal names the file to repair and the way out. Where the configuration keeps
the same host in another form, the refusal says which entry that is: the token
is there, it is only written where nothing looks for it.

Signing out is the one command that takes the entry as the file writes it, the
host it was told as much as the default one. It has to reach the entry that is
there: one written in another form would be left standing with its token by a
host that was read straight, and one that cannot be read at all is matched
letter for letter. Nothing goes anywhere from here, and no command lists the
hosts, so an entry that could not be signed out of would be invisible and
unremovable with its token valid on the server.

A host is said back to whoever gave it, and from there it travels into a
transcript, a ticket and a log. What is printed of it is the address alone:
scheme, host, port and path, put together from the parts where the value can be
read and nothing of the address still carries an `@`, and the host only as far
as it reads as one. Everything else is shown short, because the parts then say
nothing dependable about where the user information ends: everything up to and
including the last `@` is replaced, a separator written percent-encoded counts
as one, and of what follows only as much as leads up to a path, a query or a
fragment. The fault itself is named in words, so the refusal stays as clear as
it was.

A host that a session carries has been through this reading, so printing it
needs nothing further: `auth status`, the report of `doctor` and the hint of a
401 name it as it stands, and there is nothing in it to render away. Rendered
is what has not been read where it is printed, and those are five places: the
refusal of a host that was given, the refusal of the default host, the entry a
sign-out takes away, the entry a refusal names as the same host written another
way, and the hint after a request that could not be made, where the host
arrives as a bare argument from wherever the caller had it. Whoever prints a
value that has not been read adds a sixth.

A hint is a command line. It names the address without the credential, and
where nothing of the value is left it names what is to be filled in: a plain
word a shell hands on, and no address of its own, which whoever runs the line
would reach instead of their own instance.

## Requests

- A request that carries JSON never follows a redirect: it would take the
  token to an address nobody named. Two requests do follow, at most five
  redirects each, and neither takes the token anywhere new. The download of an
  avatar for an import carries none to begin with and goes only to https. The
  bytes of a file follow the redirect Minka answers with where a store serves
  them itself: the `Authorization` header comes off for every address that is
  not the host signed in to, and such an address has to be https (C-033). Go
  leaves the header on within a domain, a host below it included, so it is
  taken off here rather than left to the library. A redirect that is refused
  is a refusal, not a failure of the network, and not retryable.
- Every ID is checked locally as a UUID — one that is not ends with exit 1,
  without a request going out — and additionally escaped in the path. An ID
  typed in capitals is written in lowercase before it is compared or sent, in
  the path, the query and the body alike. Lowercase is the form Minka keeps
  an ID in. From Minka 00bbcf39 on, not yet released, it reads an ID in any
  case and names the same thing by one in capitals; a released Minka reads no
  other and names nothing by one in capitals — a person named so is refused
  as no member, and a step or a subtask named so is not deleted, with a
  success all the same. The name of a draft of a chat message is refused in
  capitals by both (C-083). A card
  may be named by its short identifier instead,
  whose shape is checked the same way; it is looked up once, in capitals,
  and the command goes on with the UUID (C-022). What rides in the query (file
  name, content of a reaction) is escaped as well.
- What reaches the command line of the Basecamp CLI during an import is a
  path of the account, or the storage address of a file. minka builds that
  address: scheme, host, the account of the run and the place of every part
  are its own. Out of an answer come the key of the stored file, which has
  to have the shape of a UUID, and the name, which is the last segment of
  the address Basecamp gives the file and must not, with its escapes taken
  away, hold `/` or `\` or be `.` or `..`: the Basecamp CLI asks for the
  path without escapes and signs what it requests, so such a name would
  lead to another path, as far as another account. No host and no path of
  an answer is passed on. Both kinds of argument stand behind `--` and are
  held against a strict pattern right before the call. See
  [Importing](import.md).
- The download of a file for an import has no limit of size and is streamed
  to disk. It is ended when no byte comes for 60 seconds, and after an hour
  at the latest.
- Timeout 30 seconds, 10 minutes for the body of an upload or a download. The
  CLI reads at most 16 MB of a response; the bytes of a file are streamed to
  disk instead, and one byte more than the file says it weighs is read, so
  that an answer which does not stop is noticed rather than kept.
- A file that is fetched is created with `O_EXCL` and 0600 and never written
  over a file that is already there; `--force` takes that entry away first —
  the link itself, never what it points at — and a directory under the name is
  refused. The name out of an answer has to be a plain name: one carrying `/`
  or `\`, or being `.` or `..`, is refused instead of cut down, because it
  would lead somewhere other than where the command was run (C-032).
- What the server is sure to refuse, the CLI refuses first: a file over 100
  MiB, a directory, a call without a circle. That saves a useless transfer and
  gives a clearer error (C-013).
