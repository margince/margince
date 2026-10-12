// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textcut_test

import (
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// "Grüße" is G r ü ß e: seven bytes, five runes, with ü at bytes 2-3 and ß at 4-5.
const greeting = "Grüße"

func TestBytesNeverEndsInsideARune(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		limit int
		want  string
	}{
		{"short string is kept whole", "abc", 10, "abc"},
		{"length equal to the limit is kept whole", greeting, 7, greeting},
		{"limit at a rune boundary", greeting, 4, "Grü"},
		{"limit inside a two-byte rune backs off", greeting, 3, "Gr"},
		{"limit inside a four-byte rune backs off", "a🚀b", 3, "a"},
		{"limit zero", greeting, 0, ""},
		{"negative limit", greeting, -1, ""},
		{"empty string", "", 0, ""},
		{"limit inside the first rune leaves nothing", "ü", 1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := textcut.Bytes(c.s, c.limit)
			if got != c.want {
				t.Fatalf("Bytes(%q, %d) = %q, want %q", c.s, c.limit, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("Bytes(%q, %d) = %q is not valid UTF-8", c.s, c.limit, got)
			}
		})
	}
}

func TestRunesCountsCharactersNotBytes(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		limit int
		want  string
	}{
		{"short string is kept whole", "abc", 10, "abc"},
		{"rune count equal to the limit is kept whole", greeting, 5, greeting},
		{"multi-byte runes count once", greeting, 3, "Grü"},
		{"four-byte rune counts once", "a🚀b", 2, "a🚀"},
		{"limit zero", greeting, 0, ""},
		{"negative limit", greeting, -1, ""},
		{"a broken byte in the kept part becomes U+FFFD", "a\xffbc", 2, "a�"},
		{"a broken byte is kept as is when nothing is cut", "a\xff", 2, "a\xff"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := textcut.Runes(c.s, c.limit); got != c.want {
				t.Fatalf("Runes(%q, %d) = %q, want %q", c.s, c.limit, got, c.want)
			}
		})
	}
}

func TestRunesAnswersWhatTheRuneConversionAnswers(t *testing.T) {
	for _, s := range []string{"", "abc", greeting, "製品カタログ", "a\xe2\x82b", "\xff\xff\xff", "🚀🚀🚀"} {
		runes := []rune(s)
		for limit := range len(runes) + 2 {
			want := s
			if limit < len(runes) {
				want = string(runes[:limit])
			}
			if got := textcut.Runes(s, limit); got != want {
				t.Fatalf("Runes(%q, %d) = %q, the conversion says %q", s, limit, got, want)
			}
		}
	}
}

func TestAMarkerIsAddedOnlyWhenSomethingIsCut(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"bytes: short string has no marker", textcut.BytesMarked("abc", 3, "…"), "abc"},
		{"bytes: a cut is marked past the limit", textcut.BytesMarked(greeting, 3, "…"), "Gr…"},
		{"bytes: limit zero still marks the cut", textcut.BytesMarked(greeting, 0, "…"), "…"},
		{"bytes: the marker is the caller's", textcut.BytesMarked("{a, b, c}", 5, "…}"), "{a, b…}"},
		{"runes: short string has no marker", textcut.RunesMarked(greeting, 5, "…"), greeting},
		{"runes: a cut is marked past the limit", textcut.RunesMarked(greeting, 3, "…"), "Grü…"},
		{"runes: limit zero still marks the cut", textcut.RunesMarked(greeting, 0, "…"), "…"},
		{"runes: empty string has no marker", textcut.RunesMarked("", 0, "…"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Fatalf("got %q, want %q", c.got, c.want)
			}
		})
	}
}
