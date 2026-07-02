package shell

import (
	"runtime"
	"testing"
)

func TestArgs(t *testing.T) {
	name, args := Args("git status")
	if runtime.GOOS == "windows" {
		// bash/sh -c ... or cmd /c ...; the script is always the last arg.
		if len(args) < 2 || args[len(args)-1] != "git status" {
			t.Fatalf("windows Args = %s %v, want the script as the last arg", name, args)
		}
	} else {
		if name != "sh" || len(args) != 2 || args[0] != "-c" || args[1] != "git status" {
			t.Fatalf("Args = %s %v, want sh -c \"git status\"", name, args)
		}
	}
}
