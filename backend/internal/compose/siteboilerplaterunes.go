// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The byte and rune arithmetic the shared-chrome stripper (siteboilerplate.go)
// runs in its innermost loop. Each helper answers exactly what the conversion
// it replaced answered — siteboilerplate_equivalence_test.go holds them to the
// old code — while reading only the bytes the answer needs.

import "unicode/utf8"

// runesAtLeast reports whether s holds at least n runes, counting them the way
// utf8.RuneCountInString does (a broken byte is one rune) but stopping at n.
// The thresholds here are a few hundred runes; the strings they are asked of
// are whole pages.
func runesAtLeast(s string, n int) bool {
	if len(s) < n {
		// Every rune is at least one byte.
		return false
	}
	if len(s) >= utf8.UTFMax*n {
		// And at most UTFMax.
		return true
	}
	count := 0
	for range s {
		count++
		if count >= n {
			return true
		}
	}
	return count >= n
}

// validPrefix is the longest prefix of s that is valid UTF-8: the prefix up
// to the first byte that does not decode. It replaces a loop that trimmed a
// byte and re-validated the whole string until it passed, which is the same
// answer at quadratic cost on a header's worth of text.
func validPrefix(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return s[:i]
		}
		i += size
	}
	return s
}

// firstDifference is the index of the first byte at which a and b, of equal
// length, differ — or their length when they do not. It narrows by comparing
// halves, which the runtime does a machine word or a vector at a time, rather
// than stepping a byte at a time through up to a header's worth of text.
func firstDifference(a, b string) int {
	if a == b {
		return len(a)
	}
	// a[:lo] == b[:lo], and they differ somewhere before hi.
	lo, hi := 0, len(a)
	for hi-lo > 64 {
		mid := lo + (hi-lo)/2
		if a[lo:mid] == b[lo:mid] {
			lo = mid
		} else {
			hi = mid
		}
	}
	for lo < hi && a[lo] == b[lo] {
		lo++
	}
	return lo
}
