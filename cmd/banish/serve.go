package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/mcp"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start banish as an MCP server (stdio transport)",
		Long:  "Expose all banish verbs as MCP tools. Add to agent config:\n  {\"banish\": {\"command\": \"banish\", \"args\": [\"serve\"]}}",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			interp := newInterpreter()
			server := mcp.NewServer(interp)

			return server.Serve(ctx)
		},
	}
}
