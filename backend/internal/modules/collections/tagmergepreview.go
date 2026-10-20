// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type TagMergeAssignment struct {
	EntityType string
	EntityID   ids.UUID
	Collapsed  bool
}

type TagMergePreview struct {
	SourceName  string
	TargetName  string
	Assignments []TagMergeAssignment
}

func (s *Store) PreviewTagMerge(ctx context.Context, source, target ids.TagID, sourceVersion *int64, targetVersion int64) (TagMergePreview, error) {
	var out TagMergePreview
	if err := requireVocabularyAuthority(ctx, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return out, err
	}
	if source == target || source.IsZero() || target.IsZero() {
		return out, apperrors.ErrInvalidArgument
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		from, err := liveSourceTag(ctx, tx, source)
		if err != nil {
			return err
		}
		to, err := liveSourceTag(ctx, tx, target)
		if err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, "tag", source.UUID, sourceVersion); err != nil {
			return err
		}
		if err := checkTargetVersion(ctx, tx, target, targetVersion); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT s.entity_type, s.entity_id, `+tagMergeOverlap+`
		 FROM taggable s WHERE s.tag_id = @source ORDER BY s.entity_type, s.entity_id LIMIT 501`, pgx.StrictNamedArgs{"source": source, "target": target})
		if err != nil {
			return err
		}
		out.Assignments, err = pgx.CollectRows(rows, pgx.RowToStructByPos[TagMergeAssignment])
		if err != nil {
			return err
		}
		if len(out.Assignments) > 500 {
			return apperrors.ErrInvalidArgument
		}
		for _, assignment := range out.Assignments {
			if !memberEntityTables[assignment.EntityType] {
				return apperrors.ErrInvalidArgument
			}
			if err := auth.Require(ctx, assignment.EntityType, principal.ActionRead); err != nil {
				return err
			}
			if err := auth.EnsureVisibleLive(ctx, tx, assignment.EntityType, assignment.EntityID); err != nil {
				return err
			}
		}
		out.SourceName, out.TargetName = from.Name, to.Name
		return nil
	})
	if err != nil {
		return TagMergePreview{}, err
	}
	return out, nil
}
