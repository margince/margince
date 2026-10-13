// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package formulacell quotes a CSV cell a spreadsheet would evaluate as a formula.
// Unguard removes that quote on import, and Unguard(Guard(s)) == s for any s.
package formulacell

import "strings"

const guardQuote = '\''

// Guard looks past quotes s already opens with, so a stored `'=x` stays distinct
// from a guarded `=x`. A bare phone number can only evaluate to itself.
func Guard(s string) string {
	if !opensOnLead(strings.TrimLeft(s, string(guardQuote))) || isBarePhone(s) {
		return s
	}
	return string(guardQuote) + s
}

// Unguard keeps a leading quote no formula lead follows: it was typed, not guarded.
func Unguard(s string) string {
	rest, quoted := strings.CutPrefix(s, string(guardQuote))
	if !quoted || !opensOnLead(strings.TrimLeft(rest, string(guardQuote))) {
		return s
	}
	return rest
}

func opensOnLead(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return true
	}
	return false
}

// isBarePhone holds for an E.164 number: a plus and 6 to 15 digits, nothing else.
func isBarePhone(s string) bool {
	digits, ok := strings.CutPrefix(s, "+")
	if !ok || len(digits) < 6 || len(digits) > 15 {
		return false
	}
	return strings.Trim(digits, "0123456789") == ""
}
