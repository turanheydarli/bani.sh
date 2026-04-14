package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build info",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("banish %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		},
	}
}
