package mcp

import (
	"encoding/json"
	"testing"
)

func TestRequestMarshal(t *testing.T) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: ToolCallParams{
			Name:      "list_issues",
			Arguments: map[string]any{"repo": "acme/api", "state": "open"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["method"] != "tools/call" {
		t.Errorf("method = %v, want tools/call", parsed["method"])
	}

	params, ok := parsed["params"].(map[string]any)
	if !ok {
		t.Fatal("params is not a map")
	}
	if params["name"] != "list_issues" {
		t.Errorf("params.name = %v, want list_issues", params["name"])
	}
}

func TestResponseUnmarshal(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"hello"}]}}`

	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("id = %d, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result ToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("content text = %q, want hello", result.Content[0].Text)
	}
}

func TestResponseError(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`

	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
	if resp.Error.Message != "method not found" {
		t.Errorf("error msg = %q, want method not found", resp.Error.Message)
	}
}

func TestClientAddServer(t *testing.T) {
	c := NewClient()
	c.AddServer(ServerConfig{
		Name:      "test",
		Transport: "stdio",
		Command:   "echo",
	})

	if _, ok := c.configs["test"]; !ok {
		t.Error("server not registered")
	}
}

func TestToolCallParams(t *testing.T) {
	params := ToolCallParams{
		Name: "list_issues",
		Arguments: map[string]any{
			"repo":  "acme/api",
			"state": "open",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)

	if parsed["name"] != "list_issues" {
		t.Errorf("name = %v, want list_issues", parsed["name"])
	}

	args := parsed["arguments"].(map[string]any)
	if args["repo"] != "acme/api" {
		t.Errorf("repo = %v, want acme/api", args["repo"])
	}
}

func TestRPCErrorString(t *testing.T) {
	e := &RPCError{Code: -32601, Message: "method not found"}
	want := "mcp error -32601: method not found"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}
