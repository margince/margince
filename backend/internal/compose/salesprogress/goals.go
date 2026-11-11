// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// progressSellers is whose goals and figures a reading adds up.
type progressSellers struct {
	team bool
	// owners are the sellers whose goals count, each one the reader measures.
	owners []ids.UUID
	// withheld is a team the reader may not measure member by member. Its
	// goal is the sum of the members' goals, so it is withheld with them.
	withheld bool
}

func (h Reading) sellers(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext) (progressSellers, error) {
	if frame.Scope.Kind == crmcontracts.ReportingScopeKindOwner && frame.Scope.Id != nil {
		return progressSellers{owners: []ids.UUID{ids.UUID(*frame.Scope.Id)}}, nil
	}
	members, err := h.Members(ctx, tx, frame.Scope)
	if err != nil {
		return progressSellers{}, err
	}
	measured, err := h.Measurable(ctx, tx)
	if err != nil {
		return progressSellers{}, err
	}
	out := progressSellers{team: true}
	for _, member := range members {
		if measured.Includes(member) && !measured.IndividualOff {
			out.owners = append(out.owners, member)
		} else {
			out.withheld = true
		}
	}
	return out, nil
}

// progressGoals holds the goals a reading compares against.
type progressGoals struct {
	readable bool
	withheld bool
	currency string
	years    map[ids.UUID]reporting.GoalYear
	drift    map[ids.UUID]bool
}

func (h Reading) goals(ctx context.Context, tx pgx.Tx, frame progressFrame, sellers progressSellers) (progressGoals, error) {
	out := progressGoals{withheld: sellers.withheld, currency: frame.currency}
	var err error
	out.years, out.readable, err = h.Reports.OwnerGoalYears(ctx, tx, sellers.owners, frame.period.year)
	if err != nil || !out.readable {
		return out, err
	}
	out.drift, err = h.Reports.GoalPlanDrift(ctx, tx, sellers.owners, frame.period.year)
	return out, err
}

// over is the goal of owners across months. yearly withholds the comparison
// from a plan another route changed, until a manager reviews it.
func (g progressGoals) over(owners []ids.UUID, months []int, yearly bool) crmcontracts.SalesProgressGoal {
	out := crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusMissing, MonthsNeeded: len(months)}
	if !g.readable || g.withheld {
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
	case yearly && slices.ContainsFunc(owners, func(owner ids.UUID) bool { return g.drift[owner] }):
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

func (g progressGoals) sum(owners []ids.UUID, months []int) goalSum {
	var sum goalSum
	for _, month := range months {
		complete := len(owners) > 0
		for _, owner := range owners {
			cell, ok := g.years[owner][month]
			switch {
			case !ok:
				complete = false
			case cell.Unit != g.currency:
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

// compared fills completion and delta, which only a complete goal over a
// complete amount supports. A zero goal has a delta and no completion.
func compared(closed crmcontracts.SalesProgressAmount, goal crmcontracts.SalesProgressGoal) (completion *float64, delta *int64) {
	if closed.Status != crmcontracts.SalesProgressAmountStatusOk || closed.AmountMinor == nil || goal.AmountMinor == nil || (goal.Status != crmcontracts.SalesProgressGoalStatusSet && goal.Status != crmcontracts.SalesProgressGoalStatusZero) {
		return nil, nil
	}
	difference := *closed.AmountMinor - *goal.AmountMinor
	delta = &difference
	if goal.Status == crmcontracts.SalesProgressGoalStatusSet {
		ratio := float64(*closed.AmountMinor) / float64(*goal.AmountMinor)
		completion = &ratio
	}
	return completion, delta
}
