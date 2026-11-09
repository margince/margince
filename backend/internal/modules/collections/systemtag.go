// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ApplySystemTagTx files each record under the tag with this name, as
// tagNamedTx finds or makes it. Human removals and archived words are kept.
func ApplySystemTagTx(ctx context.Context, tx pgx.Tx, name, entityType string, entityIDs []ids.UUID, evidence map[string]any) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	if !acceptsEntityType(entityType) {
		return 0, &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	if len(entityIDs) == 0 {
		return 0, nil
	}
	tagID, err := tagNamedTx(ctx, tx, name)
	if err != nil {
		return 0, err
	}
	if tagID.UUID == ids.Nil {
		return 0, nil
	}
	// Maintenance must not reverse a user's explicit removal.
	rows, err := tx.Query(ctx, `SELECT candidate FROM unnest(@entity_ids::uuid[]) candidate
 WHERE NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.entity_type = 'tag'
 AND a.entity_id = @tag AND a.actor_type NOT IN ('system', 'connector')
 AND a.after -> 'removed' ->> 'entity_type' = @entity_type
 AND a.after -> 'removed' ->> 'entity_id' = candidate::text)`,
		pgx.NamedArgs{"entity_ids": entityIDs, "tag": tagID, "entity_type": entityType})
	if err != nil {
		return 0, fmt.Errorf("collections: reading prior tag decisions: %w", err)
	}
	entityIDs, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("collections: reading prior tag decisions: %w", err)
	}
	applied := 0
	for _, id := range entityIDs {
		_, err := applyTagTx(ctx, tx, tagID, entityType, id, evidence)
		if errors.Is(err, apperrors.ErrConflict) {
			continue
		}
		if err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// tagNamedTx reuses a live word and leaves an archived word retired.
// A name no tag ever held is coined.
func tagNamedTx(ctx context.Context, tx pgx.Tx, name string) (ids.TagID, error) {
	name, err := ValidateTagName(name)
	if err != nil {
		return ids.TagID{}, err
	}
	var id ids.TagID
	var archived bool
	err = tx.QueryRow(ctx, `
		SELECT id, archived_at IS NOT NULL FROM tag WHERE lower(name) = lower($1)
		 ORDER BY archived_at IS NOT NULL, archived_at DESC LIMIT 1`, name).Scan(&id, &archived)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		coined, err := insertTagTx(ctx, tx, name, nil, nil)
		return coined.ID, err
	case err != nil:
		return ids.TagID{}, err
	case archived:
		return ids.TagID{}, nil
	}
	return id, nil
}
