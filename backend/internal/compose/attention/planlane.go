// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

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
func (s *Service) readingPlan(ctx context.Context, scope string, now time.Time) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	if s.weeklyPlans == nil || s.taskScope == TasksUnassigned {
		return &scoped, nil
	}
	if s.taskScope == TasksVisible {
		if scope == scopeTeam {
			return s.readingTeamPlans(ctx, now)
		}
		// All has no roster to read plans across until a policy says which members
		// it covers: every member of every team the reader oversees, or none.
		return &scoped, planUnavailable(crmcontracts.WorklistSourceUnavailableReasonWithheld)
	}
	entries, refusal := s.duePlan(ctx, laneBudget, s.taskOwner, now)
	scoped.planRows = planRowsOf(entries, now)
	return &scoped, refusal
}

// readingTeamPlans reads each roster member's plan through the plan store's own
// lead authority. A member whose read fails is left uncovered, not fatal to the page.
func (s *Service) readingTeamPlans(ctx context.Context, now time.Time) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	// Fails closed, as keepTeams does: without a roster there is no team to read for.
	if s.teammates == nil {
		return &scoped, planUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
	}
	roster, cut, err := s.degradableRoster(ctx)
	if err != nil {
		return &scoped, planUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
	}
	coverage := crmcontracts.WorklistPlanCoverage{
		Members: make([]crmcontracts.WorklistPlanCoverageMember, 0, len(roster)), Truncated: cut,
	}
	var refused *crmcontracts.WorklistSourceUnavailable
	// One lane budget for the whole team, checked between reads rather than
	// carried on ctx: a degradable read rolls back on its own ctx, so an expired
	// one would abort the snapshot every later lane shares.
	spent := s.now().Add(laneBudget)
	for _, member := range roster {
		var entries []PlanWork
		refusal := planUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
		// Each read gets what is left, so one started late cannot take a fresh budget.
		if left := spent.Sub(s.now()); left > 0 {
			entries, refusal = s.duePlan(ctx, left, member.UserID, now)
		}
		refused = louder(refused, refusal)
		coverage.Members = append(coverage.Members, crmcontracts.WorklistPlanCoverageMember{
			UserId: openapi_types.UUID(member.UserID), DisplayName: member.DisplayName, Read: refusal == nil,
		})
		scoped.planRows = append(scoped.planRows, planRowsOf(entries, now)...)
	}
	scoped.planCoverage = &coverage
	if anyPlanRead(coverage.Members) {
		return &scoped, nil
	}
	return &scoped, refused
}

// duePlan is one owner's due commitments, read so a failure is named by the page
// rather than aborting the snapshot every other source shares.
func (s *Service) duePlan(ctx context.Context, budget time.Duration, owner ids.UUID, now time.Time) ([]PlanWork, *crmcontracts.WorklistSourceUnavailable) {
	var entries []PlanWork
	err := s.degradable(ctx, budget, func(ctx context.Context) error {
		var err error
		entries, err = s.weeklyPlans.DuePlan(ctx, owner, now)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return nil, planUnavailable(crmcontracts.WorklistSourceUnavailableReasonWithheld)
	case err != nil:
		return nil, planUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
	}
	return entries, nil
}

// louder is the refusal a team reports: a failed read outranks a withheld one,
// and withheld stands only when every refusal was one.
func louder(held, next *crmcontracts.WorklistSourceUnavailable) *crmcontracts.WorklistSourceUnavailable {
	if held == nil || (next != nil && next.Reason != crmcontracts.WorklistSourceUnavailableReasonWithheld) {
		return next
	}
	return held
}

func anyPlanRead(members []crmcontracts.WorklistPlanCoverageMember) bool {
	for _, member := range members {
		if member.Read {
			return true
		}
	}
	return false
}

func planUnavailable(reason crmcontracts.WorklistSourceUnavailableReason) *crmcontracts.WorklistSourceUnavailable {
	return &crmcontracts.WorklistSourceUnavailable{Source: sourceWeeklyCommitment, Reason: reason}
}

// planRowsOf ranks due commitments under the owner who wrote them, so the scope
// filters judge a teammate's promise as theirs rather than the reader's.
func planRowsOf(entries []PlanWork, now time.Time) []ranked {
	rows := make([]ranked, 0, len(entries))
	for _, entry := range entries {
		due := entry.DueAt
		row := crmcontracts.WorklistItem{
			Id: entry.ID.String(), Source: sourceWeeklyCommitment, Category: "tasks",
			Level: levelPromise, Title: &entry.Label, DueAt: &due, Subject: entry.Subject,
			Consequence: "promise_breaks", Because: []crmcontracts.WorklistReason{},
			Actions: []crmcontracts.WorklistItemActions{},
		}
		stampDeadline(&row, &due, now)
		rows = append(rows, ranked{item: row, owner: entry.OwnerID, ownerRef: ownedBy(entry.OwnerID), deadlineAt: due, overdue: deadline.Passed(&due, now), occurredAt: now})
	}
	return rows
}
