// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reinsertMember serves both an un-archive and a removal's restore, so the two put a member back alike.
const reinsertMember = `INSERT INTO list_member (id, list_id, entity_type, entity_id, added_by, created_at, note)
	SELECT COALESCE(@row_id::uuid, uuidv7()), l.id, @entity_type, @entity_id, @added_by, @created_at, @note
	FROM list l WHERE l.id = @list_id AND l.archived_at IS NULL
	ON CONFLICT (list_id, entity_type, entity_id) DO NOTHING
	RETURNING id, list_id, entity_type, entity_id, added_by, created_at, note`

func reinsertMemberArgs(entityType string, id ids.UUID, rowID *ids.UUID, kept storekit.ListMembership) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{
		"row_id": rowID, listIDField: kept.ListID, entityTypeField: entityType, entityIDField: id,
		"added_by": kept.AddedBy, "created_at": kept.CreatedAt, noteField: kept.Note,
	}
}

// RestoreMemberRemoval puts back the membership the caller's own removal took
// off a Shortlist, as its author left it, behind the removal's own gates.
func (s *Store) RestoreMemberRemoval(ctx context.Context, listID ids.ListID, removalID ids.UUID) (memberRow, error) {
	var out memberRow
	if err := httperr.RequireBodyID(auditIDField, removalID); err != nil {
		return out, err
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = restoreMemberRemovalTx(ctx, tx, listID, removalID, nil)
		return err
	})
	return out, err
}

// RestoreBatchMemberRemovalTx is RestoreMemberRemoval for a bulk undo: it puts
// back a removal the batch wrote, under the undo's authority over that batch.
func (s *Store) RestoreBatchMemberRemovalTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, removalID, batchID ids.UUID) (memberRow, error) {
	return restoreMemberRemovalTx(ctx, tx, listID, removalID, &batchID)
}

func restoreMemberRemovalTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, removalID ids.UUID, batch *ids.UUID) (memberRow, error) {
	var out memberRow
	removed, err := ownRemoval(ctx, tx, listObject, listID.UUID, removalID, batch)
	if err != nil {
		return out, err
	}
	var record linkImage
	if err := removed.decode(&record); err != nil {
		return out, err
	}
	if record.ListMembership == nil {
		return out, ErrRemovalUnkept
	}
	change := MemberChange{EntityType: record.EntityType, EntityID: record.EntityID, Reason: ReasonChosen}
	if batch != nil {
		change.Reason = ReasonBulk
	}
	if err := admitMemberChange(ctx, tx, listID, change); err != nil {
		return out, err
	}
	if err := refuseMovedOn(ctx, tx, listObject, listID.UUID, removed, record); err != nil {
		return out, err
	}
	memberNote, err := removedMemberNote(ctx, tx, listID, removed, record)
	if err != nil {
		return out, err
	}
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return out, err
	}
	kept := *record.ListMembership
	kept.ListID, kept.Note = listID.UUID, memberNote
	err = rowScanMember(tx.QueryRow(ctx, reinsertMember, reinsertMemberArgs(record.EntityType, record.EntityID, record.RowID, kept)), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrRemovalMovedOn
	}
	if err != nil {
		return out, fmt.Errorf("put the membership back: %w", err)
	}
	_, err = recordMemberChange(storekit.WithReversal(ctx, listObject, listID.UUID, removalID),
		tx, listID, change, memberAdded, actor, nil)
	return out, err
}

// removedMemberNote reads the note from the removal's own event row; an erasure
// that deleted the row leaves nothing to put back.
func removedMemberNote(ctx context.Context, tx pgx.Tx, listID ids.ListID, r removal, record linkImage) (*string, error) {
	var note *string
	err := tx.QueryRow(ctx, `
		SELECT member_note FROM list_member_event
		 WHERE list_id = @list_id AND entity_type = @entity_type AND entity_id = @entity_id
		   AND action = 'removed' AND occurred_at = @at
		 ORDER BY id DESC LIMIT 1`,
		pgx.StrictNamedArgs{listIDField: listID, entityTypeField: record.EntityType, entityIDField: record.EntityID, "at": r.at},
	).Scan(&note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRemovalUnkept
	}
	if err != nil {
		return nil, fmt.Errorf("read the removed member's note: %w", err)
	}
	return note, nil
}

func restoreArchivedMembership(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, kept storekit.ListMembership) (bool, error) {
	back, err := storekit.TryInSavepoint(ctx, tx, reinsertMember, reinsertMemberArgs(entityType, id, nil, kept))
	if err != nil || !back {
		return false, err
	}
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	change := MemberChange{EntityType: entityType, EntityID: id, Reason: reasonRecordRestored}
	return true, recordMemberEvent(ctx, tx, ids.From[ids.ListKind](kept.ListID), change, memberAdded, actor, nil)
}

// ArchivedLinkRestore puts back the tags and memberships an archive took off.
func ArchivedLinkRestore() storekit.LinkRestore {
	return storekit.LinkRestore{Membership: restoreArchivedMembership, Tag: restoreArchivedTag}
}
