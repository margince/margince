// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// The trigger-written-column reader driven with SYNTHETIC statements, for the
// reason retainedcolumncases_test.go gives for its own: the tree is supposed to
// pass, so a reader proven only by "nothing in the tree trips it" is one that
// keeps passing after it stops working.
//
// Every MISSED row below is a shape an earlier version of the reader could not
// see, each one a dead assignment left standing. Every KEPT row is the opposite
// failure and matters as much: this gate's finding ends in a DELETION, so a
// reader that over-recognizes deletes live code. The `version = $3` predicate is
// the sharpest of those — it is the compare-and-set updateguard_test.go requires
// on the same statements, and a gate that called it a dead write would send the
// next author to remove their own concurrency guard.
//
// WHAT IS STILL NOT ON THIS LIST, stated because a list that reads as complete
// is what stops the next author looking:
//
//   - A trigger that fires CONDITIONALLY — `BEFORE UPDATE OF a, b` or one with
//     a `WHEN (…)` predicate — is not read as touching the table at all, so
//     assignments on it are never called dead. channel_connection has one
//     today. That is deliberate under-recognition: the finding is a deletion,
//     and the honest failure direction is to leave live code alone.
//   - A statement built by fmt.Sprintf or a query builder reaches the reader as
//     a literal with a hole in it. gatekit.ConcatenatedString folds a `+` chain
//     and leaves a space where the runtime value was, so the surrounding SQL
//     still reads; a table or column name that only exists at runtime does not,
//     and the write is invisible. Nothing in the swept tree does this.

import (
	"strings"
	"testing"
)

func TestTheTriggerColumnReaderSeesEveryDeadAssignment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		statement string
		// table -> the columns the reader must report as written.
		writes map[string][]string
		// wantUnreadable marks a statement the reader must REFUSE to judge
		// rather than read as clean.
		wantUnreadable bool
	}{
		{
			name:      "the plain dead assignment",
			statement: `UPDATE organization SET display_name = $2, updated_at = now() WHERE id = $1`,
			writes:    map[string][]string{"organization": {"display_name", "updated_at"}},
		}, {
			name:      "no spaces around the equals",
			statement: `UPDATE organization SET display_name=$2, version=version+1 WHERE id = $1`,
			writes:    map[string][]string{"organization": {"display_name", "version"}},
		}, {
			// The shape execute.go writes: UPDATE … FROM with the target
			// aliased. Its assignments are UNQUALIFIED, because Postgres
			// requires that on the left of a SET — here and everywhere.
			name:      "an aliased target",
			statement: `UPDATE provider_connection c SET status = $2, updated_at = now() FROM provider_connection was WHERE was.id = c.id`,
			writes:    map[string][]string{"provider_connection": {"status", "updated_at"}},
		}, {
			// Tolerance rather than a shape this tree writes: a qualified
			// target is not legal SQL, and reading it as the column it names
			// costs nothing while SKIPPING it would go quiet.
			name:      "a qualified target is read as the column it names",
			statement: `UPDATE provider_connection c SET c.status = $2, c.updated_at = now() WHERE c.id = $1`,
			writes:    map[string][]string{"provider_connection": {"status", "updated_at"}},
		}, {
			// A clause word inside a VALUE. The assignment list ends at the
			// literal's own `from` to a scan that does not track quotes, and
			// everything after it — which is where the dead assignment is —
			// becomes invisible. Reordering an assignment list is not
			// something anybody reviews as a schema change.
			name:      "a clause word inside a quoted value",
			statement: `UPDATE activity SET subject = 'a note from the report where it began', updated_at = now() WHERE id = $1`,
			writes:    map[string][]string{"activity": {"subject", "updated_at"}},
		}, {
			// The same trap on the comma, and this one fails in the direction
			// that costs most. A separator inside a string value is not a
			// separator; split there, the text after it reads as an assignment
			// of its own — so a trigger column MENTIONED in a sentence becomes
			// a trigger column WRITTEN, and this gate's finding is an
			// instruction to delete something that was never there.
			name:      "a comma and a trigger column named inside a quoted value",
			statement: `UPDATE activity SET subject = 'we moved it, updated_at = the field they meant', body = NULL WHERE id = $1`,
			writes:    map[string][]string{"activity": {"subject", "body"}},
		}, {
			// A doubled quote is how SQL escapes one. Parity carries it: the
			// run ends at the first, a new run opens at the second, and it
			// closes where the real one does.
			name:      "a doubled quote inside a quoted value",
			statement: `UPDATE activity SET subject = 'it''s from here', updated_at = now() WHERE id = $1`,
			writes:    map[string][]string{"activity": {"subject", "updated_at"}},
		}, {
			// A run that never CLOSES. Swallowed as one span, every assignment
			// after the open quote is stepped over — so a dead updated_at
			// behind it goes unseen and the gate passes, which is the
			// direction this gate must not fail in. It reaches the reader only
			// as a PIECE of an assembled statement, so it is reported instead:
			// the write is judged as unreadable rather than as clean.
			name:           "an unterminated quoted value",
			statement:      `UPDATE activity SET subject = 'the closing quote is in another literal, updated_at = now()`,
			writes:         map[string][]string{"activity": {}},
			wantUnreadable: true,
		}, {
			// Dollar-quoting, where a quote character has no meaning at all.
			name:      "a dollar-quoted value carrying a clause word",
			statement: `UPDATE activity SET subject = $tag$ where it's from $tag$, updated_at = now() WHERE id = $1`,
			writes:    map[string][]string{"activity": {"subject", "updated_at"}},
		}, {
			name:      "the alias spelled with AS",
			statement: `UPDATE organization AS o SET display_name = $2, updated_at = now() WHERE o.id = $1`,
			writes:    map[string][]string{"organization": {"display_name", "updated_at"}},
		}, {
			// SET is a legal identifier, so an aliasless statement can read as
			// one whose alias is `SET` — and then the reader finds no
			// assignment list and reports nothing at all.
			name:      "a column whose name begins with the word SET",
			statement: `UPDATE organization SET settlement_at = $2, updated_at = now() WHERE id = $1`,
			writes:    map[string][]string{"organization": {"settlement_at", "updated_at"}},
		}, {
			// The upsert arm. A BEFORE UPDATE trigger fires on it exactly as on
			// a plain UPDATE, and the table it writes is the one the INSERT
			// named — which is nowhere near the word UPDATE.
			name: "an ON CONFLICT DO UPDATE arm",
			statement: `INSERT INTO relationship (project_id, organization_id, role) VALUES ($1, $2, $3) ` +
				`ON CONFLICT (project_id, organization_id) DO UPDATE SET role = EXCLUDED.role, version = relationship.version + 1 RETURNING id`,
			writes: map[string][]string{"relationship": {"role", "version"}},
		}, {
			// The shape retentionrestricted.go already writes. A lazy regex
			// ending the SET clause at the first FROM stops INSIDE the
			// subquery, so every assignment after it — here the dead one —
			// becomes invisible, and reordering the list turns the check off.
			name: "an assignment after a subquery carrying its own FROM and WHERE",
			statement: `UPDATE activity SET redacted_fields = ARRAY(SELECT c FROM unnest(ARRAY['body']) AS c WHERE c IS NOT NULL), ` +
				`version = version + 1 WHERE id = $1`,
			writes: map[string][]string{"activity": {"redacted_fields", "version"}},
		}, {
			// Two writes in one statement, and only the second is on a
			// trigger-touched table. Read as one write, the reader either
			// misses it or attributes it to the wrong table.
			name: "a CTE that updates two tables",
			statement: `WITH target AS (SELECT id FROM activity WHERE id = ANY($1) FOR UPDATE), ` +
				`stripped AS (UPDATE activity a SET subject = NULL, updated_at = now() FROM target t WHERE a.id = t.id) ` +
				`UPDATE activity_link SET person_id = NULL WHERE activity_id = ANY($1)`,
			writes: map[string][]string{
				"activity":      {"subject", "updated_at"},
				"activity_link": {"person_id"},
			},
		}, {
			// The CAS the same statements are REQUIRED to carry
			// (updateguard_test.go). Reading a WHERE predicate as an
			// assignment would report every guarded update in the tree and
			// send the next author to delete their own guard.
			name:      "a version compared in the WHERE, not assigned",
			statement: `UPDATE organization SET display_name = $2 WHERE id = $1 AND version = $3`,
			writes:    map[string][]string{"organization": {"display_name"}},
		}, {
			// Same trap one clause further on.
			name:      "a trigger column returned, not assigned",
			statement: `UPDATE signal SET archived_at = now() WHERE id = $1 RETURNING id, updated_at, version`,
			writes:    map[string][]string{"signal": {"archived_at"}},
		}, {
			// And inside a subquery's own predicate, at depth.
			name:      "a trigger column inside a subquery predicate",
			statement: `UPDATE organization SET display_name = (SELECT name FROM staging WHERE updated_at = $2) WHERE id = $1`,
			writes:    map[string][]string{"organization": {"display_name"}},
		}, {
			// The assignment list split at PAREN DEPTH ZERO, not on every
			// comma. A comma inside a call, followed by an equality on a bare
			// column, reads as a second assignment to a reader that splits
			// naively — and this gate's finding is a DELETION, so it would send
			// the next author to remove a live comparison.
			name:      "a comparison after a comma inside a call",
			statement: `UPDATE organization SET meta = jsonb_build_object('bumped', version = $3), display_name = $2 WHERE id = $1`,
			writes:    map[string][]string{"organization": {"meta", "display_name"}},
		}, {
			name:      "a longer column that merely starts with a trigger column's name",
			statement: `UPDATE organization SET updated_at_source = $2 WHERE id = $1`,
			writes:    map[string][]string{"organization": {"updated_at_source"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string][]string{}
			for _, segment := range writeSegments(tc.statement) {
				if segment.unreadable != tc.wantUnreadable {
					t.Fatalf("on %s unreadable = %v, want %v", segment.table, segment.unreadable, tc.wantUnreadable)
				}
				if segment.unreadable {
					// Its assignments are not judged: what stands after the
					// open quote could be anything.
					got[segment.table] = []string{}
					continue
				}
				got[segment.table] = append(got[segment.table], assignedColumns(segment.assignments)...)
			}
			if len(got) != len(tc.writes) {
				t.Fatalf("read %v, want %v", got, tc.writes)
			}
			for table, want := range tc.writes {
				if strings.Join(got[table], ",") != strings.Join(want, ",") {
					t.Errorf("on %s read %v, want %v", table, got[table], want)
				}
			}
		})
	}
}

// The derivation the rulings rest on, driven against the catalog rather than
// asserted. A table read as touched when it is not turns this gate into a
// code-deleting one.
func TestTheTouchedTableDerivationReadsTheCatalog(t *testing.T) {
	t.Parallel()
	touched := touchTriggerTables(t)

	// Both arms of triggerWrites reach the tree, or half the table is a claim
	// nothing exercises.
	if got := touched["organization"]; !got["updated_at"] || !got["version"] {
		t.Errorf("organization carries set_updated_at_bump_version and reads as %v", got)
	}
	if got := touched["app_user"]; !got["updated_at"] || got["version"] {
		t.Errorf("app_user carries set_updated_at, which does not bump version, and reads as %v", got)
	}
	// A CONDITIONAL touch trigger is not read as touching at all. This is the
	// under-recognition the gate chooses on purpose, and channel_connection is
	// the table that has it: its trigger carries `WHEN ((to_jsonb(old.*) -
	// 'poll_offset') IS DISTINCT FROM …)`, so a poll-offset-only write does not
	// fire it and an assignment there is not dead.
	if got := touched["channel_connection"]; got != nil {
		t.Errorf("channel_connection's touch trigger is WHEN-conditional and must not be read as unconditional; reads as %v", got)
	}
	// A table with no touch trigger at all is absent rather than empty, or
	// every statement in the tree would be judged against nothing.
	if got := touched["activity_link"]; got != nil {
		t.Errorf("activity_link has no touch trigger and reads as %v", got)
	}
}
