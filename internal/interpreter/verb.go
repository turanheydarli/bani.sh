package interpreter

import (
	"context"
	"fmt"

	"go.bani.sh/banish/internal/ast"
)

// VerbHandler is the function signature for verb implementations.
type VerbHandler func(ctx context.Context, cmd *ast.Command, input *Result) (*Result, error)

// VerbRegistry holds named verb handlers with priority-ordered resolution.
type VerbRegistry struct {
	builtins   map[string]VerbHandler
	extensions map[string]VerbHandler
	mcp        map[string]VerbHandler
	fallback   VerbHandler
}

// NewVerbRegistry creates an empty registry.
func NewVerbRegistry() *VerbRegistry {
	return &VerbRegistry{
		builtins:   make(map[string]VerbHandler),
		extensions: make(map[string]VerbHandler),
		mcp:        make(map[string]VerbHandler),
	}
}

// RegisterBuiltin adds a builtin verb.
func (r *VerbRegistry) RegisterBuiltin(name string, h VerbHandler) {
	r.builtins[name] = h
}

// RegisterExtension adds an extension verb.
func (r *VerbRegistry) RegisterExtension(name string, h VerbHandler) {
	r.extensions[name] = h
}

// RegisterMCP adds an MCP-mapped verb.
func (r *VerbRegistry) RegisterMCP(name string, h VerbHandler) {
	r.mcp[name] = h
}

// SetFallback sets the system fallback handler for unresolved verbs.
func (r *VerbRegistry) SetFallback(h VerbHandler) {
	r.fallback = h
}

// ExtensionNames returns the names of all registered extension verbs.
func (r *VerbRegistry) ExtensionNames() []string {
	names := make([]string, 0, len(r.extensions))
	for name := range r.extensions {
		names = append(names, name)
	}
	return names
}

// BuiltinNames returns the names of all registered builtin verbs.
func (r *VerbRegistry) BuiltinNames() []string {
	names := make([]string, 0, len(r.builtins))
	for name := range r.builtins {
		if name == "__fallback__" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// Resolve looks up a verb by name in priority order:
// builtins -> extensions -> MCP -> fallback.
func (r *VerbRegistry) Resolve(name string) (VerbHandler, error) {
	if h, ok := r.builtins[name]; ok {
		return h, nil
	}
	if h, ok := r.extensions[name]; ok {
		return h, nil
	}
	if h, ok := r.mcp[name]; ok {
		return h, nil
	}
	if r.fallback != nil {
		return r.fallback, nil
	}
	return nil, fmt.Errorf("unknown verb: %s", name)
}
