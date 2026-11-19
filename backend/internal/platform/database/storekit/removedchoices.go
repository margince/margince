// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// A picklist loses the value. A multiselect keeps its other choices in their
// order, and one left with none is NULL, as a record that never held a choice.
const (
	clearPicklistChoice = `WITH hit AS (SELECT id, %[2]s AS before FROM %[1]s WHERE %[2]s = ANY ($1::text[]) FOR UPDATE)
		UPDATE %[1]s t SET %[2]s = NULL FROM hit WHERE t.id = hit.id
		RETURNING t.id, hit.before, t.%[2]s`
	clearMultiselectChoices = `WITH hit AS (SELECT id, %[2]s AS before FROM %[1]s WHERE %[2]s && $1::text[] FOR UPDATE)
		UPDATE %[1]s t SET %[2]s = NULLIF(ARRAY(SELECT m FROM unnest(hit.before) WITH ORDINALITY AS u(m, n)
		                                         WHERE m <> ALL ($1::text[]) ORDER BY n), '{}')
		  FROM hit WHERE t.id = hit.id
		RETURNING t.id, hit.before, t.%[2]s`
)

// ClearRemovedChoices takes the choices an admin removed from a picklist or
// multiselect field off every row of table that holds one. Archived rows count
// too, because the field's CHECK covers them. It answers how many rows changed.
//
// Each changed row gets an audit entry and the event announce builds, as an edit
// does. Only rows holding a removed choice are touched, and the audit rows go in
// statements of auditEachChunk rows.
func ClearRemovedChoices(ctx context.Context, tx pgx.Tx, table string, column fieldcatalog.Column, removed []string,
	announce func(changed map[string]any) events.Payload,
) (int, error) {
	if len(removed) == 0 {
		return 0, nil
	}
	var stmt string
	switch column.Type {
	case fieldcatalog.TypePicklist:
		stmt = clearPicklistChoice
	case fieldcatalog.TypeMultiselect:
		stmt = clearMultiselectChoices
	default:
		return 0, fmt.Errorf("store: a %s field has no choices to clear", column.Type)
	}
	if err := ensureCatalogColumn(ctx, tx, table, column); err != nil {
		return 0, err
	}
	cleared, err := clearChoiceRows(ctx, tx, fmt.Sprintf(stmt, pgx.Identifier{table}.Sanitize(), quoteColumnIdentifier(column.Name)), column, removed)
	if err != nil {
		return 0, fmt.Errorf("store: clearing removed choices on %s: %w", table, err)
	}
	for start := 0; start < len(cleared); start += auditEachChunk {
		chunk := cleared[start:min(start+auditEachChunk, len(cleared))]
		if err := auditAndAnnounce(ctx, tx, table, column.Name, chunk, announce); err != nil {
			return 0, err
		}
	}
	return len(cleared), nil
}

// ensureCatalogColumn refuses a column the custom-field catalog does not list
// on table with this type. The column becomes an identifier in an UPDATE, so a
// caller naming its own could otherwise write any column of the table.
func ensureCatalogColumn(ctx context.Context, tx pgx.Tx, table string, column fieldcatalog.Column) error {
	var listed bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM custom_field WHERE object = $1 AND column_name = $2 AND type = $3)`,
		table, column.Name, column.Type).Scan(&listed)
	if err != nil {
		return fmt.Errorf("store: reading the custom-field catalog: %w", err)
	}
	if !listed {
		return fmt.Errorf("store: %s is not a %s custom field of %s", column.Name, column.Type, table)
	}
	return nil
}

// clearedChoice is one row a removed choice left, in its wire shape.
//
//craft:ignore naked-any a picklist value is a string or nil and a multiselect value a []string, the shapes extractValue answers
type clearedChoice struct {
	id            ids.UUID
	before, after any
}

func clearChoiceRows(ctx context.Context, tx pgx.Tx, stmt string, column fieldcatalog.Column, removed []string) ([]clearedChoice, error) {
	rows, err := tx.Query(ctx, stmt, removed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clearedChoice
	for rows.Next() {
		var row clearedChoice
		var before, after any
		if err := rows.Scan(&row.id, &before, &after); err != nil {
			return nil, err
		}
		row.before, row.after = wireOrNull(column.Type, before), wireOrNull(column.Type, after)
		out = append(out, row)
	}
	return out, rows.Err()
}

// wireOrNull is a scanned value in its wire shape, and nil for a cleared picklist.
//
//craft:ignore naked-any same driver-scanned-value contract as extractValue
func wireOrNull(typ string, raw any) any {
	if value, ok := extractValue(typ, raw); ok {
		return value
	}
	return nil
}

func auditAndAnnounce(ctx context.Context, tx pgx.Tx, table, columnName string, chunk []clearedChoice,
	announce func(changed map[string]any) events.Payload,
) error {
	prepared := make([]auditRow, 0, len(chunk))
	for _, row := range chunk {
		audit, err := prepareAuditRow(ctx, table, row.id, map[string]any{columnName: row.before}, map[string]any{columnName: row.after}, nil)
		if err != nil {
			return err
		}
		prepared = append(prepared, audit)
	}
	auditIDs, err := insertAuditRows(ctx, tx, "update", table, prepared)
	if err != nil {
		return fmt.Errorf("store: auditing cleared choices on %s: %w", table, err)
	}
	for i, row := range chunk {
		if err := EmitEvent(ctx, tx, auditIDs[i], row.id, announce(map[string]any{columnName: row.after})); err != nil {
			return fmt.Errorf("store: announcing cleared choices on %s: %w", table, err)
		}
	}
	return nil
}
