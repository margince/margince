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

// ApplySystemTagTx files each record under the live tag with this name, and
// coins the tag when no live one holds it, so a word a user already made is
// reused. A record already filed there counts as done, which lets a correction
// run twice and change nothing the second time. It answers how many records it
// tagged. System principal only.
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
	tagID, err := liveTagNamedTx(ctx, tx, name)
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

// liveTagNamedTx is the live tag holding this name, coined when none does.
func liveTagNamedTx(ctx context.Context, tx pgx.Tx, name string) (ids.TagID, error) {
	name, err := ValidateTagName(name)
	if err != nil {
		return ids.TagID{}, err
	}
	var id ids.TagID
	err = tx.QueryRow(ctx, `SELECT id FROM tag WHERE lower(name) = lower($1) AND archived_at IS NULL`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.TagID{}, err
	}
	coined, err := insertTagTx(ctx, tx, name, nil, nil)
	return coined.ID, err
}
