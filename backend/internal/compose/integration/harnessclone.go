// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CloneActivity copies an activity the product writer wrote, count times in one
// transaction. A copy carries every column the writer set, and no audit or
// event row. Rows on a boundary the test asserts should still use the writer.
//
// vary maps a column name to a SQL expression over the template row (r) and the
// copy's 1-based position (n.ord), for a column with a unique key. It applies to
// every table with the column. Write literals only.
func (e *Env) CloneActivity(t *testing.T, template ids.ActivityID, count int, vary map[string]string) []ids.UUID {
	t.Helper()
	copies := make([]ids.UUID, count)
	for i := range copies {
		copies[i] = ids.NewV7()
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := cloneRows(ctx, tx, "activity", "id", template.UUID, copies, vary); err != nil {
			return err
		}
		children, err := activityChildTables(ctx, tx)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := cloneRows(ctx, tx, child, "activity_id", template.UUID, copies, vary); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cloning activity %s %d times: %v", template, count, err)
	}
	return copies
}

// activityChildTables are the tables whose rows exist only for one activity: an
// activity_id column that cascades with it.
func activityChildTables(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT c.conrelid::regclass::text
		  FROM pg_constraint c
		  JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		 WHERE c.contype = 'f' AND c.confrelid = 'activity'::regclass AND c.confdeltype = 'c'
		   AND cardinality(c.conkey) = 1 AND a.attname = 'activity_id'
		 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

// cloneRows inserts, for each id in copies, a copy of the rows of table whose
// keyColumn equals source. A column named in vary takes its expression; an id
// column that is the table's key is minted anew.
func cloneRows(ctx context.Context, tx pgx.Tx, table, keyColumn string, source ids.UUID, copies []ids.UUID, vary map[string]string) error {
	columns, err := writableColumns(ctx, tx, table)
	if err != nil {
		return err
	}
	names, exprs := make([]string, 0, len(columns)), make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := pgx.Identifier{column}.Sanitize()
		expr := "r." + quoted
		switch {
		case column == keyColumn:
			expr = "n.id"
		case vary[column] != "":
			expr = vary[column]
		case column == "id":
			expr = "uuidv7()"
		}
		names, exprs = append(names, quoted), append(exprs, expr)
	}
	sql := fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s r CROSS JOIN unnest($2::uuid[]) WITH ORDINALITY AS n(id, ord) WHERE r.%s = $1`,
		table, strings.Join(names, ", "), strings.Join(exprs, ", "), table, pgx.Identifier{keyColumn}.Sanitize())
	_, err = tx.Exec(ctx, sql, source, copies)
	return err
}

// writableColumns are the columns an INSERT may name: not dropped, not generated.
func writableColumns(ctx context.Context, tx pgx.Tx, table string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT attname FROM pg_attribute
		 WHERE attrelid = $1::regclass AND attnum > 0 AND NOT attisdropped AND attgenerated = ''
		 ORDER BY attnum`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}
