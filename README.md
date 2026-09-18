# minka

Command line client for the API of [Minka](https://minka.localbeach.net).
Proprietary and in-house, like Minka itself.

## Install

```bash
make install   # go install ./cmd/minka
make build     # bin/minka
```

## Sign in

Create an access token on the server and pipe it into the CLI, so it never
appears in the shell history:

```bash
beach exec ./flow token:create --email-address you@example.com --name laptop \
  | grep -o 'minka_pat_[A-Za-z0-9_-]*' \
  | minka auth login --host https://minka.localbeach.net --with-token
```

The token is stored in `~/.config/minka/config.json` with permissions 0600.
`MINKA_HOST` and `MINKA_TOKEN` override the config file.

## For agents

The description of minka for agents travels inside the binary:

```bash
minka skill install   # ~/.claude/skills/minka/SKILL.md, read by Claude Code
minka skill status    # current, differs or not installed
```

After an upgrade minka says when the installed copy differs from the one it
carries; it never replaces it by itself.

## Use

```bash
minka projects list
minka boards list <projectId>
minka boards show <boardId>
minka cards create --board <boardId> --title "Fix the login" --description - < notes.md
minka cards move <cardId> --column Doing --bottom
minka cards done <cardId>
minka cards delete <cardId>
minka cards assign <cardId> me
minka people list <projectId>
minka comments create <cardId> "Done in abc1234"
minka messages list <chatId>
minka commands        # every command with its flags
```

Output is text on a terminal and JSON otherwise; see `minka --help` and
[skills/minka/SKILL.md](skills/minka/SKILL.md) for the JSON envelope, exit
codes and how descriptions are converted.

## Develop

```bash
make check   # gofmt, go vet, tests against an in-memory fake of the API
```

Working notes for Claude Code are in [CLAUDE.md](CLAUDE.md); the documentation
is indexed in [docs/README.md](docs/README.md), starting with
the [current state](docs/work-in-progress/status.md).
