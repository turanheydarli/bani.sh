package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.banish.sh/banish/internal/shell"
)

// ExecResult holds the output of a subprocess execution.
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExecOption configures the executor.
type ExecOption func(*Executor)

// Executor runs OS commands as subprocesses.
type Executor struct {
	timeout time.Duration
	dir     string
	env     []string
}

// WithTimeout sets the default command timeout.
func WithTimeout(d time.Duration) ExecOption {
	return func(e *Executor) { e.timeout = d }
}

// WithDir sets the working directory for commands.
func WithDir(dir string) ExecOption {
	return func(e *Executor) { e.dir = dir }
}

// WithEnv sets environment variables for commands.
func WithEnv(env []string) ExecOption {
	return func(e *Executor) { e.env = env }
}

// NewExecutor creates a subprocess executor.
func NewExecutor(opts ...ExecOption) *Executor {
	e := &Executor{
		timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes a single command and captures its output.
func (e *Executor) Run(ctx context.Context, name string, args []string) (*ExecResult, error) {
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	if e.dir != "" {
		cmd.Dir = e.dir
	}
	if e.env != nil {
		cmd.Env = e.env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &ExecResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}

	if err != nil {
		// Check context first: if context was canceled or timed out,
		// the process was killed and the exit error is just a signal.
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command timed out after %v", e.timeout)
		}
		if ctx.Err() == context.Canceled {
			return result, fmt.Errorf("command canceled")
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("exec %s: %w", name, err)
	}

	return result, nil
}

// RunShell executes a command string through the OS-appropriate shell.
func (e *Executor) RunShell(ctx context.Context, command string) (*ExecResult, error) {
	name, args := shell.Args(command)
	return e.Run(ctx, name, args)
}
