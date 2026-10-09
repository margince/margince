// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package textcut shortens a string to a length without splitting a UTF-8
// sequence. The caller chooses the unit (bytes or runes) and whether a cut
// is marked; backend/gates/textcut_test.go fails a hand-written cut elsewhere.
package textcut

import "unicode/utf8"

// Bytes is the longest prefix of s that holds at most limit bytes and does not
// end inside a rune. A non-positive limit yields "".
func Bytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= 0 {
		return ""
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// BytesMarked is Bytes followed by marker when the cut drops anything. The
// marker sits past the limit, so the result can be len(marker) bytes longer.
func BytesMarked(s string, limit int, marker string) string {
	if len(s) <= limit {
		return s
	}
	return Bytes(s, limit) + marker
}

// Runes is the first limit runes of s, or s itself when it has no more. A
// non-positive limit yields "".
//
// A cut prefix answers what string([]rune(s)[:limit]) answers, so a broken
// byte in it comes back as U+FFFD. It reads only the runes it keeps.
func Runes(s string, limit int) string {
	end := 0
	for kept := 0; kept < limit && end < len(s); kept++ {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}
	if end >= len(s) {
		return s
	}
	head := s[:end]
	if utf8.ValidString(head) {
		return head
	}
	return string([]rune(head))
}

// RunesMarked is Runes followed by marker when the cut drops anything. The
// marker sits past the limit.
func RunesMarked(s string, limit int, marker string) string {
	if cut := Runes(s, limit); cut != s {
		return cut + marker
	}
	return s
}
