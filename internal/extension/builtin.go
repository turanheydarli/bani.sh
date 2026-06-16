package extension

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed builtin/*.bsh
var builtinFS embed.FS

// Builtin returns the default .bsh extensions shipped with banish, keyed by
// filename (for example "git.bsh"). They are embedded in the binary and
// deployed to ~/.banish/ext/ by banish init.
func Builtin() (map[string]string, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("extension.Builtin: %w", err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(builtinFS, "builtin/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("extension.Builtin: read %s: %w", e.Name(), err)
		}
		out[e.Name()] = string(data)
	}
	return out, nil
}
