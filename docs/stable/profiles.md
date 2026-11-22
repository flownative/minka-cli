# Profiles

A profile is one account in one organization: who (a human, an agent or a bot)
and where (the host). It has a name of its own choosing, and the configuration
file keeps per profile the host, the token and the account the token acted as
at sign-in (C-191). Several profiles may sign in to the same host: two
accounts in one organization.

## Names

Lower case letters, digits, `-` and `.`, starting with a letter or a digit, at
most 64 characters. Anything else is exit 1 (`usage`) with a hint. A name
never holds `:` or `/`, so it never reads as a host; the list of cards relies
on that (C-196).

A profile made without a name, by `auth login --host <url>` on a host with no
profile yet or from a file of hosts (below), is named after its host: the host
without the scheme, a port joined with `-` (`127.0.0.1-8081`), and `-2`, `-3`
behind it where the name is taken. The name is made of the host as far as it
may be printed, so a password written into an address never becomes part of
it.

## Which profile a command uses

Strongest first (C-192):

1. `--profile <name>`, then `--host <url>`;
2. `$MINKA_PROFILE`, then `$MINKA_HOST`;
3. the default profile.

A flag outweighs every variable of the environment. `--profile` and `--host`
together are exit 1. A host stands for the one profile on it; where several
are, for the default profile if it is among them, and otherwise the command
exits 8 (`ambiguous`) and names them. A host without a profile is a host not
signed in to (exit 3). An unknown profile is exit 1, with the profiles in the
hint. A default profile that is not in the file, or whose host cannot be read,
is exit 1 (`config`), never "no default".

`$MINKA_HOST` and `$MINKA_TOKEN` sign in without a configuration file. The
token belongs to `$MINKA_HOST`: it is used where the host of a command comes
from `--host` or `$MINKA_HOST` and is that host, and never where a profile is
named, even one on that very host (C-193); a `notice` then says that it was
not used. A profile is an account of its own, and the token of the environment
may be another one.

`auth status` names the profile and where the choice came from:
`data.profile` (null for the token of the environment), `data.profileSource`
(`--profile`, `--host`, `$MINKA_PROFILE`, `$MINKA_HOST`, `default`) and
`data.defaultProfile`, beside the fields it had before: `data.host`,
`data.hostSource` (`flag`, `environment`, `profile`, `default`) and
`data.defaultHost`, the host of the default profile. `doctor` carries
`data.profile` and `data.defaultProfile` beside `data.host` and
`data.defaultHost`, and its sign-in check says the same in words.

## Commands

- `auth login [--profile <name>] [--host <url>]`: a new name makes a profile
  and needs a host. An existing name signs that profile in again on its own
  host; another `--host` is exit 1 naming `profiles remove`, because a profile
  keeps its host (C-195). Here, and in `setup`, `--profile` and `--host` go
  together. Without `--profile` a host with one profile signs that one in
  again, a host with none gets a profile named after the host, and a host with
  several is exit 8. `data` adds `profile` and `defaultProfile` to `host`,
  `defaultHost` and `isDefault`, which now says whether the profile is the
  default.
- `profiles list`: name, host, account, `isDefault`, `isActive`, and `active`
  with the profile a command run here now uses and its source; null with the
  token of the environment, with `activeProblem` where the choice fails. It
  asks nothing of the network and prints no token.
- `profiles rename <old> <new>`: the default and the cards of `work start` go
  along. Cards left in the list under the new name belong to no profile and are
  dropped, never mixed in.
- `profiles remove <name> [--revoke]`: as `auth logout`, for the profile named.
  The cards it holds in the list of `work start` go with it.
- `profiles default <name>`.
- `auth logout [--revoke]`: removes the profile a command uses, chosen as for
  every command, so a host with several profiles and the default elsewhere is
  exit 8 and removes and revokes nothing. Its host is not read first, and a
  `--host` that cannot be read finds the one profile whose host is written
  letter for letter like it, so that one written by hand in a form nothing can
  read can be taken out again; see [Token, hosts and
  requests](security.md#host-names).
- `config set host <url>` makes the one profile on that host the default; no
  profile is exit 1, several are exit 8 naming `profiles default` (C-180).
  `config get host` is the host of the default profile.

## The default

Every session on the machine shares the default profile, so it moves only when
somebody means it to (C-179):

- `auth login` makes a profile the default only where there is none yet, or one
  no command can use; otherwise it stays, and the answer says so:
  `data.isDefault` false, `data.defaultProfile` the default, a summary and a
  text line naming it, and breadcrumbs to `--profile` and `profiles default`.
  `--make-default` moves it.
- `setup` makes the profile it signs in with the default: it asks a person at
  the terminal which host this machine works with. `data.previousDefaultProfile`
  names the one before where it moved.
- Removing the default profile leaves no default, with a `notice`, and no
  profile that happens to be left takes its place (C-181). A command without a
  profile then exits 3 with `no default profile is set` and names `--profile`
  and `profiles default`; the next sign-in sets the default again.

## The configuration file

The file keeps `profiles` and `defaultProfile`, beside `clientId` and
`comments`. Next to them it writes `hosts` and `defaultHost` in the form of the
versions before profiles, so that such a version still reads the file (C-194):

- every host carries the token and the account of the default profile where
  that one is on it, else of the first profile on it by name;
- `defaultHost` is the host of the default profile.

Where `profiles` is in the file, it is what holds. A token under `hosts` that no
profile on that host holds was put there by hand and is taken over into a
profile named after the host. A file without `profiles` is one of a version
before profiles, or one such a version wrote last: it drops what it does not
know. Every host with a token in it becomes a profile named after the host, a
host as a sign-in writes it where it can be read and as it stands otherwise,
and the default host becomes the default profile. So an older version's sign-in
or sign-out is never lost; what is lost are the names of the profiles and every
account of a host beyond the one written under it.

A plain read never writes the file. It is written in the new form when a
command writes it anyway: a sign-in, a sign-out, a rename, `config set`, the
first `clientId`.

## State per account

The cards of `work start` are kept per account (C-196): under the name of the
profile, or under the host where the token of the environment signs a command
in. The store keeps them in `accounts`, and beside them, as the configuration
file does, in `hosts` for the versions before profiles: per host the lists of
the profile that host carries in the file, or of the host itself where no
profile signs in to it.

Where the store has `accounts`, they are what holds, and a host is taken over
only where no account on it holds anything: the copy is of the profile the host
carried when it was written, which after `profiles default` or a new profile
need not be the one it carries now. A rename writes the configuration first,
so the copy goes under the new name. A store with `hosts` alone is one such a
version wrote last, and its lists go to the profile each host carries, or stay
under the host. An older version reporting from the store reports the lists of
that one profile per host; the cards of a second profile on a host it does not
see.

Removing a profile drops its lists, before the profile leaves the file: under a
name a later profile may take on another host, they would be reported there
with its token. Where a move or a drop could not reach the store, lists stay
under a name no profile has. The heartbeat passes over them, and nothing
deletes them for that: a rename writes the configuration before the store, and
a command in between would otherwise delete the lists it is moving. A sign-in
that makes a new profile clears the store under its name first, and is refused
where it cannot, so no new profile inherits them.

The heartbeat reports the list of its session in every account that holds one,
each with the token of its own profile. The mod runs it with the environment
Claude Code was started with, which need not be that of the command that took
a card up. `--profile` and `--host` narrow it to the one account. `data.profile`
names the profile where there is one account, and `data.accounts` has one entry
per account with `profile`, `sent`, `reason` and `cardIds`.

The marks of the comments read and the positions of `notifications wait` are
kept per host and per reader, a reader being the token, so two accounts on one
host never mix there, and a rename changes nothing about them.
