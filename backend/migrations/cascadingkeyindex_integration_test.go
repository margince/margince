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
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// uncoveredCascades finds cascading foreign keys whose referencing columns are
// not the LEADING columns of some usable index on the child table. Leading, not
// merely present: an index on (b, a) does not serve a cascade keyed on (a) —
// Postgres will happily scan that index end to end instead, which is the
// sequential scan again with more pages.
//
// Usable excludes an invalid index, which is never read, and a partial one,
// whose predicate the cascade's `col = $1` does not imply — `WHERE archived_at
// IS NULL` leaves out the rows an erasure still has to delete. `col IS NOT NULL`
// is the one predicate equality does imply, so those indexes count.
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
   WHERE i.indisvalid
     AND (i.indpred IS NULL
          OR (array_length(fk.conkey, 1) = 1
              AND pg_get_expr(i.indpred, i.indrelid)
                  = '(' || (SELECT att.attname FROM pg_attribute att
                             WHERE att.attrelid = fk.childoid AND att.attnum = fk.conkey[1])
                    || ' IS NOT NULL)'))
     AND (SELECT array_agg(k ORDER BY ord) FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord))
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
	// The schema declared 232 when this landed. A count that collapses means the
	// query stopped recognising them, and every one would then read as covered.
	if cascades < 200 {
		t.Fatalf("this gate found %d cascading foreign key(s) and expects at least 200 — it has "+
			"stopped recognising them rather than the schema having given them up", cascades)
	}

	if uncovered := readUncovered(ctx, t, conn); len(uncovered) > 0 {
		t.Errorf("%d cascading foreign key(s) have no index their delete can use:\n\t%s\n"+
			"Deleting the parent scans the whole child table, inside the parent's transaction "+
			"and holding its locks. Add an index on the referencing columns in the constraint's "+
			"own order — it is instant on an empty table and a CONCURRENTLY with a maintenance "+
			"window once there is data.",
			len(uncovered), strings.Join(uncovered, "\n\t"))
	}
}

// TestTheCascadeIndexQueryNoticesAWithdrawnIndex takes an index the schema is
// relying on away again, inside a transaction it rolls back. An empty result is
// the one way the gate above can fail short — a `covered` clause that matches
// everything reads as a clean schema — so the discrimination is proven rather
// than assumed.
func TestTheCascadeIndexQueryNoticesAWithdrawnIndex(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rollback: %v", err)
		}
	}()

	const anIndexACascadeUses = `
SELECT i.indexrelid::regclass::text, fk.child || ' (' || fk.cols || ')'
  FROM (SELECT c.conrelid AS childoid, c.conrelid::regclass::text AS child, c.conkey,
               (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
                  FROM unnest(c.conkey) WITH ORDINALITY AS u(k, ord)
                  JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = u.k) AS cols
          FROM pg_constraint c
          JOIN pg_class t ON t.oid = c.conrelid
          JOIN pg_namespace n ON n.oid = t.relnamespace
         WHERE c.contype = 'f' AND c.confdeltype = 'c' AND n.nspname = 'public') fk
  JOIN pg_index i ON i.indrelid = fk.childoid
 WHERE i.indisvalid AND i.indpred IS NULL AND NOT i.indisprimary
   AND (SELECT array_agg(k ORDER BY ord) FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord))
       = (SELECT array_agg(a ORDER BY ord)
            FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS x(a, ord)
           WHERE ord <= array_length(fk.conkey, 1))
 ORDER BY 1 LIMIT 1`

	var index, key string
	if err := tx.QueryRow(ctx, anIndexACascadeUses).Scan(&index, &key); err != nil {
		t.Fatalf("finding an index a cascade uses: %v", err)
	}
	if _, err := tx.Exec(ctx, "DROP INDEX "+pgx.Identifier{index}.Sanitize()); err != nil {
		t.Fatalf("dropping %s: %v", index, err)
	}

	uncovered := readUncovered(ctx, t, tx)
	if !slices.Contains(uncovered, key) {
		t.Errorf("dropped %s, the only index serving the cascade on %s, and the query still "+
			"reported it covered — it is matching something other than a usable index, so a "+
			"schema with no index at all would read as clean", index, key)
	}
}

// readUncovered runs uncoveredCascades against whatever the caller has — the
// connection, or a transaction holding a withdrawn index.
func readUncovered(ctx context.Context, t *testing.T, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
},
) []string {
	t.Helper()
	rows, err := q.Query(ctx, uncoveredCascades)
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
	return uncovered
}
