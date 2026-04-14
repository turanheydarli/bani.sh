package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Client manages connections to MCP servers.
type Client struct {
	configs    map[string]ServerConfig
	transports map[string]Transport
	mu         sync.Mutex
}

// NewClient creates an MCP client.
func NewClient() *Client {
	return &Client{
		configs:    make(map[string]ServerConfig),
		transports: make(map[string]Transport),
	}
}

// AddServer registers a server configuration. Connection is lazy.
func (c *Client) AddServer(cfg ServerConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs[cfg.Name] = cfg
}

// Call invokes a tool on a named server.
func (c *Client) Call(ctx context.Context, server string, tool string, args map[string]any) (*ToolResult, error) {
	t, err := c.getTransport(ctx, server)
	if err != nil {
		return nil, err
	}

	req := &Request{
		JSONRPC: "2.0",
		ID:      nextID(),
		Method:  "tools/call",
		Params: ToolCallParams{
			Name:      tool,
			Arguments: args,
		},
	}

	resp, err := t.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mcp call %s/%s: %w", server, tool, err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp call %s/%s: unmarshal result: %w", server, tool, err)
	}

	return &result, nil
}

// ListTools retrieves available tools from a server.
func (c *Client) ListTools(ctx context.Context, server string) ([]ToolDef, error) {
	t, err := c.getTransport(ctx, server)
	if err != nil {
		return nil, err
	}

	req := &Request{
		JSONRPC: "2.0",
		ID:      nextID(),
		Method:  "tools/list",
	}

	resp, err := t.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mcp list %s: %w", server, err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp list %s: unmarshal: %w", server, err)
	}

	return result.Tools, nil
}

// Close closes all active transports.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, t := range c.transports {
		t.Close()
		delete(c.transports, name)
	}
}

// getTransport lazily connects to a server.
func (c *Client) getTransport(ctx context.Context, server string) (Transport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if t, ok := c.transports[server]; ok {
		return t, nil
	}

	cfg, ok := c.configs[server]
	if !ok {
		return nil, fmt.Errorf("mcp: unknown server %q", server)
	}

	var t Transport
	var err error

	switch cfg.Transport {
	case "stdio", "":
		t, err = NewStdioTransport(ctx, cfg.Command, cfg.Args, cfg.Env)
	default:
		return nil, fmt.Errorf("mcp: unsupported transport %q for %s", cfg.Transport, server)
	}

	if err != nil {
		return nil, err
	}

	// Initialize the connection
	initReq := &Request{
		JSONRPC: "2.0",
		ID:      nextID(),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]string{
				"name":    "banish",
				"version": "dev",
			},
			"capabilities": map[string]any{},
		},
	}

	if _, err := t.Send(ctx, initReq); err != nil {
		t.Close()
		return nil, fmt.Errorf("mcp init %s: %w", server, err)
	}

	c.transports[server] = t
	return t, nil
}
