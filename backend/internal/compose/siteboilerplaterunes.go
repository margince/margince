// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The byte and rune arithmetic the shared-chrome stripper (siteboilerplate.go)
// runs in its innermost loop. Each helper answers exactly what the conversion
// it replaced answered — siteboilerplate_equivalence_test.go holds them to the
// old code — while reading only the bytes the answer needs.

import "unicode/utf8"

// headRunes answers what string([]rune(s)[:n]) answers when s holds more than
// n runes, and s itself otherwise — reading only those n runes.
//
// The windows this file measures are 60 and 300 runes, and they used to be
// taken by converting the WHOLE page to []rune first. Inside sharedOpening's
// pages × offsets × pages loop that was hundreds of thousands of full-page
// conversions per round: one call on a 60-page site of ~14k runes a page took
// 58 s and allocated 52 GB, which was a deep read's extraction time and most
// of the garbage a worker bursts to gigabytes on.
//
// The answer is kept byte-for-byte, broken bytes included: the old conversion
// re-encoded each one as U+FFFD, so a window over invalid UTF-8 is re-encoded
// the same way here. Valid text — nearly all of it — is returned as a slice
// of s with nothing allocated.
func headRunes(s string, n int) string {
	i := 0
	for count := 0; count < n && i < len(s); count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	if i >= len(s) {
		return s
	}
	head := s[:i]
	if utf8.ValidString(head) {
		return head
	}
	return string([]rune(head))
}

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
