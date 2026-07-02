package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/analyzer"
)

// defaultPricePerMTok is the cost in USD per million input tokens used to
// estimate dollar savings. banish compacts what the agent reads, so saved
// tokens are input tokens. The default is the Claude Opus input rate (Claude
// Code's default model); override with --price for Sonnet (3), Haiku (1), etc.
const defaultPricePerMTok = 5.0

func gainCmd() *cobra.Command {
	var jsonOut bool
	var reset bool
	var price float64

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
			costSaved := float64(stats.SavedTokens) / 1e6 * price

			if jsonOut {
				b, _ := json.Marshal(stats)
				var m map[string]any
				json.Unmarshal(b, &m)
				m["est_cost_usd"] = roundCents(costSaved)
				m["price_per_mtok"] = price
				out, _ := json.Marshal(m)
				fmt.Println(string(out))
				return nil
			}

			printGain(stats, costSaved)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&reset, "reset", false, "clear all tracking data")
	cmd.Flags().Float64Var(&price, "price", defaultPricePerMTok, "USD per million input tokens, for the cost estimate")

	cmd.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Clear all token-savings tracking data",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := resetFreqData(); err != nil {
				return err
			}
			fmt.Println("Tracking data reset.")
			return nil
		},
	})

	return cmd
}

// roundCents rounds a dollar amount to two decimal places.
func roundCents(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func printGain(s *analyzer.Stats, costSaved float64) {
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
		printKV("Est. cost saved", fmt.Sprintf("$%.2f", costSaved))
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
	path := filepath.Join(home, ".banish", "freq.json")
	return os.WriteFile(path, []byte("[]"), 0644)
}
