package query

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenType int

const (
	tokEOF tokenType = iota
	tokIdent
	tokString
	tokNumber
	tokVariable
	tokTrue
	tokFalse
	tokNull
	tokEq
	tokNe
	tokGt
	tokGe
	tokLt
	tokLe
	tokTilde
	tokAnd
	tokOr
	tokNot
	tokIn
	tokContains
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokComma
	tokDot
)

type token struct {
	typ  tokenType
	text string
	pos  int
}

func (t token) String() string {
	if t.typ == tokEOF {
		return "end of filter"
	}
	if t.text == "" {
		return t.typ.name()
	}
	return fmt.Sprintf("%q", t.text)
}

func (t tokenType) name() string {
	switch t {
	case tokEOF:
		return "end of filter"
	case tokIdent:
		return "identifier"
	case tokString:
		return "string"
	case tokNumber:
		return "number"
	case tokVariable:
		return "variable"
	case tokTrue:
		return "true"
	case tokFalse:
		return "false"
	case tokNull:
		return "null"
	case tokEq:
		return "="
	case tokNe:
		return "!="
	case tokGt:
		return ">"
	case tokGe:
		return ">="
	case tokLt:
		return "<"
	case tokLe:
		return "<="
	case tokTilde:
		return "~"
	case tokAnd:
		return "&&"
	case tokOr:
		return "||"
	case tokNot:
		return "!"
	case tokIn:
		return "in"
	case tokContains:
		return "contains"
	case tokLParen:
		return "("
	case tokRParen:
		return ")"
	case tokLBracket:
		return "["
	case tokRBracket:
		return "]"
	case tokComma:
		return ","
	case tokDot:
		return "."
	default:
		return "unknown token"
	}
}

type lexer struct {
	input string
	pos   int
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.input) {
		r, size := utf8.DecodeRuneInString(l.input[l.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		l.pos += size
	}
	start := l.pos
	if l.pos >= len(l.input) {
		return token{typ: tokEOF, pos: start}, nil
	}
	c := l.input[l.pos]
	switch c {
	case '(':
		l.pos++
		return token{typ: tokLParen, text: "(", pos: start}, nil
	case ')':
		l.pos++
		return token{typ: tokRParen, text: ")", pos: start}, nil
	case '[':
		l.pos++
		return token{typ: tokLBracket, text: "[", pos: start}, nil
	case ']':
		l.pos++
		return token{typ: tokRBracket, text: "]", pos: start}, nil
	case ',':
		l.pos++
		return token{typ: tokComma, text: ",", pos: start}, nil
	case '.':
		l.pos++
		return token{typ: tokDot, text: ".", pos: start}, nil
	case '&':
		if strings.HasPrefix(l.input[l.pos:], "&&") {
			l.pos += 2
			return token{typ: tokAnd, text: "&&", pos: start}, nil
		}
		return token{}, l.errorf(start, "expected &&")
	case '|':
		if strings.HasPrefix(l.input[l.pos:], "||") {
			l.pos += 2
			return token{typ: tokOr, text: "||", pos: start}, nil
		}
		return token{}, l.errorf(start, "expected ||")
	case '!':
		if strings.HasPrefix(l.input[l.pos:], "!=") {
			l.pos += 2
			return token{typ: tokNe, text: "!=", pos: start}, nil
		}
		l.pos++
		return token{typ: tokNot, text: "!", pos: start}, nil
	case '=':
		if strings.HasPrefix(l.input[l.pos:], "==") {
			l.pos += 2
		} else {
			l.pos++
		}
		return token{typ: tokEq, text: "=", pos: start}, nil
	case '>':
		if strings.HasPrefix(l.input[l.pos:], ">=") {
			l.pos += 2
			return token{typ: tokGe, text: ">=", pos: start}, nil
		}
		l.pos++
		return token{typ: tokGt, text: ">", pos: start}, nil
	case '<':
		if strings.HasPrefix(l.input[l.pos:], "<=") {
			l.pos += 2
			return token{typ: tokLe, text: "<=", pos: start}, nil
		}
		l.pos++
		return token{typ: tokLt, text: "<", pos: start}, nil
	case '~':
		l.pos++
		return token{typ: tokTilde, text: "~", pos: start}, nil
	case '@':
		l.pos++
		begin := l.pos
		for l.pos < len(l.input) {
			b := l.input[l.pos]
			if isIdentByte(b) || b == '.' {
				l.pos++
				continue
			}
			break
		}
		if begin == l.pos {
			return token{}, l.errorf(start, "expected variable name after @")
		}
		return token{typ: tokVariable, text: l.input[begin:l.pos], pos: start}, nil
	case '"', '\'':
		return l.lexString(c, start)
	}
	if c == '-' || (c >= '0' && c <= '9') {
		return l.lexNumber(start)
	}
	if isIdentStart(c) {
		begin := l.pos
		for l.pos < len(l.input) && isIdentByte(l.input[l.pos]) {
			l.pos++
		}
		word := l.input[begin:l.pos]
		switch strings.ToLower(word) {
		case "true":
			return token{typ: tokTrue, text: word, pos: start}, nil
		case "false":
			return token{typ: tokFalse, text: word, pos: start}, nil
		case "null":
			return token{typ: tokNull, text: word, pos: start}, nil
		case "in":
			return token{typ: tokIn, text: word, pos: start}, nil
		case "contains":
			return token{typ: tokContains, text: word, pos: start}, nil
		case "and":
			return token{typ: tokAnd, text: word, pos: start}, nil
		case "or":
			return token{typ: tokOr, text: word, pos: start}, nil
		case "not":
			return token{typ: tokNot, text: word, pos: start}, nil
		}
		return token{typ: tokIdent, text: word, pos: start}, nil
	}
	return token{}, l.errorf(start, "unexpected character %q", string(c))
}

func (l *lexer) lexString(quote byte, start int) (token, error) {
	l.pos++ // opening quote
	var b strings.Builder
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == quote {
			l.pos++
			return token{typ: tokString, text: b.String(), pos: start}, nil
		}
		if c == '\\' {
			l.pos++
			if l.pos >= len(l.input) {
				return token{}, l.errorf(start, "unterminated escape sequence")
			}
			esc := l.input[l.pos]
			l.pos++
			switch esc {
			case '"', '\'', '\\', '/':
				b.WriteByte(esc)
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'u':
				if l.pos+4 > len(l.input) {
					return token{}, l.errorf(start, "invalid unicode escape")
				}
				n, err := strconv.ParseUint(l.input[l.pos:l.pos+4], 16, 16)
				if err != nil {
					return token{}, l.errorf(start, "invalid unicode escape")
				}
				l.pos += 4
				r := rune(n)
				if utf16IsHighSurrogate(r) && l.pos+6 <= len(l.input) && l.input[l.pos] == '\\' && l.input[l.pos+1] == 'u' {
					n2, err := strconv.ParseUint(l.input[l.pos+2:l.pos+6], 16, 16)
					if err == nil && utf16IsLowSurrogate(rune(n2)) {
						r = utf16Decode(r, rune(n2))
						l.pos += 6
					}
				}
				b.WriteRune(r)
			default:
				return token{}, l.errorf(start, "unknown escape sequence \\%c", esc)
			}
			continue
		}
		b.WriteByte(c)
		l.pos++
	}
	return token{}, l.errorf(start, "unterminated string")
}

func (l *lexer) lexNumber(start int) (token, error) {
	begin := l.pos
	if l.input[l.pos] == '-' {
		l.pos++
		if l.pos >= len(l.input) || !isDigit(l.input[l.pos]) {
			return token{}, l.errorf(start, "invalid number")
		}
	}
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		if l.pos >= len(l.input) || !isDigit(l.input[l.pos]) {
			return token{}, l.errorf(start, "invalid number")
		}
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}
	if l.pos < len(l.input) && (l.input[l.pos] == 'e' || l.input[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '+' || l.input[l.pos] == '-') {
			l.pos++
		}
		if l.pos >= len(l.input) || !isDigit(l.input[l.pos]) {
			return token{}, l.errorf(start, "invalid number exponent")
		}
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}
	return token{typ: tokNumber, text: l.input[begin:l.pos], pos: start}, nil
}

func (l *lexer) errorf(pos int, format string, args ...any) error {
	return &ParseError{Pos: pos, Message: fmt.Sprintf(format, args...)}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentByte(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func utf16IsHighSurrogate(r rune) bool { return r >= 0xD800 && r <= 0xDBFF }
func utf16IsLowSurrogate(r rune) bool  { return r >= 0xDC00 && r <= 0xDFFF }
func utf16Decode(hi, lo rune) rune {
	return ((hi - 0xD800) << 10) + (lo - 0xDC00) + 0x10000
}
