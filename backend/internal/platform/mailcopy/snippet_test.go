// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWordSnippetEndsOnAWholeWordWithinItsLimit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, text string
		limit      int
		want       string
	}{
		{"text within the limit is kept whole", "alpha beta", 10, "alpha beta"},
		{"a word across the cut is dropped", "alpha beta gamma", 10, "alpha…"},
		{"a cut just before a space keeps the word", "alpha beta gamma", 11, "alpha beta…"},
		{"a comma before the cut is not left hanging", "alpha, beta gamma", 10, "alpha…"},
		{"one long word is cut by rune", "abcdefghijkl", 10, "abcdefghi…"},
		{"a word cut keeping under half the limit cuts by rune", "Re: https://example.test/a/very/long/path", 20, "Re: https://example…"},
		{"the limit counts characters, not bytes", "Grüße aus Đà Nẵng", 12, "Grüße aus…"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := WordSnippet(c.text, c.limit); got != c.want {
				t.Errorf("WordSnippet(%q, %d) = %q, want %q", c.text, c.limit, got, c.want)
			}
		})
	}
}

func TestWordSnippetCountsItsMarkAndFlattensLines(t *testing.T) {
	t.Parallel()
	got := WordSnippet(strings.Repeat("word\n", 40), 25)
	if n := utf8.RuneCountInString(got); n > 25 {
		t.Errorf("%q is %d runes, over 25", got, n)
	}
	if strings.ContainsAny(got, "\r\n") || !strings.HasSuffix(got, " word…") {
		t.Errorf("%q kept a line break or lost the whole last word", got)
	}
	if got := WordSnippet("anything", 0); got != "" {
		t.Errorf("a zero limit kept %q", got)
	}
}
