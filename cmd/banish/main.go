package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/analyzer"
	"go.bani.sh/banish/internal/compact"
	"go.bani.sh/banish/internal/extension"
	"go.bani.sh/banish/internal/interpreter"
	"go.bani.sh/banish/internal/manifest"
	"go.bani.sh/banish/internal/runtime"
)

var version = "dev"

var (
	flagHuman   bool
	flagVerbose bool
	flagTimeout string
	flagStats   bool
)

func main() {
	// Fast path: if first arg is not a known subcommand, treat all args as
	// inline banish/bash code. This eliminates `run -e` boilerplate.
	//
	//   banish "ls /var/log ext:log | count"
	//   banish ls /var/log ext:log
	//   echo "ls /tmp" | banish
	//
	if len(os.Args) > 1 && !isSubcommand(os.Args[1]) {
		source := strings.Join(os.Args[1:], " ")
		execDirect(source)
		return
	}

	// stdin pipe: `echo "ls /tmp" | banish`
	if len(os.Args) == 1 && !isTerminal() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"e\":\"IO\",\"m\":%q}\n", err.Error())
			os.Exit(1)
		}
		source := strings.TrimSpace(string(data))
		if source != "" {
			execDirect(source)
			return
		}
	}

	root := &cobra.Command{
		Use:   "banish [code]",
		Short: "Adaptive middleware and scripting runtime for LLM agents",
		Long: `banish -- token-optimized adaptive middleware, MCP server, and scripting runtime.

Usage:
  banish "ls /var/log ext:log | count"   Execute inline (shortest form)
  banish run script.bsh                  Execute a .bsh file
  banish run -e "code"                   Execute inline (explicit)
  echo "ls /tmp" | banish                Execute from stdin
  banish serve                           Start MCP server
  banish init claude-code                Set up for Claude Code`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolVar(&flagHuman, "human", false, "human-readable output")
	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "verbose error output")
	root.PersistentFlags().StringVar(&flagTimeout, "timeout", "30s", "default command timeout")
	root.PersistentFlags().BoolVar(&flagStats, "stats", false, "print token stats after execution")

	root.AddCommand(runCmd())
	root.AddCommand(checkCmd())
	root.AddCommand(versionCmd())
	root.AddCommand(schemaCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(gainCmd())
	root.AddCommand(initCmd())
	root.AddCommand(stopCmd())
	root.AddCommand(startCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(hookCmd())
	root.AddCommand(auditCmd())
	root.AddCommand(discoverCmd())
	root.AddCommand(learnCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "{\"e\":\"CLI\",\"m\":%q}\n", err.Error())
		os.Exit(1)
	}
}

// execDirect runs source code directly without cobra overhead.
func execDirect(source string) {
	interp := newInterpreter()
	tracker := analyzer.New()
	tracker.LoadFrequency() // load persistent data
	inputToks := analyzer.EstimateTokens(source)

	result, err := interp.EvalSource(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"e\":\"EXEC\",\"m\":%q}\n", err.Error())
		os.Exit(1)
	}

	if result != nil {
		outputToks := analyzer.EstimateTokens(result.String())

		tracker.Track(analyzer.Entry{
			Timestamp:  time.Now(),
			Command:    source,
			InputToks:  inputToks,
			OutputToks: outputToks,
			RawToks:    result.RawTokens,
			SavedToks:  result.RawTokens - result.OutTokens,
		})

		// Output: if no meta, return raw result (no JSON wrapper).
		// If meta present, wrap in JSON with metadata.
		hasMetadata := len(result.Meta) > 0
		if flagHuman || interp.Human() {
			fmt.Println(result.String())
		} else if hasMetadata {
			b, _ := result.JSON()
			fmt.Println(string(b))
		} else {
			// Pure response -- just the data, no wrapper
			switch v := result.Data.(type) {
			case string:
				if v != "" {
					fmt.Println(v)
				}
			case nil:
				// nothing
			default:
				b, _ := json.Marshal(v)
				fmt.Println(string(b))
			}
		}
	}

	// Persist frequency data
	tracker.SaveFrequency()

	if flagStats {
		stats := tracker.SessionStats()
		b, _ := json.Marshal(stats)
		fmt.Fprintf(os.Stderr, "\n--- stats ---\n%s\n", string(b))
	}
}

var subcommands = map[string]bool{
	"run": true, "check": true, "version": true, "schema": true,
	"serve": true, "gain": true, "init": true, "help": true,
	"stop": true, "start": true, "status": true, "hook": true, "audit": true,
	"discover": true, "learn": true,
	"--human": true, "--verbose": true, "--timeout": true, "--stats": true,
	"-h": true, "--help": true,
}

func isSubcommand(arg string) bool {
	return subcommands[arg]
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func newInterpreter() *interpreter.Interpreter {
	reg := interpreter.NewVerbRegistry()
	runtime.RegisterBuiltins(reg)

	// Collect script-based filters from extensions and manifest.
	var scriptFilters []compact.ScriptFilterDef

	// Load extensions from ~/.banish/ext/
	home, _ := os.UserHomeDir()
	if home != "" {
		loader := extension.NewLoader()
		loader.LoadDir(filepath.Join(home, ".banish", "ext"))
		loader.Register(reg)

		// Collect filters from extensions
		for _, f := range loader.Filters() {
			scriptFilters = append(scriptFilters, compact.ScriptFilterDef{
				Name: f.Name, Match: f.Match, Compact: f.Compact,
			})
		}
	}

	// Load BANISH project manifest (walk up from cwd)
	cwd, _ := os.Getwd()
	if path := manifest.FindBanishFile(cwd); path != "" {
		bf, err := manifest.LoadBanishFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "banish: BANISH parse error: %v\n", err)
		} else {
			for _, v := range bf.Verbs {
				reg.RegisterExtension(v.Name, extension.MakeVerbHandler(v.Name, v.Expand))
			}
			// Collect filters from BANISH manifest
			for _, f := range bf.Filters {
				scriptFilters = append(scriptFilters, compact.ScriptFilterDef{
					Name: f.Name, Match: f.Match, Compact: f.Compact,
				})
			}
		}
	}

	// System fallback: unknown verbs exec through shell.
	exec := runtime.NewExecutor()
	fallback := runtime.FallbackHandler(exec, scriptFilters)
	reg.SetFallback(fallback)
	reg.RegisterBuiltin("__fallback__", fallback)

	opts := []interpreter.Option{
		interpreter.WithRegistry(reg),
		interpreter.WithOutput(os.Stdout),
	}

	return interpreter.New(opts...)
}
