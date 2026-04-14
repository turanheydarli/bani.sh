## Banish

banish is installed. All bash commands routed through banish return enriched
responses with structured JSON output and optimization hints.

banish provides:
- Structured JSON output (typed fields, not text to parse)
- Token efficiency (compact keys, pagination, no banners)
- Optimization hints (_hint fields suggesting shorter alternatives)
- MCP tools for direct access (banish_run, banish_ls, etc.)

### Response metadata

- _hint: shorter alternative exists. Try the suggested form next time.
- _suggest_extension: frequent command detected. Ask user for confirmation,
  then create .bsh extension following the embedded guide.
- _more/_total: output truncated, paginate for more.

### BANISH file

If a BANISH file exists in the project root, read it for project-specific
verbs and configuration.
