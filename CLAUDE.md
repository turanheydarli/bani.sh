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

### Benchmark rule

After changing anything that affects compaction - .bsh filter packs
(internal/extension/builtin/, defaults.bsh), internal/compact/, rewrites,
or the bench corpus itself - always run the benchmark suite:

- `go test ./internal/bench/` must pass (goldens, savings thresholds,
  must-keep patterns).
- If the change intentionally alters filter output, regenerate goldens with
  `go test ./internal/bench/ -update`, then review the golden diffs before
  committing - never commit regenerated goldens unreviewed.
- The README savings table refreshes automatically on merge to main (the
  update-readme CI job); refresh locally with
  `go run ./cmd/banish bench --write-readme README.md` only to preview.
- New filters should land with a corpus fixture (raw output + manifest entry
  in internal/bench/corpus/corpus.json with min_save_pct, must_keep, and a
  general `group` label for the README row).
