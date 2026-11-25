// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal merge does with each row that names the merged-away deal.
// TestEveryDealForeignKeyJoinsTheMerge reads these statements and the waiver
// map beside it, so a table that points at deal is either moved here or
// declared there with the reason it stays.
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
	if tally.Stakeholders, err = relinkStakeholders(ctx, tx, sourceID, targetID); err != nil {
		return tally, err
	}
	if tally.Contracts, err = moveMerge(ctx, tx, `UPDATE contract SET deal_id = $2 WHERE deal_id = $1`, sourceID, targetID); err != nil {
		return tally, err
	}
	if err := relinkDealOwnedRows(ctx, tx, sourceID, targetID); err != nil {
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

// relinkStakeholders archives a stakeholder edge the survivor already holds
// (same contact, same role) and moves the rest, archived ones included so the
// provenance travels.
func relinkStakeholders(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID) (int64, error) {
	if err := execMerge(ctx, tx, `UPDATE relationship a SET archived_at = now()
 WHERE a.deal_id = $1 AND a.archived_at IS NULL AND EXISTS (
   SELECT 1 FROM relationship b
    WHERE b.deal_id = $2 AND b.kind = a.kind AND b.archived_at IS NULL
      AND b.contact_id IS NOT DISTINCT FROM a.contact_id AND b.role IS NOT DISTINCT FROM a.role)`, sourceID, targetID); err != nil {
		return 0, err
	}
	return moveMerge(ctx, tx, `UPDATE relationship SET deal_id = $2 WHERE deal_id = $1`, sourceID, targetID)
}

// relinkDealOwnedRows moves the rows other modules keep about one deal. Each
// pair is a drop of the retired copy where the survivor holds the same unique
// key, then the move.
func relinkDealOwnedRows(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID) error {
	for _, stmt := range []string{
		`DELETE FROM brief_item a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM brief_item b WHERE b.deal_id = $2 AND b.brief_run_id = a.brief_run_id)`,
		`UPDATE brief_item SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE lead SET qualified_deal_id = $2 WHERE qualified_deal_id = $1`,
		`UPDATE record_assignment a SET archived_at = now()
 WHERE a.deal_id = $1 AND a.archived_at IS NULL AND EXISTS (
   SELECT 1 FROM record_assignment b
    WHERE b.deal_id = $2 AND b.role_id = a.role_id AND b.archived_at IS NULL
      AND ((a.user_id IS NOT NULL AND b.user_id = a.user_id) OR (a.team_id IS NOT NULL AND b.team_id = a.team_id)))`,
		`UPDATE record_assignment SET deal_id = $2 WHERE deal_id = $1`,
		`DELETE FROM activity_retention_evidence a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM activity_retention_evidence b
    WHERE b.deal_id = $2 AND b.activity_id = a.activity_id AND b.basis = a.basis
      AND b.deal_name IS NOT DISTINCT FROM a.deal_name AND b.project_id IS NOT DISTINCT FROM a.project_id
      AND b.project_name IS NOT DISTINCT FROM a.project_name)`,
		`UPDATE activity_retention_evidence SET deal_id = $2 WHERE deal_id = $1`,
		`DELETE FROM deal_document_hide a WHERE a.deal_id = $1 AND EXISTS (
   SELECT 1 FROM deal_document_hide b WHERE b.deal_id = $2 AND b.attachment_id = a.attachment_id)`,
		`UPDATE deal_document_hide SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE sdr_handoff SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE deal_room SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE chat_link SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE commission_entry SET deal_id = $2 WHERE deal_id = $1`,
		`UPDATE legal_review SET deal_id = $2 WHERE deal_id = $1`,
	} {
		if err := execMerge(ctx, tx, stmt, sourceID, targetID); err != nil {
			return err
		}
	}
	return nil
}

// refuseDoubleHolders refuses a merge where both deals hold something the
// schema lets a deal hold once: a live commission, an open legal review, a
// live chat link or an open Deal Room. Moving the source's would break the
// unique index, and dropping either would lose a decision.
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
