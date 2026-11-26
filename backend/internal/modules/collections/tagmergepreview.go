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

// TagMergeAssignment marks overlaps so a merge review distinguishes moves from collapsed assignments.
type TagMergeAssignment struct {
	EntityType string
	EntityID   ids.UUID
	Collapsed  bool
}

// TagMergePreview keeps the compared tag names with the assignments they govern.
type TagMergePreview struct {
	SourceName  string
	TargetName  string
	Assignments []TagMergeAssignment
}

// PreviewTagMerge checks both versions and every affected record before disclosing the merge scope.
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
		out.Assignments, err = previewTagAssignments(ctx, tx, source, target)
		if err != nil {
			return err
		}
		out.SourceName, out.TargetName = from.Name, to.Name
		return nil
	})
	if err != nil {
		return TagMergePreview{}, err
	}
	return out, nil
}

func previewTagAssignments(ctx context.Context, tx pgx.Tx, source, target ids.TagID) ([]TagMergeAssignment, error) {
	rows, err := tx.Query(ctx, `SELECT s.entity_type, s.entity_id, `+tagMergeOverlap+`
	 FROM taggable s WHERE s.tag_id = @source ORDER BY s.entity_type, s.entity_id LIMIT 501`, pgx.StrictNamedArgs{tagMergeSourceArg: source, tagMergeTargetArg: target})
	if err != nil {
		return nil, err
	}
	assignments, err := pgx.CollectRows(rows, pgx.RowToStructByPos[TagMergeAssignment])
	if err != nil {
		return nil, err
	}
	if len(assignments) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, assignment := range assignments {
		if !acceptsEntityType(assignment.EntityType) {
			return nil, apperrors.ErrInvalidArgument
		}
		if err := auth.Require(ctx, assignment.EntityType, principal.ActionRead); err != nil {
			return nil, err
		}
		if err := auth.EnsureVisibleLive(ctx, tx, assignment.EntityType, assignment.EntityID); err != nil {
			return nil, err
		}
	}
	return assignments, nil
}
