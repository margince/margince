// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A header line is a bounded prefix of the field it renders — never a write
// that silently did not happen.

import (
	"strings"
	"testing"
)

func TestAHeaderLineIsABoundedPrefixOfWhatItRenders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
		why   string
	}{
		{
			name: "a value that fits is untouched",
			// The common case, and the one a prefix must not disturb: no cut,
			// no trim, byte-for-byte what the caller wrote.
			value: "We sell warehouse robotics.",
			want:  "We sell warehouse robotics.",
			why:   "a value inside the bound is not a rendering decision at all",
		},
		{
			name:  "exactly at the bound is untouched",
			value: strings.Repeat("a", companyDescriptionMax),
			want:  strings.Repeat("a", companyDescriptionMax),
			why:   "the CHECK admits this length, so cutting it would shorten a value the column holds",
		},
		{
			name: "one past the bound cuts at a word boundary",
			// THE CASE THAT WAS SILENTLY BROKEN. 501 characters: in contract,
			// refused by the column, and before this the write simply did not
			// happen and the header stayed blank.
			value: strings.Repeat("ab ", 166) + "xyz",
			why:   "a value one character over used to write nothing at all",
		},
		{
			name: "a single unbroken word is cut hard",
			// No boundary to cut at. A bounded value beats an empty one, and
			// this is the case a word-boundary rule has to answer rather than
			// fall through.
			value: strings.Repeat("z", companyDescriptionMax+50),
			want:  strings.Repeat("z", companyDescriptionMax),
			why:   "with no boundary to find, the hard cut is the only bounded answer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := headerLine(tc.value)
			if len([]rune(got)) > companyDescriptionMax {
				t.Fatalf("the line is %d characters, over the column's %d — the write the column would "+
					"refuse is exactly what this exists to prevent", len([]rune(got)), companyDescriptionMax)
			}
			if got == "" && tc.value != "" {
				t.Fatalf("a non-empty value rendered to nothing (%s)", tc.why)
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("got %q, want %q — %s", got, tc.want, tc.why)
			}
			if !strings.HasPrefix(tc.value, got) {
				t.Errorf("the line is not a prefix of the value it renders, so it says something the "+
					"field does not: %q", got)
			}
		})
	}
}

// A multi-byte value is bounded by CHARACTERS, because that is what the column
// counts.
//
// Postgres length() counts characters, so a byte-based cut would both refuse
// text the column accepts and split a character in half — a header rendering
// as a replacement glyph, from a field a human wrote correctly.
func TestAHeaderLineCountsCharactersNotBytes(t *testing.T) {
	value := strings.Repeat("ü", companyDescriptionMax)

	got := headerLine(value)

	if got != value {
		t.Errorf("a value of %d characters (%d bytes) was cut to %d characters — the column counts "+
			"characters, so this shortens text it would have accepted",
			len([]rune(value)), len(value), len([]rune(got)))
	}
}
