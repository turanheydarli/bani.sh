package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"go.banish.sh/banish/internal/interpreter"
	"go.banish.sh/banish/internal/rawcache"
)

// bshQuote encodes s as a .bsh double-quoted string. It escapes exactly the
// characters the lexer's readString decodes (" \ newline tab) and passes every
// other byte through literally, so the value the lexer produces is byte-for-byte
// equal to s. (fmt %q cannot be used: it emits \r, \xNN, \uNNNN escapes that the
// lexer does not decode, corrupting values with control or non-UTF-8 bytes.)
func bshQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Server exposes banish verbs as MCP tools via JSON-RPC over stdio.
type Server struct {
	interp *interpreter.Interpreter
}

// NewServer creates an MCP server wrapping the interpreter.
func NewServer(interp *interpreter.Interpreter) *Server {
	return &Server{interp: interp}
}

// Serve runs the MCP server loop, reading JSON-RPC from stdin and writing to stdout.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	writer := os.Stdout

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(writer, 0, -32700, "parse error")
			continue
		}

		resp := s.handleRequest(ctx, &req)
		s.writeResponse(writer, resp)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil // notification, no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	result, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "banish",
			"version": "dev",
		},
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
	})

	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) handleToolsList(req *Request) *Response {
	tools := s.buildToolsList()

	result, _ := json.Marshal(map[string]any{"tools": tools})
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params ToolCallParams
	if req.Params != nil {
		raw, _ := json.Marshal(req.Params)
		json.Unmarshal(raw, &params)
	}

	// Execute via banish interpreter using EvalSource.
	// Strip "banish_" prefix from tool name to get the actual verb.
	verb := strings.TrimPrefix(params.Name, "banish_")

	// banish_raw reads the raw output cache directly -- it must return the
	// cached bytes verbatim, never through the compaction pipeline.
	if verb == "raw" {
		return s.handleRawTool(req, params)
	}

	// Special case: banish_run takes a "script" argument that IS the command.
	var cmd string
	if verb == "run" {
		if script, ok := params.Arguments["script"]; ok {
			cmd = fmt.Sprintf("%v", script)
		}
	} else {
		cmd = verb
		// First positional arg (e.g. "path" for ls/read). Quote it as a .bsh
		// string so values containing the modifier separator ':' (URLs), spaces,
		// or dashes are parsed as a single target, not a key:value modifier or a
		// bash flag.
		for _, key := range []string{"path", "url", "target", "name"} {
			if v, ok := params.Arguments[key]; ok {
				cmd += " " + bshQuote(fmt.Sprint(v))
				delete(params.Arguments, key)
				break
			}
		}
		// Remaining args as modifiers. The value is quoted for the same reason -
		// a modifier value with a space or ':' would otherwise break the parse.
		for k, v := range params.Arguments {
			cmd += fmt.Sprintf(" %s:%s", k, bshQuote(fmt.Sprint(v)))
		}
	}

	r, err := s.interp.EvalSource(cmd)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		}
	}

	var text string
	if r != nil {
		b, _ := r.JSON()
		text = string(b)
	}

	result, _ := json.Marshal(ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	})

	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// handleRawTool serves banish_raw: recover the uncompacted output of a
// recent command by the hash printed in the audit footer.
func (s *Server) handleRawTool(req *Request, params ToolCallParams) *Response {
	hash := fmt.Sprint(params.Arguments["hash"])
	data, err := rawcache.Get(hash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		}
	}
	result, _ := json.Marshal(ToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	})
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) buildToolsList() []ToolDef {
	// Core tools with examples in descriptions (for agent in-context learning).
	tools := []ToolDef{
		{
			Name:        "banish_run",
			Description: "Execute a banish or bash command. Returns structured JSON.\n\nExamples:\n  ls /var/log ext:log\n  echo \"hello\" | count\n  find /tmp -name \"*.txt\"\n  @github issues repo:acme/api",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"script": map[string]string{"type": "string", "description": "Command to execute (banish or bash syntax)"},
				},
				"required": []string{"script"},
			},
		},
		{
			Name:        "banish_ls",
			Description: "List files with structured output. Returns JSON records with name (n), size (s), modified date (t).\n\nExamples:\n  path=/var/log\n  path=/var/log, ext=log\n  path=., ext=go",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]string{"type": "string", "description": "Directory path"},
					"ext":  map[string]string{"type": "string", "description": "Filter by extension"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "banish_read",
			Description: "Read file contents. Returns file content as string.\n\nExamples:\n  path=config.yaml\n  path=/etc/hosts",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]string{"type": "string", "description": "File path to read"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "banish_raw",
			Description: "Recover the uncompacted output of a recent command. Compacted results end with an audit footer naming the hash:\n  recover: banish raw a1b2c3d4\nPass that hash to get the raw stdout+stderr verbatim. Use only when the compacted output is missing something you actually need -- the footer's group labels say what kind of lines were dropped (warnings, passing tests, overflow), and recovering re-reads the full raw output. Entries expire (default 1 hour).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hash": map[string]string{"type": "string", "description": "Recover hash from the audit footer"},
				},
				"required": []string{"hash"},
			},
		},
		{
			Name:        "banish_fetch",
			Description: "HTTP request. Returns status, body, headers as JSON.\n\nExamples:\n  url=https://api.example.com\n  url=https://api.example.com, method=POST",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":    map[string]string{"type": "string", "description": "URL to fetch"},
					"method": map[string]string{"type": "string", "description": "HTTP method (default GET)"},
				},
				"required": []string{"url"},
			},
		},
	}

	// Auto-expose extension verbs (from ~/.banish/ext/ and BANISH manifest)
	// as MCP tools. Each verb becomes banish_<name> with a generic "args" input.
	reg := s.interp.Registry()
	for _, name := range reg.ExtensionNames() {
		toolName := "banish_" + name
		// Skip if it would collide with a core tool
		exists := false
		for _, t := range tools {
			if t.Name == toolName {
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		tools = append(tools, ToolDef{
			Name:        toolName,
			Description: fmt.Sprintf("Run the '%s' verb (project/extension defined).", name),
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]string{
						"type":        "string",
						"description": "Arguments to pass to the verb",
					},
				},
			},
		})
	}

	return tools
}

func (s *Server) writeResponse(w io.Writer, resp *Response) {
	if resp == nil {
		return
	}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	w.Write(data)
}

func (s *Server) writeError(w io.Writer, id int, code int, msg string) {
	resp := &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
	s.writeResponse(w, resp)
}
