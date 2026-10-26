// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithAssigneeCheck connects contextual reads to the task writer's eligibility rule.
func (s *Service) WithAssigneeCheck(check func(context.Context, pgx.Tx, ids.UserID) (bool, error)) *Service {
	s.assigneeCanHoldWork = check
	return s
}

func (s *Service) readAssignmentStates(ctx context.Context, approvals []crmcontracts.Approval) error {
	assignees := commitmentAssignees(approvals)
	if len(assignees) == 0 || s.assigneeCanHoldWork == nil {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return s.assignmentStates(ctx, tx, approvals, assignees) })
}

// Only assignees of already-visible proposals reach the writer's check.
func (s *Service) assignmentStates(ctx context.Context, tx pgx.Tx, approvals []crmcontracts.Approval, assignees map[ids.UUID][]int) error {
	if s.assigneeCanHoldWork == nil {
		return nil
	}
	for id, indices := range assignees {
		eligible, err := s.assigneeCanHoldWork(ctx, tx, ids.From[ids.UserKind](id))
		if err != nil {
			return err
		}
		for _, index := range indices {
			approvals[index].AssigneeInactive = new(!eligible)
		}
	}
	return nil
}

func commitmentAssignees(approvals []crmcontracts.Approval) map[ids.UUID][]int {
	assignees := make(map[ids.UUID][]int)
	for index, approval := range approvals {
		if approval.Kind != kindCommitmentTask || approval.Status != crmcontracts.ApprovalStatusPending || approval.ProposedChange == nil {
			continue
		}
		raw, err := json.Marshal(*approval.ProposedChange)
		if err != nil {
			continue
		}
		if id, named := approvalsubject.CommitmentAskedOf(raw); named {
			assignees[id] = append(assignees[id], index)
		}
	}
	return assignees
}
