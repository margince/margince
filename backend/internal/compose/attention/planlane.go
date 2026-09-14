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

// PlanWork retains the plan writer’s identity so agenda completion settles the same obligation.
type PlanWork struct {
	ID      ids.UUID
	OwnerID ids.UUID
	Label   string
	DueAt   time.Time
	Subject *crmcontracts.AttentionSubject
}

// WeeklyPlans supplies due commitments through the plan store’s read authority.
type WeeklyPlans interface {
	DuePlan(context.Context, ids.UUID, time.Time) ([]PlanWork, error)
}

// WithWeeklyPlans connects weekly planning to the shared attention ranking.
func (s *Service) WithWeeklyPlans(plans WeeklyPlans) *Service {
	s.weeklyPlans = plans
	return s
}

// Plan work joins the same ranking used by the browser and the worklist tool.
// The request copy carries it; the shared service never holds a reader's plan.
func (s *Service) readingPlan(ctx context.Context, now time.Time) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	if s.weeklyPlans == nil || s.taskScope == TasksUnassigned {
		return &scoped, nil
	}
	if s.taskScope == TasksVisible {
		return &scoped, &crmcontracts.WorklistSourceUnavailable{Source: sourceWeeklyCommitment, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld}
	}
	entries, err := s.weeklyPlans.DuePlan(ctx, s.taskOwner, now)
	if err != nil {
		reason := crmcontracts.WorklistSourceUnavailableReasonFailed
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			reason = crmcontracts.WorklistSourceUnavailableReasonWithheld
		}
		return &scoped, &crmcontracts.WorklistSourceUnavailable{Source: sourceWeeklyCommitment, Reason: reason}
	}
	scoped.planRows = make([]ranked, 0, len(entries))
	for _, entry := range entries {
		due := entry.DueAt
		row := crmcontracts.WorklistItem{
			Id: entry.ID.String(), Source: sourceWeeklyCommitment, Category: "tasks",
			Level: levelPromise, Title: &entry.Label, DueAt: &due, Subject: entry.Subject,
			Consequence: "promise_breaks", Because: []crmcontracts.WorklistReason{},
			Actions: []crmcontracts.WorklistItemActions{},
		}
		stampDeadline(&row, &due, now)
		scoped.planRows = append(scoped.planRows, ranked{item: row, owner: entry.OwnerID, ownerRef: ownedBy(entry.OwnerID), deadlineAt: due, overdue: deadline.Passed(&due, now), occurredAt: now})
	}
	return &scoped, nil
}
