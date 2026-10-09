// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Re-homing the rows that name a record by kind and id.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// moveKindIDRows moves the polymorphic references of a merged-away contact or
// company. They carry no foreign key, so the schema cannot stop an
// (entity_type, entity_id) pair from pointing at an archived record.
func moveKindIDRows(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	for _, stmt := range []string{
		`UPDATE signal SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
		`UPDATE attachment SET entity_id = $2 WHERE entity_type = $3 AND entity_id = $1`,
	} {
		if _, err := tx.Exec(ctx, stmt, sourceID, targetID, entityType); err != nil {
			return fmt.Errorf("move the rows naming the %s by kind and id: %w", entityType, err)
		}
	}
	return nil
}
