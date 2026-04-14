package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecBasicCommand(t *testing.T) {
	e := NewExecutor()
	r, err := e.Run(context.Background(), "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	if strings.TrimSpace(string(r.Stdout)) != "hello" {
		t.Errorf("stdout = %q, want hello", r.Stdout)
	}
	if r.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", r.ExitCode)
	}
}

func TestExecNonZeroExit(t *testing.T) {
	e := NewExecutor()
	r, err := e.Run(context.Background(), "sh", []string{"-c", "exit 42"})
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	if r.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", r.ExitCode)
	}
}

func TestExecStderr(t *testing.T) {
	e := NewExecutor()
	r, err := e.Run(context.Background(), "sh", []string{"-c", "echo oops >&2"})
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	if !strings.Contains(string(r.Stderr), "oops") {
		t.Errorf("stderr = %q, want to contain oops", r.Stderr)
	}
}

func TestExecTimeout(t *testing.T) {
	e := NewExecutor(WithTimeout(100 * time.Millisecond))
	_, err := e.Run(context.Background(), "sleep", []string{"10"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q, want timeout message", err)
	}
}

func TestExecContextCancel(t *testing.T) {
	e := NewExecutor(WithTimeout(0))
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := e.Run(ctx, "sleep", []string{"10"})
	if err == nil {
		t.Fatal("expected cancel error")
	}
}

func TestExecShell(t *testing.T) {
	e := NewExecutor()
	r, err := e.RunShell(context.Background(), "echo hello && echo world")
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	out := strings.TrimSpace(string(r.Stdout))
	if out != "hello\nworld" {
		t.Errorf("stdout = %q, want hello\\nworld", out)
	}
}

func TestExecWithDir(t *testing.T) {
	e := NewExecutor(WithDir("/tmp"))
	r, err := e.Run(context.Background(), "pwd", nil)
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	dir := strings.TrimSpace(string(r.Stdout))
	if !strings.Contains(dir, "tmp") {
		t.Errorf("pwd = %q, want /tmp", dir)
	}
}
