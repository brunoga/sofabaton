# Contributing to sofabaton

Thanks for your interest in contributing.

## Prerequisites

- Go (version specified in `go.mod`)
- [`golangci-lint`](https://golangci-lint.run/welcome/install/) for linting
- For regenerating oracle vectors only: Python 3.11+ and a checkout of
  [the upstream project](https://github.com/m3tac0de/home-assistant-sofabaton-x1s)
  (see `tools/oracle/`)

## Building

```sh
go build ./cmd/sbx
```

## Checking a change

```sh
make test    # the race tests, while working
make check   # before pushing: tidy, vet, lint and the tests
make ci      # every CI job, as CI runs them
```

`make check` is quick; CI runs the rest (fuzzing, cross-builds) on every
pull request. `make ci` runs every CI job the same way CI does: `go mod
tidy -diff`, golangci-lint, the cross-compiles for every release platform
(each with `go vet`), the race tests and a short run of every fuzz target.
Each job is also its own target (`ci-tidy`, `ci-lint`, `ci-build`,
`ci-test`, `ci-fuzz`). Run it before a release, or after changing the
build.

## Running tests

```sh
go test ./...

# With race detector (recommended before submitting a PR):
go test -race ./...

# Every fuzz target, briefly:
make fuzz
```

### Live-hub tests

Tests behind the `live` build tag talk to a real hub:

```sh
SOFABATON_HUB=192.168.1.50 make test-live
```

They only create, change and delete entities whose names start with
`ZZ-test-`, and leave the hub as they found it. Still: back up the hub first,
close the official app on every phone (the hub serves one client at a time),
and never point them at a hub you cannot restore.

## Protocol changes

Wire code is verified against bytes, not against our reading of them:

- Every decoder round-trips the captured frames in `testdata/captures/`.
- Builders match the upstream implementation's output byte for byte
  (`testdata/oracle/`, regenerated with `tools/oracle/`).
- A new fact about the protocol goes into `docs/` with its confidence tag
  (see `docs/README.md`) and, when it comes from our own hub, the capture
  that shows it.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | When to use |
|--------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `perf` | Performance improvement with no behaviour change |
| `docs` | Documentation only |
| `ci` | CI / tooling changes |
| `chore` | Maintenance (deps, build, etc.) |
| `test` | Tests only |
| `refactor` | Refactor without behaviour change |

## Pull requests

- Keep PRs focused — one logical change per PR.
- All CI checks must pass before merge.
- Include tests for any new behaviour.

## Code style

- Standard `gofmt` formatting.
- No unnecessary comments — the code should speak for itself.
- Error strings are lowercase and do not end with punctuation.
- Context is threaded through all blocking calls.
- Bytes from the hub are untrusted: decoders return errors, never panic.
- A failing input a fuzzer finds is committed under the package's
  `testdata/fuzz/` with the fix, so it stays a regression test.
