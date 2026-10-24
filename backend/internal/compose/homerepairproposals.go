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
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func homeRepairProposalDecisionTx(ctx context.Context, tx pgx.Tx, proposal approvals.RepairProposal, asOf time.Time, forDecision bool) (homeRepairTaskRef, error) {
	item := homeRepairTaskRef{ID: proposal.ID.UUID, Source: repairSourceApproval, Snapshot: proposal.Snapshot, Action: repairHumanReview, Reason: "source_rule_not_verified"}
	if !proposal.Pristine {
		item.Reason = "human_adoption_not_excluded"
		return item, nil
	}
	var err error
	switch proposal.Kind {
	case CommitmentTaskKind:
		item, err = homeRepairCommitmentProposalTx(ctx, tx, proposal, item, asOf, forDecision)
	case lifecycleProposalKind:
		item, err = homeRepairLifecycleProposalTx(ctx, tx, proposal, item, asOf, forDecision)
	default:
		return item, nil
	}
	if err != nil || item.Action != repairLeaveCurrent {
		return item, err
	}
	duplicate, err := approvals.DuplicateRepairProposalTx(ctx, tx, proposal.ID, asOf, forDecision)
	if err != nil {
		return item, err
	}
	if duplicate {
		item.Action, item.Reason = repairWithdrawProposal, "duplicate_pending_proposal"
	}
	return item, nil
}

func homeRepairCommitmentProposalTx(ctx context.Context, tx pgx.Tx, proposal approvals.RepairProposal, item homeRepairTaskRef, asOf time.Time, forDecision bool) (homeRepairTaskRef, error) {
	var staged CommitmentTaskProposal
	if !decodeRepairCommitment(proposal, &staged) {
		return item, nil
	}
	var due *time.Time
	if staged.DueDate != "" {
		at, err := commitmentDueInstant(ctx, tx, staged.DueDate, staged.DueZone)
		if err != nil {
			return item, err
		}
		due = &at
	}
	var seat *ids.UUID
	if asked, named := approvalsubject.CommitmentAskedOf(proposal.Change); named {
		seat = &asked
	}
	action, reason, err := homeRepairCommitmentAdmissionTx(ctx, tx, Commitment{SourceActivityID: staged.SourceActivityID, Seat: seat, AsOf: asOf, PrivateTo: staged.PrivateTo}, due, repairWithdrawProposal, forDecision)
	if err != nil {
		return item, err
	}
	item.Action, item.Reason = action, reason
	return item, nil
}

func validateHomeRepairProposalTx(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, asOf time.Time, forDecision bool) error {
	proposal, err := approvals.RepairProposalTx(ctx, tx, ids.From[ids.ApprovalKind](expected.ID), asOf, false)
	if err != nil {
		return err
	}
	actual, err := homeRepairProposalDecisionTx(ctx, tx, proposal, asOf, forDecision)
	if err != nil {
		return err
	}
	if actual != expected {
		return apperrors.ErrConflict
	}
	return nil
}

func applyHomeRepairProposal(ctx context.Context, tx pgx.Tx, expected homeRepairTaskRef, activation datatask.Activation, runID ids.UUID) (bool, error) {
	if err := validateHomeRepairProposalTx(ctx, tx, expected, activation.AsOf, true); err != nil {
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

func homeRepairCommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, commitment Commitment, due *time.Time, retire homeRepairAction, forDecision bool) (homeRepairAction, string, error) {
	read, err := readCommitmentEvidence(ctx, tx, commitment, due, forDecision)
	reason := read.Reason
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return repairHumanReview, string(activities.CommitmentSourceUnavailable), nil
	}
	if err != nil {
		return "", "", err
	}
	if retire == repairWithdrawProposal && commitment.proposeLate(read) {
		return repairLeaveCurrent, string(CommitmentHistoricalProposed), nil
	}
	switch reason {
	case activities.CommitmentCurrent:
		return repairLeaveCurrent, repairCurrentWork, nil
	case activities.CommitmentHistorical, activities.CommitmentBeforeBaseline, activities.CommitmentMeetingCancelled:
		return retire, string(reason), nil
	default:
		return repairHumanReview, string(reason), nil
	}
}

func decodeRepairCommitment(proposal approvals.RepairProposal, staged *CommitmentTaskProposal) bool {
	return json.Unmarshal(proposal.Change, staged) == nil && !staged.SourceActivityID.IsZero() && proposal.TargetID != nil && staged.SourceActivityID == *proposal.TargetID
}
