// Package bench measures token savings of the builtin compaction pipeline
// over a corpus of captured command outputs. Each fixture pairs a command
// with its raw output and the bounds the compacted output must satisfy:
// a minimum savings percentage and must-keep patterns that guard against
// information loss. The corpus is embedded so "banish bench" runs anywhere;
// golden files are enforced by the tests in this package.
package bench

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"go.banish.sh/banish/internal/analyzer"
	"go.banish.sh/banish/internal/compact"
	"go.banish.sh/banish/internal/extension"
)

//go:embed corpus
var corpusFS embed.FS

// Fixture is one corpus entry. Command is the command as an agent would type
// it; the pipeline applies rewrite rules to it, so Raw must hold the output
// of the rewritten command while Orig holds the original command's output
// (the honest baseline). When Orig is empty, Raw is the baseline.
type Fixture struct {
	Name       string   `json:"name"`
	Display    string   `json:"display"`
	Command    string   `json:"command"`
	Exit       int      `json:"exit"`
	MinSavePct float64  `json:"min_save_pct"`
	MustKeep   []string `json:"must_keep"`
	Readme     bool     `json:"readme"`

	Raw    string `json:"-"`
	Orig   string `json:"-"`
	Stderr string `json:"-"`
}

// Result is the measured outcome of one fixture.
type Result struct {
	Fixture  Fixture
	Executed string // command after rewrite rules
	Handler  string // native renderer or filter name; "" = raw passthrough
	Output   string
	RawToks  int64 // estimated tokens of the baseline output
	OutToks  int64 // estimated tokens after compaction
	SavePct  float64
}

// LoadCorpus reads the embedded manifest and fixture files.
func LoadCorpus() ([]Fixture, error) {
	data, err := corpusFS.ReadFile("corpus/corpus.json")
	if err != nil {
		return nil, fmt.Errorf("bench: read manifest: %w", err)
	}
	var fixtures []Fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return nil, fmt.Errorf("bench: parse manifest: %w", err)
	}
	for i := range fixtures {
		f := &fixtures[i]
		raw, err := corpusFS.ReadFile("corpus/" + f.Name + ".raw")
		if err != nil {
			return nil, fmt.Errorf("bench: fixture %s: %w", f.Name, err)
		}
		f.Raw = string(raw)
		f.Orig = readOptional("corpus/" + f.Name + ".orig")
		f.Stderr = readOptional("corpus/" + f.Name + ".stderr")
	}
	return fixtures, nil
}

func readOptional(path string) string {
	data, err := fs.ReadFile(corpusFS, path)
	if err != nil {
		return ""
	}
	return string(data)
}

// Pipeline is a hermetic compaction pipeline: embedded builtin packs plus
// embedded defaults, with user extensions and BANISH manifests deliberately
// excluded so results are reproducible on any machine.
type Pipeline struct {
	registry *compact.Registry
	rewriter *compact.Rewriter
}

// NewPipeline builds the pipeline the way cmd/banish wires the agent path:
// builtin packs first, embedded defaults last (lowest precedence).
func NewPipeline() (*Pipeline, error) {
	builtins, err := extension.Builtin()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	sort.Strings(names)

	loader := extension.NewLoader()
	for _, n := range names {
		if err := loader.LoadSource(n, builtins[n]); err != nil {
			return nil, fmt.Errorf("bench: load %s: %w", n, err)
		}
	}
	loader.LoadDefaults()

	var filters []compact.ScriptFilterDef
	var rules []compact.RewriteRule
	for _, f := range loader.Filters() {
		filters = append(filters, compact.ScriptFilterDef{
			Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
		})
	}
	for _, rw := range loader.Rewrites() {
		rules = append(rules, compact.RewriteRule{
			Name: rw.Name, Match: rw.Match, Unless: rw.Unless, To: rw.To,
		})
	}

	reg := compact.NewRegistry()
	reg.RegisterScriptFilters(filters)
	return &Pipeline{registry: reg, rewriter: compact.NewRewriter(rules)}, nil
}

// Run measures one fixture through rewrite plus the full compaction cascade.
func (p *Pipeline) Run(f Fixture) Result {
	executed, _ := p.rewriter.Rewrite(f.Command)
	out, handler := p.registry.Compact(executed, f.Raw, f.Stderr, f.Exit)
	if handler == "" {
		out = strings.TrimRight(f.Raw, "\n")
		if f.Stderr != "" {
			out += "\n[stderr] " + strings.TrimRight(f.Stderr, "\n")
		}
	}

	baseline := f.Orig
	if baseline == "" {
		baseline = f.Raw
	}
	rawToks := analyzer.EstimateTokens(baseline)
	outToks := analyzer.EstimateTokens(out)
	pct := 0.0
	if rawToks > 0 {
		pct = float64(rawToks-outToks) / float64(rawToks) * 100
	}
	return Result{
		Fixture: f, Executed: executed, Handler: handler, Output: out,
		RawToks: rawToks, OutToks: outToks, SavePct: pct,
	}
}

// RunAll loads the corpus and measures every fixture.
func RunAll() ([]Result, error) {
	fixtures, err := LoadCorpus()
	if err != nil {
		return nil, err
	}
	p, err := NewPipeline()
	if err != nil {
		return nil, err
	}
	results := make([]Result, len(fixtures))
	for i, f := range fixtures {
		results[i] = p.Run(f)
	}
	return results, nil
}

// Check returns the threshold and must-keep violations for a result,
// nil when clean.
func Check(r Result) []string {
	var v []string
	if r.SavePct < r.Fixture.MinSavePct {
		v = append(v, fmt.Sprintf("savings %.0f%% below minimum %.0f%%", r.SavePct, r.Fixture.MinSavePct))
	}
	for _, pat := range r.Fixture.MustKeep {
		re, err := regexp.Compile(pat)
		if err != nil {
			v = append(v, "invalid must_keep regex: "+pat)
			continue
		}
		if !re.MatchString(r.Output) {
			v = append(v, "must_keep pattern lost in output: "+pat)
		}
	}
	return v
}

// Totals aggregates baseline and compacted tokens across results.
func Totals(results []Result) (rawToks, outToks int64, savePct float64) {
	for _, r := range results {
		rawToks += r.RawToks
		outToks += r.OutToks
	}
	if rawToks > 0 {
		savePct = float64(rawToks-outToks) / float64(rawToks) * 100
	}
	return rawToks, outToks, savePct
}

// RenderTable formats results as an aligned text table with a totals row.
func RenderTable(results []Result) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "COMMAND\tHANDLER\tRAW\tCOMPACT\tSAVED")
	for _, r := range results {
		handler := r.Handler
		if handler == "" {
			handler = "(raw)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%.0f%%\n",
			r.Fixture.Display, handler, r.RawToks, r.OutToks, r.SavePct)
	}
	rawToks, outToks, pct := Totals(results)
	fmt.Fprintf(tw, "TOTAL\t\t%d\t%d\t%.0f%%\n", rawToks, outToks, pct)
	tw.Flush()
	return b.String()
}

// ReadmeTable renders the markdown savings table for fixtures marked
// readme: true, matching the README's Supported section format.
func ReadmeTable(results []Result) string {
	var b strings.Builder
	b.WriteString("| Command | Raw | Compacted | Savings |\n")
	b.WriteString("|---------|-----|-----------|---------|\n")
	for _, r := range results {
		if !r.Fixture.Readme {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | %d tok | %d tok | %.0f%% |\n",
			r.Fixture.Display, r.RawToks, r.OutToks, r.SavePct)
	}
	return b.String()
}

const (
	readmeBegin = "<!-- bench:begin -->"
	readmeEnd   = "<!-- bench:end -->"
)

// UpdateReadme replaces the savings table between the bench markers in the
// file at path. Returns whether the file changed.
func UpdateReadme(path string, results []Result) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	s := string(data)
	begin := strings.Index(s, readmeBegin)
	end := strings.Index(s, readmeEnd)
	if begin < 0 || end < 0 || end < begin {
		return false, fmt.Errorf("bench: markers %s / %s not found in %s", readmeBegin, readmeEnd, path)
	}
	updated := s[:begin+len(readmeBegin)] + "\n" + ReadmeTable(results) + s[end:]
	if updated == s {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(updated), 0644)
}
