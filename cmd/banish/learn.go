package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/history"
)

// lookAhead is how many later commands in the same session count as a possible
// correction for a failed command.
const lookAhead = 5

func learnCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Find command mistakes your agent corrected",
		Long: `Scan your Claude Code history for commands that failed and were then
re-run successfully with a small change -- the agent's own corrections. Seeing
the recurring ones is a cheap way to spot a flag it keeps getting wrong or a
verb worth adding.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cmds, err := history.Commands()
			if err != nil {
				return err
			}
			if len(cmds) == 0 {
				fmt.Println("No command history found under ~/.claude/projects.")
				return nil
			}

			type correction struct {
				wrong string
				right string
			}
			counts := make(map[correction]int)

			for i := range cmds {
				if !cmds[i].IsError || cmds[i].Base == "" {
					continue
				}
				// Look ahead in the same session for a successful command with
				// the same base but different text -- the corrected version.
				for j := i + 1; j < len(cmds) && j <= i+lookAhead; j++ {
					if cmds[j].Session != cmds[i].Session {
						break
					}
					if cmds[j].IsError || cmds[j].Base != cmds[i].Base {
						continue
					}
					if cmds[j].Raw == cmds[i].Raw {
						continue // identical retry, not a correction
					}
					counts[correction{wrong: cmds[i].Raw, right: cmds[j].Raw}]++
					break
				}
			}

			if len(counts) == 0 {
				fmt.Println("No corrected command mistakes found in your history.")
				return nil
			}

			type row struct {
				correction
				n int
			}
			rows := make([]row, 0, len(counts))
			for c, n := range counts {
				rows = append(rows, row{c, n})
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].n != rows[j].n {
					return rows[i].n > rows[j].n
				}
				return rows[i].wrong < rows[j].wrong
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			fmt.Println("Command mistakes your agent corrected:")
			fmt.Println(strings.Repeat("-", 50))
			for _, r := range rows {
				times := ""
				if r.n > 1 {
					times = fmt.Sprintf("  (x%d)", r.n)
				}
				fmt.Printf("  bad:  %s\n", truncate(r.wrong, 100))
				fmt.Printf("  fix:  %s%s\n\n", truncate(r.right, 100), times)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 15, "show the top N corrections")
	return cmd
}

// truncate shortens s to n runes with an ellipsis marker.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
