// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The delete actions a foreign-key census asks about, as a predicate over the
// constraint aliased c. A cascade deletes the child, while the rest only find it.
const (
	CascadingKeys = `c.confdeltype = 'c'`
	OtherKeys     = `c.confdeltype <> 'c'`
)

// uncoveredKeysTemplate finds the foreign keys of one action that no usable
// index on the child table leads with. An index on (b, a) does not serve a key
// on (a) alone. Postgres would scan that index end to end, which is the
// sequential scan again with more pages.
//
// The leading columns compare as a set, not in the constraint's order. The
// referential probe constrains every key column by equality at once. So a btree
// on (b, a) seeks just as directly for a key on (a, b).
//
// Usable excludes an invalid index, which is never read, and a partial one,
// whose predicate the cascade's `col = $1` does not imply. `col IS NOT NULL` is
// the one predicate equality does imply, so those indexes count.
const uncoveredKeysTemplate = `
WITH fk AS (
  SELECT c.oid, c.conrelid AS childoid, c.conrelid::regclass::text AS child,
         c.confrelid::regclass::text AS parent, c.conkey, c.conname
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
   WHERE c.contype = 'f' AND n.nspname = $1 AND %s
), covered AS (
  SELECT DISTINCT fk.oid
    FROM fk
    JOIN pg_index i ON i.indrelid = fk.childoid
   WHERE i.indisvalid
     -- indkey carries the INCLUDE columns too, and those are payload: an index on
     -- (b) INCLUDE (a) stores a but cannot seek on it.
     AND i.indnkeyatts >= array_length(fk.conkey, 1)
     AND (i.indpred IS NULL
          OR (array_length(fk.conkey, 1) = 1
              AND pg_get_expr(i.indpred, i.indrelid)
                  = '(' || (SELECT att.attname FROM pg_attribute att
                             WHERE att.attrelid = fk.childoid AND att.attnum = fk.conkey[1])
                    || ' IS NOT NULL)'))
     AND (SELECT array_agg(k ORDER BY k) FROM unnest(fk.conkey) AS u(k))
         = (SELECT array_agg(a ORDER BY a)
              FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS x(a, ord)
             WHERE ord <= array_length(fk.conkey, 1))
)
SELECT fk.child || '.' || fk.conname,
       fk.child || ' (' || (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
          FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord)
          JOIN pg_attribute att ON att.attrelid = fk.childoid AND att.attnum = u.k) || ')',
       fk.parent
  FROM fk
 WHERE fk.oid NOT IN (SELECT oid FROM covered)
 ORDER BY 1`

// keyCountTemplate tallies the keys of one action in one schema, indexed or
// not. It is the floor that stops a census passing by reading nothing.
const keyCountTemplate = `
SELECT count(*) FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = $1 AND %s`

// UncoveredKey is one foreign key no usable index serves. Constraint is its
// identity, child.conname, because two keys on the same columns to different
// parents read alike in columns.
type UncoveredKey struct {
	Constraint, Columns, Parent string
}

func (u UncoveredKey) String() string {
	return u.Columns + " -> " + u.Parent + " [" + u.Constraint + "]"
}

// Querier is a connection or a transaction, so a caller can read the census
// inside a transaction that withdrew an index.
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// UncoveredForeignKeys answers every foreign key of one action, on the tables of
// one schema, that no usable index serves.
func UncoveredForeignKeys(ctx context.Context, q Querier, schema, action string) ([]UncoveredKey, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(uncoveredKeysTemplate, action), schema)
	if err != nil {
		return nil, fmt.Errorf("testdb: reading the uncovered foreign keys of %s: %w", schema, err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (UncoveredKey, error) {
		var key UncoveredKey
		err := row.Scan(&key.Constraint, &key.Columns, &key.Parent)
		return key, err
	})
}

// ForeignKeyCount counts the foreign keys of one action on the tables of one
// schema.
func ForeignKeyCount(ctx context.Context, q Querier, schema, action string) (int, error) {
	var n int
	if err := q.QueryRow(ctx, fmt.Sprintf(keyCountTemplate, action), schema).Scan(&n); err != nil {
		return 0, fmt.Errorf("testdb: counting the foreign keys of %s: %w", schema, err)
	}
	return n, nil
}
