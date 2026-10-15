# First steps

After installing, a person runs `minka setup`; when something does not work,
anybody runs `minka doctor`.

## setup

`setup` asks questions, so it runs on a terminal only. Without one it exits 1
before anything is asked, sent or written, and names the single steps an agent
or a script takes instead: `auth login --with-token`, `skill install` and
`hooks install` (C-018).

In this order:

1. The config file: one that cannot be read, or that others can read, ends
   setup before the first question. A token is shown once, and one that cannot
   be saved is lost.
2. The host: `--host` or `$MINKA_HOST` when given, else it asks. An empty
   answer takes the host signed in to last; a host that cannot be used is
   asked for again.
3. The sign-in: a saved token of this host that it still accepts is kept, and
   the host becomes the default. Only a host that refuses the saved token is a
   reason to ask for a new one; a host that does not answer ends setup with
   that error. Asking means pointing to the account page of the host, where a
   token for an agent is created, and taking the token without echo. The token
   goes the same way as with `auth login`: checked against `/me` first, saved
   only when accepted. A refused token ends setup with exit 3; nothing is saved
   and no skill is installed.
4. The skill: where installing would change something and is possible, it
   asks. Only `y`, `yes` or an empty answer installs.
5. The mod: installed without a question, as `hooks install` installs it, old
   hook and all; `--no-mod` leaves it out. Where `claude` is missing or does not
   take it, setup says so and ends well all the same (C-108); see [Saying what
   you are working on](at-work.md).

Questions and progress go to stderr; stdout carries what every command prints,
here the host, the person, the state of the skill, the mod and the old hook,
with the first commands as breadcrumbs. The token is never printed.

## doctor

`doctor` is a report. It exits 0 whatever it finds, because the failing thing
is the setup, not the command; `data.healthy` is false when a check failed
(C-019).

| Check | Fails or warns when |
|---|---|
| `version` | never; says which minka this is |
| `config` | the config file cannot be read, or others can read it |
| `sign-in` | no host or token can be found: nobody is signed in, `$MINKA_TOKEN` stands without `$MINKA_HOST`, `--host` is no host. The hint names the step that needs no terminal first, then `setup`. Skipped when the config check failed |
| `host` | the host does not answer or refuses the token; carries the hint of that error |
| `skill` | warns when none is installed or the installed one differs; a skill behind a symbolic link is fine |
| `update` | warns when this minka is older than the minimum Minka named; a newer latest version is `ok` with the hint `brew upgrade minka`. Says that nothing is known yet, or that a build is no release and not compared (C-129) |

A warning leaves the report healthy. A check that was not looked at because
one before it failed has the status `skip`, so one cause shows up once. `doctor` sends one reading request, and
none without a sign-in, whose answer already counts for `update`. It reports
the state of the skill and of the version itself, so it carries no notice and
no `cliUpdate` about them.
