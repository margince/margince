// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rowMaps runs one query and returns each row as column→value.
func rowMaps(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(values))
		for i, field := range rows.FieldDescriptions() {
			row[field.Name] = readableValue(values[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// readableValue renders one column value as the document's reader needs it.
//
// A uuid column arrives from pgx as [16]byte, which encoding/json writes as
// sixteen numbers. A data subject must receive an id that reads as one.
// TestTheSubjectAccessPackageIsReadable holds it.
//
//craft:ignore naked-any a column value IS any by construction — pgx's rows.Values() returns []any, and this rewrites one of those in place. A concrete parameter would mean deciding the column's type before reading it, which is the thing the caller cannot do.
func readableValue(value any) any {
	if raw, isUUID := value.([16]byte); isUUID {
		return ids.UUID(raw).String()
	}
	return value
}
