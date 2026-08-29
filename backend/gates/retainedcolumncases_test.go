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
//     text to match. It is REPORTED instead, from two directions: from the TEXT
//     when the fragment reads as unfinished (assembledSetTarget — no
//     assignments, or an assignment list ending on a comma), and from the
//     SYNTAX when it reads as finished but is joined to a runtime value
//     (assembledSweepTargets). Both shapes are planted below.
//   - A statement assembled by something other than `+` is reported from two
//     directions as well. `fmt.Sprintf("UPDATE activity SET %s = NULL", col)`
//     leaves a fmt VERB standing where a column should be, which is the text
//     tell; and a fragment handed to a named assembler — Sprintf, a
//     strings.Builder's WriteString, strings.Replace, strings.Join — is a half
//     statement with no `+` node to say so, which is the syntax tell. The
//     assembler list is a LIST rather than "any call", because a literal inside
//     a call is every statement in this tree: tx.Exec takes the whole thing.
//   - What still gets past all four: an assembler this tree does not use, and a
//     builder that writes the column in a call carrying no SQL literal at all —
//     `b.WriteString(column)` after `b.WriteString("UPDATE activity SET ")`
//     IS caught, because the first call carries the fragment, but a builder fed
//     entirely from variables carries no text to read. Closing that needs the
//     gate to evaluate the expression rather than read it.

import (
	"os"
	"path/filepath"
	"testing"
)

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
			// A DOUBLED quote inside a quoted value, which is how SQL escapes
			// one. The split has no case for it — parity carries it, since two
			// toggles leave the scan where it was — and this is what holds that
			// claim: an escape branch that "skipped" the pair would leave the
			// scan open, the `;` would read as unquoted, and the destruction
			// after it would ride out in a fragment naming no table.
			name:      "a doubled quote and a semicolon in the same value",
			statement: `UPDATE activity SET body = 'it''s; fine', counterparty_email = NULL WHERE id = $1`,
			destroys:  true,
		}, {
			// A dollar-quoted value carrying a semicolon. Understood rather
			// than refused: the scan knows this form, and refusing what it can
			// read would cost the sweep its own statements.
			name:      "a semicolon inside a dollar-quoted value",
			statement: "UPDATE activity SET body = $$a;b$$, counterparty_email = NULL WHERE id = $1",
			destroys:  true,
		}, {
			name:      "a semicolon inside a tagged dollar quote",
			statement: "UPDATE activity SET body = $tag$a;b$tag$, counterparty_email = NULL WHERE id = $1",
			destroys:  true,
		}, {
			// A semicolon inside a COMMENT. Split there, the fragment carrying
			// the destruction names no table and the gate passes over exactly
			// the write it exists to see.
			name:      "a semicolon inside a block comment",
			statement: `UPDATE activity SET body = NULL /* ; */, counterparty_email = NULL WHERE id = $1`,
			destroys:  true,
		}, {
			name:      "a semicolon inside a line comment",
			statement: "UPDATE activity SET body = NULL, -- one; two\n counterparty_email = NULL WHERE id = $1",
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
}

// The shapes the check cannot MATCH, and therefore REPORTS. Split from the
// case table above because they ask a different question: the table asks
// whether a destructive statement is seen, and these ask whether a statement
// the gate cannot read is refused rather than read as harmless — which is how
// a shape gate goes quietly green (gate-patterns.md §D).
func TestTheRetainedColumnCheckRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	const table, column = "activity", "counterparty_email"

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

	// The same shape one assignment in. `"UPDATE activity SET body = NULL, " +
	// column + " = NULL"` leaves a SET clause that is not empty, so a check
	// asking only whether it is empty reads it as an ordinary statement — and
	// the assembled column goes unseen exactly as it did before the tripwire.
	t.Run("an assembled target behind a static assignment is reported too", func(t *testing.T) {
		stmt := collapsedSQL(`UPDATE activity SET body = NULL, `)
		if statementDestroying([]string{stmt}, table, column) != "" {
			t.Error("a statement with no column in it was read as destroying one")
		}
		if assembledSetTarget([]string{stmt}) == "" {
			t.Error("a SET clause ending on a comma was read as complete — the assignment after the separator " +
				"is assembled at runtime and the KEEPS registration is no longer checked against it")
		}
	})

	// The shape no reading of the TEXT can catch: a fragment that is a whole
	// statement with more concatenated onto it. Only the `+` node says so,
	// which is why assembledSweepTargets reads the syntax tree.
	t.Run("a complete-looking fragment joined to a runtime value is reported", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sweep.go")
		source := "package p\n\nfunc f(extra string) string {\n" +
			"\treturn `UPDATE activity SET body = NULL` + extra + ` WHERE id = $1`\n}\n"
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got := assembledSweepTargets(t, path)
		if got[table] == "" {
			t.Errorf("a statement joined to a runtime value was not reported: %v", got)
		}
		// Two literals joined must NOT be reported: sqlLiterals reads both
		// halves, so nothing is hidden, and reporting them would fire on every
		// wrapped statement in the tree and get the tripwire turned off.
		whole := filepath.Join(dir, "whole.go")
		wholeSource := "package p\n\nconst q = `UPDATE activity SET body = NULL` + ` WHERE id = $1`\n"
		if err := os.WriteFile(whole, []byte(wholeSource), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := assembledSweepTargets(t, whole); len(got) != 0 {
			t.Errorf("two literals joined were reported as assembled: %v", got)
		}
		// Parentheses are not part of the expression, and a reader that treats
		// them as one is wrong in both directions: the fragment goes unseen on
		// the joined side, and on the other side an ordinary statement reads as
		// half-assembled.
		parens := filepath.Join(dir, "parens.go")
		parenSource := "package p\n\nfunc f(extra string) string {\n" +
			"\treturn (`UPDATE activity SET body = NULL`) + extra\n}\n\n" +
			"func g() string {\n\treturn `UPDATE person SET note = NULL` + (` WHERE id = $1`)\n}\n"
		if err := os.WriteFile(parens, []byte(parenSource), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		got = assembledSweepTargets(t, parens)
		if got[table] == "" {
			t.Errorf("a parenthesized fragment joined to a runtime value was not reported: %v", got)
		}
		if got["person"] != "" {
			t.Errorf("a statement joined to a parenthesized LITERAL was reported as assembled: %v", got)
		}
	})

	// A fmt verb standing where a column should be. The literal reads as a
	// finished statement — assignments present, no dangling comma — and no `+`
	// node exists for the syntax side either, so the verb is the only tell.
	t.Run("a format verb in the SET clause is reported", func(t *testing.T) {
		stmt := collapsedSQL(`UPDATE activity SET body = NULL, %s = NULL WHERE id = $1`)
		if statementDestroying([]string{stmt}, table, column) != "" {
			t.Error("a statement with no column in it was read as destroying one")
		}
		if assembledSetTarget([]string{stmt}) == "" {
			t.Error("a SET clause naming a fmt verb was read as complete — the column arrives at runtime " +
				"and the KEEPS registration is no longer checked against it")
		}
		assertOrdinaryStatementsAreNotReported(t, percentsThatAreNotVerbs...)
		// A comment mentioning a verb is not one. The collapse strips comments
		// before anything reads the clause, so this never reaches the tell —
		// which is what has to be true, or an author explaining a statement in
		// a comment makes the gate report it.
		if got := assembledSetTarget([]string{collapsedSQL("UPDATE activity SET body = NULL /* was %s once */ WHERE id = $1")}); got != "" {
			t.Errorf("a comment mentioning a verb was reported as assembled: %q", got)
		}
		// And the verb after a static assignment, which is the position an
		// assembled sweep would really produce.
		behind := collapsedSQL(`UPDATE activity SET body = NULL, %s = NULL WHERE id = $1`)
		if assembledSetTarget([]string{behind}) == "" {
			t.Error("a verb standing where a later column should be was read as complete")
		}
	})

	// A fragment handed to an assembler. Sprintf and a builder leave no `+`
	// node, and the fragment can read as a whole statement.
	t.Run("a fragment handed to an assembler is reported", func(t *testing.T) {
		dir := t.TempDir()
		const head = "package p\n\nfunc f(extra string) string {\n"
		for _, tc := range []struct{ name, body string }{
			{"sprintf", "\treturn fmt.Sprintf(`UPDATE activity SET body = NULL, %s = NULL`, extra)\n"},
			{"join", "\treturn strings.Join([]string{`UPDATE activity SET body = NULL`, extra}, \" \")\n"},
			{"builder", "\tvar b strings.Builder\n\tb.WriteString(`UPDATE activity SET body = NULL`)\n\tb.WriteString(extra)\n\treturn b.String()\n"},
			{"replace", "\treturn strings.Replace(`UPDATE activity SET body = NULL`, `NULL`, extra, 1)\n"},
		} {
			path := filepath.Join(dir, tc.name+".go")
			if err := os.WriteFile(path, []byte(head+tc.body+"}\n"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := assembledSweepTargets(t, path); got[table] == "" {
				t.Errorf("%s: a fragment handed to an assembler was not reported: %v", tc.name, got)
			}
		}
		// The whole statement passed to a QUERY call is not an assembly, or
		// every statement in the tree would be a finding.
		whole := filepath.Join(dir, "exec.go")
		wholeSource := "package p\n\ntype T interface{ Exec(c C, q string) }\ntype C interface{}\n\nfunc g(tx T, ctx C) { tx.Exec(ctx, `UPDATE activity SET body = NULL WHERE id = $1`) }\n"
		if err := os.WriteFile(whole, []byte(wholeSource), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := assembledSweepTargets(t, whole); len(got) != 0 {
			t.Errorf("a statement passed whole to a query call was reported as assembled: %v", got)
		}
	})

	// The two halves of the report, and the reason it is one function: dropping
	// either arm is silent, because each answers for shapes the other cannot
	// see.
	t.Run("both halves of the unreadable report are consulted", func(t *testing.T) {
		fromText := []string{collapsedSQL(`UPDATE activity SET `)}
		if unreadableWriteOn(fromText, "") == "" {
			t.Error("an unfinished fragment went unreported when the syntax side had nothing to say")
		}
		ordinary := []string{collapsedSQL(`UPDATE activity SET body = NULL WHERE id = $1`)}
		if unreadableWriteOn(ordinary, `update activity set body = null`) == "" {
			t.Error("a fragment the SYNTAX reported went unreported because the text read it as complete — " +
				"this is the arm a caller keeping only assembledSetTarget would drop")
		}
		if got := unreadableWriteOn(ordinary, ""); got != "" {
			t.Errorf("an ordinary statement was reported as unreadable: %q", got)
		}
	})

	// The quoting the split cannot track. Each form ends with the scan's
	// `quoted` inverted against the truth, so the split cuts inside a string —
	// and the fragment carrying the destructive assignment names no table. The
	// gate refuses to read these rather than trusting a scan it has lost.
	t.Run("quoting the split cannot track is refused, not guessed at", func(t *testing.T) {
		for _, unreadable := range []string{
			`UPDATE activity SET body = E'a\';b', counterparty_email = NULL WHERE id = $1`,
		} {
			if quotingBeyondTheSplit(unreadable) == "" {
				t.Errorf("quoting the split cannot track went unreported: %q", unreadable)
			}
			// And it must be refused BEFORE the split, not survive it: read
			// through, the fragment holding the destruction names no table.
			if statementDestroying(sqlStatements(unreadable), table, column) != "" {
				t.Errorf("the split read %q correctly by luck; the refusal above is what this case rests on", unreadable)
			}
		}
		// Everything it CAN track is not refused, or the sweep's own statements
		// would stop being read at all — and the last two are the expensive
		// mistake: an E'' MENTIONED inside a value or a comment is not one, and
		// a tripwire that refuses a statement it reads perfectly well is one
		// somebody turns off.
		for _, readable := range []string{
			`UPDATE activity SET body = 'a;b', counterparty_email = NULL WHERE id = $1`,
			`UPDATE activity SET body = NULL WHERE id = $1 AND kind = 'note'`,
			`UPDATE person SET note = 'she said ''no''' WHERE id = $1`,
			"UPDATE activity SET body = $$a;b$$, counterparty_email = NULL WHERE id = $1",
			`UPDATE person SET note = 'E''s own note' WHERE id = $1`,
			"UPDATE person SET note = NULL -- not an E'scape\n WHERE id = $1",
		} {
			if form := quotingBeyondTheSplit(readable); form != "" {
				t.Errorf("ordinary quoting was refused as %q: %s", form, readable)
			}
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

// percentsThatAreNotVerbs are ordinary SQL texts carrying a `%`. Each is
// entirely readable, and reporting one would send an author to rewrite a
// statement that is fine — which is how a tripwire gets turned off.
var percentsThatAreNotVerbs = []string{
	`UPDATE activity SET body = 'template %s' WHERE id = $1`,
	`UPDATE activity SET query = 'foo%bar' WHERE id = $1`,
	`UPDATE activity SET body = '100% useful' WHERE id = $1`,
	`UPDATE activity SET body = NULL WHERE subject LIKE '100%'`,
}

func assertOrdinaryStatementsAreNotReported(t *testing.T, statements ...string) {
	t.Helper()
	for _, ordinary := range statements {
		if got := assembledSetTarget([]string{collapsedSQL(ordinary)}); got != "" {
			t.Errorf("an ordinary statement was reported as assembled: %q", got)
		}
	}
}
