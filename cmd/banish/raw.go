package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/rawcache"
)

func rawCmd() *cobra.Command {
	var clear bool

	cmd := &cobra.Command{
		Use:   "raw <hash>",
		Short: "Print the uncompacted output of a recent command",
		Long: `Print the raw stdout+stderr a compacted command produced, byte for byte.

Compacted outputs end with an audit footer naming the recover hash:
  recover: banish raw a1b2c3d4

Raw outputs live in ~/.banish/cache/raw/ for a limited time (default 1 hour,
50 MB cap; "cache" section in ~/.banish/config.json). Cached outputs may
contain secrets; files are private to the user (0600) and never stored inside
a repository. Disable the cache entirely with {"cache": {"raw": false}}.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if clear {
				if err := rawcache.Clear(); err != nil {
					return err
				}
				fmt.Println("Raw output cache cleared.")
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("usage: banish raw <hash> (or banish raw --clear)")
			}
			data, err := rawcache.Get(args[0])
			if err != nil {
				return err
			}
			os.Stdout.Write(data)
			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "delete all cached raw outputs")

	return cmd
}
