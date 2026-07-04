package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/scaffold"
)

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [claude-code]",
		Short: "Disable banish interception (keeps everything installed)",
		Long: `Turn banish off for an agent without uninstalling it.

Removes only the hook that routes commands through banish. Extensions, MCP
config, the hook script, and CLAUDE.md are left in place so 'banish start'
re-enables instantly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			agent := agentArg(args)
			if err := scaffold.Stop(agent); err != nil {
				return err
			}
			fmt.Printf("{\"ok\":true,\"agent\":%q,\"active\":false}\n", agent)
			return nil
		},
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start [claude-code]",
		Short: "Re-enable banish interception",
		Long:  `Turn banish back on for an agent that was stopped with 'banish stop'.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			agent := agentArg(args)
			if err := scaffold.Start(agent); err != nil {
				return err
			}
			fmt.Printf("{\"ok\":true,\"agent\":%q,\"active\":true}\n", agent)
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether banish is active for each agent",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := scaffold.Status()
			if err != nil {
				return err
			}
			b, _ := json.Marshal(st)
			fmt.Println(string(b))
			return nil
		},
	}
}

// agentArg returns the requested agent, defaulting to claude-code.
func agentArg(args []string) string {
	if len(args) == 0 {
		return "claude-code"
	}
	return args[0]
}
