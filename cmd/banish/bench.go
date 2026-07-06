package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/bench"
)

func benchCmd() *cobra.Command {
	var jsonOut bool
	var check bool
	var writeReadme string

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Measure token savings over the builtin fixture corpus",
		Long: `Runs the embedded fixture corpus through the compaction pipeline
(builtin packs + defaults, no user extensions) and reports per-command and
aggregate token savings. The same corpus gates CI via go test ./internal/bench.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			results, err := bench.RunAll()
			if err != nil {
				return err
			}

			if writeReadme != "" {
				changed, err := bench.UpdateReadme(writeReadme, results)
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
				b, _ := json.Marshal(rows)
				fmt.Println(string(b))
			} else {
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
	return cmd
}
