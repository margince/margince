// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OpenSystemTaskOnTx reports whether the system already has an open task from
// this source on the record. The caller serialises it with its own write.
func OpenSystemTaskOnTx(ctx context.Context, tx pgx.Tx, sourceSystem, entityType string, entityID ids.UUID) (bool, error) {
	column := linkColumn(entityType)
	if column == "" {
		return false, &InvalidLinkTypeError{EntityType: entityType}
	}
	var open bool
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS (
		SELECT 1 FROM activity t JOIN activity_link l ON l.activity_id = t.id
		 WHERE l.%s = $1 AND t.kind = 'task' AND t.source_system = $2
		   AND t.is_done = false AND t.archived_at IS NULL)`, column),
		entityID, sourceSystem).Scan(&open)
	if err != nil {
		return false, fmt.Errorf("activities: reading the open %s task on a %s: %w", sourceSystem, entityType, err)
	}
	return open, nil
}
