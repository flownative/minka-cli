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
minka commands        # every command with its flags
```

Output is text on a terminal and JSON otherwise; see `minka --help` and
[skills/minka/SKILL.md](skills/minka/SKILL.md) for the JSON envelope, exit
codes and how descriptions are converted.

## Develop

```bash
make check   # gofmt, go vet, tests against an in-memory fake of the API
```

Working notes for Claude Code are in [CLAUDE.md](CLAUDE.md), the current state
in [STATUS.md](STATUS.md).
