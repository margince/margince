// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxSummaryLen bounds the sentence an inbox row shows. Long enough for an
// operation, a target and the fields that matter; short enough that a
// crafted prefix cannot push them past what a human reads.
const maxSummaryLen = 240

// SanitizeSummary bounds untrusted approval captions and removes characters
// that can hide or reverse the action a reader is deciding.
func SanitizeSummary(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastWasSpace := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue // invalid encoding: not text, not shown
		case unicode.IsSpace(r):
			if !lastWasSpace && b.Len() > 0 {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		case values.IsInvisibleRune(r):
			// Cf covers the bidirectional overrides and isolates
			// (U+202A-E, U+2066-9), the directional marks (U+200E/F,
			// U+061C) and the zero-width joiners a homograph relies on.
			continue
		default:
			b.WriteRune(r)
			lastWasSpace = false
		}
	}
	return boundRunes(strings.TrimSpace(b.String()), maxSummaryLen)
}

// boundRunes truncates so the RESULT is at most n bytes, marking that it did
// so — a silently cut summary reads as a complete one. The ellipsis is part of
// the budget, not added on top of it: a caller that passes a limit means the
// output, and a bound that overshoots by three bytes is not a bound.
func boundRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const ellipsis = "…"
	return strings.TrimSpace(textcut.Bytes(s, n-len(ellipsis))) + ellipsis
}
