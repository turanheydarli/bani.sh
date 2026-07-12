Bash commands are routed through banish for output compaction and token savings.
banish provides MCP tools and auto-exposes extension verbs.
Read the BANISH file in the project root for project-specific verbs and filters.
Compacted large outputs end with an audit footer accounting for every dropped
line; "banish raw <hash>" recovers the raw output verbatim. Use it only when
the compacted output is missing something you actually need - the footer shows
the token cost of recovering.
