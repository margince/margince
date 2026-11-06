// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sendingEntityTx names the legal entity a user sends for: the live entity
// they work for, else the installation's default entity. A zero sender is
// automatic work, which sends for the default entity.
func sendingEntityTx(ctx context.Context, tx pgx.Tx, sender ids.UUID) (ids.UUID, error) {
	var entity *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT coalesce(
		    (SELECT le.id FROM legal_entity_member m
		       JOIN legal_entity le ON le.id = m.legal_entity_id AND le.archived_at IS NULL
		      WHERE m.user_id = $1),
		    (SELECT id FROM legal_entity WHERE is_default))`, zeroAsNull(sender)).Scan(&entity)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: reading the sending legal entity: %w", err)
	}
	if entity == nil {
		return ids.UUID{}, errors.New("consent: this installation has no default legal entity")
	}
	return *entity, nil
}
