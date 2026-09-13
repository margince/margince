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

type acceptedChangeAudit struct {
	ID       string `json:"id"`
	Accepted bool   `json:"accepted"`
}

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
		before := map[string]any{"applied_change": acceptedChangeAudit{ID: changeID.String(), Accepted: false}}
		after := map[string]any{"applied_change": acceptedChangeAudit{ID: changeID.String(), Accepted: true}}
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", dealID.UUID, before, after)
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
	key := AppliedChangeKey{DealID: dealID, ChangeID: changeID}
	reviews, err := readAppliedChangeReviews(ctx, tx, []AppliedChangeKey{key}, now)
	if err != nil {
		return crmcontracts.AppliedDealChangeReview{}, err
	}
	review, ok := reviews[key]
	if !ok {
		return review, apperrors.ErrNotFound
	}
	return review, nil
}

// AppliedChangeReview shares the batch reader's scope and current-state checks.
func (s *Store) AppliedChangeReview(ctx context.Context, dealID ids.DealID, changeID ids.UUID) (crmcontracts.AppliedDealChangeReview, error) {
	key := AppliedChangeKey{DealID: dealID, ChangeID: changeID}
	reviews, err := s.AppliedChangeReviews(ctx, []AppliedChangeKey{key})
	if err != nil {
		return crmcontracts.AppliedDealChangeReview{}, err
	}
	review, ok := reviews[key]
	if !ok {
		return review, apperrors.ErrNotFound
	}
	return review, nil
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
