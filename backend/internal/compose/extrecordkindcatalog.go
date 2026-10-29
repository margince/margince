// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// recordKindQuerier is the one read VerifyRecordKinds makes, so a pool and a
// transaction both serve it.
type recordKindQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// kindTableShape reads four facts about one ext table. Does it exist? Which
// columns does it have? Does a unique index cover the natural key and nothing
// else? Does the connected role hold all four table rights? A partial or
// expression index does not count, because it lets two live rows share a key.
const kindTableShape = `
SELECT to_regclass($1) IS NOT NULL,
       COALESCE((SELECT array_agg(a.attname::text) FROM pg_attribute a
                  WHERE a.attrelid = to_regclass($1) AND a.attnum > 0 AND NOT a.attisdropped), '{}'),
       EXISTS (SELECT 1 FROM pg_index i
                WHERE i.indrelid = to_regclass($1) AND i.indisunique AND i.indpred IS NULL AND i.indexprs IS NULL
                  AND i.indnatts = cardinality($2::text[])
                  AND ARRAY(SELECT a.attname::text FROM pg_attribute a
                             WHERE a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey::int2[])
                             ORDER BY a.attname) = ARRAY(SELECT unnest($2::text[]) ORDER BY 1)),
       COALESCE(has_table_privilege(current_user, to_regclass($1), 'SELECT')
            AND has_table_privilege(current_user, to_regclass($1), 'INSERT')
            AND has_table_privilege(current_user, to_regclass($1), 'UPDATE')
            AND has_table_privilege(current_user, to_regclass($1), 'DELETE'), false)`

// VerifyRecordKinds checks each declared kind against the schema the unit
// shipped. The table must exist in the ext schema and keep `id`, `archived_at`
// and every declared field. A unique index must cover the natural key and
// nothing else. Without these, core would link rows it cannot find, or let two
// rows share a key the kind says is unique.
func VerifyRecordKinds(ctx context.Context, q recordKindQuerier, kinds []recordkinds.Declared) error {
	for _, k := range kinds {
		var exists, unique, granted bool
		var columns []string
		qualified := pgx.Identifier{"ext", k.Table}.Sanitize()
		if err := q.QueryRow(ctx, kindTableShape, qualified, k.NaturalKey).Scan(&exists, &columns, &unique, &granted); err != nil {
			return fmt.Errorf("record kind %s: reading %s: %w", k.Name, qualified, err)
		}
		if !exists {
			return fmt.Errorf("record kind %s: table %s does not exist; apply the unit's migrations", k.Name, qualified)
		}
		if !granted {
			return fmt.Errorf("record kind %s: the application role lacks select, insert, update or delete on %s; grant all four to it", k.Name, qualified)
		}
		required := []string{"id", "archived_at"}
		for _, f := range k.Fields {
			required = append(required, f.Name)
		}
		for _, c := range required {
			if !slices.Contains(columns, c) {
				return fmt.Errorf("record kind %s: table %s has no column %q", k.Name, qualified, c)
			}
		}
		if !unique {
			return fmt.Errorf("record kind %s: table %s has no unique index over exactly its natural key %v", k.Name, qualified, k.NaturalKey)
		}
	}
	return nil
}
