package world

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type rulesTagParser struct {
	input string
	pos   int
}

func normalizeRulesTagToJSON(data []byte) ([]byte, error) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}

	// Fast path for strict JSON.
	var direct any
	if err := json.Unmarshal([]byte(text), &direct); err == nil {
		return []byte(text), nil
	}

	parser := &rulesTagParser{input: text}
	value, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	parser.skipSpace()
	if !parser.eof() {
		return nil, fmt.Errorf("unexpected trailing content at %d", parser.pos)
	}
	return json.Marshal(value)
}

func (p *rulesTagParser) parseValue() (any, error) {
	p.skipSpace()
	if p.eof() {
		return nil, fmt.Errorf("unexpected end of input")
	}

	switch p.peek() {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"', '\'':
		return p.parseString()
	default:
		token := p.parseBareToken()
		if token == "" {
			return nil, fmt.Errorf("empty token at %d", p.pos)
		}
		if token == "true" {
			return true, nil
		}
		if token == "false" {
			return false, nil
		}
		if token == "null" {
			return nil, nil
		}
		if n, ok := parseRulesNumber(token); ok {
			return n, nil
		}
		return token, nil
	}
}

func (p *rulesTagParser) parseObject() (map[string]any, error) {
	if err := p.consume('{'); err != nil {
		return nil, err
	}
	out := map[string]any{}
	p.skipSpace()
	if p.tryConsume('}') {
		return out, nil
	}
	for {
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		if err := p.consume(':'); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[key] = value
		p.skipSpace()
		if p.tryConsume('}') {
			return out, nil
		}
		if err := p.consume(','); err != nil {
			return nil, err
		}
	}
}

func (p *rulesTagParser) parseArray() ([]any, error) {
	if err := p.consume('['); err != nil {
		return nil, err
	}
	var out []any
	p.skipSpace()
	if p.tryConsume(']') {
		return out, nil
	}
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		p.skipSpace()
		if p.tryConsume(']') {
			return out, nil
		}
		if err := p.consume(','); err != nil {
			return nil, err
		}
	}
}

func (p *rulesTagParser) parseKey() (string, error) {
	p.skipSpace()
	if p.eof() {
		return "", fmt.Errorf("unexpected end of input reading key")
	}
	switch p.peek() {
	case '"', '\'':
		return p.parseString()
	default:
		token := p.parseBareToken()
		if token == "" {
			return "", fmt.Errorf("empty object key at %d", p.pos)
		}
		return token, nil
	}
}

func (p *rulesTagParser) parseString() (string, error) {
	if p.eof() {
		return "", fmt.Errorf("unexpected end of input reading string")
	}
	quote := p.peek()
	if quote != '"' && quote != '\'' {
		return "", fmt.Errorf("expected string at %d", p.pos)
	}
	p.pos++
	var out strings.Builder
	for !p.eof() {
		ch := p.peek()
		p.pos++
		if ch == quote {
			return out.String(), nil
		}
		if ch != '\\' {
			out.WriteByte(ch)
			continue
		}
		if p.eof() {
			return "", fmt.Errorf("unterminated escape at %d", p.pos)
		}
		esc := p.peek()
		p.pos++
		switch esc {
		case '"', '\'', '\\', '/':
			out.WriteByte(esc)
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'u':
			if p.pos+4 > len(p.input) {
				return "", fmt.Errorf("short unicode escape at %d", p.pos)
			}
			raw := p.input[p.pos : p.pos+4]
			p.pos += 4
			code, err := strconv.ParseUint(raw, 16, 16)
			if err != nil {
				return "", fmt.Errorf("invalid unicode escape %q: %w", raw, err)
			}
			out.WriteRune(rune(code))
		default:
			out.WriteByte(esc)
		}
	}
	return "", fmt.Errorf("unterminated string")
}

func (p *rulesTagParser) parseBareToken() string {
	start := p.pos
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\r', '\n', ':', ',', '{', '}', '[', ']':
			return p.input[start:p.pos]
		default:
			p.pos++
		}
	}
	return p.input[start:p.pos]
}

func parseRulesNumber(token string) (any, bool) {
	if token == "" {
		return nil, false
	}
	if strings.ContainsAny(token, ".eE") {
		v, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return nil, false
		}
		return v, true
	}
	v, err := strconv.ParseInt(token, 10, 64)
	if err == nil {
		return v, true
	}
	u, err := strconv.ParseUint(token, 10, 64)
	if err == nil {
		return u, true
	}
	return nil, false
}

func (p *rulesTagParser) skipSpace() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *rulesTagParser) consume(want byte) error {
	p.skipSpace()
	if p.eof() {
		return fmt.Errorf("expected %q at end of input", want)
	}
	if p.peek() != want {
		return fmt.Errorf("expected %q at %d, got %q", want, p.pos, p.peek())
	}
	p.pos++
	return nil
}

func (p *rulesTagParser) tryConsume(want byte) bool {
	p.skipSpace()
	if p.eof() || p.peek() != want {
		return false
	}
	p.pos++
	return true
}

func (p *rulesTagParser) peek() byte {
	return p.input[p.pos]
}

func (p *rulesTagParser) eof() bool {
	return p.pos >= len(p.input)
}
