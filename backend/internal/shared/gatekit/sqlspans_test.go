// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import "testing"

// Every shape a census reading SQL structure has to step over, and every
// lookalike it must not. Each MISSED row hides the assignments after it from
// whatever is scanning; each unterminated row is the one that must be reported
// rather than swallowed, because swallowing steps over the whole remainder and
// passes green.
func TestSQLSpanAt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		text   string
		pos    int
		length int
		closed bool
	}{
		{name: "ordinary SQL is not a span", text: "a = 1", pos: 0, length: 0, closed: true},
		{name: "a quoted value", text: "x = 'a, b' , y", pos: 4, length: 6, closed: true},
		{
			// A doubled '' is how SQL escapes one, and it needs no case in the
			// scan: the first run ends AT the first of the pair, and the second
			// of the pair opens a run that ends where the real close is. The
			// two together cover the literal, which is what a caller stepping
			// span by span sees.
			name: "a doubled quote is two runs that meet",
			text: "'it''s'", pos: 0, length: 4, closed: true,
		},
		{
			name: "and the second of the pair closes where the value really ends",
			text: "'it''s'", pos: 4, length: 3, closed: true,
		},
		{name: "an unterminated quote is reported", text: "x = 'never closed", pos: 4, length: 13, closed: false},
		{name: "a dollar-quoted value", text: "x = $$a;b$$ , y", pos: 4, length: 7, closed: true},
		{name: "a tagged dollar quote", text: "$tag$ where 'it' $tag$", pos: 0, length: 22, closed: true},
		{name: "an unterminated dollar quote is reported", text: "$tag$ never closed", pos: 0, length: 18, closed: false},
		{
			// A line comment ENDS at the newline; running to the end of the
			// text is where it ends, not a failure to close.
			name: "a line comment to the newline", text: "-- it's the owner's row\nx = 1", pos: 0, length: 24, closed: true,
		},
		{name: "a line comment to the end of the text", text: "-- it's all", pos: 0, length: 11, closed: true},
		{name: "a block comment", text: "/* ; */ x", pos: 0, length: 7, closed: true},
		{
			// Postgres NESTS these. A scan stopping at the first `*/` resumes
			// inside the comment and reads its text as SQL.
			name: "a nested block comment", text: "/* a /* b */ c */ x", pos: 0, length: 17, closed: true,
		},
		{name: "an unterminated block comment is reported", text: "/* never closed", pos: 0, length: 15, closed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			length, closed := SQLSpanAt(tc.text, tc.pos)
			if length != tc.length || closed != tc.closed {
				t.Errorf("SQLSpanAt(%q, %d) = (%d, %v), want (%d, %v)", tc.text, tc.pos, length, closed, tc.length, tc.closed)
			}
		})
	}
}
