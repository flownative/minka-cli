# minka

The command line for [Minka](https://www.flownative.com): projects, boards,
cards, chats and private conversations, from a terminal or from an AI agent.

- **Made for agents as much as for people.** Text on a terminal, JSON
  everywhere else, one exit code per class of error, and a description of
  itself that agents can read.
- **Careful with your token.** It never travels on a command line, lives in a
  file only you can read, and is only ever sent to the host it belongs to.
- **Markdown in, Markdown out.** Descriptions, comments and messages are
  written and shown as Markdown, without losing files or mentions on the way.

Proprietary and in-house, like Minka itself, until Minka is published.

## Install

```bash
brew install flownative/flownative/minka
```

The repository is private for now, so Homebrew builds minka from source and
clones it over SSH: you need read access to `flownative/minka-cli` and an SSH
key GitHub knows. `brew upgrade` brings new versions.

<details>
<summary>Without Homebrew</summary>

Download the archive for your machine from a release, check it, and put the
binary on your `PATH`. With the [GitHub CLI](https://cli.github.com):

```bash
gh release download v0.1.0 --repo flownative/minka-cli --pattern '*darwin_arm64*' --pattern checksums.txt
shasum -a 256 --check --ignore-missing checksums.txt
tar -xzf minka_0.1.0_darwin_arm64.tar.gz
mv minka_0.1.0_darwin_arm64/minka ~/bin/
```

Archives exist for `darwin_arm64`, `darwin_amd64` and `linux_amd64`. There is
no install script to pipe into a shell, on purpose.

From a checkout: `make install`.

</details>

## First steps

```bash
minka setup
```

asks for the host of your Minka, takes an access token without showing it,
checks it against the host, saves it, offers to install the skill for agents,
and installs the mod that has Claude Code report the cards a session is working
on (see below; `--no-mod` leaves it out). Create the token on your account page
in Minka, for an agent of yours; it is shown once.

Then:

```bash
minka projects list
minka cards mine
```

## Use

```bash
minka projects list
minka boards list <projectId>
minka boards show <boardId>
minka cards mine                       # what you are assigned to, across projects
minka cards show <cardId>
minka cards create --board <boardId> --title "Fix the login" --description - < notes.md
minka cards move <cardId> --column Doing
minka cards assign <cardId> me
minka cards done <cardId>
minka cards notes show <cardId>        # the working notes on the back of a card
minka cards notes set <cardId> - --expect <updatedAt> < notes.md
minka steps create <cardId> "Write the test"
minka comments create <cardId> "Done in abc1234"
minka chats list <projectId>
minka messages list <chatId>
minka messages create <chatId> - < message.md
minka conversations with <person> --project <projectId>
minka uploads create shot.png --project <projectId>
minka import basecamp --project <basecampId> --out <dir>   # a bundle for Minka's importer
minka commands                         # every command with its flags
```

IDs are UUIDs and come from earlier output; a card can also be named by its
short identifier, such as `MNK-1A`. People are given by ID, `me`,
e-mail address or name; columns by ID, type or title. Long or multi-line text
comes through stdin with `-`, so no shell quoting gets in its way.
`minka <command> --help` has the details.

### Output

| You want | Flag | You get |
|---|---|---|
| to read | none, on a terminal; `--styled` anywhere | text, rich text as Markdown |
| structure with hints | `--json`; the default without a terminal | `{ok, data, summary, notice, breadcrumbs}` |
| only the resource | `--quiet`, `-q` | `data`: the API resource, unchanged |

```bash
minka boards show <boardId> --quiet | jq '.columns[] | {title, cards: [.cards[].title]}'
```

A failure prints `{ok: false, error, code, retryable, hint, meta}` and exits
with a code for its class:

| Exit | `code` | Meaning |
|---|---|---|
| 1 | `usage`, `config` | the command line, or the config file |
| 2 | `not_found` | does not exist, or is not visible to you |
| 3 | `auth_required` | not signed in, or the token was refused |
| 4 | `forbidden` | the account may not use the API |
| 6 | `network` | no complete answer from the host |
| 7 | `api_error` | the server failed, or answered unexpectedly |
| 8 | `ambiguous` | a name matches more than one thing |
| 9 | `validation` | Minka refused the input; `meta.apiCode` says why |
| 11 | `unread` | a comment was not written: its card has comments you have not read |
| 12 | `timeout` | `notifications wait --timeout` ran out with nothing new |

`retryable` is true only where a second attempt cannot do harm. After a failed
write, read the state back before trying again.

## Signing in

`minka setup` is the way for a person. The single steps work without a
terminal:

```bash
minka auth login --host https://minka.example.com --with-token < token.txt
minka auth status
minka auth logout --host https://minka.example.com
```

- No flag takes the token itself, so it never ends up in the shell history or
  the process list.
- A token is checked against its host before it is saved.
- Several hosts can be signed in to at once; the one signed in to last is the
  default, and `--host` picks another.
- `MINKA_HOST` and `MINKA_TOKEN` sign in without the config file, for CI and
  containers. The token in `MINKA_TOKEN` is only ever sent to the host in
  `MINKA_HOST`.

## For AI agents

minka carries a description of itself for agents, the skill, inside the
binary, so it always fits the installed version:

```bash
minka skill install    # ~/.claude/skills/minka/SKILL.md, read by Claude Code
minka skill install --dir ~/.agents/skills
minka skill status     # current, differs or not installed
minka skill show       # print it
```

After an upgrade, every command says in its `notice` when the installed copy
differs from the one the binary carries. minka never replaces it by itself.

Agents also find their way with `minka commands --json`, with `--help` on any
command, and with the `breadcrumbs` of an answer, which name the commands that
commonly follow.

An agent says which cards it is working on with `minka work start <card>`, and
the board shows them as alive while `minka heartbeat` reports them. In Claude
Code the mod does that from inside the process of the session, for every
project: at the start of every turn, after every tool call and every thirty
seconds, so a long tool call keeps its tile alight and a session that ended
goes dark three minutes later.

```bash
minka hooks install    # write the mod and install it with 'claude plugin'
minka hooks status     # whether it is current and loaded
```

`minka setup` runs the install as well. It needs `claude` on the PATH, Claude
Code 2.1.287 or later (the first version that loads mods), and takes effect in
sessions started afterwards. With an older Claude Code the install says to
update it and changes nothing in its settings. Where an earlier minka wrote a
`PostToolUse` hook into `~/.claude/settings.json`, the install takes that entry
out and leaves the rest of the file as it was.

## Configuration

| | |
|---|---|
| Config file | `$MINKA_CONFIG`, else `$XDG_CONFIG_HOME/minka/config.json`, else `~/.config/minka/config.json` |
| `MINKA_HOST` | the host, unless `--host` is given |
| `MINKA_TOKEN` | the token for the host in `MINKA_HOST` |

The config file holds the hosts and their tokens and is written with
permissions 0600. A file that others can read is refused rather than used.

## Troubleshooting

```bash
minka doctor
```

checks the config file and its permissions, the sign-in, whether the host
answers and accepts the token, and the installed skill, and says what to do
about what it finds. It is a report and always exits 0; with `--json`,
`data.healthy` tells whether a check failed.

A 404 means the thing does not exist *or* belongs to a project you are not a
member of: Minka answers both the same way.

## Upgrading

```bash
brew upgrade minka
minka skill install
```

## Development

```bash
make build    # bin/minka
make check    # gofmt, go vet, tests against an in-memory fake of the API
make dist     # the archives of a release, in dist/
```

Work reaches `main` through pull requests; GitHub Actions run the checks. A
version is released by publishing a release in GitHub, see
[Releasing a version](docs/guides/release.md).

Working notes for Claude Code are in [CLAUDE.md](CLAUDE.md). The documentation
is indexed in [docs/README.md](docs/README.md), starting with the
[status](docs/work-in-progress/status.md).

## License

Proprietary, like Minka itself.
