# banish

**Token-optimized middleware for LLM coding agents.** Compacts shell command output before it reaches the model -- 30-90% savings on commands agents run constantly (`git status`, `npm install`, `cargo build`, `kubectl get`, etc.).

- Single Go binary, no runtime dependencies except `jq` for the hook
- Works with Claude Code, Cursor, and any MCP-compatible agent
- Extensible via `.bsh` scripts -- no recompilation needed
- Ships with filters for git, docker, kubectl, npm/yarn/pnpm, cargo, mvn, gradle, pytest, terraform, aws

---

## Why

A typical agent session burns thousands of tokens on command output the model does not need -- boilerplate hints, permission tables, download progress, verbose logs. `banish` sits between the agent's Bash tool and your shell, runs the real command, and returns a compacted version.

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

---

## Install

```bash
go install go.bani.sh/banish/cmd/banish@latest
```

Or build from source:

```bash
git clone https://go.bani.sh/banish
cd banish
go install ./cmd/banish
```

Then set it up for your agent (one command, global):

```bash
banish init claude-code
```

This writes to your home directory:
- `~/.banish/ext/*.bsh` -- default extensions (git, docker, kube, node, python, rust, java, cloud)
- `~/.claude/hooks/banish-hook.sh` -- PreToolUse hook that routes every Bash tool call through banish
- `~/.claude/settings.json` -- hook registered (merges with existing config)
- `~/.claude/CLAUDE.md` -- one-paragraph context appended for the agent

For Cursor: `banish init cursor`. For other MCP agents: `banish init mcp`.

---

## How it works

banish has three parts:

**1. A proxy for bash commands.** When Claude Code runs `git status`, the PreToolUse hook rewrites it to `banish "git status"`. banish executes the real command, pipes the output through a matching filter, and returns compact text. Failed filters fall back to raw output (never crashes).

**2. An MCP server.** `banish serve` exposes tools over stdio. Every verb defined in `~/.banish/ext/` is auto-exposed as an MCP tool. With the defaults, agents get `banish_gs`, `banish_dps`, `banish_kpods`, `banish_cb`, etc. -- 45+ tools out of the box.

**3. A scripting runtime.** The `.bsh` language (embedded parser) lets you define verbs, filters, and pipelines in a single file. Run inline: `banish "ls /var/log ext:log | count"`. Run scripts: `banish run build.bsh`. Check a manifest: `banish check BANISH`.

---

## Extending

Everything is extensible through `.bsh` files in `~/.banish/ext/`. No Rust, no Go, no recompile -- just a shell script wrapped in a simple directive format.

### Add a verb shortcut

```bsh
# ~/.banish/ext/mytools.bsh
!extension mytools v:1.0

!verb st
!expand exec git status --short
!help "Git status short"

!verb kpods
!args ns
!expand exec kubectl get pods -n {ns} -o wide
!help "List pods in namespace"
```

After saving, restart the MCP server. `banish_st` and `banish_kpods` are now MCP tools.

### Add an output filter

```bsh
!filter docker-build
!match docker build
!compact "grep -v '^Sending build' | grep -v '^---> ' | tail -20"
```

`!match` is a substring match against the command. `!compact` is a bash one-liner that receives raw stdout on stdin and writes filtered output to stdout.

### Project-specific verbs (BANISH file)

Create a `BANISH` file in your repo root for project-scoped commands:

```bsh
# BANISH -- my-project

!verb build
!expand exec go build -ldflags "-X main.version=dev" -o bin/myapp ./cmd/myapp

!verb test
!expand exec go test -race -count=1 ./...

!verb check
!expand exec go vet ./... && staticcheck ./...

!config
!timeout "60s"
```

Then `banish build`, `banish test`, `banish check` -- and they appear as MCP tools automatically.

---

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

---

## Architecture

```
cmd/banish/           CLI entry points (main, run, serve, init, gain, etc.)
internal/
  lexer/              Tokenizer for .bsh
  parser/             Recursive descent parser -> AST
  ast/                AST node types
  interpreter/        Tree-walking interpreter, verb registry
  runtime/            Builtin verbs (ls, read, fetch, etc.), fallback handler
  extension/          .bsh extension loader (verbs + filters)
  manifest/           BANISH project manifest parser
  compact/            Script filter runner, ANSI stripping
  mcp/                JSON-RPC 2.0 MCP server and client
  analyzer/           Frequency tracking, token accounting, SQLite-free persistence
  scaffold/           banish init -- embedded extensions and hook script
  hints/              Suggests shorter banish equivalents for common bash commands
  config/             Project config loading
foundation/errs/      Error utilities
```

Design notes:
- **Single binary, no external state** -- everything persists to `~/.banish/` (JSON files)
- **No async, no goroutines in the hot path** -- startup under 10ms
- **Fallback-first** -- if a filter fails, raw output is returned. banish never swallows data
- **Scripting IS the extension mechanism** -- we removed a 900-line TOML pipeline system in favor of shell one-liners

---

## Contributing

```bash
git clone https://go.bani.sh/banish
cd banish

# Run the full check before committing
go fmt ./...
go vet ./...
go test -race -count=1 ./...

# Build and install locally
go install -ldflags "-X main.version=dev" ./cmd/banish
```

The BANISH manifest in the repo root defines these as verbs:

```bash
banish build       # go build
banish test        # go test with race detector
banish check       # full quality gate (test + vet + staticcheck)
banish lint        # vet + staticcheck
banish vuln        # govulncheck
```

### Adding a new filter

Filters live in `internal/scaffold/extensions.go` (embedded in the binary and deployed by `banish init`). Add your filter as a `.bsh` entry, then `go install`.

Keep `!compact` one-liners simple: prefer `grep -v`, `sed`, `head`, `tail`, `cut`, `awk` over complex scripts. If a filter needs 20 lines of awk, consider whether the underlying tool has a `--quiet` flag that does the job.

### Testing a filter

```bash
# Drop a test .bsh file in ~/.banish/ext/
vim ~/.banish/ext/mytool.bsh

# Run the command -- banish applies the filter
banish "mytool do-thing"

# See how much it saved
banish gain
```

### Code style

- `anyhow`-style error wrapping with `fmt.Errorf("context: %w", err)`
- No `interface{}` / `any` unless the MCP/JSON boundary forces it
- Filter scripts are untrusted from banish's perspective -- if one fails, return raw
- Never skip hooks (`--no-verify`) unless explicitly asked

### Security

banish runs arbitrary shell commands from `.bsh` files in `~/.banish/ext/`. Extensions from untrusted sources can execute anything your shell can. Treat `.bsh` files the same as shell scripts -- review before installing, keep `~/.banish/ext/` as 0700 if you share the machine.

If you find a security issue, please email (not a public issue): security contact listed in SECURITY.md.

---

## License

MIT.
