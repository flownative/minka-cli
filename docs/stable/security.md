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

- Redirects are never followed: a redirect would carry the token to an address
  nobody named. The one exception carries no token: the download of an avatar
  for an import follows at most five redirects, and only to https. A redirect
  that is refused is a refusal, not a failure of the network, and not
  retryable.
- Every ID is checked locally as a UUID — one that is not ends with exit 1,
  without a request going out — and additionally escaped in the path. A card
  may be named by its short identifier instead, whose shape is checked the
  same way; it is looked up once, and the command goes on with the UUID
  (C-022). What rides in the query (file name, content of a reaction) is
  escaped as well.
- Timeout 30 seconds, 10 minutes for the body of an upload. The CLI reads at
  most 16 MB of a response.
- What the server is sure to refuse, the CLI refuses first: a file over 50
  MiB, a directory, a call without a circle. That saves a useless transfer and
  gives a clearer error (C-013).
