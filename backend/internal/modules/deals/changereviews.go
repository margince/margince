// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"time"

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
		out, err = readAppliedChangeReviews(ctx, tx, keys, s.clock())
		return err
	})
	return out, err
}

func readAppliedChangeReviews(ctx context.Context, tx pgx.Tx, keys []AppliedChangeKey, now time.Time) (map[AppliedChangeKey]crmcontracts.AppliedDealChangeReview, error) {
	dealIDs, changeIDs := make([]ids.UUID, len(keys)), make([]ids.UUID, len(keys))
	for i, key := range keys {
		dealIDs[i], changeIDs[i] = key.DealID.UUID, key.ChangeID
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos, changesPos, nowPos := arg(dealIDs), arg(changeIDs), arg(now)
	scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	writable, err := auth.WriteAuthorityClauseFor(ctx, dealTable, "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	if writable == "" {
		writable = "TRUE"
	}
	query := storekit.SQLf(`
 WITH requested AS (SELECT * FROM unnest($%d::uuid[], $%d::uuid[]) AS r(deal_id, change_id)),
 visible AS (SELECT d.id, d.version, d.stage_id, to_jsonb(d) AS fields, (%s) AS writable FROM deal d WHERE (%s))
 SELECT c.deal_id, c.audit_log_id, 'close_date', c.accepted_at IS NOT NULL, c.reversed_at IS NOT NULL,
        d.version, c.reversed_at IS NULL,
        c.reversed_at IS NULL AND NOT EXISTS (SELECT 1 FROM jsonb_each(a.after) f WHERE f.key = ANY(c.fields) AND d.fields->f.key IS DISTINCT FROM f.value), d.writable
 FROM deal_correction c JOIN visible d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id AND a.entity_type = 'deal' AND a.entity_id = c.deal_id
 JOIN requested r ON r.deal_id = c.deal_id AND r.change_id = c.audit_log_id
 UNION ALL
 SELECT o.deal_id, o.approval_id, 'stage', o.accepted_at IS NOT NULL, o.reversed_at IS NOT NULL,
        d.version, o.outcome = 'auto_applied' AND o.reversed_at IS NULL
          AND d.stage_id = o.to_stage_id
          AND $%d::timestamptz < o.decided_at + make_interval(hours => coalesce(o.undo_window_hours, p.undo_window_hours, 72)),
        o.outcome = 'auto_applied' AND o.reversed_at IS NULL AND d.stage_id = o.to_stage_id, d.writable
 FROM stage_progression_outcome o JOIN visible d ON d.id = o.deal_id
 JOIN requested r ON r.deal_id = o.deal_id AND r.change_id = o.approval_id
 LEFT JOIN stage_progression_policy p ON p.pipeline_id = o.pipeline_id AND p.from_stage_id = o.from_stage_id AND p.to_stage_id = o.to_stage_id
 WHERE o.outcome IN ('auto_applied', 'reversed')`, dealsPos, changesPos, writable, scope, nowPos)
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
