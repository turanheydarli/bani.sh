// Package mcp implements the MCP JSON-RPC client and server. The client
// calls external MCP servers. The server exposes banish verbs as MCP tools.
package mcp

import (
	"encoding/json"
	"fmt"
	"sync"
)

// JSON-RPC message types for MCP protocol.

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

// ToolDef describes a tool exposed by an MCP server.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolCallParams are the parameters for a tools/call request.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResult is the result of a tools/call response.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of content in a tool result.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ServerConfig holds configuration for connecting to an MCP server.
type ServerConfig struct {
	Name      string
	Transport string // "stdio" or "http"
	Command   string // for stdio: the command to spawn
	Args      []string
	Endpoint  string // for http: the URL
	Auth      string
	Env       []string
}

// nextID generates unique request IDs.
var (
	idMu    sync.Mutex
	idCount int
)

func nextID() int {
	idMu.Lock()
	defer idMu.Unlock()
	idCount++
	return idCount
}
