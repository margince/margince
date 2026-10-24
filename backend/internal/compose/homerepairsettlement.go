// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const repairRevalidateSettlement homeRepairAction = "revalidate_settlement"

// settlementRepairDecision maps a legacy verdict's category to what
// activation does. Only a revalidation writes; every other category is kept
// and named, so the preview says what stays and why.
func settlementRepairDecision(category activities.SettlementRepairCategory) (homeRepairAction, string, bool) {
	switch category {
	case activities.SettlementRevalidate:
		return repairRevalidateSettlement, "settlement_revalidate", true
	case activities.SettlementProvenanceUnknown:
		return repairHumanReview, "settlement_provenance_unknown", true
	case activities.SettlementHumanWork:
		return repairLeaveCurrent, "settlement_human_work", true
	case activities.SettlementPastHorizon:
		return repairLeaveCurrent, "settlement_past_horizon", true
	case activities.SettlementNoAnswers:
		return repairHumanReview, "settlement_no_usable_answers", true
	case activities.SettlementPending:
		return repairLeaveCurrent, "settlement_revalidation_pending", true
	case activities.SettlementUnavailable:
		return repairHumanReview, "settlement_unavailable", true
	case activities.SettlementChanged:
		return repairLeaveCurrent, "settlement_changed_since_listed", true
	default:
		return "", "", false
	}
}

func homeRepairSettlementRef(item activities.SettlementRepairItem) (homeRepairTaskRef, error) {
	ref := homeRepairTaskRef{Source: repairSourceSettlement, ID: item.RequestID, Version: item.Version, Snapshot: item.Snapshot}
	var known bool
	ref.Action, ref.Reason, known = settlementRepairDecision(item.Category)
	if !known {
		return homeRepairTaskRef{}, apperrors.ErrConflict
	}
	return ref, nil
}

func previewHomeRepairSettlementsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) (datatask.ReviewRequest, error) {
	items, err := activities.SettlementRepairPageTx(ctx, tx, after, limit, asOf)
	if err != nil {
		return datatask.ReviewRequest{}, err
	}
	scope := homeRepairScope{Tasks: make([]homeRepairTaskRef, 0, len(items))}
	for _, item := range items {
		ref, err := homeRepairSettlementRef(item)
		if err != nil {
			return datatask.ReviewRequest{}, err
		}
		scope.Tasks = append(scope.Tasks, ref)
	}
	return homeRepairReview(scope, asOf)
}

func validateHomeRepairSettlementTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, asOf time.Time) error {
	item, err := activities.SettlementRepairCurrentTx(ctx, tx, expected.ID, asOf)
	if err != nil {
		return err
	}
	ref, err := homeRepairSettlementRef(item)
	if err != nil || ref != expected {
		return apperrors.ErrConflict
	}
	return nil
}

func applyHomeRepairSettlement(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	if expected.Action != repairRevalidateSettlement {
		return false, recordHomeRepairSkip(ctx, tx, expected, expected.Reason, activation.RuleVersion, runID)
	}
	wrote, err := activities.RequestSettlementRevalidationTx(ctx, tx, activities.SettlementRepairItem{
		RequestID: expected.ID, Version: expected.Version, Snapshot: expected.Snapshot, Category: activities.SettlementRevalidate,
	}, activation.AsOf)
	if err != nil {
		return false, err
	}
	if !wrote {
		return false, recordHomeRepairSkip(ctx, tx, expected, "changed_since_review", activation.RuleVersion, runID)
	}
	return true, nil
}
