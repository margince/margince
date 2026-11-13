// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// GoalSellers is whose sales goals a reading of one scope adds up.
type GoalSellers struct {
	Team bool
	// Owners are the sellers whose goals count, each one the reader measures.
	Owners []ids.UUID
	// Withheld is a team the reader may not measure member by member. Its
	// goal is the sum of the members' goals, so it is withheld with them.
	Withheld bool
}

// GoalSellers resolves the sellers whose goals make up the goal of scope.
func (s *Service) GoalSellers(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) (GoalSellers, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return GoalSellers{}, err
	}
	if scope.Kind == reportingOwner && scope.Id != nil {
		return GoalSellers{Owners: []ids.UUID{ids.UUID(*scope.Id)}}, nil
	}
	measured, err := s.authority.MeasurableOwnerSet(ctx, tx)
	if err != nil {
		return GoalSellers{}, err
	}
	return s.goalSellersAmong(ctx, tx, scope, measured)
}

func (s *Service) goalSellersAmong(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, measured MeasurableOwners) (GoalSellers, error) {
	members, err := s.authority.MembersFor(ctx, tx, scope)
	if err != nil {
		return GoalSellers{}, err
	}
	out := GoalSellers{Team: true}
	for _, member := range members {
		if measured.Includes(member) && !measured.IndividualOff {
			out.Owners = append(out.Owners, member)
		} else {
			out.Withheld = true
		}
	}
	return out, nil
}

// GoalScope resolves the scope a goal reading names under the strict tier
// rule. The goal of a team is its members' goals, so reading it takes the
// team inside the reader's own lens. A read of its totals does not.
func (s *Service) GoalScope(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) (crmcontracts.ReportingScope, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingScope{}, err
	}
	return s.authority.AssertedScope(ctx, tx, scope)
}

// OwnerGoals holds the published monthly goals of one calendar year. It is
// the one place a goal is added up over sellers and months.
type OwnerGoals struct {
	Readable bool
	Withheld bool
	Currency string
	Years    map[ids.UUID]GoalYear
	Drift    map[ids.UUID]bool
}

// ReadOwnerGoals reads the sellers' goals for year, and which of their plans
// another route changed.
func (s *Service) ReadOwnerGoals(ctx context.Context, tx pgx.Tx, sellers GoalSellers, year int, currency string) (OwnerGoals, error) {
	out := OwnerGoals{Withheld: sellers.Withheld, Currency: currency}
	var err error
	out.Years, out.Readable, err = s.OwnerGoalYears(ctx, tx, sellers.Owners, year)
	if err != nil || !out.Readable {
		return out, err
	}
	out.Drift, err = s.GoalPlanDrift(ctx, tx, sellers.Owners, year)
	return out, err
}

// Over is the goal of owners across months. yearly withholds the comparison
// from a plan another route changed, until a manager reviews it.
func (g OwnerGoals) Over(owners []ids.UUID, months []int, yearly bool) crmcontracts.SalesProgressGoal {
	out := crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusMissing, MonthsNeeded: len(months)}
	if !g.Readable || g.Withheld {
		out.Status = crmcontracts.SalesProgressGoalStatusWithheld
		return out
	}
	sum := g.sum(owners, months)
	out.MonthsSet = sum.monthsSet
	switch {
	case sum.overflow:
		out.Status = crmcontracts.SalesProgressGoalStatusWithheld
		return out
	case sum.foreign:
		out.Status = crmcontracts.SalesProgressGoalStatusOtherCurrency
		return out
	case !sum.known:
		return out
	}
	out.AmountMinor = &sum.total
	switch {
	case out.MonthsSet < len(months):
		out.Status = crmcontracts.SalesProgressGoalStatusIncomplete
	case yearly && slices.ContainsFunc(owners, func(owner ids.UUID) bool { return g.Drift[owner] }):
		out.Status = crmcontracts.SalesProgressGoalStatusChangedOutsidePlan
	case sum.total == 0:
		out.Status = crmcontracts.SalesProgressGoalStatusZero
	default:
		out.Status = crmcontracts.SalesProgressGoalStatusSet
	}
	return out
}

// goalSum adds the goal cells of owners across months in the reading currency.
// A month counts as set only when every owner has a cell in that currency.
type goalSum struct {
	total                    int64
	monthsSet                int
	known, foreign, overflow bool
}

func (g OwnerGoals) sum(owners []ids.UUID, months []int) goalSum {
	var sum goalSum
	for _, month := range months {
		complete := len(owners) > 0
		for _, owner := range owners {
			cell, ok := g.Years[owner][month]
			switch {
			case !ok:
				complete = false
			case cell.Unit != g.Currency:
				sum.foreign, complete = true, false
			case sum.total > values.MaxExactInteger-cell.Value:
				sum.overflow = true
				return sum
			default:
				sum.total += cell.Value
				sum.known = true
			}
		}
		if complete {
			sum.monthsSet++
		}
	}
	return sum
}
