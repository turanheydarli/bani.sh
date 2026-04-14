package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func schemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Emit verb catalog as compact JSON for LLM system prompts",
		RunE: func(_ *cobra.Command, _ []string) error {
			verbs := []map[string]any{
				{"name": "echo", "args": "target", "desc": "Output literal value"},
				{"name": "ls", "args": "path ext? size? age? sort?", "desc": "List files with structured output"},
				{"name": "read", "args": "path lines?", "desc": "Read file contents"},
				{"name": "cat", "args": "path", "desc": "Alias for read"},
				{"name": "write", "args": "path", "desc": "Write piped input to file"},
				{"name": "mkdir", "args": "path", "desc": "Create directory"},
				{"name": "rm", "args": "path", "desc": "Remove file or directory"},
				{"name": "cp", "args": "src to:dst", "desc": "Copy file"},
				{"name": "mv", "args": "src to:dst", "desc": "Move or rename file"},
				{"name": "head", "args": "path? n?", "desc": "First N lines (default 10)"},
				{"name": "tail", "args": "path? n?", "desc": "Last N lines (default 10)"},
				{"name": "env", "args": "name", "desc": "Read environment variable"},
				{"name": "sleep", "args": "seconds", "desc": "Wait N seconds"},
				{"name": "count", "args": "(piped)", "desc": "Count lines or items in input"},
				{"name": "fetch", "args": "url method?", "desc": "HTTP request, returns status+body+headers"},
			}

			schema := map[string]any{
				"version": version,
				"verbs":   verbs,
				"syntax": map[string]string{
					"pipe":     "cmd1 | cmd2",
					"seq":      "cmd1 ; cmd2",
					"parallel": "cmd1 & cmd2",
					"filter":   "cmd ? key:value",
					"and":      "cmd1 && cmd2",
					"or":       "cmd1 || cmd2",
					"assign":   "$var = cmd",
					"mcp":      "@server verb modifiers",
					"redirect": "cmd -> file",
					"modifier": "key:value",
				},
			}

			b, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
}
