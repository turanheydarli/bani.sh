package lexer

import (
	"testing"

	"go.bani.sh/banish/internal/token"
)

func TestSingleCharTokens(t *testing.T) {
	input := "| ; ? @ $ ! = :"

	tests := []struct {
		wantType    token.TokenType
		wantLiteral string
	}{
		{token.Pipe, "|"},
		{token.Semicolon, ";"},
		{token.Question, "?"},
		{token.At, "@"},
		{token.Dollar, "$"},
		{token.Bang, "!"},
		{token.Equals, "="},
		{token.Colon, ":"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.wantType {
			t.Fatalf("tests[%d] - type wrong. want=%v, got=%v (%q)", i, tt.wantType, tok.Type, tok.Literal)
		}
		if tok.Literal != tt.wantLiteral {
			t.Fatalf("tests[%d] - literal wrong. want=%q, got=%q", i, tt.wantLiteral, tok.Literal)
		}
	}
}

func TestTwoCharOperators(t *testing.T) {
	input := "&& || -> <-"

	tests := []struct {
		wantType    token.TokenType
		wantLiteral string
	}{
		{token.And, "&&"},
		{token.Or, "||"},
		{token.ArrowR, "->"},
		{token.ArrowL, "<-"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.wantType {
			t.Fatalf("tests[%d] - type wrong. want=%v, got=%v (%q)", i, tt.wantType, tok.Type, tok.Literal)
		}
		if tok.Literal != tt.wantLiteral {
			t.Fatalf("tests[%d] - literal wrong. want=%q, got=%q", i, tt.wantLiteral, tok.Literal)
		}
	}
}

func TestAmpersandVsAnd(t *testing.T) {
	input := "& &&"

	l := New(input)
	tok1 := l.NextToken()
	if tok1.Type != token.Ampersand {
		t.Fatalf("first token: want AMPERSAND, got %v", tok1.Type)
	}
	tok2 := l.NextToken()
	if tok2.Type != token.And {
		t.Fatalf("second token: want AND, got %v", tok2.Type)
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`"hello world"`, "hello world"},
		{`"say \"hi\""`, `say "hi"`},
		{`"line1\nline2"`, "line1\nline2"},
		{`"tab\there"`, "tab\there"},
		{`"back\\slash"`, "back\\slash"},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != token.String {
			t.Errorf("input %s: type = %v, want STRING", tt.input, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("input %s: literal = %q, want %q", tt.input, tok.Literal, tt.want)
		}
	}
}

func TestNumbers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "42"},
		{"3.14", "3.14"},
		{"100", "100"},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != token.Number {
			t.Errorf("input %s: type = %v, want NUMBER", tt.input, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("input %s: literal = %q, want %q", tt.input, tok.Literal, tt.want)
		}
	}
}

func TestIdentifiers(t *testing.T) {
	input := "ls read write fetch gz deploy git-flow"

	wants := []string{"ls", "read", "write", "fetch", "gz", "deploy", "git-flow"}

	l := New(input)
	for i, want := range wants {
		tok := l.NextToken()
		if tok.Type != token.Ident {
			t.Fatalf("tests[%d] - type wrong. want=IDENT, got=%v (%q)", i, tok.Type, tok.Literal)
		}
		if tok.Literal != want {
			t.Fatalf("tests[%d] - literal wrong. want=%q, got=%q", i, want, tok.Literal)
		}
	}
}

func TestPaths(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/var/log", "/var/log"},
		{"./src", "./src"},
		{"~/config", "~/config"},
		{"/usr/local/bin/banish", "/usr/local/bin/banish"},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != token.Path {
			t.Errorf("input %s: type = %v, want PATH", tt.input, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("input %s: literal = %q, want %q", tt.input, tok.Literal, tt.want)
		}
	}
}

func TestGlobs(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"*.log", "*.log"},
		{"*.txt", "*.txt"},
	}

	for _, tt := range tests {
		l := New(tt.input)
		tok := l.NextToken()
		if tok.Type != token.Glob {
			t.Errorf("input %s: type = %v, want GLOB", tt.input, tok.Type)
		}
		if tok.Literal != tt.want {
			t.Errorf("input %s: literal = %q, want %q", tt.input, tok.Literal, tt.want)
		}
	}
}

func TestLineComment(t *testing.T) {
	input := "ls # this is a comment\nread"

	l := New(input)

	tok := l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "ls" {
		t.Fatalf("expected ls, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Newline {
		t.Fatalf("expected NEWLINE, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "read" {
		t.Fatalf("expected read, got %v %q", tok.Type, tok.Literal)
	}
}

func TestBlockComment(t *testing.T) {
	input := "ls #{ block\ncomment #} read"

	l := New(input)

	tok := l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "ls" {
		t.Fatalf("expected ls, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "read" {
		t.Fatalf("expected read after block comment, got %v %q", tok.Type, tok.Literal)
	}
}

func TestNewlines(t *testing.T) {
	input := "ls\nread\nwrite"

	l := New(input)
	expects := []token.TokenType{
		token.Ident, token.Newline,
		token.Ident, token.Newline,
		token.Ident, token.EOF,
	}

	for i, want := range expects {
		tok := l.NextToken()
		if tok.Type != want {
			t.Fatalf("token[%d]: want %v, got %v (%q)", i, want, tok.Type, tok.Literal)
		}
	}
}

func TestLineAndColumn(t *testing.T) {
	input := "ls /var\nread"

	l := New(input)

	tok := l.NextToken() // ls
	if tok.Line != 1 || tok.Col != 1 {
		t.Errorf("ls position = %d:%d, want 1:1", tok.Line, tok.Col)
	}

	tok = l.NextToken() // /var
	if tok.Line != 1 || tok.Col != 4 {
		t.Errorf("/var position = %d:%d, want 1:4", tok.Line, tok.Col)
	}

	tok = l.NextToken() // \n
	if tok.Line != 1 {
		t.Errorf("newline line = %d, want 1", tok.Line)
	}

	tok = l.NextToken() // read
	if tok.Line != 2 || tok.Col != 1 {
		t.Errorf("read position = %d:%d, want 2:1", tok.Line, tok.Col)
	}
}

func TestModifierPattern(t *testing.T) {
	// key:value -- colon is a separate token, key and value are idents
	input := "ext:log"

	l := New(input)

	tok := l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "ext" {
		t.Fatalf("expected ident 'ext', got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Colon {
		t.Fatalf("expected COLON, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "log" {
		t.Fatalf("expected ident 'log', got %v %q", tok.Type, tok.Literal)
	}
}

func TestModifierWithOperator(t *testing.T) {
	input := "size:>100m"

	l := New(input)

	tok := l.NextToken() // size
	if tok.Type != token.Ident || tok.Literal != "size" {
		t.Fatalf("expected ident 'size', got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken() // :
	if tok.Type != token.Colon {
		t.Fatalf("expected COLON, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken() // >100m
	if tok.Literal != ">100m" {
		t.Fatalf("expected '>100m', got %q", tok.Literal)
	}
}

// TestExampleFindLogs tokenizes: ls /var/log ext:log size:>100m age:>7d | gz
func TestExampleFindLogs(t *testing.T) {
	input := "ls /var/log ext:log size:>100m age:>7d | gz"

	wants := []struct {
		typ token.TokenType
		lit string
	}{
		{token.Ident, "ls"},
		{token.Path, "/var/log"},
		{token.Ident, "ext"},
		{token.Colon, ":"},
		{token.Ident, "log"},
		{token.Ident, "size"},
		{token.Colon, ":"},
		{token.Ident, ">100m"},
		{token.Ident, "age"},
		{token.Colon, ":"},
		{token.Ident, ">7d"},
		{token.Pipe, "|"},
		{token.Ident, "gz"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, w := range wants {
		tok := l.NextToken()
		if tok.Type != w.typ {
			t.Fatalf("token[%d]: type want=%v, got=%v (%q)", i, w.typ, tok.Type, tok.Literal)
		}
		if tok.Literal != w.lit {
			t.Fatalf("token[%d]: literal want=%q, got=%q", i, w.lit, tok.Literal)
		}
	}
}

// TestExampleParallelHealthCheck tokenizes: @k8s pods ns:prod & @db ping & @redis ping ; report
func TestExampleParallelHealthCheck(t *testing.T) {
	input := "@k8s pods ns:prod & @db ping & @redis ping ; report"

	wants := []struct {
		typ token.TokenType
		lit string
	}{
		{token.At, "@"},
		{token.Ident, "k8s"},
		{token.Ident, "pods"},
		{token.Ident, "ns"},
		{token.Colon, ":"},
		{token.Ident, "prod"},
		{token.Ampersand, "&"},
		{token.At, "@"},
		{token.Ident, "db"},
		{token.Ident, "ping"},
		{token.Ampersand, "&"},
		{token.At, "@"},
		{token.Ident, "redis"},
		{token.Ident, "ping"},
		{token.Semicolon, ";"},
		{token.Ident, "report"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, w := range wants {
		tok := l.NextToken()
		if tok.Type != w.typ {
			t.Fatalf("token[%d]: type want=%v, got=%v (%q)", i, w.typ, tok.Type, tok.Literal)
		}
		if tok.Literal != w.lit {
			t.Fatalf("token[%d]: literal want=%q, got=%q", i, w.lit, tok.Literal)
		}
	}
}

// TestExampleMCPWorkflow tokenizes the assignment + filter example
func TestExampleMCPWorkflow(t *testing.T) {
	input := "$issues = @github issues repo:acme/api state:open label:bug"

	wants := []struct {
		typ token.TokenType
		lit string
	}{
		{token.Dollar, "$"},
		{token.Ident, "issues"},
		{token.Equals, "="},
		{token.At, "@"},
		{token.Ident, "github"},
		{token.Ident, "issues"},
		{token.Ident, "repo"},
		{token.Colon, ":"},
		{token.Path, "acme/api"},
		{token.Ident, "state"},
		{token.Colon, ":"},
		{token.Ident, "open"},
		{token.Ident, "label"},
		{token.Colon, ":"},
		{token.Ident, "bug"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, w := range wants {
		tok := l.NextToken()
		if tok.Type != w.typ {
			t.Fatalf("token[%d]: type want=%v, got=%v (%q)", i, w.typ, tok.Type, tok.Literal)
		}
		if tok.Literal != w.lit {
			t.Fatalf("token[%d]: literal want=%q, got=%q", i, w.lit, tok.Literal)
		}
	}
}

// TestExampleRedirect tokenizes: ls /tmp -> out.txt
func TestExampleRedirect(t *testing.T) {
	input := "ls /tmp -> out.txt"

	wants := []struct {
		typ token.TokenType
		lit string
	}{
		{token.Ident, "ls"},
		{token.Path, "/tmp"},
		{token.ArrowR, "->"},
		{token.Ident, "out.txt"},
		{token.EOF, ""},
	}

	l := New(input)
	for i, w := range wants {
		tok := l.NextToken()
		if tok.Type != w.typ {
			t.Fatalf("token[%d]: type want=%v, got=%v (%q)", i, w.typ, tok.Type, tok.Literal)
		}
		if tok.Literal != w.lit {
			t.Fatalf("token[%d]: literal want=%q, got=%q", i, w.lit, tok.Literal)
		}
	}
}

// TestExampleDirective tokenizes: !human
func TestExampleDirective(t *testing.T) {
	input := "!human"

	l := New(input)

	tok := l.NextToken()
	if tok.Type != token.Bang || tok.Literal != "!" {
		t.Fatalf("expected BANG, got %v %q", tok.Type, tok.Literal)
	}

	tok = l.NextToken()
	if tok.Type != token.Ident || tok.Literal != "human" {
		t.Fatalf("expected ident 'human', got %v %q", tok.Type, tok.Literal)
	}
}

func BenchmarkLexer(b *testing.B) {
	input := `# Find large old logs and compress them
ls /var/log ext:log size:>100m age:>7d | gz

# Parallel health check
@k8s pods ns:prod status:running & @db ping & @redis ping ; report

# MCP-driven workflow
$issues = @github issues repo:acme/api state:open label:bug
$issues ? priority:high | @slack post #oncall

# Extension invocation
deploy staging wait:healthy rollback:auto
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := New(input)
		for {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				break
			}
		}
	}
}
