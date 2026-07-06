<p align="center">
  <img src="assets/mascot-ghost.svg" width="84" alt="banish mascot" />
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/wordmark-dark.svg" />
    <img src="assets/wordmark-light.svg" width="172" alt="banish.sh" />
  </picture>
</p>

<p align="center">
  Token-optimized middleware for LLM coding agents.<br/>
  <b>30-90% fewer tokens</b> on the commands your agent runs all day.
</p>

<p align="center">
  <a href="https://github.com/turanheydarli/bani.sh/stargazers"><img src="https://img.shields.io/github/stars/turanheydarli/bani.sh?style=flat&logo=github&color=2da44e&labelColor=1f2328" alt="stars" /></a>
  <a href="https://github.com/turanheydarli/bani.sh/graphs/contributors"><img src="https://img.shields.io/github/contributors/turanheydarli/bani.sh?style=flat&color=2da44e&labelColor=1f2328" alt="contributors" /></a>
  <a href="https://github.com/turanheydarli/bani.sh/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/turanheydarli/bani.sh/ci.yml?branch=main&style=flat&color=2da44e&labelColor=1f2328&label=build" alt="build" /></a>
  <a href="https://github.com/turanheydarli/bani.sh/releases"><img src="https://img.shields.io/github/v/release/turanheydarli/bani.sh?style=flat&color=57606a&labelColor=1f2328" alt="release" /></a>
  <img src="https://img.shields.io/github/license/turanheydarli/bani.sh?style=flat&color=57606a&labelColor=1f2328" alt="license" />
  <a href="https://github.com/turanheydarli/bani.sh/labels/good%20first%20issue"><img src="https://img.shields.io/github/issues/turanheydarli/bani.sh/good%20first%20issue?style=flat&color=bf8700&labelColor=1f2328&label=good%20first%20issue" alt="good first issue" /></a>
</p>

## The number

banish compacts shell command output before it reaches the model. Your agent runs
the same commands all day - `git status`, `npm install`, `cargo build`,
`kubectl get` - and most of that output is noise the model never needs. banish
runs the real command, strips the noise, and returns a compact version: `git status`
drops from 176 tokens to 12, a 93% cut. Run `banish gain` any time to see your own
running total.

## Install

banish is a single Go binary. Install it with:

```sh
go install go.banish.sh/banish/cmd/banish@latest
```

Then wire it into your agent with one command:

```sh
banish init claude-code
```

For Cursor, run `banish init cursor`. For any other MCP agent, run `banish init mcp`.

## Quickstart

banish sits between your agent's Bash tool and the shell. Here is the same
`git status`, before and after:

```text
git status            raw: 176 tokens
-----------------------------------
On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
	modified:   cmd/banish/gain.go
	modified:   internal/runtime/fallback.go
...

git status            via banish: 12 tokens (93% saved)
-----------------------------------
main
  modified:   cmd/banish/gain.go
  modified:   internal/runtime/fallback.go
```

banish only auto-approves commands your Claude Code permission rules already
allow. Anything that changes state - `git commit`, `git push`, `rm` - still
prompts you, exactly as it would without banish.

## Supported

Measured savings on common commands:

| Command | Raw | Compacted | Savings |
|---------|-----|-----------|---------|
| `git status` (clean) | ~40 tok | `ok main` (3 tok) | 92% |
| `git status` (dirty) | 176 tok | 67 tok | 61% |
| `git diff --stat` | 127 tok | 2 tok | 98% |
| `ls -la` (14 entries) | 245 tok | 29 tok | 87% |
| `npm install` | ~200 tok | 5 tok | ~97% |
| `cargo build` (success) | ~150 tok | `cargo build: ok` (4 tok) | 97% |
| `make` (20 files) | ~390 tok | `== 21 compile commands` (5 tok) | 98% |
| `jest` (12 suites pass) | ~168 tok | 37 tok | 77% |
| `dotnet build` | ~96 tok | 23 tok | 76% |

Ships with filters for git, docker, kubectl, npm/yarn/pnpm, cargo, maven, gradle,
pytest, terraform, aws, gh, make/cmake/ninja, jest/vitest/eslint/tsc, and dotnet.
Add your own with a `.bsh` file - no recompile.

## Contributing

New here? The [good first issues](https://github.com/turanheydarli/bani.sh/labels/good%20first%20issue)
are scoped for a first PR - a new filter is about ten lines. Pick one and we'll
help you land it. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).

<p align="center"><sub>the eyes were the ai the whole time.</sub></p>
