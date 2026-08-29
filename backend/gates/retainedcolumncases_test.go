// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H1

package gates

// The retention sweep's two SQL claims, driven with SYNTHETIC statements rather
// than the tree — the same reason extensionsqlscopecases_test.go gives for its
// own cases. The sweep is supposed to pass, so a gate proven only by "the one
// statement in the tree is clean" is one that keeps passing after it stops
// working.
//
// Two claims, not one, and the file is named for the larger: that the sweep does
// not write a column registered as KEPT (statementDestroying), and that a
// declared erasure arrives in ONE statement (oneStatementCarries).
//
// Every MISSED row below is a shape an earlier version of the check could not
// see, each one green while the sweep destroyed the column the declaration
// exists to protect. Rule 8 asks what shape of the defect a gate cannot see and
// says to plant that case; this is that list.
//
// WHAT IS STILL NOT ON IT, stated because a list that reads as complete is what
// stops the next author looking:
//
//   - A statement satisfies a declaration by EXISTING. A decoy carrying the
//     whole erasure — a helper nobody calls — answers for an action that has
//     stopped making it. Closing that needs call-graph reachability, which no
//     SQL gate in this tree has; piicoverage_test.go states it at the call site.
//   - An assembled SET target is not matched, because the column is not in the
//     text to match. It is REPORTED instead — assembledSetTarget fails on a
//     swept UPDATE whose SET clause names nothing — and the case below plants
//     that shape.

import "testing"

func TestTheRetainedColumnCheckSeesEveryDestructiveShape(t *testing.T) {
	t.Parallel()
	const table, column = "activity", "counterparty_email"

	for _, tc := range []struct {
		name      string
		statement string
		destroys  bool
	}{
		{
			name:      "the plain wipe",
			statement: `UPDATE activity SET body = NULL, counterparty_email = NULL WHERE id = $1`,
			destroys:  true,
		}, {
			name:      "no spaces around the equals",
			statement: `UPDATE activity SET body=NULL, counterparty_email=NULL WHERE id = $1`,
			destroys:  true,
		}, {
			name:      "cast to null, which is not the string NULL",
			statement: `UPDATE activity SET counterparty_email = CAST(NULL AS text) WHERE id = $1`,
			destroys:  true,
		}, {
			name:      "emptied rather than nulled",
			statement: `UPDATE activity SET counterparty_email = '' WHERE id = $1`,
			destroys:  true,
		}, {
			// The shape this tree already writes in retentionrestricted.go. A
			// lazy regex ending at the first WHERE stops inside the subquery,
			// so everything after it — here the destruction — goes unseen.
			name: "assignment after a subquery carrying its own WHERE",
			statement: `UPDATE activity SET redacted_fields = ARRAY(SELECT c FROM unnest(ARRAY['body']) AS c WHERE c IS NOT NULL), ` +
				`counterparty_email = NULL WHERE id = $1`,
			destroys: true,
		}, {
			name:      "the row removed outright, which takes the column with it",
			statement: `DELETE FROM activity WHERE id = $1`,
			destroys:  true,
		}, {
			// Selecting on the column is not writing it. A check that looked
			// anywhere in the statement rather than in the SET clause would
			// call this a finding and teach the next author to distrust it.
			name:      "named in a predicate, not assigned",
			statement: `UPDATE activity SET body = NULL WHERE id = $1 AND counterparty_email IS NOT NULL`,
			destroys:  false,
		}, {
			name:      "a different table's column of the same name",
			statement: `UPDATE activity_participant SET counterparty_email = NULL WHERE activity_id = $1`,
			destroys:  false,
		}, {
			name:      "a longer column that merely starts with the retained name",
			statement: `UPDATE activity SET counterparty_email_hash = NULL WHERE id = $1`,
			destroys:  false,
		}, {
			// gate-patterns.md §D's own example. Split naively on `;`, this is
			// two fragments and the second — `b', counterparty_email = null
			// where …` — names no table, so sqlWriteTargets drops it and the
			// destruction rides out inside a string nobody parsed.
			name:      "a semicolon inside a quoted value",
			statement: `UPDATE activity SET body = 'a;b', counterparty_email = NULL WHERE id = $1`,
			destroys:  true,
		}, {
			// The other half of the same trap: a REAL second statement still
			// has to split, so the fix for the case above must not be "stop
			// splitting".
			name: "a quoted semicolon and a real one",
			statement: `UPDATE activity SET body = 'a;b' WHERE id = $1; ` +
				`UPDATE activity SET counterparty_email = NULL WHERE id = $1`,
			destroys: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := statementDestroying(sqlStatements(tc.statement), table, column) != ""
			if got != tc.destroys {
				t.Errorf("statementDestroying(%q) = %v, want %v", tc.statement, got, tc.destroys)
			}
		})
	}

	// The shape the check cannot MATCH, and therefore reports. A column
	// assembled at runtime is not in the literal, so statementDestroying
	// answers "destroys nothing" — indistinguishable from a statement that
	// really destroys nothing, which is how a shape gate goes green (§D).
	t.Run("an assembled SET target is reported rather than read as harmless", func(t *testing.T) {
		assembled := []string{collapsedSQL(`UPDATE activity SET `)}
		if statementDestroying(assembled, table, column) != "" {
			t.Error("a statement with no column in it was read as destroying one")
		}
		if assembledSetTarget(assembled) == "" {
			t.Error("a swept UPDATE whose SET clause names no column was not reported — the column is " +
				"assembled at runtime and the KEEPS registration is no longer checked against it")
		}
		// And an ordinary statement is not reported, or the tripwire would fire
		// on every sweep and be turned off.
		if got := assembledSetTarget([]string{`UPDATE activity SET body = NULL WHERE id = $1`}); got != "" {
			t.Errorf("an ordinary UPDATE was reported as assembled: %q", got)
		}
	})

	// Two statements in one Go literal, the second destroying. Read whole, the
	// first statement's SET clause answers for both.
	t.Run("a second statement in the same literal", func(t *testing.T) {
		literal := `UPDATE activity SET body = NULL WHERE id = $1;
		            UPDATE activity SET counterparty_email = NULL WHERE id = $1`
		if statementDestroying(sqlStatements(literal), table, column) == "" {
			t.Error("a destroying second statement in the same literal went unseen — the literal is not the unit, the statement is")
		}
	})
}

// The companion claim: declared erasure assignments must arrive in ONE
// statement, so an unrelated statement — or a helper nothing calls — cannot
// answer for an erasure that no longer happens.
func TestDeclaredErasuresMustArriveInOneStatement(t *testing.T) {
	t.Parallel()
	declared := []string{"body = NULL", "raw = NULL"}

	together := []string{`update activity set body = null, raw = null, subject = $2 where id = $1`}
	if !oneStatementCarries(together, declared) {
		t.Error("one statement making both assignments was not accepted")
	}

	apart := []string{
		`update activity set body = null where id = $1`,
		`update activity set raw = null where id = $2`,
	}
	if oneStatementCarries(apart, declared) {
		t.Error("two statements each making half the erasure were accepted — a declaration describes one act, and " +
			"accepting the union lets a dead helper carrying the missing half stand in for the action that stopped making it")
	}
}
