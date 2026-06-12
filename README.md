# banish

[![Go Reference](https://pkg.go.dev/badge/go.bani.sh/banish.svg)](https://pkg.go.dev/go.bani.sh/banish)
[![Go Report Card](https://goreportcard.com/badge/go.bani.sh/banish)](https://goreportcard.com/report/go.bani.sh/banish)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Token-optimized middleware for LLM coding agents. banish compacts shell command output before it reaches the model, cutting 30-90% of the tokens agents burn on commands they run constantly, such as `git status`, `npm install`, `cargo build`, and `kubectl get`.

- Single Go binary, no runtime dependencies except `jq` for the hook
- Works with Claude Code, Cursor, and any MCP-compatible agent
- Extensible via `.bsh` scripts, no recompilation needed
- Ships with filters for git, docker, kubectl, npm/yarn/pnpm, cargo, maven, gradle, pytest, terraform, and aws

Website: https://bani.sh

## Example

A typical agent session burns thousands of tokens on output the model does not need: boilerplate hints, permission tables, download progress, and verbose logs. banish sits between the agent's Bash tool and your shell, runs the real command, and returns a compacted version.

```
git status  (raw: 176 tokens)
-----------------------------------
On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
	modified:   cmd/banish/gain.go
	modified:   internal/runtime/fallback.go
...

git status  (compacted: 12 tokens, 93% savings)
-----------------------------------
main
  modified:   cmd/banish/gain.go
  modified:   internal/runtime/fallback.go
```

Measured savings on common commands:

| Command | Raw | Compacted | Savings |
|---------|-----|-----------|---------|
| `git status` (clean) | ~40 tok | `ok main` (3 tok) | 92% |
| `git status` (dirty) | 176 tok | 67 tok | 61% |
| `git diff --stat` | 127 tok | 2 tok | 98% |
| `ls -la` (14 entries) | 245 tok | 29 tok | 87% |
| `npm install` | ~200 tok | 5 tok | ~97% |
| `cargo build` (success) | ~150 tok | `cargo build: ok` (4 tok) | 97% |

Run `banish gain` at any time to see your own cumulative savings.

## Install

```bash
go install go.bani.sh/banish/cmd/banish@latest
```

Then wire it into your agent with one command:

```bash
banish init claude-code
```

For Cursor, run `banish init cursor`. For any other MCP agent, run `banish init mcp`.

`banish init` writes to your home directory:

- `~/.banish/ext/*.bsh` -- default extensions (git, docker, kube, node, python, rust, java, cloud)
- `~/.claude/hooks/banish-hook.sh` -- PreToolUse hook that routes every Bash tool call through banish
- `~/.claude/settings.json` -- hook registered (merged with existing config)
- `~/.claude/CLAUDE.md` -- one-paragraph context appended for the agent

## How it works

banish has three parts:

1. **A proxy for bash.** A PreToolUse hook rewrites `git status` to `banish "git status"`. banish runs the real command, pipes the output through a matching filter, and returns compact text. If a filter fails, raw output is returned, so banish never crashes or swallows data.

2. **An MCP server.** `banish serve` exposes tools over stdio. Every verb in `~/.banish/ext/` is auto-exposed as an MCP tool. With the defaults, agents get 45+ tools out of the box (`banish_gs`, `banish_dps`, `banish_kpods`, and more).

3. **A scripting runtime.** The `.bsh` language lets you define verbs, filters, and pipelines in one file. Run inline (`banish "ls /var/log ext:log | count"`), run a script (`banish run build.bsh`), or check a manifest (`banish check BANISH`).

## Extending

Add verbs and filters with `.bsh` files in `~/.banish/ext/`. No Rust, no Go, no recompile.

```bsh
# ~/.banish/ext/mytools.bsh
!extension mytools v:1.0

!verb kpods
!args ns
!expand exec kubectl get pods -n {ns} -o wide
!help "List pods in namespace"

!filter docker-build
!match docker build
!compact "grep -v '^Sending build' | grep -v '^---> ' | tail -20"
```

After saving, restart the MCP server and `banish_kpods` is available as a tool. For project-scoped verbs, add a `BANISH` file to your repo root. Full guide: https://bani.sh/docs.

## CLI

```
banish "command"            Execute inline (bash or .bsh syntax)
banish run script.bsh       Run a .bsh file
banish check file.bsh       Parse a .bsh file, report errors without executing
banish serve                MCP server mode (stdio)
banish schema               Dump verb catalog as JSON (for agent system prompts)
banish gain                 Show cumulative token savings
banish gain --reset         Clear tracking data
banish init claude-code     Install hooks, extensions, CLAUDE.md (global)
banish version              Print version info
```

## Contributing

```bash
git clone https://go.bani.sh/banish
cd banish

# Run the full quality gate before committing
go test -race -count=1 ./...
go vet ./...

# Build and install locally
go install -ldflags "-X main.version=dev" ./cmd/banish
```

The `BANISH` manifest in the repo root defines `banish build`, `banish test`, `banish check`, `banish lint`, and `banish vuln` as shortcuts. New filters live in `internal/scaffold/extensions.go` (embedded in the binary and deployed by `banish init`); keep `!compact` one-liners simple and prefer `grep`, `sed`, `head`, `tail`, `cut`, and `awk` over complex scripts.

## Security

banish runs arbitrary shell commands from `.bsh` files in `~/.banish/ext/`. Treat them like any shell script: review before installing, and keep `~/.banish/ext/` at `0700` on shared machines. To report a vulnerability, follow the process in [SECURITY.md](SECURITY.md) (please do not open a public issue).

## License

MIT. See [LICENSE](LICENSE).
