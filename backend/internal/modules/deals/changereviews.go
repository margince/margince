// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AppliedChangeKey binds a receipt to its record; a change ID alone must never
// attach a review to a different deal.
type AppliedChangeKey struct {
	DealID   ids.DealID
	ChangeID ids.UUID
}

// AppliedChangeReviews re-gates records even when callers already read their
// receipts. One scoped query supplies the whole panel's live review state.
func (s *Store) AppliedChangeReviews(ctx context.Context, keys []AppliedChangeKey) (map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview{}, nil
	}
	var out map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readAppliedChangeReviews(ctx, tx, keys)
		return err
	})
	return out, err
}

// AppliedChangeReviewsTx is AppliedChangeReviews inside the caller's
// transaction, for a read that must not open a second connection.
func (s *Store) AppliedChangeReviewsTx(ctx context.Context, tx pgx.Tx, keys []AppliedChangeKey) (map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	return readAppliedChangeReviews(ctx, tx, keys)
}

func readAppliedChangeReviews(ctx context.Context, tx pgx.Tx, keys []AppliedChangeKey) (map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview, error) {
	dealIDs, changeIDs := make([]ids.UUID, len(keys)), make([]ids.UUID, len(keys))
	for i, key := range keys {
		dealIDs[i], changeIDs[i] = key.DealID.UUID, key.ChangeID
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The close-date kinds are named, so a kind with no review shape reads as none.
	closeDatePos := arg([]string{
		string(CloseDateActionAutoApply), string(CloseDateActionProvisionalConfirm),
		string(CloseDateActionDowngradeAndReview), VerdictCloseDateCorrection,
	})
	dealsPos, changesPos, stagePos := arg(dealIDs), arg(changeIDs), arg(stageCorrectionKinds)
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	writable, err := auth.WriteAuthorityClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlTrue
	}
	if writable == "" {
		writable = sqlTrue
	}
	query := storekit.SQLf(`
 WITH requested AS (SELECT * FROM unnest($%d::uuid[], $%d::uuid[]) AS r(deal_id, change_id)),
 visible AS (SELECT d.id, d.version, d.stage_id, to_jsonb(d) AS fields, (%s) AS writable FROM deal d WHERE (%s) AND d.id = ANY($%d::uuid[]))
 SELECT c.deal_id, c.audit_log_id, CASE WHEN c.correction = ANY($%d::text[]) THEN 'close_date' WHEN c.correction = 'close_lost' THEN 'close_lost' WHEN c.correction = 'deal_amount' THEN 'amount' WHEN c.correction = 'deal_probability' THEN 'probability' WHEN c.correction = 'stage_move' THEN 'stage' WHEN c.correction = 'deal_open' THEN 'deal_open' WHEN c.correction = 'deal_revive' THEN 'deal_revive' END, c.accepted_at IS NOT NULL, c.reversed_at IS NOT NULL,
        d.version, c.reversed_at IS NULL AND (c.correction <> ALL($%d::text[]) OR (c.stage_history_id = (%s)
          AND EXISTS (SELECT 1 FROM deal_stage_history moved WHERE moved.id = c.stage_history_id AND moved.from_stage_id IS NOT NULL))),
        c.reversed_at IS NULL AND NOT EXISTS (SELECT 1 FROM jsonb_each(a.after) f WHERE f.key = ANY(c.fields) AND d.fields->f.key IS DISTINCT FROM f.value), d.writable
 FROM deal_correction c JOIN visible d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id AND a.entity_type = 'deal' AND a.entity_id = c.deal_id
 JOIN requested r ON r.deal_id = c.deal_id AND r.change_id = c.audit_log_id
 WHERE c.correction = ANY($%d::text[]) OR c.correction IN ('close_lost', 'deal_amount', 'deal_probability', 'stage_move', 'deal_open', 'deal_revive')`, dealsPos, changesPos, writable, scope, dealsPos, closeDatePos, stagePos, newestStageMoveSQL("c.deal_id"), closeDatePos)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview, len(keys))
	for rows.Next() {
		var key AppliedChangeKey
		var review crmcontracts.AppliedDealChangeReview
		if err := rows.Scan(&key.DealID.UUID, &key.ChangeID, &review.Kind, &review.Accepted, &review.Reversed, &review.Version, &review.CanUndo, &review.CanAccept, &review.Writable); err != nil {
			return nil, err
		}
		review.CanUndo = review.CanUndo && review.CanAccept
		out[key] = review
	}
	return out, rows.Err()
}
