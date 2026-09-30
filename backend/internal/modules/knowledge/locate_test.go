// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import "testing"

// A citation's whole worth is that a reader can open the file and land on the
// sentence, with the sentence itself marked. These cases are the arithmetic
// that makes the landing correct at BOTH ends, and the two cases where it
// refuses to guess.
func TestAPassageLocatesAQuoteWithinIt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		passage Passage
		span    string
		want    Span
	}{
		{
			name:    "the first line reports the passage's own start line",
			passage: Passage{Text: "kept for 400 days", StartLine: 5},
			span:    "kept",
			want:    Span{Line: 5, Column: 1, EndLine: 5, EndColumn: 5},
		},
		{
			name:    "a span further along the first line moves the column, not the line",
			passage: Passage{Text: "kept for 400 days", StartLine: 5},
			span:    "400",
			want:    Span{Line: 5, Column: 10, EndLine: 5, EndColumn: 13},
		},
		{
			name:    "a newline before the span advances the line and restarts the column",
			passage: Passage{Text: "first line\nsecond line", StartLine: 5},
			span:    "second",
			want:    Span{Line: 6, Column: 1, EndLine: 6, EndColumn: 7},
		},
		{
			name:    "a span crossing a newline ends on a later line than it starts",
			passage: Passage{Text: "first line\nsecond line", StartLine: 5},
			span:    "line\nsecond",
			want:    Span{Line: 5, Column: 7, EndLine: 6, EndColumn: 7},
		},
		{
			name: "a span ending a line ends one past its last character",
			// The end is where a highlight STOPS, so it may name a column the
			// line does not have. Naming the last character instead would leave
			// no way to mark a quote that runs to the newline.
			passage: Passage{Text: "first line\nsecond line", StartLine: 5},
			span:    "first line",
			want:    Span{Line: 5, Column: 1, EndLine: 5, EndColumn: 11},
		},
		{
			name:    "the columns count CHARACTERS, so an umlaut counts once and not twice",
			passage: Passage{Text: "Löschfrist ist 400 Tage", StartLine: 12},
			span:    "400",
			// "Löschfrist ist " is 15 characters but 16 bytes. A byte count
			// would put the reader one column past the number.
			want: Span{Line: 12, Column: 16, EndLine: 12, EndColumn: 19},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.passage.Locate(tc.span); got != tc.want {
				t.Fatalf("Locate(%q) = %+v; want %+v", tc.span, got, tc.want)
			}
		})
	}
}

// A line number pointing at the wrong line is worse than none: the reader opens
// the file, reads a sentence that does not support the claim, and concludes the
// answer was invented. Both ways of not knowing return nothing at all — and
// nothing means no END either, because half a range marks the wrong words.
func TestAPassageRefusesToGuessWhereItCannotLocate(t *testing.T) {
	t.Parallel()

	t.Run("a span that is not in the passage", func(t *testing.T) {
		t.Parallel()
		if got := (Passage{Text: "kept for 400 days", StartLine: 5}).Locate("exported"); got != (Span{}) {
			t.Fatalf("a span absent from the passage located at %+v; want the zero Span", got)
		}
	})

	t.Run("a passage that never recorded a start line", func(t *testing.T) {
		t.Parallel()
		// Chunks written before the column existed carry no start line. They
		// still answer questions; they just cannot say where.
		if got := (Passage{Text: "kept for 400 days"}).Locate("kept"); got != (Span{}) {
			t.Fatalf("a passage with no start line located at %+v; want the zero Span", got)
		}
	})
}
