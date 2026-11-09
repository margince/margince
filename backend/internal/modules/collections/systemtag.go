// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ApplySystemTagTx files each record under the tag with this name, as
// tagNamedTx finds or makes it. Human removals and archived words are kept.
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
	if tagID.UUID == ids.Nil {
		return 0, nil
	}
	entityIDs = slices.Clone(entityIDs)
	slices.SortFunc(entityIDs, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	applied := 0
	for _, id := range entityIDs {
		_, err := applyTagWithRemovalPolicyTx(ctx, tx, tagID, entityType, id, evidence, true)
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

func lockTagAssignmentTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, entityType string, entityID ids.UUID) error {
	return storekit.LockWriteIdentity(ctx, tx, "tag_assignment", tagID.String()+":"+entityType+":"+entityID.String())
}
