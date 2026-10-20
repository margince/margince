// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type homeRepairAction string

const (
	repairSupersedeSuggestion homeRepairAction = "supersede_suggestion"
	repairWithdrawProposal    homeRepairAction = "withdraw_proposal"
	repairArchiveReminder     homeRepairAction = "archive_automatic_reminder"
	repairHumanReview         homeRepairAction = "human_review"
	repairLeaveCurrent        homeRepairAction = "leave_current"
)

type homeRepairTaskDecision struct {
	Task        activities.HomeRepairTask
	Action      homeRepairAction
	Reason      string
	RuleVersion string
}

func homeRepairTaskDecisionsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) ([]homeRepairTaskDecision, error) {
	tasks, err := activities.HomeRepairTasksTx(ctx, tx, after, limit)
	if err != nil {
		return nil, err
	}
	decisions := make([]homeRepairTaskDecision, 0, len(tasks))
	for _, task := range tasks {
		decision, err := homeRepairTaskDecisionTx(ctx, tx, task, asOf)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func homeRepairTaskDecisionTx(ctx context.Context, tx pgx.Tx, task activities.HomeRepairTask, asOf time.Time) (homeRepairTaskDecision, error) {
	decision := homeRepairTaskDecision{Task: task, Action: repairHumanReview, RuleVersion: activities.CommitmentAdmissionVersion}
	if !task.Pristine {
		decision.Reason = "human_adoption_not_excluded"
		return decision, nil
	}
	if task.SourceSystem == activities.EmailRequestTaskSource && task.SourceID != nil {
		past, err := activities.RequestReminderPastHorizonTx(ctx, tx, task, asOf)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			decision.Reason = string(activities.CommitmentSourceUnavailable)
			return decision, nil
		}
		if err != nil {
			return decision, err
		}
		decision.Action, decision.Reason = repairLeaveCurrent, "current_work"
		if past {
			decision.Action, decision.Reason = repairArchiveReminder, "request_past_waiting_horizon"
		}
		return decision, nil
	}
	if task.SourceSystem != activities.CommitmentTaskSource || task.SourceID == nil {
		decision.Reason = "source_rule_not_verified"
		return decision, nil
	}
	var err error
	decision.Action, decision.Reason, err = homeRepairCommitmentAdmissionTx(ctx, tx, Commitment{
		SourceActivityID: *task.SourceID, Seat: task.AssigneeID, AsOf: asOf,
	}, task.DueAt, repairArchiveReminder)
	return decision, err
}
