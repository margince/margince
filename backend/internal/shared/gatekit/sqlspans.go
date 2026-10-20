// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Stepping over the parts of a SQL statement that are not SQL.

import "strings"

// dollarTagAt returns the dollar-quote opener starting text — `$$` or
// `$ident$` — or "" when there is none. A byte scan, because callers ask at every
// position of a statement and a regexp there made the gate lane quadratic.
func dollarTagAt(text string) string {
	if len(text) < 2 || text[0] != '$' {
		return ""
	}
	if text[1] == '$' {
		return "$$"
	}
	if !identStart(text[1]) {
		return ""
	}
	for i := 2; i < len(text); i++ {
		switch {
		case text[i] == '$':
			return text[:i+1]
		case !identStart(text[i]) && (text[i] < '0' || text[i] > '9'):
			return ""
		}
	}
	return ""
}

func identStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// SQLSpanAt returns the length of the quoted or commented run starting at pos
// and whether it CLOSED, or 0 when the text there is ordinary SQL.
//
// Every census that reads a statement's structure — where a clause ends, where
// one assignment stops and the next begins, where one statement stops and the
// next begins — has to step OVER these rather than through them, and each of
// them fails differently by not doing it:
//
//   - a clause word inside a value (`SET body = 'from the report', …`) ends an
//     assignment list early, hiding every assignment after it;
//   - a comma inside a value (`'Weber, Anna'`) splits one assignment into two,
//     and the text after the comma can read as an assignment of its own — so a
//     column MENTIONED in a sentence becomes a column WRITTEN;
//   - a semicolon inside a value or a comment splits one statement into two,
//     and the fragment carrying the destruction names no table.
//
// Doubled quotes stay inside the string. Backslashes escape a character only
// when a standalone E prefix selects PostgreSQL's escape-string grammar.
// Block comments nest, so the scanner tracks their depth.
//
// A run that never closes is reported rather than swallowed, because swallowing
// it steps over the whole remainder — the under-recognition direction, which
// arrives as a green run with nothing saying the reader stopped seeing
// anything. It happens when the closing delimiter is in a piece the reader does
// not have: `"UPDATE x SET body = '" + v + "'"`. A LINE comment running to the
// end of the text is closed, not unterminated: that is where a line comment
// ends.
func SQLSpanAt(text string, pos int) (length int, closed bool) {
	switch {
	case text[pos] == '\'':
		return quotedStringSpan(text, pos)

	case strings.HasPrefix(text[pos:], "--"):
		if end := strings.IndexByte(text[pos:], '\n'); end >= 0 {
			return end + 1, true
		}
		return len(text) - pos, true
	case strings.HasPrefix(text[pos:], "/*"):
		return blockCommentSpan(text, pos)
	}
	tag := dollarTagAt(text[pos:])
	if tag == "" {
		return 0, true
	}
	if end := strings.Index(text[pos+len(tag):], tag); end >= 0 {
		return len(tag) + end + len(tag), true
	}
	return len(text) - pos, false
}

// blockCommentSpan measures a /* … */ run, counting nesting the way Postgres
// reads it: `/* a /* b */ c */` is ONE comment, and a scan stopping at the
// first `*/` would resume inside it.
func blockCommentSpan(text string, pos int) (length int, closed bool) {
	depth := 0
	for i := pos; i < len(text)-1; i++ {
		switch {
		case text[i] == '/' && text[i+1] == '*':
			depth++
			i++
		case text[i] == '*' && text[i+1] == '/':
			depth--
			i++
			if depth == 0 {
				return i - pos + 1, true
			}
		}
	}
	return len(text) - pos, false
}

func quotedStringSpan(text string, pos int) (int, bool) {
	escaped := escapeStringPrefix(text, pos)
	for i := pos + 1; i < len(text); i++ {
		if escaped && text[i] == '\\' {
			i++
			continue
		}
		if text[i] == '\'' {
			if i+1 < len(text) && text[i+1] == '\'' {
				i++
				continue
			}
			return i - pos + 1, true
		}
	}
	return len(text) - pos, false
}

func escapeStringPrefix(text string, pos int) bool {
	escaped := pos > 0 && (text[pos-1] == 'E' || text[pos-1] == 'e')
	if escaped && pos > 1 {
		before := text[pos-2]
		escaped = !(identStart(before) || before >= '0' && before <= '9' || before == '$' || before == '"' || before == '\'' || before >= 128)
	}
	return escaped
}
