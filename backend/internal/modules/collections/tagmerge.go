// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Folding one word of the vocabulary into another.
//
// Its own file rather than another entry in tagvocab.go: a merge is the one
// vocabulary act that writes two tag rows and every taggable row between them,
// and it carries the preconditions that go with that — both rows locked in id
// order, the survivor's version pinned, and the overlap counted before
// anything moves.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeResult is what a merge did, in the two numbers that differ.
type MergeResult struct {
	Moved     int
	Collapsed int
}

// MergeTags folds one tag into another: every record carrying the source ends
// up carrying the target, the source is archived, and its name is released.
//
// Moved and collapsed are counted separately because they are different facts.
// A record that carried only the source is MOVED — the target gains it. One
// that already carried both COLLAPSES — its duplicate tagging is dropped and
// the target gains nothing. An admin reading "12 moved, 3 collapsed" knows the
// target grew by 12; a single total of 15 would tell them the wrong number.
//
// The source's NAME IS RELEASED rather than kept as an alias. That is a
// deliberate product decision and it has a cost the caller has to be told
// about: links to the old tag stop working, searching its name finds nothing,
// and somebody can coin the same word again next month.
//
// expectedTargetVersion pins the SURVIVING tag. The routed id pins the row this
// destroys, and for a long time that was the whole precondition — leaving the
// word being folded into free to be renamed between a caller's decision and
// this act. Zero means unpinned, which is what a caller sending no version
// gets and what every door that has not been taught to send one passes.
// It carries no per-record row gate, unlike applyTagTx/RemoveTag: it rewrites
// every taggable row across the workspace as one vocabulary operation. No
// SEEDED role pairs tag.update with a bounded row scope — every role holding
// it today is unbounded — but the pairing is an object grant an admin could
// still set (identity.Service.SetRoleObjectGrant), which this gate does not
// itself refuse.
func (s *Store) MergeTags(ctx context.Context, source, target ids.TagID, expectedTargetVersion int64) (MergeResult, error) {
	if err := auth.Require(ctx, "tag", principal.ActionUpdate); err != nil {
		return MergeResult{}, err
	}
	// An absent into_tag_id decodes to the zero UUID with no error, so without
	// this it reaches a lookup that matches nothing and the caller is told a
	// tag they never named does not exist.
	if err := httperr.RequireBodyID(intoTagIDField, target.UUID); err != nil {
		return MergeResult{}, err
	}
	if source == target {
		return MergeResult{}, &BadInputError{Field: intoTagIDField, Reason: "must name a different tag"}
	}
	var out MergeResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Both rows, lowest id first. The count, the move, the delete and the
		// archive are four statements: without the lock a concurrent apply can
		// land between the count and the move, and the numbers reported to the
		// admin describe a state that never existed. Ordering by id is what
		// stops two merges of the same pair deadlocking on each other.
		first, second := source, target
		if second.String() < first.String() {
			first, second = second, first
		}
		for _, id := range []ids.TagID{first, second} {
			if _, err := storekit.LockRow(ctx, tx, "tag", id.UUID, storekit.IncludeArchived); err != nil {
				return err
			}
		}
		if err := requireLiveTag(ctx, tx, target, intoTagIDField); err != nil {
			return err
		}
		if err := checkTargetVersion(ctx, tx, target, expectedTargetVersion); err != nil {
			return err
		}
		sourceRow, err := scanTag(tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", source))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Count the overlap BEFORE moving anything: once the update runs, a
		// record that carried both is indistinguishable from one that carried
		// only the source.
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM taggable s
			 WHERE s.tag_id = $1
			   AND EXISTS (SELECT 1 FROM taggable t
			                WHERE t.tag_id = $2 AND t.entity_type = s.entity_type
			                  AND t.entity_id = s.entity_id)`,
			source, target).Scan(&out.Collapsed); err != nil {
			return err
		}
		moved, err := tx.Exec(ctx, `
			UPDATE taggable SET tag_id = $2
			 WHERE tag_id = $1
			   AND NOT EXISTS (SELECT 1 FROM taggable t
			                    WHERE t.tag_id = $2 AND t.entity_type = taggable.entity_type
			                      AND t.entity_id = taggable.entity_id)`, source, target)
		if err != nil {
			return err
		}
		out.Moved = int(moved.RowsAffected())
		// The duplicates the update skipped: the record already carries the
		// target, so the source's row is redundant rather than movable.
		if _, err := tx.Exec(ctx, `DELETE FROM taggable WHERE tag_id = $1`, source); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tag SET archived_at = now()
			 WHERE id = $1 AND archived_at IS NULL`, source); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", source.UUID,
			map[string]any{nameField: sourceRow.Name},
			map[string]any{
				"merged_into": target.UUID,
				"moved":       out.Moved,
				"collapsed":   out.Collapsed,
			})
		return err
	})
	return out, err
}

// requireLiveTag refuses a tag that is missing or retired, naming the field
// that carried it so the caller knows which id to fix.
func requireLiveTag(ctx context.Context, tx pgx.Tx, id ids.TagID, field string) error {
	var archived *string
	err := tx.QueryRow(ctx, `SELECT archived_at::text FROM tag WHERE id = $1`, id).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return &BadInputError{Field: field, Reason: "no such tag"}
	}
	if err != nil {
		return err
	}
	if archived != nil {
		return &BadInputError{Field: field, Reason: "names a tag that was archived"}
	}
	return nil
}

// checkTargetVersion holds the SURVIVOR's pin.
//
// Read under the lock the merge has already taken, so the check and the merge
// cannot be separated by another writer: a version read outside it would pass
// against a rename that lands before the merge's own statements.
//
// A merge is a two-row act that, until this pin, had a one-row precondition —
// the routed id pins the word being retired, and the word being folded INTO
// was free. So the survivor could be renamed between the moment a caller read
// it and the moment the merge ran, and the merge went ahead as though it had
// not been. That matters most on the confirm-first path, where a human
// approves a sentence naming both words.
//
// Zero means the caller sent none, the same reading UpdateTag takes of an
// absent If-Match.
func checkTargetVersion(ctx context.Context, tx pgx.Tx, target ids.TagID, expected int64) error {
	if expected == 0 {
		return nil
	}
	row, err := scanTag(tx.QueryRow(ctx,
		"SELECT "+tagColumns+" FROM tag WHERE id = $1", target))
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Version != expected {
		return apperrors.ErrVersionSkew
	}
	return nil
}
