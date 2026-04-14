// Package token defines the token types and Token struct used by the lexer
// and parser to represent lexical elements of .bsh source files.
package token

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Special
	Illegal TokenType = iota
	EOF
	Newline

	// Literals
	Ident
	String
	Number
	Path
	Glob

	// Sigils
	At        // @
	Dollar    // $
	Bang      // !
	Question  // ?

	// Operators
	Pipe      // |
	Semicolon // ;
	Ampersand // &
	And       // &&
	Or        // ||
	ArrowR    // ->
	ArrowL    // <-

	// Delimiters
	Colon  // :
	Equals // =

	// Comment (not emitted, but used internally by lexer)
	Comment
)

var tokenNames = map[TokenType]string{
	Illegal:   "ILLEGAL",
	EOF:       "EOF",
	Newline:   "NEWLINE",
	Ident:     "IDENT",
	String:    "STRING",
	Number:    "NUMBER",
	Path:      "PATH",
	Glob:      "GLOB",
	At:        "AT",
	Dollar:    "DOLLAR",
	Bang:      "BANG",
	Question:  "QUESTION",
	Pipe:      "PIPE",
	Semicolon: "SEMICOLON",
	Ampersand: "AMPERSAND",
	And:       "AND",
	Or:        "OR",
	ArrowR:    "ARROW_R",
	ArrowL:    "ARROW_L",
	Colon:     "COLON",
	Equals:    "EQUALS",
	Comment:   "COMMENT",
}

// String returns the human-readable name of a token type.
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

// Token represents a single lexical token with its type, literal value,
// and source position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

// Lookup returns the token type for an identifier. Banish has no keywords,
// so this always returns Ident.
func Lookup(ident string) TokenType {
	_ = ident
	return Ident
}
