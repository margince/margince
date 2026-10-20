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
		{name: "an escape string keeps its escaped quote inside the span", text: `E'it\'s UPDATE contact'`, pos: 1, length: 22, closed: true},
		{name: "an escape at the end cannot close a string", text: `E'bad\'`, pos: 1, length: 6, closed: false},
		{name: "a quoted value", text: "x = 'a, b' , y", pos: 4, length: 6, closed: true},
		{name: "doubled quotes stay in one span", text: "'it''s'", pos: 0, length: 7, closed: true},
		{name: "keyword final E does not enable escapes", text: `LIKE'a\'; DELETE FROM contact; SELECT 'x'`, pos: 4, length: 4, closed: true},
		{name: "identifier final E does not enable escapes", text: `name'a\'; DELETE FROM contact; SELECT 'x'`, pos: 4, length: 4, closed: true},
		{name: "escape strings retain doubled quotes", text: `E'it''s\' fine'`, pos: 1, length: 14, closed: true},

		{name: "an unterminated quote is reported", text: "x = 'never closed", pos: 4, length: 13, closed: false},
		{name: "a dollar-quoted value", text: "x = $$a;b$$ , y", pos: 4, length: 7, closed: true},
		{name: "a tagged dollar quote", text: "$tag$ where 'it' $tag$", pos: 0, length: 22, closed: true},
		{name: "an unterminated dollar quote is reported", text: "$tag$ never closed", pos: 0, length: 18, closed: false},
		{name: "a positional placeholder is not a dollar quote", text: "id = $1 and $2", pos: 5, length: 0, closed: true},
		{name: "a tag needs its closing dollar", text: "$tag never opened", pos: 0, length: 0, closed: true},
		{name: "a tag may carry digits after its first letter", text: "$_t1$ x $_t1$", pos: 0, length: 13, closed: true},
		{name: "a tag may not start with a digit", text: "$1x$ y", pos: 0, length: 0, closed: true},
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
