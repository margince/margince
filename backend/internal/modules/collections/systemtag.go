// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ApplySystemTagTx files each record under the tag with this name, as
// tagNamedTx finds or makes it. A word a user already made is reused. A record
// already filed there counts as done, so a second run changes nothing. It
// answers how many records it tagged. System principal only.
func ApplySystemTagTx(ctx context.Context, tx pgx.Tx, name, entityType string, entityIDs []ids.UUID, evidence map[string]any) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	if !memberEntityTables[entityType] {
		return 0, &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	if len(entityIDs) == 0 {
		return 0, nil
	}
	tagID, err := tagNamedTx(ctx, tx, name)
	if err != nil {
		return 0, err
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

// tagNamedTx is the tag holding this name, live first. An archived one is
// restored rather than coined again, so its records stay under one word.
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
		restored, err := restoreTagTx(ctx, tx, id)
		return restored.ID, err
	}
	return id, nil
}
