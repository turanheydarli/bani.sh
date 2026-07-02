package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/analyzer"
)

func runCmd() *cobra.Command {
	var inline string

	cmd := &cobra.Command{
		Use:   "run [file.bsh]",
		Short: "Execute a .bsh file or inline code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var source string

			if inline != "" {
				source = inline
			} else if len(args) == 1 {
				data, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("read %s: %w", args[0], err)
				}
				source = string(data)
			} else {
				return fmt.Errorf("usage: banish run <file.bsh> or banish run -e \"code\"")
			}

			interp := newInterpreter()
			tracker := analyzer.New()
			tracker.LoadFrequency()

			inputToks := analyzer.EstimateTokens(source)

			result, err := interp.EvalSource(source)
			if err != nil {
				return err
			}

			if result != nil {
				printed, show := renderOutput(result, flagHuman || interp.Human())
				outputToks := analyzer.EstimateTokens(printed)

				var saved int64
				if result.RawTokens > 0 || result.OutTokens > 0 {
					saved = result.RawTokens - outputToks
				}
				rewrites := int64(0)
				if result.Rewritten != "" {
					rewrites = 1
				}
				tracker.Track(analyzer.Entry{
					Timestamp:  time.Now(),
					Command:    source,
					InputToks:  inputToks,
					OutputToks: outputToks,
					RawToks:    result.RawTokens,
					SavedToks:  saved,
					Rewrites:   rewrites,
				})

				if show {
					fmt.Println(printed)
				}
			}

			tracker.SaveFrequency()

			if flagStats {
				stats := tracker.SessionStats()
				b, _ := json.Marshal(stats)
				fmt.Fprintf(os.Stderr, "\n--- stats ---\n%s\n", string(b))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&inline, "exec", "e", "", "execute inline banish code")

	return cmd
}
