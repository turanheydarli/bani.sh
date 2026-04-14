package token

import "testing"

func TestTokenTypeString(t *testing.T) {
	tests := []struct {
		typ  TokenType
		want string
	}{
		{Illegal, "ILLEGAL"},
		{EOF, "EOF"},
		{Newline, "NEWLINE"},
		{Ident, "IDENT"},
		{String, "STRING"},
		{Number, "NUMBER"},
		{Path, "PATH"},
		{Glob, "GLOB"},
		{At, "AT"},
		{Dollar, "DOLLAR"},
		{Bang, "BANG"},
		{Question, "QUESTION"},
		{Pipe, "PIPE"},
		{Semicolon, "SEMICOLON"},
		{Ampersand, "AMPERSAND"},
		{And, "AND"},
		{Or, "OR"},
		{ArrowR, "ARROW_R"},
		{ArrowL, "ARROW_L"},
		{Colon, "COLON"},
		{Equals, "EQUALS"},
		{TokenType(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		got := tt.typ.String()
		if got != tt.want {
			t.Errorf("TokenType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestLookup(t *testing.T) {
	// Banish has no keywords, everything is an identifier.
	tests := []string{"ls", "read", "write", "fetch", "deploy", "if", "for", "while"}

	for _, ident := range tests {
		got := Lookup(ident)
		if got != Ident {
			t.Errorf("Lookup(%q) = %v, want IDENT", ident, got)
		}
	}
}

func TestTokenStruct(t *testing.T) {
	tok := Token{
		Type:    Ident,
		Literal: "ls",
		Line:    1,
		Col:     1,
	}

	if tok.Type != Ident {
		t.Errorf("tok.Type = %v, want IDENT", tok.Type)
	}
	if tok.Literal != "ls" {
		t.Errorf("tok.Literal = %q, want %q", tok.Literal, "ls")
	}
	if tok.Line != 1 || tok.Col != 1 {
		t.Errorf("tok position = %d:%d, want 1:1", tok.Line, tok.Col)
	}
}
