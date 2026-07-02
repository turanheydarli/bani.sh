// Package shell selects the OS-appropriate shell for running command strings.
//
// banish wraps arbitrary command strings (pipes, redirects, quoting) and runs
// .bsh filters as shell one-liners, so it needs a shell to interpret them.
// On Unix that is "sh -c". On Windows there is no "sh", so this prefers a
// Unix-style shell if one is installed (Git for Windows, MSYS2, or WSL provide
// bash plus the coreutils the filters use) and falls back to "cmd /c".
//
// With no Unix shell on Windows, native commands still run under cmd; filters
// that call grep/sed/awk fail, and banish's fallback returns the raw output.
package shell

import (
	"os/exec"
	"runtime"
	"sync"
)

var (
	winShellOnce sync.Once
	winShellName string
	winShellArgs []string
)

// Args returns the shell executable and the arguments to run a command string.
func Args(script string) (string, []string) {
	if runtime.GOOS != "windows" {
		return "sh", []string{"-c", script}
	}
	name, flag := windowsShell()
	return name, append(append([]string{}, flag...), script)
}

// windowsShell resolves (once) the shell to use on Windows: a Unix-style bash/sh
// if present on PATH, otherwise cmd. Returned flag is the pre-script argument(s).
func windowsShell() (string, []string) {
	winShellOnce.Do(func() {
		for _, name := range []string{"bash", "sh"} {
			if path, err := exec.LookPath(name); err == nil {
				winShellName, winShellArgs = path, []string{"-c"}
				return
			}
		}
		winShellName, winShellArgs = "cmd", []string{"/c"}
	})
	return winShellName, winShellArgs
}
