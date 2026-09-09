// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// The rendered token is bounded and escaped, whatever a caller sends.
//
// Both halves were reported: the bound was applied before quoting, so escaping
// pushed the answer past it, and the doc claimed no escaping happened at all
// while `%q` was doing it. A caller who can choose the size of a refusal can
// choose how much of a transcript it fills.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestARenderedCallerTokenIsAlwaysBounded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what  string
		token string
	}{
		{"an ordinary name", "pipeline_totals"},
		{"empty", ""},
		{"space padded", "   "},
		{"long ascii", strings.Repeat("k", 4000)},
		// Escaping is what decides the length: each newline renders as two
		// bytes, so a token well inside the bound before quoting is past it
		// after.
		{"newlines that double under escaping", strings.Repeat("\n", 79)},
		{"multi-byte runes", strings.Repeat("Koerber", 400)},
		{"control bytes that escape to four bytes each", strings.Repeat("\x01", 500)},
		{"a trailing backslash at the seam", strings.Repeat(`\`, 300)},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			got := QuoteCaller(tc.token)

			if len(got) > MaxCallerToken {
				t.Errorf("rendered %d bytes, past the %d-byte bound: %s", len(got), MaxCallerToken, got)
			}
			if !utf8.ValidString(got) {
				t.Errorf("rendered invalid utf-8: %q", got)
			}
			if !strings.HasPrefix(got, `"`) {
				t.Errorf("rendered without an opening quote, so an empty name is invisible: %s", got)
			}
			if !strings.HasSuffix(got, `"`) {
				t.Errorf("rendered without a closing quote, so a reader cannot see where it ends: %s", got)
			}
		})
	}
}

// A control character never survives as itself. The refusal lands in a
// transcript whose later prompts the same model reads, so a newline in a name
// must not arrive looking like a new line of conversation.
func TestACallerTokenIsEscaped(t *testing.T) {
	t.Parallel()
	got := QuoteCaller("one\ntwo\ttab")

	for _, raw := range []string{"\n", "\t"} {
		if strings.Contains(got, raw) {
			t.Errorf("a raw control character survived into %s", got)
		}
	}
	if !strings.Contains(got, `\n`) {
		t.Errorf("the newline was not rendered visible: %s", got)
	}
}

// The seam must not read as a bug in this function: a cut landing after an odd
// run of backslashes would emit a dangling escape.
func TestTheCutNeverLeavesADanglingEscape(t *testing.T) {
	t.Parallel()
	for n := 1; n <= 200; n++ {
		got := QuoteCaller(strings.Repeat(`\`, n))
		body := strings.TrimSuffix(strings.TrimSuffix(got, `"`), "…")
		if trailingBackslashes(body)%2 == 1 {
			t.Fatalf("%d backslashes rendered with a dangling escape: %s", n, got)
		}
	}
}
