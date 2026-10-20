// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func homeRepairProposalDecisionTx(ctx context.Context, tx pgx.Tx, proposal approvals.RepairProposal, asOf time.Time) (homeRepairTaskRef, error) {
	item := homeRepairTaskRef{ID: proposal.ID.UUID, Source: "approval", Snapshot: proposal.Snapshot, Action: repairHumanReview, Reason: "source_rule_not_verified"}
	if !proposal.Pristine {
		item.Reason = "human_adoption_not_excluded"
		return item, nil
	}
	if proposal.Kind != CommitmentTaskKind {
		return item, nil
	}
	var staged CommitmentTaskProposal
	if err := json.Unmarshal(proposal.Change, &staged); err != nil || staged.SourceActivityID.IsZero() {
		return item, nil
	}
	var due *time.Time
	if staged.DueDate != "" {
		at, err := commitmentDueInstant(ctx, tx, staged.DueDate)
		if err != nil {
			return item, err
		}
		due = &at
	}
	action, reason, err := homeRepairCommitmentAdmissionTx(ctx, tx, Commitment{SourceActivityID: staged.SourceActivityID, Seat: staged.SeatID, AsOf: asOf}, due, repairWithdrawProposal)
	if err != nil {
		return item, err
	}
	item.Action, item.Reason = action, reason
	if action != repairLeaveCurrent {
		return item, nil
	}
	duplicate, err := approvals.DuplicateRepairProposalTx(ctx, tx, proposal.ID, asOf)
	if err != nil {
		return item, err
	}
	if duplicate {
		item.Action, item.Reason = repairWithdrawProposal, "duplicate_pending_proposal"
	}
	return item, nil
}

func validateHomeRepairProposalTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, asOf time.Time) error {
	proposal, err := approvals.RepairProposalTx(ctx, tx, ids.From[ids.ApprovalKind](expected.ID), asOf, false)
	if err != nil {
		return err
	}
	actual, err := homeRepairProposalDecisionTx(ctx, tx, proposal, asOf)
	if err != nil {
		return err
	}
	if actual != expected {
		return apperrors.ErrConflict
	}
	return nil
}

func applyHomeRepairProposal(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	if err := validateHomeRepairProposalTx(ctx, tx, expected, activation.AsOf); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrConflict) {
			return false, recordHomeRepairSkip(ctx, tx, expected, "changed_since_review", activation.RuleVersion, runID)
		}
		return false, err
	}
	if expected.Action != repairWithdrawProposal {
		return false, recordHomeRepairSkip(ctx, tx, expected, expected.Reason, activation.RuleVersion, runID)
	}
	changed, err := approvals.WithdrawReviewedTx(ctx, tx, ids.From[ids.ApprovalKind](expected.ID), expected.Snapshot, "home_repair/"+activation.RuleVersion+"/"+expected.Reason, activation.AsOf)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrConflict) {
		return false, recordHomeRepairSkip(ctx, tx, expected, "changed_since_review", activation.RuleVersion, runID)
	}
	return changed, err
}

func homeRepairCommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, commitment Commitment, due *time.Time, retire homeRepairAction) (homeRepairAction, string, error) {
	reason, err := commitmentAdmissionForDue(ctx, tx, commitment, due)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return repairHumanReview, string(activities.CommitmentSourceUnavailable), nil
	}
	if err != nil {
		return "", "", err
	}
	switch reason {
	case activities.CommitmentCurrent:
		return repairLeaveCurrent, "current_work", nil
	case activities.CommitmentHistorical, activities.CommitmentBeforeBaseline, activities.CommitmentMeetingCancelled:
		return retire, string(reason), nil
	default:
		return repairHumanReview, string(reason), nil
	}
}
