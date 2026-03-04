// Package logic provides a Mindustry logic processor implementation
// for the mdt-server project.
//
// This package implements a lexer, parser, and virtual machine
// for executing Mindustry logic programs.
package logic

import (
	"fmt"
	"strconv"
	"strings"
)

// ASTNode represents a node in the Abstract Syntax Tree.
type ASTNode interface {
	// nodeType returns the type of this node.
	nodeType() string
	// String returns a string representation of the node.
	String() string
}

// Statement represents a statement in the logic program.
type Statement interface {
	ASTNode
	// statementNode marks this as a statement.
	statementNode()
}

// Expression represents an expression in the logic program.
type Expression interface {
	ASTNode
	// expressionNode marks this as an expression.
	expressionNode()
}

// Program represents the root of the AST - a collection of statements.
type Program struct {
	Statements []Statement
}

// nodeType returns "Program".
func (p *Program) nodeType() string {
	return "Program"
}

// String returns a string representation of the program.
func (p *Program) String() string {
	var sb strings.Builder
	for i, stmt := range p.Statements {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(stmt.String())
	}
	return sb.String()
}

// AssignStatement represents an assignment statement.
type AssignStatement struct {
	Token Token       // The "=" token
	Name  *IdentExpr  // Variable name
	Value Expression  // Value to assign
}

// statementNode marks this as a statement.
func (s *AssignStatement) statementNode() {}

// nodeType returns "AssignStatement".
func (s *AssignStatement) nodeType() string {
	return "AssignStatement"
}

// String returns a string representation of the assignment.
func (s *AssignStatement) String() string {
	return fmt.Sprintf("%s = %s", s.Name, s.Value)
}

// PrintStatement represents a print statement (msg).
type PrintStatement struct {
	Token Token        // The "msg" token
	Value Expression   // Value to print
}

// statementNode marks this as a statement.
func (s *PrintStatement) statementNode() {}

// nodeType returns "PrintStatement".
func (s *PrintStatement) nodeType() string {
	return "PrintStatement"
}

// String returns a string representation of the print statement.
func (s *PrintStatement) String() string {
	return fmt.Sprintf("msg %s", s.Value)
}

// IfStatement represents an if conditional statement.
type IfStatement struct {
	Token     Token       // The condition token
	Condition Expression   // The condition expression
	Body      []Statement  // Body of the if statement
}

// statementNode marks this as a statement.
func (s *IfStatement) statementNode() {}

// nodeType returns "IfStatement".
func (s *IfStatement) nodeType() string {
	return "IfStatement"
}

// String returns a string representation of the if statement.
func (s *IfStatement) String() string {
	var sb strings.Builder
	sb.WriteString("if ")
	sb.WriteString(s.Condition.String())
	sb.WriteString(" { ")
	for i, stmt := range s.Body {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(stmt.String())
	}
	sb.WriteString(" }")
	return sb.String()
}

// JumpStatement represents a jump statement.
type JumpStatement struct {
	Token  Token // The "jump" token
	Target string // Target label
}

// statementNode marks this as a statement.
func (s *JumpStatement) statementNode() {}

// nodeType returns "JumpStatement".
func (s *JumpStatement) nodeType() string {
	return "JumpStatement"
}

// String returns a string representation of the jump statement.
func (s *JumpStatement) String() string {
	return fmt.Sprintf("jump %s", s.Target)
}

// LabelStatement represents a label statement.
type LabelStatement struct {
	Token Token // The "label" token
	Name  string // Label name
}

// statementNode marks this as a statement.
func (s *LabelStatement) statementNode() {}

// nodeType returns "LabelStatement".
func (s *LabelStatement) nodeType() string {
	return "LabelStatement"
}

// String returns a string representation of the label statement.
func (s *LabelStatement) String() string {
	return fmt.Sprintf("label %s", s.Name)
}

// BinaryExpr represents a binary operation.
type BinaryExpr struct {
	Token   Token       // The operator token
	Left    Expression  // Left operand
	Operator TokenType   // The operator token type
	Right   Expression  // Right operand
}

// expressionNode marks this as an expression.
func (e *BinaryExpr) expressionNode() {}

// nodeType returns "BinaryExpr".
func (e *BinaryExpr) nodeType() string {
	return "BinaryExpr"
}

// String returns a string representation of the binary expression.
func (e *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", e.Left, e.Operator, e.Right)
}

// UnaryExpr represents a unary operation.
type UnaryExpr struct {
	Token   Token       // The operator token
	Operator TokenType   // The operator token type
	Operand Expression  // The operand
}

// expressionNode marks this as an expression.
func (e *UnaryExpr) expressionNode() {}

// nodeType returns "UnaryExpr".
func (e *UnaryExpr) nodeType() string {
	return "UnaryExpr"
}

// String returns a string representation of the unary expression.
func (e *UnaryExpr) String() string {
	return fmt.Sprintf("%s%s", e.Operator, e.Operand)
}

// CallExpr represents a function call.
type CallExpr struct {
	Token Token         // The function name token
	Name  string        // Function name
	Args  []Expression  // Arguments
}

// expressionNode marks this as an expression.
func (e *CallExpr) expressionNode() {}

// nodeType returns "CallExpr".
func (e *CallExpr) nodeType() string {
	return "CallExpr"
}

// String returns a string representation of the call expression.
func (e *CallExpr) String() string {
	var sb strings.Builder
	sb.WriteString(e.Name)
	sb.WriteString("(")
	for i, arg := range e.Args {
		if i < len(e.Args) && i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(arg.String())
	}
	sb.WriteString(")")
	return sb.String()
}

// IdentExpr represents an identifier/variable reference.
type IdentExpr struct {
	Token Token  // The identifier token
	Value string // The identifier value
}

// expressionNode marks this as an expression.
func (e *IdentExpr) expressionNode() {}

// nodeType returns "IdentExpr".
func (e *IdentExpr) nodeType() string {
	return "IdentExpr"
}

// String returns a string representation of the identifier.
func (e *IdentExpr) String() string {
	return e.Value
}

// IntExpr represents an integer literal.
type IntExpr struct {
	Token Token // The integer token
	Value int64  // The integer value
}

// expressionNode marks this as an expression.
func (e *IntExpr) expressionNode() {}

// nodeType returns "IntExpr".
func (e *IntExpr) nodeType() string {
	return "IntExpr"
}

// String returns a string representation of the integer.
func (e *IntExpr) String() string {
	return strconv.FormatInt(e.Value, 10)
}

// FloatExpr represents a floating-point literal.
type FloatExpr struct {
	Token Token   // The float token
	Value float64 // The float value
}

// expressionNode marks this as an expression.
func (e *FloatExpr) expressionNode() {}

// nodeType returns "FloatExpr".
func (e *FloatExpr) nodeType() string {
	return "FloatExpr"
}

// String returns a string representation of the float.
func (e *FloatExpr) String() string {
	return strconv.FormatFloat(e.Value, 'f', -1, 64)
}

// BoolExpr represents a boolean literal.
type BoolExpr struct {
	Token Token  // The boolean token
	Value bool   // The boolean value
}

// expressionNode marks this as an expression.
func (e *BoolExpr) expressionNode() {}

// nodeType returns "BoolExpr".
func (e *BoolExpr) nodeType() string {
	return "BoolExpr"
}

// String returns a string representation of the boolean.
func (e *BoolExpr) String() string {
	if e.Value {
		return "true"
	}
	return "false"
}

// Error represents a parsing error.
type Error struct {
	Message  string
	Position Position
}

// Error returns the error message.
func (e *Error) Error() string {
	if e.Position.Line > 0 {
		return fmt.Sprintf("%s at %s", e.Message, e.Position)
	}
	return e.Message
}

// Parser represents a parser for the logic language.
type Parser struct {
	tokens  []Token   // All tokens from lexer
	pos     int       // Current position in tokens
	errors  []error   // Parse errors
}

// NewParser creates a new parser for the given tokens.
func NewParser(tokens []Token) *Parser {
	return &Parser{
		tokens: tokens,
		pos:    0,
		errors: make([]error, 0),
	}
}

// curToken returns the current token.
func (p *Parser) curToken() Token {
	if p.pos < 0 || p.pos >= len(p.tokens) {
		return Token{Type: EOF}
	}
	return p.tokens[p.pos]
}

// peekToken returns the token at the given offset from current position.
func (p *Parser) peekToken(offset int) Token {
	pos := p.pos + offset
	if pos < 0 || pos >= len(p.tokens) {
		return Token{Type: EOF}
	}
	return p.tokens[pos]
}

// nextToken advances to the next token.
func (p *Parser) nextToken() {
	p.pos++
}

// ParseProgram parses the entire program.
func (p *Parser) ParseProgram() *Program {
	program := &Program{Statements: make([]Statement, 0)}

	for p.curToken().Type != EOF && p.curToken().Type != ErrorToken {
		if stmt := p.ParseStatement(); stmt != nil {
			program.Statements = append(program.Statements, stmt)
		} else {
			// Skip token on error
			if p.curToken().Type != EOF {
				p.nextToken()
			}
		}
	}

	return program
}

// ParseStatement parses a single statement.
func (p *Parser) ParseStatement() Statement {
	switch p.curToken().Type {
	case EOF:
		return nil

	case Ident:
		// Could be an assignment or function call
		if p.peekToken(1).Type == Equal {
			return p.parseAssignStatement()
		}
		return nil // Function calls are not statements in this simple parser

	case Msg:
		return p.parsePrintStatement()

	case Mov:
		return p.parseMoveStatement()

	case Add, Sub, Mul, Div, ModOp, Pow, AndOp, OrOp, XorOp, NotOp, Min, Max, Diff, Angle, Length, Rotate:
		return p.parseMathStatement()

	case Jump, Jz, Jnz:
		return p.parseJumpStatement()

	case Label:
		return p.parseLabelStatement()

	case Halt:
		return p.parseHaltStatement()

	case Input:
		return p.parseInputStatement()

	case Output:
		return p.parseOutputStatement()

	case Clk, ClkSet:
		return p.parseClockStatement()

	case Wait:
		return p.parseWaitStatement()

	default:
		// Skip unknown tokens
		if p.curToken().Type != EOF {
			p.errors = append(p.errors, &Error{
				Message:  fmt.Sprintf("unexpected token %s", p.curToken()),
				Position: p.curToken().Position,
			})
			p.nextToken()
		}
		return nil
	}
}

// parseAssignStatement parses an assignment statement.
func (p *Parser) parseAssignStatement() Statement {
	name := &IdentExpr{Token: p.curToken(), Value: p.curToken().Literal}
	p.nextToken() // Skip identifier

	if p.curToken().Type != Equal {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected '=' after identifier, got %s", p.curToken()),
			Position: p.curToken().Position,
		})
		return nil
	}
	p.nextToken() // Skip '='

	value := p.ParseExpression()
	if value == nil {
		p.errors = append(p.errors, &Error{
			Message:  "expected expression after '='",
			Position: p.curToken().Position,
		})
		return nil
	}

	return &AssignStatement{
		Token:  p.tokens[p.pos-1],
		Name:   name,
		Value:  value,
	}
}

// parsePrintStatement parses a print statement.
func (p *Parser) parsePrintStatement() Statement {
	stmt := &PrintStatement{Token: p.curToken()}
	p.nextToken() // Skip "msg"

	value := p.ParseExpression()
	if value != nil {
		stmt.Value = value
	} else {
		stmt.Value = &IntExpr{Token: p.curToken(), Value: 0}
	}

	return stmt
}

// parseMoveStatement parses a mov statement.
func (p *Parser) parseMoveStatement() Statement {
	p.nextToken() // Skip "mov"

	dest := p.ParseExpression()
	if dest == nil {
		p.errors = append(p.errors, &Error{
			Message:  "expected destination after 'mov'",
			Position: p.curToken().Position,
		})
		return nil
	}

	if p.curToken().Type != Comma {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected ',' after destination, got %s", p.curToken()),
			Position: p.curToken().Position,
		})
		return nil
	}
	p.nextToken() // Skip ','

	source := p.ParseExpression()
	if source == nil {
		p.errors = append(p.errors, &Error{
			Message:  "expected source after ','",
			Position: p.curToken().Position,
		})
		return nil
	}

	return &AssignStatement{
		Token:  p.tokens[p.pos-2],
		Name:   dest.(*IdentExpr),
		Value:  source,
	}
}

// parseMathStatement parses a math operation statement.
func (p *Parser) parseMathStatement() Statement {
	opToken := p.curToken()
	p.nextToken() // Skip operator

	dest := p.ParseExpression()
	if dest == nil {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected destination after '%s'", opToken.Literal),
			Position: p.curToken().Position,
		})
		return nil
	}

	if p.curToken().Type != Comma {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected ',' after destination, got %s", p.curToken()),
			Position: p.curToken().Position,
		})
		return nil
	}
	p.nextToken() // Skip ','

	arg1 := p.ParseExpression()
	if arg1 == nil {
		p.errors = append(p.errors, &Error{
			Message:  "expected first argument after ','",
			Position: p.curToken().Position,
		})
		return nil
	}

	var arg2 Expression
	if p.curToken().Type == Comma {
		p.nextToken() // Skip ','
		arg2 = p.ParseExpression()
		if arg2 == nil {
			p.errors = append(p.errors, &Error{
				Message:  "expected second argument after ','",
				Position: p.curToken().Position,
			})
			return nil
		}
	}

	// For binary operations
	var value Expression
	if arg2 != nil {
		value = &BinaryExpr{
			Token:    opToken,
			Left:     arg1,
			Operator: opToken.Type,
			Right:    arg2,
		}
	} else {
		// Unary operation or assign to self
		value = arg1
	}

	return &AssignStatement{
		Token:  opToken,
		Name:   dest.(*IdentExpr),
		Value:  value,
	}
}

// parseJumpStatement parses a jump statement.
func (p *Parser) parseJumpStatement() Statement {
	stmt := &JumpStatement{Token: p.curToken()}
	p.nextToken() // Skip "jump", "jz", or "jnz"

	if p.curToken().Type != Ident {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected label name after '%s', got %s", p.curToken().Literal, p.curToken()),
			Position: p.curToken().Position,
		})
		return nil
	}

	stmt.Target = p.curToken().Literal
	p.nextToken()

	return stmt
}

// parseLabelStatement parses a label statement.
func (p *Parser) parseLabelStatement() Statement {
	stmt := &LabelStatement{Token: p.curToken()}
	p.nextToken() // Skip "label"

	if p.curToken().Type != Ident {
		p.errors = append(p.errors, &Error{
			Message:  "expected label name after 'label'",
			Position: p.curToken().Position,
		})
		return nil
	}

	stmt.Name = p.curToken().Literal
	p.nextToken()

	return stmt
}

// parseHaltStatement parses a halt statement.
func (p *Parser) parseHaltStatement() Statement {
	p.nextToken() // Skip "halt"
	return &PrintStatement{
		Token: p.tokens[p.pos-1],
		Value: &IntExpr{Token: p.tokens[p.pos-1], Value: 0},
	}
}

// parseInputStatement parses an input statement.
func (p *Parser) parseInputStatement() Statement {
	p.nextToken() // Skip "input"

	if p.curToken().Type != Ident {
		p.errors = append(p.errors, &Error{
			Message:  "expected variable name after 'input'",
			Position: p.curToken().Position,
		})
		return nil
	}

	return &AssignStatement{
		Token: p.tokens[p.pos-2],
		Name:  &IdentExpr{Token: p.tokens[p.pos-1], Value: p.tokens[p.pos-1].Literal},
		Value: &CallExpr{Token: p.tokens[p.pos-2], Name: "input", Args: []Expression{&IntExpr{Token: p.tokens[p.pos-2], Value: 0}}},
	}
}

// parseOutputStatement parses an output statement.
func (p *Parser) parseOutputStatement() Statement {
	p.nextToken() // Skip "output"

	if p.curToken().Type != Ident {
		p.errors = append(p.errors, &Error{
			Message:  "expected variable name after 'output'",
			Position: p.curToken().Position,
		})
		return nil
	}

	return &PrintStatement{
		Token: p.tokens[p.pos-2],
		Value: &IdentExpr{Token: p.tokens[p.pos-1], Value: p.tokens[p.pos-1].Literal},
	}
}

// parseClockStatement parses a clock statement.
func (p *Parser) parseClockStatement() Statement {
	p.nextToken() // Skip "clk" or "clkSet"

	if p.curToken().Type != Ident {
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("expected variable name after '%s'", p.tokens[p.pos-1].Literal),
			Position: p.curToken().Position,
		})
		return nil
	}

	return &AssignStatement{
		Token: p.tokens[p.pos-2],
		Name:  &IdentExpr{Token: p.tokens[p.pos-1], Value: p.tokens[p.pos-1].Literal},
		Value: &CallExpr{Token: p.tokens[p.pos-2], Name: p.tokens[p.pos-2].Literal, Args: []Expression{}},
	}
}

// parseWaitStatement parses a wait statement.
func (p *Parser) parseWaitStatement() Statement {
	p.nextToken() // Skip "wait"

	var duration Expression
	if p.curToken().Type == Int || p.curToken().Type == Float {
		if p.curToken().Type == Int {
			val, _ := strconv.ParseInt(p.curToken().Literal, 10, 64)
			duration = &IntExpr{Token: p.curToken(), Value: val}
		} else {
			val, _ := strconv.ParseFloat(p.curToken().Literal, 64)
			duration = &FloatExpr{Token: p.curToken(), Value: val}
		}
		p.nextToken()
	} else {
		p.errors = append(p.errors, &Error{
			Message:  "expected number after 'wait'",
			Position: p.curToken().Position,
		})
	}

	return &PrintStatement{Token: p.tokens[p.pos-1], Value: duration}
}

// ParseExpression parses an expression.
func (p *Parser) ParseExpression() Expression {
	return p.parseLogicalOr()
}

// parseLogicalOr parses logical OR expressions.
func (p *Parser) parseLogicalOr() Expression {
	left := p.parseLogicalAnd()

	for p.curToken().Type == Or {
		op := p.curToken()
		p.nextToken() // Skip "||"
		right := p.parseLogicalAnd()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseLogicalAnd parses logical AND expressions.
func (p *Parser) parseLogicalAnd() Expression {
	left := p.parseEquality()

	for p.curToken().Type == And {
		op := p.curToken()
		p.nextToken() // Skip "&&"
		right := p.parseEquality()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseEquality parses equality comparisons.
func (p *Parser) parseEquality() Expression {
	left := p.parseComparison()

	for p.curToken().Type == Equal || p.curToken().Type == NotEqual {
		op := p.curToken()
		p.nextToken() // Skip operator
		right := p.parseComparison()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseComparison parses comparison expressions.
func (p *Parser) parseComparison() Expression {
	left := p.parseTerm()

	for p.curToken().Type == LessThan || p.curToken().Type == LessEqual ||
		p.curToken().Type == GreaterThan || p.curToken().Type == GreaterEqual {
		op := p.curToken()
		p.nextToken() // Skip operator
		right := p.parseTerm()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseTerm parses addition and subtraction.
func (p *Parser) parseTerm() Expression {
	left := p.parseFactor()

	for p.curToken().Type == Plus || p.curToken().Type == Minus {
		op := p.curToken()
		p.nextToken() // Skip operator
		right := p.parseFactor()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseFactor parses multiplication and division.
func (p *Parser) parseFactor() Expression {
	left := p.parseUnary()

	for p.curToken().Type == Multiply || p.curToken().Type == Divide ||
		p.curToken().Type == Mod {
		op := p.curToken()
		p.nextToken() // Skip operator
		right := p.parseUnary()
		left = &BinaryExpr{
			Token:    op,
			Left:     left,
			Operator: op.Type,
			Right:    right,
		}
	}

	return left
}

// parseUnary parses unary operators.
func (p *Parser) parseUnary() Expression {
	if p.curToken().Type == Not || p.curToken().Type == Minus {
		op := p.curToken()
		p.nextToken() // Skip operator

		operand := p.parseUnary()
		return &UnaryExpr{
			Token:    op,
			Operator: op.Type,
			Operand:  operand,
		}
	}

	return p.parsePrimary()
}

// parsePrimary parses primary expressions (literals, identifiers, parentheses).
func (p *Parser) parsePrimary() Expression {
	switch p.curToken().Type {
	case EOF:
		return nil

	case Int:
		val, err := strconv.ParseInt(p.curToken().Literal, 10, 64)
		if err != nil {
			p.errors = append(p.errors, &Error{
				Message:  fmt.Sprintf("invalid integer literal: %s", p.curToken().Literal),
				Position: p.curToken().Position,
			})
			val = 0
		}
		p.nextToken()
		return &IntExpr{Token: p.tokens[p.pos-1], Value: val}

	case Float:
		val, err := strconv.ParseFloat(p.curToken().Literal, 64)
		if err != nil {
			p.errors = append(p.errors, &Error{
				Message:  fmt.Sprintf("invalid floating-point literal: %s", p.curToken().Literal),
				Position: p.curToken().Position,
			})
			val = 0
		}
		p.nextToken()
		return &FloatExpr{Token: p.tokens[p.pos-1], Value: val}

	case True:
		p.nextToken()
		return &BoolExpr{Token: p.tokens[p.pos-1], Value: true}

	case False:
		p.nextToken()
		return &BoolExpr{Token: p.tokens[p.pos-1], Value: false}

	case Ident:
		val := p.curToken().Literal
		p.nextToken()

		// Check for function call
		if p.curToken().Type == LeftParen {
			p.nextToken() // Skip "("

			var args []Expression
			if p.curToken().Type != RightParen {
				for {
					arg := p.ParseExpression()
					if arg != nil {
						args = append(args, arg)
					}

					if p.curToken().Type == Comma {
						p.nextToken() // Skip ","
					} else {
						break
					}
				}
			}

			if p.curToken().Type == RightParen {
				p.nextToken() // Skip ")"
			} else {
				p.errors = append(p.errors, &Error{
					Message:  "expected ')' in function call",
					Position: p.curToken().Position,
				})
			}

			return &CallExpr{
				Token: p.tokens[p.pos-len(args)-2],
				Name:  val,
				Args:  args,
			}
		}

		return &IdentExpr{Token: p.tokens[p.pos-1], Value: val}

	case LeftParen:
		p.nextToken() // Skip "("

		expr := p.ParseExpression()
		if expr == nil {
			p.errors = append(p.errors, &Error{
				Message:  "expected expression in parentheses",
				Position: p.curToken().Position,
			})
			return nil
		}

		if p.curToken().Type == RightParen {
			p.nextToken() // Skip ")"
		} else {
			p.errors = append(p.errors, &Error{
				Message:  "expected ')' after expression",
				Position: p.curToken().Position,
			})
		}

		return expr

	default:
		p.errors = append(p.errors, &Error{
			Message:  fmt.Sprintf("unexpected token in expression: %s", p.curToken()),
			Position: p.curToken().Position,
		})
		return nil
	}
}

// Errors returns any parsing errors encountered.
func (p *Parser) Errors() []error {
	return p.errors
}

// HasErrors returns true if there are parsing errors.
func (p *Parser) HasErrors() bool {
	return len(p.errors) > 0
}

// ErrorString returns all errors as a single string.
func (p *Parser) ErrorString() string {
	var sb strings.Builder
	for i, err := range p.errors {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

// ParseProgram parses the given input string into a program.
func ParseProgram(input string) (*Program, []error) {
	tokens, err := Scan(input)
	if err != nil {
		return nil, []error{err}
	}

	parser := NewParser(tokens)
	program := parser.ParseProgram()

	if len(parser.errors) > 0 {
		return program, parser.errors
	}

	return program, nil
}

// ParseStatement parses a single statement from the given input.
func ParseStatement(input string) (Statement, []error) {
	tokens, err := Scan(input)
	if err != nil {
		return nil, []error{err}
	}

	parser := NewParser(tokens)
	stmt := parser.ParseStatement()

	if len(parser.errors) > 0 {
		return stmt, parser.errors
	}

	return stmt, nil
}

// ParseExpression parses a single expression from the given input.
func ParseExpression(input string) (Expression, []error) {
	tokens, err := Scan(input)
	if err != nil {
		return nil, []error{err}
	}

	parser := NewParser(tokens)
	expr := parser.ParseExpression()

	if len(parser.errors) > 0 {
		return expr, parser.errors
	}

	return expr, nil
}
