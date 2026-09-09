// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A caller's text never keeps its ability to forge the frame around it.
//
// echoSafe escaped ASCII control characters only, which admits every character
// ABOVE ascii that ends a line: U+2028 and U+2029 are line terminators to a
// great many tokenizers, U+0085 to some, U+202E reverses the reading order of
// everything after it, and U+200B is invisible. A caller who could not write a
// newline into a tool result could write U+2028 and get one.

import (
	"strings"
	"testing"
)

func TestEchoSafeEscapesEverythingThatDoesNotPrint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		raw  string
	}{
		{"line separator", "\u2028"},
		{"paragraph separator", "\u2029"},
		{"next line", "\u0085"},
		{"right-to-left override", "\u202e"},
		{"zero width space", "\u200b"},
		{"a bare escape byte", "\x1b"},
		{"a byte that is not utf-8", "\xff"},
		{"a tag character above the BMP", "\U000e0020"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			got := echoSafe("before"+tc.raw+"after", maxBadArgsDetail)

			if strings.Contains(got, tc.raw) {
				t.Errorf("%s survived into the tool result verbatim: %q", tc.what, got)
			}
			if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
				t.Errorf("the caller's own text was lost around the escape: %q", got)
			}
		})
	}
}

// A REPLACEMENT CHARACTER the caller actually sent is reported as itself.
//
// It is the one rune a range loop cannot tell from a broken byte — both arrive
// as RuneError — and deciding between them by re-validating the single lead byte
// gets this case wrong: U+FFFD came back as a bare \xef with its two
// continuation bytes dropped, so the refusal named a byte nobody sent.
func TestEchoSafeReportsARealReplacementCharacterAsItself(t *testing.T) {
	t.Parallel()
	const sent = "before\ufffdafter"

	got := echoSafe(sent, maxBadArgsDetail)

	if got != sent {
		t.Errorf("a legitimately encoded U+FFFD was rewritten: %q became %q", sent, got)
	}
}

// An astral escape must be one a reader can parse back. `\u` takes four digits,
// so U+E0020 rendered as `\ue0020` reads as `\ue002` followed by a `0`.
func TestEchoSafeRendersAnAstralRuneUnambiguously(t *testing.T) {
	t.Parallel()
	got := echoSafe("tag\U000e0020here", maxBadArgsDetail)

	if !strings.Contains(got, `\U000e0020`) {
		t.Errorf("an astral rune was not rendered as an eight-digit escape: %q", got)
	}
}

// Ordinary text is left alone. Escaping everything unprintable must not start
// mangling a real name — a German or Vietnamese company name is what this
// surface refuses against every day.
func TestEchoSafeLeavesRealNamesAlone(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"K\u00f6rber Sensorik Rollout",
		"C\u00f4ng ty X\u00e2y d\u1ef1ng",
		"deals-by-stage",
		"amount_base_minor",
		"a name with spaces",
	} {
		if got := echoSafe(name, maxBadArgsDetail); got != name {
			t.Errorf("echoSafe rewrote an ordinary name: %q became %q", name, got)
		}
	}
}
