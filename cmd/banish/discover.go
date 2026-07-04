package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/history"
)

// nonFilterable are base commands that are stateful or trivial, so a compaction
// filter never makes sense for them. They are excluded from discover.
var nonFilterable = map[string]bool{
	"banish": true, "cd": true, "export": true, "source": true, "alias": true,
	"eval": true, "sleep": true, "echo": true, "pwd": true, "true": true,
	"false": true, "set": true, "unset": true, "wait": true, "exit": true,
}

func discoverCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find frequent commands that have no banish filter yet",
		Long: `Scan your Claude Code history for the commands your agent runs most, and
surface the frequent ones that no installed .bsh filter compacts yet -- the best
candidates for a new filter. For each, banish prints a ready-to-edit stub.`,
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

			matches := loadMatchPatterns()

			freq := make(map[string]int)
			example := make(map[string]string)
			for _, c := range cmds {
				if c.Base == "" || nonFilterable[c.Base] {
					continue
				}
				if covered(c.Raw, matches) {
					continue
				}
				freq[c.Base]++
				if example[c.Base] == "" {
					example[c.Base] = c.Raw
				}
			}
			if len(freq) == 0 {
				fmt.Println("Every command in your history is already covered by a filter. Nice.")
				return nil
			}

			type row struct {
				base  string
				count int
			}
			rows := make([]row, 0, len(freq))
			for b, n := range freq {
				rows = append(rows, row{b, n})
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].count != rows[j].count {
					return rows[i].count > rows[j].count
				}
				return rows[i].base < rows[j].base
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			fmt.Println("Frequent commands with no filter yet:")
			fmt.Println(strings.Repeat("-", 50))
			for _, r := range rows {
				fmt.Printf("  %-16s %4d runs\n", r.base, r.count)
			}
			fmt.Println()
			fmt.Println("Add a filter for the top one with a .bsh in ~/.banish/ext/:")
			fmt.Println()
			top := rows[0].base
			fmt.Printf("  !filter %s\n", top)
			fmt.Printf("  !match %s\n", top)
			fmt.Printf("  !compact \"head -40\"   # tune this: grep/sed/tail the noise away\n")
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "show the top N uncovered commands")
	return cmd
}

// loadMatchPatterns returns the substrings from every !match line in the
// installed .bsh extensions (~/.banish/ext). A command containing any of these
// substrings is already handled by a filter.
func loadMatchPatterns() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	dir := filepath.Join(home, ".banish", "ext")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var patterns []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bsh") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if p, ok := strings.CutPrefix(line, "!match "); ok {
				if p = strings.TrimSpace(p); p != "" {
					patterns = append(patterns, p)
				}
			}
		}
		f.Close()
	}
	return patterns
}

// covered reports whether any filter pattern matches the command.
func covered(cmd string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(cmd, p) {
			return true
		}
	}
	return false
}
