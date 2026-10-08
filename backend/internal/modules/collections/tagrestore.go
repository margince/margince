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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	tagApplied = "applied"
	tagRemoved = "removed"
)

// reinsertTag serves both an un-archive and a removal's restore, so the two put a tagging back alike.
const reinsertTag = `INSERT INTO taggable (id, tag_id, entity_type, entity_id, assigned_by, assigned_by_kind, assigned_at, created_at)
	SELECT COALESCE($7::uuid, uuidv7()), $1, $2, $3, $4, $5, $6, COALESCE($8::timestamptz, now())
	WHERE EXISTS (SELECT 1 FROM tag WHERE id = $1 AND archived_at IS NULL)
	ON CONFLICT (tag_id, entity_type, entity_id) DO NOTHING
	RETURNING id, tag_id, entity_type, entity_id, created_at`

func auditTagLink(ctx context.Context, tx pgx.Tx, tagID ids.TagID, change string, link linkImage) (ids.UUID, error) {
	return storekit.AuditEvent(ctx, tx, "update", "tag", tagID.UUID, map[string]any{change: link})
}

// RestoreTagRemoval puts back the tagging the caller's own removal took off,
// with the assignment it had, behind the removal's own gates.
func (s *Store) RestoreTagRemoval(ctx context.Context, tagID ids.TagID, removalID ids.UUID) (taggableRow, error) {
	var out taggableRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		removed, err := ownRemoval(ctx, tx, "tag", tagID.UUID, removalID)
		if err != nil {
			return err
		}
		var record linkImage
		if err := removed.decode(&record); err != nil {
			return err
		}
		if record.TagAssignment == nil {
			return ErrRemovalUnkept
		}
		if err := requireTagRemoval(ctx, record.EntityType, record.EntityID); err != nil {
			return err
		}
		if err := refuseRetiredTag(ctx, tx, tagID); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(ctx, tx, record.EntityType, record.EntityID); err != nil {
			return err
		}
		if err := refuseMovedOn(ctx, tx, "tag", tagID.UUID, removed, record); err != nil {
			return err
		}
		kept := record.TagAssignment
		err = tx.QueryRow(ctx, reinsertTag, tagID, record.EntityType, record.EntityID,
			kept.AssignedBy, kept.AssignedByKind, kept.AssignedAt, record.RowID, record.RowCreatedAt,
		).Scan(&out.ID, &out.TagID, &out.EntityType, &out.EntityID, &out.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRemovalMovedOn
		}
		if err != nil {
			return fmt.Errorf("put the tagging back: %w", err)
		}
		_, err = auditTagLink(storekit.WithReversal(ctx, "tag", tagID.UUID, removalID), tx, tagID, tagApplied,
			linkImage{EntityType: record.EntityType, EntityID: record.EntityID})
		return err
	})
	return out, err
}

// refuseRetiredTag refuses a retired word as apply does: restoring it would coin it again.
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
	return storekit.TryInSavepoint(ctx, tx, reinsertTag,
		kept.TagID, entityType, id, kept.AssignedBy, kept.AssignedByKind, kept.AssignedAt, nil, nil)
}
