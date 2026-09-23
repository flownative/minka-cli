# Testing

`make check` is the threshold: gofmt check, `go vet`, all tests. It runs before
every report and every commit.

On a pull request and on `main`, GitHub Actions run the same threshold, check
that `go.mod` is tidy, build the binary and scan for known vulnerabilities
with `govulncheck`. Actions are pinned to a commit, and a workflow holds no
more permissions than its job needs.

## Against the fake API, not against an instance

`internal/fakeapi` mirrors the contract of `/api/v1`: the same paths, status
codes and error codes, checked in the same order, with the byte-identical 401
and 404 bodies. It records the requests, so a test sees what really went out
(C-005).

When the API changes, the fake is adapted first, then the CLI. What the fake
does not mirror belongs as a sentence in the status, so that nobody assumes a
certainty that does not exist.

## Sabotage proofs

Every safeguard gets a test and a proof that the test bites: take the safeguard
out in a copy of the repository, check that the code compiles, and see the test
go red (C-006).

A proof that does not compile is none — then the compiler says no, not the
test. If a test stays green, an assertion is missing: sharpen the test first,
then repeat the proof.

The copy lives in the scratchpad, never in the repository.

## What a test may demand of time

A test may set an upper bound, one so wide that only a break trips it: the
seconds within which a download that was given up has to be back say that the
Basecamp CLI was ended and not waited out. What a test may not do is make the
pace of another program its precondition — that it gets going, or brings its
next byte, within a span the test set. That is a bet on the load of the
machine, and it is lost sooner or later (C-035).

What a test has to know about a program it runs, it therefore takes from the
code that runs it, not from the program: one that is ended before it has run a
line of its own leaves no note behind.

Where the pace is the behaviour itself, the test is the named exception and
stands with its reason in the [Status](../work-in-progress/status.md).

## Checking live

A live run shows what no fake shows: the real answers of the server. Rules
apply to it, see [Live check](../guides/live-check.md).

## What a test pins down

- The contract: envelope, exit code, `meta.apiCode`, the request that was sent.
- The rule, not the wording — unless the hint is itself the help somebody needs
  (the author hint on a 404, the advice on `--content-type`).
- The cases an API message describes: 201 versus 200, run twice, by somebody
  else, at the wrong address.
- A rule that holds for every command of a kind is tested against a list the
  command tree is held against, so that a new command cannot go untested:
  every command is known to write into a project or not.
