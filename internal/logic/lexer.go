// Package logic provides a Mindustry logic processor implementation
// for the mdt-server project.
//
// This package implements a lexer, parser, and virtual machine
// for executing Mindustry logic programs.
package logic

import (
	"fmt"
	"strings"
)

// TokenType represents the type of a token in the logic language.
type TokenType int

const (
	// ErrorToken represents an error token.
	ErrorToken TokenType = iota
	// EOF represents the end of file.
	EOF
	// Ident represents an identifier (variable name).
	Ident
	// Int represents an integer literal.
	Int
	// Float represents a floating-point literal.
	Float
	// String represents a string literal.
	String
	// Bool represents a boolean literal.
	Bool

	// Operators
	Operator
	Plus        // +
	Minus       // -
	Multiply    // *
	Divide      // /
	Mod         // %
	Power       // ^
	Equal       // =
	NotEqual    // !=
	LessThan    // <
	LessEqual   // <=
	GreaterThan // >
	GreaterEqual // >=
	And         // &&
	Or          // ||
	Xor         // ^^
	Not         // !

	// Keywords
	Keyword
	Msg         // msg
	Mov         // mov
	Add         // add
	Sub         // sub
	Mul         // mul
	Div         // div
	ModOp       // mod
	Pow         // pow
	AndOp       // and
	OrOp        // or
	XorOp       // xor
	NotOp       // not
	Min         // min
	Max         // max
	Diff        // diff
	Angle       // angle
	Length      // length
	Rotate      // rotate
	Draw        // draw
	DrawLine    // drawLine
	DrawRect    // drawRect
	DrawPoly    // drawPoly
	DrawCircle  // drawCircle
	Clk         // clk
	ClkSet      // clkSet
	Input       // input
	Output      // output
	Jump        // jump
	Jz          // jz
	Jnz         // jnz
	Top         // top
	Set         // set
	SetCons     // setCons
	GetPos      // getPos
	SetPos      // setPos
	Flag        // flag
	End         // end
	Label       // label
	Call        // call
	Ret         // ret
	Halt        // halt
	Wait        // wait
	Uqueue      // uqueue
	Ucontrol    // ucontrol
	Ubuild      // ubuild
	Udelete     // udelete
	Umine       // umine
	Umove       // umove
	Utarget     // utarget
	Upayload    // upayload
	Ucore       // ucore
	Syn         // syn
	Pkilla      // pkilla
	Pkillb      // pkillb
	Pkillc      // pkillc
	Pkilld      // pkilld
	PkillAll    // pkillAll

	// Separators
	Separator
	LeftParen   // (
	RightParen  // )
	LeftBrace   // {
	RightBrace  // }
	Comma       // ,
	Period      // .
	Hash        // #
	Dollar      // $

	// Literals
	True  // true
	False // false
)

// keywordMap maps keywords to their token types.
var keywordMap = map[string]TokenType{
	"msg":       Msg,
	"mov":       Mov,
	"add":       Add,
	"sub":       Sub,
	"mul":       Mul,
	"div":       Div,
	"mod":       ModOp,
	"pow":       Pow,
	"and":       AndOp,
	"or":        OrOp,
	"xor":       XorOp,
	"not":       NotOp,
	"min":       Min,
	"max":       Max,
	"diff":      Diff,
	"angle":     Angle,
	"length":    Length,
	"rotate":    Rotate,
	"draw":      Draw,
	"drawLine":  DrawLine,
	"drawRect":  DrawRect,
	"drawPoly":  DrawPoly,
	"drawCircle": DrawCircle,
	"clk":       Clk,
	"clkSet":    ClkSet,
	"input":     Input,
	"output":    Output,
	"jump":      Jump,
	"jz":        Jz,
	"jnz":       Jnz,
	"top":       Top,
	"set":       Set,
	"setCons":   SetCons,
	"getPos":    GetPos,
	"setPos":    SetPos,
	"flag":      Flag,
	"end":       End,
	"label":     Label,
	"call":      Call,
	"ret":       Ret,
	"halt":      Halt,
	"wait":      Wait,
	"uqueue":    Uqueue,
	"ucontrol":  Ucontrol,
	"ubuild":    Ubuild,
	"udelete":   Udelete,
	"umine":     Umine,
	"umove":     Umove,
	"utarget":   Utarget,
	"upayload":  Upayload,
	"ucore":     Ucore,
	"syn":       Syn,
	"pkilla":    Pkilla,
	"pkillb":    Pkillb,
	"pkillc":    Pkillc,
	"pkilld":    Pkilld,
	"pkillAll":  PkillAll,
	"true":      True,
	"false":     False,
}

// Position represents a position in the source code.
type Position struct {
	Line   int // Line number (1-indexed)
	Column int // Column number (1-indexed)
}

// String returns a string representation of the position.
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Token represents a lexical token in the logic language.
type Token struct {
	Type    TokenType
	Literal string
	Position
}

// String returns a string representation of the token.
func (t Token) String() string {
	switch t.Type {
	case ErrorToken:
		return fmt.Sprintf("ERROR(%s)", t.Literal)
	case EOF:
		return "EOF"
	case Ident:
		return fmt.Sprintf("IDENT(%s)", t.Literal)
	case Int:
		return fmt.Sprintf("INT(%s)", t.Literal)
	case Float:
		return fmt.Sprintf("FLOAT(%s)", t.Literal)
	case String:
		return fmt.Sprintf("STRING(%s)", t.Literal)
	case Bool:
		return fmt.Sprintf("BOOL(%s)", t.Literal)
	case Operator:
		return fmt.Sprintf("OP(%s)", t.Literal)
	case Keyword:
		return fmt.Sprintf("KEYWORD(%s)", t.Literal)
	case Separator:
		return fmt.Sprintf("SEP(%s)", t.Literal)
	case True, False:
		return fmt.Sprintf("BOOL(%s)", t.Literal)
	default:
		return fmt.Sprintf("%v(%s)", t.Type, t.Literal)
	}
}

// Lexer represents a lexical analyzer for the logic language.
type Lexer struct {
	input     string // The input string to tokenize
	pos       int    // Current position in the input
	readPos   int    // Next position to read
	ch        byte   // Current character
	line      int    // Current line number
	column    int    // Current column number
	startPos  Position // Start position of current token
	tokens    []Token // Buffer of tokens
}

// NewLexer creates a new lexer for the given input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:   input,
		line:    1,
		column:  1,
		startPos: Position{Line: 1, Column: 1},
	}
	l.readChar()
	return l
}

// readChar reads the next character from the input.
func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0 // EOF
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	if l.ch == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	l.readPos++
}

// peekChar returns the next character without advancing the position.
func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

// isLetter checks if a character is a letter or underscore.
func (l *Lexer) isLetter(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_'
}

// isDigit checks if a character is a digit.
func (l *Lexer) isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isAlphaNum checks if a character is alphanumeric.
func (l *Lexer) isAlphaNum(ch byte) bool {
	return l.isLetter(ch) || l.isDigit(ch)
}

// readIdentifier reads an identifier or keyword.
func (l *Lexer) readIdentifier() string {
	start := l.pos
	for l.isAlphaNum(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

// readNumber reads a number (integer or float).
func (l *Lexer) readNumber() string {
	start := l.pos
	isFloat := false
	for l.isDigit(l.ch) || (l.ch == '.' && !isFloat) {
		if l.ch == '.' {
			isFloat = true
		}
		l.readChar()
	}
	return l.input[start:l.pos]
}

// readString reads a string literal.
func (l *Lexer) readString() (string, error) {
	l.readChar() // Skip opening quote
	start := l.pos
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // Skip escape character
		}
		l.readChar()
	}
	if l.ch == 0 {
		return "", fmt.Errorf("unterminated string at line %d", l.line)
	}
	l.readChar() // Skip closing quote
	return l.input[start:l.pos-1], nil
}

// skipWhitespace skips whitespace characters.
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
		l.readChar()
	}
}

// skipComment skips a single-line or multi-line comment.
func (l *Lexer) skipComment() {
	if l.ch == '#' {
		// Single-line comment: skip until end of line
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	} else if l.ch == '/' && l.peekChar() == '/' {
		// Alternative single-line comment
		l.readChar() // Skip /
		l.readChar() // Skip /
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	}
}

// newToken creates a new token with the given type and literal.
func (l *Lexer) newToken(tokenType TokenType, literal string) Token {
	return Token{
		Type:    tokenType,
		Literal: literal,
		Position: Position{
			Line:   l.startPos.Line,
			Column: l.startPos.Column,
		},
	}
}

// newErrorToken creates an error token with the given message.
func (l *Lexer) newErrorToken(message string) Token {
	return Token{
		Type:    ErrorToken,
		Literal: message,
		Position: Position{
			Line:   l.line,
			Column: l.column,
		},
	}
}

// nextToken returns the next token from the input.
func (l *Lexer) nextToken() Token {
	l.skipWhitespace()

	// Skip comments
	l.skipComment()
	l.skipWhitespace()

	l.startPos = Position{Line: l.line, Column: l.column}

	if l.ch == 0 {
		return l.newToken(EOF, "")
	}

	// Check for operators and multi-character tokens first
	if l.ch == '=' && l.peekChar() == '=' {
		l.readChar()
		l.readChar()
		return l.newToken(Equal, "==")
	}
	if l.ch == '!' && l.peekChar() == '=' {
		l.readChar()
		l.readChar()
		return l.newToken(NotEqual, "!=")
	}
	if l.ch == '<' && l.peekChar() == '=' {
		l.readChar()
		l.readChar()
		return l.newToken(LessEqual, "<=")
	}
	if l.ch == '<' && l.peekChar() == '>' {
		l.readChar()
		l.readChar()
		return l.newToken(NotEqual, "<>")
	}
	if l.ch == '>' && l.peekChar() == '=' {
		l.readChar()
		l.readChar()
		return l.newToken(GreaterEqual, ">=")
	}
	if l.ch == '&' && l.peekChar() == '&' {
		l.readChar()
		l.readChar()
		return l.newToken(And, "&&")
	}
	if l.ch == '|' && l.peekChar() == '|' {
		l.readChar()
		l.readChar()
		return l.newToken(Or, "||")
	}
	if l.ch == '^' && l.peekChar() == '^' {
		l.readChar()
		l.readChar()
		return l.newToken(Xor, "^^")
	}

	// Single character tokens
	switch l.ch {
	case '+':
		l.readChar()
		return l.newToken(Plus, "+")
	case '-':
		l.readChar()
		return l.newToken(Minus, "-")
	case '*':
		l.readChar()
		return l.newToken(Multiply, "*")
	case '/':
		l.readChar()
		return l.newToken(Divide, "/")
	case '%':
		l.readChar()
		return l.newToken(Mod, "%")
	case '^':
		l.readChar()
		return l.newToken(Power, "^")
	case '=':
		l.readChar()
		return l.newToken(Equal, "=")
	case '<':
		l.readChar()
		return l.newToken(LessThan, "<")
	case '>':
		l.readChar()
		return l.newToken(GreaterThan, ">")
	case '!':
		l.readChar()
		return l.newToken(Not, "!")
	case '(':
		l.readChar()
		return l.newToken(LeftParen, "(")
	case ')':
		l.readChar()
		return l.newToken(RightParen, ")")
	case '{':
		l.readChar()
		return l.newToken(LeftBrace, "{")
	case '}':
		l.readChar()
		return l.newToken(RightBrace, "}")
	case ',':
		l.readChar()
		return l.newToken(Comma, ",")
	case '.':
		l.readChar()
		return l.newToken(Period, ".")
	case '#':
		l.readChar()
		return l.newToken(Hash, "#")
	case '$':
		l.readChar()
		return l.newToken(Dollar, "$")
	case '"':
		str, err := l.readString()
		if err != nil {
			return l.newErrorToken(err.Error())
		}
		return l.newToken(String, str)
	}

	// Check for numbers
	if l.isDigit(l.ch) {
		numStr := l.readNumber()
		// Check if it looks like a float (has a decimal point)
		if strings.Contains(numStr, ".") {
			return l.newToken(Float, numStr)
		}
		return l.newToken(Int, numStr)
	}

	// Check for identifiers and keywords
	if l.isLetter(l.ch) {
		ident := l.readIdentifier()
		if tokType, ok := keywordMap[ident]; ok {
			return l.newToken(tokType, ident)
		}
		return l.newToken(Ident, ident)
	}

	// Unknown character
	return l.newErrorToken(fmt.Sprintf("unexpected character: %q", l.ch))
}

// NextToken returns the next token from the input.
// This method returns tokens from the internal buffer,
// refilling the buffer when empty.
func (l *Lexer) NextToken() Token {
	if len(l.tokens) > 0 {
		token := l.tokens[0]
		l.tokens = l.tokens[1:]
		return token
	}
	return l.nextToken()
}

// Lex returns all tokens from the input.
func (l *Lexer) Lex() []Token {
	var tokens []Token
	for {
		token := l.NextToken()
		if token.Type == EOF {
			tokens = append(tokens, token)
			break
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// BufferToken buffers a token for later retrieval.
func (l *Lexer) BufferToken(token Token) {
	l.tokens = append(l.tokens, token)
}

// BufferTokens buffers multiple tokens for later retrieval.
func (l *Lexer) BufferTokens(tokens []Token) {
	l.tokens = append(l.tokens, tokens...)
}

// Reset resets the lexer to the beginning of the input.
func (l *Lexer) Reset(input string) {
	l.input = input
	l.pos = 0
	l.readPos = 0
	l.line = 1
	l.column = 1
	l.ch = 0
	l.startPos = Position{Line: 1, Column: 1}
	l.tokens = nil
	l.readChar()
}

// Scan scans the input and returns tokens.
// This function is an alias for Lex.
func Scan(input string) ([]Token, error) {
	lexer := NewLexer(input)
	var tokens []Token

	for {
		token := lexer.NextToken()
		tokens = append(tokens, token)

		if token.Type == EOF || token.Type == ErrorToken {
			break
		}
	}

	if len(tokens) > 0 && tokens[len(tokens)-1].Type == ErrorToken {
		return nil, fmt.Errorf("lexing error at %s: %s", tokens[len(tokens)-1].Position, tokens[len(tokens)-1].Literal)
	}

	return tokens, nil
}

// Scanner is an alias for Lexer for compatibility.
type Scanner Lexer

// NewScanner creates a new scanner from input.
func NewScanner(input string) *Scanner {
	l := NewLexer(input)
	return (*Scanner)(l)
}

// ScanToken scans and returns the next token.
func (s *Scanner) ScanToken() Token {
	return (*Lexer)(s).NextToken()
}

// ScanAll scans and returns all tokens.
func (s *Scanner) ScanAll() []Token {
	return (*Lexer)(s).Lex()
}

// IsKeyword checks if the token is a keyword.
func IsKeyword(token Token) bool {
	return token.Type >= Keyword && token.Type <= PkillAll
}

// IsOperator checks if the token is an operator.
func IsOperator(token Token) bool {
	switch token.Type {
	case Operator, Plus, Minus, Multiply, Divide, Mod, Power,
		Equal, NotEqual, LessThan, LessEqual, GreaterThan, GreaterEqual,
		And, Or, Xor, Not:
		return true
	default:
		return false
	}
}

// IsSeparator checks if the token is a separator.
func IsSeparator(token Token) bool {
	switch token.Type {
	case Separator, LeftParen, RightParen, LeftBrace, RightBrace,
		Comma, Period, Hash, Dollar:
		return true
	default:
		return false
	}
}

// IsLiteral checks if the token is a literal value.
func IsLiteral(token Token) bool {
	return token.Type == Int || token.Type == Float || token.Type == String || token.Type == True || token.Type == False
}

// IsIdentifier checks if the token is an identifier.
func IsIdentifier(token Token) bool {
	return token.Type == Ident
}

// IsNumber checks if the token is a number (int or float).
func IsNumber(token Token) bool {
	return token.Type == Int || token.Type == Float
}

// IsControlFlow checks if the token is a control flow keyword.
func IsControlFlow(token Token) bool {
	switch token.Type {
	case Jump, Jz, Jnz, Top, Label, Call, Ret, Halt, End, Wait, Flag:
		return true
	default:
		return false
	}
}

// IsDrawCommand checks if the token is a draw command.
func IsDrawCommand(token Token) bool {
	switch token.Type {
	case Draw, DrawLine, DrawRect, DrawPoly, DrawCircle:
		return true
	default:
		return false
	}
}

// IsUnitCommand checks if the token is a unit command.
func IsUnitCommand(token Token) bool {
	switch token.Type {
	case Ubuild, Udelete, Umine, Umove, Utarget, Upayload, Ucontrol, Ucore:
		return true
	default:
		return false
	}
}

// IsUnitQueueCommand checks if the token is a unit queue command.
func IsUnitQueueCommand(token Token) bool {
	switch token.Type {
	case Uqueue, Ubuild, Udelete, Umine, Umove, Utarget, Upayload, Ucontrol, Ucore:
		return true
	default:
		return false
	}
}

// IsSyncCommand checks if the token is a sync command.
func IsSyncCommand(token Token) bool {
	switch token.Type {
	case Syn, Pkilla, Pkillb, Pkillc, Pkilld, PkillAll:
		return true
	default:
		return false
	}
}

// IsIOCommand checks if the token is an I/O command.
func IsIOCommand(token Token) bool {
	switch token.Type {
	case Input, Output:
		return true
	default:
		return false
	}
}

// IsMathCommand checks if the token is a math command.
func IsMathCommand(token Token) bool {
	switch token.Type {
	case Add, Sub, Mul, Div, ModOp, Pow, Min, Max, Diff, Angle, Length, Rotate:
		return true
	default:
		return false
	}
}

// IsBasicCommand checks if the token is a basic command.
func IsBasicCommand(token Token) bool {
	switch token.Type {
	case Mov, Add, Sub, Mul, Div, ModOp, Pow, AndOp, OrOp, XorOp, NotOp, Min, Max, Diff, Angle, Length, Rotate, Draw, DrawLine, DrawRect, DrawPoly, DrawCircle, Clk, ClkSet, Input, Output, Jump, Jz, Jnz, Top, Set, SetCons, GetPos, SetPos, Flag, End, Label, Call, Ret, Halt, Wait, Uqueue, Ucontrol, Ubuild, Udelete, Umine, Umove, Utarget, Upayload, Ucore, Syn, Pkilla, Pkillb, Pkillc, Pkilld, PkillAll:
		return true
	default:
		return false
	}
}

// IsValue checks if the token is a value (literal or identifier).
func IsValue(token Token) bool {
	return IsLiteral(token) || IsIdentifier(token)
}

// IsExpressionElement checks if the token can be part of an expression.
func IsExpressionElement(token Token) bool {
	return IsValue(token) || IsOperator(token) || IsSeparator(token) || IsOperator(token) || IsSeparator(token)
}

// IsStatementStart checks if the token can start a statement.
func IsStatementStart(token Token) bool {
	switch token.Type {
	case Msg, Mov, Add, Sub, Mul, Div, ModOp, Pow, AndOp, OrOp, XorOp, NotOp, Min, Max, Diff, Angle, Length, Rotate, Draw, DrawLine, DrawRect, DrawPoly, DrawCircle, Clk, ClkSet, Input, Output, Jump, Jz, Jnz, Top, Set, SetCons, GetPos, SetPos, Flag, Label, Call, Halt, Wait, Uqueue, Ucontrol, Ubuild, Udelete, Umine, Umove, Utarget, Upayload, Ucore, Syn, Pkilla, Pkillb, Pkillc, Pkilld, PkillAll:
		return true
	default:
		return IsIdentifier(token)
	}
}

// IsEndStatement checks if the token ends a statement.
func IsEndStatement(token Token) bool {
	return token.Type == Hash || token.Type == EOF || token.Type == Separator
}

// IsLineEnd checks if the token represents a line ending.
func IsLineEnd(token Token) bool {
	return token.Type == Hash || token.Type == EOF
}

// IsError checks if the token is an error.
func IsError(token Token) bool {
	return token.Type == ErrorToken
}

// IsEOF checks if the token is end of file.
func IsEOF(token Token) bool {
	return token.Type == EOF
}

// IsIdent checks if the token is an identifier with the given name.
func IsIdent(token Token, name string) bool {
	return token.Type == Ident && token.Literal == name
}

// IsKeywordType checks if the token is the given keyword.
func IsKeywordType(token Token, keyword TokenType) bool {
	return token.Type == keyword
}

// TokenEquality checks if two tokens are equal.
func TokenEquality(t1, t2 Token) bool {
	return t1.Type == t2.Type && t1.Literal == t2.Literal && t1.Line == t2.Line && t1.Column == t2.Column
}

// TokenInSlice checks if a token is in a slice.
func TokenInSlice(token Token, tokens []Token) bool {
	for _, t := range tokens {
		if TokenEquality(token, t) {
			return true
		}
	}
	return false
}

// TokenIndex returns the index of a token in a slice.
func TokenIndex(token Token, tokens []Token) int {
	for i, t := range tokens {
		if TokenEquality(token, t) {
			return i
		}
	}
	return -1
}

// TokenCount counts occurrences of a token in a slice.
func TokenCount(token Token, tokens []Token) int {
	count := 0
	for _, t := range tokens {
		if TokenEquality(token, t) {
			count++
		}
	}
	return count
}

// TokenFilter filters tokens by a predicate.
func TokenFilter(tokens []Token, pred func(Token) bool) []Token {
	result := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		if pred(t) {
			result = append(result, t)
		}
	}
	return result
}

// TokenMap maps tokens to a new slice.
func TokenMap(tokens []Token, fn func(Token) Token) []Token {
	result := make([]Token, len(tokens))
	for i, t := range tokens {
		result[i] = fn(t)
	}
	return result
}

// TokenCopy copies a slice of tokens.
func TokenCopy(tokens []Token) []Token {
	result := make([]Token, len(tokens))
	copy(result, tokens)
	return result
}

// TokenConcat concatenates multiple slices of tokens.
func TokenConcat(tokenSlices ...[]Token) []Token {
	totalLen := 0
	for _, slice := range tokenSlices {
		totalLen += len(slice)
	}
	result := make([]Token, 0, totalLen)
	for _, slice := range tokenSlices {
		result = append(result, slice...)
	}
	return result
}

// TokenPrepend prepends tokens to a slice.
func TokenPrepend(tokens []Token, prepend ...Token) []Token {
	return append(prepend, tokens...)
}

// TokenAppend appends tokens to a slice.
func TokenAppend(tokens []Token, appended ...Token) []Token {
	return append(tokens, appended...)
}

// TokenTake takes the first n tokens.
func TokenTake(tokens []Token, n int) []Token {
	if n < 0 {
		n = 0
	}
	if n > len(tokens) {
		n = len(tokens)
	}
	return tokens[:n]
}

// TokenDrop drops the first n tokens.
func TokenDrop(tokens []Token, n int) []Token {
	if n < 0 {
		n = 0
	}
	if n > len(tokens) {
		n = len(tokens)
	}
	return tokens[n:]
}

// TokenSplit splits tokens at position.
func TokenSplit(tokens []Token, pos int) ([]Token, []Token) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(tokens) {
		pos = len(tokens)
	}
	return tokens[:pos], tokens[pos:]
}

// TokenReverse reverses a slice of tokens.
func TokenReverse(tokens []Token) []Token {
	result := make([]Token, len(tokens))
	for i, t := range tokens {
		result[len(tokens)-1-i] = t
	}
	return result
}

// DebugTokens returns a formatted string of all tokens.
func DebugTokens(tokens []Token) string {
	var sb strings.Builder
	for _, tok := range tokens {
		sb.WriteString(tok.String())
		sb.WriteString("\n")
	}
	return sb.String()
}
