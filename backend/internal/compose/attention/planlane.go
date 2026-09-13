// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sourceWeeklyCommitment = "weekly_commitment"

type PlanWork struct {
	ID      ids.UUID
	OwnerID ids.UUID
	Label   string
	DueAt   time.Time
	Subject *crmcontracts.AttentionSubject
}

type WeeklyPlans interface {
	DuePlan(context.Context, ids.UUID, time.Time) ([]PlanWork, error)
}

func (s *Service) WithWeeklyPlans(plans WeeklyPlans) *Service {
	s.weeklyPlans = plans
	return s
}

// Plan work joins the same ranking used by the browser and the worklist tool.
// The request copy carries it; the shared service never holds a reader's plan.
func (s *Service) readingPlan(ctx context.Context, now time.Time) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	copy := *s
	if s.weeklyPlans == nil || s.taskScope == TasksUnassigned {
		return &copy, nil
	}
	entries, err := s.weeklyPlans.DuePlan(ctx, s.taskOwner, now)
	if err != nil {
		reason := crmcontracts.WorklistSourceUnavailableReasonFailed
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			reason = crmcontracts.WorklistSourceUnavailableReasonWithheld
		}
		return &copy, &crmcontracts.WorklistSourceUnavailable{Source: sourceWeeklyCommitment, Reason: reason}
	}
	copy.planRows = make([]ranked, 0, len(entries))
	for _, entry := range entries {
		due := entry.DueAt
		row := crmcontracts.WorklistItem{
			Id: entry.ID.String(), Source: sourceWeeklyCommitment, Category: "tasks",
			Level: levelAgreed, Title: &entry.Label, DueAt: &due, Subject: entry.Subject,
			Consequence: "promise_breaks", Because: []crmcontracts.WorklistReason{},
			Actions: []crmcontracts.WorklistItemActions{},
		}
		if deadline.Passed(&due, now) {
			row.Level = levelPromise
		}
		stampDeadline(&row, &due, now)
		copy.planRows = append(copy.planRows, ranked{item: row, owner: entry.OwnerID, ownerRef: ownedBy(entry.OwnerID), deadlineAt: due, overdue: deadline.Passed(&due, now), occurredAt: now})
	}
	return &copy, nil
}
