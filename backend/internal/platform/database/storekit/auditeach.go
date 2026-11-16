// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditEachChunk bounds one statement: 5 binds a row, under Postgres's 65535.
const auditEachChunk = 1000

// AuditEach is Audit for many rows that changed the same way, such as rows a
// merge moved from one record to another: one audit row each, written in a few
// statements rather than one per row.
//
//craft:ignore naked-any the audit seam: before/after images are each entity's own snapshot shape, serialized to jsonb
func AuditEach(ctx context.Context, tx pgx.Tx, action, entityType string, entityIDs []ids.UUID, before, after any) error {
	if bindsTheBeforeImageRule(action) && AbsentImage(before) {
		return fmt.Errorf("store: auditing an update of %s with no before-image: record what the fields held", entityType)
	}
	rows := make([]auditRow, 0, min(len(entityIDs), auditEachChunk))
	for _, id := range entityIDs {
		row, err := prepareAuditRow(ctx, entityType, id, before, after, nil)
		if err != nil {
			return err
		}
		rows = append(rows, row)
		if len(rows) == auditEachChunk {
			if err := insertAuditRows(ctx, tx, action, entityType, rows); err != nil {
				return err
			}
			rows = rows[:0]
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return insertAuditRows(ctx, tx, action, entityType, rows)
}
