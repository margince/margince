// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// goalAccess is what the caller may do with one seller's goals. A caller
// outside the seller's reach gets ErrNotFound instead, whether or not a plan
// exists, so the refusal says nothing about the plan.
type goalAccess struct {
	self bool
	// edit is drafting, publishing, discarding and withdrawing.
	edit bool
	// drafts is reading the shared draft, its reasons and the history: the
	// seller and those who set their goals, never a peer.
	drafts bool
}

func (s *Service) goalAccess(ctx context.Context, tx pgx.Tx, owner ids.UUID) (goalAccess, error) {
	if err := auth.Require(ctx, "sales_target", principal.ActionRead); err != nil {
		return goalAccess{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return goalAccess{}, apperrors.ErrPermissionDenied
	}
	// The switch's own rule names the reader's seat, which an agent on a
	// passport shares with its human.
	self := auth.MayMeasureSeat(ctx, false, owner)
	if !self {
		// The individual-performance switch and the reader's population both
		// live in this one rule, so a goal is read where its figures are.
		measured, err := s.authority.MeasurableOwners(ctx, tx, []ids.UUID{owner})
		if err != nil {
			return goalAccess{}, err
		}
		if len(measured) == 0 {
			return goalAccess{}, apperrors.ErrNotFound
		}
	}
	edit, err := s.goalWriteReaches(ctx, tx, actor, owner)
	if err != nil {
		return goalAccess{}, err
	}
	return goalAccess{self: self, edit: edit, drafts: self || edit}, nil
}

// goalWriteReaches asks the target route's own write rule of an owner scope.
// A grant to read revenue alone never passes it.
func (s *Service) goalWriteReaches(ctx context.Context, tx pgx.Tx, actor principal.Principal, owner ids.UUID) (bool, error) {
	if !actor.HumanOrItsAgent() || !actor.SeatType.CanMutate() {
		return false, nil
	}
	id := openapi_types.UUID(owner)
	return s.writeReaches(ctx, tx, crmcontracts.ReportingScope{Kind: reportingOwner, Id: &id})
}

// requireGoalEdit refuses a plan write the caller may not make.
func (s *Service) requireGoalEdit(ctx context.Context, tx pgx.Tx, owner ids.UUID) error {
	access, err := s.goalAccess(ctx, tx, owner)
	if err != nil {
		return err
	}
	if !access.edit {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// planOwnedIdentity reports the seller and year a target input belongs to when
// it is a month a sales goal plan can own.
func planOwnedIdentity(in crmcontracts.ReportingTargetInput) (ids.UUID, int, bool) {
	if in.Metric != goalMetric || in.Scope.Kind != reportingOwner || in.Scope.Id == nil || in.PipelineId != nil || in.PeriodKind != crmcontracts.ReportingTargetInputPeriodKindMonth {
		return ids.Nil, 0, false
	}
	return ids.UUID(*in.Scope.Id), in.PeriodStart.Year(), true
}

// planOwnedSQL is planOwnedIdentity over a published plan, asked of the
// target a statement aliases t with its current revision r.
const planOwnedSQL = `(t.metric='` + string(goalMetric) + `' AND t.scope_kind='owner' AND t.pipeline_id IS NULL AND t.period_kind='month' AND EXISTS (
	SELECT 1 FROM sales_goal_plan p WHERE p.owner_id=t.scope_id AND p.published_revision IS NOT NULL
	AND p.year=left(r.definition->>'period_start',4)::int))`

// refusePlanOwned keeps the target routes off the months a published plan
// owns, so the twelve months always add up to the plan. It takes the same lock
// a publication takes, so the two cannot interleave.
func (s *Service) refusePlanOwned(ctx context.Context, tx pgx.Tx, in crmcontracts.ReportingTargetInput) error {
	owner, year, eligible := planOwnedIdentity(in)
	if !eligible {
		return nil
	}
	if err := lockGoalYear(ctx, tx, owner, year); err != nil {
		return err
	}
	plan, err := loadGoalPlan(ctx, tx, owner, year)
	if err != nil {
		return err
	}
	if plan != nil && plan.Published != nil {
		return refuse("period_start", "plan_owned", "this month belongs to the published sales goals for "+strconv.Itoa(year)+"; change it on the Sales goals page")
	}
	return nil
}

// goalWriteReachesCaller is goalWriteReaches for the caller in ctx.
func (s *Service) goalWriteReachesCaller(ctx context.Context, tx pgx.Tx, owner ids.UUID) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	return s.goalWriteReaches(ctx, tx, actor, owner)
}

// CanSetGoals answers whether the caller may set this seller's sales goals,
// under the same rule the plan writes enforce.
func (s *Service) CanSetGoals(ctx context.Context, tx pgx.Tx, owner ids.UUID) (bool, error) {
	access, err := s.goalAccess(ctx, tx, owner)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return access.edit, err
}

// GoalPlanVisible refuses a caller who may no longer read this seller's
// draft and history, which every plan write answers with.
func (s *Service) GoalPlanVisible(ctx context.Context, owner ids.UUID) error {
	return s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		access, err := s.goalAccess(ctx, tx, owner)
		if err != nil {
			return err
		}
		if !access.drafts {
			return apperrors.ErrNotFound
		}
		return nil
	})
}
