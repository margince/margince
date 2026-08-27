// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import "testing"

// A citation's whole worth is that a reader can open the file and land on the
// sentence. These cases are the arithmetic that makes the landing correct, and
// the two cases where it refuses to guess.
func TestAPassageLocatesAQuoteWithinIt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		passage   Passage
		span      string
		line, col int
	}{
		{
			name:    "the first line reports the passage's own start line",
			passage: Passage{Text: "kept for 400 days", StartLine: 5},
			span:    "kept",
			line:    5,
			col:     1,
		},
		{
			name:    "a span further along the first line moves the column, not the line",
			passage: Passage{Text: "kept for 400 days", StartLine: 5},
			span:    "400",
			line:    5,
			col:     10,
		},
		{
			name:    "a newline before the span advances the line and restarts the column",
			passage: Passage{Text: "first line\nsecond line", StartLine: 5},
			span:    "second",
			line:    6,
			col:     1,
		},
		{
			name:    "the column counts CHARACTERS, so an umlaut counts once and not twice",
			passage: Passage{Text: "Löschfrist ist 400 Tage", StartLine: 12},
			span:    "400",
			line:    12,
			// "Löschfrist ist " is 15 characters but 16 bytes. A byte count
			// would put the reader one column past the number.
			col: 16,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line, col := tc.passage.Locate(tc.span)
			if line != tc.line || col != tc.col {
				t.Fatalf("Locate(%q) = line %d, column %d; want line %d, column %d",
					tc.span, line, col, tc.line, tc.col)
			}
		})
	}
}

// A line number pointing at the wrong line is worse than none: the reader opens
// the file, reads a sentence that does not support the claim, and concludes the
// answer was invented. Both ways of not knowing return nothing at all.
func TestAPassageRefusesToGuessWhereItCannotLocate(t *testing.T) {
	t.Parallel()

	t.Run("a span that is not in the passage", func(t *testing.T) {
		t.Parallel()
		line, col := Passage{Text: "kept for 400 days", StartLine: 5}.Locate("exported")
		if line != 0 || col != 0 {
			t.Fatalf("a span absent from the passage located at line %d, column %d; want 0, 0", line, col)
		}
	})

	t.Run("a passage that never recorded a start line", func(t *testing.T) {
		t.Parallel()
		// Chunks written before the column existed carry no start line. They
		// still answer questions; they just cannot say where.
		line, col := Passage{Text: "kept for 400 days"}.Locate("kept")
		if line != 0 || col != 0 {
			t.Fatalf("a passage with no start line located at line %d, column %d; want 0, 0", line, col)
		}
	})
}
