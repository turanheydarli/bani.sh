package parser

import (
	"testing"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/lexer"
)

func parse(t *testing.T, input string) *ast.Program {
	t.Helper()
	l := lexer.New(input)
	p := New(l)
	prog := p.ParseProgram()
	for _, err := range p.Errors() {
		t.Errorf("parse error: %s", err)
	}
	return prog
}

func TestSimpleCommand(t *testing.T) {
	prog := parse(t, "ls /var/log")

	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	id, ok := cmd.Verb.(*ast.Identifier)
	if !ok || id.Value != "ls" {
		t.Fatalf("verb = %v, want ls", cmd.Verb)
	}

	path, ok := cmd.Target.(*ast.PathLiteral)
	if !ok || path.Value != "/var/log" {
		t.Fatalf("target = %v, want /var/log", cmd.Target)
	}
}

func TestCommandWithModifiers(t *testing.T) {
	prog := parse(t, "ls /var/log ext:log size:>100m")

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	if len(cmd.Modifiers) != 2 {
		t.Fatalf("expected 2 modifiers, got %d", len(cmd.Modifiers))
	}

	if cmd.Modifiers[0].Key != "ext" || cmd.Modifiers[0].Value != "log" {
		t.Errorf("modifier[0] = %s:%s, want ext:log", cmd.Modifiers[0].Key, cmd.Modifiers[0].Value)
	}

	if cmd.Modifiers[1].Key != "size" || cmd.Modifiers[1].Value != ">100m" {
		t.Errorf("modifier[1] = %s:%s, want size:>100m", cmd.Modifiers[1].Key, cmd.Modifiers[1].Value)
	}
}

func TestPipeline(t *testing.T) {
	prog := parse(t, "ls /var/log | gz")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(pipe.Commands))
	}

	if pipe.Commands[0].Verb.String() != "ls" {
		t.Errorf("cmd[0] verb = %s, want ls", pipe.Commands[0].Verb)
	}
	if pipe.Commands[1].Verb.String() != "gz" {
		t.Errorf("cmd[1] verb = %s, want gz", pipe.Commands[1].Verb)
	}
}

func TestAssignment(t *testing.T) {
	prog := parse(t, "$issues = @github issues repo:acme/api state:open")

	a, ok := prog.Statements[0].(*ast.Assignment)
	if !ok {
		t.Fatalf("expected Assignment, got %T", prog.Statements[0])
	}

	if a.Name != "issues" {
		t.Errorf("name = %s, want issues", a.Name)
	}

	cmd, ok := a.Value.(*ast.Command)
	if !ok {
		t.Fatalf("value: expected Command, got %T", a.Value)
	}

	mcp, ok := cmd.Verb.(*ast.MCPCall)
	if !ok {
		t.Fatalf("verb: expected MCPCall, got %T", cmd.Verb)
	}
	if mcp.Server != "github" || mcp.Verb != "issues" {
		t.Errorf("mcp = @%s %s, want @github issues", mcp.Server, mcp.Verb)
	}

	if len(cmd.Modifiers) != 2 {
		t.Fatalf("expected 2 modifiers, got %d", len(cmd.Modifiers))
	}
}

func TestDirective(t *testing.T) {
	prog := parse(t, "!human")

	d, ok := prog.Statements[0].(*ast.Directive)
	if !ok {
		t.Fatalf("expected Directive, got %T", prog.Statements[0])
	}
	if d.Name != "human" {
		t.Errorf("name = %s, want human", d.Name)
	}
}

func TestDirectiveWithArgs(t *testing.T) {
	prog := parse(t, "!extension deploy v:1.0")

	d, ok := prog.Statements[0].(*ast.Directive)
	if !ok {
		t.Fatalf("expected Directive, got %T", prog.Statements[0])
	}
	if d.Name != "extension" {
		t.Errorf("name = %s, want extension", d.Name)
	}
	if len(d.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(d.Args))
	}
}

func TestMCPCall(t *testing.T) {
	prog := parse(t, "@github issues repo:acme/api")

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	mcp, ok := cmd.Verb.(*ast.MCPCall)
	if !ok {
		t.Fatalf("verb: expected MCPCall, got %T", cmd.Verb)
	}
	if mcp.Server != "github" || mcp.Verb != "issues" {
		t.Errorf("mcp = @%s %s, want @github issues", mcp.Server, mcp.Verb)
	}
}

func TestParallel(t *testing.T) {
	prog := parse(t, "@db ping & @redis ping")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(pipe.Commands))
	}
	if len(pipe.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(pipe.Ops))
	}
}

func TestConditionals(t *testing.T) {
	prog := parse(t, "cmd1 && cmd2 || cmd3")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(pipe.Commands))
	}
}

func TestRedirectOut(t *testing.T) {
	prog := parse(t, "ls /tmp -> out.txt")

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	if cmd.Redirect == nil {
		t.Fatal("expected redirect")
	}
	if cmd.Redirect.Path != "out.txt" {
		t.Errorf("redirect path = %s, want out.txt", cmd.Redirect.Path)
	}
}

func TestRedirectIn(t *testing.T) {
	prog := parse(t, "sort <- data.txt")

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	if cmd.Redirect == nil {
		t.Fatal("expected redirect")
	}
	if cmd.Redirect.Path != "data.txt" {
		t.Errorf("redirect path = %s, want data.txt", cmd.Redirect.Path)
	}
}

func TestVariableRefAsPipelineStart(t *testing.T) {
	prog := parse(t, "$issues ? priority:high")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(pipe.Commands))
	}

	// First command verb should be $issues
	v, ok := pipe.Commands[0].Verb.(*ast.VariableRef)
	if !ok {
		t.Fatalf("cmd[0] verb: expected VariableRef, got %T", pipe.Commands[0].Verb)
	}
	if v.Name != "issues" {
		t.Errorf("cmd[0] verb name = %s, want issues", v.Name)
	}
}

func TestMultilineProgram(t *testing.T) {
	input := `ls /var/log ext:log size:>100m age:>7d | gz
@k8s pods ns:prod & @db ping
deploy staging wait:healthy`

	prog := parse(t, input)

	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
}

// TestExampleFindLogs: ls /var/log ext:log size:>100m age:>7d | gz
func TestExampleFindLogs(t *testing.T) {
	prog := parse(t, "ls /var/log ext:log size:>100m age:>7d | gz")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(pipe.Commands))
	}

	ls := pipe.Commands[0]
	if ls.Verb.String() != "ls" {
		t.Errorf("cmd[0] verb = %s, want ls", ls.Verb)
	}
	if ls.Target == nil || ls.Target.String() != "/var/log" {
		t.Errorf("cmd[0] target = %v, want /var/log", ls.Target)
	}
	if len(ls.Modifiers) != 3 {
		t.Fatalf("expected 3 modifiers, got %d", len(ls.Modifiers))
	}
}

// TestExampleParallelHealth: @k8s pods ns:prod status:running & @db ping & @redis ping ; report
func TestExampleParallelHealth(t *testing.T) {
	prog := parse(t, "@k8s pods ns:prod status:running & @db ping & @redis ping ; report")

	pipe, ok := prog.Statements[0].(*ast.Pipeline)
	if !ok {
		t.Fatalf("expected Pipeline, got %T", prog.Statements[0])
	}

	if len(pipe.Commands) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(pipe.Commands))
	}
}

// TestExampleDeploy: deploy staging wait:healthy rollback:auto
func TestExampleDeploy(t *testing.T) {
	prog := parse(t, "deploy staging wait:healthy rollback:auto")

	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		t.Fatalf("expected Command, got %T", prog.Statements[0])
	}

	if cmd.Verb.String() != "deploy" {
		t.Errorf("verb = %s, want deploy", cmd.Verb)
	}

	if cmd.Target == nil || cmd.Target.String() != "staging" {
		t.Errorf("target = %v, want staging", cmd.Target)
	}

	if len(cmd.Modifiers) != 2 {
		t.Fatalf("expected 2 modifiers, got %d", len(cmd.Modifiers))
	}
}

func TestStringRoundtrip(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ls /var/log", "ls /var/log"},
		{"!human", "!human"},
		{"ls /tmp -> out.txt", "ls /tmp -> out.txt"},
		{"@db ping & @redis ping", "@db ping & @redis ping"},
	}

	for _, tt := range tests {
		prog := parse(t, tt.input)
		got := prog.String()
		if got != tt.want {
			t.Errorf("input %q: String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		input     string
		wantError bool
	}{
		{"$", true},         // missing name
		{"$ = ls", true},    // missing name
		{"!", true},         // missing directive name
		{"@", true},         // missing server name
		{"@github", true},   // missing verb after server
	}

	for _, tt := range tests {
		l := lexer.New(tt.input)
		p := New(l)
		p.ParseProgram()
		if tt.wantError && len(p.Errors()) == 0 {
			t.Errorf("input %q: expected error, got none", tt.input)
		}
	}
}

func TestErrorRecovery(t *testing.T) {
	// @ without server name is a parse error. Parser should recover
	// and still parse the second line.
	input := "@\nls /var/log"

	l := lexer.New(input)
	p := New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) == 0 {
		t.Error("expected parse error for first line")
	}

	// Should still parse the second line
	found := false
	for _, s := range prog.Statements {
		if cmd, ok := s.(*ast.Command); ok {
			if cmd.Verb.String() == "ls" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected ls command from second line after error recovery")
	}
}
