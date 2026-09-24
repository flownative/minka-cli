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
