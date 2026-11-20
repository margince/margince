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

// choiceRemoval is how one choice type finds a row holding a removed choice and
// what the column holds once they are gone. %[1]s is the quoted column and $1
// the removed choices. A picklist loses the value. A multiselect keeps its other
// choices in their order, and one left with none is NULL.
type choiceRemoval struct{ holds, after string }

var choiceRemovals = map[string]choiceRemoval{
	fieldcatalog.TypePicklist: {holds: `%[1]s = ANY ($1::text[])`, after: `NULL::text`},
	fieldcatalog.TypeMultiselect: {
		holds: `%[1]s && $1::text[]`,
		after: `NULLIF(ARRAY(SELECT m FROM unnest(%[1]s) WITH ORDINALITY AS u(m, n) WHERE m <> ALL ($1::text[]) ORDER BY n), '{}')`,
	},
}

// ChoiceChange is one row a removed choice leaves, in the wire shape of the
// column before and after.
//
//craft:ignore naked-any a picklist value is a string or nil and a multiselect value a []string, the shapes extractValue answers
type ChoiceChange struct {
	ID            ids.UUID
	Before, After any
}

// LockRemovedChoices locks every row of table that holds a choice removed from
// column, archived rows too, and answers what each holds now and after. It
// writes nothing, for a module whose update goes through a writer of its own.
func LockRemovedChoices(ctx context.Context, tx pgx.Tx, table string, column fieldcatalog.Column, removed []string) ([]ChoiceChange, error) {
	removal, ok := choiceRemovals[column.Type]
	if !ok {
		return nil, fmt.Errorf("store: a %s field has no choices to clear", column.Type)
	}
	if len(removed) == 0 {
		return nil, nil
	}
	if err := ensureCatalogColumn(ctx, tx, table, column); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id, %[1]s, `+removal.after+` FROM %[2]s WHERE `+removal.holds+` FOR UPDATE`,
		quoteColumnIdentifier(column.Name), pgx.Identifier{table}.Sanitize()), removed)
	if err != nil {
		return nil, fmt.Errorf("store: locking rows that hold a removed choice on %s: %w", table, err)
	}
	defer rows.Close()
	var out []ChoiceChange
	for rows.Next() {
		var change ChoiceChange
		var before, after any
		if err := rows.Scan(&change.ID, &before, &after); err != nil {
			return nil, err
		}
		change.Before, change.After = wireOrNull(column.Type, before), wireOrNull(column.Type, after)
		out = append(out, change)
	}
	return out, rows.Err()
}

// ClearRemovedChoices takes the choices an admin removed from a picklist or
// multiselect field off every row of table that holds one. Archived rows count
// too, because the field's CHECK covers them. It answers how many rows changed.
//
// Each changed row gets an audit entry and the event announce builds, as an edit
// does. The update, the audit rows and the events each go in one statement per
// auditEachChunk rows.
func ClearRemovedChoices(ctx context.Context, tx pgx.Tx, table string, column fieldcatalog.Column, removed []string,
	announce func(changed map[string]any) events.Payload,
) (int, error) {
	changes, err := LockRemovedChoices(ctx, tx, table, column, removed)
	if err != nil || len(changes) == 0 {
		return 0, err
	}
	removal := choiceRemovals[column.Type]
	update := fmt.Sprintf(`UPDATE %[2]s SET %[1]s = `+removal.after+` WHERE id = ANY ($2::uuid[]) AND `+removal.holds,
		quoteColumnIdentifier(column.Name), pgx.Identifier{table}.Sanitize())
	for start := 0; start < len(changes); start += auditEachChunk {
		chunk := changes[start:min(start+auditEachChunk, len(changes))]
		chunkIDs := make([]ids.UUID, len(chunk))
		for i, change := range chunk {
			chunkIDs[i] = change.ID
		}
		if _, err := tx.Exec(ctx, update, removed, chunkIDs); err != nil {
			return 0, fmt.Errorf("store: clearing removed choices on %s: %w", table, err)
		}
		if err := auditAndAnnounce(ctx, tx, table, column.Name, chunk, announce); err != nil {
			return 0, err
		}
	}
	return len(changes), nil
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

// wireOrNull is a scanned value in its wire shape, and nil for a cleared picklist.
//
//craft:ignore naked-any same driver-scanned-value contract as extractValue
func wireOrNull(typ string, raw any) any {
	if value, ok := extractValue(typ, raw); ok {
		return value
	}
	return nil
}

func auditAndAnnounce(ctx context.Context, tx pgx.Tx, table, columnName string, chunk []ChoiceChange,
	announce func(changed map[string]any) events.Payload,
) error {
	prepared := make([]auditRow, 0, len(chunk))
	for _, change := range chunk {
		audit, err := prepareAuditRow(ctx, table, change.ID, map[string]any{columnName: change.Before}, map[string]any{columnName: change.After}, nil)
		if err != nil {
			return err
		}
		prepared = append(prepared, audit)
	}
	auditIDs, err := insertAuditRows(ctx, tx, "update", table, prepared)
	if err != nil {
		return fmt.Errorf("store: auditing cleared choices on %s: %w", table, err)
	}
	staged := make([]stagedEvent, 0, len(chunk))
	for i, change := range chunk {
		payload := announce(map[string]any{columnName: change.After})
		event, err := stageEnvelope(ctx, auditIDs[i], payload.EventType(), payload.EntityType(), change.ID, payload)
		if err != nil {
			return fmt.Errorf("store: announcing cleared choices on %s: %w", table, err)
		}
		staged = append(staged, event)
	}
	if err := insertEvents(ctx, tx, staged); err != nil {
		return fmt.Errorf("store: announcing cleared choices on %s: %w", table, err)
	}
	return nil
}
