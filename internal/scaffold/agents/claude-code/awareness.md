## Banish

banish is installed. Bash commands run through banish, which compacts
noisy command output (git, grep, ls, find, go test, npm, cargo, make,
docker, kubectl, gh, aws, az, gcloud, and more) before you read it.
Run commands normally - compaction is automatic and small outputs pass
through untouched.

### Nothing is lost

When compaction drops lines from a large output, the result ends with an
audit footer accounting for every dropped line and the filter responsible:

    --- banish: dropped 222 lines ---
      groups:
        - filter: grep.per-group  lines: 119
      recover: banish raw a1b2c3d4 (costs ~7009 tokens, only if needed)

Group labels tell you what was dropped (warnings, passing tests, overflow).
Trust the compacted view by default; run `banish raw <hash>` (or the
banish_raw MCP tool) only when a detail you actually need is missing - it
re-reads the full raw output at the token cost shown. Cached raw outputs
expire after 1 hour.

### MCP tools

banish_run (execute a command), banish_raw (recover raw output),
banish_ls, banish_read, banish_fetch. Extension verbs are auto-exposed
as banish_<verb>.

### Extensions

~/.banish/ext/*.bsh define verb shortcuts (gs = git status --short) and
output filters. If a BANISH file exists in the project root, read it for
project-specific verbs and filters. `banish gain` shows cumulative token
savings; BANISH_TRACE=1 annotates dropped lines inline for filter debugging.
