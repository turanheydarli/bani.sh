package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"go.banish.sh/banish/internal/selfupdate"
)

// updateCheckEnabled reports whether background update notices should run. They
// are off when explicitly disabled, and off unless stderr is a terminal, so
// agents, hooks, and CI never see the notice or pay for the network check.
func updateCheckEnabled() bool {
	if os.Getenv("BANISH_NO_UPDATE_CHECK") != "" {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// maybeNotifyUpdate prints a one-line notice to stderr when a newer release is
// available. It relies on a cache refreshed at most once a day and never blocks
// for more than a short timeout.
func maybeNotifyUpdate() {
	if !updateCheckEnabled() {
		return
	}
	tag := selfupdate.LatestCached(2 * time.Second)
	if tag == "" {
		return
	}
	if selfupdate.IsNewer(tag, version) {
		fmt.Fprintf(os.Stderr, "banish: %s is available (you have %s). Run 'banish upgrade'.\n", tag, version)
	}
}

// noticeCommands are the human-facing subcommands after which an update notice
// may print. Machine-facing paths (serve, hook, run, schema, check, bench, and
// the direct-exec proxy path) are excluded so agent output is never touched.
var noticeCommands = map[string]bool{
	"version": true, "status": true, "start": true, "stop": true, "init": true,
}

func noticeAllowed(name string) bool { return noticeCommands[name] }

func upgradeCmd() *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Update banish to the latest release",
		Long: `Download the latest banish release, verify its checksum, and replace
the running binary in place.

  banish upgrade           Install the latest release
  banish upgrade --check   Report whether a newer release exists, without installing

Set BANISH_NO_UPDATE_CHECK=1 to silence the periodic update notice.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			rel, err := selfupdate.Latest(ctx)
			if err != nil {
				return fmt.Errorf("check for updates: %w", err)
			}

			if !selfupdate.IsNewer(rel.Tag, version) {
				fmt.Printf("banish is up to date (%s).\n", version)
				return nil
			}

			if checkOnly {
				fmt.Printf("banish %s is available (you have %s). Run 'banish upgrade' to install.\n", rel.Tag, version)
				return nil
			}

			ver := trimV(rel.Tag)
			fmt.Printf("banish: upgrading %s -> %s\n", version, rel.Tag)
			path, err := selfupdate.Apply(ctx, rel, ver)
			if err != nil {
				return err
			}
			fmt.Printf("banish: upgraded to %s at %s\n", rel.Tag, path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "report whether a newer release exists, without installing")
	return cmd
}

func uninstallCmd() *cobra.Command {
	var purge bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the banish binary",
		Long: `Remove the banish binary from disk.

  banish uninstall           Remove the binary
  banish uninstall --purge   Also remove ~/.banish (extensions, cache, savings data)

Agent wiring written by 'banish init' (hooks and MCP config) is left in place;
remove it from your agent's settings, or run 'banish stop' first to disable the
bash hook.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}

			if !yes {
				fmt.Printf("This will remove %s", exe)
				if purge {
					fmt.Print(" and ~/.banish")
				}
				fmt.Print(". Continue? [y/N] ")
				var reply string
				fmt.Scanln(&reply)
				if reply != "y" && reply != "Y" {
					fmt.Println("banish: uninstall cancelled")
					return nil
				}
			}

			if purge {
				if home, err := os.UserHomeDir(); err == nil && home != "" {
					dir := filepath.Join(home, ".banish")
					if err := os.RemoveAll(dir); err != nil {
						fmt.Fprintf(os.Stderr, "banish: could not remove %s: %v\n", dir, err)
					} else {
						fmt.Printf("banish: removed %s\n", dir)
					}
				}
			}

			if err := os.Remove(exe); err != nil {
				if os.IsPermission(err) {
					return fmt.Errorf("cannot remove %s without elevated permissions - re-run with sudo", exe)
				}
				return err
			}
			fmt.Printf("banish: removed %s\n", exe)
			if runtime.GOOS != "windows" {
				fmt.Println("banish: uninstalled.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "also remove ~/.banish (extensions, cache, savings data)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// trimV drops a leading "v" from a release tag for use in asset names.
func trimV(tag string) string {
	if len(tag) > 0 && (tag[0] == 'v' || tag[0] == 'V') {
		return tag[1:]
	}
	return tag
}
