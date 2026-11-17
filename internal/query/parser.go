package query

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	maxFilterBytes = 16 * 1024
	maxConditions  = 64
	maxInList      = 200
	maxDepth       = 32
)

// ParseError describes a syntax or complexity error in a filter.
type ParseError struct {
	Pos     int    `json:"pos"`
	Message string `json:"message"`
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("filter error at byte %d: %s", e.Pos, e.Message)
}

type parser struct {
	lex        *lexer
	cur        token
	err        error
	conditions int
	depth      int
}

// Parse compiles a filter string into an AST. An empty filter returns nil.
func Parse(input string) (Node, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	if len(input) > maxFilterBytes {
		return nil, &ParseError{Message: fmt.Sprintf("filter exceeds %d bytes", maxFilterBytes)}
	}
	p := &parser{lex: &lexer{input: input}}
	p.advance()
	if p.err != nil {
		return nil, p.err
	}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.err != nil {
		return nil, p.err
	}
	if p.cur.typ != tokEOF {
		return nil, p.errorf("unexpected %s after expression", p.cur)
	}
	return n, nil
}

func (p *parser) advance() {
	if p.err != nil {
		return
	}
	t, err := p.lex.next()
	if err != nil {
		p.err = err
		return
	}
	p.cur = t
}

func (p *parser) errorf(format string, args ...any) error {
	return &ParseError{Pos: p.cur.pos, Message: fmt.Sprintf(format, args...)}
}

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur.typ == tokOr {
		op := "||"
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &Logical{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur.typ == tokAnd {
		op := "&&"
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &Logical{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Node, error) {
	if p.cur.typ == tokNot {
		p.depth++
		if p.depth > maxDepth {
			return nil, p.errorf("expression is too deeply nested (max %d)", maxDepth)
		}
		defer func() { p.depth-- }()
		p.advance()
		n, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Not{X: n}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Node, error) {
	if p.cur.typ == tokLParen {
		p.depth++
		if p.depth > maxDepth {
			return nil, p.errorf("expression is too deeply nested (max %d)", maxDepth)
		}
		defer func() { p.depth-- }()
		p.advance()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur.typ != tokRParen {
			return nil, p.errorf("expected ')' but found %s", p.cur)
		}
		p.advance()
		return n, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (Node, error) {
	if p.cur.typ != tokIdent {
		return nil, p.errorf("expected a field path but found %s", p.cur)
	}
	path := []string{p.cur.text}
	p.advance()
	for p.cur.typ == tokDot {
		p.advance()
		if p.cur.typ != tokIdent {
			return nil, p.errorf("expected field name after '.' but found %s", p.cur)
		}
		path = append(path, p.cur.text)
		p.advance()
	}

	p.conditions++
	if p.conditions > maxConditions {
		return nil, p.errorf("too many conditions (max %d)", maxConditions)
	}

	var op string
	switch p.cur.typ {
	case tokEq:
		op = "="
	case tokNe:
		op = "!="
	case tokGt:
		op = ">"
	case tokGe:
		op = ">="
	case tokLt:
		op = "<"
	case tokLe:
		op = "<="
	case tokTilde:
		op = "~"
	case tokIn:
		op = "in"
	case tokContains:
		op = "contains"
	default:
		return nil, p.errorf("expected a comparison operator after %s, found %s", strings.Join(path, "."), p.cur)
	}
	p.advance()

	if op == "in" {
		list, err := p.parseList()
		if err != nil {
			return nil, err
		}
		return newComparison(path, op, list), nil
	}
	v, err := p.parseValue(false)
	if err != nil {
		return nil, err
	}
	return newComparison(path, op, v), nil
}

func newComparison(path []string, op string, v Value) *Comparison {
	c := &Comparison{Path: path, Op: op, Value: v, pathStr: strings.Join(path, ".")}
	if !valueHasVars(v) {
		c.staticValue = resolveValue(v, nil)
		c.isStatic = true
	}
	return c
}

func valueHasVars(v Value) bool {
	switch v.Kind {
	case ValueVariable:
		return true
	case ValueList:
		for _, item := range v.Items {
			if valueHasVars(item) {
				return true
			}
		}
	}
	return false
}

func (p *parser) parseValue(allowList bool) (Value, error) {
	switch p.cur.typ {
	case tokString:
		v := Value{Kind: ValueString, Str: p.cur.text}
		p.advance()
		return v, nil
	case tokNumber:
		f, err := strconv.ParseFloat(p.cur.text, 64)
		if err != nil {
			return Value{}, p.errorf("invalid number %q", p.cur.text)
		}
		v := Value{Kind: ValueNumber, Num: f}
		p.advance()
		return v, nil
	case tokTrue:
		p.advance()
		return Value{Kind: ValueBool, Bool: true}, nil
	case tokFalse:
		p.advance()
		return Value{Kind: ValueBool}, nil
	case tokNull:
		p.advance()
		return Value{Kind: ValueNull}, nil
	case tokVariable:
		parts := strings.Split(p.cur.text, ".")
		if parts[0] != "now" && parts[0] != "auth" {
			return Value{}, p.errorf("unknown variable @%s (available: @now, @auth)", p.cur.text)
		}
		if parts[0] == "now" && len(parts) != 1 {
			return Value{}, p.errorf("@now does not have fields")
		}
		for _, part := range parts {
			if part == "" {
				return Value{}, p.errorf("invalid variable @%s", p.cur.text)
			}
		}
		v := Value{Kind: ValueVariable, Vars: parts}
		p.advance()
		return v, nil
	case tokLBracket:
		if !allowList {
			return Value{}, p.errorf("lists are only allowed with the 'in' operator")
		}
		return p.parseList()
	default:
		return Value{}, p.errorf("expected a value but found %s", p.cur)
	}
}

func (p *parser) parseList() (Value, error) {
	if p.cur.typ != tokLBracket {
		return Value{}, p.errorf("expected '[' but found %s", p.cur)
	}
	p.advance()
	list := Value{Kind: ValueList}
	if p.cur.typ == tokRBracket {
		p.advance()
		return list, nil
	}
	for {
		if len(list.Items) >= maxInList {
			return Value{}, p.errorf("list has too many items (max %d)", maxInList)
		}
		item, err := p.parseValue(false)
		if err != nil {
			return Value{}, err
		}
		list.Items = append(list.Items, item)
		if p.cur.typ == tokComma {
			p.advance()
			continue
		}
		break
	}
	if p.cur.typ != tokRBracket {
		return Value{}, p.errorf("expected ']' but found %s", p.cur)
	}
	p.advance()
	return list, nil
}
