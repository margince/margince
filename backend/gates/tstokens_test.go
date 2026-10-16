// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// A TypeScript tokenizer for the gates that judge object literals in the
// frontend. Comments, regular expressions and JSX text become no tokens, so a
// quote inside one cannot open a string, and a template literal is one token.
// JSX expression containers keep their braces and their tokens.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type tsTokenKind int

const (
	tkIdent tsTokenKind = iota
	tkString
	tkTemplate
	tkNumber
	tkPunct
)

// tsToken is one token. A template's text is its value only when it holds no
// substitution; computed reports one that does.
type tsToken struct {
	kind     tsTokenKind
	text     string
	computed bool
	pos      int
}

// tsPunctuators are tried longest first.
var tsPunctuators = []string{
	">>>=", "...", "===", "!==", "**=", "<<=", ">>=", ">>>", "&&=", "||=", "??=",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "++", "--", "+=", "-=", "*=", "/=", "%=",
	"&=", "|=", "^=", "**", "<<", ">>",
}

type tsLexer struct {
	src    string
	pos    int
	jsx    bool
	tokens []tsToken
}

// tokenizeTS reads source as TypeScript, and as TSX when jsx is set. An error
// names the line it could not read past.
func tokenizeTS(source string, jsx bool) ([]tsToken, error) {
	lexer := &tsLexer{src: source, jsx: jsx}
	if err := lexer.code(false); err != nil {
		return nil, fmt.Errorf("line %d: %w", lineAt(source, lexer.pos), err)
	}
	return lexer.tokens, nil
}

func lineAt(source string, pos int) int {
	return strings.Count(source[:min(pos, len(source))], "\n") + 1
}

// code reads tokens until the end of input, or, when nested, until the brace
// that closes the expression it was called for.
func (l *tsLexer) code(nested bool) error {
	depth := 0
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			l.pos++
		case strings.HasPrefix(l.src[l.pos:], "//"):
			l.skipLine()
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			end := strings.Index(l.src[l.pos+2:], "*/")
			if end < 0 {
				return errors.New("a block comment never closes")
			}
			l.pos += end + 4
		case ch == '"' || ch == '\'':
			if err := l.quoted(ch); err != nil {
				return err
			}
		case ch == '`':
			if err := l.template(); err != nil {
				return err
			}
		case ch == '/' && l.operandExpected():
			if err := l.regex(); err != nil {
				return err
			}
		case ch == '<' && l.jsx && l.operandExpected() && l.opensElement():
			if err := l.element(); err != nil {
				return err
			}
		case isIdentStart(ch):
			l.word(tkIdent)
		case ch >= '0' && ch <= '9' || ch == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1]):
			l.word(tkNumber)
		case ch == '{':
			depth++
			l.punct("{")
		case ch == '}':
			if nested && depth == 0 {
				l.pos++
				return nil
			}
			depth--
			l.punct("}")
		default:
			l.punctuator()
		}
	}
	if nested {
		return errors.New("an expression inside a template or JSX never closes")
	}
	return nil
}

func (l *tsLexer) skipLine() {
	end := strings.IndexByte(l.src[l.pos:], '\n')
	if end < 0 {
		l.pos = len(l.src)
		return
	}
	l.pos += end
}

func (l *tsLexer) punct(text string) {
	l.tokens = append(l.tokens, tsToken{kind: tkPunct, text: text, pos: l.pos})
	l.pos += len(text)
}

func (l *tsLexer) punctuator() {
	for _, candidate := range tsPunctuators {
		if strings.HasPrefix(l.src[l.pos:], candidate) {
			l.punct(candidate)
			return
		}
	}
	l.punct(l.src[l.pos : l.pos+1])
}

func (l *tsLexer) word(kind tsTokenKind) {
	start := l.pos
	for l.pos < len(l.src) && (isIdentPart(l.src[l.pos]) || kind == tkNumber && l.src[l.pos] == '.') {
		l.pos++
	}
	l.tokens = append(l.tokens, tsToken{kind: kind, text: l.src[start:l.pos], pos: start})
}

func isIdentStart(ch byte) bool {
	return ch == '_' || ch == '$' || ch == '#' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= 0x80
}

func isIdentPart(ch byte) bool { return isIdentStart(ch) || isDigit(ch) }

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

// operandExpected reports whether the next token starts an operand, which is
// what tells a regular expression from a division and JSX from a comparison.
func (l *tsLexer) operandExpected() bool {
	if len(l.tokens) == 0 {
		return true
	}
	last := l.tokens[len(l.tokens)-1]
	switch last.kind {
	case tkString, tkTemplate, tkNumber:
		return false
	case tkIdent:
		return isKeyword(last.text)
	}
	return last.text != ")" && last.text != "]"
}

// isKeyword reports the words after which an operand follows.
func isKeyword(word string) bool {
	switch word {
	case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "throw", "case",
		"do", "else", "yield", "await", "default", "extends":
		return true
	}
	return false
}

// quoted reads a single- or double-quoted string, decoding its escapes.
func (l *tsLexer) quoted(quote byte) error {
	start := l.pos
	var text strings.Builder
	for l.pos++; l.pos < len(l.src); l.pos++ {
		ch := l.src[l.pos]
		switch {
		case ch == quote:
			l.pos++
			l.tokens = append(l.tokens, tsToken{kind: tkString, text: text.String(), pos: start})
			return nil
		case ch == '\n':
			return errors.New("a string never closes")
		case ch == '\\' && l.pos+1 < len(l.src):
			l.pos++
			text.WriteString(tsEscape(l.src[l.pos]))
		default:
			text.WriteByte(ch)
		}
	}
	return errors.New("a string never closes")
}

// tsEscape decodes the escapes a code would carry; any other escaped
// character stands for itself.
func tsEscape(ch byte) string {
	switch ch {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case '\n':
		return ""
	}
	return string(ch)
}

// template reads a template literal as one token. A substitution's tokens
// are read and dropped, so its braces cannot unbalance the code around it.
func (l *tsLexer) template() error {
	start := l.pos
	var text strings.Builder
	computed := false
	for l.pos++; l.pos < len(l.src); l.pos++ {
		ch := l.src[l.pos]
		switch {
		case ch == '`':
			l.pos++
			l.tokens = append(l.tokens, tsToken{kind: tkTemplate, text: text.String(), computed: computed, pos: start})
			return nil
		case ch == '\\' && l.pos+1 < len(l.src):
			l.pos++
			text.WriteString(tsEscape(l.src[l.pos]))
		case strings.HasPrefix(l.src[l.pos:], "${"):
			computed = true
			kept := len(l.tokens)
			l.pos += 2
			if err := l.code(true); err != nil {
				return err
			}
			l.tokens = l.tokens[:kept]
			l.pos--
		default:
			text.WriteByte(ch)
		}
	}
	return errors.New("a template literal never closes")
}

func (l *tsLexer) regex() error {
	inClass := false
	for l.pos++; l.pos < len(l.src); l.pos++ {
		switch ch := l.src[l.pos]; {
		case ch == '\\':
			l.pos++
		case ch == '\n':
			return errors.New("a regular expression never closes")
		case ch == '[':
			inClass = true
		case ch == ']':
			inClass = false
		case ch == '/' && !inClass:
			l.pos++
			for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
				l.pos++
			}
			// A regular expression is an operand, which this token records.
			l.tokens = append(l.tokens, tsToken{kind: tkNumber, text: "/regex/", pos: l.pos})
			return nil
		}
	}
	return errors.New("a regular expression never closes")
}

// opensElement reports a `<` followed by a tag name or a fragment's `>`.
func (l *tsLexer) opensElement() bool {
	if l.pos+1 >= len(l.src) {
		return false
	}
	next := l.src[l.pos+1]
	return next == '>' || isIdentStart(next) && next != '#'
}

// element reads one JSX element, its attributes and its children. Text is
// dropped; an expression container keeps its braces and tokens.
func (l *tsLexer) element() error {
	l.pos++
	if l.pos < len(l.src) && l.src[l.pos] == '>' {
		l.pos++
		return l.children("")
	}
	name := l.jsxName()
	if err := l.skipTypeArguments(); err != nil {
		return err
	}
	for {
		l.skipSpace()
		if l.pos >= len(l.src) {
			return fmt.Errorf("JSX element <%s> never closes its tag", name)
		}
		switch {
		case strings.HasPrefix(l.src[l.pos:], "//"):
			l.skipLine()
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			end := strings.Index(l.src[l.pos+2:], "*/")
			if end < 0 {
				return errors.New("a block comment never closes")
			}
			l.pos += end + 4
		case strings.HasPrefix(l.src[l.pos:], "/>"):
			l.pos += 2
			l.endOperand()
			return nil
		case l.src[l.pos] == '>':
			l.pos++
			return l.children(name)
		case l.src[l.pos] == '{':
			if err := l.container(); err != nil {
				return err
			}
		case l.src[l.pos] == '"' || l.src[l.pos] == '\'':
			end := strings.IndexByte(l.src[l.pos+1:], l.src[l.pos])
			if end < 0 {
				return errors.New("a JSX attribute string never closes")
			}
			l.pos += end + 2
		case l.src[l.pos] == '=':
			l.pos++
		default:
			if l.jsxName() == "" {
				return fmt.Errorf("JSX element <%s> holds %q where an attribute belongs", name, l.src[l.pos])
			}
		}
	}
}

// children reads up to and through the closing tag of name.
func (l *tsLexer) children(name string) error {
	for l.pos < len(l.src) {
		switch {
		case strings.HasPrefix(l.src[l.pos:], "</"):
			l.pos += 2
			closing := l.jsxName()
			l.skipSpace()
			if closing != name || l.pos >= len(l.src) || l.src[l.pos] != '>' {
				return fmt.Errorf("JSX element <%s> is closed by </%s>", name, closing)
			}
			l.pos++
			l.endOperand()
			return nil
		case l.src[l.pos] == '<':
			if err := l.element(); err != nil {
				return err
			}
		case l.src[l.pos] == '{':
			if err := l.container(); err != nil {
				return err
			}
		default:
			l.pos++
		}
	}
	return fmt.Errorf("JSX element <%s> never closes", name)
}

// container reads `{ expression }` inside JSX, keeping both braces.
func (l *tsLexer) container() error {
	l.punct("{")
	if err := l.code(true); err != nil {
		return err
	}
	l.tokens = append(l.tokens, tsToken{kind: tkPunct, text: "}", pos: l.pos - 1})
	return nil
}

// endOperand marks the end of an element, which is an operand.
func (l *tsLexer) endOperand() {
	l.tokens = append(l.tokens, tsToken{kind: tkNumber, text: "<jsx/>", pos: l.pos})
}

// skipTypeArguments passes over a generic component's `<Row>`.
func (l *tsLexer) skipTypeArguments() error {
	if l.pos >= len(l.src) || l.src[l.pos] != '<' {
		return nil
	}
	depth := 0
	for ; l.pos < len(l.src); l.pos++ {
		switch l.src[l.pos] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				l.pos++
				return nil
			}
		}
	}
	return errors.New("a JSX element's type arguments never close")
}

func (l *tsLexer) jsxName() string {
	start := l.pos
	for l.pos < len(l.src) && (isIdentPart(l.src[l.pos]) || strings.IndexByte(".:-", l.src[l.pos]) >= 0) {
		l.pos++
	}
	return l.src[start:l.pos]
}

func (l *tsLexer) skipSpace() {
	for l.pos < len(l.src) && strings.IndexByte(" \t\r\n", l.src[l.pos]) >= 0 {
		l.pos++
	}
}

// unquotedNumber reads a numeric token as an integer, for a status.
func unquotedNumber(token tsToken) (int, bool) {
	if token.kind != tkNumber {
		return 0, false
	}
	value, err := strconv.Atoi(strings.ReplaceAll(token.text, "_", ""))
	return value, err == nil
}
