// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailsubject holds the prefixes a mail client puts in front of a
// subject line, so the draft checker and the "was this answered" query read
// one list.
//
// Reply and forward prefixes are kept apart on purpose. A reply to a sender
// with their own subject answers them; a forward to them passes something on
// and answers nothing, so only the reply list may count as an answer.
//
// Stdlib only, which the shared tier requires.
package mailsubject

import (
	"regexp"
	"strings"
	"unicode"
)

// ReplyPrefixes are the ways a client marks a subject as a reply, in the
// languages this product writes. Lower case, colon included.
func ReplyPrefixes() []string {
	return []string{"re:", "aw:", "antw:"}
}

// ForwardPrefixes are the ways a client marks a subject as a forward.
func ForwardPrefixes() []string {
	return []string{"fwd:", "wg:"}
}

// ReplyPrefixPattern matches any run of reply prefixes at the start of a
// subject, with the spaces around them. It reads the same in Go's regexp and
// in a Postgres regular expression, so SQL can strip what ReplyPrefixes
// names without a second copy of the list.
func ReplyPrefixPattern() string {
	words := make([]string, 0, len(ReplyPrefixes()))
	for _, prefix := range ReplyPrefixes() {
		words = append(words, strings.TrimSuffix(prefix, ":"))
	}
	return `^(\s*(` + strings.Join(words, "|") + `)\s*:)+\s*`
}

var (
	replyPrefixes = regexp.MustCompile(`(?i)` + ReplyPrefixPattern())
	innerSpaces   = regexp.MustCompile(`\s+`)
)

// Normalized is a subject as the answer check compares it: reply prefixes and
// outer spaces removed, inner runs of space folded, lower-cased. A forward
// prefix stays. The SQL in activities/answered.go (normalisedSubject) spells
// the same steps over the same pattern; TestNormalizedMatchesTheAnswerCheck
// holds the two to one answer.
func Normalized(subject string) string {
	subject = strings.Map(func(r rune) rune {
		if isPostgresSpace(r) {
			return ' '
		}
		return r
	}, subject)
	stripped := replyPrefixes.ReplaceAllString(subject, "")
	return strings.ToLower(strings.Trim(innerSpaces.ReplaceAllString(stripped, " "), " "))
}

// isPostgresSpace is what the database's \s matches: Unicode white space
// except the no-break spaces, which its locale does not class as space. Go's
// own \s is ASCII only, so every such rune is made a plain space before the
// shared pattern runs, and btrim's plain-space trim follows.
func isPostgresSpace(r rune) bool {
	switch r {
	case '\u00a0', '\u2007', '\u202f':
		return false
	}
	return unicode.IsSpace(r)
}
