# CLAUDE.md

Instructions for Claude Code for working on this repository.

## Conversation

The client is Robert; Shaun stands in for him with the same rights.
Conversations with both are in German, with the informal "du". Everything
written down is English: code, comments, commit messages, help texts, the
output of the CLI, the documentation and the skills of this project.

## What this is

`minka` is the command line for the JSON API of Minka (`/api/v1`). First of all
for dogfooding: Robert and Claude sessions use it to manage the development
tasks of Minka. It is explicitly built for AI agents as well: stable `--json` output,
an exit code per class of error, a description of the commands in
`skills/minka/SKILL.md`.

The model for its concepts is [basecamp-cli](https://github.com/basecamp/basecamp-cli):
adopt the principles, not the scope. Not part of it: AI integrations,
notifications, an interactive TUI, anything the API does not offer.

Proprietary like Minka, until Minka is published. The repository lives on
GitHub at `git@github.com:flownative/minka-cli.git`. Work reaches `main`
through a pull request from a branch of its own; GitHub Actions run the checks
on it. Pushing, and anything that rewrites pushed history, only when the client
says so. How a version gets released is laid down in the
[release concept](docs/work-in-progress/release-concept.md).

## Documentation

- **Index of all documents:** [docs/README.md](docs/README.md)
  — one line per document, saying when you need it. Start with little, load
  more when needed.
- **First look of every session:** [Status](docs/work-in-progress/status.md).
- **Decision register:** [docs/decisions.md](docs/decisions.md)
  (C-001…) — per decision, briefly: what holds, why, where it is implemented.
- Structure and lifecycle of the documentation: `.claude/rules/documentation.md`
  (loads by itself when working under `docs/`).

Assume nothing about existing code, look it up. The documentation update
belongs in the same commit as the code change.

## Stack and layout

Go (see `go.mod`), [cobra](https://github.com/spf13/cobra) for commands,
[goldmark](https://github.com/yuin/goldmark) for Markdown,
`golang.org/x/net/html` for HTML, `golang.org/x/term` for the hidden token
prompt.

```
cmd/minka/            main: calls cli.Run and exits with its exit code
internal/api/         HTTP client and the types of the resources
internal/config/      hosts and tokens in one file with 0600
internal/markdown/    Markdown → HTML, only elements the server keeps
internal/richtext/    HTML of a text → Markdown for reading
internal/cli/         commands, output, exit codes
internal/fakeapi/     in-memory Minka API for tests (httptest)
skills/               SKILL.md for agents, embedded into the binary
```

```bash
make build        # bin/minka
make test         # go test ./...
make check        # gofmt check, go vet, tests; before every report and commit
```

## The API contract

The source of truth lives in the Minka repository, **read only**:

- `Documentation/stable/02-architecture/authorization.md`, section "API"
- `Documentation/decisions.md` (E-…)
- Code under `DistributionPackages/Flownative.Minka/Classes/Presentation/Api/`

`internal/fakeapi` mirrors this contract. When the API changes, adapt the fake
first, then the CLI.

The API is "internal, unstable". Questions, contradictions and wishes go as a
message to the Minka coordinator session (currently `minka-92`), not as a
change into the Minka repository. Check every message about an API change
against the code: controllers, resources and tests decide, not the message.
Where the two differ, that goes back to the coordination.

## Rules towards the Minka repository

`/Users/robert/Sites/minka` is there **for reading only**. Change no files, run
no Git commands that change anything, no `composer`, `phpunit`,
`npm run test:browser`, no migrations: there is exactly one test database, and
other agents are testing in it at the same time.

Allowed: reading, `beach exec ./flow token:create|token:list|token:revoke`,
live calls. Where writing live is allowed is laid down in the
[live check](docs/guides/live-check.md).

Tokens never appear in the repository, in commits, tests or logs, only in the
configuration file of the CLI (`~/.config/minka/config.json`, 0600).

## Conventions

The rules in force are under `docs/stable/`; their reasons are in the
register. For the work itself:

- Comments follow the skill `code-comments` (intention instead of mechanics,
  timeless, no references to cards, documentation or decision IDs).
- Commit messages follow the skill `git-workflow`, English, no session links.
- Every safeguard gets a test and a sabotage proof
  ([Testing](docs/stable/testing.md)).

## Working with subagents

As in Minka: an implementer builds, a reviewer who changes nothing checks.
Nobody checks their own work. Research that produces a lot of text goes to a
subagent with a terse report format.

## Central best practices (Flownative skills)

Skills from github.com/flownative/flownative-best-practices, symlinked at
`.claude/skills`. Update: `git -C .claude/skills pull origin main`.

| Task | Skill |
|------|-------|
| Code comments | `/code-comments` |
| Refactoring, removing code, code review | `/code-quality`, `/review` |
| Commit message, tidying a branch | `/git-workflow` |
| Recording a new best practice | `/learn-best-practice` |

The skills are mostly written for PHP, Flow and Neos, and in German. What
carries over are the rules on comments, naming, commits and reviews; where they
contradict this repository, this repository wins.
