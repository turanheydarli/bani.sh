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
				outputToks := analyzer.EstimateTokens(result.String())
				savings := 0
				if result.Hint != nil {
					savings = result.Hint.Saved
				}
				tracker.Track(analyzer.Entry{
					Timestamp: time.Now(),
					Command:   source,
					InputToks: inputToks,
					OutputToks: outputToks,
					Savings:   savings,
				})

				// Check for extension suggestion
				builtins := map[string]bool{
					"ls": true, "read": true, "cat": true, "write": true,
					"mkdir": true, "rm": true, "cp": true, "mv": true,
					"head": true, "tail": true, "echo": true, "env": true,
					"sleep": true, "count": true, "fetch": true,
				}
				if suggest := tracker.SuggestExtension(source, builtins); suggest != nil {
					if result.Meta == nil {
						result.Meta = make(map[string]any)
					}
					result.Meta["_suggest_extension"] = suggest
				}

				// Output: pure when no metadata, JSON-wrapped when hints/suggestions present
				hasMetadata := result.Hint != nil || len(result.Meta) > 0
				if flagHuman || interp.Human() {
					fmt.Println(result.String())
				} else if hasMetadata {
					b, _ := result.JSON()
					fmt.Println(string(b))
				} else {
					switch v := result.Data.(type) {
					case string:
						if v != "" {
							fmt.Println(v)
						}
					case nil:
						// nothing
					default:
						b, _ := json.Marshal(v)
						fmt.Println(string(b))
					}
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
