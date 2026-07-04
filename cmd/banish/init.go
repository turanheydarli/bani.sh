package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/scaffold"
)

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [claude-code|cursor|mcp]",
		Short: "Set up banish for a project or agent",
		Long: `Initialize banish:
  banish init              Create a starter BANISH file for this project
  banish init claude-code  Global setup (hooks + extensions + CLAUDE.md)
  banish init cursor       Set up for Cursor (MCP + .cursorrules)
  banish init mcp          MCP config only (for other agents)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir, _ := os.Getwd()

			if len(args) == 0 {
				if err := scaffold.InitProject(dir); err != nil {
					return err
				}
				fmt.Println("{\"ok\":true,\"created\":\"BANISH\"}")
				return nil
			}

			switch args[0] {
			case "claude-code":
				if err := scaffold.InitClaudeCode(dir); err != nil {
					return err
				}
				fmt.Println("banish configured globally for Claude Code.")
				fmt.Println("  ~/.banish/ext/        extensions deployed")
				fmt.Println("  ~/.claude/.mcp.json   MCP server registered")
				fmt.Println("  ~/.claude/hooks/      bash hook installed")
				fmt.Println("  ~/.claude/settings.json  hook registered")
				fmt.Println("  ~/.claude/CLAUDE.md   context added")
				fmt.Println("")
				fmt.Println("All Claude Code sessions will now use banish automatically.")

			case "cursor":
				if err := scaffold.InitCursor(dir); err != nil {
					return err
				}
				fmt.Println("{\"ok\":true,\"created\":[\".cursor/mcp.json\",\".cursorrules\"]}")

			case "mcp":
				if err := scaffold.InitMCPOnly(dir); err != nil {
					return err
				}
				fmt.Println("{\"ok\":true,\"created\":[\".mcp.json\"]}")

			default:
				return fmt.Errorf("unknown target: %s (use claude-code, cursor, or mcp)", args[0])
			}

			return nil
		},
	}

	return cmd
}
