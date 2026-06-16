package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// auditEntry mirrors one line of ~/.banish/hook-audit.jsonl.
type auditEntry struct {
	TS      string `json:"ts"`
	Host    string `json:"host"`
	Command string `json:"command"`
}

func auditCmd() *cobra.Command {
	var clear bool
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show commands banish auto-approved via the hook",
		Long: `List the commands banish let run without prompting you, because your
agent's permission rules already allowed them. Only auto-approvals are recorded;
commands that prompted you or were deferred are not.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				return fmt.Errorf("cannot determine home directory")
			}
			path := filepath.Join(home, ".banish", "hook-audit.jsonl")

			if clear {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Println("Audit log cleared.")
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No auto-approvals recorded yet.")
					return nil
				}
				return err
			}
			defer f.Close()

			var entries []auditEntry
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				var e auditEntry
				if json.Unmarshal(scanner.Bytes(), &e) == nil {
					entries = append(entries, e)
				}
			}
			if err := scanner.Err(); err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No auto-approvals recorded yet.")
				return nil
			}

			start := 0
			if limit > 0 && len(entries) > limit {
				start = len(entries) - limit
			}
			shown := entries[start:]

			fmt.Printf("Auto-approved commands (%d total, showing %d):\n", len(entries), len(shown))
			for _, e := range shown {
				fmt.Printf("  %s  [%s]  %s\n", e.TS, e.Host, e.Command)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "delete the audit log")
	cmd.Flags().IntVar(&limit, "limit", 20, "show the most recent N entries (0 for all)")
	return cmd
}
