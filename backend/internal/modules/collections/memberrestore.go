// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		removed, err := ownRemoval(ctx, tx, listObject, listID.UUID, removalID)
		if err != nil {
			return err
		}
		var record linkImage
		if err := removed.decode(&record); err != nil {
			return err
		}
		if record.ListMembership == nil {
			return ErrRemovalUnkept
		}
		change := MemberChange{EntityType: record.EntityType, EntityID: record.EntityID, Reason: ReasonChosen}
		if err := admitMemberChange(ctx, tx, listID, change); err != nil {
			return err
		}
		if err := refuseMovedOn(ctx, tx, listObject, listID.UUID, removed, record); err != nil {
			return err
		}
		memberNote, err := removedMemberNote(ctx, tx, listID, removed, record)
		if err != nil {
			return err
		}
		actor, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		kept := *record.ListMembership
		kept.ListID, kept.Note = listID.UUID, memberNote
		err = rowScanMember(tx.QueryRow(ctx, reinsertMember, reinsertMemberArgs(record.EntityType, record.EntityID, record.RowID, kept)), &out)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRemovalMovedOn
		}
		if err != nil {
			return fmt.Errorf("put the membership back: %w", err)
		}
		_, err = recordMemberChange(storekit.WithReversal(ctx, listObject, listID.UUID, removalID),
			tx, listID, change, memberAdded, actor, nil)
		return err
	})
	return out, err
}

// removedMemberNote reads the note from the removal's own event row; an erasure
// that deleted the row leaves nothing to put back.
func removedMemberNote(ctx context.Context, tx pgx.Tx, listID ids.ListID, r removal, record linkImage) (*string, error) {
	var note *string
	err := tx.QueryRow(ctx, `
		SELECT member_note FROM list_member_event
		 WHERE list_id = $1 AND entity_type = $2 AND entity_id = $3 AND action = 'removed' AND occurred_at = $4
		 ORDER BY id DESC LIMIT 1`,
		listID, record.EntityType, record.EntityID, r.at).Scan(&note)
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
