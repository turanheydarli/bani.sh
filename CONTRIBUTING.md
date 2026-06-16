# Contributing to banish

Welcome - we're glad you're here, and no contribution is too small. A typo fix,
a sharper filter, a better error message: all of it counts.

## Good first issues

Start with the [good first issues](https://github.com/turanheydarli/bani.sh/labels/good%20first%20issue).
They're scoped so you can land a real change without reading the whole codebase
first. Pick one, say hi on the issue, and a maintainer will review your first PR
hands-on and help you get it merged.

## Set up

banish is a single Go binary with no external runtime dependencies.

```sh
git clone https://github.com/turanheydarli/bani.sh
cd bani.sh
go install ./cmd/banish
```

Run the quality gate before you push. The repo ships these as `BANISH` verbs:

```sh
banish test     # go test with the race detector
banish check    # full gate: test + vet + staticcheck
```

If you'd rather call Go directly: `go test -race -count=1 ./...` and `go vet ./...`.

## Make your change

The most common first change is a new output filter - about ten lines of `.bsh`.
A filter matches a command and pipes its raw output through a shell one-liner that
strips the noise:

```bsh
!filter docker-build
!match docker build
!compact "grep -v '^Sending build' | grep -v '^---> ' | tail -20"
```

`!match` is a substring match against the command; `!compact` receives raw stdout
on stdin and writes the compact version to stdout. The built-in filters are plain
`.bsh` files in [internal/extension/builtin/](internal/extension/builtin/), one
per ecosystem, embedded into the binary at build time. Add your filter to the
matching file (or create a new one), then `go install` and try it with
`banish "docker build ."`. Keep the one-liner simple: reach for `grep`, `sed`,
`head`, `tail`, `cut`, and `awk` before anything heavier. If a filter fails,
banish returns the raw output, so a partial contribution never breaks anyone.

## Open the PR

Branch off `main` with a short, descriptive name (`filter-docker-build`,
`fix-git-status-empty`). A good PR description says what it changes and shows a
quick before/after token count if you have one. Don't worry about a perfect diff - maintainers
will help shape it. The goal is the merge, not a flawless first try.

## Recognition

Every merged PR is credited by name in the release notes. Your change ships, and
your name ships with it.

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/),
applied evenly to everyone. Be kind, assume good faith, and help the next person
land their first PR the way someone helped you.
