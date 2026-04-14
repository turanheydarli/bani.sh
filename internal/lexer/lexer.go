// Package lexer implements the scanner that produces a token stream
// from .bsh source bytes.
package lexer

import "go.bani.sh/banish/internal/token"

// Lexer reads source bytes and produces tokens.
type Lexer struct {
	input   string
	pos     int  // current position (points to current char)
	readPos int  // next read position (after current char)
	ch      byte // current char under examination
	line    int
	col     int
}

// New creates a new Lexer for the given input string.
func New(input string) *Lexer {
	l := &Lexer{
		input: input,
		line:  1,
		col:   0,
	}
	l.readChar()
	return l
}

// NextToken scans and returns the next token from the input.
func (l *Lexer) NextToken() token.Token {
	l.skipWhitespace()

	tok := token.Token{Line: l.line, Col: l.col}

	switch l.ch {
	case 0:
		tok.Type = token.EOF
		tok.Literal = ""

	case '\n':
		tok.Type = token.Newline
		tok.Literal = "\n"
		l.line++
		l.col = 0
		l.readChar()

	case '#':
		l.skipComment()
		return l.NextToken()

	case '|':
		if l.peekChar() == '|' {
			col := l.col
			l.readChar()
			tok.Type = token.Or
			tok.Literal = "||"
			tok.Col = col
			l.readChar()
		} else {
			tok.Type = token.Pipe
			tok.Literal = "|"
			l.readChar()
		}

	case ';':
		tok.Type = token.Semicolon
		tok.Literal = ";"
		l.readChar()

	case '&':
		if l.peekChar() == '&' {
			col := l.col
			l.readChar()
			tok.Type = token.And
			tok.Literal = "&&"
			tok.Col = col
			l.readChar()
		} else {
			tok.Type = token.Ampersand
			tok.Literal = "&"
			l.readChar()
		}

	case '?':
		tok.Type = token.Question
		tok.Literal = "?"
		l.readChar()

	case '@':
		tok.Type = token.At
		tok.Literal = "@"
		l.readChar()

	case '$':
		tok.Type = token.Dollar
		tok.Literal = "$"
		l.readChar()

	case '!':
		tok.Type = token.Bang
		tok.Literal = "!"
		l.readChar()

	case '=':
		tok.Type = token.Equals
		tok.Literal = "="
		l.readChar()

	case ':':
		tok.Type = token.Colon
		tok.Literal = ":"
		l.readChar()

	case '-':
		if l.peekChar() == '>' {
			col := l.col
			l.readChar()
			tok.Type = token.ArrowR
			tok.Literal = "->"
			tok.Col = col
			l.readChar()
		} else {
			tok = l.readWord(tok)
		}

	case '<':
		if l.peekChar() == '-' {
			col := l.col
			l.readChar()
			tok.Type = token.ArrowL
			tok.Literal = "<-"
			tok.Col = col
			l.readChar()
		} else {
			tok = l.readWord(tok)
		}

	case '"':
		tok.Type = token.String
		tok.Literal = l.readString()

	default:
		if isDigit(l.ch) {
			tok.Literal = l.readNumber()
			tok.Type = token.Number
		} else if isWordStart(l.ch) {
			tok = l.readWord(tok)
		} else {
			tok.Type = token.Illegal
			tok.Literal = string(l.ch)
			l.readChar()
		}
	}

	return tok
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
	l.col++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	if l.peekChar() == '{' {
		// Block comment #{ ... #}
		l.readChar() // skip {
		l.readChar()
		for {
			if l.ch == 0 {
				return
			}
			if l.ch == '#' && l.peekChar() == '}' {
				l.readChar() // skip }
				l.readChar()
				return
			}
			if l.ch == '\n' {
				l.line++
				l.col = 0
			}
			l.readChar()
		}
	}
	// Line comment: consume to end of line
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening "
	start := l.pos
	var result []byte

	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			result = append(result, l.input[start:l.pos]...)
			l.readChar()
			switch l.ch {
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			default:
				result = append(result, '\\', l.ch)
			}
			l.readChar()
			start = l.pos
			continue
		}
		l.readChar()
	}

	result = append(result, l.input[start:l.pos]...)
	l.readChar() // skip closing "
	return string(result)
}

func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // skip .
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	return l.input[start:l.pos]
}

// readWord reads an identifier, path, or glob. The token type is determined
// by the characters found: if it contains / or starts with . or ~, it is a
// path; if it contains *, it is a glob; otherwise it is an ident.
func (l *Lexer) readWord(tok token.Token) token.Token {
	start := l.pos
	hasSlash := false
	hasGlob := false

	for isWordChar(l.ch) {
		if l.ch == '/' {
			hasSlash = true
		}
		if l.ch == '*' {
			hasGlob = true
		}
		l.readChar()
	}

	tok.Literal = l.input[start:l.pos]

	switch {
	case hasGlob:
		tok.Type = token.Glob
	case hasSlash || tok.Literal[0] == '.' || tok.Literal[0] == '~':
		tok.Type = token.Path
	default:
		tok.Type = token.Lookup(tok.Literal)
	}

	return tok
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isWordStart(ch byte) bool {
	return isLetter(ch) || ch == '_' || ch == '/' || ch == '.' || ch == '~' || ch == '-' || ch == '*' || ch == '>' || ch == '<' || ch == '+'
}

func isWordChar(ch byte) bool {
	return isLetter(ch) || isDigit(ch) || ch == '_' || ch == '-' || ch == '/' || ch == '.' || ch == '*' || ch == '~' || ch == '>' || ch == '<' || ch == '+'
}
