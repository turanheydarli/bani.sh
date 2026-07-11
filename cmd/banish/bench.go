package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/bench"
	"go.banish.sh/banish/internal/token/counter"
)

func benchCmd() *cobra.Command {
	var jsonOut bool
	var check bool
	var writeReadme string
	var tokenizer string

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Measure token savings over the builtin fixture corpus",
		Long: `Runs the embedded fixture corpus through the compaction pipeline
(builtin packs + defaults, no user extensions) and reports per-command and
aggregate token savings. The same corpus gates CI via go test ./internal/bench.

Token counts default to a char-based estimate (~4 chars/token, roughly
+/-30% error) so runs are deterministic and offline. Pass
--tokenizer=anthropic to measure with Anthropic's count_tokens endpoint
(requires ANTHROPIC_API_KEY; results are cached in ~/.banish/tokcache.json).
The default can also be set via "tokenizer" in ~/.banish/config.json.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := benchCounter(tokenizer)
			if err != nil {
				return err
			}

			results, err := bench.RunAllWith(c)
			if err != nil {
				return err
			}
			if f, ok := c.(interface{ Flush() error }); ok {
				defer f.Flush()
			}

			if writeReadme != "" {
				changed, err := bench.UpdateReadme(writeReadme, results, c.Name())
				if err != nil {
					return err
				}
				if changed {
					fmt.Fprintf(os.Stderr, "updated savings table in %s\n", writeReadme)
				} else {
					fmt.Fprintf(os.Stderr, "%s already up to date\n", writeReadme)
				}
			}

			if jsonOut {
				type row struct {
					Name     string  `json:"name"`
					Display  string  `json:"display"`
					Executed string  `json:"executed"`
					Handler  string  `json:"handler"`
					RawToks  int64   `json:"raw_toks"`
					OutToks  int64   `json:"out_toks"`
					SavePct  float64 `json:"save_pct"`
				}
				rows := make([]row, len(results))
				for i, r := range results {
					rows[i] = row{
						Name: r.Fixture.Name, Display: r.Fixture.Display,
						Executed: r.Executed, Handler: r.Handler,
						RawToks: r.RawToks, OutToks: r.OutToks, SavePct: r.SavePct,
					}
				}
				fmt.Fprintf(os.Stderr, "tokenizer: %s\n", c.Name())
				b, _ := json.Marshal(rows)
				fmt.Println(string(b))
			} else {
				fmt.Printf("tokenizer: %s\n\n", c.Name())
				fmt.Print(bench.RenderTable(results))
			}

			if check {
				failed := false
				for _, r := range results {
					for _, v := range bench.Check(r) {
						failed = true
						fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", r.Fixture.Name, v)
					}
				}
				if failed {
					return fmt.Errorf("bench check failed")
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit results as JSON")
	cmd.Flags().BoolVar(&check, "check", false, "exit non-zero if any fixture misses its thresholds")
	cmd.Flags().StringVar(&writeReadme, "write-readme", "", "regenerate the savings table between bench markers in the given file")
	cmd.Flags().StringVar(&tokenizer, "tokenizer", "", "token counter: heuristic or anthropic (default: config, then heuristic)")
	return cmd
}

// benchCounter resolves the tokenizer for a bench run. An explicit flag
// wins; --tokenizer=anthropic without a key is an error rather than a silent
// downgrade, because the caller asked for real numbers. With no flag the
// config decides, falling back to the heuristic.
func benchCounter(flag string) (counter.Counter, error) {
	switch flag {
	case "heuristic":
		return counter.CharHeuristic{}, nil
	case "anthropic":
		c, err := counter.NewAnthropic(counter.LoadConfig().TokenizerModel)
		if err != nil {
			return nil, err
		}
		return c, nil
	case "":
		return counter.FromConfig(), nil
	default:
		return nil, fmt.Errorf("unknown tokenizer %q (want heuristic or anthropic)", flag)
	}
}
