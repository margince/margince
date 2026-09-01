// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

import "testing"

// Which way the PII census fails when it cannot read a statement.
//
// It is a census over an Art. 17 erasure, so the two directions are not
// symmetric: reporting a redaction it cannot read costs somebody a look at one
// statement, and skipping one costs a person the deletion they asked for while
// the build stays green. Every case here is a shape where the reading runs out,
// and every one of them must fail CLOSED.
func TestAStatementTheCensusCannotReadIsNotReadAsClean(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		statement string
		want      bool
	}{
		"a rendered redaction reads": {"UPDATE person SET note = NULL WHERE id = $1", true},
		"two rendered clauses read":  {"UPDATE person SET a = NULL, b = NULL WHERE id = $1", true},
		// A VALUE the reader could not render hides no column: `note` is
		// plainly assigned, and what it is assigned TO is a question this
		// census does not ask of any statement, rendered or not. Refusing here
		// took a fully accounted-for table out of the census for one interval
		// built by a helper.
		"a value built from a const": {"UPDATE person SET note = " + unresolved + " WHERE id = $1", true},
		// The marker on the LEFT is another matter: there it could be standing
		// over an assignment, and what it clears is what nobody can see.
		"a column built from a const": {"UPDATE person SET " + unresolved + " = NULL WHERE id = $1", false},
		"a marker among real pairs":   {"UPDATE person SET a = NULL, " + unresolved + ", b = NULL", false},
		"commas inside a value":       {"UPDATE person SET a = coalesce(x, ''), b = NULL", true},
		"an array value with commas":  {"UPDATE person SET a = ARRAY[x, y], b = NULL", true},
		// THE WHOLE CLAUSE swallowed, which is `"UPDATE person " + clearEverything`.
		// The `set` token goes with it, so the clause pattern finds nothing at
		// all — and answering "readable" for a statement with no readable
		// assignment in it is this census skipping the one write it could not
		// read.
		"the clause swallowed whole": {"UPDATE person " + unresolved, false},
		"nothing but the marker":     {unresolved, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := assignmentsAreReadable(tc.statement); got != tc.want {
				t.Errorf("assignmentsAreReadable(%q) = %v, want %v", tc.statement, got, tc.want)
			}
		})
	}
}

// And which writes count as redacting a table at all.
//
// `writeTarget` matches `INSERT INTO` too — an insert's ON CONFLICT can carry an
// UPDATE, so it has to. A write with no SET clause assigns nothing, and
// registering its table as judged demanded a baseline for every text column of a
// table the cascade may never redact.
func TestOnlyAnAssigningWriteRegistersATableAsRedacted(t *testing.T) {
	t.Parallel()
	columns := map[string]map[string]string{"person": {"note": "text", "full_name": "text"}}
	for name, tc := range map[string]struct {
		statement string
		want      map[string]bool
	}{
		"an update assigns": {
			"UPDATE person SET note = NULL WHERE id = $1",
			map[string]bool{"note": true},
		},
		"an upsert's DO UPDATE assigns": {
			"INSERT INTO person (id, note) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET note = NULL",
			map[string]bool{"note": true},
		},
		"a plain insert assigns nothing": {
			"INSERT INTO person (id, note) VALUES ($1, $2)",
			nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := erasureRedactedColumns(t, columns, []string{tc.statement})
			if tc.want == nil {
				if _, judged := got["person"]; judged {
					t.Errorf("a write that assigns nothing registered person as judged: %v — every "+
						"text column of it would then need a baseline entry", got)
				}
				return
			}
			if len(got["person"]) != len(tc.want) {
				t.Fatalf("redacted %v, want %v", got["person"], tc.want)
			}
			for column := range tc.want {
				if !got["person"][column] {
					t.Errorf("%s was assigned and is not reported as redacted: %v", column, got["person"])
				}
			}
		})
	}
}
