## Banish

banish is installed. Bash commands are automatically routed through banish
for output compaction and token savings.

What banish does:
- Compacts command output (git status, log, diff, ls, find, grep, etc.)
- Tracks token savings (run banish gain to see stats)
- Provides MCP tools (banish_run, banish_ls, banish_read, banish_fetch)
- Auto-exposes extension verbs as MCP tools

Extensions in ~/.banish/ext/ define:
- Verb shortcuts (gs = git status --short, dps = docker ps compact, etc.)
- Output filters (strip noise from git, npm, cargo, docker, kubectl, etc.)

### BANISH file

If a BANISH file exists in the project root, read it for project-specific
verbs and filters.
