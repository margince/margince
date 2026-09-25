// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every ON DELETE CASCADE foreign key can find its children by index.
//
// Postgres has to locate the child rows before it can cascade a delete, so a
// referencing column with no index makes that a sequential scan of the child
// table — inside the parent's delete transaction, holding its locks. Article 17
// erasure deletes contacts by design and cascades into five such tables at once.
//
// The set is DERIVED from pg_constraint rather than listed here. A gate that
// carried its own copy of the schema would pass over the next cascading key
// somebody adds, which is the only failure that matters: the indexes this
// asserts are cheap to create on an empty table and expensive on a full one.

import (
	"context"
	"strings"
	"testing"
)

// uncoveredCascades finds cascading foreign keys whose referencing columns are
// not the LEADING columns of some index on the child table. Leading, not merely
// present: an index on (b, a) does not serve a cascade keyed on (a).
const uncoveredCascades = `
WITH fk AS (
  SELECT c.oid, c.conrelid AS childoid, c.conrelid::regclass::text AS child, c.conkey
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
   WHERE c.contype = 'f' AND c.confdeltype = 'c' AND n.nspname = 'public'
), covered AS (
  SELECT DISTINCT fk.oid
    FROM fk
    JOIN pg_index i ON i.indrelid = fk.childoid
   WHERE (SELECT array_agg(k ORDER BY ord) FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord))
         = (SELECT array_agg(a ORDER BY ord)
              FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS x(a, ord)
             WHERE ord <= array_length(fk.conkey, 1))
)
SELECT fk.child || ' (' || (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
          FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord)
          JOIN pg_attribute att ON att.attrelid = fk.childoid AND att.attnum = u.k) || ')'
  FROM fk
 WHERE fk.oid NOT IN (SELECT oid FROM covered)
 ORDER BY 1`

// cascadeCount tallies the schema's cascading keys, indexed or not. It is the
// floor that stops this passing by reading nothing.
const cascadeCount = `
SELECT count(*) FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND c.confdeltype = 'c' AND n.nspname = 'public'`

func TestEveryCascadingKeyCanFindItsChildren(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var cascades int
	if err := conn.QueryRow(ctx, cascadeCount).Scan(&cascades); err != nil {
		t.Fatalf("counting cascading keys: %v", err)
	}
	// The schema declared thirty-odd when this landed. A count that collapses
	// means the query stopped recognising them, and every one would then read
	// as covered.
	if cascades < 20 {
		t.Fatalf("this gate found %d cascading foreign key(s) and expects at least 20 — it has "+
			"stopped recognising them rather than the schema having given them up", cascades)
	}

	rows, err := conn.Query(ctx, uncoveredCascades)
	if err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	defer rows.Close()
	var uncovered []string
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		uncovered = append(uncovered, one)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	if len(uncovered) > 0 {
		t.Errorf("%d cascading foreign key(s) have no index their delete can use:\n\t%s\n"+
			"Deleting the parent scans the whole child table, inside the parent's transaction "+
			"and holding its locks. Add an index on the referencing columns in the constraint's "+
			"own order — it is instant on an empty table and a CONCURRENTLY with a maintenance "+
			"window once there is data.",
			len(uncovered), strings.Join(uncovered, "\n\t"))
	}
}
