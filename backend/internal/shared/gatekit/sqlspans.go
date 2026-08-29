// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Stepping over the parts of a SQL statement that are not SQL.

import (
	"regexp"
	"strings"
)

// dollarTagRe matches a dollar-quote opener at the current position.
var dollarTagRe = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*\$|^\$\$`)

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
// A doubled ” inside a string needs no case of its own: the run ends at the
// first quote, the next character opens a new run, and that one closes where
// the real one does. Block comments NEST, which Postgres allows and a naive
// scan to the first `*/` gets wrong.
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
		for i := pos + 1; i < len(text); i++ {
			if text[i] == '\'' {
				return i - pos + 1, true
			}
		}
		return len(text) - pos, false
	case strings.HasPrefix(text[pos:], "--"):
		if end := strings.IndexByte(text[pos:], '\n'); end >= 0 {
			return end + 1, true
		}
		return len(text) - pos, true
	case strings.HasPrefix(text[pos:], "/*"):
		return blockCommentSpan(text, pos)
	}
	tag := dollarTagRe.FindString(text[pos:])
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
