// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AcceptAppliedChange records agreement with one applied change, not a new
// approval or a replay of its effects. The audit records the deciding human.
func (s *Store) AcceptAppliedChange(ctx context.Context, dealID ids.DealID, changeID ids.UUID, version int64) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAcceptedCorrection(ctx, tx, dealID, changeID); err != nil {
			return err
		}
		if err := lockDealForReversal(ctx, tx, dealID); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		review, err := readAppliedChangeReview(ctx, tx, dealID, changeID, s.clock())
		if err != nil {
			return err
		}
		if review.Version != version {
			return apperrors.ErrVersionSkew
		}
		if review.Reversed || !review.CanAccept {
			return apperrors.ErrConflict
		}
		if review.Accepted {
			return nil
		}
		if err := stampChangeAccepted(ctx, tx, dealID, changeID, review.Kind); err != nil {
			return err
		}
		after := map[string]any{"accepted_change_id": changeID.String()}
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", dealID.UUID, map[string]any{"accepted_change_id": nil}, after)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, dealID.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: after})
	})
}

func stampChangeAccepted(ctx context.Context, tx pgx.Tx, dealID ids.DealID, changeID ids.UUID, kind crmcontracts.AppliedDealChangeReviewKind) error {
	args := []any{dealID.UUID, changeID}
	query := storekit.SQLf(`UPDATE deal_correction SET accepted_at = now() WHERE deal_id = $%d AND audit_log_id = $%d AND reversed_at IS NULL`, len(args)-1, len(args))
	if kind == "stage" {
		query = storekit.SQLf(`UPDATE stage_progression_outcome SET accepted_at = now() WHERE deal_id = $%d AND approval_id = $%d AND reversed_at IS NULL AND outcome = 'auto_applied'`, len(args)-1, len(args))
	}
	result, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("record acceptance of the change: %w", err)
	}
	if result.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	return nil
}

func readAppliedChangeReview(ctx context.Context, tx pgx.Tx, dealID ids.DealID, changeID ids.UUID, now time.Time) (crmcontracts.AppliedDealChangeReview, error) {
	var out crmcontracts.AppliedDealChangeReview
	args := []any{dealID.UUID, changeID, now}
	query := storekit.SQLf(`
 SELECT 'close_date', c.accepted_at IS NOT NULL, c.reversed_at IS NOT NULL,
        d.version, c.reversed_at IS NULL,
        c.reversed_at IS NULL AND NOT EXISTS (SELECT 1 FROM jsonb_each(a.after) f WHERE f.key = ANY(c.fields) AND to_jsonb(d)->f.key IS DISTINCT FROM f.value)
 FROM deal_correction c JOIN deal d ON d.id = c.deal_id JOIN audit_log a ON a.id = c.audit_log_id
 WHERE c.deal_id = $%d AND c.audit_log_id = $%d
 UNION ALL
 SELECT 'stage', o.accepted_at IS NOT NULL, o.reversed_at IS NOT NULL,
        d.version, o.outcome = 'auto_applied' AND o.reversed_at IS NULL
          AND d.stage_id = o.to_stage_id
          AND $%d::timestamptz < o.decided_at + make_interval(hours => coalesce(o.undo_window_hours, p.undo_window_hours, 72)),
        o.outcome = 'auto_applied' AND o.reversed_at IS NULL AND d.stage_id = o.to_stage_id
 FROM stage_progression_outcome o JOIN deal d ON d.id = o.deal_id
 LEFT JOIN stage_progression_policy p ON p.pipeline_id = o.pipeline_id AND p.from_stage_id = o.from_stage_id AND p.to_stage_id = o.to_stage_id
 WHERE o.deal_id = $%d AND o.approval_id = $%d AND o.outcome IN ('auto_applied', 'reversed')`, len(args)-2, len(args)-1, len(args), len(args)-2, len(args)-1)
	err := tx.QueryRow(ctx, query, args...).Scan(&out.Kind, &out.Accepted, &out.Reversed, &out.Version, &out.CanUndo, &out.CanAccept)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	out.CanUndo = out.CanUndo && out.CanAccept
	return out, err
}

// AppliedChangeReview re-gates the record even when the caller already read a
// receipt; that receipt may have survived a permission or ownership change.
func (s *Store) AppliedChangeReview(ctx context.Context, dealID ids.DealID, changeID ids.UUID) (crmcontracts.AppliedDealChangeReview, error) {
	var out crmcontracts.AppliedDealChangeReview
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		var err error
		out, err = readAppliedChangeReview(ctx, tx, dealID, changeID, s.clock())
		if err != nil {
			return err
		}
		out.Writable, err = auth.WritableBy(ctx, tx, dealTable, dealID.UUID)
		return err
	})
	return out, err
}

// Correction reversal takes the correction lock before the deal lock. Acceptance
// follows that order too; stage progression has no correction row to lock.
func lockAcceptedCorrection(ctx context.Context, tx pgx.Tx, dealID ids.DealID, changeID ids.UUID) error {
	args := []any{dealID.UUID, changeID}
	var id ids.UUID
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT id FROM deal_correction WHERE deal_id = $%d AND audit_log_id = $%d FOR UPDATE`, len(args)-1, len(args)), args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
