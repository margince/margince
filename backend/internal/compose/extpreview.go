// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// previewProjection reads the columns and rows of a filter preview for the
// matched ids. A core table's projection is the export's, schema-derived and
// masked. An extension kind has no schema the export knows, so its columns are
// `id` and the fields it declared, less any the caller's masks withhold.
func previewProjection(ctx context.Context, tx pgx.Tx, engine storekit.Query, matched []ids.UUID) ([]string, [][]any, error) {
	if engine.Object == "" {
		columns, err := exportableColumns(ctx, tx, engine.Table)
		if err != nil {
			return nil, nil, err
		}
		rows, err := readRowsByID(ctx, tx, engine.Table, columns, matched)
		return columns, rows, err
	}
	return extensionPreviewRows(ctx, tx, engine, matched)
}

// extensionPreviewRows reads an extension kind's matched rows. The columns come
// from the engine's own fields, which are the declared ones, so the preview can
// name nothing the filter could not.
func extensionPreviewRows(ctx context.Context, tx pgx.Tx, engine storekit.Query, matched []ids.UUID) ([]string, [][]any, error) {
	columns := []string{"id"}
	for name, field := range engine.Fields {
		if !field.Withheld {
			columns = append(columns, name)
		}
	}
	slices.Sort(columns[1:])
	if len(matched) == 0 {
		return columns, nil, nil
	}
	selects := make([]string, len(columns))
	for i, column := range columns {
		selects[i] = "t." + pgx.Identifier{column}.Sanitize()
	}
	sql := fmt.Sprintf("SELECT %s FROM %s t WHERE t.id = ANY($1) ORDER BY t.id", strings.Join(selects, ", "), engine.Table)
	pgRows, err := tx.Query(ctx, sql, matched)
	if err != nil {
		return nil, nil, fmt.Errorf("filter preview read %s: %w", engine.Table, err)
	}
	defer pgRows.Close()
	var rows [][]any
	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, nil, err
		}
		rows = append(rows, values)
	}
	return columns, rows, pgRows.Err()
}
