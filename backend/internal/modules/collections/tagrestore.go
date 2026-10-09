// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reinsertTag serves both an un-archive and a removal's restore, so the two put a tagging back alike.
const reinsertTag = `INSERT INTO taggable (id, tag_id, entity_type, entity_id, assigned_by, assigned_by_kind, assigned_at, created_at)
	SELECT COALESCE(@row_id::uuid, uuidv7()), @tag_id, @entity_type, @entity_id, @assigned_by, @assigned_by_kind,
	       @assigned_at, COALESCE(@row_created_at::timestamptz, now())
	WHERE EXISTS (SELECT 1 FROM tag WHERE id = @tag_id AND archived_at IS NULL)
	ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING
	RETURNING id, tag_id, entity_type, entity_id, created_at`

func reinsertTagArgs(entityType string, id ids.UUID, kept storekit.TagAssignment, rowID *ids.UUID, rowCreatedAt *time.Time) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{
		"row_id": rowID, "tag_id": kept.TagID, entityTypeField: entityType, entityIDField: id,
		"assigned_by": kept.AssignedBy, "assigned_by_kind": kept.AssignedByKind, "assigned_at": kept.AssignedAt,
		"row_created_at": rowCreatedAt,
	}
}

// RestoreTagRemoval puts back the tagging the caller's own removal took off,
// with the assignment it had, behind the removal's own gates.
func (s *Store) RestoreTagRemoval(ctx context.Context, tagID ids.TagID, removalID ids.UUID) (taggableRow, error) {
	var out taggableRow
	if err := httperr.RequireBodyID(auditIDField, removalID); err != nil {
		return out, err
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = restoreTagRemovalTx(ctx, tx, tagID, removalID, nil)
		return err
	})
	return out, err
}

// RestoreBatchTagRemovalTx is RestoreTagRemoval for a bulk undo: it puts back a
// removal the batch wrote, under the undo's authority over that batch.
func (s *Store) RestoreBatchTagRemovalTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, removalID, batchID ids.UUID) (taggableRow, error) {
	return restoreTagRemovalTx(ctx, tx, tagID, removalID, &batchID)
}

func restoreTagRemovalTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, removalID ids.UUID, batch *ids.UUID) (taggableRow, error) {
	var out taggableRow
	removed, err := ownRemoval(ctx, tx, "tag", tagID.UUID, removalID, batch)
	if err != nil {
		return out, err
	}
	var record linkImage
	if err := removed.decode(&record); err != nil {
		return out, err
	}
	if record.TagAssignment == nil {
		return out, ErrRemovalUnkept
	}
	if err := requireTagRemoval(ctx, record.EntityType, record.EntityID); err != nil {
		return out, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, record.EntityType, record.EntityID); err != nil {
		return out, err
	}
	if err := refuseRetiredTag(ctx, tx, tagID); err != nil {
		return out, err
	}
	if err := refuseMovedOn(ctx, tx, "tag", tagID.UUID, removed, record); err != nil {
		return out, err
	}
	kept := *record.TagAssignment
	kept.TagID = tagID.UUID
	err = tx.QueryRow(ctx, reinsertTag, reinsertTagArgs(record.EntityType, record.EntityID, kept, record.RowID, record.RowCreatedAt)).
		Scan(&out.ID, &out.TagID, &out.EntityType, &out.EntityID, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrRemovalMovedOn
	}
	if err != nil {
		return out, fmt.Errorf("put the tagging back: %w", err)
	}
	_, err = auditTagLink(storekit.WithReversal(ctx, "tag", tagID.UUID, removalID), tx, tagID, tagApplied,
		linkImage{EntityType: record.EntityType, EntityID: record.EntityID}, nil)
	return out, err
}

// An archived tag takes no new taggings, so the restore refuses it as apply does.
func refuseRetiredTag(ctx context.Context, tx pgx.Tx, tagID ids.TagID) error {
	var archived *time.Time
	err := tx.QueryRow(ctx, `SELECT archived_at FROM tag WHERE id = $1`, tagID).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if archived != nil {
		return ErrTagRetiredRestore
	}
	return nil
}

func restoreArchivedTag(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, kept storekit.TagAssignment) (bool, error) {
	return storekit.TryInSavepoint(ctx, tx, reinsertTag, reinsertTagArgs(entityType, id, kept, nil, nil))
}
