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

## Host names

`https://` is added, `/api/v1` and a default port are removed. `http://` is
accepted for loopback only. Other paths are refused.

This holds for the default host of the configuration as well, whoever wrote it
there. The host is a key of the list of cards and part of the name of a lock
beside it, so every process has to make the same string of a host, and a value
written by hand in another form would have one process looking at another list
than the next. A host that cannot be read at all is passed on as it stands, so
that what says so is the one command looking for a token under it and not every
command at once. Where the configuration keeps the same host in another form, the
refusal says which entry that is: the token is there, it is only written where
nothing looks for it.

A host is said back to whoever gave it, and from there it travels
into a transcript, a ticket and a log. What is printed of it is the address
alone: scheme, host, port and path, put together from the parts where the value
can be read and nothing of the address still carries an `@`. Everything else is
shown short, because the parts then say nothing dependable about where the user
information ends: everything up to and including the last `@` is replaced, a
separator written percent-encoded counts as one, and of what follows only as much
as leads up to a path, a query or a fragment. The fault itself is named in words,
so the refusal stays as clear as it was.

Every place that prints a host renders it this way first — a message, a hint, a
field of the envelope alike. This is a rule and not a list of places, and it has
to be one: as soon as a token stands under an entry written by hand, a session
comes about, and from there the value reaches anything that prints the host or a
URL built from it, an error text of the standard library included. Such a text
has the password replaced where the standard library wrote it, and nothing
printed beside it may put it back.

What the rule does not reach yet stands behind such a session: `auth status`,
`doctor`, the hint of a 401, `setup` and the search for a card print the host as
the file holds it, and the keepalive carries it in the argv of its child, where
every local process sees it. That is open, and it is named here rather than left
to be found. Inside the CLI the value stays as it was given anyway, as a key of
the list of cards and part of the name of a lock beside it.

A hint is a command line. It names the address without the credential, and where
nothing of the value is left it names what is to be filled in: a plain word a
shell hands on, and no address of its own, which whoever runs the line would
reach instead of their own instance.

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
  without a request going out — and additionally escaped in the path. A card
  may be named by its short identifier instead, whose shape is checked the
  same way; it is looked up once, and the command goes on with the UUID
  (C-022). What rides in the query (file name, content of a reaction) is
  escaped as well.
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
