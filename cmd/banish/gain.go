package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func gainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gain",
		Short: "Show token savings statistics",
		Run: func(_ *cobra.Command, _ []string) {
			// Session-only stats for now. Persistent SQLite in future.
			fmt.Println("{\"note\":\"session stats available via --stats flag on run\"}")
		},
	}
}
