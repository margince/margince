// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal merge does with each row that names the merged-away deal.
// TestEveryDealForeignKeyJoinsTheMerge reads these statements and its waiver
// map, so a table pointing at deal is moved here or declared there.
//
// A row the survivor already holds under the same unique key stands, and the
// retired copy is dropped or archived. That is the rule recordmerge.RelinkLinkRows
// uses for links, applied to the rest.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkDealReferences moves every reference to the source onto the target and
// answers the tally for the audit rows.
func (s *Store) relinkDealReferences(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID) (mergeTally, error) {
	var tally mergeTally
	var err error
	if tally.Offers, err = moveMerge(ctx, tx, `UPDATE offer SET deal_id = $2 WHERE deal_id = $1`, sourceID, targetID); err != nil {
		return tally, err
	}
	if tally.ActivityLinks, err = recordmerge.RelinkLinkRows(ctx, tx, dealTable, sourceID.UUID, targetID.UUID); err != nil {
		return tally, fmt.Errorf("relink activity, list and tag rows: %w", err)
	}
	if err := s.moveOwnedRefs(ctx, tx, sourceID, targetID, &tally); err != nil {
		return tally, err
	}
	if _, err = moveMerge(ctx, tx, `UPDATE legal_review SET deal_id = $2 WHERE deal_id = $1`, sourceID, targetID); err != nil {
		return tally, err
	}
	if err := s.installation.CarryGrants(ctx, tx, dealTable, sourceID.UUID, targetID.UUID); err != nil {
		return tally, fmt.Errorf("carry the merged-away deal's grants: %w", err)
	}
	if err := recordmerge.MoveKindIDRows(ctx, tx, dealTable, sourceID.UUID, targetID.UUID); err != nil {
		return tally, err
	}
	return tally, nil
}

// moveOwnedRefs asks every module that keeps rows about a deal to move them,
// and records what each moved. A module writes only its own tables, so each
// owner's seam does its own statements and audit rows.
func (s *Store) moveOwnedRefs(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID, tally *mergeTally) error {
	tally.Moved = map[string]int64{}
	for _, move := range s.installation.MoveDealRefs {
		moved, err := move(ctx, tx, sourceID.UUID, targetID.UUID)
		if err != nil {
			return fmt.Errorf("move what another module keeps about the merged-away deal: %w", err)
		}
		for _, rows := range moved {
			tally.Moved[rows.Table] += int64(len(rows.IDs))
		}
	}
	tally.Stakeholders, tally.Contracts = tally.Moved["relationship"], tally.Moved["contract"]
	return nil
}

// refuseDoubleHolders refuses a merge where both deals hold what a deal holds
// once. That is a live commission, an open legal review, a live chat link or an
// open Deal Room. Moving the source's would break the unique index. Dropping
// either would lose a decision.
func refuseDoubleHolders(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID) error {
	var commission, review, chat, room bool
	err := tx.QueryRow(ctx, `SELECT
  EXISTS (SELECT 1 FROM commission_entry WHERE deal_id = $1 AND status <> 'void' AND reversal_of IS NULL)
  AND EXISTS (SELECT 1 FROM commission_entry WHERE deal_id = $2 AND status <> 'void' AND reversal_of IS NULL),
  EXISTS (SELECT 1 FROM legal_review WHERE deal_id = $1 AND status = 'requested')
  AND EXISTS (SELECT 1 FROM legal_review WHERE deal_id = $2 AND status = 'requested'),
  EXISTS (SELECT 1 FROM chat_link WHERE deal_id = $1 AND archived_at IS NULL)
  AND EXISTS (SELECT 1 FROM chat_link WHERE deal_id = $2 AND archived_at IS NULL),
  EXISTS (SELECT 1 FROM deal_room WHERE deal_id = $1 AND archived_at IS NULL AND state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused'))
  AND EXISTS (SELECT 1 FROM deal_room WHERE deal_id = $2 AND archived_at IS NULL AND state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused'))`,
		sourceID, targetID).Scan(&commission, &review, &chat, &room)
	if err != nil {
		return fmt.Errorf("check what both deals hold: %w", err)
	}
	for _, held := range []struct {
		both bool
		what string
	}{
		{commission, "a live commission"}, {review, "an open legal review"},
		{chat, "a live chat link"}, {room, "an open Deal Room"},
	} {
		if held.both {
			return &MergeRefusedError{Code: mergeRefusedBoth,
				Message: "both deals hold " + held.what + ", and a deal holds only one; close one of them first"}
		}
	}
	return nil
}

// execMerge runs one relink statement with the source as $1 and the target as $2.
func execMerge(ctx context.Context, tx pgx.Tx, stmt string, sourceID, targetID ids.DealID) error {
	_, err := moveMerge(ctx, tx, stmt, sourceID, targetID)
	return err
}

// moveMerge is execMerge answering how many rows the statement changed.
func moveMerge(ctx context.Context, tx pgx.Tx, stmt string, sourceID, targetID ids.DealID) (int64, error) {
	tag, err := tx.Exec(ctx, stmt, sourceID, targetID)
	if err != nil {
		return 0, fmt.Errorf("relink a deal reference: %w", err)
	}
	return tag.RowsAffected(), nil
}
