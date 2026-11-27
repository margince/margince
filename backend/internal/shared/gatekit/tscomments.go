// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Reading TypeScript with its comments blanked, for the censuses that match
// patterns over the frontend's source.
//
// A regular expression cannot do it. A `//` inside a URL string starts no
// comment, and a quote inside a regex literal opens no string. A comment
// becomes spaces with its newlines kept, so a match keeps its line and offset.
// A block comment that never closes is left as it is. It is most often a
// census's own cut through one, and that cut should read as a fault.

import "strings"

// TSWithoutComments blanks every comment outside a string or regex literal.
func TSWithoutComments(source string) string {
	code, _ := scanTS(source)
	return code
}

// TSCode is TSWithoutComments with the text of every string literal it
// passed, in source order, so a census reads both from one pass.
func TSCode(source string) (code string, literals []string) {
	return scanTS(source)
}

func scanTS(source string) (string, []string) {
	code := []byte(source)
	var literals []string
	// operand says the last token ends an operand, so a `/` after it divides.
	operand := false
	for i := 0; i < len(code); {
		if end, isComment := tsCommentEnd(source, i); isComment {
			blankKeepingLines(code[i:end])
			i = end
			continue
		}
		c := code[i]
		switch {
		case c == '"' || c == '\'' || c == '`':
			end := TSLiteralEnd(source, i)
			literals = append(literals, source[i+1:max(i+1, end)])
			i, operand = end+1, true
		case c == '/' && !operand:
			end, isRegex := tsRegexEnd(source, i)
			i, operand = end+1, isRegex
		case isTSWordByte(c):
			start := i
			i = tsWordEnd(source, i)
			operand = !tsExpressionKeywords[source[start:i]]
		default:
			if !isTSSpace(c) {
				operand = c == ')' || c == ']' || c == '}'
			}
			i++
		}
	}
	return string(code), literals
}

// tsCommentEnd is the offset just past a comment that opens at start, and
// whether one does. A block comment that never closes opens none.
func tsCommentEnd(source string, start int) (int, bool) {
	if source[start] != '/' || start+1 >= len(source) {
		return start, false
	}
	switch source[start+1] {
	case '/':
		if end := strings.IndexByte(source[start:], '\n'); end >= 0 {
			return start + end, true
		}
		return len(source), true
	case '*':
		if end := strings.Index(source[start+2:], "*/"); end >= 0 {
			return start + 2 + end + 2, true
		}
	}
	return start, false
}

// TSLiteralEnd is the offset of the quote closing the literal opened at start.
// A quote or apostrophe that never closes ends at its line's end, as
// TypeScript ends it; a template literal may span lines.
func TSLiteralEnd(source string, start int) int {
	quote := source[start]
	for i := start + 1; i < len(source); i++ {
		switch {
		case source[i] == '\\':
			i++
		case source[i] == quote:
			return i
		case source[i] == '\n' && quote != '`':
			return i
		}
	}
	return len(source) - 1
}

// tsRegexEnd is the offset of the slash closing a regex literal opened at
// start. A slash with no closing one on its line is division, so it answers
// start and false.
func tsRegexEnd(source string, start int) (int, bool) {
	inClass := false
	for i := start + 1; i < len(source); i++ {
		switch c := source[i]; {
		case c == '\\':
			i++
		case c == '\n':
			return start, false
		case inClass:
			inClass = c != ']'
		case c == '[':
			inClass = true
		case c == '/':
			return i, true
		}
	}
	return start, false
}

// tsExpressionKeywords are the words an expression may follow, so a `/` after
// one starts a regex literal rather than dividing.
var tsExpressionKeywords = map[string]bool{
	"return": true, "typeof": true, "instanceof": true, "in": true, "of": true, "new": true,
	"delete": true, "void": true, "throw": true, "case": true, "do": true, "else": true,
	"yield": true, "await": true,
}

func blankKeepingLines(text []byte) {
	for i, c := range text {
		if c != '\n' {
			text[i] = ' '
		}
	}
}

func tsWordEnd(source string, start int) int {
	end := start
	for end < len(source) && isTSWordByte(source[end]) {
		end++
	}
	return end
}

func isTSWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 ||
		('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

func isTSSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
