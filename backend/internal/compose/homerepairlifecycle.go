// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func homeRepairLifecycleProposalTx(ctx context.Context, tx pgx.Tx, proposal approvals.RepairProposal, item homeRepairTaskRef, asOf time.Time, forDecision bool) (homeRepairTaskRef, error) {
	var staged lifecycleProposal
	if err := json.Unmarshal(proposal.Change, &staged); err != nil || staged.Evidence == nil ||
		proposal.TargetID == nil || proposal.TargetType == nil || *proposal.TargetType != "company" ||
		staged.CompanyID.UUID != *proposal.TargetID || staged.SignalID != staged.Evidence.SignalID {
		return item, nil
	}
	reconcile := lifecyclereview.Reconcile
	if forDecision {
		reconcile = lifecyclereview.ReconcileForDecision
	}
	current, err := reconcile(ctx, tx, staged.CompanyID, asOf)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		item.Reason = "source_unavailable"
		return item, nil
	}
	if err != nil {
		return item, err
	}
	if current.Incomplete || current.SourceID.IsZero() || current.Scope == "unknown" {
		return item, nil
	}
	switch {
	case current.ActiveContract || current.ActiveDelivery:
		item.Action, item.Reason = repairWithdrawProposal, "continuing_engagement"
	case current.Scope == "workstream":
		item.Action, item.Reason = repairWithdrawProposal, "ending_is_not_account_wide"
	case !lifecyclereview.Matches(current, staged.Evidence):
		item.Action, item.Reason = repairWithdrawProposal, "lifecycle_evidence_changed"
	default:
		item.Action, item.Reason = repairLeaveCurrent, "current_work"
	}
	return item, nil
}
