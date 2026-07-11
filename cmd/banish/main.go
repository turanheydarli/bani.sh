package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/analyzer"
	"go.banish.sh/banish/internal/compact"
	"go.banish.sh/banish/internal/extension"
	"go.banish.sh/banish/internal/interpreter"
	"go.banish.sh/banish/internal/manifest"
	"go.banish.sh/banish/internal/runtime"
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
		Use:     "banish [code]",
		Version: version,
		Short:   "Adaptive middleware and scripting runtime for LLM agents",
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
		// After a human-facing subcommand, surface a one-line notice if a newer
		// release exists. Gated to a terminal and an allowlist so agents, hooks,
		// and CI never see it.
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			if noticeAllowed(cmd.Name()) {
				maybeNotifyUpdate()
			}
		},
	}

	root.SetVersionTemplate(fmt.Sprintf("banish {{.Version}} %s/%s\n", stdruntime.GOOS, stdruntime.GOARCH))

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
	root.AddCommand(benchCmd())
	root.AddCommand(upgradeCmd())
	root.AddCommand(uninstallCmd())

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
	inputToks := analyzer.EstimateTokensCharBased(source)

	result, err := interp.EvalSource(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"e\":\"EXEC\",\"m\":%q}\n", err.Error())
		os.Exit(1)
	}

	if result != nil {
		// Measure what actually gets printed (including any JSON envelope)
		// so savings reflect what the agent reads, not an intermediate form.
		printed, show := renderOutput(result, flagHuman || interp.Human())
		outputToks := analyzer.EstimateTokensCharBased(printed)

		var saved int64
		if result.RawTokens > 0 || result.OutTokens > 0 {
			saved = result.RawTokens - outputToks
		}
		rewrites := int64(0)
		if result.Rewritten != "" {
			rewrites = 1
		}
		tracker.Track(analyzer.Entry{
			Timestamp:  time.Now(),
			Command:    source,
			InputToks:  inputToks,
			OutputToks: outputToks,
			RawToks:    result.RawTokens,
			SavedToks:  saved,
			Rewrites:   rewrites,
		})

		if show {
			fmt.Println(printed)
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
	"discover": true, "learn": true, "bench": true, "upgrade": true, "uninstall": true,
	"--human": true, "--verbose": true, "--timeout": true, "--stats": true,
	"-h": true, "--help": true,
	"--version": true, "-v": true,
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

// renderOutput returns exactly what execDirect/run will print for a result,
// and whether anything should be printed at all. Keeping rendering and token
// accounting on the same string means envelope overhead is measured too.
func renderOutput(result *interpreter.Result, human bool) (string, bool) {
	if human {
		return result.String(), true
	}
	if len(result.Meta) > 0 {
		b, _ := result.JSON()
		return string(b), true
	}
	switch v := result.Data.(type) {
	case string:
		return v, v != ""
	case nil:
		return "", false
	default:
		b, _ := json.Marshal(v)
		return string(b), true
	}
}

func newInterpreter() *interpreter.Interpreter {
	reg := interpreter.NewVerbRegistry()
	runtime.RegisterBuiltins(reg)

	// Collect filters and rewrites in precedence order: user extensions
	// first, then project manifest, then embedded defaults. First match
	// wins among equal-length patterns, so user definitions override.
	var scriptFilters []compact.ScriptFilterDef
	var rewrites []compact.RewriteRule

	collectExt := func(loader *extension.Loader) {
		for _, f := range loader.Filters() {
			scriptFilters = append(scriptFilters, compact.ScriptFilterDef{
				Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
			})
		}
		for _, rw := range loader.Rewrites() {
			rewrites = append(rewrites, compact.RewriteRule{
				Name: rw.Name, Match: rw.Match, Unless: rw.Unless, To: rw.To,
				Announce: rw.Announce,
			})
		}
	}

	// Load extensions from ~/.banish/ext/
	home, _ := os.UserHomeDir()
	if home != "" {
		loader := extension.NewLoader()
		loader.LoadDir(filepath.Join(home, ".banish", "ext"))
		loader.Register(reg)
		collectExt(loader)
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
			for _, f := range bf.Filters {
				scriptFilters = append(scriptFilters, compact.ScriptFilterDef{
					Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
				})
			}
			for _, rw := range bf.Rewrites {
				rewrites = append(rewrites, compact.RewriteRule{
					Name: rw.Name, Match: rw.Match, Unless: rw.Unless, To: rw.To,
				})
			}
		}
	}

	// Embedded defaults last: same .bsh DSL, lowest precedence.
	defaults := extension.NewLoader()
	defaults.LoadDefaults()
	collectExt(defaults)

	// System fallback: unknown verbs exec through shell.
	exec := runtime.NewExecutor()
	fallback := runtime.FallbackHandler(exec, scriptFilters, rewrites)
	reg.SetFallback(fallback)
	reg.RegisterBuiltin("__fallback__", fallback)

	opts := []interpreter.Option{
		interpreter.WithRegistry(reg),
		interpreter.WithOutput(os.Stdout),
	}

	return interpreter.New(opts...)
}
