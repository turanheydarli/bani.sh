package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/interpreter"
)

// RegisterBuiltins registers all core verbs into the registry.
func RegisterBuiltins(reg *interpreter.VerbRegistry) {
	reg.RegisterBuiltin("echo", bEcho)
	reg.RegisterBuiltin("ls", bLs)
	reg.RegisterBuiltin("read", bRead)
	reg.RegisterBuiltin("cat", bRead) // alias
	reg.RegisterBuiltin("write", bWrite)
	reg.RegisterBuiltin("mkdir", bMkdir)
	reg.RegisterBuiltin("rm", bRm)
	reg.RegisterBuiltin("cp", bCp)
	reg.RegisterBuiltin("mv", bMv)
	reg.RegisterBuiltin("head", bHead)
	reg.RegisterBuiltin("tail", bTail)
	reg.RegisterBuiltin("env", bEnv)
	reg.RegisterBuiltin("sleep", bSleep)
	reg.RegisterBuiltin("count", bCount)
	reg.RegisterBuiltin("fetch", bFetch)
}

// modVal returns the value of a modifier by key, or empty string.
func modVal(cmd *ast.Command, key string) string {
	for _, m := range cmd.Modifiers {
		if m.Key == key {
			return m.Value
		}
	}
	return ""
}

// target returns the target's raw value, or empty. It unwraps literal nodes so
// callers get the underlying value (e.g. a URL) rather than the re-serialized
// form: StringLiteral.String() re-adds surrounding quotes, which would corrupt a
// quoted argument like "https://example.com".
func target(cmd *ast.Command) string {
	if cmd.Target == nil {
		return ""
	}
	switch t := cmd.Target.(type) {
	case *ast.StringLiteral:
		return t.Value
	case *ast.Identifier:
		return t.Value
	case *ast.NumberLiteral:
		return t.Value
	default:
		return cmd.Target.String()
	}
}

// =========================================================================
// echo

func bEcho(_ context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
	if cmd.Target != nil {
		return interpreter.NewResult(cmd.Target.String()), nil
	}
	if input != nil {
		return input, nil
	}
	return interpreter.NewResult(""), nil
}

// =========================================================================
// ls

type fileEntry struct {
	Name string `json:"n"`
	Size int64  `json:"s"`
	Dir  bool   `json:"d,omitempty"`
	Mod  string `json:"t"`
}

func bLs(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	dir := target(cmd)
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ls: %w", err)
	}

	ext := modVal(cmd, "ext")

	var files []fileEntry
	for _, e := range entries {
		if ext != "" {
			fileExt := strings.TrimPrefix(filepath.Ext(e.Name()), ".")
			if fileExt != ext {
				continue
			}
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			Name: e.Name(),
			Size: info.Size(),
			Dir:  e.IsDir(),
			Mod:  info.ModTime().Format(time.DateOnly),
		})
	}

	return interpreter.NewResult(files), nil
}

// =========================================================================
// read / cat

func bRead(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	path := target(cmd)
	if path == "" {
		return nil, fmt.Errorf("read: no file specified")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return interpreter.NewResult(string(data)), nil
}

// =========================================================================
// write

func bWrite(_ context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
	path := target(cmd)
	if path == "" {
		return nil, fmt.Errorf("write: no file specified")
	}

	var content string
	if input != nil {
		content = fmt.Sprintf("%v", input.Data)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	return interpreter.NewResult(map[string]any{"n": path, "s": len(content)}), nil
}

// =========================================================================
// mkdir

func bMkdir(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	path := target(cmd)
	if path == "" {
		return nil, fmt.Errorf("mkdir: no path specified")
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	return interpreter.NewResult(map[string]any{"n": path}), nil
}

// =========================================================================
// rm

func bRm(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	path := target(cmd)
	if path == "" {
		return nil, fmt.Errorf("rm: no path specified")
	}
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("rm: %w", err)
	}
	return interpreter.NewResult(map[string]any{"n": path}), nil
}

// =========================================================================
// cp

func bCp(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	src := target(cmd)
	dst := modVal(cmd, "to")
	if src == "" || dst == "" {
		return nil, fmt.Errorf("cp: usage: cp <src> to:<dst>")
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("cp: %w", err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return nil, fmt.Errorf("cp: %w", err)
	}

	return interpreter.NewResult(map[string]any{"from": src, "to": dst, "s": len(data)}), nil
}

// =========================================================================
// mv

func bMv(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	src := target(cmd)
	dst := modVal(cmd, "to")
	if src == "" || dst == "" {
		return nil, fmt.Errorf("mv: usage: mv <src> to:<dst>")
	}

	if err := os.Rename(src, dst); err != nil {
		return nil, fmt.Errorf("mv: %w", err)
	}

	return interpreter.NewResult(map[string]any{"from": src, "to": dst}), nil
}

// =========================================================================
// head

func bHead(_ context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
	n := 10
	if v := modVal(cmd, "n"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}

	var text string
	if input != nil {
		text = fmt.Sprintf("%v", input.Data)
	} else if t := target(cmd); t != "" {
		data, err := os.ReadFile(t)
		if err != nil {
			return nil, fmt.Errorf("head: %w", err)
		}
		text = string(data)
	}

	lines := strings.SplitN(text, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}

	return interpreter.NewResult(strings.Join(lines, "\n")), nil
}

// =========================================================================
// tail

func bTail(_ context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
	n := 10
	if v := modVal(cmd, "n"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}

	var text string
	if input != nil {
		text = fmt.Sprintf("%v", input.Data)
	} else if t := target(cmd); t != "" {
		data, err := os.ReadFile(t)
		if err != nil {
			return nil, fmt.Errorf("tail: %w", err)
		}
		text = string(data)
	}

	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return interpreter.NewResult(strings.Join(lines, "\n")), nil
}

// =========================================================================
// env

func bEnv(_ context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	name := target(cmd)
	if name == "" {
		return nil, fmt.Errorf("env: no variable name specified")
	}
	val, ok := os.LookupEnv(name)
	if !ok {
		return interpreter.NewResult(nil), nil
	}
	return interpreter.NewResult(val), nil
}

// =========================================================================
// sleep

func bSleep(ctx context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	var secs float64
	t := target(cmd)
	if t == "" {
		return nil, fmt.Errorf("sleep: no duration specified")
	}
	fmt.Sscanf(t, "%f", &secs)

	select {
	case <-time.After(time.Duration(secs * float64(time.Second))):
		return interpreter.NewResult(map[string]any{"slept": secs}), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// =========================================================================
// count

func bCount(_ context.Context, _ *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
	if input == nil {
		return interpreter.NewResult(0), nil
	}

	switch v := input.Data.(type) {
	case string:
		lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
		return interpreter.NewResult(len(lines)), nil
	case []any:
		return interpreter.NewResult(len(v)), nil
	case []fileEntry:
		return interpreter.NewResult(len(v)), nil
	default:
		return interpreter.NewResult(1), nil
	}
}

// =========================================================================
// fetch

func bFetch(ctx context.Context, cmd *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
	url := target(cmd)
	if url == "" {
		return nil, fmt.Errorf("fetch: no URL specified")
	}

	method := strings.ToUpper(modVal(cmd, "method"))
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch: reading body: %w", err)
	}

	result := map[string]any{
		"status": resp.StatusCode,
		"body":   string(body),
	}

	// Sort and include response headers
	headers := make(map[string]string)
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		headers[strings.ToLower(k)] = resp.Header.Get(k)
	}
	result["headers"] = headers

	return interpreter.NewResult(result), nil
}
