// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RemoveTag takes ONE tag off ONE record, leaving the tag itself alone.
//
// The vocabulary had no way back. ApplyTag added a tagging and ArchiveTag
// retired a tag from the whole workspace, so the only way to undo a mistaken
// tag was to retire the tag for everybody — which is not undo, it is a second
// mistake with a wider blast radius.
//
// Same gates as applying, for the same reasons: the target's own UPDATE
// object grant, and EnsureWritableLive because taking a tag off a record
// changes it — a caller who may only see the row must not be able to. A
// record that does not exist or is archived still answers not-found rather
// than confirming it exists by refusing differently.
// Removing a tagging that is not there is NOT an error — the caller asked for
// a state, and the state is already true (idempotent by intent, which is what
// makes a retry safe).
func (s *Store) RemoveTag(ctx context.Context, tagID ids.TagID, entityType string, entityID ids.UUID) error {
	if err := requireTagRemoval(ctx, entityType, entityID); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := removeTagTx(ctx, tx, tagID, entityType, entityID, nil)
		return err
	})
}

// RemoveTagTx is RemoveTag inside a caller-opened transaction, for a bulk
// change that takes one tag off many records in one commit. It answers whether
// the record carried the tag, since a bulk change reports the rows it changed.
func (s *Store) RemoveTagTx(ctx context.Context, tx pgx.Tx, tagID ids.TagID, entityType string, entityID ids.UUID) (bool, error) {
	if err := requireTagRemoval(ctx, entityType, entityID); err != nil {
		return false, err
	}
	return removeTagTx(ctx, tx, tagID, entityType, entityID, nil)
}

// RemoveTagAssignmentTx is RemoveTagTx held to one assignment: it takes the tag
// off only while the record still carries the very assignment named, so undoing
// a bulk tag never removes a tag somebody put back on after it. It answers
// whether that assignment was still there.
func (s *Store) RemoveTagAssignmentTx(
	ctx context.Context, tx pgx.Tx, assignment ids.UUID, tagID ids.TagID, entityType string, entityID ids.UUID,
) (bool, error) {
	if err := requireTagRemoval(ctx, entityType, entityID); err != nil {
		return false, err
	}
	return removeTagTx(ctx, tx, tagID, entityType, entityID, &assignment)
}

// requireTagRemoval is the object half of taking a tag off a record: the
// vocabulary's read grant, and UPDATE on the target, the same gate applying
// holds — taking a tag off a record changes the record, and a caller who may
// only read it must not be able to. The ROW scope is removeTagTx's.
func requireTagRemoval(ctx context.Context, entityType string, entityID ids.UUID) error {
	if err := httperr.RequireBodyID(entityIDField, entityID); err != nil {
		return err
	}
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return err
	}
	if !memberEntityTables[entityType] {
		return &BadInputError{Field: entityTypeField, Reason: "must be " + memberEntityVocabulary}
	}
	return auth.Require(ctx, entityType, principal.ActionUpdate)
}

// removeTagTx is the removal itself, shared by both entry points above.
//
// A RETIRED WORD IS STILL REMOVABLE, which is the one place this differs from
// applyTagTx and the reason the archived_at read is not shared with it.
// Applying a retired word is coining it again and is refused. Taking one OFF a
// record is the cleanup retiring the word left behind, and refusing it strands
// the record: recordTagRows returns archived assignments deliberately, so the
// surface hands a caller a retired tag it must be able to take off. Only a tag
// that never existed is not-found here.
func removeTagTx(
	ctx context.Context, tx pgx.Tx, tagID ids.TagID, entityType string, entityID ids.UUID, assignment *ids.UUID,
) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM tag WHERE id = $1)`, tagID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, apperrors.ErrNotFound
	}
	// Same reasoning as applyTagTx: removing a tag CHANGES the record too,
	// so the gate is write authority, not merely visibility.
	if err := auth.EnsureWritableLive(ctx, tx, entityType, entityID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM taggable WHERE tag_id = $1 AND entity_type = $2 AND entity_id = $3
		   AND ($4::uuid IS NULL OR id = $4)`,
		tagID, entityType, entityID, assignment)
	if err != nil {
		return false, err
	}
	// Audited only when something was actually removed: an audit row for a
	// tagging that was never there describes an event that did not happen.
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", "tag", tagID.UUID, map[string]any{
		"removed": map[string]any{"entity_type": entityType, "entity_id": entityID},
	})
	return err == nil, err
}

// CheckTagChange refuses a tag verb before any record is tried: no read grant
// on the vocabulary, a tag that does not exist, or — for putting it on — a
// retired one. Taking a retired tag off stays allowed, as removeTagTx allows it.
func (s *Store) CheckTagChange(ctx context.Context, tagID ids.TagID, applying bool) error {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var archived *time.Time
		err := tx.QueryRow(ctx, `SELECT archived_at FROM tag WHERE id = $1`, tagID).Scan(&archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && applying && archived != nil) {
			return apperrors.ErrNotFound
		}
		return err
	})
}
