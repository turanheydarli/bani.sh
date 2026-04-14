package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Transport is the interface for communicating with an MCP server.
type Transport interface {
	Send(ctx context.Context, req *Request) (*Response, error)
	Close() error
}

// StdioTransport spawns a process and communicates via stdin/stdout JSON-RPC.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
}

// NewStdioTransport creates a transport by spawning the given command.
func NewStdioTransport(ctx context.Context, command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("mcp stdio: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("mcp stdio: start %s: %w", command, err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
	}, nil
}

// Send sends a JSON-RPC request and reads the response.
func (t *StdioTransport) Send(_ context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: marshal request: %w", err)
	}

	data = append(data, '\n')
	if _, err := t.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("mcp stdio: write: %w", err)
	}

	if !t.stdout.Scan() {
		if err := t.stdout.Err(); err != nil {
			return nil, fmt.Errorf("mcp stdio: read: %w", err)
		}
		return nil, fmt.Errorf("mcp stdio: server closed connection")
	}

	var resp Response
	if err := json.Unmarshal(t.stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("mcp stdio: unmarshal response: %w", err)
	}

	return &resp, nil
}

// Close terminates the subprocess.
func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}
