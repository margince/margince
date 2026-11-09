// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func nameReviewEntities(ctx context.Context, tx pgx.Tx, refusals []RefusedRecipient) error {
	entities := make([]ids.UUID, 0, len(refusals))
	for _, refusal := range refusals {
		if refusal.SendingEntity != nil {
			entities = append(entities, *refusal.SendingEntity)
		}
	}
	if len(entities) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id, display_name FROM legal_entity WHERE id = ANY($1)`, entities)
	if err != nil {
		return fmt.Errorf("consent: reading review companies: %w", err)
	}
	names := make(map[ids.UUID]string)
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("consent: reading a review company: %w", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("consent: reading review companies: %w", err)
	}
	for i := range refusals {
		if refusals[i].SendingEntity != nil {
			refusals[i].SendingEntityName = names[*refusals[i].SendingEntity]
		}
	}
	return nil
}
