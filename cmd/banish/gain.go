package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/analyzer"
)

func gainCmd() *cobra.Command {
	var jsonOut bool
	var reset bool

	cmd := &cobra.Command{
		Use:   "gain",
		Short: "Show token savings statistics",
		Long:  "Display cumulative token savings from output compaction.\nData is loaded from ~/.banish/freq.json.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if reset {
				if err := resetFreqData(); err != nil {
					return err
				}
				fmt.Println("Tracking data reset.")
				return nil
			}

			a := analyzer.New()
			a.LoadFrequency()
			stats := a.SessionStats()

			if jsonOut {
				b, _ := json.Marshal(stats)
				fmt.Println(string(b))
				return nil
			}

			printGain(stats)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&reset, "reset", false, "clear all tracking data")
	return cmd
}

func printGain(s *analyzer.Stats) {
	if s.Commands == 0 {
		fmt.Println("No tracking data yet.")
		fmt.Println("Run some commands through banish to start tracking savings.")
		return
	}

	fmt.Println("Banish Token Savings")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Println()

	printKV("Total commands", fmt.Sprintf("%d", s.Commands))
	printKV("Input tokens", formatToks(s.InputTokens))
	printKV("Output tokens", formatToks(s.OutputTokens))

	if s.RawTokens > 0 {
		printKV("Raw tokens (before)", formatToks(s.RawTokens))
		printKV("Tokens saved", fmt.Sprintf("%s (%.1f%%)", formatToks(s.SavedTokens), s.SavingsPct))
	}

	fmt.Println()

	if len(s.TopVerbs) > 0 {
		fmt.Println("By Command")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("  %-20s %6s %10s\n", "Command", "Count", "Saved")
		fmt.Println(strings.Repeat("-", 50))

		// Sort by saved descending
		verbs := make([]analyzer.VerbStat, len(s.TopVerbs))
		copy(verbs, s.TopVerbs)
		sort.Slice(verbs, func(i, j int) bool {
			return verbs[i].Saved > verbs[j].Saved
		})

		for _, v := range verbs {
			name := v.Name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			savedStr := formatToks(v.Saved)
			if v.Saved == 0 {
				savedStr = "-"
			}
			fmt.Printf("  %-20s %6d %10s\n", name, v.Count, savedStr)
		}
		fmt.Println()
	}
}

func printKV(key, value string) {
	fmt.Printf("%-20s %s\n", key+":", value)
}

func formatToks(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}

	s := fmt.Sprintf("%d", n)
	if n >= 1000000 {
		s = fmt.Sprintf("%.1fM", float64(n)/1000000)
	} else if n >= 1000 {
		s = fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if neg {
		return "-" + s
	}
	return s
}

// resetFreqData clears the frequency data file. Used by banish gain --reset.
func resetFreqData() error {
	home, _ := os.UserHomeDir()
	if home == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	path := home + "/.banish/freq.json"
	return os.WriteFile(path, []byte("[]"), 0644)
}
