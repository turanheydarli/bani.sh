package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
)

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check <file.bsh>",
		Short: "Parse a .bsh file and report errors without executing",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}

			l := lexer.New(string(data))
			p := parser.New(l)
			prog := p.ParseProgram()

			if errs := p.Errors(); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "%s\n", e)
				}
				os.Exit(2)
			}

			fmt.Printf("{\"ok\":true,\"statements\":%d}\n", len(prog.Statements))
			return nil
		},
	}
}
