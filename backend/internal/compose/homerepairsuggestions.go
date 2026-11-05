// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func homeRepairSuggestionRef(repair deals.SuggestionRepair) homeRepairTaskRef {
	action := repairLeaveCurrent
	if repair.Supersede {
		action = repairSupersedeSuggestion
	} else if repair.Reason != deals.RepairReasonCurrentWork {
		action = repairHumanReview
	}
	return homeRepairTaskRef{Source: repairSourceSuggestion, ID: repair.ID, Version: repair.Version, Action: action, Reason: repair.Reason}
}

func previewHomeRepairSuggestionsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	repairs, err := deals.SuggestionRepairPageTx(ctx, tx, after, limit, asOf, dealSuggestionEffects{}.BuyerStillVerified)
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, len(repairs))}
	for _, repair := range repairs {
		scope.Tasks = append(scope.Tasks, homeRepairSuggestionRef(repair))
	}
	return homeRepairReview(scope, asOf)
}

func validateHomeRepairSuggestionTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, asOf time.Time) error {
	if err := deals.RequireSuggestionDecision(ctx, false); err != nil {
		return err
	}
	actual, err := deals.PreviewSuggestionRepairTx(ctx, tx, expected.ID, asOf, dealSuggestionEffects{}.BuyerStillVerified)
	if err != nil {
		return err
	}
	if homeRepairSuggestionRef(actual) != expected {
		return apperrors.ErrConflict
	}
	return nil
}

func applyHomeRepairSuggestion(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	if expected.Action != repairSupersedeSuggestion {
		return false, recordHomeRepairSkip(ctx, tx, expected, expected.Reason, activation.RuleVersion, runID)
	}
	reviewed := deals.SuggestionRepair{ID: expected.ID, Version: expected.Version, Reason: expected.Reason, Supersede: true}
	changed, err := deals.SupersedeReviewedSuggestionTx(ctx, tx, reviewed, deals.SuggestionRepairProvenance{RuleVersion: activation.RuleVersion, RunID: runID}, activation.AsOf, dealSuggestionEffects{}.BuyerStillVerified)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, recordHomeRepairSkip(ctx, tx, expected, "permission_no_longer_available", activation.RuleVersion, runID)
	}
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
		return false, recordHomeRepairSkip(ctx, tx, expected, "changed_since_review", activation.RuleVersion, runID)
	}
	return changed, err
}
